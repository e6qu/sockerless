package cloudrun

import (
	"fmt"

	"google.golang.org/api/dns/v1"
)

// Ensure resolve methods are available for DNS lookup integration.
var _ = (*Server).cloudServiceResolve

// cloudServiceRegister creates an A record in the network's Cloud DNS managed
// zone, mapping hostname to the container's IP address. This enables
// DNS-based service discovery between containers in the same Docker network.
func (s *Server) cloudServiceRegister(containerID, hostname, ip, networkID string) error {
	state, ok := s.NetworkState.Get(networkID)
	if !ok || state.ManagedZoneName == "" {
		s.Logger.Debug().
			Str("container", containerID).
			Str("network", networkID).
			Msg("no Cloud DNS zone for network, skipping service registration")
		return nil
	}

	fqdn := hostname + "." + state.DNSName

	rrset := &dns.ResourceRecordSet{
		Name:    fqdn,
		Type:    "A",
		Ttl:     60,
		Rrdatas: []string{ip},
	}

	created, err := s.gcp.DNS.ResourceRecordSets.Create(
		s.config.Project, state.ManagedZoneName, rrset,
	).Context(s.ctx()).Do()
	if err != nil {
		s.Logger.Error().Err(err).
			Str("hostname", hostname).
			Str("fqdn", fqdn).
			Str("ip", ip).
			Str("zone", state.ManagedZoneName).
			Msg("failed to create DNS A record")
		return fmt.Errorf("DNS register failed for %s → %s: %w", hostname, ip, err)
	}

	s.Logger.Info().
		Str("fqdn", created.Name).
		Str("ip", ip).
		Str("zone", state.ManagedZoneName).
		Str("container", containerID).
		Msg("registered DNS A record for service discovery")

	return nil
}

// cloudServiceDeregister removes the A record for a hostname from the
// network's Cloud DNS managed zone.
func (s *Server) cloudServiceDeregister(containerID, hostname, networkID string) error {
	state, ok := s.NetworkState.Get(networkID)
	if !ok || state.ManagedZoneName == "" {
		return nil
	}

	fqdn := hostname + "." + state.DNSName

	_, err := s.gcp.DNS.ResourceRecordSets.Delete(
		s.config.Project, state.ManagedZoneName, fqdn, "A",
	).Context(s.ctx()).Do()
	if err != nil {
		s.Logger.Warn().Err(err).
			Str("hostname", hostname).
			Str("fqdn", fqdn).
			Str("zone", state.ManagedZoneName).
			Msg("failed to delete DNS A record")
		return fmt.Errorf("DNS deregister failed for %s: %w", hostname, err)
	}

	s.Logger.Info().
		Str("fqdn", fqdn).
		Str("zone", state.ManagedZoneName).
		Str("container", containerID).
		Msg("deregistered DNS A record")

	return nil
}

// cloudServiceResolve looks up A records for a service name in the network's
// Cloud DNS managed zone and returns the associated IP addresses.
func (s *Server) cloudServiceResolve(serviceName, networkID string) ([]string, error) {
	state, ok := s.NetworkState.Get(networkID)
	if !ok || state.ManagedZoneName == "" {
		return nil, fmt.Errorf("no Cloud DNS zone for network %q", networkID)
	}

	fqdn := serviceName + "." + state.DNSName

	resp, err := s.gcp.DNS.ResourceRecordSets.List(
		s.config.Project, state.ManagedZoneName,
	).Name(fqdn).Type("A").Context(s.ctx()).Do()
	if err != nil {
		return nil, fmt.Errorf("DNS resolve failed for %s: %w", serviceName, err)
	}

	var ips []string
	for _, rrs := range resp.Rrsets {
		if rrs.Name == fqdn && rrs.Type == "A" {
			ips = append(ips, rrs.Rrdatas...)
		}
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no DNS records found for %s in network %s", serviceName, networkID)
	}

	return ips, nil
}
