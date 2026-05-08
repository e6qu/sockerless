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

// RegisterContainer dispatches to either the A-record path or the
// CNAME path based on endpoint.Metadata["kind"]:
//
//   - "a-record" (default): cloudServiceRegister with endpoint.IPAddress.
//   - "cname":              cloudServiceRegisterCNAME with the underlying
//     Cloud Run service URI from
//     endpoint.Metadata["service-name"].
//
// container-id passes through endpoint.Metadata["container-id"].
func (d *cloudDNSDiscovery) RegisterContainer(ctx context.Context, networkID, name, containerID string, endpoint *core.CloudEndpoint) error {
	if endpoint == nil {
		return nil
	}
	if endpoint.Metadata != nil && endpoint.Metadata["kind"] == "cname" {
		serviceName := endpoint.Metadata["service-name"]
		return d.s.cloudServiceRegisterCNAME(ctx, containerID, name, serviceName, networkID)
	}
	return d.s.cloudServiceRegister(containerID, name, endpoint.IPAddress, networkID)
}

// DeregisterContainer mirrors RegisterContainer's dispatch via
// metadata["kind"]. Caller passes the same kind used at register time.
// When metadata is nil, the A-record path is used (which is the
// most common register shape).
func (d *cloudDNSDiscovery) DeregisterContainer(ctx context.Context, networkID, name, containerID string) error {
	// On deregister we don't have the original endpoint, so we try
	// the CNAME-aware deregister path which is a no-op when no CNAME
	// exists, then the A-record deregister. Both are idempotent.
	if err := d.s.cloudServiceDeregisterCNAME(ctx, containerID, name, networkID); err != nil {
		_ = d.s.cloudServiceDeregister(containerID, name, networkID)
		return err
	}
	return d.s.cloudServiceDeregister(containerID, name, networkID)
}

// DeregisterContainerCNAME is an explicit-kind variant that callers
// holding the original metadata can use to skip the A-record fallback
// path. Lives outside the interface to keep the contract simple.
func (d *cloudDNSDiscovery) DeregisterContainerCNAME(ctx context.Context, networkID, name string) error {
	return d.s.cloudServiceDeregisterCNAME(ctx, "", name, networkID)
}

// DeregisterContainerARecord is the explicit A-record-only variant.
func (d *cloudDNSDiscovery) DeregisterContainerARecord(networkID, name string) error {
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
