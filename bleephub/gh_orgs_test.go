package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func ghPut(t *testing.T, path string, token string, body interface{}) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest("PUT", testBaseURL+path, bodyReader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decodeJSONArray(t *testing.T, resp *http.Response) []map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	var data []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode JSON array: %v", err)
	}
	return data
}

// TestCreateOrg verifies POST /api/v3/user/orgs → 201.
func TestCreateOrg(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/orgs", defaultToken, map[string]interface{}{
		"login":       "testorg-create",
		"name":        "Test Organization",
		"description": "A test org",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	if data["login"] != "testorg-create" {
		t.Fatalf("expected login=testorg-create, got %v", data["login"])
	}
	if data["name"] != "Test Organization" {
		t.Fatalf("expected name='Test Organization', got %v", data["name"])
	}
	if data["description"] != "A test org" {
		t.Fatalf("expected description='A test org', got %v", data["description"])
	}
	if data["type"] != "Organization" {
		t.Fatalf("expected type=Organization, got %v", data["type"])
	}
}

// TestCreateOrgNoAuth verifies POST /api/v3/user/orgs without token → 401.
func TestCreateOrgNoAuth(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/orgs", "", map[string]interface{}{
		"login": "should-fail",
	})
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

// TestGetOrg verifies GET /api/v3/orgs/{org} → 200.
func TestGetOrg(t *testing.T) {
	ghPost(t, "/api/v3/user/orgs", defaultToken, map[string]interface{}{
		"login": "testorg-get",
		"name":  "Get Org",
	})

	resp := ghGet(t, "/api/v3/orgs/testorg-get", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	if data["login"] != "testorg-get" {
		t.Fatalf("expected login=testorg-get, got %v", data["login"])
	}
	if data["name"] != "Get Org" {
		t.Fatalf("expected name='Get Org', got %v", data["name"])
	}
}

// TestGetOrgNotFound verifies GET for nonexistent org → 404.
func TestGetOrgNotFound(t *testing.T) {
	resp := ghGet(t, "/api/v3/orgs/nonexistent-org", "")
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// TestUpdateOrg verifies PATCH → description changed.
func TestUpdateOrg(t *testing.T) {
	ghPost(t, "/api/v3/user/orgs", defaultToken, map[string]interface{}{
		"login": "testorg-update",
	})

	resp := ghPatch(t, "/api/v3/orgs/testorg-update", defaultToken, map[string]interface{}{
		"description": "Updated org description",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	if data["description"] != "Updated org description" {
		t.Fatalf("expected updated description, got %v", data["description"])
	}
}

// TestDeleteOrg verifies DELETE → 204, subsequent GET → 404.
func TestDeleteOrg(t *testing.T) {
	ghPost(t, "/api/v3/user/orgs", defaultToken, map[string]interface{}{
		"login": "testorg-delete",
	})

	resp := ghDelete(t, "/api/v3/orgs/testorg-delete", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	resp2 := ghGet(t, "/api/v3/orgs/testorg-delete", "")
	defer resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Fatalf("expected 404 after delete, got %d", resp2.StatusCode)
	}
}

// TestListUserOrgs verifies GET /api/v3/user/orgs → array with created org.
func TestListAuthUserOrgs(t *testing.T) {
	ghPost(t, "/api/v3/user/orgs", defaultToken, map[string]interface{}{
		"login": "testorg-list",
	})

	resp := ghGet(t, "/api/v3/user/orgs", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	orgs := decodeJSONArray(t, resp)
	found := false
	for _, o := range orgs {
		if o["login"] == "testorg-list" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected testorg-list in user orgs")
	}
}

// TestCreateTeam verifies POST /api/v3/orgs/{org}/teams → 201.
func TestCreateTeam(t *testing.T) {
	ghPost(t, "/api/v3/user/orgs", defaultToken, map[string]interface{}{
		"login": "testorg-team",
	})

	resp := ghPost(t, "/api/v3/orgs/testorg-team/teams", defaultToken, map[string]interface{}{
		"name":        "Developers",
		"description": "Dev team",
		"privacy":     "closed",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	if data["name"] != "Developers" {
		t.Fatalf("expected name=Developers, got %v", data["name"])
	}
	if data["slug"] != "developers" {
		t.Fatalf("expected slug=developers, got %v", data["slug"])
	}
	if data["privacy"] != "closed" {
		t.Fatalf("expected privacy=closed, got %v", data["privacy"])
	}
}

// TestListTeams verifies GET /api/v3/orgs/{org}/teams → array.
func TestListTeams(t *testing.T) {
	ghPost(t, "/api/v3/user/orgs", defaultToken, map[string]interface{}{
		"login": "testorg-listteams",
	})
	ghPost(t, "/api/v3/orgs/testorg-listteams/teams", defaultToken, map[string]interface{}{
		"name": "Alpha",
	})

	resp := ghGet(t, "/api/v3/orgs/testorg-listteams/teams", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	teams := decodeJSONArray(t, resp)
	if len(teams) == 0 {
		t.Fatal("expected at least 1 team")
	}
}

// TestGetTeam verifies GET /api/v3/orgs/{org}/teams/{slug} → 200.
func TestGetTeam(t *testing.T) {
	ghPost(t, "/api/v3/user/orgs", defaultToken, map[string]interface{}{
		"login": "testorg-getteam",
	})
	ghPost(t, "/api/v3/orgs/testorg-getteam/teams", defaultToken, map[string]interface{}{
		"name": "Backend Team",
	})

	resp := ghGet(t, "/api/v3/orgs/testorg-getteam/teams/backend-team", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	if data["name"] != "Backend Team" {
		t.Fatalf("expected name='Backend Team', got %v", data["name"])
	}
	if data["slug"] != "backend-team" {
		t.Fatalf("expected slug=backend-team, got %v", data["slug"])
	}
}

// TestDeleteTeam verifies DELETE → 204.
func TestDeleteTeam(t *testing.T) {
	ghPost(t, "/api/v3/user/orgs", defaultToken, map[string]interface{}{
		"login": "testorg-delteam",
	})
	ghPost(t, "/api/v3/orgs/testorg-delteam/teams", defaultToken, map[string]interface{}{
		"name": "Temp Team",
	})

	resp := ghDelete(t, "/api/v3/orgs/testorg-delteam/teams/temp-team", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	resp2 := ghGet(t, "/api/v3/orgs/testorg-delteam/teams/temp-team", defaultToken)
	defer resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Fatalf("expected 404 after delete, got %d", resp2.StatusCode)
	}
}

// TestOrgMembership verifies PUT/GET membership → role correct.
func TestOrgMembership(t *testing.T) {
	ghPost(t, "/api/v3/user/orgs", defaultToken, map[string]interface{}{
		"login": "testorg-membership",
	})

	// Get the auto-created admin membership
	resp := ghGet(t, "/api/v3/orgs/testorg-membership/memberships/admin", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200 for get membership, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	if data["role"] != "admin" {
		t.Fatalf("expected role=admin, got %v", data["role"])
	}
	if data["state"] != "active" {
		t.Fatalf("expected state=active, got %v", data["state"])
	}
}

// TestRemoveMembership verifies DELETE → 204.
func TestRemoveMembership(t *testing.T) {
	ghPost(t, "/api/v3/user/orgs", defaultToken, map[string]interface{}{
		"login": "testorg-rmmember",
	})

	// Set membership (admin is already a member, but set again as member role)
	ghPut(t, "/api/v3/orgs/testorg-rmmember/memberships/admin", defaultToken, map[string]interface{}{
		"role": "admin",
	})

	resp := ghDelete(t, "/api/v3/orgs/testorg-rmmember/memberships/admin", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

// TestTeamRepoPermission verifies team repo access.
func TestTeamRepoPermission(t *testing.T) {
	ghPost(t, "/api/v3/user/orgs", defaultToken, map[string]interface{}{
		"login": "testorg-teamrepo",
	})
	ghPost(t, "/api/v3/orgs/testorg-teamrepo/teams", defaultToken, map[string]interface{}{
		"name":       "Devs",
		"permission": "push",
	})

	// Create a repo under the org
	ghPost(t, "/api/v3/orgs/testorg-teamrepo/repos", defaultToken, map[string]interface{}{
		"name":    "team-repo",
		"private": true,
	})

	// Add repo to team
	resp := ghPut(t, "/api/v3/orgs/testorg-teamrepo/teams/devs/repos/testorg-teamrepo/team-repo", defaultToken, nil)
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204 for add team repo, got %d", resp.StatusCode)
	}

	// Remove repo from team
	resp2 := ghDelete(t, "/api/v3/orgs/testorg-teamrepo/teams/devs/repos/testorg-teamrepo/team-repo", defaultToken)
	defer resp2.Body.Close()
	if resp2.StatusCode != 204 {
		t.Fatalf("expected 204 for remove team repo, got %d", resp2.StatusCode)
	}
}

// TestGraphQLViewerOrgs verifies viewer { organizations } query.
func TestGraphQLViewerOrgs(t *testing.T) {
	ghPost(t, "/api/v3/user/orgs", defaultToken, map[string]interface{}{
		"login": "testorg-gql",
		"name":  "GQL Org",
	})

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]string{
		"query": `{viewer{organizations(first:100){nodes{login,name},totalCount}}}`,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	d, _ := data["data"].(map[string]interface{})
	viewer, _ := d["viewer"].(map[string]interface{})
	orgs, _ := viewer["organizations"].(map[string]interface{})
	if orgs == nil {
		t.Fatalf("expected organizations in viewer: %v", data)
	}

	totalCount, _ := orgs["totalCount"].(float64)
	if totalCount < 1 {
		t.Fatalf("expected totalCount >= 1, got %v", totalCount)
	}

	nodes, _ := orgs["nodes"].([]interface{})
	found := false
	for _, n := range nodes {
		nm, _ := n.(map[string]interface{})
		if nm["login"] == "testorg-gql" {
			if nm["name"] != "GQL Org" {
				t.Fatalf("expected name='GQL Org', got %v", nm["name"])
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("testorg-gql not found in viewer organizations nodes")
	}
}

// TestGraphQLOrganization verifies the organization query.
func TestGraphQLOrganization(t *testing.T) {
	ghPost(t, "/api/v3/user/orgs", defaultToken, map[string]interface{}{
		"login": "testorg-gqlquery",
		"name":  "Query Org",
	})

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]string{
		"query": `{organization(login:"testorg-gqlquery"){login,name,description}}`,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	d, _ := data["data"].(map[string]interface{})
	org, _ := d["organization"].(map[string]interface{})
	if org == nil {
		t.Fatalf("expected organization in data: %v", data)
	}
	if org["login"] != "testorg-gqlquery" {
		t.Fatalf("expected login=testorg-gqlquery, got %v", org["login"])
	}
	if org["name"] != "Query Org" {
		t.Fatalf("expected name='Query Org', got %v", org["name"])
	}
}

// TestGraphQLOrgNotFound verifies null result for nonexistent org.
func TestGraphQLOrgNotFound(t *testing.T) {
	resp := ghPost(t, "/api/graphql", defaultToken, map[string]string{
		"query": `{organization(login:"no-such-org"){login}}`,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	d, _ := data["data"].(map[string]interface{})
	if d["organization"] != nil {
		t.Fatalf("expected null organization, got %v", d["organization"])
	}
}

// TestCreateOrgRepo verifies POST /api/v3/orgs/{org}/repos → 201.
func TestCreateOrgRepo(t *testing.T) {
	ghPost(t, "/api/v3/user/orgs", defaultToken, map[string]interface{}{
		"login": "testorg-repo",
	})

	resp := ghPost(t, "/api/v3/orgs/testorg-repo/repos", defaultToken, map[string]interface{}{
		"name":        "org-repo",
		"description": "Org-owned repo",
		"private":     false,
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	if data["name"] != "org-repo" {
		t.Fatalf("expected name=org-repo, got %v", data["name"])
	}
	if data["full_name"] != "testorg-repo/org-repo" {
		t.Fatalf("expected full_name=testorg-repo/org-repo, got %v", data["full_name"])
	}
}
