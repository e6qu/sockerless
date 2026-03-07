package azf

import (
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
