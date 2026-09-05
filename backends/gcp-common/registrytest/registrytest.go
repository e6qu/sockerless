// Package registrytest provisions what a Docker client needs to push into
// Google Artifact Registry, the way an operator does it against the real
// service: a repository created through the Artifact Registry API, an OAuth 2.0
// access token minted from a service-account key, and a `docker login` with
// that token as the `oauth2accesstoken` password — the exchange `gcloud auth
// configure-docker` performs. The integration harnesses of the Google Cloud
// backends use it to stand in for Terraform and the operator's shell; nothing
// here knows whether the coordinates it is handed name the real service or a
// simulator.
package registrytest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
)

// LoginUsername is the username Artifact Registry documents for a Docker
// login whose password is an OAuth 2.0 access token.
const LoginUsername = "oauth2accesstoken"

// AccessToken mints an OAuth 2.0 access token from a service-account key file,
// by the JWT-bearer grant at the key's own `token_uri` — what
// `gcloud auth activate-service-account` followed by
// `gcloud auth print-access-token` yields.
func AccessToken(ctx context.Context, serviceAccountKey []byte) (string, error) {
	cfg, err := google.JWTConfigFromJSON(serviceAccountKey, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", fmt.Errorf("service-account key: %w", err)
	}
	tok, err := cfg.TokenSource(ctx).Token()
	if err != nil {
		return "", fmt.Errorf("JWT-bearer grant at %s: %w", cfg.TokenURL, err)
	}
	return tok.AccessToken, nil
}

// CreateDockerRepository creates a standard Docker-format repository through
// the Artifact Registry API at `apiEndpoint` (the Google Cloud API
// coordinate), authenticated with `bearer`. A repository that already exists
// is the requested state, as it is for a Terraform apply.
func CreateDockerRepository(ctx context.Context, apiEndpoint, bearer, project, location, repositoryID string) error {
	body := []byte(`{"format":"DOCKER"}`)
	u := fmt.Sprintf("%s/v1/projects/%s/locations/%s/repositories?repositoryId=%s",
		strings.TrimRight(apiEndpoint, "/"), project, location, repositoryID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("create repository %s: %w", repositoryID, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	switch resp.StatusCode {
	case http.StatusOK, http.StatusConflict:
		return nil
	default:
		return fmt.Errorf("create repository %s: HTTP %d: %s", repositoryID, resp.StatusCode, strings.TrimSpace(string(data)))
	}
}

// NewDockerConfigDir creates a Docker CLI configuration directory for a
// harness to log in with, so the login neither lands in the operator's own
// configuration nor in a credential store. It carries over everything from
// the ambient configuration (`DOCKER_CONFIG`, else `~/.docker`) except the
// credential-store settings — the current context in particular, which is how
// a Docker Desktop or Podman engine is reached — and is meant to be removed
// when the harness is done. Every `docker` invocation that should see the
// login runs with `Env` from `Env(dir)`.
func NewDockerConfigDir() (string, error) {
	dir, err := os.MkdirTemp("", "sockerless-docker-config-*")
	if err != nil {
		return "", err
	}
	cfg := map[string]any{}
	if src, ok := ambientDockerConfig(); ok {
		raw, err := os.ReadFile(src)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", src, err)
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return "", fmt.Errorf("parse %s: %w", src, err)
		}
		delete(cfg, "credsStore")
		delete(cfg, "credHelpers")
		delete(cfg, "auths")
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), raw, 0o600); err != nil {
		return "", err
	}
	return dir, nil
}

func ambientDockerConfig() (string, bool) {
	dir := os.Getenv("DOCKER_CONFIG")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		dir = filepath.Join(home, ".docker")
	}
	path := filepath.Join(dir, "config.json")
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	return path, true
}

// Env returns the environment a `docker` command runs with to use the
// configuration directory: the ambient environment with `DOCKER_CONFIG`
// pointing at `dir`.
func Env(dir string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "DOCKER_CONFIG=") {
			env = append(env, kv)
		}
	}
	return append(env, "DOCKER_CONFIG="+dir)
}

// DockerLogin logs the Docker CLI configured in `dir` in to the registry at
// `registryHost` with an access token:
//
//	docker login -u oauth2accesstoken --password-stdin <registryHost>
//
// The engine performs the registry handshake itself, so the login proves the
// registry accepts the token before anything is pushed with it.
func DockerLogin(ctx context.Context, dir, registryHost, accessToken string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "login", "--username", LoginUsername, "--password-stdin", registryHost)
	cmd.Env = Env(dir)
	cmd.Stdin = strings.NewReader(accessToken)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker login %s: %w: %s", registryHost, err, strings.TrimSpace(string(out)))
	}
	return nil
}
