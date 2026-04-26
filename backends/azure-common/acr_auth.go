// Package azurecommon provides shared Azure functionality for ACA and AZF backends.
package azurecommon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
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
}

// NewACRAuthProvider creates a new ACRAuthProvider.
func NewACRAuthProvider(logger zerolog.Logger) *ACRAuthProvider {
	return &ACRAuthProvider{Logger: logger}
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

// OnPush is a no-op for ACR — repositories are created implicitly on
// first push and the actual blob upload is done by
// BaseServer.ImagePush via core.OCIPush, which has access to the
// image's layer data through the local store. OnPush used to also
// call OCIPush here without layer data, which always failed (BUG-763).
func (a *ACRAuthProvider) OnPush(imageID, registry, repo, tag string) error {
	return nil
}

// OnTag syncs a tag to ACR by fetching the source manifest and re-putting it with the new tag.
// Errors are returned to the caller (ImageManager) which aggregates
// them and surfaces via HTTP error per BUG-825 + the no-fallbacks rule.
func (a *ACRAuthProvider) OnTag(imageID, registry, repo, newTag string) error {
	token, err := a.GetToken(registry)
	if err != nil {
		return fmt.Errorf("ACR auth: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	baseURL := fmt.Sprintf("https://%s", registry)

	// Try to get existing manifest for the source image
	srcDigest := strings.TrimPrefix(imageID, "sha256:")
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

// OnRemove deletes manifests from ACR. Graceful on 404/405 (already
// gone / sim doesn't support DELETE). BUG-829: previously every
// per-tag failure was logged + `continue`, so OnRemove returned nil
// success even when some tags couldn't be removed and the ACR-side
// state diverged from local. Now aggregates per-tag failures and
// surfaces them per BUG-825 + the no-fallbacks rule.
func (a *ACRAuthProvider) OnRemove(registry, repo string, tags []string) error {
	token, err := a.GetToken(registry)
	if err != nil {
		return fmt.Errorf("ACR auth: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}

	var failures []string
	for _, tag := range tags {
		// Try to get the manifest digest first.
		manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)
		req, _ := http.NewRequest("HEAD", manifestURL, nil)
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
		delURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, digest)
		delReq, _ := http.NewRequest("DELETE", delURL, nil)
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
