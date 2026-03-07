package azf

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/rs/zerolog"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// ACRAuthProvider handles authentication and OCI operations for Azure Container Registry.
type ACRAuthProvider struct {
	Logger zerolog.Logger
}

// GetToken returns a Bearer token for the given ACR registry using DefaultAzureCredential.
func (a *ACRAuthProvider) GetToken(registry string) (string, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return "", err
	}
	scope := fmt.Sprintf("https://%s/.default", registry)
	token, err := cred.GetToken(context.Background(), policy.TokenRequestOptions{Scopes: []string{scope}})
	if err != nil {
		return "", err
	}
	return "Bearer " + token.Token, nil
}

// IsCloudRegistry returns true if the registry is an Azure Container Registry.
func (a *ACRAuthProvider) IsCloudRegistry(registry string) bool {
	return strings.HasSuffix(registry, ".azurecr.io")
}

// OnPush pushes a synthetic OCI image to ACR via the Distribution API.
// Errors are non-fatal — the caller logs warnings on failure.
func (a *ACRAuthProvider) OnPush(img *api.Image, registry, repo, tag string) error {
	token, err := a.GetToken(registry)
	if err != nil {
		return fmt.Errorf("ACR auth: %w", err)
	}

	_, err = core.OCIPush(core.OCIPushOptions{
		Registry:   registry,
		Repository: repo,
		Tag:        tag,
		AuthToken:  token,
	})
	return err
}

// OnTag syncs a tag to ACR by fetching the source manifest and re-putting it with the new tag.
// Errors are non-fatal — the caller logs warnings on failure.
func (a *ACRAuthProvider) OnTag(img *api.Image, registry, repo, newTag string) error {
	token, err := a.GetToken(registry)
	if err != nil {
		return fmt.Errorf("ACR auth: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	baseURL := fmt.Sprintf("https://%s", registry)

	// Try to get existing manifest for the source image
	srcDigest := strings.TrimPrefix(img.ID, "sha256:")
	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/sha256:%s", baseURL, repo, srcDigest)
	req, _ := http.NewRequest("GET", manifestURL, nil)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")
	core.SetOCIAuth(req, token)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("manifest fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("source manifest not found in ACR: %d", resp.StatusCode)
	}

	manifestData, _ := io.ReadAll(resp.Body)
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/vnd.docker.distribution.manifest.v2+json"
	}

	// PUT manifest with new tag
	putURL := fmt.Sprintf("%s/v2/%s/manifests/%s", baseURL, repo, newTag)
	putReq, _ := http.NewRequest("PUT", putURL, bytes.NewReader(manifestData))
	putReq.Header.Set("Content-Type", contentType)
	core.SetOCIAuth(putReq, token)

	putResp, err := client.Do(putReq)
	if err != nil {
		return fmt.Errorf("manifest PUT: %w", err)
	}
	putResp.Body.Close()

	if putResp.StatusCode != 201 && putResp.StatusCode != 200 {
		return fmt.Errorf("manifest PUT returned %d", putResp.StatusCode)
	}

	return nil
}

// OnRemove deletes a manifest from ACR. Graceful on 404/405.
// Errors are non-fatal — the caller logs warnings on failure.
func (a *ACRAuthProvider) OnRemove(registry, repo string, tags []string) error {
	token, err := a.GetToken(registry)
	if err != nil {
		return fmt.Errorf("ACR auth: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}

	for _, tag := range tags {
		// Try to get the manifest digest first
		manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)
		req, _ := http.NewRequest("HEAD", manifestURL, nil)
		req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")
		core.SetOCIAuth(req, token)

		headResp, err := client.Do(req)
		if err != nil {
			a.Logger.Warn().Err(err).Str("registry", registry).Str("tag", tag).Msg("ACR remove: manifest HEAD failed")
			continue
		}
		headResp.Body.Close()

		if headResp.StatusCode == 404 || headResp.StatusCode == 405 {
			continue // manifest not found or delete not supported — graceful
		}

		if headResp.StatusCode != 200 {
			a.Logger.Warn().Int("status", headResp.StatusCode).Str("registry", registry).Str("tag", tag).Msg("ACR remove: manifest not found")
			continue
		}

		digest := headResp.Header.Get("Docker-Content-Digest")
		if digest == "" {
			digest = tag
		}

		// DELETE manifest
		delURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, digest)
		delReq, _ := http.NewRequest("DELETE", delURL, nil)
		core.SetOCIAuth(delReq, token)

		delResp, err := client.Do(delReq)
		if err != nil {
			a.Logger.Warn().Err(err).Str("registry", registry).Str("tag", tag).Msg("ACR remove: manifest DELETE failed")
			continue
		}
		delResp.Body.Close()

		if delResp.StatusCode != 202 && delResp.StatusCode != 200 && delResp.StatusCode != 404 && delResp.StatusCode != 405 {
			a.Logger.Warn().Int("status", delResp.StatusCode).Str("registry", registry).Str("tag", tag).Msg("ACR remove: manifest DELETE returned error")
		}
	}

	return nil
}

// fetchImageConfig fetches the image config from a registry, with ACR auth support.
// For non-ACR registries, delegates to core.FetchImageConfig for caching.
func (a *ACRAuthProvider) fetchImageConfig(ref, authHeader string) (*api.ContainerConfig, error) {
	registry, repo, tag := parseImageRef(ref)

	// For non-ACR registries, use core's cached FetchImageConfig
	if !a.IsCloudRegistry(registry) {
		return core.FetchImageConfig(ref, authHeader)
	}

	// ACR: authenticate and fetch directly
	token, err := a.GetToken(registry)
	if err != nil {
		return nil, fmt.Errorf("ACR auth failed: %w", err)
	}

	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)
	req, _ := http.NewRequest("GET", manifestURL, nil)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")
	core.SetOCIAuth(req, token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("manifest fetch failed: %d", resp.StatusCode)
	}

	var manifest struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, err
	}

	configURL := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, repo, manifest.Config.Digest)
	req, _ = http.NewRequest("GET", configURL, nil)
	core.SetOCIAuth(req, token)

	resp, err = client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("config blob fetch failed: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var imageConfig struct {
		Config struct {
			Env          []string            `json:"Env"`
			Cmd          []string            `json:"Cmd"`
			Entrypoint   []string            `json:"Entrypoint"`
			WorkingDir   string              `json:"WorkingDir"`
			User         string              `json:"User"`
			Labels       map[string]string   `json:"Labels"`
			Volumes      map[string]struct{} `json:"Volumes"`
			ExposedPorts map[string]struct{} `json:"ExposedPorts"`
		} `json:"config"`
	}
	if err := json.Unmarshal(body, &imageConfig); err != nil {
		return nil, err
	}

	return &api.ContainerConfig{
		Env:          imageConfig.Config.Env,
		Cmd:          imageConfig.Config.Cmd,
		Entrypoint:   imageConfig.Config.Entrypoint,
		WorkingDir:   imageConfig.Config.WorkingDir,
		User:         imageConfig.Config.User,
		Labels:       imageConfig.Config.Labels,
		Volumes:      imageConfig.Config.Volumes,
		ExposedPorts: imageConfig.Config.ExposedPorts,
		Image:        ref,
	}, nil
}

// parseImageRef splits an image reference into registry, repository, and tag.
func parseImageRef(ref string) (registry, repo, tag string) {
	tag = "latest"
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		afterColon := ref[idx+1:]
		if !strings.Contains(afterColon, "/") {
			tag = afterColon
			ref = ref[:idx]
		}
	}

	if strings.Contains(ref, ".") || strings.Contains(ref, ":") {
		parts := strings.SplitN(ref, "/", 2)
		if len(parts) == 2 {
			registry = parts[0]
			repo = parts[1]
			return
		}
	}

	registry = "registry-1.docker.io"
	if !strings.Contains(ref, "/") {
		repo = "library/" + ref
	} else {
		repo = ref
	}
	return
}

// syntheticImageSize returns a deterministic size from the ref hash.
func syntheticImageSize(ref string) int64 {
	h := fnv.New32a()
	h.Write([]byte(ref))
	return int64(10_000_000 + h.Sum32()%90_000_000)
}

// syntheticImageID returns a deterministic image ID from the ref.
func syntheticImageID(ref string) string {
	hash := sha256.Sum256([]byte(ref))
	return fmt.Sprintf("sha256:%x", hash)
}
