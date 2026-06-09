package azure_sdk_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Graph provisioning helpers ---

type graphGroup struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

type graphUser struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	UserPrincipalName string `json:"userPrincipalName"`
}

func createGraphGroup(t *testing.T, displayName string) graphGroup {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"displayName":     displayName,
		"mailNickname":    displayName,
		"securityEnabled": true,
		"mailEnabled":     false,
	})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1.0/groups", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var grp graphGroup
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&grp))
	require.NotEmpty(t, grp.ID)
	t.Cleanup(func() { deleteGraphGroup(t, grp.ID) })
	return grp
}

func deleteGraphGroup(t *testing.T, id string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, baseURL+"/v1.0/groups/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
}

func createGraphUser(t *testing.T, displayName, upn string) graphUser {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"displayName":       displayName,
		"userPrincipalName": upn,
		"mailNickname":      displayName,
		"accountEnabled":    true,
		"passwordProfile": map[string]any{
			"password":                      "Test1234!",
			"forceChangePasswordNextSignIn": false,
		},
	})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1.0/users", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var u graphUser
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&u))
	require.NotEmpty(t, u.ID)
	t.Cleanup(func() { deleteGraphUser(t, u.ID) })
	return u
}

func deleteGraphUser(t *testing.T, id string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, baseURL+"/v1.0/users/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
}

func addGroupMember(t *testing.T, groupID, userID string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"@odata.id": baseURL + "/v1.0/directoryObjects/" + userID,
	})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1.0/groups/"+groupID+"/members/$ref", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

// doROPC exchanges username+password for tokens and returns the decoded id_token payload.
func doROPC(t *testing.T, tenant, clientID, username string) map[string]any {
	t.Helper()
	tokenBody := requestAzureToken(t, tenant, "oauth2/v2.0/token", url.Values{
		"grant_type": {"password"},
		"client_id":  {clientID},
		"username":   {username},
		"password":   {"irrelevant"},
		"scope":      {"openid profile email"},
	})
	require.NotEmpty(t, tokenBody.IDToken, "ROPC must return id_token when scope includes openid")
	return azureTokenPayload(t, tokenBody.IDToken)
}

// --- Standard provisioning + ROPC tests ---

func TestEntra_GraphGroupCRUD(t *testing.T) {
	grp := createGraphGroup(t, "SDK-Test-Group")

	// GET by ID
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1.0/groups/"+grp.ID, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got graphGroup
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, grp.ID, got.ID)
	assert.Equal(t, "SDK-Test-Group", got.DisplayName)
}

func TestEntra_GraphUserCRUD(t *testing.T) {
	u := createGraphUser(t, "SDK Test User", "sdktest@example.com")

	// GET by ID
	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1.0/users/"+u.ID, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got graphUser
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, u.ID, got.ID)
	assert.Equal(t, "sdktest@example.com", got.UserPrincipalName)
}

func TestEntra_GraphGroupMembership(t *testing.T) {
	grp := createGraphGroup(t, "SDK-Membership-Group")
	u := createGraphUser(t, "Member User", "member@example.com")
	addGroupMember(t, grp.ID, u.ID)

	// GET members
	resp, err := http.Get(baseURL + "/v1.0/groups/" + grp.ID + "/members")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		Value []graphUser `json:"value"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Value, 1)
	assert.Equal(t, u.ID, body.Value[0].ID)

	// DELETE member
	req, _ := http.NewRequest(http.MethodDelete, baseURL+"/v1.0/groups/"+grp.ID+"/members/"+u.ID+"/$ref", nil)
	delResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	delResp.Body.Close()
	require.Equal(t, http.StatusNoContent, delResp.StatusCode)
}

func TestEntra_ROPCIDTokenGroupsClaim(t *testing.T) {
	grp := createGraphGroup(t, "ROPC-Admins")
	u := createGraphUser(t, "ROPC User", "ropc-user@example.com")
	addGroupMember(t, grp.ID, u.ID)

	payload := doROPC(t, "tenant-ropc", "client-ropc", "ropc-user@example.com")

	assert.Equal(t, u.ID, payload["oid"])
	assert.Equal(t, "ROPC User", payload["name"])
	groupsRaw, ok := payload["groups"]
	require.True(t, ok, "id_token must contain groups claim")
	groups, ok := groupsRaw.([]any)
	require.True(t, ok)
	assert.Contains(t, groups, grp.ID)
}

func TestEntra_DuplicateUPNRejectedAndROPCClaimsStayDeterministic(t *testing.T) {
	grp := createGraphGroup(t, "ROPC-Duplicate-UPN")
	u := createGraphUser(t, "Duplicate UPN User", "duplicate-upn-sdk@example.com")
	addGroupMember(t, grp.ID, u.ID)

	body, _ := json.Marshal(map[string]any{
		"displayName":       "Duplicate UPN User Two",
		"userPrincipalName": "DUPLICATE-UPN-SDK@example.com",
		"mailNickname":      "Duplicate UPN User Two",
		"accountEnabled":    true,
		"passwordProfile": map[string]any{
			"password":                      "Test1234!",
			"forceChangePasswordNextSignIn": false,
		},
	})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1.0/users", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var errBody struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errBody))
	assert.Equal(t, "Request_BadRequest", errBody.Error.Code)
	assert.Contains(t, errBody.Error.Message, "userPrincipalName")

	payload := doROPC(t, "tenant-ropc-duplicate", "client-ropc-duplicate", "duplicate-upn-sdk@example.com")
	assert.Equal(t, u.ID, payload["oid"])
	groupsRaw, ok := payload["groups"]
	require.True(t, ok, "id_token must contain groups claim")
	groups, ok := groupsRaw.([]any)
	require.True(t, ok)
	assert.Contains(t, groups, grp.ID)
}

func TestEntra_ROPCUnknownUserReturns400(t *testing.T) {
	tokenResp, err := http.PostForm(baseURL+"/tenant-ropc-unknown/oauth2/v2.0/token", url.Values{
		"grant_type": {"password"},
		"client_id":  {"client-ropc"},
		"username":   {"nobody@example.com"},
		"password":   {"x"},
		"scope":      {"openid"},
	})
	require.NoError(t, err)
	defer tokenResp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, tokenResp.StatusCode)
}

func TestEntra_IDTokenGroupsClaim(t *testing.T) {
	grp := createGraphGroup(t, "Auth-Admins")
	u := createGraphUser(t, "Auth Groups User", "authgroups@example.com")
	addGroupMember(t, grp.ID, u.ID)

	// Auth code flow uses the sim-active user (default). Provision via ROPC
	// which targets the specific user.
	payload := doROPC(t, "tenant-entra-groups", "client-entra-groups", "authgroups@example.com")

	assert.Equal(t, u.ID, payload["oid"])
	groupsRaw, ok := payload["groups"]
	require.True(t, ok, "id_token must contain groups claim")
	groups, ok := groupsRaw.([]any)
	require.True(t, ok)
	assert.Contains(t, groups, grp.ID)
}

func TestEntra_IDTokenNoGroupsClaimWhenUserHasNoGroups(t *testing.T) {
	u := createGraphUser(t, "No Groups User", "nogroups@example.com")
	payload := doROPC(t, "tenant-entra-nogroups", "client-entra-nogroups", "nogroups@example.com")
	assert.Equal(t, u.ID, payload["oid"])
	_, hasGroups := payload["groups"]
	assert.False(t, hasGroups, "id_token must not contain groups claim when user has no groups")
}

func TestEntra_GraphMemberOf(t *testing.T) {
	grp1 := createGraphGroup(t, "Engineering")
	grp2 := createGraphGroup(t, "Platform")
	u := createGraphUser(t, "Graph User", "graph-user@example.com")
	addGroupMember(t, grp1.ID, u.ID)
	addGroupMember(t, grp2.ID, u.ID)

	// Get an access token via ROPC (oid in the token will be the created user's oid).
	tokenBody := requestAzureToken(t, "tenant-entra-graph", "oauth2/v2.0/token", url.Values{
		"grant_type": {"password"},
		"client_id":  {"client-entra-graph"},
		"username":   {"graph-user@example.com"},
		"password":   {"x"},
		"scope":      {"https://graph.microsoft.com/.default"},
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
	assert.Equal(t, "Engineering", ids[grp1.ID])
	assert.Equal(t, "Platform", ids[grp2.ID])
}

func TestEntra_GraphTransitiveMemberOf(t *testing.T) {
	grp := createGraphGroup(t, "Infra")
	u := createGraphUser(t, "Transitive User", "transitive@example.com")
	addGroupMember(t, grp.ID, u.ID)

	tokenBody := requestAzureToken(t, "tenant-entra-transitive", "oauth2/v2.0/token", url.Values{
		"grant_type": {"password"},
		"client_id":  {"client-entra-transitive"},
		"username":   {"transitive@example.com"},
		"password":   {"x"},
		"scope":      {"https://graph.microsoft.com/.default"},
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
	assert.Equal(t, grp.ID, body.Value[0].ID)
}

func TestEntra_DiscoveryAdvertisesGroupsClaimAndROPC(t *testing.T) {
	resp, err := http.Get(baseURL + "/tenant-groups-discovery/.well-known/openid-configuration")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		ClaimsSupported     []string `json:"claims_supported"`
		GrantTypesSupported []string `json:"grant_types_supported"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body.ClaimsSupported, "groups")
	assert.Contains(t, body.GrantTypesSupported, "password")
}
