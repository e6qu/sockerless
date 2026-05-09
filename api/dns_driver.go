// DNS driver mechanism categories.
//
// Sibling to NetworkDiscoveryKind. The discovery driver answers
// "which container holds which name" (registration); this dimension
// answers "how does the workload's resolver find them at runtime"
// — primarily by configuring the search domain on /etc/resolv.conf
// and naming the cloud-product-specific resolver mechanism.

package api

// DNSMechanism names a cloud-side DNS resolution mechanism.
type DNSMechanism string

const (
	// DNSMechanismCloudMap uses AWS Cloud Map private namespaces.
	// Workloads on awsvpc-mode tasks reach peers via the VPC's
	// .2 resolver which forwards to the Route 53 PrivateHostedZone
	// backing the Cloud Map namespace.
	DNSMechanismCloudMap DNSMechanism = "cloud-map"

	// DNSMechanismCloudDNSZone uses GCP Cloud DNS managed zones.
	// Cloud Run / Cloud Functions resolve via Cloud DNS reachable
	// over the VPC connector.
	DNSMechanismCloudDNSZone DNSMechanism = "cloud-dns-zone"

	// DNSMechanismServiceDiscovery is a generic service-discovery
	// primitive — used when the cloud's mechanism doesn't match
	// the cloud-map / cloud-dns-zone / private-dns-zone shapes.
	DNSMechanismServiceDiscovery DNSMechanism = "service-discovery"

	// DNSMechanismPrivateDNSZone uses Azure Private DNS Zones.
	// Container Apps in the same managed environment resolve via
	// the environment's built-in resolver; cross-environment requires
	// explicit Private DNS Zone setup.
	DNSMechanismPrivateDNSZone DNSMechanism = "private-dns-zone"

	// DNSMechanismNone disables suffix injection. Used when peers are
	// reachable via /etc/hosts (single multi-container revision) or
	// when no inter-container discovery exists (Lambda, AZF).
	DNSMechanismNone DNSMechanism = "none"
)

// IsValid reports whether m is one of the documented mechanisms.
func (m DNSMechanism) IsValid() bool {
	switch m {
	case DNSMechanismCloudMap,
		DNSMechanismCloudDNSZone,
		DNSMechanismServiceDiscovery,
		DNSMechanismPrivateDNSZone,
		DNSMechanismNone:
		return true
	}
	return false
}

// String makes DNSMechanism satisfy fmt.Stringer.
func (m DNSMechanism) String() string { return string(m) }

// AllDNSMechanisms is the closed set of valid mechanisms.
var AllDNSMechanisms = []DNSMechanism{
	DNSMechanismCloudMap,
	DNSMechanismCloudDNSZone,
	DNSMechanismServiceDiscovery,
	DNSMechanismPrivateDNSZone,
	DNSMechanismNone,
}
