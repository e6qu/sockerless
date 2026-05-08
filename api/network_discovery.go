// Network discovery driver categories.
//
// Distinct from api.NetworkDriver (which owns network create/list/remove,
// the Docker network REST API). This driver dimension answers "how do
// containers in the same user-defined network discover and reach each
// other by name?" — separate from VPC/subnet plumbing.
//
// Each backend selects exactly one NetworkDiscoveryKind at startup,
// defaulting to a per-backend value. Operator override via
// SOCKERLESS_<BACKEND>_NETWORK_DISCOVERY env. Empty or unknown name →
// backend startup error (no silent fallback).

package api

// NetworkDiscoveryKind names a discovery mechanism.
type NetworkDiscoveryKind string

const (
	// NetworkDiscoveryHostAliases injects /etc/hosts entries on each
	// container at materialize time. Works when peers share loopback
	// (multi-container revision) or when peer IPs are stable and known
	// at materialize time.
	NetworkDiscoveryHostAliases NetworkDiscoveryKind = "host-aliases"

	// NetworkDiscoveryCloudDNS uses a cloud-managed DNS zone with per-
	// container A/CNAME records. Examples: GCP Cloud DNS private zone,
	// Azure Private DNS Zone.
	NetworkDiscoveryCloudDNS NetworkDiscoveryKind = "cloud-dns"

	// NetworkDiscoveryServiceMesh uses cloud-native service-discovery
	// primitives (namespace + service + instance records). Example:
	// AWS Cloud Map.
	NetworkDiscoveryServiceMesh NetworkDiscoveryKind = "service-mesh"

	// NetworkDiscoveryNATGatewayOnly grants outbound internet access
	// without inter-container discovery. Used for one-shot workloads
	// that don't need to address peers (Lambda, single-container
	// Cloud Run Jobs).
	NetworkDiscoveryNATGatewayOnly NetworkDiscoveryKind = "nat-gateway-only"
)

// IsValid reports whether k is one of the four documented kinds.
func (k NetworkDiscoveryKind) IsValid() bool {
	switch k {
	case NetworkDiscoveryHostAliases,
		NetworkDiscoveryCloudDNS,
		NetworkDiscoveryServiceMesh,
		NetworkDiscoveryNATGatewayOnly:
		return true
	}
	return false
}

// String makes NetworkDiscoveryKind satisfy fmt.Stringer.
func (k NetworkDiscoveryKind) String() string { return string(k) }

// AllNetworkDiscoveryKinds is the closed set of valid kinds, useful
// for error-message enumeration and test sweeps.
var AllNetworkDiscoveryKinds = []NetworkDiscoveryKind{
	NetworkDiscoveryHostAliases,
	NetworkDiscoveryCloudDNS,
	NetworkDiscoveryServiceMesh,
	NetworkDiscoveryNATGatewayOnly,
}
