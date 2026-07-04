package bleephub

import (
	"fmt"
	"net/http"
	"testing"
)

const enterpriseAPI = "/api/v3/enterprises/bleephub"

// createEnterpriseTestUser registers a non-site-admin user with a PAT on the
// shared test server and returns its token.
func createEnterpriseTestUser(t *testing.T, login string) string {
	t.Helper()
	testServer.store.mu.Lock()
	u := &User{ID: testServer.store.NextUser, Login: login, NodeID: fmt.Sprintf("U_ent%d", testServer.store.NextUser), Type: "User"}
	testServer.store.NextUser++
	testServer.store.Users[u.ID] = u
	testServer.store.UsersByLogin[u.Login] = u
	tok := &Token{Value: "ghp_" + login + "0000000000000000000000000000", UserID: u.ID, Scopes: "repo"}
	testServer.store.Tokens[tok.Value] = tok
	testServer.store.mu.Unlock()
	return tok.Value
}

// createEnterpriseTestOrg provisions an organization owned by the seeded
// admin through the GitHub Enterprise Server admin org-creation endpoint.
func createEnterpriseTestOrg(t *testing.T, login string) {
	t.Helper()
	resp := ghPost(t, "/api/v3/admin/organizations", defaultToken, map[string]interface{}{
		"login": login,
		"admin": "admin",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create org %s: got %d, want 201", login, resp.StatusCode)
	}
}

func TestEnterpriseTeams_CreateGetUpdateDelete(t *testing.T) {
	// Create.
	resp := ghPost(t, enterpriseAPI+"/teams", defaultToken, map[string]interface{}{
		"name":        "Justice League",
		"description": "A great team.",
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create: got %d, want 201", resp.StatusCode)
	}
	team := decodeJSON(t, resp)
	if team["slug"] != "justice-league" {
		t.Fatalf("slug = %v, want justice-league", team["slug"])
	}
	if team["organization_selection_type"] != "disabled" {
		t.Fatalf("organization_selection_type = %v, want disabled (create default)", team["organization_selection_type"])
	}
	if _, ok := team["group_id"]; !ok {
		t.Fatal("group_id member missing (required by the enterprise-team schema)")
	}
	if team["url"] == nil || team["html_url"] == nil || team["members_url"] == nil {
		t.Fatalf("missing url members: %v", team)
	}

	// Duplicate name → 422.
	resp = ghPost(t, enterpriseAPI+"/teams", defaultToken, map[string]interface{}{"name": "Justice League"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate create: got %d, want 422", resp.StatusCode)
	}

	// Get by slug.
	resp = ghGet(t, enterpriseAPI+"/teams/justice-league", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("get: got %d, want 200", resp.StatusCode)
	}
	got := decodeJSON(t, resp)
	if got["name"] != "Justice League" {
		t.Fatalf("get name = %v", got["name"])
	}

	// List contains it.
	resp = ghGet(t, enterpriseAPI+"/teams", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list: got %d, want 200", resp.StatusCode)
	}
	found := false
	for _, item := range decodeJSONArray(t, resp) {
		if item["slug"] == "justice-league" {
			found = true
		}
	}
	if !found {
		t.Fatal("list does not contain the created team")
	}

	// Update: rename re-slugs, selection type changes.
	resp = ghPatch(t, enterpriseAPI+"/teams/justice-league", defaultToken, map[string]interface{}{
		"name":                        "Justice Society",
		"organization_selection_type": "selected",
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("patch: got %d, want 200", resp.StatusCode)
	}
	updated := decodeJSON(t, resp)
	if updated["slug"] != "justice-society" {
		t.Fatalf("patched slug = %v, want justice-society (rename re-slugs)", updated["slug"])
	}
	if updated["organization_selection_type"] != "selected" {
		t.Fatalf("patched organization_selection_type = %v, want selected", updated["organization_selection_type"])
	}

	// Old slug is gone, new slug resolves.
	resp = ghGet(t, enterpriseAPI+"/teams/justice-league", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("old slug after rename: got %d, want 404", resp.StatusCode)
	}

	// Delete.
	resp = ghDelete(t, enterpriseAPI+"/teams/justice-society", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: got %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, enterpriseAPI+"/teams/justice-society", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete: got %d, want 404", resp.StatusCode)
	}
}

func TestEnterpriseTeams_AuthAndUnknownEnterprise(t *testing.T) {
	// Unknown enterprise slug → 404.
	resp := ghGet(t, "/api/v3/enterprises/not-this-one/teams", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown enterprise: got %d, want 404", resp.StatusCode)
	}

	// Unauthenticated → 401.
	resp = ghGet(t, enterpriseAPI+"/teams", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list: got %d, want 401", resp.StatusCode)
	}

	// Non-owner create → 403; missing name → 422.
	memberTok := createEnterpriseTestUser(t, "ent-member")
	resp = ghPost(t, enterpriseAPI+"/teams", memberTok, map[string]interface{}{"name": "Nope"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-owner create: got %d, want 403", resp.StatusCode)
	}
	resp = ghPost(t, enterpriseAPI+"/teams", defaultToken, map[string]interface{}{"description": "no name"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("missing name: got %d, want 422", resp.StatusCode)
	}

	// A plain member can read the team list.
	resp = ghGet(t, enterpriseAPI+"/teams", memberTok)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("member list: got %d, want 200", resp.StatusCode)
	}
}

func TestEnterpriseTeamMemberships_Flow(t *testing.T) {
	resp := ghPost(t, enterpriseAPI+"/teams", defaultToken, map[string]interface{}{"name": "Membership Crew"})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create team: got %d", resp.StatusCode)
	}
	resp.Body.Close()

	tokA := createEnterpriseTestUser(t, "ent-mem-a")
	_ = createEnterpriseTestUser(t, "ent-mem-b")

	base := enterpriseAPI + "/teams/membership-crew/memberships"

	// PUT single membership → 201 simple-user.
	resp = ghPut(t, base+"/ent-mem-a", defaultToken, nil)
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("put membership: got %d, want 201", resp.StatusCode)
	}
	added := decodeJSON(t, resp)
	if added["login"] != "ent-mem-a" {
		t.Fatalf("put membership login = %v", added["login"])
	}

	// Unknown user → 404.
	resp = ghPut(t, base+"/no-such-user", defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("put unknown user: got %d, want 404", resp.StatusCode)
	}

	// Non-owner cannot add → 403.
	resp = ghPut(t, base+"/ent-mem-b", tokA, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-owner put: got %d, want 403", resp.StatusCode)
	}

	// Bulk add → 200 array of the added users.
	resp = ghPost(t, base+"/add", defaultToken, map[string]interface{}{
		"usernames": []string{"ent-mem-b"},
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("bulk add: got %d, want 200", resp.StatusCode)
	}
	bulk := decodeJSONArray(t, resp)
	if len(bulk) != 1 || bulk[0]["login"] != "ent-mem-b" {
		t.Fatalf("bulk add response = %v", bulk)
	}

	// GET single membership.
	resp = ghGet(t, base+"/ent-mem-a", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("get membership: got %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// List has both, sorted by user ID.
	resp = ghGet(t, base, defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list memberships: got %d, want 200", resp.StatusCode)
	}
	members := decodeJSONArray(t, resp)
	if len(members) != 2 {
		t.Fatalf("membership count = %d, want 2", len(members))
	}

	// Bulk remove → 200 array of removed users.
	resp = ghPost(t, base+"/remove", defaultToken, map[string]interface{}{
		"usernames": []string{"ent-mem-b"},
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("bulk remove: got %d, want 200", resp.StatusCode)
	}
	removed := decodeJSONArray(t, resp)
	if len(removed) != 1 || removed[0]["login"] != "ent-mem-b" {
		t.Fatalf("bulk remove response = %v", removed)
	}

	// DELETE single membership → 204, then GET → 404.
	resp = ghDelete(t, base+"/ent-mem-a", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete membership: got %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, base+"/ent-mem-a", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete: got %d, want 404", resp.StatusCode)
	}
}

func TestEnterpriseTeamOrganizations_Assignments(t *testing.T) {
	createEnterpriseTestOrg(t, "ent-team-org-1")
	createEnterpriseTestOrg(t, "ent-team-org-2")

	// Selection type "disabled" (default): assignments cannot be edited.
	resp := ghPost(t, enterpriseAPI+"/teams", defaultToken, map[string]interface{}{"name": "Org Squad"})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create team: got %d", resp.StatusCode)
	}
	resp.Body.Close()
	base := enterpriseAPI + "/teams/org-squad/organizations"

	resp = ghPut(t, base+"/ent-team-org-1", defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("assign while disabled: got %d, want 422", resp.StatusCode)
	}

	// Switch to "selected" and assign.
	resp = ghPatch(t, enterpriseAPI+"/teams/org-squad", defaultToken, map[string]interface{}{
		"organization_selection_type": "selected",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch selection type: got %d, want 200", resp.StatusCode)
	}

	resp = ghPut(t, base+"/ent-team-org-1", defaultToken, nil)
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("assign org: got %d, want 201", resp.StatusCode)
	}
	assigned := decodeJSON(t, resp)
	if assigned["login"] != "ent-team-org-1" {
		t.Fatalf("assigned org login = %v", assigned["login"])
	}

	// Bulk add the second org.
	resp = ghPost(t, base+"/add", defaultToken, map[string]interface{}{
		"organization_slugs": []string{"ent-team-org-2"},
	})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("bulk add orgs: got %d, want 200", resp.StatusCode)
	}
	bulk := decodeJSONArray(t, resp)
	if len(bulk) != 1 || bulk[0]["login"] != "ent-team-org-2" {
		t.Fatalf("bulk add response = %v", bulk)
	}

	// List returns both assignments.
	resp = ghGet(t, base, defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list assignments: got %d, want 200", resp.StatusCode)
	}
	if got := len(decodeJSONArray(t, resp)); got != 2 {
		t.Fatalf("assignment count = %d, want 2", got)
	}

	// Single-assignment read.
	resp = ghGet(t, base+"/ent-team-org-2", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get assignment: got %d, want 200", resp.StatusCode)
	}

	// Bulk remove → 204.
	resp = ghPost(t, base+"/remove", defaultToken, map[string]interface{}{
		"organization_slugs": []string{"ent-team-org-2"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk remove orgs: got %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, base+"/ent-team-org-2", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get removed assignment: got %d, want 404", resp.StatusCode)
	}

	// DELETE single assignment.
	resp = ghDelete(t, base+"/ent-team-org-1", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete assignment: got %d, want 204", resp.StatusCode)
	}

	// Selection type "all" derives every organization on the instance.
	resp = ghPatch(t, enterpriseAPI+"/teams/org-squad", defaultToken, map[string]interface{}{
		"organization_selection_type": "all",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch selection all: got %d, want 200", resp.StatusCode)
	}
	resp = ghGet(t, base+"/ent-team-org-2", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("assignment under all: got %d, want 200 (every org assigned)", resp.StatusCode)
	}

	// Cleanup so team lists elsewhere stay predictable.
	resp = ghDelete(t, enterpriseAPI+"/teams/org-squad", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("cleanup delete: got %d", resp.StatusCode)
	}
}
