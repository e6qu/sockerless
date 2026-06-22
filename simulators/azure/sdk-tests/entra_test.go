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

// --- Application + service principal provisioning ---
//
// These tests exercise the Microsoft Graph application + service-principal
// surface end-to-end. The routes covered (placeholders resolve to runtime
// object IDs at request time):
//   GET /v1.0/applications/{appObjectId}
//   PATCH /v1.0/applications/{appObjectId}
//   DELETE /v1.0/applications/{appObjectId}
//   GET /v1.0/servicePrincipals/{spId}
//   PATCH /v1.0/servicePrincipals/{spId}
//   DELETE /v1.0/servicePrincipals/{spId}
//   POST /v1.0/servicePrincipals/{spId}/addPassword
//   PATCH /v1.0/users/{userId}

func createGraphApplication(t *testing.T, displayName string) (objectID, appID string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"displayName":    displayName,
		"signInAudience": "AzureADMyOrg",
	})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1.0/applications", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var app struct {
		ID    string `json:"id"`
		AppID string `json:"appId"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&app))
	require.NotEmpty(t, app.ID)
	require.NotEmpty(t, app.AppID)
	t.Cleanup(func() {
		r, _ := http.NewRequest(http.MethodDelete, baseURL+"/v1.0/applications/"+app.ID, nil)
		cr, _ := http.DefaultClient.Do(r)
		if cr != nil {
			cr.Body.Close()
		}
	})
	return app.ID, app.AppID
}

func createGraphServicePrincipal(t *testing.T, appID, displayName string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"appId":       appID,
		"displayName": displayName,
	})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1.0/servicePrincipals", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var sp struct {
		ID                   string `json:"id"`
		AppID                string `json:"appId"`
		ServicePrincipalType string `json:"servicePrincipalType"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sp))
	require.NotEmpty(t, sp.ID)
	assert.Equal(t, appID, sp.AppID)
	assert.Equal(t, "Application", sp.ServicePrincipalType)
	t.Cleanup(func() {
		r, _ := http.NewRequest(http.MethodDelete, baseURL+"/v1.0/servicePrincipals/"+sp.ID, nil)
		cr, _ := http.DefaultClient.Do(r)
		if cr != nil {
			cr.Body.Close()
		}
	})
	return sp.ID
}

func TestEntra_ApplicationAndServicePrincipal(t *testing.T) {
	objectID, appID := createGraphApplication(t, "SDK-App")

	// GET the application by object ID.
	resp, err := http.Get(baseURL + "/v1.0/applications/" + objectID)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var gotApp struct {
		ID          string `json:"id"`
		AppID       string `json:"appId"`
		DisplayName string `json:"displayName"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&gotApp))
	assert.Equal(t, appID, gotApp.AppID)
	assert.Equal(t, "SDK-App", gotApp.DisplayName)

	spID := createGraphServicePrincipal(t, appID, "SDK-App")

	// GET the service principal by object ID.
	spResp, err := http.Get(baseURL + "/v1.0/servicePrincipals/" + spID)
	require.NoError(t, err)
	defer spResp.Body.Close()
	require.Equal(t, http.StatusOK, spResp.StatusCode)

	// LIST servicePrincipals filtered by appId.
	listResp, err := http.Get(baseURL + "/v1.0/servicePrincipals?$filter=appId+eq+'" + url.QueryEscape(appID) + "'")
	require.NoError(t, err)
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	var list struct {
		Value []struct {
			ID    string `json:"id"`
			AppID string `json:"appId"`
		} `json:"value"`
	}
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&list))
	require.Len(t, list.Value, 1, "$filter=appId must return exactly the matching SP")
	assert.Equal(t, spID, list.Value[0].ID)

	// PATCH the application's displayName (204), then read it back.
	patchApp, _ := json.Marshal(map[string]any{"displayName": "SDK-App-Renamed"})
	patchAppReq, err := http.NewRequest(http.MethodPatch, baseURL+"/v1.0/applications/"+objectID, bytes.NewReader(patchApp))
	require.NoError(t, err)
	patchAppReq.Header.Set("Content-Type", "application/json")
	patchAppResp, err := http.DefaultClient.Do(patchAppReq)
	require.NoError(t, err)
	patchAppResp.Body.Close()
	require.Equal(t, http.StatusNoContent, patchAppResp.StatusCode)

	// PATCH the service principal's displayName (204).
	patchSP, _ := json.Marshal(map[string]any{"displayName": "SDK-SP-Renamed"})
	patchSPReq, err := http.NewRequest(http.MethodPatch, baseURL+"/v1.0/servicePrincipals/"+spID, bytes.NewReader(patchSP))
	require.NoError(t, err)
	patchSPReq.Header.Set("Content-Type", "application/json")
	patchSPResp, err := http.DefaultClient.Do(patchSPReq)
	require.NoError(t, err)
	patchSPResp.Body.Close()
	require.Equal(t, http.StatusNoContent, patchSPResp.StatusCode)

	getRenamed, err := http.Get(baseURL + "/v1.0/applications/" + objectID)
	require.NoError(t, err)
	defer getRenamed.Body.Close()
	var renamed struct {
		DisplayName string `json:"displayName"`
	}
	require.NoError(t, json.NewDecoder(getRenamed.Body).Decode(&renamed))
	assert.Equal(t, "SDK-App-Renamed", renamed.DisplayName)
}

func TestEntra_ServicePrincipalAddPassword(t *testing.T) {
	_, appID := createGraphApplication(t, "SDK-Secret-App")
	spID := createGraphServicePrincipal(t, appID, "SDK-Secret-App")

	body, _ := json.Marshal(map[string]any{
		"passwordCredential": map[string]any{"displayName": "rbac-secret"},
	})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1.0/servicePrincipals/"+spID+"/addPassword", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var cred struct {
		KeyID      string `json:"keyId"`
		SecretText string `json:"secretText"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&cred))
	assert.NotEmpty(t, cred.KeyID)
	assert.NotEmpty(t, cred.SecretText, "addPassword must return the generated secretText")

	// A subsequent GET must list the credential but never echo the secretText.
	getResp, err := http.Get(baseURL + "/v1.0/servicePrincipals/" + spID)
	require.NoError(t, err)
	defer getResp.Body.Close()
	var got struct {
		PasswordCredentials []struct {
			KeyID      string `json:"keyId"`
			SecretText string `json:"secretText"`
		} `json:"passwordCredentials"`
	}
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&got))
	require.Len(t, got.PasswordCredentials, 1)
	assert.Equal(t, cred.KeyID, got.PasswordCredentials[0].KeyID)
	assert.Empty(t, got.PasswordCredentials[0].SecretText, "secretText must not be returned on read")
}

func TestEntra_UserPatch(t *testing.T) {
	u := createGraphUser(t, "Patch Me", "patch-me@example.com")

	patch, _ := json.Marshal(map[string]any{
		"displayName": "Patched Name",
		"mail":        "patched@example.com",
	})
	req, err := http.NewRequest(http.MethodPatch, baseURL+"/v1.0/users/"+u.ID, bytes.NewReader(patch))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode, "Graph user PATCH returns 204 No Content")

	getResp, err := http.Get(baseURL + "/v1.0/users/" + u.ID)
	require.NoError(t, err)
	defer getResp.Body.Close()
	var got struct {
		DisplayName string `json:"displayName"`
		Mail        string `json:"mail"`
	}
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&got))
	assert.Equal(t, "Patched Name", got.DisplayName)
	assert.Equal(t, "patched@example.com", got.Mail)
}

func TestEntra_GroupMemberODataID(t *testing.T) {
	grp := createGraphGroup(t, "OData-Group")
	u := createGraphUser(t, "OData Member", "odata-member@example.com")
	addGroupMember(t, grp.ID, u.ID)

	resp, err := http.Get(baseURL + "/v1.0/groups/" + grp.ID + "/members")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		Value []struct {
			ODataID string `json:"@odata.id"`
			ID      string `json:"id"`
		} `json:"value"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Value, 1)
	assert.Contains(t, body.Value[0].ODataID, "/v1.0/directoryObjects/"+u.ID,
		"group members must carry an @odata.id binding")
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
