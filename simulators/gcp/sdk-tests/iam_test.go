package gcp_sdk_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
)

func iamService(t *testing.T) *iam.Service {
	t.Helper()
	svc, err := iam.NewService(ctx,
		option.WithEndpoint(baseURL),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)
	return svc
}

func TestIAM_CreateServiceAccount(t *testing.T) {
	svc := iamService(t)

	sa, err := svc.Projects.ServiceAccounts.Create("projects/test-project",
		&iam.CreateServiceAccountRequest{
			AccountId: "test-sa",
			ServiceAccount: &iam.ServiceAccount{
				DisplayName: "Test Service Account",
			},
		}).Do()
	require.NoError(t, err)
	assert.Contains(t, sa.Email, "test-sa")
	assert.Contains(t, sa.Name, "test-sa")
}

func TestIAM_GetServiceAccount(t *testing.T) {
	svc := iamService(t)

	created, err := svc.Projects.ServiceAccounts.Create("projects/test-project",
		&iam.CreateServiceAccountRequest{
			AccountId: "get-sa",
			ServiceAccount: &iam.ServiceAccount{
				DisplayName: "Get SA",
			},
		}).Do()
	require.NoError(t, err)

	got, err := svc.Projects.ServiceAccounts.Get(created.Name).Do()
	require.NoError(t, err)
	assert.Equal(t, created.Email, got.Email)
}

func TestIAM_ListServiceAccounts(t *testing.T) {
	svc := iamService(t)

	_, err := svc.Projects.ServiceAccounts.Create("projects/test-project",
		&iam.CreateServiceAccountRequest{
			AccountId: "list-sa",
			ServiceAccount: &iam.ServiceAccount{
				DisplayName: "List SA",
			},
		}).Do()
	require.NoError(t, err)

	resp, err := svc.Projects.ServiceAccounts.List("projects/test-project").Do()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(resp.Accounts), 1)
}

func TestIAM_DeleteServiceAccount(t *testing.T) {
	svc := iamService(t)

	created, err := svc.Projects.ServiceAccounts.Create("projects/test-project",
		&iam.CreateServiceAccountRequest{
			AccountId: "del-sa",
			ServiceAccount: &iam.ServiceAccount{
				DisplayName: "Delete SA",
			},
		}).Do()
	require.NoError(t, err)

	_, err = svc.Projects.ServiceAccounts.Delete(created.Name).Do()
	require.NoError(t, err)
}
