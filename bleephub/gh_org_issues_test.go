package bleephub

import (
	"net/http"
	"testing"
)

func TestOrgIssues_ListForAuthenticatedUser(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "orgissues-org", "Org Issues Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	if testServer.store.CreateOrgRepo(org, admin, "orgissues-repo", "", false) == nil {
		t.Fatal("create org repo failed")
	}

	// One issue assigned to the caller, one merely authored by them.
	resp := ghPost(t, "/api/v3/repos/orgissues-org/orgissues-repo/issues", defaultToken, map[string]interface{}{
		"title":     "assigned to admin",
		"assignees": []string{"admin"},
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create assigned issue: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ghPost(t, "/api/v3/repos/orgissues-org/orgissues-repo/issues", defaultToken, map[string]interface{}{
		"title": "authored by admin",
	})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create authored issue: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Default filter=assigned returns only the assigned issue, with its
	// repository attached.
	resp = ghGet(t, "/api/v3/orgs/orgissues-org/issues", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list org issues: %d", resp.StatusCode)
	}
	assigned := decodeJSONArray(t, resp)
	if len(assigned) != 1 || assigned[0]["title"] != "assigned to admin" {
		t.Fatalf("assigned filter wrong: %v", assigned)
	}
	repoJSON, _ := assigned[0]["repository"].(map[string]interface{})
	if repoJSON == nil || repoJSON["full_name"] != "orgissues-org/orgissues-repo" {
		t.Fatalf("issue repository missing: %v", assigned[0])
	}

	// filter=created returns both authored issues.
	resp = ghGet(t, "/api/v3/orgs/orgissues-org/issues?filter=created", defaultToken)
	created := decodeJSONArray(t, resp)
	if len(created) != 2 {
		t.Fatalf("created filter = %d issues, want 2", len(created))
	}

	// State filtering: close one issue, open only returns the other.
	resp = ghPatch(t, "/api/v3/repos/orgissues-org/orgissues-repo/issues/2", defaultToken, map[string]interface{}{"state": "closed"})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("close issue: %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = ghGet(t, "/api/v3/orgs/orgissues-org/issues?filter=all&state=open", defaultToken)
	open := decodeJSONArray(t, resp)
	if len(open) != 1 || open[0]["title"] != "assigned to admin" {
		t.Fatalf("open filter wrong: %v", open)
	}
	resp = ghGet(t, "/api/v3/orgs/orgissues-org/issues?filter=all&state=closed", defaultToken)
	closed := decodeJSONArray(t, resp)
	if len(closed) != 1 || closed[0]["title"] != "authored by admin" {
		t.Fatalf("closed filter wrong: %v", closed)
	}

	// Unknown org and unauthenticated caller.
	resp = ghGet(t, "/api/v3/orgs/no-such-issues-org/issues", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown org issues: %d, want 404", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/orgs/orgissues-org/issues", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated org issues: %d, want 401", resp.StatusCode)
	}
}
