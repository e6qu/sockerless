// cloud-dns-zone DNS driver shared by every GCP-product backend.
// Returns the Cloud DNS zone's `dns-name` (e.g. `tf-net.local.`) the
// workload's resolver should append to short-name lookups within the
// network. cloudrun reads this from its per-network state.
//
// Per-backend wiring: backend startup constructs with a closure that
// reads the per-backend NetworkState and returns the zone DNS name.

package gcpcommon

import (
	"context"

	"github.com/sockerless/api"
)

// CloudDNSZoneDNS implements core.DNSDriver for the cloud-dns-zone
// mechanism. The per-backend state lookup is supplied as a callback
// captured at construction.
type CloudDNSZoneDNS struct {
	// LookupZoneDNSName returns the Cloud DNS zone's `dns-name` for
	// the given network ID. Empty string + nil error means "no zone
	// configured for this network" (NoOp behaviour for that network).
	LookupZoneDNSName func(ctx context.Context, networkID string) (string, error)
}

func (d *CloudDNSZoneDNS) Mechanism() api.DNSMechanism { return api.DNSMechanismCloudDNSZone }

func (d *CloudDNSZoneDNS) SearchDomain(ctx context.Context, networkID string) (string, error) {
	if d.LookupZoneDNSName == nil {
		return "", nil
	}
	return d.LookupZoneDNSName(ctx, networkID)
}
