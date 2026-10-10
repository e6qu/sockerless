package core

import "strings"

// SplitDockerRef splits an image reference into registry, repository,
// and version: the suffix that names the image within its repository,
// `@<digest>` when the reference carries a digest (Docker resolves
// `name:tag@digest` by the digest alone) and `:<tag>` otherwise, with the
// tag defaulting to "latest". A first path segment that contains a `.` or
// `:` is the registry, as in Docker's own reference grammar. This is the
// decomposition the cloud image resolvers rewrite from: they rebuild a
// reference as `<host>/<repository><version>`.
//
//	"nginx:alpine"                  → ("", "nginx", ":alpine")
//	"docker.io/library/alpine:3.18" → ("docker.io", "library/alpine", ":3.18")
//	"ghcr.io/owner/repo"            → ("ghcr.io", "owner/repo", ":latest")
//	"alpine:3.20@sha256:abc…"       → ("", "alpine", "@sha256:abc…")
func SplitDockerRef(ref string) (registry, repo, version string) {
	if i := strings.IndexByte(ref, '/'); i > 0 {
		if prefix := ref[:i]; strings.ContainsAny(prefix, ".:") {
			registry = prefix
			ref = ref[i+1:]
		}
	}
	repo, tag, digest := SplitImageReference(ref)
	switch {
	case digest != "":
		version = "@" + digest
	case tag != "":
		version = ":" + tag
	default:
		version = ":latest"
	}
	return registry, repo, version
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
