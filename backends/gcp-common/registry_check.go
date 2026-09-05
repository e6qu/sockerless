package gcpcommon

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

// CheckTagExistsTimeout caps the Artifact Registry HEAD probe. A registry HEAD
// answers well under a second; the bound keeps a ContainerStart that misses the
// cache faster than it was before the probe existed.
const CheckTagExistsTimeout = 5 * time.Second

// CheckTagExists reports whether `imageURI` (a fully qualified
// `<host>/<project>/<repository>/<image>:<tag>` reference) is already present
// in Artifact Registry, by a HEAD on its manifest authenticated with
// Application Default Credentials and dialed at the registry endpoint
// coordinate — the same destination the overlay build pushes to and the
// workload pulls from.
//
// Only a definite answer from the registry is a result: 200 means present, 404
// means absent. Everything else is an error — a credential that cannot be
// minted, a registry that refuses the credential, a repository the identity
// cannot reach, a transport failure — because each of those would also fail the
// Cloud Build that a cache miss triggers, and treating them as a miss would
// spend that build on every start while hiding the cause.
//
// Cloud Build deduplicates sources by layer, not by tag, so even a fully cached
// build costs ~25-30s of staging, worker start, and manifest push. For overlays
// already pinned in the registry (prewarmed, or reused from a prior deploy)
// this probe preempts the whole round-trip.
func CheckTagExists(ctx context.Context, registryEndpoint, imageURI string) (bool, error) {
	registry, repo, tag, ok := splitRegistryRepoTag(imageURI)
	if !ok {
		return false, fmt.Errorf("image reference %q is not <host>/<repository>/<image>:<tag>", imageURI)
	}
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return false, fmt.Errorf("find default credentials: %w", err)
	}
	token, err := creds.TokenSource.Token()
	if err != nil {
		return false, fmt.Errorf("mint access token: %w", err)
	}
	headURL := fmt.Sprintf("%s/v2/%s/manifests/%s",
		core.OCIRegistryBaseURL(RegistryEndpointURL(registryEndpoint), registry), repo, tag)
	checkCtx, cancel := context.WithTimeout(ctx, CheckTagExistsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(checkCtx, http.MethodHead, headURL, nil)
	if err != nil {
		return false, err
	}
	core.SetOCIHost(req, registry)
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.index.v1+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("HEAD %s: %w", headURL, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("HEAD %s: HTTP %d: %s", headURL, resp.StatusCode, strings.TrimSpace(string(body)))
	}
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
