package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// SyntheticNetworkDriver implements api.NetworkDriver using in-memory store.
// This is the default driver on all platforms.
type SyntheticNetworkDriver struct {
	Store   *Store
	IPAlloc *IPAllocator
}

func (d *SyntheticNetworkDriver) Name() string { return "synthetic" }

func (d *SyntheticNetworkDriver) Create(_ context.Context, name string, opts *api.NetworkCreateRequest) (*api.NetworkCreateResponse, error) {
	if name == "" {
		return nil, fmt.Errorf("network name is required")
	}

	// Check for duplicate name
	for _, n := range d.Store.Networks.List() {
		if n.Name == name {
			return nil, fmt.Errorf("network with name %s already exists", name)
		}
	}

	id := GenerateID()
	driver := opts.Driver
	if driver == "" {
		driver = "bridge"
	}

	// IPAM allocation
	ipam := api.IPAM{Driver: "default"}
	if opts.IPAM != nil {
		ipam = *opts.IPAM
	}
	if len(ipam.Config) == 0 {
		cfg := d.IPAlloc.AllocateSubnet(id, nil)
		ipam.Config = []api.IPAMConfig{cfg}
	} else {
		d.IPAlloc.AllocateSubnet(id, &ipam.Config[0])
	}

	labels := opts.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	options := opts.Options
	if options == nil {
		options = make(map[string]string)
	}

	network := api.Network{
		Name:       name,
		ID:         id,
		Created:    time.Now().UTC().Format(time.RFC3339Nano),
		Scope:      "local",
		Driver:     driver,
		EnableIPv6: opts.EnableIPv6,
		IPAM:       ipam,
		Internal:   opts.Internal,
		Attachable: opts.Attachable,
		Ingress:    opts.Ingress,
		Containers: make(map[string]api.EndpointResource),
		Options:    options,
		Labels:     labels,
	}

	d.Store.Networks.Put(id, network)
	return &api.NetworkCreateResponse{ID: id}, nil
}

func (d *SyntheticNetworkDriver) Inspect(_ context.Context, ref string) (*api.Network, error) {
	n, ok := d.Store.ResolveNetwork(ref)
	if !ok {
		return nil, fmt.Errorf("network %s not found", ref)
	}
	return &n, nil
}

func (d *SyntheticNetworkDriver) List(_ context.Context, filters map[string][]string) ([]*api.Network, error) {
	var result []*api.Network
	for _, n := range d.Store.Networks.List() {
		if !MatchNetworkFilters(n, filters) {
			continue
		}
		n := n
		result = append(result, &n)
	}
	if result == nil {
		result = []*api.Network{}
	}
	return result, nil
}

func (d *SyntheticNetworkDriver) Remove(_ context.Context, ref string) error {
	n, ok := d.Store.ResolveNetwork(ref)
	if !ok {
		return fmt.Errorf("network %s not found", ref)
	}
	if n.Name == "bridge" || n.Name == "host" || n.Name == "none" {
		return fmt.Errorf("%s is a pre-defined network and cannot be removed", n.Name)
	}
	d.Store.Networks.Delete(n.ID)
	d.IPAlloc.ReleaseSubnet(n.ID)
	return nil
}

func (d *SyntheticNetworkDriver) Connect(_ context.Context, networkID, containerID string, config *api.EndpointSettings) error {
	net, ok := d.Store.ResolveNetwork(networkID)
	if !ok {
		return fmt.Errorf("network %s not found", networkID)
	}

	ip, prefixLen, gateway, mac := d.IPAlloc.AllocateIP(net.ID)

	endpoint := api.EndpointSettings{
		NetworkID:   net.ID,
		EndpointID:  GenerateID()[:16],
		Gateway:     gateway,
		IPAddress:   ip,
		IPPrefixLen: prefixLen,
		MacAddress:  mac,
	}

	if config != nil {
		if config.IPAddress != "" {
			endpoint.IPAddress = config.IPAddress
		}
		if len(config.Aliases) > 0 {
			endpoint.Aliases = config.Aliases
		}
	}

	// Add container to network's Containers map
	c, _ := d.Store.Containers.Get(containerID)
	d.Store.Networks.Update(net.ID, func(n *api.Network) {
		n.Containers[containerID] = api.EndpointResource{
			Name:        strings.TrimPrefix(c.Name, "/"),
			EndpointID:  endpoint.EndpointID,
			MacAddress:  endpoint.MacAddress,
			IPv4Address: endpoint.IPAddress + fmt.Sprintf("/%d", endpoint.IPPrefixLen),
		}
	})

	// Add network to container's NetworkSettings
	d.Store.Containers.Update(containerID, func(c *api.Container) {
		c.NetworkSettings.Networks[net.Name] = &endpoint
	})

	return nil
}

func (d *SyntheticNetworkDriver) Disconnect(_ context.Context, networkID, containerID string) error {
	net, ok := d.Store.ResolveNetwork(networkID)
	if !ok {
		return fmt.Errorf("network %s not found", networkID)
	}

	// Get the container's IP before removing, so we can release it
	c, _ := d.Store.Containers.Get(containerID)
	if ep, ok := c.NetworkSettings.Networks[net.Name]; ok && ep != nil {
		d.IPAlloc.ReleaseIP(net.ID, ep.IPAddress)
	}

	d.Store.Networks.Update(net.ID, func(n *api.Network) {
		delete(n.Containers, containerID)
	})

	d.Store.Containers.Update(containerID, func(c *api.Container) {
		delete(c.NetworkSettings.Networks, net.Name)
	})

	return nil
}

func (d *SyntheticNetworkDriver) Prune(_ context.Context, filters map[string][]string) (*api.NetworkPruneResponse, error) {
	pruned := d.Store.Networks.PruneIf(func(_ string, n api.Network) bool {
		if n.Name == "bridge" || n.Name == "host" || n.Name == "none" {
			return false
		}
		if len(n.Containers) > 0 {
			return false
		}
		return MatchNetworkPruneFilters(n, filters)
	})

	deleted := make([]string, 0, len(pruned))
	for _, n := range pruned {
		d.IPAlloc.ReleaseSubnet(n.ID)
		deleted = append(deleted, n.ID)
	}
	return &api.NetworkPruneResponse{NetworksDeleted: deleted}, nil
}
