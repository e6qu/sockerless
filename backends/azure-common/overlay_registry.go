package azurecommon

import (
	"strings"

	core "github.com/sockerless/backend-core"
)

// ACREndpointEnv is the environment variable that carries the Azure Container
// Registry endpoint coordinate. A backend reads it once into its Config and
// hands that value to every place the registry is named or dialed.
const ACREndpointEnv = "SOCKERLESS_AZURE_ACR_ENDPOINT"

// AzureRegistryHost returns the registry host for sockerless overlay/workload
// image refs. It mirrors gcp-common's OverlayRegistryHost: the default is
// `<acrName>.azurecr.io`, but an operator can relocate the registry — a
// sovereign/custom-cloud ACR, or a reachable local/simulated registry — with
// the registry endpoint coordinate (registryEndpoint, from
// SOCKERLESS_AZURE_ACR_ENDPOINT), the same coordinate the overlay build→push
// path honours. Keeping the resolvers on this coordinate (instead of a
// hardcoded `.azurecr.io`) means the ref the backend deploys/pulls points at
// the registry the overlay was actually pushed to.
func AzureRegistryHost(acrName, registryEndpoint string) string {
	if host := core.RegistryCoordinateHost(registryEndpoint); host != "" {
		return host
	}
	return strings.TrimSuffix(acrName, ".azurecr.io") + ".azurecr.io"
}
