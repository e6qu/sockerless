package gcpcommon

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
	"golang.org/x/oauth2/google"
)

// ARAuthProvider handles Artifact Registry authentication and OCI operations
// for GCP cloud registries (*.gcr.io, *-docker.pkg.dev).
// Implements core.AuthProvider.
type ARAuthProvider struct {
	ctx         func() context.Context
	logger      zerolog.Logger
	endpointURL string
}

// NewARAuthProvider creates a new ARAuthProvider.
func NewARAuthProvider(ctxFunc func() context.Context, logger zerolog.Logger, endpointURL ...string) *ARAuthProvider {
	p := &ARAuthProvider{ctx: ctxFunc, logger: logger}
	if len(endpointURL) > 0 {
		p.endpointURL = endpointURL[0]
	}
	return p
}

// GetToken returns a Bearer token for the given registry using Application Default Credentials.
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

// RegistryEndpoint returns the operator-configured Artifact Registry endpoint
// override, if any. The image reference remains the cloud AR/GCR reference.
func (a *ARAuthProvider) RegistryEndpoint(registry string) string {
	if !a.IsCloudRegistry(registry) {
		return ""
	}
	return a.endpointURL
}

// OnPush is a no-op for Artifact Registry — repositories are created
// implicitly on first push, and the actual blob upload is done by
// BaseServer.ImagePush via core.OCIPush, which has access to the
// image's layer data through the local store. OnPush used to also
// call OCIPush here without layer data, which always failed.
func (a *ARAuthProvider) OnPush(imageID, registry, repo, tag string) error {
	return nil
}

// OnTag is a no-op for Artifact Registry — manifest re-PUT for the
// new tag is handled by BaseServer.ImagePush.
func (a *ARAuthProvider) OnTag(imageID, registry, repo, newTag string) error {
	_ = imageID
	_ = registry
	_ = repo
	_ = newTag
	return nil
}

// OnRemove deletes manifests from Artifact Registry by tag.
// Gracefully handles 404 (already deleted) and 405 (not supported by registry/simulator).
// The auth token is obtained internally via GetToken.
func (a *ARAuthProvider) OnRemove(registry, repo string, tags []string) error {
	authToken, err := a.GetToken(registry)
	if err != nil {
		return fmt.Errorf("get token for remove: %w", err)
	}

	// Aggregate per-tag failures and return them to the ImageManager
	// which surfaces the combined error (previously each per-tag failure
	// was logged + `continue`, so OnRemove returned nil even when some
	// tags couldn't be deleted — `docker rmi <ar-uri>` reported success
	// while the AR-side state diverged).
	var failures []string
	for _, tag := range tags {
		deleteBase := "https://" + registry
		if endpoint := strings.TrimRight(a.RegistryEndpoint(registry), "/"); endpoint != "" {
			deleteBase = endpoint
		}
		deleteURL := fmt.Sprintf("%s/v2/%s/manifests/%s", deleteBase, repo, tag)
		req, rerr := http.NewRequest(http.MethodDelete, deleteURL, nil)
		if rerr != nil {
			failures = append(failures, fmt.Sprintf("%s: build request: %v", tag, rerr))
			continue
		}
		core.SetOCIAuth(req, authToken)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, rerr := client.Do(req)
		if rerr != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", tag, rerr))
			continue
		}
		io.ReadAll(resp.Body) //nolint:errcheck
		_ = resp.Body.Close()

		// 200, 202: success; 404: already gone; 405: not supported (simulator).
		switch resp.StatusCode {
		case http.StatusOK, http.StatusAccepted, http.StatusNotFound, http.StatusMethodNotAllowed:
			// OK
		default:
			failures = append(failures, fmt.Sprintf("%s: HTTP %d", tag, resp.StatusCode))
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("AR delete failed for some tags: %s", strings.Join(failures, "; "))
	}
	return nil
}

// Compile-time check that ARAuthProvider implements core.AuthProvider.
var _ core.AuthProvider = (*ARAuthProvider)(nil)
