// none-internal access driver for the ACA backend. ACA jobs in a
// managed environment are reachable only through the environment's
// internal network — the network layer enforces access control, no
// per-call credential is required. WorkloadPrincipal is empty (Azure
// managed-identity wiring would attach here once the ACA backend
// grows explicit principal config).

package aca

import (
	"context"
	"net/http"

	"github.com/sockerless/api"
)

type noneInternalAccess struct {
	s *Server
}

func newNoneInternalAccess(s *Server) *noneInternalAccess {
	return &noneInternalAccess{s: s}
}

func (a *noneInternalAccess) Mechanism() api.AccessMechanism {
	return api.AccessMechanismNoneInternal
}

func (a *noneInternalAccess) WorkloadPrincipal() string { return "" }

func (a *noneInternalAccess) AuthenticatedClient(context.Context, string) (*http.Client, error) {
	return http.DefaultClient, nil
}
