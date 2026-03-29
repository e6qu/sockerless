package ecs

import "github.com/sockerless/api"

// NetworkCreate creates a Docker network with cloud backing (VPC security group + Cloud Map namespace).
func (s *Server) NetworkCreate(req *api.NetworkCreateRequest) (*api.NetworkCreateResponse, error) {
	resp, err := s.BaseServer.NetworkCreate(req)
	if err != nil {
		return nil, err
	}

	// Create VPC security group for network isolation
	if err := s.cloudNetworkCreate(req.Name, resp.ID); err != nil {
		s.Logger.Error().Err(err).Str("network", req.Name).Msg("failed to create cloud network resources")
	}

	// Create Cloud Map namespace for DNS-based service discovery
	if err := s.cloudNamespaceCreate(req.Name, resp.ID); err != nil {
		s.Logger.Warn().Err(err).Str("network", req.Name).Msg("failed to create Cloud Map namespace")
	}

	return resp, nil
}

// NetworkRemove removes a Docker network and its cloud resources.
func (s *Server) NetworkRemove(id string) error {
	n, ok := s.Store.ResolveNetwork(id)
	if !ok {
		return &api.NotFoundError{Resource: "network", ID: id}
	}

	// Clean up Cloud Map namespace first
	if err := s.cloudNamespaceDelete(n.ID); err != nil {
		s.Logger.Warn().Err(err).Str("network", n.Name).Msg("failed to delete Cloud Map namespace")
	}

	// Clean up VPC security group
	if err := s.cloudNetworkDelete(n.ID); err != nil {
		s.Logger.Warn().Err(err).Str("network", n.Name).Msg("failed to delete cloud network resources")
	}

	return s.BaseServer.NetworkRemove(id)
}

// NetworkConnect connects a container to a network with cloud security group association.
func (s *Server) NetworkConnect(id string, req *api.NetworkConnectRequest) error {
	if err := s.BaseServer.NetworkConnect(id, req); err != nil {
		return err
	}

	net, ok := s.Store.ResolveNetwork(id)
	if !ok {
		return nil // base succeeded, cloud is best-effort
	}
	containerID, ok := s.Store.ResolveContainerID(req.Container)
	if !ok {
		return nil
	}

	if err := s.cloudNetworkConnect(net.ID, containerID); err != nil {
		s.Logger.Warn().Err(err).Msg("failed to associate cloud security group")
	}

	return nil
}

// NetworkDisconnect disconnects a container from a network and removes SG association.
func (s *Server) NetworkDisconnect(id string, req *api.NetworkDisconnectRequest) error {
	net, ok := s.Store.ResolveNetwork(id)
	if ok {
		containerID, _ := s.Store.ResolveContainerID(req.Container)
		if containerID != "" {
			if err := s.cloudNetworkDisconnect(net.ID, containerID); err != nil {
				s.Logger.Warn().Err(err).Msg("failed to remove cloud security group association")
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
