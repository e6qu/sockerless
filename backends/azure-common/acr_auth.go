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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
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
}

// NewACRAuthProvider creates a new ACRAuthProvider. The registry endpoint
// coordinate (SOCKERLESS_AZURE_ACR_ENDPOINT) is honored: by default the real
// `<acr>.azurecr.io` host, but a harness pointed at the simulator (or a
// sovereign cloud) sets it to a different registry coordinate — e.g. the
// simulator's `/v2/` address — and OnTag/OnRemove target it. The backend code is
// identical against cloud and sim; only the coordinate value differs. An
// explicit endpointURL may be passed; if omitted the env coordinate is read.
func NewACRAuthProvider(logger zerolog.Logger, endpointURL ...string) *ACRAuthProvider {
	p := &ACRAuthProvider{Logger: logger}
	if len(endpointURL) > 0 && endpointURL[0] != "" {
		p.endpointURL = endpointURL[0]
	} else {
		p.endpointURL = strings.TrimSpace(os.Getenv("SOCKERLESS_AZURE_ACR_ENDPOINT"))
	}
	return p
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

// IsCloudRegistry returns true if the registry is an Azure Container Registry
// (*.azurecr.io) — or the relocated ACR coordinate
// (SOCKERLESS_AZURE_ACR_ENDPOINT) a sim harness sets, which carries
// overlay/base refs on its own host rather than `<acr>.azurecr.io`. Mirrors
// gcp-common's ARAuthProvider.IsCloudRegistry recognizing the overlay
// coordinate.
func (a *ACRAuthProvider) IsCloudRegistry(registry string) bool {
	return strings.HasSuffix(registry, ".azurecr.io") || a.isCoordinateRegistry(registry)
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

// OnPush is a no-op for ACR — repositories are created implicitly on
// first push and the actual blob upload is done by
// BaseServer.ImagePush via core.OCIPush, which has access to the
// image's layer data through the local store. OnPush used to also
// call OCIPush here without layer data, which always failed.
func (a *ACRAuthProvider) OnPush(imageID, registry, repo, tag string) error {
	return nil
}

// OnTag syncs a tag to ACR by fetching the source manifest and re-putting it with the new tag.
// Errors are returned to the caller (ImageManager) which aggregates
// them and surfaces via HTTP error per the no-fallbacks rule.
func (a *ACRAuthProvider) OnTag(imageID, registry, repo, newTag string) error {
	token, err := a.GetToken(registry)
	if err != nil {
		return fmt.Errorf("ACR auth: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	baseURL := a.ociBaseURL(registry)

	// Try to get existing manifest for the source image
	srcDigest := strings.TrimPrefix(imageID, "sha256:")
	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/sha256:%s", baseURL, repo, srcDigest)
	req, err := http.NewRequest("GET", manifestURL, nil)
	if err != nil {
		return fmt.Errorf("build manifest GET request: %w", err)
	}
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
	putReq, err := http.NewRequest("PUT", putURL, bytes.NewReader(manifestData))
	if err != nil {
		return fmt.Errorf("build manifest PUT request: %w", err)
	}
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

// OnRemove deletes manifests from ACR. Graceful on 404/405 (already
// gone / sim doesn't support DELETE). Aggregates per-tag failures and
// surfaces them per the no-fallbacks rule (previously each per-tag
// failure was logged + `continue`, so OnRemove returned nil success
// even when some tags couldn't be removed and the ACR-side state
// diverged from local).
func (a *ACRAuthProvider) OnRemove(registry, repo string, tags []string) error {
	token, err := a.GetToken(registry)
	if err != nil {
		return fmt.Errorf("ACR auth: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}

	baseURL := a.ociBaseURL(registry)

	var failures []string
	for _, tag := range tags {
		// Try to get the manifest digest first.
		manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", baseURL, repo, tag)
		req, rerr := http.NewRequest("HEAD", manifestURL, nil)
		if rerr != nil {
			failures = append(failures, fmt.Sprintf("%s: build HEAD request: %v", tag, rerr))
			continue
		}
		req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")
		core.SetOCIAuth(req, token)

		headResp, err := client.Do(req)
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
		delReq, rerr := http.NewRequest("DELETE", delURL, nil)
		if rerr != nil {
			failures = append(failures, fmt.Sprintf("%s: build DELETE request: %v", tag, rerr))
			continue
		}
		core.SetOCIAuth(delReq, token)

		delResp, err := client.Do(delReq)
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
