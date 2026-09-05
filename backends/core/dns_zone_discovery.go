package core

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

// A network on Google Cloud or Microsoft Azure is a private DNS zone, and
// container discovery is records in that zone: an A record when the
// workload has a per-instance address, a CNAME at the fronting service's
// hostname when it does not. The record operations are the cloud's; the
// dispatch, the placeholder-address gate, the zone lookup, and the
// idempotent deregistration are the same on both, and live here.

// DNSZoneRecords is what a cloud supplies to DNSZoneDiscovery. Z is the
// cloud's per-network zone state (a Google Cloud managed zone name plus
// DNS name; an Azure Private DNS zone name).
type DNSZoneRecords[Z any] interface {
	// LookupZone resolves the network's zone, reaching the cloud when the
	// local view has none. false means the network has no zone and
	// registration is a no-op.
	LookupZone(ctx context.Context, networkID string) (Z, bool)
	// GetZone returns only the locally known zone, for deregistration and
	// resolution after a register already looked it up.
	GetZone(networkID string) (Z, bool)
	// ZoneName is the zone's identifier for log lines. Empty means the
	// zone state is unusable and is treated as no zone.
	ZoneName(zone Z) string
	// CreateA, DeleteA, CreateCNAME, and DeleteCNAME manage the records for
	// hostname in zone. Delete of a record that does not exist succeeds.
	CreateA(ctx context.Context, zone Z, hostname, ip string) error
	DeleteA(ctx context.Context, zone Z, hostname string) error
	CreateCNAME(ctx context.Context, zone Z, hostname, target string) error
	DeleteCNAME(ctx context.Context, zone Z, hostname string) error
	// ResolveA returns the addresses hostname's A record holds; none when
	// the record is absent.
	ResolveA(ctx context.Context, zone Z, hostname string) ([]string, error)
	// CNAMETarget resolves the fronting service's hostname from the
	// service name a CNAME registration carries. Empty means the service
	// is not ready yet and the registration is skipped for now.
	CNAMETarget(ctx context.Context, serviceName string) (string, error)
}

// DNSZoneDiscovery is the NetworkDiscoveryDriver over a DNSZoneRecords.
type DNSZoneDiscovery[Z any] struct {
	records DNSZoneRecords[Z]
	kind    api.NetworkDiscoveryKind
	logger  zerolog.Logger
}

// NewDNSZoneDiscovery wraps records as a discovery driver of the given kind.
func NewDNSZoneDiscovery[Z any](records DNSZoneRecords[Z], kind api.NetworkDiscoveryKind, logger zerolog.Logger) *DNSZoneDiscovery[Z] {
	return &DNSZoneDiscovery[Z]{records: records, kind: kind, logger: logger}
}

// RegisterContainer writes a CNAME when endpoint.Metadata["kind"] is
// "cname" (the target is resolved from Metadata["service-name"]), and an
// A record otherwise.
func (d *DNSZoneDiscovery[Z]) RegisterContainer(ctx context.Context, networkID, name, containerID string, endpoint *CloudEndpoint) error {
	if endpoint == nil {
		return nil
	}
	if endpoint.Metadata != nil && endpoint.Metadata["kind"] == "cname" {
		return d.registerCNAME(ctx, containerID, name, endpoint.Metadata["service-name"], networkID)
	}
	return d.registerA(ctx, containerID, name, endpoint.IPAddress, networkID)
}

// DeregisterContainer removes both record kinds, because the caller does
// not know which was registered. Both are idempotent.
func (d *DNSZoneDiscovery[Z]) DeregisterContainer(ctx context.Context, networkID, name, containerID string) error {
	if err := d.deregister(ctx, containerID, name, networkID, "CNAME", d.records.DeleteCNAME); err != nil {
		_ = d.deregister(ctx, containerID, name, networkID, "A", d.records.DeleteA)
		return err
	}
	return d.deregister(ctx, containerID, name, networkID, "A", d.records.DeleteA)
}

// DeregisterContainerCNAME removes only the CNAME record.
func (d *DNSZoneDiscovery[Z]) DeregisterContainerCNAME(ctx context.Context, networkID, name string) error {
	return d.deregister(ctx, "", name, networkID, "CNAME", d.records.DeleteCNAME)
}

// DeregisterContainerARecord removes only the A record.
func (d *DNSZoneDiscovery[Z]) DeregisterContainerARecord(ctx context.Context, networkID, name string) error {
	return d.deregister(ctx, "", name, networkID, "A", d.records.DeleteA)
}

// ResolveName returns the first address of name's A record, or (nil, nil)
// when the name has none.
func (d *DNSZoneDiscovery[Z]) ResolveName(ctx context.Context, networkID, name string) (*CloudEndpoint, error) {
	zone, ok := d.records.GetZone(networkID)
	if !ok || d.records.ZoneName(zone) == "" {
		return nil, fmt.Errorf("network %q has no DNS zone", networkID)
	}
	ips, err := d.records.ResolveA(ctx, zone, name)
	if err != nil {
		return nil, fmt.Errorf("DNS resolve failed for %s in network %s: %w", name, networkID, err)
	}
	if len(ips) == 0 {
		return nil, nil
	}
	return &CloudEndpoint{IPAddress: ips[0]}, nil
}

// Kind returns the driver's category.
func (d *DNSZoneDiscovery[Z]) Kind() api.NetworkDiscoveryKind { return d.kind }

func (d *DNSZoneDiscovery[Z]) registerA(ctx context.Context, containerID, hostname, ip, networkID string) error {
	if ip == "" || ip == "0.0.0.0" {
		d.logger.Info().Str("container", shortID(containerID)).Str("hostname", hostname).Str("network", networkID).
			Msg("skipping DNS register: workload has no per-instance IP; enable CNAME-based discovery instead")
		return nil
	}
	zone, ok := d.records.LookupZone(ctx, networkID)
	if !ok || d.records.ZoneName(zone) == "" {
		d.logger.Debug().Str("container", shortID(containerID)).Str("network", networkID).
			Msg("no DNS zone for network, skipping service registration")
		return nil
	}
	if err := d.records.CreateA(ctx, zone, hostname, ip); err != nil {
		d.logger.Error().Err(err).Str("hostname", hostname).Str("ip", ip).Str("zone", d.records.ZoneName(zone)).
			Msg("failed to create DNS A record")
		return fmt.Errorf("DNS register failed for %s → %s: %w", hostname, ip, err)
	}
	d.logger.Info().Str("hostname", hostname).Str("ip", ip).Str("zone", d.records.ZoneName(zone)).Str("container", shortID(containerID)).
		Msg("registered DNS A record for service discovery")
	return nil
}

func (d *DNSZoneDiscovery[Z]) registerCNAME(ctx context.Context, containerID, hostname, serviceName, networkID string) error {
	if serviceName == "" {
		return nil
	}
	target, err := d.records.CNAMETarget(ctx, serviceName)
	if err != nil {
		return fmt.Errorf("resolve DNS CNAME target for %s: %w", serviceName, err)
	}
	if target == "" {
		d.logger.Info().Str("container", shortID(containerID)).Str("hostname", hostname).
			Msg("service has no hostname yet; skipping DNS CNAME")
		return nil
	}
	zone, ok := d.records.LookupZone(ctx, networkID)
	if !ok || d.records.ZoneName(zone) == "" {
		return nil
	}
	if err := d.records.CreateCNAME(ctx, zone, hostname, target); err != nil {
		d.logger.Error().Err(err).Str("hostname", hostname).Str("target", target).Str("zone", d.records.ZoneName(zone)).
			Msg("failed to create DNS CNAME record")
		return fmt.Errorf("DNS CNAME register failed for %s → %s: %w", hostname, target, err)
	}
	d.logger.Info().Str("hostname", hostname).Str("target", target).Str("zone", d.records.ZoneName(zone)).Str("container", shortID(containerID)).
		Msg("registered DNS CNAME record for service discovery")
	return nil
}

func (d *DNSZoneDiscovery[Z]) deregister(ctx context.Context, containerID, hostname, networkID, recordType string, del func(context.Context, Z, string) error) error {
	zone, ok := d.records.GetZone(networkID)
	if !ok || d.records.ZoneName(zone) == "" {
		return nil
	}
	if err := del(ctx, zone, hostname); err != nil {
		d.logger.Warn().Err(err).Str("hostname", hostname).Str("zone", d.records.ZoneName(zone)).Str("container", shortID(containerID)).
			Msgf("failed to delete DNS %s record", recordType)
		return fmt.Errorf("DNS %s deregister failed for %s: %w", recordType, hostname, err)
	}
	d.logger.Debug().Str("hostname", hostname).Str("zone", d.records.ZoneName(zone)).Str("container", shortID(containerID)).
		Msgf("deregistered DNS %s record", recordType)
	return nil
}
