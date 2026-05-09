// Network discovery driver registry.
//
// Distinct from CloudNetworkDriver in network_driver.go (which owns
// VPC/subnet/IP allocation). This dimension answers the "name → reachable
// peer" question — how containers in the same user-defined network
// discover each other.

package core

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/sockerless/api"
)

// NetworkDiscoveryDriver maps container-name → reachable endpoint within
// a user-defined network. Implementations:
//   - host-aliases: write /etc/hosts on each container.
//   - cloud-dns:    upsert a per-container A/CNAME record.
//   - service-mesh: register an instance in a cloud service-discovery
//     namespace (e.g. AWS Cloud Map).
//   - nat-gateway-only: no peer-discovery (no-op).
//
// All methods are idempotent: registering the same (network, name) twice
// must succeed; deregistering an already-gone name must succeed; resolving
// an unknown name returns (nil, nil) without error.
type NetworkDiscoveryDriver interface {
	// RegisterContainer makes the container reachable on `networkID`.
	// `name` is the discoverable hostname (peers query it). `containerID`
	// is sockerless's internal container ID (used by adapters like AWS
	// Cloud Map that key instances by container-ID rather than hostname).
	// `endpoint` carries the IP and per-cloud metadata (e.g.
	// metadata["kind"]="cname" for cloudrun's UseService flow).
	RegisterContainer(ctx context.Context, networkID, name, containerID string, endpoint *CloudEndpoint) error

	// DeregisterContainer removes the discoverability entry. Adapters
	// may use either `name` or `containerID` depending on how their
	// underlying cloud primitive keys instances.
	DeregisterContainer(ctx context.Context, networkID, name, containerID string) error

	// ResolveName looks up a peer's endpoint by name. Returns (nil, nil)
	// if the name is unknown; the caller may fall through to other
	// resolution paths (e.g. /etc/resolv.conf).
	ResolveName(ctx context.Context, networkID, name string) (*CloudEndpoint, error)

	// Kind returns the driver's category.
	Kind() api.NetworkDiscoveryKind
}

// NetworkDiscoveryConstructor builds a driver from a backend-specific
// context (cloud client, region, etc.). The deps map is whatever the
// per-backend translator provides; cast inside the constructor.
type NetworkDiscoveryConstructor func(deps map[string]any) (NetworkDiscoveryDriver, error)

// network discovery registry — populated by per-cloud-common packages
// at init() time, looked up at backend startup by name.
var (
	networkDiscoveryRegistryMu sync.RWMutex
	networkDiscoveryRegistry   = map[api.NetworkDiscoveryKind]NetworkDiscoveryConstructor{}
)

// RegisterNetworkDiscoveryDriver makes a driver available under its
// kind name. Per-cloud-common packages call this from their init().
// Re-registration is allowed (last wins) for ease of testing.
func RegisterNetworkDiscoveryDriver(kind api.NetworkDiscoveryKind, ctor NetworkDiscoveryConstructor) {
	networkDiscoveryRegistryMu.Lock()
	defer networkDiscoveryRegistryMu.Unlock()
	networkDiscoveryRegistry[kind] = ctor
}

// ResolveNetworkDiscoveryDriver looks up the constructor for `kind` and
// builds a driver. **No-fallbacks**: empty/unknown kind → error.
// Returns the per-backend default's *constructor*; callers pass their
// deps map (cloud clients etc.).
func ResolveNetworkDiscoveryDriver(kind api.NetworkDiscoveryKind, deps map[string]any) (NetworkDiscoveryDriver, error) {
	if !kind.IsValid() {
		return nil, fmt.Errorf("network-discovery driver: invalid kind %q (one of %v required)",
			kind, api.AllNetworkDiscoveryKinds)
	}
	networkDiscoveryRegistryMu.RLock()
	ctor, ok := networkDiscoveryRegistry[kind]
	networkDiscoveryRegistryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("network-discovery driver: kind %q is valid but no constructor registered (per-cloud-common pkg not imported?)", kind)
	}
	return ctor(deps)
}

// NoOpNetworkDiscovery is the registered constructor for
// `nat-gateway-only` — discovery is intentionally a no-op (containers
// reach the internet via NAT but don't address each other by name).
type NoOpNetworkDiscovery struct{}

func (NoOpNetworkDiscovery) RegisterContainer(ctx context.Context, networkID, name, containerID string, endpoint *CloudEndpoint) error {
	return nil
}
func (NoOpNetworkDiscovery) DeregisterContainer(ctx context.Context, networkID, name, containerID string) error {
	return nil
}
func (NoOpNetworkDiscovery) ResolveName(ctx context.Context, networkID, name string) (*CloudEndpoint, error) {
	return nil, nil
}
func (NoOpNetworkDiscovery) Kind() api.NetworkDiscoveryKind {
	return api.NetworkDiscoveryNATGatewayOnly
}

func init() {
	RegisterNetworkDiscoveryDriver(api.NetworkDiscoveryNATGatewayOnly, func(deps map[string]any) (NetworkDiscoveryDriver, error) {
		return NoOpNetworkDiscovery{}, nil
	})
}

// ParseNetworkDiscoveryEnv reads the operator's chosen kind from an env
// var (typically SOCKERLESS_<BACKEND>_NETWORK_DISCOVERY) and falls back
// to the backend's default when unset. Empty value uses default; unknown
// value returns an error (no fallback to default).
//
// `envValue` is the raw string from os.Getenv; `backendDefault` is the
// per-backend default the call site decides.
func ParseNetworkDiscoveryEnv(envValue string, backendDefault api.NetworkDiscoveryKind) (api.NetworkDiscoveryKind, error) {
	v := strings.TrimSpace(envValue)
	if v == "" {
		if !backendDefault.IsValid() {
			return "", fmt.Errorf("network-discovery driver: backend default %q is invalid", backendDefault)
		}
		return backendDefault, nil
	}
	k := api.NetworkDiscoveryKind(v)
	if !k.IsValid() {
		return "", fmt.Errorf("network-discovery driver: env value %q is invalid (one of %v required)",
			v, api.AllNetworkDiscoveryKinds)
	}
	return k, nil
}
