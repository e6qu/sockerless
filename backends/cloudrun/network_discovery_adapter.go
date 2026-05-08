// Phase 124 — cloud-DNS network-discovery driver for the Cloud Run
// backend. Adapter that satisfies core.NetworkDiscoveryDriver by
// delegating to the existing cloudServiceRegister*/Deregister*/Resolve
// methods on *Server (which already speak the GCP DNS + Cloud Run
// Services SDKs against the resolved network state).
//
// Lives in the backend (not gcp-common) because the implementation
// closes over per-backend state (s.gcp clients, s.NetworkState,
// s.config.Project, s.Logger). A future refactor could extract a
// minimal "DNS registrar" struct into gcp-common; for now the adapter
// keeps the driver-interface surface stable while reusing the
// already-tested code paths.

package cloudrun

import (
	"context"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// cloudDNSDiscovery satisfies core.NetworkDiscoveryDriver by delegating
// to the *Server's existing cloud-DNS methods.
type cloudDNSDiscovery struct {
	s *Server
}

// newCloudDNSDiscovery wraps a Server in the discovery-driver interface.
func newCloudDNSDiscovery(s *Server) *cloudDNSDiscovery {
	return &cloudDNSDiscovery{s: s}
}

func (d *cloudDNSDiscovery) RegisterContainer(ctx context.Context, networkID, name string, endpoint *core.CloudEndpoint) error {
	if endpoint == nil {
		return nil
	}
	// Delegate to the existing A-record path. The CNAME path
	// (cloudServiceRegisterCNAME) is reserved for the UseService flow
	// and is invoked separately at materialize time when the backend
	// has the underlying Cloud Run service name in hand — that
	// remains a direct call site for now.
	containerID := ""
	if endpoint.Metadata != nil {
		containerID = endpoint.Metadata["container-id"]
	}
	return d.s.cloudServiceRegister(containerID, name, endpoint.IPAddress, networkID)
}

func (d *cloudDNSDiscovery) DeregisterContainer(ctx context.Context, networkID, name string) error {
	// Container ID is not strictly needed for delete (cloudServiceDeregister
	// uses hostname + networkID to locate the record); pass empty.
	return d.s.cloudServiceDeregister("", name, networkID)
}

func (d *cloudDNSDiscovery) ResolveName(ctx context.Context, networkID, name string) (*core.CloudEndpoint, error) {
	ips, err := d.s.cloudServiceResolve(name, networkID)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, nil
	}
	return &core.CloudEndpoint{IPAddress: ips[0]}, nil
}

func (d *cloudDNSDiscovery) Kind() api.NetworkDiscoveryKind {
	return api.NetworkDiscoveryCloudDNS
}
