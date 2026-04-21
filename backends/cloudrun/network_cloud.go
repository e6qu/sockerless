package cloudrun

import (
	"fmt"
	"strings"

	"google.golang.org/api/dns/v1"
	"google.golang.org/api/googleapi"
)

// cloudNetworkCreate creates a Cloud DNS private managed zone for the given
// Docker network. This enables containers in the same network to resolve each
// other by hostname. Firewall rules would require a Compute client, which the
// CloudRun backend does not have — DNS-based name resolution is the primary
// mechanism for intra-network service discovery.
func (s *Server) cloudNetworkCreate(name, networkID string) error {
	zoneName := sanitizeZoneName(name)
	dnsName := zoneName + ".internal."

	zone := &dns.ManagedZone{
		Name:        zoneName,
		DnsName:     dnsName,
		Description: fmt.Sprintf("Sockerless Docker network: %s", name),
		Visibility:  "private",
	}

	created, err := s.gcp.DNS.ManagedZones.Create(s.config.Project, zone).Context(s.ctx()).Do()
	if err != nil {
		// Reuse existing zone on conflict— idempotent retry).
		if gerr, ok := err.(*googleapi.Error); ok && gerr.Code == 409 {
			existing, getErr := s.gcp.DNS.ManagedZones.Get(s.config.Project, zoneName).Context(s.ctx()).Do()
			if getErr != nil {
				return fmt.Errorf("cloud DNS zone %s exists but Get returned: %w", zoneName, getErr)
			}
			s.Logger.Info().Str("zone", existing.Name).Str("network", name).Msg("reusing existing Cloud DNS managed zone")
			s.NetworkState.Put(networkID, NetworkState{
				ManagedZoneName: existing.Name,
				DNSName:         existing.DnsName,
			})
			return nil
		}
		s.Logger.Error().Err(err).
			Str("zone", zoneName).
			Str("network", name).
			Msg("failed to create Cloud DNS managed zone")
		return fmt.Errorf("cloud DNS zone create failed for network %q: %w", name, err)
	}

	s.Logger.Info().
		Str("zone", created.Name).
		Str("dnsName", created.DnsName).
		Str("network", name).
		Msg("created Cloud DNS managed zone for network")

	s.NetworkState.Put(networkID, NetworkState{
		ManagedZoneName: created.Name,
		DNSName:         created.DnsName,
	})

	return nil
}

// cloudNetworkDelete removes the Cloud DNS managed zone for a Docker network.
// All resource record sets (except SOA and NS) must be deleted before the zone
// can be removed. This performs best-effort cleanup of DNS records first.
func (s *Server) cloudNetworkDelete(networkID string) error {
	state, ok := s.NetworkState.Get(networkID)
	if !ok || state.ManagedZoneName == "" {
		return nil // No cloud state to clean up
	}

	// Delete all non-default record sets (SOA and NS are auto-managed)
	if err := s.deleteAllRecordSets(state.ManagedZoneName); err != nil {
		s.Logger.Warn().Err(err).
			Str("zone", state.ManagedZoneName).
			Msg("failed to clean up DNS records before zone deletion")
	}

	err := s.gcp.DNS.ManagedZones.Delete(s.config.Project, state.ManagedZoneName).Context(s.ctx()).Do()
	if err != nil {
		s.Logger.Error().Err(err).
			Str("zone", state.ManagedZoneName).
			Msg("failed to delete Cloud DNS managed zone")
		return fmt.Errorf("cloud DNS zone delete failed for network %q: %w", networkID, err)
	}

	s.Logger.Info().
		Str("zone", state.ManagedZoneName).
		Msg("deleted Cloud DNS managed zone")

	s.NetworkState.Delete(networkID)
	return nil
}

// deleteAllRecordSets removes all A record sets from a managed zone.
// SOA and NS records are managed by Cloud DNS and cannot be deleted.
func (s *Server) deleteAllRecordSets(zoneName string) error {
	resp, err := s.gcp.DNS.ResourceRecordSets.List(s.config.Project, zoneName).Context(s.ctx()).Do()
	if err != nil {
		return fmt.Errorf("list record sets: %w", err)
	}

	for _, rrs := range resp.Rrsets {
		if rrs.Type == "SOA" || rrs.Type == "NS" {
			continue
		}
		_, err := s.gcp.DNS.ResourceRecordSets.Delete(
			s.config.Project, zoneName, rrs.Name, rrs.Type,
		).Context(s.ctx()).Do()
		if err != nil {
			s.Logger.Warn().Err(err).
				Str("name", rrs.Name).
				Str("type", rrs.Type).
				Msg("failed to delete DNS record")
		}
	}

	return nil
}

// sanitizeZoneName converts a Docker network name to a valid Cloud DNS zone
// name. Zone names must match [a-z]([-a-z0-9]*[a-z0-9])? and be at most 63
// characters.
func sanitizeZoneName(name string) string {
	name = strings.ToLower(name)
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, name)

	// Trim leading/trailing hyphens
	name = strings.Trim(name, "-")

	if len(name) > 63 {
		name = name[:63]
		name = strings.TrimRight(name, "-")
	}

	if name == "" {
		name = "net"
	}

	return name
}
