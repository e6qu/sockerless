package aca

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
)

// Ensure resolve methods are available for DNS lookup integration.
var _ = (*Server).cloudServiceResolve

// cloudServiceRegister creates a Private DNS A-record for the
// container's hostname inside the network's zone. Uses the real Azure
// Private DNS SDK. The zone is created per-network in
// `cloudNetworkCreate`; the record maps hostname -> container IP.
// ACA Job executions don't have addressable per-execution IPs
// reachable from other Jobs the way Fargate ENIs are. The caller passes
// `ep.IPAddress` which is seeded as the empty-string placeholder ""
// (the gate below still accepts the legacy "0.0.0.0" for backward
// compat). Skip registration in that case rather than write a useless
// A-record. Architectural follow-up: ACA Apps with ingress carry stable
// URLs and could be CNAME-discovered via the driver framework's
// RegistryDriver dimension.
func (s *Server) cloudServiceRegister(containerID, hostname, ip, networkID string) error {
	if ip == "" || ip == "0.0.0.0" {
		s.Logger.Info().Str("container", containerID).Str("hostname", hostname).Str("network", networkID).
			Msg("skipping Private DNS register: ACA Jobs have no per-execution IP; enable UseApp for CNAME-based discovery")
		return nil
	}
	// cloud-fallback lookup for DNS zone state.
	state, ok := s.resolveNetworkState(s.ctx(), networkID)
	if !ok || state.DNSZoneName == "" {
		s.Logger.Debug().
			Str("container", containerID).
			Str("network", networkID).
			Msg("no Private DNS zone for network, skipping service registration")
		return nil
	}

	rs := armprivatedns.RecordSet{
		Properties: &armprivatedns.RecordSetProperties{
			TTL: to.Ptr(int64(60)),
			ARecords: []*armprivatedns.ARecord{
				{IPv4Address: to.Ptr(ip)},
			},
		},
	}

	_, err := s.azure.PrivateDNSRecords.CreateOrUpdate(
		s.ctx(),
		s.config.ResourceGroup,
		state.DNSZoneName,
		armprivatedns.RecordTypeA,
		hostname,
		rs,
		nil,
	)
	if err != nil {
		s.Logger.Error().Err(err).
			Str("hostname", hostname).
			Str("ip", ip).
			Str("zone", state.DNSZoneName).
			Msg("failed to create Private DNS A record")
		return fmt.Errorf("DNS register failed for %s -> %s: %w", hostname, ip, err)
	}

	s.Logger.Info().
		Str("hostname", hostname).
		Str("ip", ip).
		Str("zone", state.DNSZoneName).
		Str("container", containerID[:12]).
		Msg("registered DNS A record for service discovery")
	return nil
}

// cloudServiceDeregister removes the A record for the container's
// hostname from the network's Private DNS zone.
func (s *Server) cloudServiceDeregister(containerID, hostname, networkID string) error {
	state, ok := s.NetworkState.Get(networkID)
	if !ok || state.DNSZoneName == "" {
		return nil
	}

	_, err := s.azure.PrivateDNSRecords.Delete(
		s.ctx(),
		s.config.ResourceGroup,
		state.DNSZoneName,
		armprivatedns.RecordTypeA,
		hostname,
		nil,
	)
	if err != nil && !isNotFound(err) {
		s.Logger.Warn().Err(err).
			Str("hostname", hostname).
			Str("zone", state.DNSZoneName).
			Msg("failed to delete Private DNS A record")
		return err
	}
	s.Logger.Debug().
		Str("hostname", hostname).
		Str("zone", state.DNSZoneName).
		Str("container", containerID[:12]).
		Msg("deregistered DNS A record")
	return nil
}

// cloudServiceResolve looks up the IPs for a service name in the
// network's Private DNS zone. Returns the record's A-record IPv4
// addresses.
func (s *Server) cloudServiceResolve(serviceName, networkID string) ([]string, error) {
	state, ok := s.NetworkState.Get(networkID)
	if !ok || state.DNSZoneName == "" {
		return nil, fmt.Errorf("network %s has no Private DNS zone", networkID)
	}

	resp, err := s.azure.PrivateDNSRecords.Get(
		s.ctx(),
		s.config.ResourceGroup,
		state.DNSZoneName,
		armprivatedns.RecordTypeA,
		serviceName,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to discover %s in zone %s: %w", serviceName, state.DNSZoneName, err)
	}
	if resp.Properties == nil {
		return nil, nil
	}
	ips := make([]string, 0, len(resp.Properties.ARecords))
	for _, a := range resp.Properties.ARecords {
		if a != nil && a.IPv4Address != nil {
			ips = append(ips, *a.IPv4Address)
		}
	}
	return ips, nil
}

// cloudServiceRegisterCNAME creates a Private DNS CNAME record mapping
// the container's hostname to the ContainerApp's LatestRevisionFqdn.
// parallel to cloudrun's cloudServiceRegisterCNAME: Apps have
// stable internal FQDNs (reachable from other apps in the same
// managed environment), so peers resolve by hostname → CNAME → FQDN.
func (s *Server) cloudServiceRegisterCNAME(ctx context.Context, containerID, hostname, appName, networkID string) error {
	if s.azure == nil || s.azure.ContainerApps == nil || appName == "" {
		return nil
	}
	appResp, err := s.azure.ContainerApps.Get(ctx, s.config.ResourceGroup, appName, nil)
	if err != nil {
		return fmt.Errorf("get containerapp for DNS target: %w", err)
	}
	target := ""
	if appResp.Properties != nil && appResp.Properties.LatestRevisionFqdn != nil {
		target = *appResp.Properties.LatestRevisionFqdn
	}
	if target == "" {
		s.Logger.Info().Str("container", containerID).Str("hostname", hostname).
			Msg("ContainerApp.LatestRevisionFqdn empty (not ready?); skipping Private DNS CNAME")
		return nil
	}

	state, ok := s.resolveNetworkState(ctx, networkID)
	if !ok || state.DNSZoneName == "" {
		return nil
	}

	rs := armprivatedns.RecordSet{
		Properties: &armprivatedns.RecordSetProperties{
			TTL:         to.Ptr(int64(60)),
			CnameRecord: &armprivatedns.CnameRecord{Cname: to.Ptr(target)},
		},
	}

	_, err = s.azure.PrivateDNSRecords.CreateOrUpdate(
		ctx,
		s.config.ResourceGroup,
		state.DNSZoneName,
		armprivatedns.RecordTypeCNAME,
		hostname,
		rs,
		nil,
	)
	if err != nil {
		s.Logger.Error().Err(err).
			Str("hostname", hostname).
			Str("target", target).
			Str("zone", state.DNSZoneName).
			Msg("failed to create Private DNS CNAME record")
		return fmt.Errorf("DNS CNAME register failed for %s → %s: %w", hostname, target, err)
	}

	s.Logger.Info().
		Str("hostname", hostname).
		Str("target", target).
		Str("zone", state.DNSZoneName).
		Str("container", containerID[:12]).
		Msg("registered DNS CNAME record for service discovery")
	return nil
}

// cloudServiceDeregisterCNAME removes the CNAME the UseApp path writes.
// Separate from cloudServiceDeregister because Private DNS wants the
// exact record type on delete.
func (s *Server) cloudServiceDeregisterCNAME(ctx context.Context, containerID, hostname, networkID string) error {
	state, ok := s.NetworkState.Get(networkID)
	if !ok || state.DNSZoneName == "" {
		return nil
	}
	_, err := s.azure.PrivateDNSRecords.Delete(
		ctx,
		s.config.ResourceGroup,
		state.DNSZoneName,
		armprivatedns.RecordTypeCNAME,
		hostname,
		nil,
	)
	if err != nil && !isNotFound(err) {
		s.Logger.Warn().Err(err).
			Str("hostname", hostname).
			Str("zone", state.DNSZoneName).
			Str("container", containerID[:12]).
			Msg("failed to delete Private DNS CNAME record")
		return err
	}
	return nil
}

// isNotFound reports whether the error is a 404 from the Private DNS
// API (so deregister of a non-existent record is idempotent).
func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "ResourceNotFound")
}
