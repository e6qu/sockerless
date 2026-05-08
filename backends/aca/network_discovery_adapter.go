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

func (d *acaCloudDNSDiscovery) RegisterContainer(ctx context.Context, networkID, name string, endpoint *core.CloudEndpoint) error {
	if endpoint == nil {
		return nil
	}
	containerID := ""
	if endpoint.Metadata != nil {
		containerID = endpoint.Metadata["container-id"]
	}
	return d.s.cloudServiceRegister(containerID, name, endpoint.IPAddress, networkID)
}

func (d *acaCloudDNSDiscovery) DeregisterContainer(ctx context.Context, networkID, name string) error {
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
