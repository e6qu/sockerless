// private-dns-zone DNS driver shared by every Azure-product backend.
// Returns the Azure Private DNS zone name (e.g. `aca-net.privatelink`)
// the workload's resolver should append to short-name lookups within
// the network. ACA reads this from its per-network state; AZF will
// once its DNS path is wired.
//
// Per-backend wiring: backend startup constructs with a closure that
// reads the per-backend NetworkState and returns the zone name.

package azurecommon

import (
	"context"

	"github.com/sockerless/api"
)

// PrivateDNSZoneDNS implements core.DNSDriver for the private-dns-zone
// mechanism. The per-backend state lookup is supplied as a callback
// captured at construction.
type PrivateDNSZoneDNS struct {
	// LookupZoneName returns the Private DNS zone name for the given
	// network ID. Empty string + nil error means "no zone configured
	// for this network" (NoOp behaviour for that network).
	LookupZoneName func(ctx context.Context, networkID string) (string, error)
}

func (d *PrivateDNSZoneDNS) Mechanism() api.DNSMechanism { return api.DNSMechanismPrivateDNSZone }

func (d *PrivateDNSZoneDNS) SearchDomain(ctx context.Context, networkID string) (string, error) {
	if d.LookupZoneName == nil {
		return "", nil
	}
	return d.LookupZoneName(ctx, networkID)
}
