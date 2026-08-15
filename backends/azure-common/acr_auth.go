// Package azurecommon provides shared Azure functionality for ACA and AZF backends.
package azurecommon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// ACRAuthProvider handles authentication and OCI operations for Azure Container Registry.
// Implements core.AuthProvider.
type ACRAuthProvider struct {
	Logger zerolog.Logger
	// endpointURL is the backend-reachable registry endpoint override
	// (SOCKERLESS_AZURE_ACR_ENDPOINT). When set — a relocated/sovereign or
	// simulated registry — OCI HTTP (tag/remove) routes here instead of
	// dialing the published `<acr>.azurecr.io` over the wrong scheme. Empty
	// on the real public cloud.
	endpointURL string
	// tokens mints the registry's own access tokens, which is the only
	// credential its /v2/ data plane accepts.
	tokens *acrTokenService
}

// NewACRAuthProvider creates a new ACRAuthProvider. The registry endpoint
// coordinate (SOCKERLESS_AZURE_ACR_ENDPOINT) is honored: by default the real
// `<acr>.azurecr.io` host, but a harness pointed at the simulator (or a
// sovereign cloud) sets it to a different registry coordinate — e.g. the
// simulator's `/v2/` address — and OnTag/OnRemove target it. The backend code is
// identical against cloud and sim; only the coordinate value differs. An
// explicit endpointURL may be passed; if omitted the env coordinate is read.
func NewACRAuthProvider(logger zerolog.Logger, endpointURL ...string) *ACRAuthProvider {
	p := &ACRAuthProvider{Logger: logger, tokens: newACRTokenService(nil)}
	if len(endpointURL) > 0 && endpointURL[0] != "" {
		p.endpointURL = endpointURL[0]
	} else {
		p.endpointURL = strings.TrimSpace(os.Getenv("SOCKERLESS_AZURE_ACR_ENDPOINT"))
	}
	return p
}

// NewACRAuthProviderWithCredential creates an ACRAuthProvider that
// authenticates as an already-resolved Microsoft Entra identity — the one the
// rest of the backend's Azure clients hold — instead of resolving the ambient
// one itself. The registry endpoint coordinate is honored exactly as in
// NewACRAuthProvider.
func NewACRAuthProviderWithCredential(logger zerolog.Logger, cred azcore.TokenCredential, endpointURL string) *ACRAuthProvider {
	p := NewACRAuthProvider(logger, endpointURL)
	p.tokens = newACRTokenService(cred)
	return p
}

// GetToken returns the Bearer credential the registry's Docker Registry HTTP
// API v2 data plane accepts for `repository` and `actions`.
//
// An Azure Container Registry does not accept a Microsoft Entra token on /v2/.
// The Entra token is exchanged at the registry's own token service — Entra
// token for an ACR refresh token at /oauth2/exchange, refresh token plus the
// requested scope for a scoped ACR access token at /oauth2/token — and the
// access token that comes back is the Bearer the data plane honours. The scope
// is the access the operation needs: an empty repository addresses the
// registry's catalog, and the actions are the Docker Registry HTTP API v2 ones
// ("pull", "push", "delete", "metadata_read").
func (a *ACRAuthProvider) GetToken(registry, repository string, actions ...string) (string, error) {
	token, err := a.tokens.AccessToken(context.Background(), a.ociBaseURL(registry), registry,
		ACRRegistryScope(repository, actions...))
	if err != nil {
		return "", err
	}
	return "Bearer " + token, nil
}

// acrLoginServerSuffixes are the login-server suffixes Azure Container
// Registry serves on, one per cloud: the public cloud, Azure China and Azure
// Government. A registry in any of them is ours and is reached with the
// registry's own token, not anonymously.
var acrLoginServerSuffixes = []string{".azurecr.io", ".azurecr.cn", ".azurecr.us"}

// IsCloudRegistry returns true if the registry is an Azure Container Registry
// — or the relocated ACR coordinate (SOCKERLESS_AZURE_ACR_ENDPOINT) a sim
// harness sets, which carries overlay/base refs on its own host rather than
// `<acr>.azurecr.io`. Mirrors gcp-common's ARAuthProvider.IsCloudRegistry
// recognizing the overlay coordinate.
func (a *ACRAuthProvider) IsCloudRegistry(registry string) bool {
	for _, suffix := range acrLoginServerSuffixes {
		if strings.HasSuffix(registry, suffix) {
			return true
		}
	}
	return a.isCoordinateRegistry(registry)
}

// isCoordinateRegistry reports whether `registry` is the relocated ACR
// coordinate host (the bare host of endpointURL).
func (a *ACRAuthProvider) isCoordinateRegistry(registry string) bool {
	host := a.coordinateHost()
	return host != "" && registry == host
}

// coordinateHost returns the bare host of endpointURL (scheme + trailing slash
// stripped), or "" when no coordinate is set.
func (a *ACRAuthProvider) coordinateHost() string {
	ep := strings.TrimSpace(a.endpointURL)
	if ep == "" {
		return ""
	}
	ep = strings.TrimPrefix(ep, "https://")
	ep = strings.TrimPrefix(ep, "http://")
	return strings.TrimRight(ep, "/")
}

// RegistryEndpoint returns the ACR endpoint override (the backend-reachable
// sim/cloud endpoint, with scheme), if any, for a cloud-registry ref. The image
// reference itself remains the cloud ACR (or relocated-coordinate) reference;
// only the network destination of registry HTTP changes — so OnTag/OnRemove for
// a coordinate ref are routed to the backend-reachable endpoint instead of
// dialing the published coordinate host over the wrong scheme. Mirrors
// gcp-common's ARAuthProvider.RegistryEndpoint.
func (a *ACRAuthProvider) RegistryEndpoint(registry string) string {
	if !a.IsCloudRegistry(registry) {
		return ""
	}
	return a.endpointURL
}

// ociBaseURL returns the base URL ("scheme://host", no trailing slash) to dial
// OCI registry HTTP for `registry`: the endpoint override when set, otherwise
// `https://<registry>`.
func (a *ACRAuthProvider) ociBaseURL(registry string) string {
	if ep := strings.TrimRight(a.RegistryEndpoint(registry), "/"); ep != "" {
		return ep
	}
	return "https://" + registry
}

// OnPush is a no-op for ACR — repositories are created implicitly on first
// push, and the blob upload itself is done by BaseServer.ImagePush via
// core.OCIPush, which is the only place with access to the image's layer data.
func (a *ACRAuthProvider) OnPush(imageID, registry, repo, tag string) error {
	return nil
}

// acrDo sends one registry request authenticated with an ACR access token for
// `scope`, routed to the registry named by `registry` even when the endpoint
// coordinate relocates the network destination. A registry that refuses the
// token is the authority on it having gone stale, so a 401 drops the cached
// token and the request is sent once more with a freshly minted one — the same
// renewal the Azure SDK for Go's registry clients perform.
func (a *ACRAuthProvider) acrDo(client *http.Client, registry, scope string, build func() (*http.Request, error)) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		token, err := a.tokens.AccessToken(context.Background(), a.ociBaseURL(registry), registry, scope)
		if err != nil {
			return nil, fmt.Errorf("ACR auth: %w", err)
		}
		req, err := build()
		if err != nil {
			return nil, err
		}
		core.SetOCIHost(req, registry)
		core.SetOCIAuth(req, "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusUnauthorized || attempt > 0 {
			return resp, nil
		}
		resp.Body.Close()
		a.tokens.Invalidate(registry)
	}
}

// OnTag syncs a tag to ACR by fetching the source manifest and re-putting it with the new tag.
// Errors are returned to the caller (ImageManager) which aggregates
// them and surfaces via HTTP error per the no-fallbacks rule.
func (a *ACRAuthProvider) OnTag(imageID, registry, repo, newTag string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	baseURL := a.ociBaseURL(registry)
	// Reading a manifest pulls and writing one pushes, and a registry requires
	// pull alongside push to accept a write.
	scope := ACRRegistryScope(repo, core.ActionPull, core.ActionPush)

	// Try to get existing manifest for the source image
	srcDigest := strings.TrimPrefix(imageID, "sha256:")
	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/sha256:%s", baseURL, repo, srcDigest)
	resp, err := a.acrDo(client, registry, scope, func() (*http.Request, error) {
		req, rerr := http.NewRequest("GET", manifestURL, nil)
		if rerr != nil {
			return nil, fmt.Errorf("build manifest GET request: %w", rerr)
		}
		req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")
		return req, nil
	})
	if err != nil {
		return fmt.Errorf("manifest fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("source manifest not found in ACR: %d", resp.StatusCode)
	}

	manifestData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read source manifest: %w", err)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/vnd.docker.distribution.manifest.v2+json"
	}

	// PUT manifest with new tag
	putURL := fmt.Sprintf("%s/v2/%s/manifests/%s", baseURL, repo, newTag)
	putResp, err := a.acrDo(client, registry, scope, func() (*http.Request, error) {
		req, rerr := http.NewRequest("PUT", putURL, bytes.NewReader(manifestData))
		if rerr != nil {
			return nil, fmt.Errorf("build manifest PUT request: %w", rerr)
		}
		req.Header.Set("Content-Type", contentType)
		return req, nil
	})
	if err != nil {
		return fmt.Errorf("manifest PUT: %w", err)
	}
	putResp.Body.Close()

	if putResp.StatusCode != 201 && putResp.StatusCode != 200 {
		return fmt.Errorf("manifest PUT returned %d", putResp.StatusCode)
	}

	return nil
}

// OnRemove deletes manifests from ACR. A 404 means the manifest is already
// gone and a 405 means the registry does not accept deletions, and neither is
// a failure to remove something. Every other per-tag failure is aggregated and
// returned, so a removal that left ACR holding tags the local store no longer
// has reports it rather than answering success.
func (a *ACRAuthProvider) OnRemove(registry, repo string, tags []string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	baseURL := a.ociBaseURL(registry)
	// Reading the manifest to resolve its digest pulls; removing it deletes.
	scope := ACRRegistryScope(repo, core.ActionPull, core.ActionDelete)

	var failures []string
	for _, tag := range tags {
		// Try to get the manifest digest first.
		manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", baseURL, repo, tag)
		headResp, err := a.acrDo(client, registry, scope, func() (*http.Request, error) {
			req, rerr := http.NewRequest("HEAD", manifestURL, nil)
			if rerr != nil {
				return nil, fmt.Errorf("build HEAD request: %w", rerr)
			}
			req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")
			return req, nil
		})
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: HEAD failed: %v", tag, err))
			continue
		}
		headResp.Body.Close()

		if headResp.StatusCode == 404 || headResp.StatusCode == 405 {
			continue // manifest not found / DELETE not supported by sim — graceful
		}

		if headResp.StatusCode != 200 {
			failures = append(failures, fmt.Sprintf("%s: HEAD HTTP %d", tag, headResp.StatusCode))
			continue
		}

		digest := headResp.Header.Get("Docker-Content-Digest")
		if digest == "" {
			digest = tag
		}

		// DELETE manifest.
		delURL := fmt.Sprintf("%s/v2/%s/manifests/%s", baseURL, repo, digest)
		delResp, err := a.acrDo(client, registry, scope, func() (*http.Request, error) {
			return http.NewRequest("DELETE", delURL, nil)
		})
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: DELETE failed: %v", tag, err))
			continue
		}
		delResp.Body.Close()

		switch delResp.StatusCode {
		case http.StatusOK, http.StatusAccepted, http.StatusNotFound, http.StatusMethodNotAllowed:
			// OK
		default:
			failures = append(failures, fmt.Sprintf("%s: DELETE HTTP %d", tag, delResp.StatusCode))
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("ACR delete failed for some tags: %s", strings.Join(failures, "; "))
	}
	return nil
}

// Compile-time check that ACRAuthProvider implements core.AuthProvider.
var _ core.AuthProvider = (*ACRAuthProvider)(nil)
