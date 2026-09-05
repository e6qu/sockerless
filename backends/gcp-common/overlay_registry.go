package gcpcommon

import "strings"

// The Artifact Registry endpoint coordinate.
//
// Google Artifact Registry is reached at `<region>-docker.pkg.dev`, and that
// host is both the network destination of every registry request and the host
// an image reference carries. An operator relocates the registry — a harness
// pointed at the simulator, a registry reached through a private endpoint — by
// setting SOCKERLESS_GCP_AR_ENDPOINT, the same coordinate azure-common honours
// as SOCKERLESS_AZURE_ACR_ENDPOINT. The value is read once into the backend's
// Config (`ARRegistryEndpoint`) and flows from there into every place the
// registry is named or dialed: the overlay build→push, the workload→pull
// reference, the tag-existence probe, the metadata fetch, and the image
// resolver. One coordinate, one reading; the code is identical against cloud
// and simulator.
//
// The coordinate may carry a scheme. Image references carry its host
// (`OverlayRegistryHost`); registry HTTP is dialed at its URL
// (`RegistryEndpointURL`), defaulting to https:// when the coordinate names a
// bare host, because that is what a registry host means.

// OverlayRegistryHost returns the Artifact Registry host that image references
// carry: the host of the relocated coordinate when one is set, else the real
// `<region>-docker.pkg.dev`.
func OverlayRegistryHost(region, registryEndpoint string) string {
	if host := registryCoordinateHost(registryEndpoint); host != "" {
		return host
	}
	return region + "-docker.pkg.dev"
}

// RegistryEndpointURL returns the `scheme://host` registry HTTP is dialed at
// when the registry is relocated, or "" when Artifact Registry is reached at the
// host its references name (https://<host>).
func RegistryEndpointURL(registryEndpoint string) string {
	ep := strings.TrimRight(strings.TrimSpace(registryEndpoint), "/")
	if ep == "" {
		return ""
	}
	if !strings.HasPrefix(ep, "http://") && !strings.HasPrefix(ep, "https://") {
		return "https://" + ep
	}
	return ep
}

// IsRelocatedRegistry reports whether `registry` — the host of an image
// reference — is the relocated Artifact Registry coordinate. Such a reference
// is a cloud-registry reference: it is authenticated with the backend's Google
// credential and dialed at the coordinate, exactly like one carrying
// `<region>-docker.pkg.dev`. Always false when no coordinate is set.
func IsRelocatedRegistry(registry, registryEndpoint string) bool {
	host := registryCoordinateHost(registryEndpoint)
	return host != "" && registry == host
}

// registryCoordinateHost returns the bare host of the coordinate (scheme and
// trailing slash stripped), or "" when the registry is not relocated.
func registryCoordinateHost(registryEndpoint string) string {
	ep := strings.TrimSpace(registryEndpoint)
	if ep == "" {
		return ""
	}
	ep = strings.TrimPrefix(ep, "https://")
	ep = strings.TrimPrefix(ep, "http://")
	return strings.TrimRight(ep, "/")
}
