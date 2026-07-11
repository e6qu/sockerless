package bleephub

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Live-server shape tests for the new Search, Notifications, and Rulesets
// surfaces. These travel through the shared TestMain server so the OpenAPI
// response-shape validator observes them.

func TestLiveSearch_IssuesAndReposAndUsers(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "live-search-repo", "", false)
	testServer.store.CreateIssue(repo.ID, admin.ID, "live bug", "body", nil, nil, 0)

	resp := authedGet(t, "/api/v3/search/issues?q=live+bug")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("search issues: %d body=%s", resp.StatusCode, body)
	}
	var issuesResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&issuesResp); err != nil {
		t.Fatalf("decode search issues: %v", err)
	}
	resp.Body.Close()
	if items, ok := issuesResp["items"].([]any); !ok || len(items) != 1 {
		t.Errorf("expected 1 issue result, got %+v", issuesResp["items"])
	}

	resp = authedGet(t, "/api/v3/search/repositories?q=live-search-repo")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("search repos: %d body=%s", resp.StatusCode, body)
	}
	json.NewDecoder(resp.Body).Decode(&map[string]any{})
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/search/users?q=admin")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("search users: %d body=%s", resp.StatusCode, body)
	}
	json.NewDecoder(resp.Body).Decode(&map[string]any{})
	resp.Body.Close()
}

func TestLiveNotifications_Threads(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "live-notif-repo", "", false)
	testServer.store.CreateIssue(repo.ID, admin.ID, "live notif issue", "body", nil, nil, 0)

	resp := authedGet(t, "/api/v3/notifications")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list notifications: %d body=%s", resp.StatusCode, body)
	}
	var threads []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&threads); err != nil {
		t.Fatalf("decode notifications: %v", err)
	}
	resp.Body.Close()
	if len(threads) == 0 {
		t.Fatalf("expected at least 1 notification, got 0")
	}

	var threadID string
	for _, th := range threads {
		repoObj, _ := th["repository"].(map[string]any)
		if repoObj != nil && repoObj["full_name"] == "admin/live-notif-repo" {
			threadID, _ = th["id"].(string)
			break
		}
	}
	if threadID == "" {
		t.Fatalf("notification for live-notif-repo not found in %+v", threads)
	}
	req, _ := http.NewRequest("PATCH", testBaseURL+"/api/v3/notifications/threads/"+threadID, strings.NewReader(""))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch thread: %v", err)
	}
	if resp.StatusCode != http.StatusResetContent {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("patch thread: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/live-notif-repo/notifications")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("repo notifications: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()
}

func TestLiveRulesets_CRUD(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	testServer.store.CreateRepo(admin, "live-ruleset-repo", "", false)

	create, _ := json.Marshal(map[string]any{
		"name":        "live-protect-main",
		"target":      "branch",
		"enforcement": "active",
		"conditions": map[string]any{
			"ref_name": map[string]any{
				"include": []string{"~DEFAULT_BRANCH"},
			},
		},
		"rules": []map[string]any{
			{"type": "creation"},
			{"type": "required_linear_history"},
		},
	})
	resp, _ := authedPost("/api/v3/repos/admin/live-ruleset-repo/rulesets", "application/json", strings.NewReader(string(create)))
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create ruleset: %d body=%s", resp.StatusCode, body)
	}
	var created map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created ruleset: %v", err)
	}
	resp.Body.Close()
	rsID := int(created["id"].(float64))

	resp = authedGet(t, "/api/v3/repos/admin/live-ruleset-repo/rulesets")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list rulesets: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/live-ruleset-repo/rulesets/"+itoa(rsID))
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get ruleset: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/live-ruleset-repo/rules/branches/main")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("branch rules: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()
}

func TestLiveSecretScanning_CRUD(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	testServer.store.CreateRepo(admin, "live-secret-scanning-repo", "", false)

	created := seedSecretAlert(t, "admin", "live-secret-scanning-repo", "github_personal_access_token")
	number := int(created["number"].(float64))

	resp := authedGet(t, "/api/v3/repos/admin/live-secret-scanning-repo/secret-scanning/alerts")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list alerts: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/live-secret-scanning-repo/secret-scanning/alerts/"+itoa(number))
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get alert: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/live-secret-scanning-repo/secret-scanning/alerts/"+itoa(number)+"/locations")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get locations: %d body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()
}
