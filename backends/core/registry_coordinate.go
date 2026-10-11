package core

import "strings"

// A registry endpoint coordinate relocates a cloud's container registry: an
// operator names the address the registry is reached at — a harness pointed at
// a simulator, a sovereign cloud, a registry behind a private endpoint — and
// every image reference into the registry carries the coordinate's host while
// registry HTTP is dialed at its URL. Amazon ECR, Google Artifact Registry, and
// Azure Container Registry each take one; these two functions are how all
// three read it.
//
// The coordinate may carry a scheme. A coordinate that names a bare host means
// https://, because that is what a registry host means.

// RegistryCoordinateHost returns the bare host a registry coordinate names
// (scheme and trailing slash stripped), or "" when no coordinate is set.
func RegistryCoordinateHost(coordinate string) string {
	ep := strings.TrimSpace(coordinate)
	if ep == "" {
		return ""
	}
	ep = strings.TrimPrefix(ep, "https://")
	ep = strings.TrimPrefix(ep, "http://")
	return strings.TrimRight(ep, "/")
}

// RegistryCoordinateURL returns the `scheme://host` registry HTTP is dialed at
// for a coordinate, or "" when no coordinate is set.
func RegistryCoordinateURL(coordinate string) string {
	ep := strings.TrimRight(strings.TrimSpace(coordinate), "/")
	if ep == "" {
		return ""
	}
	if !strings.HasPrefix(ep, "http://") && !strings.HasPrefix(ep, "https://") {
		return "https://" + ep
	}
	return ep
}
