package gcpcommon

import "strings"

// ResolveGCPImageURI converts a Docker image reference to an Artifact Registry URI
// that Cloud Run and Cloud Run Functions can use.
//
// If the image is already an Artifact Registry or GCR URI, it is returned as-is.
// Otherwise, the reference is rewritten to point to an Artifact Registry remote
// repository that proxies Docker Hub. The remote repository ("docker-hub") must
// be pre-configured at the project level.
//
// Examples:
//
//	"alpine:latest"        → "{region}-docker.pkg.dev/{project}/docker-hub/library/alpine:latest"
//	"nginx:alpine"         → "{region}-docker.pkg.dev/{project}/docker-hub/library/nginx:alpine"
//	"myorg/app:v1"         → "{region}-docker.pkg.dev/{project}/docker-hub/myorg/app:v1"
//	"{region}-docker.pkg.dev/{project}/my-repo/img:tag" → used as-is
//	"gcr.io/{project}/img:tag"                          → used as-is
func ResolveGCPImageURI(ref, project, region string) string {
	// Already an Artifact Registry URI — use as-is
	if strings.Contains(ref, "-docker.pkg.dev/") {
		return ref
	}

	// Already a GCR URI — use as-is
	if strings.HasSuffix(strings.SplitN(ref, "/", 2)[0], ".gcr.io") {
		return ref
	}

	// BUG-918: gitlab-runner permission containers reference images by
	// bare `sha256:<digest>` (no repo). The legacy `parseDockerRef` would
	// split this on `:` producing repo="sha256" tag="<digest>" → AR URL
	// `<AR>/docker-hub/library/sha256:<digest>` which Cloud Run rejects.
	// Bare digest refs can't be rewritten to AR — they must already be
	// in the local image store. Return as-is; caller (cloudrun backend)
	// resolves via Store.ResolveImage before calling us, so this path
	// only fires when Store lookup missed (genuine error).
	if strings.HasPrefix(ref, "sha256:") && !strings.Contains(ref, "/") {
		return ref
	}

	// Parse the Docker reference
	registry, repo, tag := parseDockerRef(ref)

	// Only rewrite Docker Hub images
	switch registry {
	case "", "docker.io", "registry-1.docker.io":
		// Docker Hub library images: "alpine" → "library/alpine"
		if !strings.Contains(repo, "/") {
			repo = "library/" + repo
		}
	default:
		// Non-Docker Hub registries (ghcr.io, quay.io, etc.) — return as-is
		return ref
	}

	// Rewrite to Artifact Registry remote repository URI
	return region + "-docker.pkg.dev/" + project + "/docker-hub/" + repo + ":" + tag
}

// parseDockerRef splits a Docker image reference into registry, repo, and tag.
// "nginx:alpine" → ("", "nginx", "alpine")
// "docker.io/library/alpine:3.18" → ("docker.io", "library/alpine", "3.18")
func parseDockerRef(ref string) (registry, repo, tag string) {
	tag = "latest"

	// Check for explicit registry (contains . or :port before first /)
	if i := strings.IndexByte(ref, '/'); i > 0 {
		prefix := ref[:i]
		if strings.ContainsAny(prefix, ".:") {
			registry = prefix
			ref = ref[i+1:]
		}
	}

	// Split repo:tag
	if i := strings.LastIndexByte(ref, ':'); i > 0 {
		repo = ref[:i]
		tag = ref[i+1:]
	} else {
		repo = ref
	}
	return
}
