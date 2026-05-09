// Access driver registry.
//
// Sibling to NetworkDiscoveryDriver and DNSDriver. Models two coupled
// concerns: (1) the cloud-native principal a workload runs as, and
// (2) the per-call credential a caller mints to invoke the workload's
// HTTP endpoint. The driver only owns the caller-side signer plus a
// typed declaration of which mechanism the backend uses; workload-side
// credentials come from cloud metadata services using the bound
// principal.

package core

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/sockerless/api"
)

// AccessDriver provides ingress-auth metadata for a backend.
//
// Mechanism reports the cloud-product mechanism (iam-role / id-token /
// mTLS / none-internal) so callers and telemetry can introspect.
//
// WorkloadPrincipal returns the cloud-native identity the workload runs
// as (IAM role ARN, GCP service-account email, Azure managed-id
// client-id). Empty string means platform default.
//
// AuthenticatedClient returns an *http.Client suitable for invoking the
// workload's endpoint at `audience`. id-token: client mints + attaches
// a short-lived JWT. iam-role: returns http.DefaultClient (SigV4
// happens at SDK layer for AWS-SDK paths). mTLS: returns a client
// preconfigured with mTLS material. none-internal: returns
// http.DefaultClient.
type AccessDriver interface {
	Mechanism() api.AccessMechanism
	WorkloadPrincipal() string
	AuthenticatedClient(ctx context.Context, audience string) (*http.Client, error)
}

// AccessConstructor builds a driver from a backend-specific deps map.
type AccessConstructor func(deps map[string]any) (AccessDriver, error)

var (
	accessRegistryMu sync.RWMutex
	accessRegistry   = map[api.AccessMechanism]AccessConstructor{}
)

// RegisterAccessDriver makes a driver available under its mechanism name.
func RegisterAccessDriver(m api.AccessMechanism, ctor AccessConstructor) {
	accessRegistryMu.Lock()
	defer accessRegistryMu.Unlock()
	accessRegistry[m] = ctor
}

// ResolveAccessDriver looks up the constructor for `m` and builds a
// driver. Empty/unknown mechanism → error (no fallback).
func ResolveAccessDriver(m api.AccessMechanism, deps map[string]any) (AccessDriver, error) {
	if !m.IsValid() {
		return nil, fmt.Errorf("access driver: invalid mechanism %q (one of %v required)",
			m, api.AllAccessMechanisms)
	}
	accessRegistryMu.RLock()
	ctor, ok := accessRegistry[m]
	accessRegistryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("access driver: mechanism %q is valid but no constructor registered", m)
	}
	return ctor(deps)
}

// NoneInternalAccess is the default — no credential, no principal,
// mechanism=none-internal. Used when the workload is reachable only
// over a private VPC.
type NoneInternalAccess struct{}

func (NoneInternalAccess) Mechanism() api.AccessMechanism { return api.AccessMechanismNoneInternal }
func (NoneInternalAccess) WorkloadPrincipal() string      { return "" }
func (NoneInternalAccess) AuthenticatedClient(context.Context, string) (*http.Client, error) {
	return http.DefaultClient, nil
}

func init() {
	RegisterAccessDriver(api.AccessMechanismNoneInternal, func(deps map[string]any) (AccessDriver, error) {
		return NoneInternalAccess{}, nil
	})
}

// ParseAccessMechanismEnv reads the operator's chosen mechanism from an
// env var (typically SOCKERLESS_<BACKEND>_ACCESS_MECHANISM) and falls
// back to the backend's default when unset. Empty value uses default;
// unknown returns an error (no fallback).
func ParseAccessMechanismEnv(envValue string, backendDefault api.AccessMechanism) (api.AccessMechanism, error) {
	v := strings.TrimSpace(envValue)
	if v == "" {
		if !backendDefault.IsValid() {
			return "", fmt.Errorf("access driver: backend default %q is invalid", backendDefault)
		}
		return backendDefault, nil
	}
	m := api.AccessMechanism(v)
	if !m.IsValid() {
		return "", fmt.Errorf("access driver: env value %q is invalid (one of %v required)",
			v, api.AllAccessMechanisms)
	}
	return m, nil
}
