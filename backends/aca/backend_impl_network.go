package aca

import (
	"context"
	"fmt"
	"strings"

	"github.com/sockerless/api"
)

// NetworkCreate creates a Docker network and its Azure cloud
// resources — a per-network NSG + Private DNS zone (BUG-702/703).
// Cloud-side failures surface as response Warnings (BUG-700) so
// callers know what degraded, matching the ECS and Cloud Run
// backends.
func (s *Server) NetworkCreate(req *api.NetworkCreateRequest) (*api.NetworkCreateResponse, error) {
	resp, err := s.BaseServer.NetworkCreate(req)
	if err != nil {
		return nil, err
	}

	var warnings []string
	if err := s.cloudNetworkCreate(req.Name, resp.ID); err != nil {
		s.Logger.Warn().Err(err).Str("network", req.Name).Msg("failed to create cloud network resources")
		warnings = append(warnings, "Azure cloud network resources: "+err.Error())
	}
	if len(warnings) > 0 {
		if resp.Warning != "" {
			warnings = append([]string{resp.Warning}, warnings...)
		}
		resp.Warning = strings.Join(warnings, "; ")
	}

	return resp, nil
}

// NetworkRemove removes a Docker network and its cloud state.
func (s *Server) NetworkRemove(id string) error {
	n, ok := s.Store.ResolveNetwork(id)
	if !ok {
		return &api.NotFoundError{Resource: "network", ID: id}
	}

	// Clean up cloud network state (Private DNS zone + NSG tracking)
	_ = s.cloudNetworkDelete(n.ID)

	return s.BaseServer.NetworkRemove(id)
}

// NetworkConnect connects a container to a network with service registration.
func (s *Server) NetworkConnect(id string, req *api.NetworkConnectRequest) error {
	if err := s.BaseServer.NetworkConnect(id, req); err != nil {
		return err
	}

	net, ok := s.Store.ResolveNetwork(id)
	if !ok {
		return nil
	}
	containerID, ok := s.ResolveContainerIDAuto(context.Background(), req.Container)
	if !ok {
		return nil
	}

	// Track NSG rule for this container-network association
	ruleName := fmt.Sprintf("allow-%s-%s", containerID[:12], net.Name)
	if err := s.cloudNetworkAddRule(net.ID, ruleName); err != nil {
		s.Logger.Warn().Err(err).Str("rule", ruleName).Msg("failed to create NSG rule")
	}

	// Register container in service discovery
	c, _ := s.ResolveContainerAuto(context.Background(), containerID)
	hostname := strings.TrimPrefix(c.Name, "/")
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID == net.ID && ep.IPAddress != "" {
			if err := s.cloudServiceRegister(containerID, hostname, ep.IPAddress, net.ID); err != nil {
				s.Logger.Warn().Err(err).Msg("failed to register service in Private DNS")
			}
			break
		}
	}

	return nil
}

// NetworkDisconnect disconnects a container from a network and deregisters it.
func (s *Server) NetworkDisconnect(id string, req *api.NetworkDisconnectRequest) error {
	// Deregister from service discovery before disconnecting
	net, ok := s.Store.ResolveNetwork(id)
	if ok {
		containerID, _ := s.ResolveContainerIDAuto(context.Background(), req.Container)
		if containerID != "" {
			c, cOk := s.ResolveContainerAuto(context.Background(), containerID)
			hostname := ""
			if cOk {
				hostname = strings.TrimPrefix(c.Name, "/")
			}
			_ = s.cloudServiceDeregister(containerID, hostname, net.ID)
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
