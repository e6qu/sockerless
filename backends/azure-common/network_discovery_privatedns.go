// private-dns network-discovery driver shared by every Azure-product
// backend. Pattern B — backend-specific NetworkState lookups arrive
// via callbacks captured at construction; the driver itself owns the
// Azure Private DNS + Container Apps SDK calls.
//
// Per-backend wiring: backend startup constructs PrivateDNSDiscovery
// with the privatedns RecordSets client, optional ContainerApps client
// (for the CNAME path), resource group, logger, and two
// network-state callbacks (Get for local-only, Lookup for cloud-fallback).

package azurecommon

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
	"github.com/rs/zerolog"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// PrivateDNSNetworkState is the per-network state the discovery driver
// needs to operate against an Azure Private DNS zone.
type PrivateDNSNetworkState struct {
	DNSZoneName string // Azure Private DNS zone backing this network
}

// PrivateDNSDiscoveryConfig is the constructor arg. ContainerApps is
// optional — only used by the CNAME path (backends that don't deploy
// ContainerApps for per-container CNAMEs leave it nil).
type PrivateDNSDiscoveryConfig struct {
	PrivateDNSRecords *armprivatedns.RecordSetsClient
	ContainerApps     *armappcontainers.ContainerAppsClient
	ResourceGroup     string
	Logger            zerolog.Logger
	// LookupNetwork resolves NetworkState through the cloud-fallback
	// path. Returns false when no zone exists for the network — register
	// becomes a no-op.
	LookupNetwork func(ctx context.Context, networkID string) (PrivateDNSNetworkState, bool)
	// GetNetwork returns the locally-cached NetworkState only. Used by
	// deregister + resolve.
	GetNetwork func(networkID string) (PrivateDNSNetworkState, bool)
}

// PrivateDNSDiscovery satisfies core.NetworkDiscoveryDriver via the
// Azure Private DNS zone mechanism.
type PrivateDNSDiscovery struct {
	cfg PrivateDNSDiscoveryConfig
}

// NewPrivateDNSDiscovery wraps the per-backend SDK clients + state
// callbacks in the discovery-driver shape.
func NewPrivateDNSDiscovery(cfg PrivateDNSDiscoveryConfig) *PrivateDNSDiscovery {
	return &PrivateDNSDiscovery{cfg: cfg}
}

func (d *PrivateDNSDiscovery) RegisterContainer(ctx context.Context, networkID, name, containerID string, endpoint *core.CloudEndpoint) error {
	if endpoint == nil {
		return nil
	}
	if endpoint.Metadata != nil && endpoint.Metadata["kind"] == "cname" {
		appName := endpoint.Metadata["service-name"]
		return d.registerCNAME(ctx, containerID, name, appName, networkID)
	}
	return d.registerA(ctx, containerID, name, endpoint.IPAddress, networkID)
}

func (d *PrivateDNSDiscovery) DeregisterContainer(ctx context.Context, networkID, name, containerID string) error {
	if err := d.deregisterCNAME(ctx, containerID, name, networkID); err != nil {
		_ = d.deregisterA(ctx, containerID, name, networkID)
		return err
	}
	return d.deregisterA(ctx, containerID, name, networkID)
}

func (d *PrivateDNSDiscovery) DeregisterContainerCNAME(ctx context.Context, networkID, name string) error {
	return d.deregisterCNAME(ctx, "", name, networkID)
}

func (d *PrivateDNSDiscovery) DeregisterContainerARecord(ctx context.Context, networkID, name string) error {
	return d.deregisterA(ctx, "", name, networkID)
}

func (d *PrivateDNSDiscovery) ResolveName(ctx context.Context, networkID, name string) (*core.CloudEndpoint, error) {
	ips, err := d.resolveA(ctx, name, networkID)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, nil
	}
	return &core.CloudEndpoint{IPAddress: ips[0]}, nil
}

func (d *PrivateDNSDiscovery) Kind() api.NetworkDiscoveryKind {
	return api.NetworkDiscoveryCloudDNS
}

func (d *PrivateDNSDiscovery) registerA(ctx context.Context, containerID, hostname, ip, networkID string) error {
	if ip == "" || ip == "0.0.0.0" {
		d.cfg.Logger.Info().Str("container", containerID).Str("hostname", hostname).Str("network", networkID).
			Msg("skipping Private DNS register: workload has no per-instance IP; enable CNAME-based discovery instead")
		return nil
	}
	state, ok := d.cfg.LookupNetwork(ctx, networkID)
	if !ok || state.DNSZoneName == "" {
		d.cfg.Logger.Debug().
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

	_, err := d.cfg.PrivateDNSRecords.CreateOrUpdate(
		ctx,
		d.cfg.ResourceGroup,
		state.DNSZoneName,
		armprivatedns.RecordTypeA,
		hostname,
		rs,
		nil,
	)
	if err != nil {
		d.cfg.Logger.Error().Err(err).
			Str("hostname", hostname).
			Str("ip", ip).
			Str("zone", state.DNSZoneName).
			Msg("failed to create Private DNS A record")
		return fmt.Errorf("DNS register failed for %s -> %s: %w", hostname, ip, err)
	}

	cid := containerID
	if len(cid) > 12 {
		cid = cid[:12]
	}
	d.cfg.Logger.Info().
		Str("hostname", hostname).
		Str("ip", ip).
		Str("zone", state.DNSZoneName).
		Str("container", cid).
		Msg("registered DNS A record for service discovery")
	return nil
}

func (d *PrivateDNSDiscovery) deregisterA(ctx context.Context, containerID, hostname, networkID string) error {
	state, ok := d.cfg.GetNetwork(networkID)
	if !ok || state.DNSZoneName == "" {
		return nil
	}

	_, err := d.cfg.PrivateDNSRecords.Delete(
		ctx,
		d.cfg.ResourceGroup,
		state.DNSZoneName,
		armprivatedns.RecordTypeA,
		hostname,
		nil,
	)
	if err != nil && !privateDNSIsNotFound(err) {
		d.cfg.Logger.Warn().Err(err).
			Str("hostname", hostname).
			Str("zone", state.DNSZoneName).
			Msg("failed to delete Private DNS A record")
		return err
	}
	cid := containerID
	if len(cid) > 12 {
		cid = cid[:12]
	}
	d.cfg.Logger.Debug().
		Str("hostname", hostname).
		Str("zone", state.DNSZoneName).
		Str("container", cid).
		Msg("deregistered DNS A record")
	return nil
}

// registerCNAME points the container's hostname at the ContainerApp's
// LatestRevisionFqdn. Apps have stable internal FQDNs reachable from
// peers in the same managed environment.
func (d *PrivateDNSDiscovery) registerCNAME(ctx context.Context, containerID, hostname, appName, networkID string) error {
	if d.cfg.ContainerApps == nil || appName == "" {
		return nil
	}
	appResp, err := d.cfg.ContainerApps.Get(ctx, d.cfg.ResourceGroup, appName, nil)
	if err != nil {
		return fmt.Errorf("get containerapp for DNS target: %w", err)
	}
	target := ""
	if appResp.Properties != nil && appResp.Properties.LatestRevisionFqdn != nil {
		target = *appResp.Properties.LatestRevisionFqdn
	}
	if target == "" {
		d.cfg.Logger.Info().Str("container", containerID).Str("hostname", hostname).
			Msg("ContainerApp.LatestRevisionFqdn empty (not ready?); skipping Private DNS CNAME")
		return nil
	}

	state, ok := d.cfg.LookupNetwork(ctx, networkID)
	if !ok || state.DNSZoneName == "" {
		return nil
	}

	rs := armprivatedns.RecordSet{
		Properties: &armprivatedns.RecordSetProperties{
			TTL:         to.Ptr(int64(60)),
			CnameRecord: &armprivatedns.CnameRecord{Cname: to.Ptr(target)},
		},
	}

	_, err = d.cfg.PrivateDNSRecords.CreateOrUpdate(
		ctx,
		d.cfg.ResourceGroup,
		state.DNSZoneName,
		armprivatedns.RecordTypeCNAME,
		hostname,
		rs,
		nil,
	)
	if err != nil {
		d.cfg.Logger.Error().Err(err).
			Str("hostname", hostname).
			Str("target", target).
			Str("zone", state.DNSZoneName).
			Msg("failed to create Private DNS CNAME record")
		return fmt.Errorf("DNS CNAME register failed for %s → %s: %w", hostname, target, err)
	}

	cid := containerID
	if len(cid) > 12 {
		cid = cid[:12]
	}
	d.cfg.Logger.Info().
		Str("hostname", hostname).
		Str("target", target).
		Str("zone", state.DNSZoneName).
		Str("container", cid).
		Msg("registered DNS CNAME record for service discovery")
	return nil
}

func (d *PrivateDNSDiscovery) deregisterCNAME(ctx context.Context, containerID, hostname, networkID string) error {
	state, ok := d.cfg.GetNetwork(networkID)
	if !ok || state.DNSZoneName == "" {
		return nil
	}
	_, err := d.cfg.PrivateDNSRecords.Delete(
		ctx,
		d.cfg.ResourceGroup,
		state.DNSZoneName,
		armprivatedns.RecordTypeCNAME,
		hostname,
		nil,
	)
	if err != nil && !privateDNSIsNotFound(err) {
		cid := containerID
		if len(cid) > 12 {
			cid = cid[:12]
		}
		d.cfg.Logger.Warn().Err(err).
			Str("hostname", hostname).
			Str("zone", state.DNSZoneName).
			Str("container", cid).
			Msg("failed to delete Private DNS CNAME record")
		return err
	}
	return nil
}

func (d *PrivateDNSDiscovery) resolveA(ctx context.Context, serviceName, networkID string) ([]string, error) {
	state, ok := d.cfg.GetNetwork(networkID)
	if !ok || state.DNSZoneName == "" {
		return nil, fmt.Errorf("network %s has no Private DNS zone", networkID)
	}

	resp, err := d.cfg.PrivateDNSRecords.Get(
		ctx,
		d.cfg.ResourceGroup,
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

// privateDNSIsNotFound reports whether the error is a 404 from the
// Private DNS API so deregister of a non-existent record is idempotent.
func privateDNSIsNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "ResourceNotFound")
}
