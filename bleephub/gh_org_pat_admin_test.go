package bleephub

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// seedPATGrantRequest files a pending fine-grained personal access token
// grant request through the internal seed endpoint and returns the request
// row (including the raw token value).
func seedPATGrantRequest(t *testing.T, orgLogin string, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	resp, err := authedPost("/internal/orgs/"+orgLogin+"/personal-access-token-requests", "application/json", bytes.NewReader(mustJSON(body)))
	if err != nil {
		t.Fatalf("seed PAT grant request: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("seed PAT grant request: %d body=%s", resp.StatusCode, b)
	}
	return decodeJSON(t, resp)
}

func TestOrgPATGrantRequests_ApproveRevokeLifecycle(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "pat-org", "PAT Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	owner := createTestUser(t, "pat-owner")
	testServer.store.SetMembership(org.Login, owner.ID, OrgRoleMember, MembershipStateActive)
	repo := testServer.store.CreateOrgRepo(org, admin, "pat-repo", "", false)
	if repo == nil {
		t.Fatal("create org repo failed")
	}

	seeded := seedPATGrantRequest(t, "pat-org", map[string]interface{}{
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

	// Pending request appears in the admin listing.
	resp = ghGet(t, "/api/v3/orgs/pat-org/personal-access-token-requests", defaultToken)
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
	resp = ghGet(t, fmt.Sprintf("/api/v3/orgs/pat-org/personal-access-token-requests/%d/repositories", requestID), defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list request repositories: %d", resp.StatusCode)
	}
	reqRepos := decodeJSONArray(t, resp)
	if len(reqRepos) != 1 || reqRepos[0]["full_name"] != "pat-org/pat-repo" {
		t.Fatalf("request repositories wrong: %v", reqRepos)
	}

	// Approve: the request becomes an active grant.
	resp = ghPost(t, fmt.Sprintf("/api/v3/orgs/pat-org/personal-access-token-requests/%d", requestID), defaultToken, map[string]interface{}{"action": "approve"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("approve request: %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/pat-org/personal-access-token-requests", defaultToken)
	if remaining := decodeJSONArray(t, resp); len(remaining) != 0 {
		t.Fatalf("request not consumed by approval: %v", remaining)
	}

	resp = ghGet(t, "/api/v3/orgs/pat-org/personal-access-tokens", defaultToken)
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

	// Grant repositories round-trip.
	resp = ghGet(t, fmt.Sprintf("/api/v3/orgs/pat-org/personal-access-tokens/%d/repositories", grantID), defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list grant repositories: %d", resp.StatusCode)
	}
	grantRepos := decodeJSONArray(t, resp)
	if len(grantRepos) != 1 || grantRepos[0]["full_name"] != "pat-org/pat-repo" {
		t.Fatalf("grant repositories wrong: %v", grantRepos)
	}

	// Revoke the single grant.
	resp = ghPost(t, fmt.Sprintf("/api/v3/orgs/pat-org/personal-access-tokens/%d", grantID), defaultToken, map[string]interface{}{"action": "revoke"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke grant: %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/pat-org/personal-access-tokens", defaultToken)
	if remaining := decodeJSONArray(t, resp); len(remaining) != 0 {
		t.Fatalf("grant not removed by revoke: %v", remaining)
	}
}

func TestOrgPATGrantRequests_BulkReviewAndBulkRevoke(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "pat-bulk-org", "PAT Bulk Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	owner := createTestUser(t, "pat-bulk-owner")
	testServer.store.SetMembership(org.Login, owner.ID, OrgRoleMember, MembershipStateActive)

	seed := func(name string) int {
		seeded := seedPATGrantRequest(t, "pat-bulk-org", map[string]interface{}{
			"owner":      "pat-bulk-owner",
			"token_name": name,
		})
		return int(seeded["id"].(float64))
	}

	// Bulk deny removes both requests without creating grants.
	a, b := seed("bulk-a"), seed("bulk-b")
	resp := ghPost(t, "/api/v3/orgs/pat-bulk-org/personal-access-token-requests", defaultToken, map[string]interface{}{
		"pat_request_ids": []int{a, b},
		"action":          "deny",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("bulk deny: %d, want 202", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/pat-bulk-org/personal-access-token-requests", defaultToken)
	if remaining := decodeJSONArray(t, resp); len(remaining) != 0 {
		t.Fatalf("bulk deny left requests: %v", remaining)
	}
	resp = ghGet(t, "/api/v3/orgs/pat-bulk-org/personal-access-tokens", defaultToken)
	if grants := decodeJSONArray(t, resp); len(grants) != 0 {
		t.Fatalf("bulk deny created grants: %v", grants)
	}

	// Bulk approve, then bulk revoke.
	c := seed("bulk-c")
	resp = ghPost(t, "/api/v3/orgs/pat-bulk-org/personal-access-token-requests", defaultToken, map[string]interface{}{
		"pat_request_ids": []int{c},
		"action":          "approve",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("bulk approve: %d, want 202", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/pat-bulk-org/personal-access-tokens", defaultToken)
	grants := decodeJSONArray(t, resp)
	if len(grants) != 1 {
		t.Fatalf("bulk approve grants = %v, want 1", grants)
	}
	grantID := int(grants[0]["id"].(float64))
	resp = ghPost(t, "/api/v3/orgs/pat-bulk-org/personal-access-tokens", defaultToken, map[string]interface{}{
		"action":  "revoke",
		"pat_ids": []int{grantID},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("bulk revoke: %d, want 202", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/pat-bulk-org/personal-access-tokens", defaultToken)
	if remaining := decodeJSONArray(t, resp); len(remaining) != 0 {
		t.Fatalf("bulk revoke left grants: %v", remaining)
	}

	// Validation: bad action, unknown request, unknown grant.
	resp = ghPost(t, "/api/v3/orgs/pat-bulk-org/personal-access-token-requests", defaultToken, map[string]interface{}{
		"pat_request_ids": []int{1},
		"action":          "escalate",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid bulk action: %d, want 422", resp.StatusCode)
	}
	resp = ghPost(t, "/api/v3/orgs/pat-bulk-org/personal-access-token-requests/999999", defaultToken, map[string]interface{}{"action": "approve"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("review unknown request: %d, want 404", resp.StatusCode)
	}
	resp = ghPost(t, "/api/v3/orgs/pat-bulk-org/personal-access-tokens/999999", defaultToken, map[string]interface{}{"action": "revoke"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("revoke unknown grant: %d, want 404", resp.StatusCode)
	}
}
