package gcpcommon

import (
	"os"
	"strings"
)

// OverlayRegistryHost returns the Artifact Registry host for sockerless overlay
// image refs. It mirrors azure-common's SOCKERLESS_AZURE_ACR_ENDPOINT: the
// registry endpoint is a coordinate, not a code path. By default it is the real
// Artifact Registry host `<region>-docker.pkg.dev`; an operator (or a test
// harness pointed at the simulator) sets SOCKERLESS_GCP_AR_ENDPOINT to a
// different registry coordinate — e.g. the simulator's `/v2/` address — and the
// overlay build→push and the workload→pull both target it. The backend code is
// identical against cloud and sim; only the coordinate value differs. The value
// is a registry host (an http:// or https:// scheme, if present, is stripped).
func OverlayRegistryHost(region string) string {
	if ep := strings.TrimSpace(os.Getenv("SOCKERLESS_GCP_AR_ENDPOINT")); ep != "" {
		ep = strings.TrimPrefix(ep, "https://")
		ep = strings.TrimPrefix(ep, "http://")
		return strings.TrimRight(ep, "/")
	}
	return region + "-docker.pkg.dev"
}
