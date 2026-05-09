// Private-DNS-zone DNS driver for the ACA backend. Reads the resolved
// network state's DNSZoneName (the Azure Private DNS zone backing the
// network) and returns it as the workload's DNS search domain.

package aca

import (
	"context"

	"github.com/sockerless/api"
)

type privateDNSZoneDNS struct {
	s *Server
}

func newPrivateDNSZoneDNS(s *Server) *privateDNSZoneDNS {
	return &privateDNSZoneDNS{s: s}
}

func (d *privateDNSZoneDNS) SearchDomain(ctx context.Context, networkID string) (string, error) {
	state, ok := d.s.resolveNetworkState(ctx, networkID)
	if !ok {
		return "", nil
	}
	return state.DNSZoneName, nil
}

func (d *privateDNSZoneDNS) Mechanism() api.DNSMechanism { return api.DNSMechanismPrivateDNSZone }
