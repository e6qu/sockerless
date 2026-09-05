// cloud-dns network-discovery driver shared by every Google Cloud backend.
// A Docker network is a Cloud DNS managed zone; the shared
// core.DNSZoneDiscovery decides which record a container gets, and this
// file supplies the Cloud DNS record operations and the Cloud Run
// service-URL lookup a CNAME target needs.
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

// CloudDNSNetworkState is the per-network zone state: the managed zone's
// name and the DNS name records are created under.
type CloudDNSNetworkState struct {
	ManagedZoneName string // Cloud DNS managed zone name
	DNSName         string // DNS zone name (e.g., "network-name.internal.")
}

// CloudDNSDiscoveryConfig is the constructor argument. RunServices is only
// needed by the CNAME path; a backend that never fronts a container with a
// Cloud Run service leaves it nil.
type CloudDNSDiscoveryConfig struct {
	DNS         *dns.Service
	RunServices *runv2.ServicesClient
	Project     string
	Logger      zerolog.Logger
	// LookupNetwork resolves the network's zone, reaching the cloud when
	// the local view has none. false means the network has no zone.
	LookupNetwork func(ctx context.Context, networkID string) (CloudDNSNetworkState, bool)
	// GetNetwork returns only the locally known zone.
	GetNetwork func(networkID string) (CloudDNSNetworkState, bool)
}

// CloudDNSDiscovery is the core.NetworkDiscoveryDriver over Cloud DNS.
type CloudDNSDiscovery = core.DNSZoneDiscovery[CloudDNSNetworkState]

// NewCloudDNSDiscovery wraps the SDK clients and zone callbacks as a
// discovery driver.
func NewCloudDNSDiscovery(cfg CloudDNSDiscoveryConfig) *CloudDNSDiscovery {
	return core.NewDNSZoneDiscovery[CloudDNSNetworkState](cloudDNSZone{cfg: cfg}, api.NetworkDiscoveryCloudDNS, cfg.Logger)
}

// cloudDNSZone implements core.DNSZoneRecords over the Cloud DNS API.
type cloudDNSZone struct {
	cfg CloudDNSDiscoveryConfig
}

func (z cloudDNSZone) LookupZone(ctx context.Context, networkID string) (CloudDNSNetworkState, bool) {
	return z.cfg.LookupNetwork(ctx, networkID)
}

func (z cloudDNSZone) GetZone(networkID string) (CloudDNSNetworkState, bool) {
	return z.cfg.GetNetwork(networkID)
}

func (z cloudDNSZone) ZoneName(zone CloudDNSNetworkState) string { return zone.ManagedZoneName }

func (z cloudDNSZone) CreateA(ctx context.Context, zone CloudDNSNetworkState, hostname, ip string) error {
	return z.create(ctx, zone, hostname, "A", ip)
}

func (z cloudDNSZone) DeleteA(ctx context.Context, zone CloudDNSNetworkState, hostname string) error {
	return z.delete(ctx, zone, hostname, "A")
}

func (z cloudDNSZone) CreateCNAME(ctx context.Context, zone CloudDNSNetworkState, hostname, target string) error {
	return z.create(ctx, zone, hostname, "CNAME", target+".")
}

func (z cloudDNSZone) DeleteCNAME(ctx context.Context, zone CloudDNSNetworkState, hostname string) error {
	return z.delete(ctx, zone, hostname, "CNAME")
}

func (z cloudDNSZone) ResolveA(ctx context.Context, zone CloudDNSNetworkState, hostname string) ([]string, error) {
	fqdn := hostname + "." + zone.DNSName
	resp, err := z.cfg.DNS.ResourceRecordSets.List(z.cfg.Project, zone.ManagedZoneName).Name(fqdn).Type("A").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	var ips []string
	for _, rrs := range resp.Rrsets {
		if rrs.Name == fqdn && rrs.Type == "A" {
			ips = append(ips, rrs.Rrdatas...)
		}
	}
	return ips, nil
}

// CNAMETarget reads the Cloud Run service's URI and returns its host, the
// target a container's CNAME points at. An empty URI means the service is
// not ready yet.
func (z cloudDNSZone) CNAMETarget(ctx context.Context, serviceName string) (string, error) {
	if z.cfg.RunServices == nil {
		return "", nil
	}
	svc, err := z.cfg.RunServices.GetService(ctx, &runpb.GetServiceRequest{Name: serviceName})
	if err != nil {
		return "", fmt.Errorf("get service for DNS target: %w", err)
	}
	return serviceURIHost(svc.Uri), nil
}

func (z cloudDNSZone) create(ctx context.Context, zone CloudDNSNetworkState, hostname, recordType, rrdata string) error {
	_, err := z.cfg.DNS.ResourceRecordSets.Create(z.cfg.Project, zone.ManagedZoneName, &dns.ResourceRecordSet{
		Name:    hostname + "." + zone.DNSName,
		Type:    recordType,
		Ttl:     60,
		Rrdatas: []string{rrdata},
	}).Context(ctx).Do()
	return err
}

func (z cloudDNSZone) delete(ctx context.Context, zone CloudDNSNetworkState, hostname, recordType string) error {
	_, err := z.cfg.DNS.ResourceRecordSets.Delete(z.cfg.Project, zone.ManagedZoneName, hostname+"."+zone.DNSName, recordType).Context(ctx).Do()
	if err != nil && IsNotFound(err) {
		return nil
	}
	return err
}

// serviceURIHost extracts the hostname from a Cloud Run Service.Uri
// ("https://sockerless-svc-abc-xxx.a.run.app" → "sockerless-svc-abc-xxx.a.run.app").
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
