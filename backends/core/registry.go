package core

import (
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

// imageConfigCache is an in-memory cache for fetched image configs.
var imageConfigCache = struct {
	sync.RWMutex
	m map[string]*api.ContainerConfig
}{m: make(map[string]*api.ContainerConfig)}

// FetchImageConfig fetches the real image config from a Docker v2 registry.
// Returns nil, nil if fetching is skipped (SOCKERLESS_SKIP_IMAGE_CONFIG=true)
// or the image reference can't be resolved (graceful fallback to synthetic config).
func FetchImageConfig(ref string, basicAuth ...string) (*api.ContainerConfig, error) {
	if os.Getenv("SOCKERLESS_SKIP_IMAGE_CONFIG") == "true" {
		return nil, nil
	}

	// Check cache
	imageConfigCache.RLock()
	if cfg, ok := imageConfigCache.m[ref]; ok {
		imageConfigCache.RUnlock()
		return cfg, nil
	}
	imageConfigCache.RUnlock()

	rc := parseImageRef(ref)

	auth := ""
	if len(basicAuth) > 0 {
		auth = basicAuth[0]
	}

	cfg, err := fetchConfigFromRegistry(rc, auth)
	if err != nil {
		// Graceful fallback: return nil so caller uses synthetic config
		return nil, nil
	}

	// Cache the result
	imageConfigCache.Lock()
	imageConfigCache.m[ref] = cfg
	imageConfigCache.Unlock()

	return cfg, nil
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

// fetchConfigFromRegistry fetches the container config from a v2 registry.
func fetchConfigFromRegistry(rc registryConfig, basicAuth string) (*api.ContainerConfig, error) {
	// Step 1: Get auth token
	token, err := getRegistryToken(rc, basicAuth)
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	// Step 2: Get manifest (try manifest list first, then single manifest)
	configDigest, err := getConfigDigest(rc, token)
	if err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}

	// Step 3: Get config blob
	cfg, err := getConfigBlob(rc, token, configDigest)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	return cfg, nil
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
	MediaType string         `json:"mediaType"`
	Config    manifestConfig `json:"config"`
}

type manifestConfig struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
}

// ociImageConfig represents the OCI/Docker image config blob.
type ociImageConfig struct {
	Architecture string              `json:"architecture"`
	OS           string              `json:"os"`
	Config       ociContainerConfig  `json:"config"`
}

type ociContainerConfig struct {
	Env          []string            `json:"Env"`
	Cmd          []string            `json:"Cmd"`
	Entrypoint   []string            `json:"Entrypoint"`
	WorkingDir   string              `json:"WorkingDir"`
	Labels       map[string]string   `json:"Labels"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts"`
}

// getConfigDigest resolves the image manifest to get the config blob digest.
func getConfigDigest(rc registryConfig, token string) (string, error) {
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
		return "", err
	}

	// Check if this is a manifest list
	if strings.Contains(mediaType, "manifest.list") || strings.Contains(mediaType, "image.index") {
		var ml manifestList
		if err := json.Unmarshal(body, &ml); err != nil {
			return "", fmt.Errorf("decode manifest list: %w", err)
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
			return "", fmt.Errorf("no suitable manifest in manifest list")
		}

		// Fetch the platform-specific manifest
		platformURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s",
			rc.Registry, rc.Repository, digest)
		body, _, err = registryGet(platformURL, token, []string{
			"application/vnd.oci.image.manifest.v1+json",
			"application/vnd.docker.distribution.manifest.v2+json",
		})
		if err != nil {
			return "", fmt.Errorf("fetch platform manifest: %w", err)
		}
	}

	// Parse single manifest
	var sm singleManifest
	if err := json.Unmarshal(body, &sm); err != nil {
		return "", fmt.Errorf("decode manifest: %w", err)
	}

	if sm.Config.Digest == "" {
		return "", fmt.Errorf("no config digest in manifest")
	}

	return sm.Config.Digest, nil
}

// getConfigBlob fetches and parses the image config blob.
func getConfigBlob(rc registryConfig, token, digest string) (*api.ContainerConfig, error) {
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

	cfg := &api.ContainerConfig{
		Env:          ociCfg.Config.Env,
		Cmd:          ociCfg.Config.Cmd,
		Entrypoint:   ociCfg.Config.Entrypoint,
		WorkingDir:   ociCfg.Config.WorkingDir,
		Labels:       ociCfg.Config.Labels,
		ExposedPorts: ociCfg.Config.ExposedPorts,
	}

	if cfg.Labels == nil {
		cfg.Labels = make(map[string]string)
	}

	return cfg, nil
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
