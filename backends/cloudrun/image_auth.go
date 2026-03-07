package cloudrun

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	core "github.com/sockerless/backend-core"
	"golang.org/x/oauth2/google"
)

// ARAuthProvider handles Artifact Registry authentication and OCI operations
// for GCP cloud registries (*.gcr.io, *-docker.pkg.dev).
type ARAuthProvider struct {
	ctx func() context.Context
}

// NewARAuthProvider creates a new ARAuthProvider.
func NewARAuthProvider(ctxFunc func() context.Context) *ARAuthProvider {
	return &ARAuthProvider{ctx: ctxFunc}
}

// GetToken returns a Bearer token for the given registry using Application Default Credentials.
// Only call this for GCP registries (checked via IsCloudRegistry).
func (a *ARAuthProvider) GetToken(registry string) (string, error) {
	creds, err := google.FindDefaultCredentials(a.ctx(), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", fmt.Errorf("find default credentials: %w", err)
	}
	token, err := creds.TokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("get token: %w", err)
	}
	return "Bearer " + token.AccessToken, nil
}

// IsCloudRegistry returns true if the registry is a GCP Artifact Registry or GCR.
func (a *ARAuthProvider) IsCloudRegistry(registry string) bool {
	return core.IsGCPRegistry(registry)
}

// OnPush pushes a synthetic image to an OCI-compliant GCP registry using core.OCIPush.
// Returns the push result or an error. The caller should treat errors as non-fatal
// for cloud operations.
func (a *ARAuthProvider) OnPush(registry, repo, tag, authToken string) (*core.OCIPushResult, error) {
	return core.OCIPush(core.OCIPushOptions{
		Registry:   registry,
		Repository: repo,
		Tag:        tag,
		AuthToken:  authToken,
	})
}

// OnTag re-pushes a manifest with a new tag to sync the tag in Artifact Registry.
// This is a best-effort operation; callers should log and ignore errors.
func (a *ARAuthProvider) OnTag(registry, repo, newTag, authToken string) error {
	_, err := core.OCIPush(core.OCIPushOptions{
		Registry:   registry,
		Repository: repo,
		Tag:        newTag,
		AuthToken:  authToken,
	})
	return err
}

// OnRemove deletes a manifest from Artifact Registry by tag/digest.
// Gracefully handles 404 (already deleted) and 405 (not supported by registry/simulator).
// This is a best-effort operation; callers should log and ignore errors.
func (a *ARAuthProvider) OnRemove(registry, repo, ref, authToken string) error {
	deleteURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, ref)
	req, err := http.NewRequest(http.MethodDelete, deleteURL, nil)
	if err != nil {
		return fmt.Errorf("create delete request: %w", err)
	}
	core.SetOCIAuth(req, authToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("delete request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	io.ReadAll(resp.Body) //nolint:errcheck

	// 200, 202: success; 404: already gone; 405: not supported (simulator)
	switch resp.StatusCode {
	case http.StatusOK, http.StatusAccepted, http.StatusNotFound, http.StatusMethodNotAllowed:
		return nil
	default:
		return fmt.Errorf("delete manifest returned %d", resp.StatusCode)
	}
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
