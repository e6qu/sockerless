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
)

// OCIPushOptions configures an OCI push operation.
type OCIPushOptions struct {
	Registry   string // e.g. "us-docker.pkg.dev"
	Repository string // e.g. "myproject/myrepo/myimage"
	Tag        string // e.g. "latest"
	AuthToken  string // Bearer token or empty
}

// OCIPushResult contains the result of an OCI push.
type OCIPushResult struct {
	ManifestDigest string
	ConfigDigest   string
	LayerDigest    string
}

// ociPushClient is an HTTP client with timeouts for OCI push requests.
var ociPushClient = &http.Client{
	Timeout: 60 * time.Second,
}

// OCIPush pushes a synthetic image to an OCI-compliant registry.
// It uploads a synthetic config blob, a synthetic layer blob, and a manifest.
// Returns the push result or an error. The caller should treat errors as non-fatal.
func OCIPush(opts OCIPushOptions) (*OCIPushResult, error) {
	baseURL := fmt.Sprintf("https://%s/v2/%s", opts.Registry, opts.Repository)

	// 1. Check registry connectivity
	if err := ociPing(baseURL, opts.AuthToken); err != nil {
		return nil, fmt.Errorf("registry ping failed: %w", err)
	}

	// 2. Create synthetic config blob
	configJSON, err := json.Marshal(map[string]any{
		"architecture": "amd64",
		"os":           "linux",
		"created":      time.Now().UTC().Format(time.RFC3339),
		"config":       map[string]any{},
		"rootfs": map[string]any{
			"type":     "layers",
			"diff_ids": []string{},
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

	// 4. Create and upload synthetic layer blob (empty gzip)
	// Minimal valid gzip: 1f 8b 08 00 ... (20 bytes)
	layerData := []byte{0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff,
		0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	layerDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(layerData))

	if err := ociUploadBlob(baseURL, opts.AuthToken, layerDigest, layerData, "application/vnd.docker.image.rootfs.diff.tar.gzip"); err != nil {
		return nil, fmt.Errorf("upload layer blob: %w", err)
	}

	// 5. Create and PUT manifest
	manifest, err := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.docker.distribution.manifest.v2+json",
		"config": map[string]any{
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"size":      len(configJSON),
			"digest":    configDigest,
		},
		"layers": []map[string]any{
			{
				"mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
				"size":      len(layerData),
				"digest":    layerDigest,
			},
		},
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

	return &OCIPushResult{
		ManifestDigest: manifestDigest,
		ConfigDigest:   configDigest,
		LayerDigest:    layerDigest,
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
