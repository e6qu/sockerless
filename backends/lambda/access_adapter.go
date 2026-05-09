// iam-role access driver for the Lambda backend. AWS IAM is enforced
// at the SDK layer via SigV4; the caller-side http.Client returned
// here is only used for non-SDK HTTP paths. The workload principal is
// the function execution role attached to every Lambda function.

package lambda

import (
	"context"
	"net/http"

	"github.com/sockerless/api"
)

type iamRoleAccess struct {
	s *Server
}

func newIAMRoleAccess(s *Server) *iamRoleAccess {
	return &iamRoleAccess{s: s}
}

func (a *iamRoleAccess) Mechanism() api.AccessMechanism { return api.AccessMechanismIAMRole }

func (a *iamRoleAccess) WorkloadPrincipal() string {
	return a.s.config.RoleARN
}

func (a *iamRoleAccess) AuthenticatedClient(context.Context, string) (*http.Client, error) {
	return http.DefaultClient, nil
}
