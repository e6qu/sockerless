// Host-aliases network-discovery driver.
//
// Tracks (network, name) → endpoint mappings in process memory.
// The backend reads the mapping at workload-host materialize time and
// emits SOCKERLESS_HOST_ALIASES on the container so the bootstrap can
// write /etc/hosts entries. ResolveName is the read path used by the
// backend's container-create translator.
//
// Single-process scope: this driver is appropriate when sockerless's
// own backend process owns the network's containers (typical for the
// runner-task-as-backend deployment). For cross-process discovery, use
// cloud-dns or service-mesh.

package core

import (
	"context"
	"sync"

	"github.com/sockerless/api"
)

// HostAliasesDiscovery is a process-local registry of container endpoints
// keyed by (networkID, name). Suitable when peer containers live in the
// same backend instance (Cloud Run multi-container revision, ECS task
// with multiple containers, ACA app with sidecars).
type HostAliasesDiscovery struct {
	mu       sync.RWMutex
	registry map[string]map[string]*CloudEndpoint // networkID → name → endpoint
}

// NewHostAliasesDiscovery returns a fresh in-process registry.
func NewHostAliasesDiscovery() *HostAliasesDiscovery {
	return &HostAliasesDiscovery{registry: map[string]map[string]*CloudEndpoint{}}
}

func (h *HostAliasesDiscovery) RegisterContainer(_ context.Context, networkID, name, containerID string, endpoint *CloudEndpoint) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.registry[networkID] == nil {
		h.registry[networkID] = map[string]*CloudEndpoint{}
	}
	h.registry[networkID][name] = endpoint
	return nil
}

func (h *HostAliasesDiscovery) DeregisterContainer(_ context.Context, networkID, name, containerID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if peers, ok := h.registry[networkID]; ok {
		delete(peers, name)
		if len(peers) == 0 {
			delete(h.registry, networkID)
		}
	}
	return nil
}

func (h *HostAliasesDiscovery) ResolveName(_ context.Context, networkID, name string) (*CloudEndpoint, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if peers, ok := h.registry[networkID]; ok {
		if ep, ok := peers[name]; ok {
			return ep, nil
		}
	}
	return nil, nil
}

// PeersOnNetwork returns name → endpoint for every registered peer on
// `networkID`. Backends call this at materialize-time to build the
// SOCKERLESS_HOST_ALIASES env value for a container being added to the
// network. Returns a snapshot — caller may freely modify the map.
func (h *HostAliasesDiscovery) PeersOnNetwork(networkID string) map[string]*CloudEndpoint {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := map[string]*CloudEndpoint{}
	for name, ep := range h.registry[networkID] {
		out[name] = ep
	}
	return out
}

func (h *HostAliasesDiscovery) Kind() api.NetworkDiscoveryKind {
	return api.NetworkDiscoveryHostAliases
}

func init() {
	RegisterNetworkDiscoveryDriver(api.NetworkDiscoveryHostAliases, func(deps map[string]any) (NetworkDiscoveryDriver, error) {
		return NewHostAliasesDiscovery(), nil
	})
}
