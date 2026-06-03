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

// seedEntraUserCLI seeds an Entra user via the sim-internal PUT endpoint using
// a plain HTTP request (the seed endpoint is not an ARM resource).
func seedEntraUserCLI(t *testing.T, oid string, body string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, baseURL+"/sim/v1/entra/users/"+oid, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func deleteEntraUserCLI(t *testing.T, oid string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, baseURL+"/sim/v1/entra/users/"+oid, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
}

func pkceChallenge(verifier string) string {
	d := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(d[:])
}

func TestEntra_SeedAndGraphMemberOf(t *testing.T) {
	oid := "cli-entra-oid"
	seedEntraUserCLI(t, oid, `{
		"sub": "cli-entra-sub",
		"preferredUsername": "cli-user@example.com",
		"name": "CLI User",
		"groups": [
			{"id": "cli-grp-alpha", "displayName": "Alpha"},
			{"id": "cli-grp-beta",  "displayName": "Beta"}
		]
	}`)
	defer deleteEntraUserCLI(t, oid)

	// Obtain a Graph-scoped access token via client_credentials.
	tokenResp, err := http.PostForm(baseURL+"/cli-entra-tenant/oauth2/v2.0/token", url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"cli-client"},
		"client_secret": {"secret"},
		"scope":         {"https://graph.microsoft.com/.default"},
	})
	require.NoError(t, err)
	defer tokenResp.Body.Close()
	require.Equal(t, http.StatusOK, tokenResp.StatusCode)
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(tokenResp.Body).Decode(&tok))
	require.NotEmpty(t, tok.AccessToken)

	// Call Graph GET /v1.0/me/memberOf.
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
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
		} `json:"value"`
	}
	require.NoError(t, json.Unmarshal(raw, &body), string(raw))
	require.Len(t, body.Value, 2)
	ids := map[string]string{}
	for _, g := range body.Value {
		ids[g.ID] = g.DisplayName
	}
	assert.Equal(t, "Alpha", ids["cli-grp-alpha"])
	assert.Equal(t, "Beta", ids["cli-grp-beta"])
}

func TestEntra_IDTokenGroupsViaAuthCodeFlow(t *testing.T) {
	oid := "cli-idtoken-oid"
	seedEntraUserCLI(t, oid, `{
		"sub": "cli-idtoken-sub",
		"preferredUsername": "idtoken@example.com",
		"name": "IDToken User",
		"groups": [{"id": "cli-grp-viewer", "displayName": "Viewers"}]
	}`)
	defer deleteEntraUserCLI(t, oid)

	tenant := "cli-idtoken-tenant"
	clientID := "cli-idtoken-client"
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

	assert.Equal(t, oid, payload["oid"])
	groupsRaw, ok := payload["groups"]
	require.True(t, ok, "id_token must contain groups claim")
	groups, ok := groupsRaw.([]any)
	require.True(t, ok)
	require.Len(t, groups, 1)
	assert.Equal(t, "cli-grp-viewer", groups[0])
}
