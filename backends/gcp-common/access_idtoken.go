// id-token access driver shared by every GCP-product backend.
// Wraps google.golang.org/api/idtoken: each AuthenticatedClient call
// mints + auto-attaches a short-lived Google ID token whose audience
// matches the workload URL. Cloud Run / Cloud Functions reject
// unauthenticated invokes by default.
//
// Per-backend wiring: backend startup constructs with the per-backend
// service-account email (from SOCKERLESS_<BACKEND>_SERVICE_ACCOUNT)
// and registers as the backend's core.AccessDriver.

package gcpcommon

import (
	"context"
	"fmt"
	"net/http"

	"github.com/sockerless/api"
	"google.golang.org/api/idtoken"
)

// IDTokenAccess implements core.AccessDriver for the id-token mechanism.
type IDTokenAccess struct {
	// ServiceAccount is the GCP service-account email the workload
	// runs as (the WorkloadPrincipal). Captured at construction;
	// empty string means platform default.
	ServiceAccount string
}

// NewIDTokenAccess constructs a driver with the operator-supplied
// workload principal. Pass empty string when the backend doesn't
// configure a non-default service account.
func NewIDTokenAccess(serviceAccount string) *IDTokenAccess {
	return &IDTokenAccess{ServiceAccount: serviceAccount}
}

func (a *IDTokenAccess) Mechanism() api.AccessMechanism { return api.AccessMechanismIDToken }

func (a *IDTokenAccess) WorkloadPrincipal() string { return a.ServiceAccount }

func (a *IDTokenAccess) AuthenticatedClient(ctx context.Context, audience string) (*http.Client, error) {
	client, err := idtoken.NewClient(ctx, audience)
	if err != nil {
		return nil, fmt.Errorf("idtoken.NewClient(%s): %w", audience, err)
	}
	return client, nil
}
