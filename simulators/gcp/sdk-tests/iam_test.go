package gcp_sdk_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iam/v1"
	iamcredentials "google.golang.org/api/iamcredentials/v1"
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

// iamCredentialsService points the iamcredentials SDK at the sim. The
// :generateIdToken endpoint shares the same sim handler as
// :generateAccessToken — both routed via the {emailAction} path-param
// switch in simulators/gcp/iam.go.
func iamCredentialsService(t *testing.T) *iamcredentials.Service {
	t.Helper()
	svc, err := iamcredentials.NewService(ctx,
		option.WithEndpoint(baseURL),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)
	return svc
}

// TestIAMCredentials_GenerateIdToken — Access driver's id-token category.
// Cross-Service auth chains call generateIdToken with a target audience
// (the receiving service's URL); the mint returns a JWT whose `aud`
// claim equals that audience. The sim signs with HS256 against a per-
// process key so SDKs that pre-decode the token accept its structure;
// downstream sim handlers don't validate the signature.
func TestIAMCredentials_GenerateIdToken(t *testing.T) {
	iamSvc := iamService(t)
	credSvc := iamCredentialsService(t)

	created, err := iamSvc.Projects.ServiceAccounts.Create("projects/test-project",
		&iam.CreateServiceAccountRequest{
			AccountId: "id-token-sa",
			ServiceAccount: &iam.ServiceAccount{
				DisplayName: "ID-token SA",
			},
		}).Do()
	require.NoError(t, err)

	resp, err := credSvc.Projects.ServiceAccounts.GenerateIdToken(created.Name,
		&iamcredentials.GenerateIdTokenRequest{
			Audience:     "https://target-svc.run.app",
			IncludeEmail: true,
		}).Do()
	require.NoError(t, err)
	require.NotEmpty(t, resp.Token, "expected non-empty ID token")

	// JWT has 3 base64url segments; the middle is the claims set.
	parts := strings.Split(resp.Token, ".")
	require.Len(t, parts, 3, "ID token must be a 3-segment JWT")

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var claims map[string]any
	require.NoError(t, json.Unmarshal(payload, &claims))
	assert.Equal(t, "https://target-svc.run.app", claims["aud"], "aud claim must match request audience")
	assert.Equal(t, created.Email, claims["sub"], "sub claim must match SA email")
	assert.Equal(t, created.Email, claims["email"], "email claim must be present when includeEmail=true")
}

func TestIAMCredentials_GenerateIdToken_RejectsUnknownSA(t *testing.T) {
	credSvc := iamCredentialsService(t)
	_, err := credSvc.Projects.ServiceAccounts.GenerateIdToken(
		"projects/test-project/serviceAccounts/missing@test-project.iam.gserviceaccount.com",
		&iamcredentials.GenerateIdTokenRequest{Audience: "https://x"},
	).Do()
	require.Error(t, err)
}

func TestIAMCredentials_GenerateIdToken_RequiresAudience(t *testing.T) {
	iamSvc := iamService(t)
	credSvc := iamCredentialsService(t)

	created, err := iamSvc.Projects.ServiceAccounts.Create("projects/test-project",
		&iam.CreateServiceAccountRequest{
			AccountId:      "no-aud-sa",
			ServiceAccount: &iam.ServiceAccount{},
		}).Do()
	require.NoError(t, err)

	_, err = credSvc.Projects.ServiceAccounts.GenerateIdToken(created.Name,
		&iamcredentials.GenerateIdTokenRequest{}, // no audience
	).Do()
	require.Error(t, err)
}
