package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AssembleMultiArchManifest is the shared implementation that every
// per-cloud `CloudBuildService.AssembleMultiArchManifest` delegates
// to. Discovers the per-arch image manifests + their architectures
// from the registry, builds an OCI image-index pointing at them, and
// PUTs it under `opts.Tag`. Works against any OCI distribution v2
// registry that returns the standard manifest media types — ECR,
// Artifact Registry, Azure Container Registry, Docker Hub, anything
// else conforming to the spec.
//
// `tokenForRepo(repo)` is a callback the per-cloud caller supplies to
// mint the right bearer token for the registry (ECRAuthProvider for
// AWS, ARAuthProvider for GCP, ACRAuthProvider for Azure). The
// returned token is set as `Authorization: Bearer <token>` on every
// registry HTTP call.
//
// On success, subsequent `docker pull <opts.Tag>` from a host of any
// arch advertised in `opts.PerArchTags` resolves to the matching
// per-arch image — same shape multi-arch images on Docker Hub use.
func AssembleMultiArchManifest(ctx context.Context, opts MultiArchManifestOptions, tokenForRepo func(repo string) (string, error)) error {
	if opts.Tag == "" {
		return fmt.Errorf("multiarch: Tag required")
	}
	if len(opts.PerArchTags) == 0 {
		return fmt.Errorf("multiarch: at least one PerArchTag required")
	}

	// All per-arch refs must live in the same repo as the index tag —
	// OCI distribution requires manifest-list entries to reference
	// blobs within the same repository.
	indexRC := parseImageRef(opts.Tag)
	for _, t := range opts.PerArchTags {
		rc := parseImageRef(t)
		if rc.Registry != indexRC.Registry || rc.Repository != indexRC.Repository {
			return fmt.Errorf("multiarch: per-arch tag %q must share registry+repo with index tag %q (got %s/%s vs %s/%s)",
				t, opts.Tag, rc.Registry, rc.Repository, indexRC.Registry, indexRC.Repository)
		}
	}

	token, err := tokenForRepo(indexRC.Registry + "/" + indexRC.Repository)
	if err != nil {
		return fmt.Errorf("multiarch: token: %w", err)
	}

	// For each per-arch tag, fetch its manifest digest + size + the
	// architecture/os from its config blob. These three fields are
	// what the manifest-list entry needs.
	type entry struct {
		digest    string
		size      int64
		mediaType string
		arch      string
		os        string
	}
	entries := make([]entry, 0, len(opts.PerArchTags))
	for _, t := range opts.PerArchTags {
		rc := parseImageRef(t)
		dig, sz, mt, err := fetchManifestDigest(ctx, rc, token)
		if err != nil {
			return fmt.Errorf("multiarch: fetch %s manifest: %w", t, err)
		}
		arch, osName, err := fetchPlatformFromConfig(ctx, rc, token, dig)
		if err != nil {
			// Fall back to single-platform GET to extract platform
			// from the manifest descriptor itself.
			arch, osName, err = fetchPlatformViaManifest(ctx, rc, token)
			if err != nil {
				return fmt.Errorf("multiarch: fetch %s platform: %w", t, err)
			}
		}
		entries = append(entries, entry{digest: dig, size: sz, mediaType: mt, arch: arch, os: osName})
	}

	// Build the OCI image-index. Use Docker schema 2 manifest-list
	// media type for broadest compatibility — every modern docker /
	// containerd / podman accepts it, and Docker Hub itself still
	// emits it for many official images.
	type platform struct {
		Architecture string `json:"architecture"`
		OS           string `json:"os"`
	}
	type manifestEntry struct {
		MediaType string   `json:"mediaType"`
		Size      int64    `json:"size"`
		Digest    string   `json:"digest"`
		Platform  platform `json:"platform"`
	}
	index := struct {
		SchemaVersion int             `json:"schemaVersion"`
		MediaType     string          `json:"mediaType"`
		Manifests     []manifestEntry `json:"manifests"`
	}{
		SchemaVersion: 2,
		MediaType:     "application/vnd.docker.distribution.manifest.list.v2+json",
	}
	for _, e := range entries {
		index.Manifests = append(index.Manifests, manifestEntry{
			MediaType: e.mediaType,
			Size:      e.size,
			Digest:    e.digest,
			Platform:  platform{Architecture: e.arch, OS: e.os},
		})
	}

	body, err := json.Marshal(index)
	if err != nil {
		return fmt.Errorf("multiarch: marshal index: %w", err)
	}

	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", indexRC.Registry, indexRC.Repository, indexRC.Tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("multiarch: build PUT: %w", err)
	}
	req.Header.Set("Content-Type", index.MediaType)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("multiarch: PUT %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("multiarch: PUT %s: %d: %s", url, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

// fetchManifestDigest GETs the manifest at the per-arch tag and
// returns the manifest's digest, size, and media type — the three
// fields the index entry needs.
func fetchManifestDigest(ctx context.Context, rc registryConfig, token string) (digest string, size int64, mediaType string, err error) {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", rc.Registry, rc.Repository, rc.Tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, "", err
	}
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
	}, ","))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", 0, "", fmt.Errorf("GET %s: %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	digest = resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", 0, "", fmt.Errorf("GET %s: missing Docker-Content-Digest header", url)
	}
	mediaType = resp.Header.Get("Content-Type")
	return digest, int64(len(body)), mediaType, nil
}

// fetchPlatformFromConfig reads the per-arch image's config blob to
// extract architecture + os. The manifest's `config.digest` field
// points at the config blob; the blob has top-level `architecture`
// and `os` fields (per OCI image spec).
func fetchPlatformFromConfig(ctx context.Context, rc registryConfig, token, manifestDigest string) (arch, osName string, err error) {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", rc.Registry, rc.Repository, manifestDigest)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json,application/vnd.oci.image.manifest.v1+json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("GET manifest by digest %s: %d", manifestDigest, resp.StatusCode)
	}
	var m struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return "", "", fmt.Errorf("decode manifest: %w", err)
	}
	if m.Config.Digest == "" {
		return "", "", fmt.Errorf("manifest has no config.digest")
	}

	cfgURL := fmt.Sprintf("https://%s/v2/%s/blobs/%s", rc.Registry, rc.Repository, m.Config.Digest)
	cfgReq, err := http.NewRequestWithContext(ctx, http.MethodGet, cfgURL, nil)
	if err != nil {
		return "", "", err
	}
	cfgReq.Header.Set("Authorization", "Bearer "+token)
	cfgResp, err := http.DefaultClient.Do(cfgReq)
	if err != nil {
		return "", "", err
	}
	defer cfgResp.Body.Close()
	cfgBody, _ := io.ReadAll(cfgResp.Body)
	if cfgResp.StatusCode >= 300 {
		return "", "", fmt.Errorf("GET config blob %s: %d", m.Config.Digest, cfgResp.StatusCode)
	}
	var cfg struct {
		Architecture string `json:"architecture"`
		OS           string `json:"os"`
	}
	if err := json.Unmarshal(cfgBody, &cfg); err != nil {
		return "", "", fmt.Errorf("decode config: %w", err)
	}
	if cfg.Architecture == "" {
		return "", "", fmt.Errorf("config blob missing architecture field")
	}
	if cfg.OS == "" {
		cfg.OS = "linux"
	}
	return cfg.Architecture, cfg.OS, nil
}

// fetchPlatformViaManifest is the fallback path for registries that
// don't fully cooperate with the config-blob lookup — extract arch
// from the per-arch tag's name (`-amd64` / `-arm64` suffix). Same
// convention the runner-image Makefiles use.
func fetchPlatformViaManifest(_ context.Context, rc registryConfig, _ string) (arch, osName string, err error) {
	if strings.HasSuffix(rc.Tag, "-amd64") {
		return "amd64", "linux", nil
	}
	if strings.HasSuffix(rc.Tag, "-arm64") {
		return "arm64", "linux", nil
	}
	if strings.HasSuffix(rc.Tag, "-arm") {
		return "arm", "linux", nil
	}
	return "", "", fmt.Errorf("can't infer platform from tag %q (no -amd64 / -arm64 suffix)", rc.Tag)
}
