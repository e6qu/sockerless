// iam-role access driver shared by every AWS-product backend. AWS IAM
// is enforced at the SDK layer via SigV4; the caller-side http.Client
// returned here is only used for non-SDK HTTP paths. The workload
// principal is the IAM role ARN attached to the task / function.
//
// Per-backend wiring: backend startup constructs with the per-backend
// IAM role ARN and registers as the backend's core.AccessDriver.

package awscommon

import (
	"context"
	"net/http"

	"github.com/sockerless/api"
)

// IAMRoleAccess implements core.AccessDriver for the iam-role mechanism.
type IAMRoleAccess struct {
	// RoleARN is the IAM role ARN the workload runs as (TaskRoleArn
	// for ECS, function execution role for Lambda). Captured at
	// construction; empty string means platform default.
	RoleARN string
}

// NewIAMRoleAccess constructs a driver with the operator-supplied
// workload principal.
func NewIAMRoleAccess(roleARN string) *IAMRoleAccess {
	return &IAMRoleAccess{RoleARN: roleARN}
}

func (a *IAMRoleAccess) Mechanism() api.AccessMechanism { return api.AccessMechanismIAMRole }

func (a *IAMRoleAccess) WorkloadPrincipal() string { return a.RoleARN }

func (a *IAMRoleAccess) AuthenticatedClient(context.Context, string) (*http.Client, error) {
	return http.DefaultClient, nil
}
