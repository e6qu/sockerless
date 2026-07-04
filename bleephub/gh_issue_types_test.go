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
