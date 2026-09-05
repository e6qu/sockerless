package core

import "strings"

// SplitDockerRef splits an image reference into registry, repository,
// and tag, defaulting the tag to "latest". A first path segment that
// contains a `.` or `:` is the registry, as in Docker's own reference
// grammar. This is the decomposition the cloud image resolvers rewrite
// from; ParseImageRef is the stricter parser that keeps digests and does
// not invent a tag.
//
//	"nginx:alpine"                  → ("", "nginx", "alpine")
//	"docker.io/library/alpine:3.18" → ("docker.io", "library/alpine", "3.18")
//	"ghcr.io/owner/repo"            → ("ghcr.io", "owner/repo", "latest")
func SplitDockerRef(ref string) (registry, repo, tag string) {
	tag = "latest"
	if i := strings.IndexByte(ref, '/'); i > 0 {
		if prefix := ref[:i]; strings.ContainsAny(prefix, ".:") {
			registry = prefix
			ref = ref[i+1:]
		}
	}
	if i := strings.LastIndexByte(ref, ':'); i > 0 {
		repo, tag = ref[:i], ref[i+1:]
	} else {
		repo = ref
	}
	return registry, repo, tag
}

// ArchFromPlatform extracts the Docker architecture ("arm64" or "amd64")
// from a `linux/<arch>` build platform. A backend reports the cloud
// workload's architecture, not its own host's, in the Docker version
// response, so a client such as gitlab-runner selects a matching helper
// image. Anything that is not arm64 is amd64.
func ArchFromPlatform(platform string) string {
	if i := strings.LastIndex(platform, "/"); i >= 0 {
		platform = platform[i+1:]
	}
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "arm64", "aarch64":
		return "arm64"
	default:
		return "amd64"
	}
}
