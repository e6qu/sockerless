// DNS driver registry.
//
// Sibling to NetworkDiscoveryDriver. The discovery driver answers
// "which container holds which name" (registration); this dimension
// answers "how does the workload's resolver find them at runtime"
// — primarily by configuring the DNS search domain on the workload's
// /etc/resolv.conf and naming the cloud-product mechanism the suffix
// depends on.

package core

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/sockerless/api"
)

// DNSDriver provides DNS-related metadata for workloads on a network.
//
// SearchDomain is the suffix that should be appended to /etc/resolv.conf
// `search` so workloads can resolve `<peer-name>` (no FQDN) within the
// network.
//
// Mechanism reports the cloud-product-specific resolver mechanism so
// callers can introspect (telemetry / debugging / sim-parity matrix
// reporting). Backends that don't need DNS (single-container Lambda,
// AZF) return DNSMechanismNone.
type DNSDriver interface {
	SearchDomain(ctx context.Context, networkID string) (string, error)
	Mechanism() api.DNSMechanism
}

// DNSConstructor builds a driver from a backend-specific deps map.
type DNSConstructor func(deps map[string]any) (DNSDriver, error)

// dns driver registry — populated by per-backend init() or by direct
// construction at backend startup.
var (
	dnsRegistryMu sync.RWMutex
	dnsRegistry   = map[api.DNSMechanism]DNSConstructor{}
)

// RegisterDNSDriver makes a driver available under its mechanism name.
func RegisterDNSDriver(m api.DNSMechanism, ctor DNSConstructor) {
	dnsRegistryMu.Lock()
	defer dnsRegistryMu.Unlock()
	dnsRegistry[m] = ctor
}

// ResolveDNSDriver looks up the constructor for `m` and builds a
// driver. Empty/unknown mechanism → error (no fallback).
func ResolveDNSDriver(m api.DNSMechanism, deps map[string]any) (DNSDriver, error) {
	if !m.IsValid() {
		return nil, fmt.Errorf("dns driver: invalid mechanism %q (one of %v required)",
			m, api.AllDNSMechanisms)
	}
	dnsRegistryMu.RLock()
	ctor, ok := dnsRegistry[m]
	dnsRegistryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("dns driver: mechanism %q is valid but no constructor registered", m)
	}
	return ctor(deps)
}

// NoOpDNS is the default — no search domain injected, mechanism=none.
type NoOpDNS struct{}

func (NoOpDNS) SearchDomain(context.Context, string) (string, error) { return "", nil }
func (NoOpDNS) Mechanism() api.DNSMechanism                          { return api.DNSMechanismNone }

func init() {
	RegisterDNSDriver(api.DNSMechanismNone, func(deps map[string]any) (DNSDriver, error) {
		return NoOpDNS{}, nil
	})
}

// ParseDNSMechanismEnv reads the operator's chosen mechanism from an
// env var (typically SOCKERLESS_<BACKEND>_DNS_MECHANISM) and falls
// back to the backend's default when unset. Empty value uses default;
// unknown returns an error (no fallback).
func ParseDNSMechanismEnv(envValue string, backendDefault api.DNSMechanism) (api.DNSMechanism, error) {
	v := strings.TrimSpace(envValue)
	if v == "" {
		if !backendDefault.IsValid() {
			return "", fmt.Errorf("dns driver: backend default %q is invalid", backendDefault)
		}
		return backendDefault, nil
	}
	m := api.DNSMechanism(v)
	if !m.IsValid() {
		return "", fmt.Errorf("dns driver: env value %q is invalid (one of %v required)",
			v, api.AllDNSMechanisms)
	}
	return m, nil
}

// DNSSearchDomainEnvName is the env var the workload-host materializer
// sets on each container so the bootstrap can write the /etc/resolv.conf
// `search` line at startup. Empty / unset → bootstrap leaves resolv.conf
// untouched.
const DNSSearchDomainEnvName = "SOCKERLESS_DNS_SEARCH_DOMAIN"

// DNSSearchDomainEnvIfSet returns the KEY=VALUE env entry to append on
// the workload container, or "" if the suffix is empty (no-op) or
// userEnv already carries the var (operator override wins).
func DNSSearchDomainEnvIfSet(userEnv []string, suffix string) string {
	if suffix == "" {
		return ""
	}
	if hasEnv(userEnv, DNSSearchDomainEnvName) {
		return ""
	}
	return DNSSearchDomainEnvName + "=" + suffix
}
