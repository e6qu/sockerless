package azure_cli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Standard Graph provisioning helpers (endpoint-only, swappable with real cloud) ---

func createEntraGroup(t *testing.T, displayName string) string {
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
	var grp struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&grp))
	require.NotEmpty(t, grp.ID)
	t.Cleanup(func() {
		r, _ := http.NewRequest(http.MethodDelete, baseURL+"/v1.0/groups/"+grp.ID, nil)
		resp, _ := http.DefaultClient.Do(r)
		if resp != nil {
			resp.Body.Close()
		}
	})
	return grp.ID
}

func createEntraUser(t *testing.T, displayName, upn string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"displayName":       displayName,
		"userPrincipalName": upn,
		"mailNickname":      displayName,
		"accountEnabled":    true,
		"passwordProfile":   map[string]any{"password": "Test1234!", "forceChangePasswordNextSignIn": false},
	})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1.0/users", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var u struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&u))
	require.NotEmpty(t, u.ID)
	t.Cleanup(func() {
		r, _ := http.NewRequest(http.MethodDelete, baseURL+"/v1.0/users/"+u.ID, nil)
		resp, _ := http.DefaultClient.Do(r)
		if resp != nil {
			resp.Body.Close()
		}
	})
	return u.ID
}

func addEntraGroupMember(t *testing.T, groupID, userID string) {
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

func pkceChallenge(verifier string) string {
	d := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(d[:])
}

// TestEntra_StandardProvisioningAndGraphMemberOf provisions a user and group via
// standard Graph endpoints and verifies GET /v1.0/me/memberOf returns the correct
// groups when the access token is obtained via ROPC.
func TestEntra_StandardProvisioningAndGraphMemberOf(t *testing.T) {
	groupID := createEntraGroup(t, "CLI-Alpha")
	_ = createEntraGroup(t, "CLI-Beta") // second group, user not a member
	userID := createEntraUser(t, "CLI User", "cli-user@example.com")
	addEntraGroupMember(t, groupID, userID)

	// Obtain a Graph-scoped access token via ROPC — standard non-interactive user grant.
	tokenResp, err := http.PostForm(baseURL+"/cli-entra-tenant/oauth2/v2.0/token", url.Values{
		"grant_type": {"password"},
		"client_id":  {"cli-client"},
		"username":   {"cli-user@example.com"},
		"password":   {"x"},
		"scope":      {"https://graph.microsoft.com/.default"},
	})
	require.NoError(t, err)
	defer tokenResp.Body.Close()
	require.Equal(t, http.StatusOK, tokenResp.StatusCode)
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(tokenResp.Body).Decode(&tok))
	require.NotEmpty(t, tok.AccessToken)

	req, err := http.NewRequest(http.MethodGet, baseURL+"/v1.0/me/memberOf", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	gResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer gResp.Body.Close()
	require.Equal(t, http.StatusOK, gResp.StatusCode)

	raw, err := io.ReadAll(gResp.Body)
	require.NoError(t, err)
	var body struct {
		Value []struct {
			ID string `json:"id"`
		} `json:"value"`
	}
	require.NoError(t, json.Unmarshal(raw, &body), string(raw))
	require.Len(t, body.Value, 1, "user is member of exactly one group")
	assert.Equal(t, groupID, body.Value[0].ID)
}

// TestEntra_IDTokenGroupsViaROPC provisions a user and group via standard Graph
// endpoints and verifies the id_token returned by ROPC carries the groups claim.
func TestEntra_IDTokenGroupsViaROPC(t *testing.T) {
	groupID := createEntraGroup(t, "CLI-Viewers")
	userID := createEntraUser(t, "IDToken ROPC User", "idtoken-ropc@example.com")
	addEntraGroupMember(t, groupID, userID)

	tenant := "cli-idtoken-tenant"
	clientID := "cli-idtoken-client"

	tokenResp, err := http.PostForm(baseURL+"/"+tenant+"/oauth2/v2.0/token", url.Values{
		"grant_type": {"password"},
		"client_id":  {clientID},
		"username":   {"idtoken-ropc@example.com"},
		"password":   {"x"},
		"scope":      {"openid profile email"},
	})
	require.NoError(t, err)
	defer tokenResp.Body.Close()
	require.Equal(t, http.StatusOK, tokenResp.StatusCode)
	var tokBody struct {
		IDToken string `json:"id_token"`
	}
	require.NoError(t, json.NewDecoder(tokenResp.Body).Decode(&tokBody))
	require.NotEmpty(t, tokBody.IDToken)

	parts := strings.Split(tokBody.IDToken, ".")
	require.Len(t, parts, 3)
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(payloadJSON, &payload))

	assert.Equal(t, userID, payload["oid"])
	groupsRaw, ok := payload["groups"]
	require.True(t, ok, "id_token must contain groups claim")
	groups, ok := groupsRaw.([]any)
	require.True(t, ok)
	assert.Contains(t, groups, groupID)
}

func TestEntra_DuplicateUPNRejectedAndROPCClaimsStayDeterministicCLI(t *testing.T) {
	groupID := createEntraGroup(t, "CLI-Duplicate-UPN")
	userID := createEntraUser(t, "CLI Duplicate UPN User", "duplicate-upn-cli@example.com")
	addEntraGroupMember(t, groupID, userID)

	body, _ := json.Marshal(map[string]any{
		"displayName":       "CLI Duplicate UPN User Two",
		"userPrincipalName": "DUPLICATE-UPN-CLI@example.com",
		"mailNickname":      "CLI Duplicate UPN User Two",
		"accountEnabled":    true,
		"passwordProfile":   map[string]any{"password": "Test1234!", "forceChangePasswordNextSignIn": false},
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

	tokenResp, err := http.PostForm(baseURL+"/cli-duplicate-upn-tenant/oauth2/v2.0/token", url.Values{
		"grant_type": {"password"},
		"client_id":  {"cli-duplicate-upn-client"},
		"username":   {"duplicate-upn-cli@example.com"},
		"password":   {"x"},
		"scope":      {"openid profile email"},
	})
	require.NoError(t, err)
	defer tokenResp.Body.Close()
	require.Equal(t, http.StatusOK, tokenResp.StatusCode)
	var tokBody struct {
		IDToken string `json:"id_token"`
	}
	require.NoError(t, json.NewDecoder(tokenResp.Body).Decode(&tokBody))
	require.NotEmpty(t, tokBody.IDToken)

	parts := strings.Split(tokBody.IDToken, ".")
	require.Len(t, parts, 3)
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(payloadJSON, &payload))
	assert.Equal(t, userID, payload["oid"])
	groupsRaw, ok := payload["groups"]
	require.True(t, ok, "id_token must contain groups claim")
	groups, ok := groupsRaw.([]any)
	require.True(t, ok)
	assert.Contains(t, groups, groupID)
}

// TestEntra_IDTokenGroupsViaAuthCodeFlow provisions a user+group via standard Graph
// endpoints, seeds that user as the sim-active user via the auth code flow's
// auto-issue mechanism, then verifies the id_token carries groups from the
// membership store. The auth code flow uses the sim-active user; ROPC is the
// standard non-interactive path when a specific user is needed.
func TestEntra_IDTokenGroupsViaAuthCodeFlow(t *testing.T) {
	groupID := createEntraGroup(t, "CLI-AuthCode-Group")
	// Seed via sim path so auth code flow (which uses active user) picks up this user.
	// This tests that inline Groups on the sim-seed path still work alongside the
	// standard membership store.
	seedBody := fmt.Sprintf(`{
		"sub": "cli-authcode-sub",
		"preferredUsername": "authcode@example.com",
		"name": "AuthCode User",
		"groups": [{"id": %q, "displayName": "CLI-AuthCode-Group"}]
	}`, groupID)
	req, err := http.NewRequest(http.MethodPut, baseURL+"/sim/v1/entra/users/cli-authcode-oid", bytes.NewBufferString(seedBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	t.Cleanup(func() {
		r, _ := http.NewRequest(http.MethodDelete, baseURL+"/sim/v1/entra/users/cli-authcode-oid", nil)
		cr, _ := http.DefaultClient.Do(r)
		if cr != nil {
			cr.Body.Close()
		}
	})

	tenant := "cli-idtoken-tenant-ac"
	clientID := "cli-idtoken-client-ac"
	redirectURI := "http://127.0.0.1/callback"
	verifier := "ThisIsntRandomButItNeedsToBe43CharactersLong"
	challenge := pkceChallenge(verifier)

	authURL := fmt.Sprintf("%s/%s/oauth2/v2.0/authorize?%s", baseURL, tenant, url.Values{
		"client_id":             {clientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"scope":                 {"openid profile email"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode())
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

	tokenResp, err := http.PostForm(baseURL+"/"+tenant+"/oauth2/v2.0/token", url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	})
	require.NoError(t, err)
	defer tokenResp.Body.Close()
	require.Equal(t, http.StatusOK, tokenResp.StatusCode)
	var tokBody struct {
		IDToken string `json:"id_token"`
	}
	require.NoError(t, json.NewDecoder(tokenResp.Body).Decode(&tokBody))
	require.NotEmpty(t, tokBody.IDToken)

	parts := strings.Split(tokBody.IDToken, ".")
	require.Len(t, parts, 3)
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(payloadJSON, &payload))

	assert.Equal(t, "cli-authcode-oid", payload["oid"])
	groupsRaw, ok := payload["groups"]
	require.True(t, ok, "id_token must contain groups claim")
	groups, ok := groupsRaw.([]any)
	require.True(t, ok)
	assert.Contains(t, groups, groupID)
}
