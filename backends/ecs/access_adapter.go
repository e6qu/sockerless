// iam-role access driver for the ECS backend. AWS IAM is enforced at
// the SDK layer via SigV4; the caller-side http.Client returned here
// is only used for non-SDK HTTP paths (currently none). The workload
// principal is the TaskRole ARN attached to every ECS task definition.

package ecs

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
	return a.s.config.TaskRoleARN
}

func (a *iamRoleAccess) AuthenticatedClient(context.Context, string) (*http.Client, error) {
	return http.DefaultClient, nil
}
