//go:build linux

package core

import (
	"context"
	"os"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

// LinuxNetworkDriver adds real network namespace operations on top of
// synthetic networking. When netns is `Available()` (kernel + capabilities
// permit), creation must succeed — BUG-823: previously a netns
// CreateNamespace / CreateVethPair failure was logged as a warning and
// the driver silently continued in synthetic-only mode, which produced
// containers that thought they had real networking but actually didn't
// (cross-container DNS / port bindings broke without an obvious cause).
// Now any netns failure surfaces as a real error and the network create
// rolls back. Operators who want best-effort degradation can opt in via
// `SOCKERLESS_NETNS_BEST_EFFORT=1`, in which case a warning is logged and
// the synthetic-only path is used — but only when explicitly chosen.
type LinuxNetworkDriver struct {
	Synthetic *SyntheticNetworkDriver
	Netns     *NetnsManager
	Logger    zerolog.Logger
}

func (d *LinuxNetworkDriver) Name() string { return "linux" }

func (d *LinuxNetworkDriver) Create(ctx context.Context, name string, opts *api.NetworkCreateRequest) (*api.NetworkCreateResponse, error) {
	resp, err := d.Synthetic.Create(ctx, name, opts)
	if err != nil {
		return nil, err
	}

	if d.Netns.Available() {
		// Resolve the network to get IPAM config
		net, ok := d.Synthetic.Store.Networks.Get(resp.ID)
		if ok && len(net.IPAM.Config) > 0 {
			gateway := net.IPAM.Config[0].Gateway
			subnet := net.IPAM.Config[0].Subnet
			if nsErr := d.Netns.CreateNamespace(resp.ID, name, gateway, subnet); nsErr != nil {
				if os.Getenv("SOCKERLESS_NETNS_BEST_EFFORT") == "1" {
					d.Logger.Warn().Err(nsErr).Str("network", name).Msg("netns creation failed, falling back to synthetic networking (SOCKERLESS_NETNS_BEST_EFFORT=1)")
				} else {
					// Roll back the synthetic record so the caller
					// doesn't see a half-created network.
					_ = d.Synthetic.Remove(ctx, resp.ID)
					return nil, &api.ServerError{Message: "netns creation failed: " + nsErr.Error() + " — set SOCKERLESS_NETNS_BEST_EFFORT=1 to fall back to synthetic networking"}
				}
			}
		}
	}

	return resp, nil
}

func (d *LinuxNetworkDriver) Inspect(ctx context.Context, ref string) (*api.Network, error) {
	return d.Synthetic.Inspect(ctx, ref)
}

func (d *LinuxNetworkDriver) List(ctx context.Context, filters map[string][]string) ([]*api.Network, error) {
	return d.Synthetic.List(ctx, filters)
}

func (d *LinuxNetworkDriver) Remove(ctx context.Context, ref string) error {
	// Get network ID before removing
	net, ok := d.Synthetic.Store.ResolveNetwork(ref)
	if ok && d.Netns.Available() {
		if nsErr := d.Netns.DeleteNamespace(net.ID); nsErr != nil {
			d.Logger.Warn().Err(nsErr).Str("network", net.Name).Msg("netns deletion failed")
		}
	}
	return d.Synthetic.Remove(ctx, ref)
}

func (d *LinuxNetworkDriver) Connect(ctx context.Context, networkID, containerID string, config *api.EndpointSettings) error {
	if err := d.Synthetic.Connect(ctx, networkID, containerID, config); err != nil {
		return err
	}

	if d.Netns.Available() {
		// Get the allocated IP from the container's endpoint
		c, _ := d.Synthetic.Store.Containers.Get(containerID)
		net, ok := d.Synthetic.Store.ResolveNetwork(networkID)
		if ok {
			if ep, epOk := c.NetworkSettings.Networks[net.Name]; epOk && ep != nil {
				if nsErr := d.Netns.CreateVethPair(net.ID, containerID, ep.IPAddress); nsErr != nil {
					if os.Getenv("SOCKERLESS_NETNS_BEST_EFFORT") == "1" {
						d.Logger.Warn().Err(nsErr).Str("container", containerID).Msg("veth creation failed, falling back to synthetic networking (SOCKERLESS_NETNS_BEST_EFFORT=1)")
					} else {
						// Roll back the synthetic Connect so the caller
						// doesn't think the container is on the network.
						_ = d.Synthetic.Disconnect(ctx, networkID, containerID)
						return &api.ServerError{Message: "veth creation failed: " + nsErr.Error() + " — set SOCKERLESS_NETNS_BEST_EFFORT=1 to fall back to synthetic networking"}
					}
				}
			}
		}
	}

	return nil
}

func (d *LinuxNetworkDriver) Disconnect(ctx context.Context, networkID, containerID string) error {
	if d.Netns.Available() {
		net, ok := d.Synthetic.Store.ResolveNetwork(networkID)
		if ok {
			if nsErr := d.Netns.RemoveVethPair(net.ID, containerID); nsErr != nil {
				d.Logger.Warn().Err(nsErr).Str("container", containerID).Msg("veth removal failed")
			}
		}
	}
	return d.Synthetic.Disconnect(ctx, networkID, containerID)
}

func (d *LinuxNetworkDriver) Prune(ctx context.Context, filters map[string][]string) (*api.NetworkPruneResponse, error) {
	// Get networks that will be pruned, to clean up netns
	if d.Netns.Available() {
		networks, _ := d.Synthetic.List(ctx, nil)
		for _, n := range networks {
			if n.Name == "bridge" || n.Name == "host" || n.Name == "none" {
				continue
			}
			if len(n.Containers) == 0 && MatchNetworkPruneFilters(*n, filters) {
				_ = d.Netns.DeleteNamespace(n.ID)
			}
		}
	}
	return d.Synthetic.Prune(ctx, filters)
}

// NewPlatformNetworkDriver creates a Linux-aware network driver wrapping the synthetic driver.
func NewPlatformNetworkDriver(synthetic *SyntheticNetworkDriver, logger zerolog.Logger) api.NetworkDriver {
	netns := NewNetnsManager()
	return &LinuxNetworkDriver{
		Synthetic: synthetic,
		Netns:     netns,
		Logger:    logger,
	}
}
