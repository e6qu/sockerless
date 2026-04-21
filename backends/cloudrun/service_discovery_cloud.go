package cloudrun

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"google.golang.org/api/dns/v1"
)

// Ensure resolve methods are available for DNS lookup integration.
var _ = (*Server).cloudServiceResolve

// cloudServiceRegister creates an A record in the network's Cloud DNS managed
// zone, mapping hostname to the container's IP address. This enables
// DNS-based service discovery between containers in the same Docker network.
// Cloud Run Job executions don't have addressable per-execution IPs
// reachable from other Jobs in the same VPC the way Fargate ENIs are. The
// caller passes `ep.IPAddress` which is seeded as the placeholder "0.0.0.0".
// Skip registration in that case rather than write a useless A-record.
// Under UseService, callers use cloudServiceRegisterCNAME instead — Services
// have stable per-revision URLs that work with VPC connector egress.
func (s *Server) cloudServiceRegister(containerID, hostname, ip, networkID string) error {
	if ip == "" || ip == "0.0.0.0" {
		s.Logger.Info().Str("container", containerID).Str("hostname", hostname).Str("network", networkID).
			Msg("skipping Cloud DNS register: Cloud Run Jobs have no per-execution IP; enable UseService for CNAME-based discovery")
		return nil
	}
	// cloud-fallback lookup for zone state.
	state, ok := s.resolveNetworkState(s.ctx(), networkID)
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

// cloudServiceRegisterCNAME creates a CNAME record in the network's Cloud
// DNS managed zone, pointing the container's hostname at its Cloud Run
// Service URL. Used by the UseService path because Services have stable
// URLs (reachable over the VPC connector) instead of per-instance IPs.
// The target is derived from the Service.Uri field — we strip the
// scheme and any trailing slash so the record stores the DNS-resolvable
// hostname. The record is terminated with a "." so Cloud DNS treats it
// as a fully-qualified name.
func (s *Server) cloudServiceRegisterCNAME(ctx context.Context, containerID, hostname, serviceName, networkID string) error {
	if s.gcp == nil || s.gcp.Services == nil || serviceName == "" {
		return nil
	}
	svc, err := s.gcp.Services.GetService(ctx, &runpb.GetServiceRequest{Name: serviceName})
	if err != nil {
		return fmt.Errorf("get service for DNS target: %w", err)
	}
	target := serviceURIHost(svc.Uri)
	if target == "" {
		s.Logger.Info().Str("container", containerID).Str("hostname", hostname).
			Msg("Service.Uri empty (not ready yet?); skipping Cloud DNS CNAME")
		return nil
	}

	state, ok := s.resolveNetworkState(ctx, networkID)
	if !ok || state.ManagedZoneName == "" {
		return nil
	}

	fqdn := hostname + "." + state.DNSName
	rrset := &dns.ResourceRecordSet{
		Name:    fqdn,
		Type:    "CNAME",
		Ttl:     60,
		Rrdatas: []string{target + "."},
	}

	created, err := s.gcp.DNS.ResourceRecordSets.Create(
		s.config.Project, state.ManagedZoneName, rrset,
	).Context(ctx).Do()
	if err != nil {
		s.Logger.Error().Err(err).
			Str("hostname", hostname).
			Str("fqdn", fqdn).
			Str("target", target).
			Str("zone", state.ManagedZoneName).
			Msg("failed to create DNS CNAME record")
		return fmt.Errorf("DNS CNAME register failed for %s → %s: %w", hostname, target, err)
	}

	s.Logger.Info().
		Str("fqdn", created.Name).
		Str("target", target).
		Str("zone", state.ManagedZoneName).
		Str("container", containerID).
		Msg("registered DNS CNAME record for service discovery")
	return nil
}

// cloudServiceDeregisterCNAME removes the CNAME the UseService path writes.
// Separate from cloudServiceDeregister because Cloud DNS wants the exact
// record type when deleting.
func (s *Server) cloudServiceDeregisterCNAME(ctx context.Context, containerID, hostname, networkID string) error {
	state, ok := s.NetworkState.Get(networkID)
	if !ok || state.ManagedZoneName == "" {
		return nil
	}
	fqdn := hostname + "." + state.DNSName
	_, err := s.gcp.DNS.ResourceRecordSets.Delete(
		s.config.Project, state.ManagedZoneName, fqdn, "CNAME",
	).Context(ctx).Do()
	if err != nil {
		s.Logger.Warn().Err(err).
			Str("hostname", hostname).
			Str("fqdn", fqdn).
			Str("zone", state.ManagedZoneName).
			Str("container", containerID).
			Msg("failed to delete DNS CNAME record")
		return fmt.Errorf("DNS CNAME deregister failed for %s: %w", hostname, err)
	}
	return nil
}

// serviceURIHost extracts the hostname from a Cloud Run Service.Uri
// (e.g. "https://sockerless-svc-abc-xxx.a.run.app" → "sockerless-svc-abc-xxx.a.run.app").
// Returns "" if uri is empty or unparseable.
func serviceURIHost(uri string) string {
	if uri == "" {
		return ""
	}
	if !strings.Contains(uri, "://") {
		return strings.TrimSuffix(uri, "/")
	}
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return u.Host
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
