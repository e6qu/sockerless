package aca

import (
	"context"
	"fmt"
	"strings"

	"github.com/sockerless/api"
)

// NetworkCreate creates a Docker network with NSG tracking.
func (s *Server) NetworkCreate(req *api.NetworkCreateRequest) (*api.NetworkCreateResponse, error) {
	resp, err := s.BaseServer.NetworkCreate(req)
	if err != nil {
		return nil, err
	}

	// Set up cloud network state (NSG tracking)
	s.cloudNetworkCreate(req.Name, resp.ID)

	return resp, nil
}

// NetworkRemove removes a Docker network and its cloud state.
func (s *Server) NetworkRemove(id string) error {
	n, ok := s.Store.ResolveNetwork(id)
	if !ok {
		return &api.NotFoundError{Resource: "network", ID: id}
	}

	// Clean up cloud network state
	s.cloudNetworkDelete(n.ID)

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
	s.cloudNetworkAddRule(net.ID, ruleName)

	// Register container in service discovery
	c, _ := s.ResolveContainerAuto(context.Background(), containerID)
	hostname := strings.TrimPrefix(c.Name, "/")
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID == net.ID && ep.IPAddress != "" {
			s.cloudServiceRegister(containerID, hostname, ep.IPAddress, net.ID)
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
			s.cloudServiceDeregister(containerID, net.ID)
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
