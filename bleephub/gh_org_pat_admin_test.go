package bleephub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// createPATGrantRequest files a pending fine-grained personal access token
// request through the authenticated browser settings workflow and returns the
// one-time credential response.
func createPATGrantRequest(t *testing.T, orgLogin, ownerToken string, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	delete(body, "owner")
	if tokenName, ok := body["token_name"]; ok {
		body["name"] = tokenName
		delete(body, "token_name")
	}
	body["resource_owner"] = orgLogin
	if _, ok := body["repository_selection"]; !ok {
		body["repository_selection"] = "none"
	}
	resp := ghPost(t, "/settings/personal-access-tokens", ownerToken, body)
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create PAT grant request: %d body=%s", resp.StatusCode, b)
	}
	return decodeJSON(t, resp)
}

func orgPATAdminAppToken(t *testing.T, org *Org) string {
	t.Helper()
	admin := testServer.store.UsersByLogin["admin"]
	permissions := map[string]string{"organization_personal_access_token_requests": "write", "organization_personal_access_tokens": "write"}
	app := testServer.store.CreateApp(admin.ID, "PAT Administration "+org.Login, "", permissions, nil)
	installation := testServer.store.CreateInstallation(app.ID, "Organization", org.ID, org.Login, permissions, nil)
	return testServer.store.CreateInstallationToken(installation.ID, app.ID, permissions, nil).Token
}

func TestOrgPATGrantRequests_ApproveRevokeLifecycle(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "pat-org", "PAT Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	owner := createTestUser(t, "pat-owner")
	testServer.store.SetMembership(org.Login, owner.ID, OrgRoleMember, MembershipStateActive)
	ownerToken := testServer.store.CreateToken(owner.ID, "repo").Value
	appToken := orgPATAdminAppToken(t, org)
	repo := testServer.store.CreateOrgRepo(org, admin, "pat-repo", "", true)
	if repo == nil {
		t.Fatal("create org repo failed")
	}
	otherRepo := testServer.store.CreateOrgRepo(org, admin, "pat-other", "", true)
	publicRepo := testServer.store.CreateOrgRepo(org, admin, "pat-public", "", false)

	seeded := createPATGrantRequest(t, "pat-org", ownerToken, map[string]interface{}{
		"owner":                "pat-owner",
		"token_name":           "ci-token",
		"reason":               "deploy pipeline",
		"repository_selection": "subset",
		"repository_ids":       []int{repo.ID},
		"permissions":          map[string]interface{}{"repository": map[string]string{"contents": "read"}},
	})
	requestID := int(seeded["id"].(float64))
	tokenValue, _ := seeded["token"].(string)
	if !strings.HasPrefix(tokenValue, "github_pat_") {
		t.Fatalf("seeded token value = %q, want github_pat_ prefix", tokenValue)
	}

	// The grant request describes a real token identity: it authenticates.
	resp := ghGet(t, "/api/v3/user", tokenValue)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("fine-grained token auth: %d", resp.StatusCode)
	}
	me := decodeJSON(t, resp)
	if me["login"] != "pat-owner" {
		t.Fatalf("fine-grained token user = %v, want pat-owner", me["login"])
	}
	resp = ghGet(t, "/api/v3/repos/pat-org/pat-repo", tokenValue)
	if resp.StatusCode != http.StatusForbidden {
		resp.Body.Close()
		t.Fatalf("pending token private repository status = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ghGet(t, "/api/v3/user/repos", tokenValue)
	pendingRepos := decodeJSONArray(t, resp)
	if len(pendingRepos) != 1 || pendingRepos[0]["id"] != float64(publicRepo.ID) {
		t.Fatalf("pending token repository inventory = %v, want only public repository", pendingRepos)
	}

	// GitHub's organization administration endpoints are GitHub App-only.
	resp = ghGet(t, "/api/v3/orgs/pat-org/personal-access-token-requests", defaultToken)
	if resp.StatusCode != http.StatusForbidden {
		resp.Body.Close()
		t.Fatalf("classic PAT organization review status = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()
	// Pending request appears in the admin listing.
	resp = ghGet(t, "/api/v3/orgs/pat-org/personal-access-token-requests", appToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list PAT grant requests: %d", resp.StatusCode)
	}
	requests := decodeJSONArray(t, resp)
	if len(requests) != 1 {
		t.Fatalf("expected 1 pending request, got %v", requests)
	}
	row := requests[0]
	if row["token_name"] != "ci-token" || row["repository_selection"] != "subset" || row["reason"] != "deploy pipeline" {
		t.Fatalf("pending request row wrong: %v", row)
	}
	if ownerJSON, _ := row["owner"].(map[string]interface{}); ownerJSON == nil || ownerJSON["login"] != "pat-owner" {
		t.Fatalf("pending request owner wrong: %v", row["owner"])
	}
	if row["token_expired"] != false || row["token_expires_at"] != nil || row["token_last_used_at"] != nil {
		t.Fatalf("token expiry fields wrong: %v", row)
	}

	// The requested repositories are listed in minimal-repository shape.
	resp = ghGet(t, fmt.Sprintf("/api/v3/orgs/pat-org/personal-access-token-requests/%d/repositories", requestID), appToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list request repositories: %d", resp.StatusCode)
	}
	reqRepos := decodeJSONArray(t, resp)
	if len(reqRepos) != 1 || reqRepos[0]["full_name"] != "pat-org/pat-repo" {
		t.Fatalf("request repositories wrong: %v", reqRepos)
	}

	// Approve: the request becomes an active grant.
	resp = ghPost(t, fmt.Sprintf("/api/v3/orgs/pat-org/personal-access-token-requests/%d", requestID), appToken, map[string]interface{}{"action": "approve"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("approve request: %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/pat-org/personal-access-token-requests", appToken)
	if remaining := decodeJSONArray(t, resp); len(remaining) != 0 {
		t.Fatalf("request not consumed by approval: %v", remaining)
	}

	resp = ghGet(t, "/api/v3/orgs/pat-org/personal-access-tokens", appToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list PAT grants: %d", resp.StatusCode)
	}
	grants := decodeJSONArray(t, resp)
	if len(grants) != 1 {
		t.Fatalf("expected 1 grant after approval, got %v", grants)
	}
	grant := grants[0]
	if grant["token_name"] != "ci-token" || grant["token_id"] != row["token_id"] {
		t.Fatalf("grant does not carry the request's token identity: %v vs %v", grant, row)
	}
	if at, _ := grant["access_granted_at"].(string); at == "" {
		t.Fatalf("grant missing access_granted_at: %v", grant)
	}
	grantID := int(grant["id"].(float64))
	resp = ghGet(t, "/api/v3/repos/pat-org/pat-repo", tokenValue)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("approved selected repository status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ghGet(t, "/api/v3/user/repos", tokenValue)
	approvedRepos := decodeJSONArray(t, resp)
	approvedIDs := map[int]bool{}
	for _, listed := range approvedRepos {
		approvedIDs[int(listed["id"].(float64))] = true
	}
	if !approvedIDs[repo.ID] || !approvedIDs[publicRepo.ID] || approvedIDs[otherRepo.ID] {
		t.Fatalf("approved token repository inventory = %v", approvedRepos)
	}
	resp = ghGet(t, "/api/v3/repos/pat-org/"+otherRepo.Name, tokenValue)
	if resp.StatusCode != http.StatusForbidden {
		resp.Body.Close()
		t.Fatalf("unselected repository status = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ghPatch(t, "/api/v3/repos/pat-org/pat-repo", tokenValue, map[string]interface{}{"description": "forbidden"})
	if resp.StatusCode != http.StatusForbidden {
		resp.Body.Close()
		t.Fatalf("ungranted administration status = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ghGet(t, "/api/v3/repos/pat-org/pat-repo/issues", tokenValue)
	if resp.StatusCode != http.StatusForbidden {
		resp.Body.Close()
		t.Fatalf("ungranted issues read status = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()

	// Grant repositories round-trip.
	resp = ghGet(t, fmt.Sprintf("/api/v3/orgs/pat-org/personal-access-tokens/%d/repositories", grantID), appToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list grant repositories: %d", resp.StatusCode)
	}
	grantRepos := decodeJSONArray(t, resp)
	if len(grantRepos) != 1 || grantRepos[0]["full_name"] != "pat-org/pat-repo" {
		t.Fatalf("grant repositories wrong: %v", grantRepos)
	}

	// Revoke the single grant.
	resp = ghPost(t, fmt.Sprintf("/api/v3/orgs/pat-org/personal-access-tokens/%d", grantID), appToken, map[string]interface{}{"action": "revoke"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke grant: %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/pat-org/personal-access-tokens", appToken)
	if remaining := decodeJSONArray(t, resp); len(remaining) != 0 {
		t.Fatalf("grant not removed by revoke: %v", remaining)
	}
	resp = ghGet(t, "/api/v3/repos/pat-org/pat-repo", tokenValue)
	if resp.StatusCode != http.StatusForbidden {
		resp.Body.Close()
		t.Fatalf("revoked token private repository status = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestOrgPATGrantRequests_BulkReviewAndBulkRevoke(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "pat-bulk-org", "PAT Bulk Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	owner := createTestUser(t, "pat-bulk-owner")
	testServer.store.SetMembership(org.Login, owner.ID, OrgRoleMember, MembershipStateActive)
	ownerToken := testServer.store.CreateToken(owner.ID, "repo").Value
	appToken := orgPATAdminAppToken(t, org)

	seed := func(name string) int {
		seeded := createPATGrantRequest(t, "pat-bulk-org", ownerToken, map[string]interface{}{
			"owner":      "pat-bulk-owner",
			"token_name": name,
		})
		return int(seeded["id"].(float64))
	}

	// Bulk deny removes both requests without creating grants.
	a, b := seed("bulk-a"), seed("bulk-b")
	resp := ghPost(t, "/api/v3/orgs/pat-bulk-org/personal-access-token-requests", appToken, map[string]interface{}{
		"pat_request_ids": []int{a, b},
		"action":          "deny",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("bulk deny: %d, want 202", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/pat-bulk-org/personal-access-token-requests", appToken)
	if remaining := decodeJSONArray(t, resp); len(remaining) != 0 {
		t.Fatalf("bulk deny left requests: %v", remaining)
	}
	resp = ghGet(t, "/api/v3/orgs/pat-bulk-org/personal-access-tokens", appToken)
	if grants := decodeJSONArray(t, resp); len(grants) != 0 {
		t.Fatalf("bulk deny created grants: %v", grants)
	}

	// Bulk approve, then bulk revoke.
	c := seed("bulk-c")
	resp = ghPost(t, "/api/v3/orgs/pat-bulk-org/personal-access-token-requests", appToken, map[string]interface{}{
		"pat_request_ids": []int{c},
		"action":          "approve",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("bulk approve: %d, want 202", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/pat-bulk-org/personal-access-tokens", appToken)
	grants := decodeJSONArray(t, resp)
	if len(grants) != 1 {
		t.Fatalf("bulk approve grants = %v, want 1", grants)
	}
	grantID := int(grants[0]["id"].(float64))
	resp = ghPost(t, "/api/v3/orgs/pat-bulk-org/personal-access-tokens", appToken, map[string]interface{}{
		"action":  "revoke",
		"pat_ids": []int{grantID},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("bulk revoke: %d, want 202", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/pat-bulk-org/personal-access-tokens", appToken)
	if remaining := decodeJSONArray(t, resp); len(remaining) != 0 {
		t.Fatalf("bulk revoke left grants: %v", remaining)
	}

	// Validation: bad action, unknown request, unknown grant.
	resp = ghPost(t, "/api/v3/orgs/pat-bulk-org/personal-access-token-requests", appToken, map[string]interface{}{
		"pat_request_ids": []int{1},
		"action":          "escalate",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid bulk action: %d, want 422", resp.StatusCode)
	}
	resp = ghPost(t, "/api/v3/orgs/pat-bulk-org/personal-access-token-requests/999999", appToken, map[string]interface{}{"action": "approve"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("review unknown request: %d, want 404", resp.StatusCode)
	}
	resp = ghPost(t, "/api/v3/orgs/pat-bulk-org/personal-access-tokens/999999", appToken, map[string]interface{}{"action": "revoke"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("revoke unknown grant: %d, want 404", resp.StatusCode)
	}
}

func TestFineGrainedPATBrowserCreationShowsCredentialOnceAndDeletesIt(t *testing.T) {
	user := createTestUser(t, "pat-browser-owner")
	loginToken := testServer.store.CreateToken(user.ID, "repo").Value
	created := decodeJSONWithStatus(t, ghPost(t, "/settings/personal-access-tokens", loginToken, map[string]interface{}{
		"name":                 "local automation",
		"resource_owner":       user.Login,
		"repository_selection": "none",
		"permissions":          map[string]interface{}{"repository": map[string]string{"contents": "read"}},
	}), http.StatusCreated)
	credential, _ := created["token"].(string)
	if !strings.HasPrefix(credential, "github_pat_") {
		t.Fatalf("created credential = %q", credential)
	}
	if created["status"] != "active" {
		t.Fatalf("personal-owner token status = %v, want active", created["status"])
	}
	tokenID := int(created["id"].(float64))

	resp := ghGet(t, "/settings/personal-access-tokens", loginToken)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || strings.Contains(string(body), credential) {
		t.Fatalf("settings list status/body leaked one-time credential: %d %s", resp.StatusCode, body)
	}

	resp = ghGet(t, "/api/v3/user", credential)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("created fine-grained credential status = %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ghDelete(t, fmt.Sprintf("/settings/personal-access-tokens/%d", tokenID), loginToken)
	if resp.StatusCode != http.StatusNoContent {
		resp.Body.Close()
		t.Fatalf("delete credential status = %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ghGet(t, "/api/v3/user", credential)
	if resp.StatusCode != http.StatusUnauthorized {
		resp.Body.Close()
		t.Fatalf("deleted credential status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestFineGrainedPATBrowserSessionCreatesFirstCredential(t *testing.T) {
	user := createTestUser(t, "pat-browser-session-owner")
	loginRequest := httptest.NewRequest(http.MethodGet, testBaseURL+"/", nil)
	loginResponse := httptest.NewRecorder()
	testServer.createBrowserSession(loginResponse, loginRequest, user)
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("browser session cookies = %d, want 1", len(cookies))
	}
	body := bytes.NewBufferString(`{"name":"browser bootstrap","resource_owner":"` + user.Login + `","repository_selection":"none","permissions":{"repository":{"contents":"read"}}}`)
	req, err := http.NewRequest(http.MethodPost, testBaseURL+"/settings/personal-access-tokens", body)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookies[0])
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("browser-session token creation status = %d body=%s", resp.StatusCode, raw)
	}
	created := map[string]interface{}{}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if credential, _ := created["token"].(string); !strings.HasPrefix(credential, "github_pat_") {
		t.Fatalf("browser-session credential = %q", credential)
	}
}

func TestFineGrainedPATExpirationStopsAuthentication(t *testing.T) {
	user := createTestUser(t, "pat-expiration-owner")
	expiresAt := time.Now().UTC().Add(time.Hour)
	token, err := testServer.store.CreateUserFineGrainedPAT(user.ID, createPersonalAccessTokenWebRequest{
		Name: "expiring token", ResourceOwner: user.Login, RepositorySelection: "none", ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("create fine-grained personal access token: %v", err)
	}
	resp := ghGet(t, "/api/v3/user", token.Value)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("unexpired token status = %d", resp.StatusCode)
	}
	resp.Body.Close()
	past := time.Now().UTC().Add(-time.Second)
	testServer.store.mu.Lock()
	testServer.store.Tokens[token.Value].ExpiresAt = &past
	testServer.store.mu.Unlock()
	resp = ghGet(t, "/api/v3/user", token.Value)
	if resp.StatusCode != http.StatusUnauthorized {
		resp.Body.Close()
		t.Fatalf("expired token status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
}
