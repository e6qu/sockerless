// private-dns network-discovery driver shared by every Microsoft Azure
// backend. A Docker network is an Azure Private DNS zone; the shared
// core.DNSZoneDiscovery decides which record a container gets, and this
// file supplies the Private DNS record operations and the Azure Container
// Apps lookup a CNAME target needs.
package azurecommon

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/privatedns/armprivatedns"
	"github.com/rs/zerolog"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// PrivateDNSNetworkState is the per-network zone state.
type PrivateDNSNetworkState struct {
	DNSZoneName string // Azure Private DNS zone backing this network
}

// PrivateDNSDiscoveryConfig is the constructor argument. ContainerApps is
// only needed by the CNAME path; a backend that never fronts a container
// with a Container App leaves it nil.
type PrivateDNSDiscoveryConfig struct {
	PrivateDNSRecords *armprivatedns.RecordSetsClient
	ContainerApps     *armappcontainers.ContainerAppsClient
	ResourceGroup     string
	Logger            zerolog.Logger
	// LookupNetwork resolves the network's zone, reaching the cloud when
	// the local view has none. false means the network has no zone.
	LookupNetwork func(ctx context.Context, networkID string) (PrivateDNSNetworkState, bool)
	// GetNetwork returns only the locally known zone.
	GetNetwork func(networkID string) (PrivateDNSNetworkState, bool)
}

// PrivateDNSDiscovery is the core.NetworkDiscoveryDriver over Azure
// Private DNS.
type PrivateDNSDiscovery = core.DNSZoneDiscovery[PrivateDNSNetworkState]

// NewPrivateDNSDiscovery wraps the SDK clients and zone callbacks as a
// discovery driver.
func NewPrivateDNSDiscovery(cfg PrivateDNSDiscoveryConfig) *PrivateDNSDiscovery {
	return core.NewDNSZoneDiscovery[PrivateDNSNetworkState](privateDNSZone{cfg: cfg}, api.NetworkDiscoveryCloudDNS, cfg.Logger)
}

// privateDNSZone implements core.DNSZoneRecords over the Private DNS API.
type privateDNSZone struct {
	cfg PrivateDNSDiscoveryConfig
}

func (z privateDNSZone) LookupZone(ctx context.Context, networkID string) (PrivateDNSNetworkState, bool) {
	return z.cfg.LookupNetwork(ctx, networkID)
}

func (z privateDNSZone) GetZone(networkID string) (PrivateDNSNetworkState, bool) {
	return z.cfg.GetNetwork(networkID)
}

func (z privateDNSZone) ZoneName(zone PrivateDNSNetworkState) string { return zone.DNSZoneName }

func (z privateDNSZone) CreateA(ctx context.Context, zone PrivateDNSNetworkState, hostname, ip string) error {
	_, err := z.cfg.PrivateDNSRecords.CreateOrUpdate(ctx, z.cfg.ResourceGroup, zone.DNSZoneName, armprivatedns.RecordTypeA, hostname, armprivatedns.RecordSet{
		Properties: &armprivatedns.RecordSetProperties{
			TTL:      to.Ptr(int64(60)),
			ARecords: []*armprivatedns.ARecord{{IPv4Address: to.Ptr(ip)}},
		},
	}, nil)
	return err
}

func (z privateDNSZone) DeleteA(ctx context.Context, zone PrivateDNSNetworkState, hostname string) error {
	return z.delete(ctx, zone, hostname, armprivatedns.RecordTypeA)
}

func (z privateDNSZone) CreateCNAME(ctx context.Context, zone PrivateDNSNetworkState, hostname, target string) error {
	_, err := z.cfg.PrivateDNSRecords.CreateOrUpdate(ctx, z.cfg.ResourceGroup, zone.DNSZoneName, armprivatedns.RecordTypeCNAME, hostname, armprivatedns.RecordSet{
		Properties: &armprivatedns.RecordSetProperties{
			TTL:         to.Ptr(int64(60)),
			CnameRecord: &armprivatedns.CnameRecord{Cname: to.Ptr(target)},
		},
	}, nil)
	return err
}

func (z privateDNSZone) DeleteCNAME(ctx context.Context, zone PrivateDNSNetworkState, hostname string) error {
	return z.delete(ctx, zone, hostname, armprivatedns.RecordTypeCNAME)
}

func (z privateDNSZone) ResolveA(ctx context.Context, zone PrivateDNSNetworkState, hostname string) ([]string, error) {
	resp, err := z.cfg.PrivateDNSRecords.Get(ctx, z.cfg.ResourceGroup, zone.DNSZoneName, armprivatedns.RecordTypeA, hostname, nil)
	if err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, err
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

// CNAMETarget reads the Container App's latest revision FQDN, the stable
// internal hostname peers in the managed environment reach it by. An
// empty FQDN means the app is not ready yet.
func (z privateDNSZone) CNAMETarget(ctx context.Context, appName string) (string, error) {
	if z.cfg.ContainerApps == nil {
		return "", nil
	}
	resp, err := z.cfg.ContainerApps.Get(ctx, z.cfg.ResourceGroup, appName, nil)
	if err != nil {
		return "", fmt.Errorf("get containerapp for DNS target: %w", err)
	}
	if resp.Properties != nil && resp.Properties.LatestRevisionFqdn != nil {
		return *resp.Properties.LatestRevisionFqdn, nil
	}
	return "", nil
}

func (z privateDNSZone) delete(ctx context.Context, zone PrivateDNSNetworkState, hostname string, recordType armprivatedns.RecordType) error {
	_, err := z.cfg.PrivateDNSRecords.Delete(ctx, z.cfg.ResourceGroup, zone.DNSZoneName, recordType, hostname, nil)
	if err != nil && IsNotFound(err) {
		return nil
	}
	return err
}
