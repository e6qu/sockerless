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
//
// The repository and actions are not part of the request: Google Artifact
// Registry accepts the identity's own OAuth2 access token on every call, and
// what it authorizes is decided by that identity's IAM roles rather than by a
// scope named when the token is minted.
func (a *ARAuthProvider) GetToken(registry, repository string, actions ...string) (string, error) {
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

// IsCloudRegistry returns true if the registry is a GCP Artifact Registry or
// GCR — or the relocated AR coordinate (SOCKERLESS_GCP_AR_ENDPOINT) a sim
// harness sets, which carries overlay/base refs on its own host rather than
// `<region>-docker.pkg.dev`.
func (a *ARAuthProvider) IsCloudRegistry(registry string) bool {
	return core.IsGCPRegistry(registry) || IsOverlayCoordinateRegistry(registry)
}

// RegistryEndpoint returns the Artifact Registry endpoint override (the
// backend's reachable sim/cloud endpoint), if any, for a cloud-registry ref.
// The image reference itself remains the cloud AR/GCR (or relocated-coordinate)
// reference; only the network destination of registry HTTP changes — so a
// metadata fetch for a coordinate ref like `127.0.0.1:5000/...` is routed to the
// backend-reachable sim endpoint instead of dialing the published coordinate
// host (unreachable from inside the backend's container, and over the wrong
// scheme).
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
	authToken, err := a.GetToken(registry, repo, core.ActionPull, core.ActionDelete)
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
		core.SetOCIHost(req, registry)
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
