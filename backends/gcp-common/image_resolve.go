package gcpcommon

import (
	"strings"

	core "github.com/sockerless/backend-core"
)

// ResolveGCPImageURI converts a Docker image reference to an Artifact Registry URI
// that Cloud Run and Cloud Run Functions can use.
//
// If the image is already an Artifact Registry or GCR URI, it is returned as-is.
// Otherwise, the reference is rewritten to point to an Artifact Registry remote
// repository that proxies Docker Hub. The remote repository ("docker-hub") must
// be pre-configured at the project level.
//
// The Artifact Registry host comes from the registry endpoint coordinate
// (`registryEndpoint`, Config.ARRegistryEndpoint): by default the real
// `<region>-docker.pkg.dev`, or the host of a relocated registry — a harness
// pointed at the simulator names the simulator's `/v2/` address, and the same
// rewrite then routes the base image through that registry's docker-hub
// remote repository. The backend code is identical against cloud and
// simulator; only the coordinate value differs.
//
// Examples (real-cloud default host shown; the sim coordinate substitutes for it):
//
//	"alpine:latest"        → "{host}/{project}/docker-hub/library/alpine:latest"
//	"nginx:alpine"         → "{host}/{project}/docker-hub/library/nginx:alpine"
//	"myorg/app:v1"         → "{host}/{project}/docker-hub/myorg/app:v1"
//	"{region}-docker.pkg.dev/{project}/my-repo/img:tag" → used as-is
//	"gcr.io/{project}/img:tag"                          → used as-is
func ResolveGCPImageURI(ref, project, region, registryEndpoint string) string {
	// Already an Artifact Registry URI — use as-is
	if strings.Contains(ref, "-docker.pkg.dev/") {
		return ref
	}

	// Already a GCR URI — use as-is
	if strings.HasSuffix(strings.SplitN(ref, "/", 2)[0], ".gcr.io") {
		return ref
	}

	// gitlab-runner permission containers reference images by bare
	// `sha256:<digest>` (no repo). The legacy `parseDockerRef` would
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
	registry, repo, tag := core.SplitDockerRef(ref)

	// Cloud Run only accepts images from gcr.io / docker.pkg.dev /
	// docker.io. Other registries must be mirrored via AR remote
	// repositories named after the registry. Add a mapping per remote
	// repo created in terraform/modules/cloudrun/main.tf.
	var arRepo string
	switch registry {
	case "", "docker.io", "registry-1.docker.io":
		arRepo = "docker-hub"
		// Docker Hub library images: "alpine" → "library/alpine"
		if !strings.Contains(repo, "/") {
			repo = "library/" + repo
		}
	case "registry.gitlab.com":
		// gitlab-runner-helper image lives here; AR remote-proxy
		// `gitlab-registry` proxies registry.gitlab.com.
		arRepo = "gitlab-registry"
	default:
		// Other registries (ghcr.io, quay.io, etc.) — return as-is.
		// Cloud Run will reject if not on its allow-list, but that's
		// the operator's responsibility to set up an AR proxy for.
		return ref
	}

	// Rewrite to the Artifact Registry remote repository at the registry the
	// coordinate names.
	return OverlayRegistryHost(region, registryEndpoint) + "/" + project + "/" + arRepo + "/" + repo + ":" + tag
}
