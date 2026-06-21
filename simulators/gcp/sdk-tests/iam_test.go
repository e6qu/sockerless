package gcp_sdk_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/cloudresourcemanager/v1"
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

// TestIAM_ListServiceAccountsPagination covers the pageSize/pageToken support
// ListServiceAccounts previously dropped (it returned the whole set, no token).
func TestIAM_ListServiceAccountsPagination(t *testing.T) {
	svc := iamService(t)
	const project = "projects/iam-page-project"
	for _, id := range []string{"sa-a", "sa-b", "sa-c"} {
		_, err := svc.Projects.ServiceAccounts.Create(project,
			&iam.CreateServiceAccountRequest{
				AccountId:      id,
				ServiceAccount: &iam.ServiceAccount{DisplayName: id},
			}).Do()
		require.NoError(t, err)
	}

	page1, err := svc.Projects.ServiceAccounts.List(project).PageSize(2).Do()
	require.NoError(t, err)
	require.Len(t, page1.Accounts, 2)
	require.NotEmpty(t, page1.NextPageToken, "pageSize=2 over 3 accounts → NextPageToken")

	page2, err := svc.Projects.ServiceAccounts.List(project).PageSize(2).PageToken(page1.NextPageToken).Do()
	require.NoError(t, err)
	assert.Len(t, page2.Accounts, 1)
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

func TestIAM_ServiceAccountKeysCRUD(t *testing.T) {
	svc := iamService(t)

	// Create a parent service account.
	sa, err := svc.Projects.ServiceAccounts.Create("projects/sa-keys-project",
		&iam.CreateServiceAccountRequest{
			AccountId:      "key-owner-sa",
			ServiceAccount: &iam.ServiceAccount{DisplayName: "Key Owner"},
		}).Do()
	require.NoError(t, err)

	// Create a key — response must include privateKeyData.
	key, err := svc.Projects.ServiceAccounts.Keys.Create(sa.Name,
		&iam.CreateServiceAccountKeyRequest{}).Do()
	require.NoError(t, err)
	require.NotEmpty(t, key.Name, "key name must be set")
	require.NotEmpty(t, key.PrivateKeyData, "privateKeyData must be non-empty on create")
	assert.Equal(t, "KEY_ALG_RSA_2048", key.KeyAlgorithm)

	// Decode privateKeyData: must be base64-encoded JSON with correct fields.
	raw, err := base64.StdEncoding.DecodeString(key.PrivateKeyData)
	require.NoError(t, err)
	var keyFile map[string]string
	require.NoError(t, json.Unmarshal(raw, &keyFile))
	assert.Equal(t, "service_account", keyFile["type"])
	assert.NotEmpty(t, keyFile["private_key"])
	assert.Equal(t, "sa-keys-project", keyFile["project_id"])

	// Extract key ID from the resource name (…/keys/{keyId}).
	keyID := key.Name[strings.LastIndex(key.Name, "/")+1:]

	// Get key — no privateKeyData.
	got, err := svc.Projects.ServiceAccounts.Keys.Get(key.Name).Do()
	require.NoError(t, err)
	assert.Equal(t, key.Name, got.Name)
	assert.Empty(t, got.PrivateKeyData, "GET must not return privateKeyData")

	// List keys — must include the created key.
	list, err := svc.Projects.ServiceAccounts.Keys.List(sa.Name).Do()
	require.NoError(t, err)
	found := false
	for _, k := range list.Keys {
		if strings.HasSuffix(k.Name, "/"+keyID) {
			found = true
		}
	}
	assert.True(t, found, "created key must appear in list")

	// Delete the key.
	_, err = svc.Projects.ServiceAccounts.Keys.Delete(key.Name).Do()
	require.NoError(t, err)

	// Get after delete must fail.
	_, err = svc.Projects.ServiceAccounts.Keys.Get(key.Name).Do()
	require.Error(t, err)
}

func crmService(t *testing.T) *cloudresourcemanager.Service {
	t.Helper()
	svc, err := cloudresourcemanager.NewService(ctx,
		option.WithEndpoint(baseURL),
		option.WithoutAuthentication(),
	)
	require.NoError(t, err)
	return svc
}

func TestIAM_ProjectIAMPolicy(t *testing.T) {
	svc := crmService(t)
	project := "iam-policy-project"

	// GetIamPolicy on a fresh project returns empty bindings.
	policy, err := svc.Projects.GetIamPolicy(project,
		&cloudresourcemanager.GetIamPolicyRequest{}).Do()
	require.NoError(t, err)
	assert.NotEmpty(t, policy.Etag)

	// SetIamPolicy — add a binding.
	updated, err := svc.Projects.SetIamPolicy(project,
		&cloudresourcemanager.SetIamPolicyRequest{
			Policy: &cloudresourcemanager.Policy{
				Bindings: []*cloudresourcemanager.Binding{
					{
						Role:    "roles/viewer",
						Members: []string{"serviceAccount:robot@iam-policy-project.iam.gserviceaccount.com"},
					},
				},
			},
		}).Do()
	require.NoError(t, err)
	require.Len(t, updated.Bindings, 1)
	assert.Equal(t, "roles/viewer", updated.Bindings[0].Role)

	// GetIamPolicy must reflect the set policy.
	got, err := svc.Projects.GetIamPolicy(project,
		&cloudresourcemanager.GetIamPolicyRequest{}).Do()
	require.NoError(t, err)
	require.Len(t, got.Bindings, 1)
	assert.Equal(t, "roles/viewer", got.Bindings[0].Role)
	assert.Contains(t, got.Bindings[0].Members, "serviceAccount:robot@iam-policy-project.iam.gserviceaccount.com")
}
