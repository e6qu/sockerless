package azurecommon

import "strings"

// ResolveAzureImageURI normalizes a Docker image reference for Azure Container Apps
// and Azure Functions.
//
// If the image is already an ACR URI (*.azurecr.io/*), it is returned as-is.
// If an acrName is provided, Docker Hub images are rewritten to pull from ACR.
// Otherwise, the reference is normalized with the docker.io prefix for clarity.
//
// Azure Container Apps supports Docker Hub public images directly, so when no
// acrName is given, this function just normalizes the reference.
//
// Examples (no acrName):
//
//	"alpine:latest"                → "docker.io/library/alpine:latest"
//	"myorg/app:v1"                 → "docker.io/myorg/app:v1"
//	"myacr.azurecr.io/img:tag"    → "myacr.azurecr.io/img:tag" (unchanged)
//
// Examples (acrName="myacr"):
//
//	"alpine:latest"                → "myacr.azurecr.io/library/alpine:latest"
//	"myorg/app:v1"                 → "myacr.azurecr.io/myorg/app:v1"
//	"myacr.azurecr.io/img:tag"    → "myacr.azurecr.io/img:tag" (unchanged)
func ResolveAzureImageURI(ref, acrName string) string {
	// Already an ACR URI — use as-is
	if strings.Contains(ref, ".azurecr.io/") {
		return ref
	}

	// Parse the Docker reference
	registry, repo, tag := parseDockerRef(ref)

	// Only handle Docker Hub images
	switch registry {
	case "", "docker.io", "registry-1.docker.io":
		// Docker Hub library images: "alpine" → "library/alpine"
		if !strings.Contains(repo, "/") {
			repo = "library/" + repo
		}
	default:
		// Non-Docker Hub registries — return as-is
		return ref
	}

	// If ACR name is provided, rewrite to ACR URI
	if acrName != "" {
		// Strip any trailing .azurecr.io if someone passed the full hostname
		acrName = strings.TrimSuffix(acrName, ".azurecr.io")
		return acrName + ".azurecr.io/" + repo + ":" + tag
	}

	// No ACR — normalize with docker.io prefix
	return "docker.io/" + repo + ":" + tag
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
