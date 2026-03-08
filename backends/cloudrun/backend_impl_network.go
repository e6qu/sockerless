package cloudrun

import (
	"strings"

	"github.com/sockerless/api"
)

// NetworkCreate creates a Docker network with Cloud DNS backing.
func (s *Server) NetworkCreate(req *api.NetworkCreateRequest) (*api.NetworkCreateResponse, error) {
	resp, err := s.BaseServer.NetworkCreate(req)
	if err != nil {
		return nil, err
	}

	// Create Cloud DNS managed zone for service discovery
	if err := s.cloudNetworkCreate(req.Name, resp.ID); err != nil {
		s.Logger.Warn().Err(err).Str("network", req.Name).Msg("failed to create Cloud DNS zone")
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
	containerID, ok := s.Store.ResolveContainerID(req.Container)
	if !ok {
		return nil
	}
	c, _ := s.Store.Containers.Get(containerID)
	hostname := strings.TrimPrefix(c.Name, "/")
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
		containerID, _ := s.Store.ResolveContainerID(req.Container)
		if containerID != "" {
			c, _ := s.Store.Containers.Get(containerID)
			hostname := strings.TrimPrefix(c.Name, "/")
			if err := s.cloudServiceDeregister(containerID, hostname, net.ID); err != nil {
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
