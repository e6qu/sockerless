package gcpcommon

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2/google"
)

// CheckTagExistsTimeout caps the AR HEAD precheck. The default Docker
// registry HEAD response is well under a second; a 5s timeout makes the
// overall ContainerStart faster on cache misses (Cloud Build path) than
// it was before the precheck existed.
const CheckTagExistsTimeout = 5 * time.Second

// CheckTagExists returns true if the tag already exists in Artifact
// Registry. The tag must be a fully-qualified <region>-docker.pkg.dev URI
// (e.g. us-central1-docker.pkg.dev/PROJECT/REPO/IMAGE:TAG).
//
// On any non-200 response or transport error, returns false — the caller
// proceeds to Cloud Build. False positives (returning false when the tag
// IS present) cost the cache miss only, which is the prior behaviour.
// False negatives are impossible because we always re-tag the same content
// hash.
//
// Rationale: Cloud Build's source-deduplication is layer-level, not
// tag-level. Even on a full cache hit it still spends ~25-30s on stage,
// build worker startup, and manifest push. For overlay images that are
// already pinned in AR (prewarm pool, or warm reuse from a prior Service
// deploy), this HEAD preempts the entire Cloud Build round-trip and
// keeps ContainerStart well under gitlab-runner's 120s SDK timeout.
func CheckTagExists(ctx context.Context, imageURI string) bool {
	registry, repo, tag, ok := splitRegistryRepoTag(imageURI)
	if !ok {
		return false
	}
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return false
	}
	token, err := creds.TokenSource.Token()
	if err != nil {
		return false
	}
	headURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)
	checkCtx, cancel := context.WithTimeout(ctx, CheckTagExistsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(checkCtx, http.MethodHead, headURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.index.v1+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// splitRegistryRepoTag parses <registry>/<repo>/<image>:<tag> into
// (registry, "<repo>/<image>", tag, true) or returns ok=false.
func splitRegistryRepoTag(uri string) (registry, repo, tag string, ok bool) {
	colonIdx := strings.LastIndex(uri, ":")
	slashIdx := strings.LastIndex(uri, "/")
	if colonIdx <= slashIdx || colonIdx == -1 {
		return "", "", "", false
	}
	tag = uri[colonIdx+1:]
	prefix := uri[:colonIdx]
	firstSlash := strings.Index(prefix, "/")
	if firstSlash == -1 {
		return "", "", "", false
	}
	registry = prefix[:firstSlash]
	repo = prefix[firstSlash+1:]
	if registry == "" || repo == "" || tag == "" {
		return "", "", "", false
	}
	return registry, repo, tag, true
}
