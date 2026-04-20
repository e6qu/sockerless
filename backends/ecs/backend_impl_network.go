package ecs

import (
	"context"

	"github.com/sockerless/api"
)

// NetworkCreate creates a Docker network with cloud backing (VPC
// security group + Cloud Map namespace). Cloud-side failures surface
// via the response's Warning field rather than being swallowed so
// callers (docker CLI, test harnesses) can see that cross-container
// DNS or network isolation may not work for this network.
func (s *Server) NetworkCreate(req *api.NetworkCreateRequest) (*api.NetworkCreateResponse, error) {
	resp, err := s.BaseServer.NetworkCreate(req)
	if err != nil {
		return nil, err
	}

	var warnings []string

	if err := s.cloudNetworkCreate(req.Name, resp.ID); err != nil {
		s.Logger.Error().Err(err).Str("network", req.Name).Msg("failed to create cloud network resources")
		warnings = append(warnings, "VPC security group: "+err.Error())
	}

	if err := s.cloudNamespaceCreate(req.Name, resp.ID); err != nil {
		s.Logger.Warn().Err(err).Str("network", req.Name).Msg("failed to create Cloud Map namespace")
		warnings = append(warnings, "Cloud Map namespace (cross-container DNS): "+err.Error())
	}

	if len(warnings) > 0 {
		resp.Warning = joinWarnings(resp.Warning, warnings...)
	}

	return resp, nil
}

// joinWarnings combines an existing warning string (possibly set by
// the BaseServer) with additional cloud-side messages, separated by
// "; ". Matches how docker clients display the Warning field.
func joinWarnings(existing string, extras ...string) string {
	parts := make([]string, 0, len(extras)+1)
	if existing != "" {
		parts = append(parts, existing)
	}
	parts = append(parts, extras...)
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "; "
		}
		out += p
	}
	return out
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
	containerID, ok := s.ResolveContainerIDAuto(context.Background(), req.Container)
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
		containerID, _ := s.ResolveContainerIDAuto(context.Background(), req.Container)
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
