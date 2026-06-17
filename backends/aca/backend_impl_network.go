package aca

import (
	"context"
	"fmt"
	"strings"

	"github.com/sockerless/api"
	azurecommon "github.com/sockerless/azure-common"
	core "github.com/sockerless/backend-core"
)

// NetworkCreate creates a Docker network and its Azure cloud
// resources — a per-network NSG + Private DNS zone.
// Cloud-side failures surface as response Warnings so callers know
// what degraded, matching the ECS and Cloud Run backends.
func (s *Server) NetworkCreate(req *api.NetworkCreateRequest) (*api.NetworkCreateResponse, error) {
	resp, err := s.BaseServer.NetworkCreate(req)
	if err != nil {
		return nil, err
	}

	var warnings []string
	provisionDNSZone := s.config.NetworkDiscovery == api.NetworkDiscoveryCloudDNS
	if err := s.cloudNetworkCreate(req.Name, resp.ID, provisionDNSZone); err != nil {
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

	// Clean up cloud network state (Private DNS zone + NSG tracking) and the
	// local metadata. Surface a cloud-cleanup failure — swallowing it orphans
	// the Private DNS zone / NSG while reporting a successful network removal.
	cloudErr := s.cloudNetworkDelete(n.ID)
	if err := s.BaseServer.NetworkRemove(id); err != nil {
		return err
	}
	if cloudErr != nil {
		return fmt.Errorf("network %s removed locally but cloud cleanup failed: %w", id, cloudErr)
	}
	return nil
}

// registerContainerServiceDiscovery registers the network aliases a client
// requested (`docker create --network X --network-alias web`) in the cloud
// DNS for each network the container is attached to. Called at ContainerStart
// once the App is materialized (the create-with-network path never fires
// NetworkConnect, so the runner's `--network-alias web` would otherwise be
// lost). Apps register CNAMEs to the App; Jobs register A-records to the
// container IP.
//
// Only the explicit aliases are registered — never the container's own
// hostname. A peer reaches a service by the alias the workflow declared
// (`services: { web: … }` → `http://web`); registering the hostname would
// add no reachable name anyone uses, and (in App mode) would force the sim
// to refresh that App's env-network attachment to realize the alias — which
// for the aliasless job container would needlessly tear down and re-establish
// its reverse-agent channel mid-exec. Service containers have aliases and
// start before the job, so realizing their aliases happens early and idle.
func (s *Server) registerContainerServiceDiscovery(id string, c api.Container) {
	for _, ep := range c.NetworkSettings.Networks {
		if ep == nil || ep.NetworkID == "" {
			continue
		}
		if len(ep.Aliases) == 0 {
			continue
		}
		names := append([]string(nil), ep.Aliases...)
		if s.config.UseApp {
			appState, ok := s.resolveAppACAState(s.ctx(), id)
			if !ok || appState.AppName == "" {
				continue
			}
			for _, n := range names {
				if err := s.NetworkDiscovery.RegisterContainer(s.ctx(), ep.NetworkID, n, id, &core.CloudEndpoint{
					Metadata: map[string]string{"kind": "cname", "service-name": appState.AppName},
				}); err != nil {
					s.Logger.Warn().Err(err).Str("name", n).Msg("failed to register service name in Private DNS")
				}
			}
			continue
		}
		if ep.IPAddress == "" {
			continue
		}
		for _, n := range names {
			if err := s.NetworkDiscovery.RegisterContainer(s.ctx(), ep.NetworkID, n, id, &core.CloudEndpoint{
				IPAddress: ep.IPAddress,
			}); err != nil {
				s.Logger.Warn().Err(err).Str("name", n).Msg("failed to register service name in Private DNS")
			}
		}
	}
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

	// Register container in service discovery.
	c, _ := s.ResolveContainerAuto(context.Background(), containerID)
	hostname := strings.TrimPrefix(c.Name, "/")

	// — Apps path: register a CNAME pointing at the
	// ContainerApp's LatestRevisionFqdn. Apps have peer-reachable
	// internal FQDNs inside the managed environment, unlike Jobs.
	if s.config.UseApp {
		if appState, ok := s.resolveAppACAState(s.ctx(), containerID); ok && appState.AppName != "" {
			if err := s.NetworkDiscovery.RegisterContainer(s.ctx(), net.ID, hostname, containerID, &core.CloudEndpoint{
				Metadata: map[string]string{
					"kind":         "cname",
					"service-name": appState.AppName,
				},
			}); err != nil {
				s.Logger.Warn().Err(err).Msg("failed to register CNAME in Private DNS")
			}
		}
		return nil
	}

	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID == net.ID && ep.IPAddress != "" {
			if err := s.NetworkDiscovery.RegisterContainer(s.ctx(), net.ID, hostname, containerID, &core.CloudEndpoint{
				IPAddress: ep.IPAddress,
			}); err != nil {
				s.Logger.Warn().Err(err).Msg("failed to register service in Private DNS")
			}
			break
		}
	}

	return nil
}

// NetworkDisconnect disconnects a container from a network and deregisters it.
func (s *Server) NetworkDisconnect(id string, req *api.NetworkDisconnectRequest) error {
	// Deregister from service discovery before disconnecting.
	net, ok := s.Store.ResolveNetwork(id)
	if ok {
		containerID, _ := s.ResolveContainerIDAuto(context.Background(), req.Container)
		if containerID != "" {
			c, cOk := s.ResolveContainerAuto(context.Background(), containerID)
			hostname := ""
			if cOk {
				hostname = strings.TrimPrefix(c.Name, "/")
			}
			// Route through the network-discovery driver. UseApp → CNAME,
			// else A-record.
			if cd, ok := s.NetworkDiscovery.(*azurecommon.PrivateDNSDiscovery); ok {
				if s.config.UseApp {
					_ = cd.DeregisterContainerCNAME(s.ctx(), net.ID, hostname)
				} else {
					_ = cd.DeregisterContainerARecord(s.ctx(), net.ID, hostname)
				}
			} else {
				_ = s.NetworkDiscovery.DeregisterContainer(s.ctx(), net.ID, hostname, containerID)
			}
		}
	}
	return s.BaseServer.NetworkDisconnect(id, req)
}

// NetworkInspect returns details about a network, with container membership
// reported from cloud-truth (the running Apps tagged with this network) rather
// than local synthetic Store state. A stateless backend never owns membership;
// reporting stale stopped containers makes a docker-host client (gitlab-runner)
// loop trying to disconnect "zombie" containers it can't remove.
func (s *Server) NetworkInspect(id string) (*api.Network, error) {
	net, err := s.BaseServer.NetworkInspect(id)
	if err != nil {
		return nil, err
	}
	setNetworkMembership(net, s.cloudNetworkMembers())
	return net, nil
}

// NetworkList lists networks, each with cloud-truth container membership.
func (s *Server) NetworkList(filters map[string][]string) ([]*api.Network, error) {
	nets, err := s.BaseServer.NetworkList(filters)
	if err != nil {
		return nil, err
	}
	members := s.cloudNetworkMembers()
	for _, net := range nets {
		setNetworkMembership(net, members)
	}
	return nets, nil
}

// cloudNetworkMembers returns the running cloud containers (one cloud query) so
// network-membership reporting reflects the cloud, not local Store state.
func (s *Server) cloudNetworkMembers() []api.Container {
	containers, err := s.CloudState.ListContainers(s.ctx(), true, nil)
	if err != nil {
		s.Logger.Debug().Err(err).Msg("network membership: ListContainers failed; reporting no members")
		return nil
	}
	return containers
}

// setNetworkMembership rewrites net.Containers to exactly the running containers
// whose NetworkSettings reference this network (by name or ID).
func setNetworkMembership(net *api.Network, containers []api.Container) {
	if net == nil {
		return
	}
	members := make(map[string]api.EndpointResource)
	for _, c := range containers {
		if !c.State.Running {
			continue
		}
		for netName, ep := range c.NetworkSettings.Networks {
			if netName != net.Name && (ep == nil || ep.NetworkID != net.ID) {
				continue
			}
			res := api.EndpointResource{Name: strings.TrimPrefix(c.Name, "/")}
			if ep != nil {
				res.EndpointID = ep.EndpointID
				res.IPv4Address = ep.IPAddress
				res.MacAddress = ep.MacAddress
			}
			members[c.ID] = res
			break
		}
	}
	net.Containers = members
}

// NetworkPrune prunes unused networks.
func (s *Server) NetworkPrune(filters map[string][]string) (*api.NetworkPruneResponse, error) {
	return s.BaseServer.NetworkPrune(filters)
}
