// Phase 124 — cloud-DNS network-discovery driver for the ACA backend.
// Adapter that satisfies core.NetworkDiscoveryDriver by delegating to
// the existing cloudServiceRegister/Deregister/Resolve methods on
// *Server (which already speak Azure Private DNS Zones via the
// privatedns client held on the Server).
//
// Lives in the backend (not azure-common) for the same reason as the
// cloudrun adapter: the implementation closes over per-backend state.

package aca

import (
	"context"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

type acaCloudDNSDiscovery struct {
	s *Server
}

func newACACloudDNSDiscovery(s *Server) *acaCloudDNSDiscovery {
	return &acaCloudDNSDiscovery{s: s}
}

// RegisterContainer dispatches to A-record or CNAME based on
// endpoint.Metadata["kind"] (default: a-record). For "cname",
// metadata["service-name"] holds the underlying ACA app name that
// the CNAME target resolves to.
func (d *acaCloudDNSDiscovery) RegisterContainer(ctx context.Context, networkID, name, containerID string, endpoint *core.CloudEndpoint) error {
	if endpoint == nil {
		return nil
	}
	if endpoint.Metadata != nil && endpoint.Metadata["kind"] == "cname" {
		appName := endpoint.Metadata["service-name"]
		return d.s.cloudServiceRegisterCNAME(ctx, containerID, name, appName, networkID)
	}
	return d.s.cloudServiceRegister(containerID, name, endpoint.IPAddress, networkID)
}

func (d *acaCloudDNSDiscovery) DeregisterContainer(ctx context.Context, networkID, name, containerID string) error {
	if err := d.s.cloudServiceDeregisterCNAME(ctx, containerID, name, networkID); err != nil {
		_ = d.s.cloudServiceDeregister(containerID, name, networkID)
		return err
	}
	return d.s.cloudServiceDeregister(containerID, name, networkID)
}

// DeregisterContainerCNAME is the CNAME-only variant for callers that
// know the original register kind.
func (d *acaCloudDNSDiscovery) DeregisterContainerCNAME(ctx context.Context, networkID, name string) error {
	return d.s.cloudServiceDeregisterCNAME(ctx, "", name, networkID)
}

// DeregisterContainerARecord is the A-record-only variant.
func (d *acaCloudDNSDiscovery) DeregisterContainerARecord(networkID, name string) error {
	return d.s.cloudServiceDeregister("", name, networkID)
}

func (d *acaCloudDNSDiscovery) ResolveName(ctx context.Context, networkID, name string) (*core.CloudEndpoint, error) {
	ips, err := d.s.cloudServiceResolve(name, networkID)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, nil
	}
	return &core.CloudEndpoint{IPAddress: ips[0]}, nil
}

func (d *acaCloudDNSDiscovery) Kind() api.NetworkDiscoveryKind {
	return api.NetworkDiscoveryCloudDNS
}
