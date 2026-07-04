package bleephub

import (
	"testing"
)

func TestAuthenticatedUserIssues(t *testing.T) {
	user := createTestUser(t, "issues-across-repos")
	token := testServer.store.CreateToken(user.ID, "repo").Value
	repoKey := createTestRepo(t)

	// An issue assigned to the user (default filter=assigned must return it).
	resp := ghPost(t, "/api/v3/repos/"+repoKey+"/issues", defaultToken, map[string]interface{}{
		"title":     "assigned to target user",
		"assignees": []string{user.Login},
	})
	decodeJSONWithStatus(t, resp, 201)

	// An issue created by the user (filter=created).
	resp = ghPost(t, "/api/v3/repos/"+repoKey+"/issues", token, map[string]interface{}{
		"title": "created by target user",
	})
	decodeJSONWithStatus(t, resp, 201)

	// Default filter: assigned, open.
	resp = ghGet(t, "/api/v3/issues", token)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("GET /issues status = %d", resp.StatusCode)
	}
	issues := decodeJSONArray(t, resp)
	if len(issues) != 1 || issues[0]["title"] != "assigned to target user" {
		t.Fatalf("assigned filter returned %d issues: %v", len(issues), issues)
	}
	// Cross-repo listings carry the repository member.
	repoJSON, _ := issues[0]["repository"].(map[string]interface{})
	if repoJSON == nil || repoJSON["full_name"] != repoKey {
		t.Fatalf("issue repository = %v", issues[0]["repository"])
	}

	// filter=created
	resp = ghGet(t, "/api/v3/issues?filter=created", token)
	issues = decodeJSONArray(t, resp)
	if len(issues) != 1 || issues[0]["title"] != "created by target user" {
		t.Fatalf("created filter returned %d issues: %v", len(issues), issues)
	}

	// filter=all returns both involvements, newest first by default.
	resp = ghGet(t, "/api/v3/issues?filter=all", token)
	issues = decodeJSONArray(t, resp)
	if len(issues) != 2 {
		t.Fatalf("all filter returned %d issues, want 2", len(issues))
	}

	// state=closed excludes the open issues.
	resp = ghGet(t, "/api/v3/issues?state=closed", token)
	issues = decodeJSONArray(t, resp)
	if len(issues) != 0 {
		t.Fatalf("closed filter returned %d issues, want 0", len(issues))
	}

	// Unauthenticated → 401.
	resp = ghGet(t, "/api/v3/issues", "")
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("unauthenticated status = %d, want 401", resp.StatusCode)
	}

	// Invalid filter → 422.
	resp = ghGet(t, "/api/v3/issues?filter=bogus", token)
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("invalid filter status = %d, want 422", resp.StatusCode)
	}
}

func TestAuthenticatedUserIssuesLabelFilter(t *testing.T) {
	user := createTestUser(t, "issues-label-filter")
	token := testServer.store.CreateToken(user.ID, "repo").Value
	repoKey := createTestRepo(t)

	resp := ghPost(t, "/api/v3/repos/"+repoKey+"/labels", defaultToken, map[string]interface{}{
		"name": "wanted-label", "color": "d73a4a",
	})
	decodeJSONWithStatus(t, resp, 201)

	resp = ghPost(t, "/api/v3/repos/"+repoKey+"/issues", token, map[string]interface{}{
		"title":  "labelled issue",
		"labels": []string{"wanted-label"},
	})
	decodeJSONWithStatus(t, resp, 201)
	resp = ghPost(t, "/api/v3/repos/"+repoKey+"/issues", token, map[string]interface{}{
		"title": "unlabelled issue",
	})
	decodeJSONWithStatus(t, resp, 201)

	resp = ghGet(t, "/api/v3/issues?filter=created&labels=wanted-label", token)
	issues := decodeJSONArray(t, resp)
	if len(issues) != 1 || issues[0]["title"] != "labelled issue" {
		t.Fatalf("label filter returned %d issues: %v", len(issues), issues)
	}
}
