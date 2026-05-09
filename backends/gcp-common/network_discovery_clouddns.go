// cloud-dns network-discovery driver shared by every GCP-product
// backend. Pattern B — backend-specific NetworkState lookups arrive
// via callbacks captured at construction; the driver itself owns the
// GCP DNS + Cloud Run Services SDK calls.
//
// Per-backend wiring: backend startup constructs CloudDNSDiscovery
// with its DNS service, RunServices client, project, logger, and two
// network-state callbacks (Get for local-only, Lookup for cloud-fallback).

package gcpcommon

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	runv2 "cloud.google.com/go/run/apiv2"
	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/rs/zerolog"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	"google.golang.org/api/dns/v1"
)

// CloudDNSNetworkState is the per-network state the discovery driver
// needs to operate on a Cloud DNS managed zone. Per-backend
// NetworkState objects expose this via the callbacks below.
type CloudDNSNetworkState struct {
	ManagedZoneName string // Cloud DNS managed zone name
	DNSName         string // DNS zone name (e.g., "network-name.internal.")
}

// CloudDNSDiscoveryConfig is the constructor arg for the discovery
// driver. All fields are required except RunServices (only used by the
// CNAME path; backends that don't materialise per-container Cloud Run
// Services can leave it nil).
type CloudDNSDiscoveryConfig struct {
	DNS         *dns.Service
	RunServices *runv2.ServicesClient
	Project     string
	Logger      zerolog.Logger
	// LookupNetwork resolves NetworkState through the cloud-fallback
	// path (caller may issue a `dns.ManagedZones.Get` to populate it).
	// Returns false when no zone exists for the network — register/
	// resolve become no-ops.
	LookupNetwork func(ctx context.Context, networkID string) (CloudDNSNetworkState, bool)
	// GetNetwork returns the locally-cached NetworkState only. Used by
	// deregister + resolve where the caller assumes the zone was
	// already resolved at register time.
	GetNetwork func(networkID string) (CloudDNSNetworkState, bool)
}

// CloudDNSDiscovery satisfies core.NetworkDiscoveryDriver via the
// Cloud DNS managed-zone mechanism.
type CloudDNSDiscovery struct {
	cfg CloudDNSDiscoveryConfig
}

// NewCloudDNSDiscovery wraps the per-backend SDK clients + state
// callbacks in the discovery-driver shape.
func NewCloudDNSDiscovery(cfg CloudDNSDiscoveryConfig) *CloudDNSDiscovery {
	return &CloudDNSDiscovery{cfg: cfg}
}

// RegisterContainer dispatches to either the A-record path or the
// CNAME path based on endpoint.Metadata["kind"]:
//
//   - "a-record" (default): registerA with endpoint.IPAddress.
//   - "cname":              registerCNAME with the underlying
//     Cloud Run service URI from endpoint.Metadata["service-name"].
func (d *CloudDNSDiscovery) RegisterContainer(ctx context.Context, networkID, name, containerID string, endpoint *core.CloudEndpoint) error {
	if endpoint == nil {
		return nil
	}
	if endpoint.Metadata != nil && endpoint.Metadata["kind"] == "cname" {
		serviceName := endpoint.Metadata["service-name"]
		return d.registerCNAME(ctx, containerID, name, serviceName, networkID)
	}
	return d.registerA(ctx, containerID, name, endpoint.IPAddress, networkID)
}

// DeregisterContainer mirrors RegisterContainer's dispatch — when the
// caller doesn't have the original endpoint, both paths are tried in
// sequence (CNAME first, then A-record). Both are idempotent.
func (d *CloudDNSDiscovery) DeregisterContainer(ctx context.Context, networkID, name, containerID string) error {
	if err := d.deregisterCNAME(ctx, containerID, name, networkID); err != nil {
		_ = d.deregisterA(containerID, name, networkID)
		return err
	}
	return d.deregisterA(containerID, name, networkID)
}

// DeregisterContainerCNAME is the explicit-kind variant for callers
// that know the original register kind was CNAME.
func (d *CloudDNSDiscovery) DeregisterContainerCNAME(ctx context.Context, networkID, name string) error {
	return d.deregisterCNAME(ctx, "", name, networkID)
}

// DeregisterContainerARecord is the explicit-kind A-record variant.
func (d *CloudDNSDiscovery) DeregisterContainerARecord(networkID, name string) error {
	return d.deregisterA("", name, networkID)
}

// ResolveName looks up A records for `name` in the network's Cloud
// DNS managed zone and returns the first matching IP via CloudEndpoint.
func (d *CloudDNSDiscovery) ResolveName(ctx context.Context, networkID, name string) (*core.CloudEndpoint, error) {
	ips, err := d.resolveA(ctx, name, networkID)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, nil
	}
	return &core.CloudEndpoint{IPAddress: ips[0]}, nil
}

func (d *CloudDNSDiscovery) Kind() api.NetworkDiscoveryKind {
	return api.NetworkDiscoveryCloudDNS
}

// registerA creates a Cloud DNS A record mapping hostname → ip in the
// network's managed zone. Caller passes the empty IP placeholder when
// the workload is fronted by something without a per-instance address
// (e.g. Cloud Run Jobs); the gate skips registration in that case.
// Legacy "0.0.0.0" placeholder accepted for back-compat.
func (d *CloudDNSDiscovery) registerA(ctx context.Context, containerID, hostname, ip, networkID string) error {
	if ip == "" || ip == "0.0.0.0" {
		d.cfg.Logger.Info().Str("container", containerID).Str("hostname", hostname).Str("network", networkID).
			Msg("skipping Cloud DNS register: workload has no per-instance IP; enable CNAME-based discovery instead")
		return nil
	}
	state, ok := d.cfg.LookupNetwork(ctx, networkID)
	if !ok || state.ManagedZoneName == "" {
		d.cfg.Logger.Debug().
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

	created, err := d.cfg.DNS.ResourceRecordSets.Create(
		d.cfg.Project, state.ManagedZoneName, rrset,
	).Context(ctx).Do()
	if err != nil {
		d.cfg.Logger.Error().Err(err).
			Str("hostname", hostname).
			Str("fqdn", fqdn).
			Str("ip", ip).
			Str("zone", state.ManagedZoneName).
			Msg("failed to create DNS A record")
		return fmt.Errorf("DNS register failed for %s → %s: %w", hostname, ip, err)
	}

	d.cfg.Logger.Info().
		Str("fqdn", created.Name).
		Str("ip", ip).
		Str("zone", state.ManagedZoneName).
		Str("container", containerID).
		Msg("registered DNS A record for service discovery")
	return nil
}

func (d *CloudDNSDiscovery) deregisterA(containerID, hostname, networkID string) error {
	state, ok := d.cfg.GetNetwork(networkID)
	if !ok || state.ManagedZoneName == "" {
		return nil
	}
	fqdn := hostname + "." + state.DNSName
	_, err := d.cfg.DNS.ResourceRecordSets.Delete(
		d.cfg.Project, state.ManagedZoneName, fqdn, "A",
	).Do()
	if err != nil {
		d.cfg.Logger.Warn().Err(err).
			Str("hostname", hostname).
			Str("fqdn", fqdn).
			Str("zone", state.ManagedZoneName).
			Msg("failed to delete DNS A record")
		return fmt.Errorf("DNS deregister failed for %s: %w", hostname, err)
	}
	d.cfg.Logger.Info().
		Str("fqdn", fqdn).
		Str("zone", state.ManagedZoneName).
		Str("container", containerID).
		Msg("deregistered DNS A record")
	return nil
}

// registerCNAME creates a Cloud DNS CNAME record pointing the
// container's hostname at its Cloud Run Service's URL host. Caller
// provides the full Cloud Run service resource name (e.g.
// `projects/p/locations/l/services/sockerless-svc-XXX`); the driver
// reads Service.Uri to extract the DNS-resolvable target host.
func (d *CloudDNSDiscovery) registerCNAME(ctx context.Context, containerID, hostname, serviceName, networkID string) error {
	if d.cfg.RunServices == nil || serviceName == "" {
		return nil
	}
	svc, err := d.cfg.RunServices.GetService(ctx, &runpb.GetServiceRequest{Name: serviceName})
	if err != nil {
		return fmt.Errorf("get service for DNS target: %w", err)
	}
	target := serviceURIHost(svc.Uri)
	if target == "" {
		d.cfg.Logger.Info().Str("container", containerID).Str("hostname", hostname).
			Msg("Service.Uri empty (not ready yet?); skipping Cloud DNS CNAME")
		return nil
	}

	state, ok := d.cfg.LookupNetwork(ctx, networkID)
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

	created, err := d.cfg.DNS.ResourceRecordSets.Create(
		d.cfg.Project, state.ManagedZoneName, rrset,
	).Context(ctx).Do()
	if err != nil {
		d.cfg.Logger.Error().Err(err).
			Str("hostname", hostname).
			Str("fqdn", fqdn).
			Str("target", target).
			Str("zone", state.ManagedZoneName).
			Msg("failed to create DNS CNAME record")
		return fmt.Errorf("DNS CNAME register failed for %s → %s: %w", hostname, target, err)
	}

	d.cfg.Logger.Info().
		Str("fqdn", created.Name).
		Str("target", target).
		Str("zone", state.ManagedZoneName).
		Str("container", containerID).
		Msg("registered DNS CNAME record for service discovery")
	return nil
}

func (d *CloudDNSDiscovery) deregisterCNAME(ctx context.Context, containerID, hostname, networkID string) error {
	state, ok := d.cfg.GetNetwork(networkID)
	if !ok || state.ManagedZoneName == "" {
		return nil
	}
	fqdn := hostname + "." + state.DNSName
	_, err := d.cfg.DNS.ResourceRecordSets.Delete(
		d.cfg.Project, state.ManagedZoneName, fqdn, "CNAME",
	).Context(ctx).Do()
	if err != nil {
		d.cfg.Logger.Warn().Err(err).
			Str("hostname", hostname).
			Str("fqdn", fqdn).
			Str("zone", state.ManagedZoneName).
			Str("container", containerID).
			Msg("failed to delete DNS CNAME record")
		return fmt.Errorf("DNS CNAME deregister failed for %s: %w", hostname, err)
	}
	return nil
}

func (d *CloudDNSDiscovery) resolveA(ctx context.Context, serviceName, networkID string) ([]string, error) {
	state, ok := d.cfg.GetNetwork(networkID)
	if !ok || state.ManagedZoneName == "" {
		return nil, fmt.Errorf("no Cloud DNS zone for network %q", networkID)
	}

	fqdn := serviceName + "." + state.DNSName
	resp, err := d.cfg.DNS.ResourceRecordSets.List(
		d.cfg.Project, state.ManagedZoneName,
	).Name(fqdn).Type("A").Context(ctx).Do()
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
