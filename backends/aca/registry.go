package aca

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/sockerless/api"
)

// fetchImageConfig fetches the image config from a registry.
func (s *Server) fetchImageConfig(ref, authHeader string) (*api.ContainerConfig, error) {
	registry, repo, tag := parseImageRef(ref)

	token := ""
	if strings.HasSuffix(registry, ".azurecr.io") {
		t, err := s.getACRToken(registry)
		if err != nil {
			return nil, fmt.Errorf("ACR auth failed: %w", err)
		}
		token = t
	} else if authHeader != "" {
		token = authHeader
	} else if registry == "registry-1.docker.io" || registry == "docker.io" {
		registry = "registry-1.docker.io"
		t, err := getDockerHubToken(repo)
		if err != nil {
			return nil, fmt.Errorf("docker hub auth failed: %w", err)
		}
		token = "Bearer " + t
	}

	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)
	req, _ := http.NewRequest("GET", manifestURL, nil)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")
	if token != "" {
		if strings.HasPrefix(token, "Basic ") || strings.HasPrefix(token, "Bearer ") {
			req.Header.Set("Authorization", token)
		} else {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

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
	if token != "" {
		if strings.HasPrefix(token, "Basic ") || strings.HasPrefix(token, "Bearer ") {
			req.Header.Set("Authorization", token)
		} else {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

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

// getACRToken gets an access token for Azure Container Registry.
func (s *Server) getACRToken(registry string) (string, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return "", err
	}
	scope := fmt.Sprintf("https://%s/.default", registry)
	token, err := cred.GetToken(s.ctx(), policy.TokenRequestOptions{Scopes: []string{scope}})
	if err != nil {
		return "", err
	}
	return "Bearer " + token.Token, nil
}

// getDockerHubToken gets an anonymous token for Docker Hub.
func getDockerHubToken(repo string) (string, error) {
	url := fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull", repo)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Token, nil
}

