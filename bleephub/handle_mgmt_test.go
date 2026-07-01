package bleephub

import (
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"
)

func TestListWorkflowsEmpty(t *testing.T) {
	resp := authedGet(t, "/internal/workflows")
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var workflows []workflowView
	if err := json.NewDecoder(resp.Body).Decode(&workflows); err != nil {
		t.Fatal(err)
	}
	// May not be empty if other tests have run — just verify shape
	_ = workflows
}

func TestListWorkflowsWithData(t *testing.T) {
	// Seed a workflow
	testServer.store.mu.Lock()
	testServer.store.Workflows["test-wf-1"] = &Workflow{
		ID:        "test-wf-1",
		Name:      "CI Pipeline",
		RunID:     42,
		Status:    "completed",
		Result:    "success",
		CreatedAt: time.Now(),
		EventName: "push",
		Jobs: map[string]*WorkflowJob{
			"build": {Key: "build", JobID: "j1", DisplayName: "Build", Status: "completed", Result: "success"},
		},
	}
	testServer.store.mu.Unlock()

	defer func() {
		testServer.store.mu.Lock()
		delete(testServer.store.Workflows, "test-wf-1")
		testServer.store.mu.Unlock()
	}()

	resp := authedGet(t, "/internal/workflows")
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var workflows []workflowView
	if err := json.NewDecoder(resp.Body).Decode(&workflows); err != nil {
		t.Fatal(err)
	}
	if len(workflows) < 1 {
		t.Fatal("expected at least 1 workflow")
	}

	found := false
	for _, wf := range workflows {
		if wf.ID == "test-wf-1" {
			found = true
			if wf.Name != "CI Pipeline" {
				t.Errorf("expected name 'CI Pipeline', got %q", wf.Name)
			}
			if wf.Status != "completed" {
				t.Errorf("expected status 'completed', got %q", wf.Status)
			}
			if len(wf.Jobs) != 1 {
				t.Errorf("expected 1 job, got %d", len(wf.Jobs))
			}
		}
	}
	if !found {
		t.Error("test-wf-1 not found in response")
	}
}

func TestListSessions(t *testing.T) {
	resp := authedGet(t, "/internal/sessions")
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var sessions []sessionView
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		t.Fatal(err)
	}
	_ = sessions
}

func TestListRepos(t *testing.T) {
	resp := authedGet(t, "/internal/repos")
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var repos []repoView
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		t.Fatal(err)
	}
	_ = repos
}

func TestGetWorkflowNotFound(t *testing.T) {
	resp := authedGet(t, "/internal/workflows/nonexistent")
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGetWorkflowLogs(t *testing.T) {
	// Seed a workflow with log lines
	testServer.store.mu.Lock()
	testServer.store.Workflows["test-wf-logs"] = &Workflow{
		ID:        "test-wf-logs",
		Name:      "Log Test",
		RunID:     99,
		Status:    "completed",
		Result:    "success",
		CreatedAt: time.Now(),
		Jobs: map[string]*WorkflowJob{
			"test": {Key: "test", JobID: "j-log-1", DisplayName: "Test", Status: "completed", Result: "success"},
		},
	}
	testServer.store.LogLines["j-log-1"] = []string{"line 1", "line 2"}
	testServer.store.mu.Unlock()

	defer func() {
		testServer.store.mu.Lock()
		delete(testServer.store.Workflows, "test-wf-logs")
		delete(testServer.store.LogLines, "j-log-1")
		testServer.store.mu.Unlock()
	}()

	resp := authedGet(t, "/internal/workflows/test-wf-logs/logs")
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var logs map[string][]string
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		t.Fatal(err)
	}
	if lines, ok := logs["j-log-1"]; !ok || len(lines) != 2 {
		t.Errorf("expected 2 log lines for j-log-1, got %v", logs)
	}
}

func TestInternalUsersCRUD(t *testing.T) {
	create := ghPost(t, "/internal/users", defaultToken, map[string]interface{}{
		"login": "mgmtuser",
		"name":  "Mgmt User",
		"email": "m@example.com",
	})
	if create.StatusCode != 201 {
		body, _ := io.ReadAll(create.Body)
		create.Body.Close()
		t.Fatalf("create user: expected 201, got %d: %s", create.StatusCode, body)
	}
	user := decodeJSON(t, create)
	id := int(user["id"].(float64))
	if user["login"] != "mgmtuser" {
		t.Fatalf("expected login mgmtuser, got %v", user["login"])
	}

	list := ghGet(t, "/internal/users", defaultToken)
	if list.StatusCode != 200 {
		body, _ := io.ReadAll(list.Body)
		list.Body.Close()
		t.Fatalf("list users: expected 200, got %d: %s", list.StatusCode, body)
	}
	users := decodeJSONArray(t, list)
	found := false
	for _, u := range users {
		if int(u["id"].(float64)) == id {
			found = true
			if u["login"] != "mgmtuser" {
				t.Errorf("list login = %v, want mgmtuser", u["login"])
			}
			if u["type"] != "User" {
				t.Errorf("list type = %v, want User", u["type"])
			}
		}
	}
	if !found {
		t.Fatal("created user not in list")
	}

	get := ghGet(t, fmt.Sprintf("/internal/users/%d", id), defaultToken)
	if get.StatusCode != 200 {
		body, _ := io.ReadAll(get.Body)
		get.Body.Close()
		t.Fatalf("get user: expected 200, got %d: %s", get.StatusCode, body)
	}
	got := decodeJSON(t, get)
	if got["login"] != "mgmtuser" {
		t.Errorf("get login = %v, want mgmtuser", got["login"])
	}

	patch := ghPatch(t, fmt.Sprintf("/internal/users/%d", id), defaultToken, map[string]interface{}{
		"name":       "Updated",
		"email":      "u@example.com",
		"site_admin": true,
	})
	if patch.StatusCode != 200 {
		body, _ := io.ReadAll(patch.Body)
		patch.Body.Close()
		t.Fatalf("patch user: expected 200, got %d: %s", patch.StatusCode, body)
	}
	updated := decodeJSON(t, patch)
	if updated["name"] != "Updated" {
		t.Errorf("updated name = %v, want Updated", updated["name"])
	}
	if updated["email"] != "u@example.com" {
		t.Errorf("updated email = %v, want u@example.com", updated["email"])
	}
	if updated["site_admin"] != true {
		t.Errorf("updated site_admin = %v, want true", updated["site_admin"])
	}

	del := ghDelete(t, fmt.Sprintf("/internal/users/%d", id), defaultToken)
	defer del.Body.Close()
	if del.StatusCode != 204 {
		t.Fatalf("delete user: expected 204, got %d", del.StatusCode)
	}

	get2 := ghGet(t, fmt.Sprintf("/internal/users/%d", id), defaultToken)
	if get2.StatusCode != 404 {
		body, _ := io.ReadAll(get2.Body)
		get2.Body.Close()
		t.Fatalf("get after delete: expected 404, got %d: %s", get2.StatusCode, body)
	}
	get2.Body.Close()
}

func TestInternalOrgsCRUD(t *testing.T) {
	create := ghPost(t, "/internal/orgs", defaultToken, map[string]interface{}{
		"login": "mgmtorg",
		"name":  "Mgmt Org",
	})
	if create.StatusCode != 201 {
		body, _ := io.ReadAll(create.Body)
		create.Body.Close()
		t.Fatalf("create org: expected 201, got %d: %s", create.StatusCode, body)
	}
	org := decodeJSON(t, create)
	id := int(org["id"].(float64))

	list := ghGet(t, "/internal/orgs", defaultToken)
	if list.StatusCode != 200 {
		body, _ := io.ReadAll(list.Body)
		list.Body.Close()
		t.Fatalf("list orgs: expected 200, got %d: %s", list.StatusCode, body)
	}
	orgs := decodeJSONArray(t, list)
	found := false
	for _, o := range orgs {
		if int(o["id"].(float64)) == id {
			found = true
			if o["login"] != "mgmtorg" {
				t.Errorf("list login = %v, want mgmtorg", o["login"])
			}
		}
	}
	if !found {
		t.Fatal("created org not in list")
	}

	get := ghGet(t, fmt.Sprintf("/internal/orgs/%d", id), defaultToken)
	if get.StatusCode != 200 {
		body, _ := io.ReadAll(get.Body)
		get.Body.Close()
		t.Fatalf("get org: expected 200, got %d: %s", get.StatusCode, body)
	}
	got := decodeJSON(t, get)
	if got["login"] != "mgmtorg" {
		t.Errorf("get login = %v, want mgmtorg", got["login"])
	}

	patch := ghPatch(t, fmt.Sprintf("/internal/orgs/%d", id), defaultToken, map[string]interface{}{
		"name":        "Updated Org",
		"description": "updated description",
	})
	if patch.StatusCode != 200 {
		body, _ := io.ReadAll(patch.Body)
		patch.Body.Close()
		t.Fatalf("patch org: expected 200, got %d: %s", patch.StatusCode, body)
	}
	updated := decodeJSON(t, patch)
	if updated["name"] != "Updated Org" {
		t.Errorf("updated name = %v, want Updated Org", updated["name"])
	}
	if updated["description"] != "updated description" {
		t.Errorf("updated description = %v, want updated description", updated["description"])
	}

	del := ghDelete(t, fmt.Sprintf("/internal/orgs/%d", id), defaultToken)
	defer del.Body.Close()
	if del.StatusCode != 204 {
		t.Fatalf("delete org: expected 204, got %d", del.StatusCode)
	}

	get2 := ghGet(t, fmt.Sprintf("/internal/orgs/%d", id), defaultToken)
	if get2.StatusCode != 404 {
		body, _ := io.ReadAll(get2.Body)
		get2.Body.Close()
		t.Fatalf("get after delete: expected 404, got %d: %s", get2.StatusCode, body)
	}
	get2.Body.Close()
}

func TestInternalTeamsCRUD(t *testing.T) {
	orgResp := ghPost(t, "/internal/orgs", defaultToken, map[string]interface{}{
		"login": "team-mgmt-org",
		"name":  "Team Mgmt Org",
	})
	if orgResp.StatusCode != 201 {
		body, _ := io.ReadAll(orgResp.Body)
		orgResp.Body.Close()
		t.Fatalf("create org: expected 201, got %d: %s", orgResp.StatusCode, body)
	}
	org := decodeJSON(t, orgResp)
	orgLogin := org["login"].(string)
	orgID := int(org["id"].(float64))

	teamResp := ghPost(t, fmt.Sprintf("/api/v3/orgs/%s/teams", orgLogin), defaultToken, map[string]interface{}{
		"name":        "Platform",
		"description": "the platform team",
		"privacy":     "closed",
	})
	if teamResp.StatusCode != 201 {
		body, _ := io.ReadAll(teamResp.Body)
		teamResp.Body.Close()
		t.Fatalf("create team: expected 201, got %d: %s", teamResp.StatusCode, body)
	}
	team := decodeJSON(t, teamResp)
	teamID := int(team["id"].(float64))

	list := ghGet(t, "/internal/teams", defaultToken)
	if list.StatusCode != 200 {
		body, _ := io.ReadAll(list.Body)
		list.Body.Close()
		t.Fatalf("list teams: expected 200, got %d: %s", list.StatusCode, body)
	}
	teams := decodeJSONArray(t, list)
	found := false
	for _, tm := range teams {
		if int(tm["id"].(float64)) == teamID {
			found = true
			if tm["name"] != "Platform" {
				t.Errorf("list name = %v, want Platform", tm["name"])
			}
			if tm["org"] != orgLogin {
				t.Errorf("list org = %v, want %s", tm["org"], orgLogin)
			}
		}
	}
	if !found {
		t.Fatal("created team not in list")
	}

	get := ghGet(t, fmt.Sprintf("/internal/teams/%d", teamID), defaultToken)
	if get.StatusCode != 200 {
		body, _ := io.ReadAll(get.Body)
		get.Body.Close()
		t.Fatalf("get team: expected 200, got %d: %s", get.StatusCode, body)
	}
	got := decodeJSON(t, get)
	if got["name"] != "Platform" {
		t.Errorf("get name = %v, want Platform", got["name"])
	}

	patch := ghPatch(t, fmt.Sprintf("/internal/teams/%d", teamID), defaultToken, map[string]interface{}{
		"name":        "Core",
		"description": "updated",
		"privacy":     "secret",
	})
	if patch.StatusCode != 200 {
		body, _ := io.ReadAll(patch.Body)
		patch.Body.Close()
		t.Fatalf("patch team: expected 200, got %d: %s", patch.StatusCode, body)
	}
	updated := decodeJSON(t, patch)
	if updated["name"] != "Core" {
		t.Errorf("updated name = %v, want Core", updated["name"])
	}
	if updated["slug"] != "core" {
		t.Errorf("updated slug = %v, want core", updated["slug"])
	}
	if updated["privacy"] != "secret" {
		t.Errorf("updated privacy = %v, want secret", updated["privacy"])
	}

	del := ghDelete(t, fmt.Sprintf("/internal/teams/%d", teamID), defaultToken)
	defer del.Body.Close()
	if del.StatusCode != 204 {
		t.Fatalf("delete team: expected 204, got %d", del.StatusCode)
	}

	get2 := ghGet(t, fmt.Sprintf("/internal/teams/%d", teamID), defaultToken)
	if get2.StatusCode != 404 {
		body, _ := io.ReadAll(get2.Body)
		get2.Body.Close()
		t.Fatalf("get after delete: expected 404, got %d: %s", get2.StatusCode, body)
	}
	get2.Body.Close()

	ghDelete(t, fmt.Sprintf("/internal/orgs/%d", orgID), defaultToken).Body.Close()
}

func TestInternalAuditLog(t *testing.T) {
	e1 := ghPost(t, "/internal/audit-log/events", defaultToken, map[string]interface{}{
		"actor":       "admin",
		"action":      "user.login",
		"target_type": "user",
		"target_id":   "1",
		"details":     map[string]interface{}{"ip": "1.2.3.4"},
	})
	if e1.StatusCode != 201 {
		body, _ := io.ReadAll(e1.Body)
		e1.Body.Close()
		t.Fatalf("create audit event: expected 201, got %d: %s", e1.StatusCode, body)
	}
	event1 := decodeJSON(t, e1)
	if event1["action"] != "user.login" {
		t.Errorf("event action = %v, want user.login", event1["action"])
	}
	if event1["actor"] != "admin" {
		t.Errorf("event actor = %v, want admin", event1["actor"])
	}

	e2 := ghPost(t, "/internal/audit-log/events", defaultToken, map[string]interface{}{
		"actor":       "admin",
		"action":      "org.create",
		"target_type": "org",
		"target_id":   "2",
		"org":         "mgmt-org",
		"details":     map[string]interface{}{"name": "mgmt-org"},
	})
	if e2.StatusCode != 201 {
		body, _ := io.ReadAll(e2.Body)
		e2.Body.Close()
		t.Fatalf("create audit event: expected 201, got %d: %s", e2.StatusCode, body)
	}
	e2.Body.Close()

	list := ghGet(t, "/internal/audit-log", defaultToken)
	if list.StatusCode != 200 {
		body, _ := io.ReadAll(list.Body)
		list.Body.Close()
		t.Fatalf("list audit log: expected 200, got %d: %s", list.StatusCode, body)
	}
	entries := decodeJSONArray(t, list)
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 audit events, got %d", len(entries))
	}
	if entries[0]["action"] != "org.create" {
		t.Errorf("newest event action = %v, want org.create", entries[0]["action"])
	}

	filtered := ghGet(t, "/internal/audit-log?org=mgmt-org", defaultToken)
	if filtered.StatusCode != 200 {
		body, _ := io.ReadAll(filtered.Body)
		filtered.Body.Close()
		t.Fatalf("filtered audit log: expected 200, got %d: %s", filtered.StatusCode, body)
	}
	orgEntries := decodeJSONArray(t, filtered)
	if len(orgEntries) != 1 {
		t.Errorf("org filter returned %d entries, want 1", len(orgEntries))
	}

	actorFilter := ghGet(t, "/internal/audit-log?actor=admin&action=user.login", defaultToken)
	if actorFilter.StatusCode != 200 {
		body, _ := io.ReadAll(actorFilter.Body)
		actorFilter.Body.Close()
		t.Fatalf("actor/action filter: expected 200, got %d: %s", actorFilter.StatusCode, body)
	}
	actorEntries := decodeJSONArray(t, actorFilter)
	if len(actorEntries) != 1 {
		t.Errorf("actor/action filter returned %d entries, want 1", len(actorEntries))
	}

	from := time.Now().Add(-time.Hour).Format(time.RFC3339)
	to := time.Now().Add(time.Hour).Format(time.RFC3339)
	timeFilter := ghGet(t, fmt.Sprintf("/internal/audit-log?from=%s&to=%s", from, to), defaultToken)
	if timeFilter.StatusCode != 200 {
		body, _ := io.ReadAll(timeFilter.Body)
		timeFilter.Body.Close()
		t.Fatalf("time filter: expected 200, got %d: %s", timeFilter.StatusCode, body)
	}
	timeEntries := decodeJSONArray(t, timeFilter)
	if len(timeEntries) < 2 {
		t.Errorf("time filter returned %d entries, want at least 2", len(timeEntries))
	}
}
