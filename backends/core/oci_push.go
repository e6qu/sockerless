package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// OCIPushOptions configures an OCI push operation.
type OCIPushOptions struct {
	Registry   string // e.g. "us-docker.pkg.dev"
	Repository string // e.g. "myproject/myrepo/myimage"
	Tag        string // e.g. "latest"
	AuthToken  string // Bearer token or empty
	// LayerContent provides real layer data (keyed by digest).
	LayerContent func(digest string) ([]byte, bool)
	// ImageLayers is the ordered list of diff_ids (uncompressed layer
	// digests) from the image's RootFS.Layers. They become both the
	// rootfs.diff_ids in the config blob AND the source of layer
	// blobs in the manifest — they must match for the OCI spec to
	// hold.
	ImageLayers []string
	// Architecture / OS describe the image's target platform. Defaults
	// to amd64/linux when empty (matches Docker's default tag).
	Architecture string
	OS           string
	// Config is the image's runtime config (Cmd, Env, Entrypoint,
	// WorkingDir, …) that gets serialised into the OCI config blob.
	// Optional — when nil, the config object in the manifest is empty.
	Config *imageConfigBlob
}

// imageConfigBlob is the shape that gets serialised into an OCI image
// config blob's `config` field. Mirrors the subset of
// api.ContainerConfig that registries care about; unset fields are
// elided from JSON via omitempty.
type imageConfigBlob struct {
	User         string              `json:"User,omitempty"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts,omitempty"`
	Env          []string            `json:"Env,omitempty"`
	Cmd          []string            `json:"Cmd,omitempty"`
	Entrypoint   []string            `json:"Entrypoint,omitempty"`
	WorkingDir   string              `json:"WorkingDir,omitempty"`
	Labels       map[string]string   `json:"Labels,omitempty"`
}

// imageConfigFromAPI converts the stored api.ContainerConfig into the
// OCI config-blob shape for OCIPush. Returns nil when the source
// config is empty so OCIPush serialises an empty `config: {}` per the
// OCI spec.
func imageConfigFromAPI(c api.ContainerConfig) *imageConfigBlob {
	if c.User == "" && len(c.Env) == 0 && len(c.Cmd) == 0 && len(c.Entrypoint) == 0 && c.WorkingDir == "" && len(c.Labels) == 0 && len(c.ExposedPorts) == 0 {
		return nil
	}
	out := &imageConfigBlob{
		User:       c.User,
		Env:        c.Env,
		Cmd:        c.Cmd,
		Entrypoint: c.Entrypoint,
		WorkingDir: c.WorkingDir,
		Labels:     c.Labels,
	}
	if len(c.ExposedPorts) > 0 {
		out.ExposedPorts = make(map[string]struct{}, len(c.ExposedPorts))
		for p := range c.ExposedPorts {
			out.ExposedPorts[string(p)] = struct{}{}
		}
	}
	return out
}

// OCIPushResult contains the result of an OCI push.
type OCIPushResult struct {
	ManifestDigest string
	ConfigDigest   string
	LayerDigest    string   // First layer digest (backward compat)
	LayerDigests   []string // All layer digests
}

// ociPushClient is an HTTP client with timeouts for OCI push requests.
var ociPushClient = &http.Client{
	Timeout: 60 * time.Second,
}

// OCIPush pushes an image to an OCI-compliant registry. It uploads the
// config blob, all referenced layer blobs (using `opts.LayerContent`
// keyed by diff_id), and the v2 manifest. Returns the push result or
// an error. Callers are responsible for ensuring `opts.ImageLayers`
// and `opts.LayerContent` together describe the full image — the
// `rootfs.diff_ids` in the config blob is built from `ImageLayers` so
// it matches the manifest's layer list.
func OCIPush(opts OCIPushOptions) (*OCIPushResult, error) {
	baseURL := fmt.Sprintf("https://%s/v2/%s", opts.Registry, opts.Repository)

	// 1. Check registry connectivity
	if err := ociPing(baseURL, opts.AuthToken); err != nil {
		return nil, fmt.Errorf("registry ping failed: %w", err)
	}

	// 2. Build the OCI config blob from the caller's metadata.
	// rootfs.diff_ids must match the layers actually uploaded
	// below — clients use this to verify the manifest's chain-of-trust.
	arch := opts.Architecture
	if arch == "" {
		arch = "amd64"
	}
	osName := opts.OS
	if osName == "" {
		osName = "linux"
	}
	var cfg any
	if opts.Config != nil {
		cfg = opts.Config
	} else {
		cfg = map[string]any{}
	}
	configJSON, err := json.Marshal(map[string]any{
		"architecture": arch,
		"os":           osName,
		"created":      time.Now().UTC().Format(time.RFC3339),
		"config":       cfg,
		"rootfs": map[string]any{
			"type":     "layers",
			"diff_ids": opts.ImageLayers,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	configDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(configJSON))

	// 3. Upload config blob
	if err := ociUploadBlob(baseURL, opts.AuthToken, configDigest, configJSON, "application/vnd.docker.container.image.v1+json"); err != nil {
		return nil, fmt.Errorf("upload config blob: %w", err)
	}

	// 4. Upload layer blobs — use real content from LayerContent when available.
	type layerEntry struct {
		digest string
		size   int
	}
	var layerEntries []layerEntry

	if len(opts.ImageLayers) > 0 && opts.LayerContent != nil {
		for _, diffID := range opts.ImageLayers {
			if content, ok := opts.LayerContent(diffID); ok {
				digest := fmt.Sprintf("sha256:%x", sha256.Sum256(content))
				if err := ociUploadBlob(baseURL, opts.AuthToken, digest, content, "application/vnd.docker.image.rootfs.diff.tar.gzip"); err != nil {
					return nil, fmt.Errorf("upload layer blob: %w", err)
				}
				layerEntries = append(layerEntries, layerEntry{digest: digest, size: len(content)})
			}
		}
	}

	if len(layerEntries) == 0 {
		return nil, fmt.Errorf("push failed: image has no layer data available")
	}

	// 5. Create and PUT manifest
	manifestLayers := make([]map[string]any, len(layerEntries))
	for i, le := range layerEntries {
		manifestLayers[i] = map[string]any{
			"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
			"size":      le.size,
			"digest":    le.digest,
		}
	}

	manifest, err := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.docker.distribution.manifest.v2+json",
		"config": map[string]any{
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size":      len(configJSON),
			"digest":    configDigest,
		},
		"layers": manifestLayers,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}

	manifestDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(manifest))

	tag := opts.Tag
	if tag == "" {
		tag = "latest"
	}

	manifestURL := fmt.Sprintf("%s/manifests/%s", baseURL, tag)
	req, err := http.NewRequest(http.MethodPut, manifestURL, bytes.NewReader(manifest))
	if err != nil {
		return nil, fmt.Errorf("create manifest request: %w", err)
	}
	req.Header.Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
	if opts.AuthToken != "" {
		SetOCIAuth(req, opts.AuthToken)
	}

	resp, err := ociPushClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("put manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("put manifest returned %d: %s", resp.StatusCode, string(body))
	}

	var allDigests []string
	firstDigest := ""
	for _, le := range layerEntries {
		allDigests = append(allDigests, le.digest)
		if firstDigest == "" {
			firstDigest = le.digest
		}
	}

	return &OCIPushResult{
		ManifestDigest: manifestDigest,
		ConfigDigest:   configDigest,
		LayerDigest:    firstDigest,
		LayerDigests:   allDigests,
	}, nil
}

// ociPing checks registry connectivity via GET /v2/.
func ociPing(baseURL, authToken string) error {
	// Use the base registry URL (/v2/) for ping, not /v2/{repo}/
	parts := strings.SplitN(baseURL, "/v2/", 2)
	pingURL := parts[0] + "/v2/"

	req, err := http.NewRequest(http.MethodGet, pingURL, nil)
	if err != nil {
		return err
	}
	if authToken != "" {
		SetOCIAuth(req, authToken)
	}

	resp, err := ociPushClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Accept 200 and 401 (means registry is alive, auth may be needed at endpoint level)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnauthorized {
		return fmt.Errorf("ping returned %d", resp.StatusCode)
	}
	return nil
}

// ociUploadBlob uploads a blob via the two-step upload process:
// POST /v2/{name}/blobs/uploads/ to initiate, then PUT with ?digest= to complete.
func ociUploadBlob(baseURL, authToken, digest string, data []byte, contentType string) error {
	// Step 1: Initiate upload
	initiateURL := baseURL + "/blobs/uploads/"
	req, err := http.NewRequest(http.MethodPost, initiateURL, nil)
	if err != nil {
		return fmt.Errorf("create initiate request: %w", err)
	}
	if authToken != "" {
		SetOCIAuth(req, authToken)
	}

	initResp, err := ociPushClient.Do(req)
	if err != nil {
		return fmt.Errorf("initiate upload: %w", err)
	}
	defer initResp.Body.Close()

	if initResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(initResp.Body)
		return fmt.Errorf("initiate upload returned %d: %s", initResp.StatusCode, string(body))
	}

	// Get upload location from header
	location := initResp.Header.Get("Location")
	if location == "" {
		return fmt.Errorf("no Location header in upload initiate response")
	}

	// Make location absolute if relative
	if !strings.HasPrefix(location, "http") {
		// Parse the baseURL to get scheme+host
		parts := strings.SplitN(baseURL, "/v2/", 2)
		location = parts[0] + location
	}

	// Step 2: Complete upload
	separator := "?"
	if strings.Contains(location, "?") {
		separator = "&"
	}
	completeURL := fmt.Sprintf("%s%sdigest=%s", location, separator, digest)

	req, err = http.NewRequest(http.MethodPut, completeURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create complete request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	if authToken != "" {
		SetOCIAuth(req, authToken)
	}

	completeResp, err := ociPushClient.Do(req)
	if err != nil {
		return fmt.Errorf("complete upload: %w", err)
	}
	defer completeResp.Body.Close()

	if completeResp.StatusCode != http.StatusCreated && completeResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(completeResp.Body)
		return fmt.Errorf("complete upload returned %d: %s", completeResp.StatusCode, string(body))
	}

	return nil
}

// SetOCIAuth sets the Authorization header for OCI registry requests.
// Handles both "Bearer <token>" and raw token formats.
func SetOCIAuth(req *http.Request, token string) {
	if strings.HasPrefix(token, "Bearer ") || strings.HasPrefix(token, "Basic ") {
		req.Header.Set("Authorization", token)
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// IsGCPRegistry returns true if the registry is a GCP Artifact Registry or GCR.
func IsGCPRegistry(registry string) bool {
	return strings.HasSuffix(registry, ".gcr.io") || strings.HasSuffix(registry, "-docker.pkg.dev")
}
