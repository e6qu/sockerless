// Cloud-DNS-zone DNS driver for the Cloud Run backend. Reads the
// resolved network state's DNSName (the zone's `dns-name` field) and
// returns it as the workload's DNS search domain.

package cloudrun

import (
	"context"

	"github.com/sockerless/api"
)

type cloudDNSZoneDNS struct {
	s *Server
}

func newCloudDNSZoneDNS(s *Server) *cloudDNSZoneDNS {
	return &cloudDNSZoneDNS{s: s}
}

func (d *cloudDNSZoneDNS) SearchDomain(ctx context.Context, networkID string) (string, error) {
	state, ok := d.s.resolveNetworkState(ctx, networkID)
	if !ok {
		return "", nil
	}
	return state.DNSName, nil
}

func (d *cloudDNSZoneDNS) Mechanism() api.DNSMechanism { return api.DNSMechanismCloudDNSZone }
