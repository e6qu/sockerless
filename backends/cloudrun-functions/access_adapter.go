// id-token access driver for the Cloud Run Functions backend. Wraps
// google.golang.org/api/idtoken: each AuthenticatedClient call mints
// + auto-attaches a short-lived Google ID token whose audience matches
// the Function URL. CRF rejects unauthenticated invokes by default —
// without this Authorization header the POST returns 401/403 before
// the bootstrap sees it.

package gcf

import (
	"context"
	"fmt"
	"net/http"

	"github.com/sockerless/api"
	"google.golang.org/api/idtoken"
)

type idTokenAccess struct {
	s *Server
}

func newIDTokenAccess(s *Server) *idTokenAccess {
	return &idTokenAccess{s: s}
}

func (a *idTokenAccess) Mechanism() api.AccessMechanism { return api.AccessMechanismIDToken }

func (a *idTokenAccess) WorkloadPrincipal() string {
	return a.s.config.ServiceAccount
}

func (a *idTokenAccess) AuthenticatedClient(ctx context.Context, audience string) (*http.Client, error) {
	client, err := idtoken.NewClient(ctx, audience)
	if err != nil {
		return nil, fmt.Errorf("idtoken.NewClient(%s): %w", audience, err)
	}
	return client, nil
}
