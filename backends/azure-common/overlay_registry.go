package azurecommon

import (
	"os"
	"strings"
)

// AzureRegistryHost returns the registry host for sockerless overlay/workload
// image refs. It mirrors gcp-common's OverlayRegistryHost: the default is
// `<acrName>.azurecr.io`, but an operator can relocate the registry — a
// sovereign/custom-cloud ACR, or a reachable local/simulated registry — via
// SOCKERLESS_AZURE_ACR_ENDPOINT, the same coordinate the overlay build→push
// path in build.go already honors. Keeping the resolvers on this coordinate
// (instead of a hardcoded `.azurecr.io`) means the ref the backend deploys/pulls
// points at the registry the overlay was actually pushed to.
func AzureRegistryHost(acrName string) string {
	if ep := azureRegistryCoordinate(); ep != "" {
		return ep
	}
	return strings.TrimSuffix(acrName, ".azurecr.io") + ".azurecr.io"
}

// azureRegistryCoordinate returns the relocated registry host from
// SOCKERLESS_AZURE_ACR_ENDPOINT (scheme + trailing slash stripped), or "" when
// unset (use the real `<acr>.azurecr.io`).
func azureRegistryCoordinate() string {
	ep := strings.TrimSpace(os.Getenv("SOCKERLESS_AZURE_ACR_ENDPOINT"))
	if ep == "" {
		return ""
	}
	ep = strings.TrimPrefix(ep, "https://")
	ep = strings.TrimPrefix(ep, "http://")
	return strings.TrimRight(ep, "/")
}
