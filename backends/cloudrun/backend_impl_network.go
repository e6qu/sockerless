package cloudrun

import (
	"context"
	"strings"

	"github.com/sockerless/api"
)

// NetworkCreate creates a Docker network with Cloud DNS backing.
// Cloud DNS zone creation failures surface via the response's Warning
// field so callers see that cross-container DNS may not work on this
// network.
func (s *Server) NetworkCreate(req *api.NetworkCreateRequest) (*api.NetworkCreateResponse, error) {
	resp, err := s.BaseServer.NetworkCreate(req)
	if err != nil {
		return nil, err
	}

	if err := s.cloudNetworkCreate(req.Name, resp.ID); err != nil {
		s.Logger.Warn().Err(err).Str("network", req.Name).Msg("failed to create Cloud DNS zone")
		msg := "Cloud DNS zone (cross-container DNS): " + err.Error()
		if resp.Warning != "" {
			resp.Warning = resp.Warning + "; " + msg
		} else {
			resp.Warning = msg
		}
	}

	return resp, nil
}

// NetworkRemove removes a Docker network and its Cloud DNS zone.
func (s *Server) NetworkRemove(id string) error {
	n, ok := s.Store.ResolveNetwork(id)
	if !ok {
		return &api.NotFoundError{Resource: "network", ID: id}
	}

	// Clean up Cloud DNS zone first
	if err := s.cloudNetworkDelete(n.ID); err != nil {
		s.Logger.Warn().Err(err).Str("network", n.Name).Msg("failed to delete Cloud DNS zone")
	}

	return s.BaseServer.NetworkRemove(id)
}

// NetworkConnect connects a container to a network.
func (s *Server) NetworkConnect(id string, req *api.NetworkConnectRequest) error {
	if err := s.BaseServer.NetworkConnect(id, req); err != nil {
		return err
	}

	// Register container hostname in Cloud DNS for service discovery
	net, ok := s.Store.ResolveNetwork(id)
	if !ok {
		return nil
	}
	containerID, ok := s.ResolveContainerIDAuto(context.Background(), req.Container)
	if !ok {
		return nil
	}
	c, _ := s.ResolveContainerAuto(context.Background(), containerID)
	hostname := strings.TrimPrefix(c.Name, "/")

	// Phase 87 — Services path: register a CNAME pointing at the
	// Service URL. A-records don't apply because Cloud Run Services
	// have no per-instance IP; peers reach each other via the Service
	// URL over the VPC connector.
	if s.config.UseService {
		if svcState, ok := s.resolveServiceCloudRunState(s.ctx(), containerID); ok && svcState.ServiceName != "" {
			if err := s.cloudServiceRegisterCNAME(s.ctx(), containerID, hostname, svcState.ServiceName, net.ID); err != nil {
				s.Logger.Warn().Err(err).Msg("failed to register CNAME in Cloud DNS")
			}
		}
		return nil
	}

	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID == net.ID && ep.IPAddress != "" {
			if err := s.cloudServiceRegister(containerID, hostname, ep.IPAddress, net.ID); err != nil {
				s.Logger.Warn().Err(err).Msg("failed to register in Cloud DNS")
			}
			break
		}
	}

	return nil
}

// NetworkDisconnect disconnects a container from a network.
func (s *Server) NetworkDisconnect(id string, req *api.NetworkDisconnectRequest) error {
	// Deregister from Cloud DNS before disconnecting
	net, ok := s.Store.ResolveNetwork(id)
	if ok {
		containerID, _ := s.ResolveContainerIDAuto(context.Background(), req.Container)
		if containerID != "" {
			c, _ := s.ResolveContainerAuto(context.Background(), containerID)
			hostname := strings.TrimPrefix(c.Name, "/")
			if s.config.UseService {
				if err := s.cloudServiceDeregisterCNAME(s.ctx(), containerID, hostname, net.ID); err != nil {
					s.Logger.Warn().Err(err).Msg("failed to deregister CNAME from Cloud DNS")
				}
			} else if err := s.cloudServiceDeregister(containerID, hostname, net.ID); err != nil {
				s.Logger.Warn().Err(err).Msg("failed to deregister from Cloud DNS")
			}
		}
	}
	return s.BaseServer.NetworkDisconnect(id, req)
}

// NetworkInspect returns details about a network.
func (s *Server) NetworkInspect(id string) (*api.Network, error) {
	return s.BaseServer.NetworkInspect(id)
}

// NetworkList lists networks.
func (s *Server) NetworkList(filters map[string][]string) ([]*api.Network, error) {
	return s.BaseServer.NetworkList(filters)
}

// NetworkPrune prunes unused networks.
func (s *Server) NetworkPrune(filters map[string][]string) (*api.NetworkPruneResponse, error) {
	return s.BaseServer.NetworkPrune(filters)
}
