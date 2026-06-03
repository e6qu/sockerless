package azure_sdk_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// entraGroup mirrors the EntraGroup struct in the sim for seeding.
type entraGroup struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// entraUserSeed is the request body for PUT /sim/v1/entra/users/{oid}.
type entraUserSeed struct {
	OID               string       `json:"oid"`
	Sub               string       `json:"sub"`
	PreferredUsername string       `json:"preferredUsername"`
	Name              string       `json:"name"`
	Email             string       `json:"email,omitempty"`
	Groups            []entraGroup `json:"groups"`
}

func seedEntraUser(t *testing.T, oid string, user entraUserSeed) {
	t.Helper()
	user.OID = oid
	body, err := json.Marshal(user)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, baseURL+"/sim/v1/entra/users/"+oid, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func deleteEntraUser(t *testing.T, oid string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, baseURL+"/sim/v1/entra/users/"+oid, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
}

// doEntraAuthCodeFlow runs the PKCE authorization code flow and returns the
// decoded id_token payload.
func doEntraAuthCodeFlow(t *testing.T, tenant, clientID string) map[string]any {
	t.Helper()
	redirectURI := "http://127.0.0.1/callback"
	verifier := "ThisIsntRandomButItNeedsToBe43CharactersLong"
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])

	authURL := baseURL + "/" + tenant + "/oauth2/v2.0/authorize?" + url.Values{
		"client_id":             {clientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"scope":                 {"openid profile email"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()
	noRedirect := http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	authResp, err := noRedirect.Get(authURL)
	require.NoError(t, err)
	authResp.Body.Close()
	require.Equal(t, http.StatusFound, authResp.StatusCode)
	callback, err := url.Parse(authResp.Header.Get("Location"))
	require.NoError(t, err)
	code := callback.Query().Get("code")
	require.NotEmpty(t, code)

	tokenBody := requestAzureToken(t, tenant, "oauth2/v2.0/token", url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	})
	require.NotEmpty(t, tokenBody.IDToken)
	return azureTokenPayload(t, tokenBody.IDToken)
}

func TestEntra_IDTokenGroupsClaim(t *testing.T) {
	oid := "entra-groups-oid"
	seedEntraUser(t, oid, entraUserSeed{
		Sub:               "entra-groups-sub",
		PreferredUsername: "groups-user@example.com",
		Name:              "Groups User",
		Groups: []entraGroup{
			{ID: "group-admin-id", DisplayName: "Admins"},
			{ID: "group-dev-id", DisplayName: "Developers"},
		},
	})
	defer deleteEntraUser(t, oid)

	payload := doEntraAuthCodeFlow(t, "tenant-entra-groups", "client-entra-groups")

	assert.Equal(t, oid, payload["oid"])
	assert.Equal(t, "entra-groups-sub", payload["sub"])
	assert.Equal(t, "Groups User", payload["name"])
	groupsRaw, ok := payload["groups"]
	require.True(t, ok, "id_token must contain groups claim")
	groups, ok := groupsRaw.([]any)
	require.True(t, ok)
	require.Len(t, groups, 2)
	assert.Contains(t, groups, "group-admin-id")
	assert.Contains(t, groups, "group-dev-id")
}

func TestEntra_IDTokenNoGroupsClaimWhenUnseeded(t *testing.T) {
	// Ensure the default user (test-oid) has no seeded groups in store.
	deleteEntraUser(t, "test-oid")

	payload := doEntraAuthCodeFlow(t, "tenant-entra-nogroups", "client-entra-nogroups")

	assert.Equal(t, "test-oid", payload["oid"])
	assert.Equal(t, "Sockerless Test User", payload["name"])
	_, hasGroups := payload["groups"]
	assert.False(t, hasGroups, "id_token must not contain groups claim for unseeded default user")
}

func TestEntra_IDTokenUsesSeededIdentity(t *testing.T) {
	oid := "entra-identity-oid"
	seedEntraUser(t, oid, entraUserSeed{
		Sub:               "entra-identity-sub",
		PreferredUsername: "alice@example.com",
		Name:              "Alice",
		Email:             "alice@example.com",
		Groups:            []entraGroup{},
	})
	defer deleteEntraUser(t, oid)

	payload := doEntraAuthCodeFlow(t, "tenant-entra-identity", "client-entra-identity")

	assert.Equal(t, oid, payload["oid"])
	assert.Equal(t, "entra-identity-sub", payload["sub"])
	assert.Equal(t, "Alice", payload["name"])
	assert.Equal(t, "alice@example.com", payload["preferred_username"])
	assert.Equal(t, "alice@example.com", payload["email"])
}

func TestEntra_GraphMemberOf(t *testing.T) {
	oid := "entra-graph-oid"
	seedEntraUser(t, oid, entraUserSeed{
		Sub:               "entra-graph-sub",
		PreferredUsername: "graph-user@example.com",
		Name:              "Graph User",
		Groups: []entraGroup{
			{ID: "grp-engineering", DisplayName: "Engineering"},
			{ID: "grp-platform", DisplayName: "Platform"},
		},
	})
	defer deleteEntraUser(t, oid)

	// Get an access token (oid in the token will be the seeded oid).
	tokenBody := requestAzureToken(t, "tenant-entra-graph", "oauth2/v2.0/token", url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"client-entra-graph"},
		"client_secret": {"secret"},
		"scope":         {"https://graph.microsoft.com/.default"},
	})

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1.0/me/memberOf", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+tokenBody.AccessToken)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Value []struct {
			ODataType   string `json:"@odata.type"`
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
		} `json:"value"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Value, 2)
	ids := make(map[string]string)
	for _, g := range body.Value {
		assert.Equal(t, "#microsoft.graph.group", g.ODataType)
		ids[g.ID] = g.DisplayName
	}
	assert.Equal(t, "Engineering", ids["grp-engineering"])
	assert.Equal(t, "Platform", ids["grp-platform"])
}

func TestEntra_GraphTransitiveMemberOf(t *testing.T) {
	oid := "entra-transitive-oid"
	seedEntraUser(t, oid, entraUserSeed{
		Sub:               "entra-transitive-sub",
		PreferredUsername: "transitive@example.com",
		Name:              "Transitive User",
		Groups:            []entraGroup{{ID: "grp-infra", DisplayName: "Infra"}},
	})
	defer deleteEntraUser(t, oid)

	tokenBody := requestAzureToken(t, "tenant-entra-transitive", "oauth2/v2.0/token", url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"client-entra-transitive"},
		"client_secret": {"secret"},
		"scope":         {"https://graph.microsoft.com/.default"},
	})

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1.0/me/transitiveMemberOf", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+tokenBody.AccessToken)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Value []struct {
			ID string `json:"id"`
		} `json:"value"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Value, 1)
	assert.Equal(t, "grp-infra", body.Value[0].ID)
}

func TestEntra_DiscoveryAdvertisesGroupsClaim(t *testing.T) {
	resp, err := http.Get(baseURL + "/tenant-groups-discovery/.well-known/openid-configuration")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		ClaimsSupported []string `json:"claims_supported"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body.ClaimsSupported, "groups")
}
