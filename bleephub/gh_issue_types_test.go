package bleephub

import (
	"testing"
)

func TestOrgIssueTypes_CRUD(t *testing.T) {
	org := createTestOrg(t)

	// Create.
	resp := ghPost(t, "/api/v3/orgs/"+org+"/issue-types", defaultToken, map[string]interface{}{
		"name":        "Epic",
		"description": "An issue type for a multi-week tracking of work",
		"is_enabled":  true,
		"color":       "green",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("create issue type: %d", resp.StatusCode)
	}
	created := decodeJSON(t, resp)
	if created["name"] != "Epic" || created["color"] != "green" || created["is_enabled"] != true {
		t.Fatalf("created issue type = %v", created)
	}
	if created["id"] == nil || created["node_id"] == nil {
		t.Fatalf("created issue type missing id/node_id: %v", created)
	}
	id := itoa(int(created["id"].(float64)))

	// List.
	resp = ghGet(t, "/api/v3/orgs/"+org+"/issue-types", defaultToken)
	if resp.StatusCode != 200 {
		t.Fatalf("list issue types: %d", resp.StatusCode)
	}
	list := decodeJSONArray(t, resp)
	if len(list) != 1 || list[0]["name"] != "Epic" {
		t.Fatalf("list = %v", list)
	}

	// Update via PUT.
	resp = ghPut(t, "/api/v3/orgs/"+org+"/issue-types/"+id, defaultToken, map[string]interface{}{
		"name":       "Initiative",
		"is_enabled": false,
	})
	if resp.StatusCode != 200 {
		t.Fatalf("update issue type: %d", resp.StatusCode)
	}
	updated := decodeJSON(t, resp)
	if updated["name"] != "Initiative" || updated["is_enabled"] != false {
		t.Fatalf("updated issue type = %v", updated)
	}
	if updated["description"] != nil || updated["color"] != nil {
		t.Fatalf("PUT must replace optional fields, got %v", updated)
	}

	// Delete.
	resp = ghDelete(t, "/api/v3/orgs/"+org+"/issue-types/"+id, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete issue type: %d", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/"+org+"/issue-types", defaultToken)
	if got := decodeJSONArray(t, resp); len(got) != 0 {
		t.Fatalf("expected empty list after delete, got %v", got)
	}

	// Deleting again is a 404.
	resp = ghDelete(t, "/api/v3/orgs/"+org+"/issue-types/"+id, defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("delete missing issue type: %d", resp.StatusCode)
	}
}

func TestOrgIssueTypes_Validation(t *testing.T) {
	org := createTestOrg(t)

	// Unsupported color.
	resp := ghPost(t, "/api/v3/orgs/"+org+"/issue-types", defaultToken, map[string]interface{}{
		"name":       "Bug",
		"is_enabled": true,
		"color":      "chartreuse",
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("invalid color: %d", resp.StatusCode)
	}

	// Missing is_enabled.
	resp = ghPost(t, "/api/v3/orgs/"+org+"/issue-types", defaultToken, map[string]interface{}{
		"name": "Bug",
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("missing is_enabled: %d", resp.StatusCode)
	}

	// Unknown org.
	resp = ghGet(t, "/api/v3/orgs/no-such-org-issue-types/issue-types", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown org: %d", resp.StatusCode)
	}
}

func TestIssueTypeAssignmentREST(t *testing.T) {
	org := createTestOrg(t)
	repoName, _ := createOrgRepoForGovernance(t, org)
	repoFullName := org + "/" + repoName
	created := decodeJSONWithStatus(t, ghPost(t, "/api/v3/orgs/"+org+"/issue-types", defaultToken, map[string]interface{}{
		"name":       "Bug",
		"is_enabled": true,
		"color":      "red",
	}), 200)
	typeID := int(created["id"].(float64))

	issue := decodeJSONWithStatus(t, ghPost(t, "/api/v3/repos/"+repoFullName+"/issues", defaultToken, map[string]interface{}{
		"title":         "typed issue",
		"issue_type_id": typeID,
	}), 201)
	repo := testServer.store.GetRepo(org, repoName)
	stored := testServer.store.GetIssueByNumber(repo.ID, int(issue["number"].(float64)))
	if stored == nil {
		t.Fatal("stored issue not found")
	}
	if stored.IssueTypeID != typeID {
		t.Fatalf("stored IssueTypeID = %d, want %d", stored.IssueTypeID, typeID)
	}

	issue = decodeJSONWithStatus(t, ghPatch(t, "/api/v3/repos/"+repoFullName+"/issues/1", defaultToken, map[string]interface{}{
		"issue_type_id": nil,
	}), 200)
	_ = issue
	stored = testServer.store.GetIssueByNumber(repo.ID, 1)
	if stored == nil {
		t.Fatal("stored issue not found after clear")
	}
	if stored.IssueTypeID != 0 {
		t.Fatalf("cleared IssueTypeID = %d, want 0", stored.IssueTypeID)
	}
}

func TestIssueTypeAssignmentRESTValidation(t *testing.T) {
	org := createTestOrg(t)
	repoName, _ := createOrgRepoForGovernance(t, org)
	repoFullName := org + "/" + repoName
	disabled := decodeJSONWithStatus(t, ghPost(t, "/api/v3/orgs/"+org+"/issue-types", defaultToken, map[string]interface{}{
		"name":       "Disabled",
		"is_enabled": false,
	}), 200)

	otherOrg := createTestOrg(t)
	otherType := decodeJSONWithStatus(t, ghPost(t, "/api/v3/orgs/"+otherOrg+"/issue-types", defaultToken, map[string]interface{}{
		"name":       "Other org type",
		"is_enabled": true,
	}), 200)

	resp := ghPost(t, "/api/v3/repos/"+repoFullName+"/issues", defaultToken, map[string]interface{}{
		"title":         "disabled type",
		"issue_type_id": int(disabled["id"].(float64)),
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("disabled issue type create status = %d, want 422", resp.StatusCode)
	}

	issue := decodeJSONWithStatus(t, ghPost(t, "/api/v3/repos/"+repoFullName+"/issues", defaultToken, map[string]interface{}{
		"title": "untyped issue",
	}), 201)
	repo := testServer.store.GetRepo(org, repoName)
	stored := testServer.store.GetIssueByNumber(repo.ID, int(issue["number"].(float64)))
	if stored == nil {
		t.Fatal("stored issue not found")
	}
	if stored.IssueTypeID != 0 {
		t.Fatalf("untyped IssueTypeID = %d, want 0", stored.IssueTypeID)
	}

	resp = ghPatch(t, "/api/v3/repos/"+repoFullName+"/issues/1", defaultToken, map[string]interface{}{
		"issue_type_id": int(otherType["id"].(float64)),
	})
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("wrong-org issue type patch status = %d, want 422", resp.StatusCode)
	}
}

func TestIssueTypeAssignmentPersists(t *testing.T) {
	st2 := reloadedStore(t, func(_ *Persistence, st *Store) {
		st.SeedDefaultUser()
		admin := st.Users[1]
		org := st.CreateOrg(admin, "persist-issue-type-org", "Persist Issue Type", "")
		repo := st.CreateOrgRepo(org, admin, "persist-issue-type-repo", "", false)
		it := st.CreateIssueType(org.Login, "Epic", nil, nil, true)
		issue := st.CreateIssue(repo.ID, admin.ID, "typed", "", nil, nil, 0)
		st.UpdateIssue(issue.ID, func(i *Issue) {
			i.IssueTypeID = it.ID
		})
	})

	repo := st2.GetRepo("persist-issue-type-org", "persist-issue-type-repo")
	if repo == nil {
		t.Fatal("repo did not reload")
	}
	issue := st2.GetIssueByNumber(repo.ID, 1)
	if issue == nil {
		t.Fatal("issue did not reload")
	}
	it := st2.GetAssignableIssueTypeForRepo(repo, issue.IssueTypeID)
	if it == nil || it.Name != "Epic" {
		t.Fatalf("reloaded issue type = %#v for issue %#v", it, issue)
	}
}
