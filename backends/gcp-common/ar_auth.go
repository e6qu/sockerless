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
// for Google Cloud registries (*.gcr.io, *-docker.pkg.dev, and the relocated
// Artifact Registry coordinate). Implements core.AuthProvider.
type ARAuthProvider struct {
	ctx    func() context.Context
	logger zerolog.Logger
	// registryEndpoint is the Artifact Registry endpoint coordinate
	// (Config.ARRegistryEndpoint, from SOCKERLESS_GCP_AR_ENDPOINT): "" when
	// the registry is reached at the host its references name, else the
	// coordinate whose host those references carry and whose URL registry
	// HTTP is dialed at.
	registryEndpoint string
}

// NewARAuthProvider creates an ARAuthProvider for the Artifact Registry
// reached at `registryEndpoint` — the same coordinate the overlay build
// pushes to and image references are resolved against, so a reference that
// carries the coordinate's host is recognised as a cloud-registry reference
// and dialed at the coordinate. Empty means the real `<region>-docker.pkg.dev`.
func NewARAuthProvider(ctxFunc func() context.Context, logger zerolog.Logger, registryEndpoint string) *ARAuthProvider {
	return &ARAuthProvider{ctx: ctxFunc, logger: logger, registryEndpoint: registryEndpoint}
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

// IsCloudRegistry returns true if the registry is a Google Artifact Registry or
// Container Registry host, or the relocated Artifact Registry coordinate — a
// reference that carries the coordinate's host instead of
// `<region>-docker.pkg.dev` is the same registry under another address.
func (a *ARAuthProvider) IsCloudRegistry(registry string) bool {
	return core.IsGCPRegistry(registry) || IsRelocatedRegistry(registry, a.registryEndpoint)
}

// RegistryEndpoint returns the URL registry HTTP is dialed at for a
// cloud-registry reference: the relocated coordinate when one is set, "" when
// the reference's own host is the destination. The reference itself is
// unchanged; only the network destination — and its scheme — comes from the
// coordinate, so a reference carrying `127.0.0.1:5000/...` is dialed at
// `http://127.0.0.1:5000` when that is what the coordinate says.
func (a *ARAuthProvider) RegistryEndpoint(registry string) string {
	if !a.IsCloudRegistry(registry) {
		return ""
	}
	return RegistryEndpointURL(a.registryEndpoint)
}

// OnPush is a no-op for Artifact Registry: the blob and manifest upload is
// done by BaseServer.ImagePush via core.OCIPush, which has the image's layer
// data through the local store. The repository the push lands in is operator
// infrastructure (Terraform creates it), as it is for the real service.
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
