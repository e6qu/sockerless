package core

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sockerless/api"
)

// registryConfig holds parsed image reference components.
type registryConfig struct {
	Registry   string // e.g. "registry-1.docker.io"
	Repository string // e.g. "library/alpine"
	Tag        string // e.g. "latest"
}

// ImageMetadataResult holds all metadata fetched from a Docker v2 / OCI registry.
// This replaces the old FetchImageConfig which only returned ContainerConfig.
type ImageMetadataResult struct {
	Config         *api.ContainerConfig // Cmd, Entrypoint, Env, WorkingDir, Labels, etc.
	ConfigDigest   string               // Real config blob digest (e.g. "sha256:abc...")
	ManifestDigest string               // Manifest digest (e.g. "sha256:def...")
	LayerDigests   []string             // diff_ids from config blob rootfs
	LayerSizes     []int64              // Layer sizes from manifest (parallel to LayerDigests)
	TotalSize      int64                // Sum of all layer sizes + config size
	History        []ImageHistoryItem   // Build history from config blob
	Architecture   string               // e.g. "amd64"
	OS             string               // e.g. "linux"
	Author         string
	Created        string // RFC3339
}

// ImageHistoryItem represents a single build step from the image config.
type ImageHistoryItem struct {
	CreatedBy  string
	Created    string // RFC3339
	EmptyLayer bool
	Comment    string
}

// imageMetadataCache is an in-memory cache for fetched image metadata.
var imageMetadataCache = struct {
	sync.RWMutex
	m map[string]*ImageMetadataResult
}{m: make(map[string]*ImageMetadataResult)}

// FetchImageMetadata fetches rich image metadata from a Docker v2 registry.
// Returns nil, nil if fetching is skipped (SOCKERLESS_SKIP_IMAGE_CONFIG=true)
// or the image reference can't be resolved (graceful fallback to synthetic).
// Logs a warning on fetch failure instead of silently returning nil.
func FetchImageMetadata(ref string, basicAuth ...string) (*ImageMetadataResult, error) {
	// Guard against empty reference (causes 401 from registry)
	if ref == "" {
		return nil, nil
	}
	if os.Getenv("SOCKERLESS_SKIP_IMAGE_CONFIG") == "true" {
		return nil, nil
	}

	// Check cache
	imageMetadataCache.RLock()
	if meta, ok := imageMetadataCache.m[ref]; ok {
		imageMetadataCache.RUnlock()
		return meta, nil
	}
	imageMetadataCache.RUnlock()

	rc := parseImageRef(ref)

	auth := ""
	if len(basicAuth) > 0 {
		auth = basicAuth[0]
	}

	meta, err := fetchMetadataFromRegistry(rc, auth)
	if err != nil {
		// Log warning instead of silent fallback.
		fmt.Fprintf(os.Stderr, "[WARN] FetchImageMetadata(%q): %v (falling back to synthetic)\n", ref, err)
		return nil, nil
	}

	// Cache the result
	imageMetadataCache.Lock()
	imageMetadataCache.m[ref] = meta
	imageMetadataCache.Unlock()

	return meta, nil
}

// FetchImageConfig fetches the real image config from a Docker v2 registry.
// Wrapper around FetchImageMetadata for backward compatibility.
func FetchImageConfig(ref string, basicAuth ...string) (*api.ContainerConfig, error) {
	meta, err := FetchImageMetadata(ref, basicAuth...)
	if meta != nil {
		return meta.Config, err
	}
	return nil, err
}

// parseImageRef parses a Docker image reference into registry, repository, tag.
func parseImageRef(ref string) registryConfig {
	// Remove digest if present (we use tag for manifest lookup)
	if at := strings.Index(ref, "@"); at >= 0 {
		ref = ref[:at]
	}

	// Split tag
	tag := "latest"
	// Handle the case where the ref has a port (e.g. localhost:5000/image:tag)
	// by only splitting on the last colon after the last slash
	lastSlash := strings.LastIndex(ref, "/")
	colonAfterSlash := strings.LastIndex(ref[lastSlash+1:], ":")
	if colonAfterSlash >= 0 {
		colonPos := lastSlash + 1 + colonAfterSlash
		tag = ref[colonPos+1:]
		ref = ref[:colonPos]
	}

	// Determine registry and repository
	registry := "registry-1.docker.io"
	repo := ref

	// Check if the first component looks like a registry (has a dot or colon)
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		registry = parts[0]
		repo = parts[1]
		// Docker Hub special case: docker.io → registry-1.docker.io
		if registry == "docker.io" {
			registry = "registry-1.docker.io"
		}
	} else if len(parts) == 1 {
		// Simple name like "alpine" → library/alpine
		repo = "library/" + ref
	}
	// else: user/repo format (e.g. "myuser/myimage"), defaults to Docker Hub.
	// repo stays as-is.

	return registryConfig{
		Registry:   registry,
		Repository: repo,
		Tag:        tag,
	}
}

// registryClient is an HTTP client with timeouts for registry requests.
var registryClient = &http.Client{
	Timeout: 15 * time.Second,
}

// fetchMetadataFromRegistry fetches rich image metadata from a v2 registry.
func fetchMetadataFromRegistry(rc registryConfig, basicAuth string) (*ImageMetadataResult, error) {
	// Step 1: Get auth token
	token, err := getRegistryToken(rc, basicAuth)
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	// Step 2: Get manifest info (config digest, layer digests/sizes, manifest digest)
	minfo, err := getManifestInfo(rc, token)
	if err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}

	// Step 3: Get config blob (full OCI config with rootfs, history, etc.)
	ociCfg, err := getFullConfigBlob(rc, token, minfo.configDigest)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	cfg := &api.ContainerConfig{
		Env:          ociCfg.Config.Env,
		Cmd:          ociCfg.Config.Cmd,
		Entrypoint:   ociCfg.Config.Entrypoint,
		WorkingDir:   ociCfg.Config.WorkingDir,
		User:         ociCfg.Config.User,
		Labels:       ociCfg.Config.Labels,
		ExposedPorts: ociCfg.Config.ExposedPorts,
	}
	if cfg.Labels == nil {
		cfg.Labels = make(map[string]string)
	}

	// Build history items
	var history []ImageHistoryItem
	for _, h := range ociCfg.History {
		history = append(history, ImageHistoryItem(h))
	}

	// Calculate total size from manifest layers
	var totalSize int64
	for _, ls := range minfo.layerSizes {
		totalSize += ls
	}
	totalSize += minfo.configSize

	return &ImageMetadataResult{
		Config:         cfg,
		ConfigDigest:   minfo.configDigest,
		ManifestDigest: minfo.manifestDigest,
		LayerDigests:   ociCfg.RootFS.DiffIDs,
		LayerSizes:     minfo.layerSizes,
		TotalSize:      totalSize,
		History:        history,
		Architecture:   ociCfg.Architecture,
		OS:             ociCfg.OS,
		Author:         ociCfg.Author,
		Created:        ociCfg.Created,
	}, nil
}

// tokenResponse is the response from a Docker registry auth endpoint.
type tokenResponse struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
}

// getRegistryToken gets an auth token for the given registry/repo.
// If basicAuth is non-empty, it is sent as Basic auth on the token exchange.
func getRegistryToken(rc registryConfig, basicAuth string) (string, error) {
	// Try to access the manifests endpoint first to discover auth
	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s",
		rc.Registry, rc.Repository, rc.Tag)
	resp, err := registryClient.Get(manifestURL)
	if err != nil {
		return "", err
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		// No auth required
		return "", nil
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return "", fmt.Errorf("unexpected status %d from registry", resp.StatusCode)
	}

	// Parse Www-Authenticate header
	authHeader := resp.Header.Get("Www-Authenticate")
	if authHeader == "" {
		return "", fmt.Errorf("no Www-Authenticate header in 401 response")
	}

	realm, params := parseWWWAuthenticate(authHeader)
	if realm == "" {
		return "", fmt.Errorf("no realm in Www-Authenticate header")
	}

	// Build token request
	tokenURL, err := url.Parse(realm)
	if err != nil {
		return "", fmt.Errorf("invalid realm URL: %w", err)
	}
	q := tokenURL.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	tokenURL.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", tokenURL.String(), nil)
	if err != nil {
		return "", err
	}
	if basicAuth != "" {
		req.Header.Set("Authorization", "Basic "+basicAuth)
	}

	resp, err = registryClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", err
	}

	if tr.Token != "" {
		return tr.Token, nil
	}
	return tr.AccessToken, nil
}

// parseWWWAuthenticate parses a Bearer Www-Authenticate header.
// Example: Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/alpine:pull"
func parseWWWAuthenticate(header string) (realm string, params map[string]string) {
	params = make(map[string]string)

	header = strings.TrimPrefix(header, "Bearer ")
	header = strings.TrimPrefix(header, "bearer ")

	// Split on commas, but respect quoted strings
	for _, part := range splitAuthParams(header) {
		part = strings.TrimSpace(part)
		eq := strings.IndexByte(part, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(part[:eq])
		val := strings.TrimSpace(part[eq+1:])
		val = strings.Trim(val, "\"")

		if key == "realm" {
			realm = val
		} else {
			params[key] = val
		}
	}
	return
}

// splitAuthParams splits a Bearer auth header value on commas,
// respecting quoted strings.
func splitAuthParams(s string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '"' {
			inQuote = !inQuote
			current.WriteByte(ch)
		} else if ch == ',' && !inQuote {
			parts = append(parts, current.String())
			current.Reset()
		} else {
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

// manifestList represents a Docker manifest list or OCI image index.
type manifestList struct {
	MediaType string             `json:"mediaType"`
	Manifests []manifestListItem `json:"manifests"`
}

type manifestListItem struct {
	MediaType string           `json:"mediaType"`
	Digest    string           `json:"digest"`
	Platform  manifestPlatform `json:"platform"`
}

type manifestPlatform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
}

// singleManifest represents a Docker v2 or OCI image manifest.
type singleManifest struct {
	MediaType string          `json:"mediaType"`
	Config    manifestConfig  `json:"config"`
	Layers    []manifestLayer `json:"layers"`
}

type manifestConfig struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

type manifestLayer struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

// ociImageConfig represents the OCI/Docker image config blob.
type ociImageConfig struct {
	Architecture string             `json:"architecture"`
	OS           string             `json:"os"`
	Author       string             `json:"author"`
	Created      string             `json:"created"`
	Config       ociContainerConfig `json:"config"`
	RootFS       ociRootFS          `json:"rootfs"`
	History      []ociHistoryItem   `json:"history"`
}

type ociContainerConfig struct {
	Env          []string            `json:"Env"`
	Cmd          []string            `json:"Cmd"`
	Entrypoint   []string            `json:"Entrypoint"`
	WorkingDir   string              `json:"WorkingDir"`
	User         string              `json:"User"`
	Labels       map[string]string   `json:"Labels"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts"`
}

type ociRootFS struct {
	Type    string   `json:"type"`
	DiffIDs []string `json:"diff_ids"`
}

type ociHistoryItem struct {
	CreatedBy  string `json:"created_by"`
	Created    string `json:"created"`
	EmptyLayer bool   `json:"empty_layer"`
	Comment    string `json:"comment"`
}

// manifestInfo holds the parsed manifest data.
type manifestInfo struct {
	configDigest   string
	configSize     int64
	manifestDigest string
	layerSizes     []int64
}

// getManifestInfo resolves the image manifest to get config digest, layer info,
// and manifest digest.
func getManifestInfo(rc registryConfig, token string) (*manifestInfo, error) {
	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s",
		rc.Registry, rc.Repository, rc.Tag)

	// Try manifest list first (for multi-arch images)
	body, mediaType, err := registryGet(manifestURL, token, []string{
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	})
	if err != nil {
		return nil, err
	}

	// Check if this is a manifest list
	if strings.Contains(mediaType, "manifest.list") || strings.Contains(mediaType, "image.index") {
		var ml manifestList
		if err := json.Unmarshal(body, &ml); err != nil {
			return nil, fmt.Errorf("decode manifest list: %w", err)
		}

		// Find amd64/linux manifest
		digest := ""
		for _, m := range ml.Manifests {
			if m.Platform.Architecture == "amd64" && m.Platform.OS == "linux" {
				digest = m.Digest
				break
			}
		}
		if digest == "" && len(ml.Manifests) > 0 {
			// Fallback to first manifest
			digest = ml.Manifests[0].Digest
		}
		if digest == "" {
			return nil, fmt.Errorf("no suitable manifest in manifest list")
		}

		// Fetch the platform-specific manifest
		platformURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s",
			rc.Registry, rc.Repository, digest)
		body, _, err = registryGet(platformURL, token, []string{
			"application/vnd.oci.image.manifest.v1+json",
			"application/vnd.docker.distribution.manifest.v2+json",
		})
		if err != nil {
			return nil, fmt.Errorf("fetch platform manifest: %w", err)
		}
	}

	// Compute manifest digest from the body
	manifestHash := sha256.Sum256(body)
	manifestDigest := fmt.Sprintf("sha256:%x", manifestHash)

	// Parse single manifest
	var sm singleManifest
	if err := json.Unmarshal(body, &sm); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}

	if sm.Config.Digest == "" {
		return nil, fmt.Errorf("no config digest in manifest")
	}

	// Extract layer sizes from manifest
	layerSizes := make([]int64, len(sm.Layers))
	for i, l := range sm.Layers {
		layerSizes[i] = l.Size
	}

	return &manifestInfo{
		configDigest:   sm.Config.Digest,
		configSize:     sm.Config.Size,
		manifestDigest: manifestDigest,
		layerSizes:     layerSizes,
	}, nil
}

// getFullConfigBlob fetches and parses the full OCI image config blob.
func getFullConfigBlob(rc registryConfig, token, digest string) (*ociImageConfig, error) {
	blobURL := fmt.Sprintf("https://%s/v2/%s/blobs/%s",
		rc.Registry, rc.Repository, digest)

	body, _, err := registryGet(blobURL, token, nil)
	if err != nil {
		return nil, err
	}

	var ociCfg ociImageConfig
	if err := json.Unmarshal(body, &ociCfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	return &ociCfg, nil
}

// registryGet performs an authenticated GET request to a registry endpoint.
func registryGet(url, token string, accept []string) ([]byte, string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", err
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	if len(accept) > 0 {
		req.Header.Set("Accept", strings.Join(accept, ", "))
	}

	resp, err := registryClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("registry returned %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	contentType := resp.Header.Get("Content-Type")
	return body, contentType, nil
}
