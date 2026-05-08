// Phase 124 — service-mesh network-discovery driver for the ECS
// backend. Adapter that satisfies core.NetworkDiscoveryDriver by
// delegating to the existing cloudServiceRegister/Deregister/Resolve
// methods on *Server (which already speak AWS Cloud Map via the
// servicediscovery client held on the Server).
//
// Lives in the backend (not aws-common) because the implementation
// closes over per-backend state. The driver-interface surface lets
// callers migrate to driver-mediated registration incrementally.

package ecs

import (
	"context"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

type cloudMapDiscovery struct {
	s *Server
}

func newCloudMapDiscovery(s *Server) *cloudMapDiscovery {
	return &cloudMapDiscovery{s: s}
}

func (d *cloudMapDiscovery) RegisterContainer(ctx context.Context, networkID, name string, endpoint *core.CloudEndpoint) error {
	if endpoint == nil {
		return nil
	}
	containerID := ""
	if endpoint.Metadata != nil {
		containerID = endpoint.Metadata["container-id"]
	}
	return d.s.cloudServiceRegister(containerID, name, endpoint.IPAddress, networkID)
}

func (d *cloudMapDiscovery) DeregisterContainer(ctx context.Context, networkID, name string) error {
	// ECS deregister works by container-id (the Cloud Map instance
	// keyed off the registered ID); we don't know it from name alone
	// at this layer. Use empty container-id; the underlying method
	// uses cloud-state lookup to find the right service+instance.
	return d.s.cloudServiceDeregister("", networkID)
}

func (d *cloudMapDiscovery) ResolveName(ctx context.Context, networkID, name string) (*core.CloudEndpoint, error) {
	ips, err := d.s.cloudServiceResolve(name, networkID)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, nil
	}
	return &core.CloudEndpoint{IPAddress: ips[0]}, nil
}

func (d *cloudMapDiscovery) Kind() api.NetworkDiscoveryKind {
	return api.NetworkDiscoveryServiceMesh
}
