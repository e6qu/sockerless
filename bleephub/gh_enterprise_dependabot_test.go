package bleephub

import (
	"net/http"
	"testing"
)

func TestEnterpriseDependabotRepositoryAccess(t *testing.T) {
	createEnterpriseTestOrg(t, "ent-dep-access-org")
	resp := ghPost(t, "/api/v3/orgs/ent-dep-access-org/repos", defaultToken, map[string]interface{}{"name": "access-repo"})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create org repo: got %d, want 201", resp.StatusCode)
	}
	repo := decodeJSON(t, resp)
	repoID := int(repo["id"].(float64))

	access := enterpriseAPI + "/dependabot/repository-access"

	// Initial state: no default level (null), empty list.
	resp = ghGet(t, access, defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("get access: got %d, want 200", resp.StatusCode)
	}
	initial := decodeJSON(t, resp)
	if initial["default_level"] != nil {
		t.Fatalf("initial default_level = %v, want null", initial["default_level"])
	}
	if repos, ok := initial["accessible_repositories"].([]interface{}); !ok {
		t.Fatalf("accessible_repositories missing or not an array: %v", initial["accessible_repositories"])
	} else {
		for _, entry := range repos {
			if m, _ := entry.(map[string]interface{}); m != nil && int(m["id"].(float64)) == repoID {
				t.Fatal("repository accessible before being granted")
			}
		}
	}

	// Grant access.
	resp = ghPatch(t, access, defaultToken, map[string]interface{}{
		"repository_ids_to_add": []int{repoID},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("patch add: got %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, access, defaultToken)
	granted := decodeJSON(t, resp)
	found := false
	for _, entry := range granted["accessible_repositories"].([]interface{}) {
		if m, _ := entry.(map[string]interface{}); m != nil && int(m["id"].(float64)) == repoID {
			found = true
			if m["full_name"] != "ent-dep-access-org/access-repo" {
				t.Fatalf("accessible repo full_name = %v", m["full_name"])
			}
		}
	}
	if !found {
		t.Fatal("granted repository not in accessible_repositories")
	}

	// Unknown repository ID → 404.
	resp = ghPatch(t, access, defaultToken, map[string]interface{}{
		"repository_ids_to_add": []int{99999999},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("patch unknown repo: got %d, want 404", resp.StatusCode)
	}

	// Default level: invalid enum → 422, valid → 204 and round-trips.
	resp = ghPut(t, access+"/default-level", defaultToken, map[string]interface{}{"default_level": "everything"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("put invalid default level: got %d, want 422", resp.StatusCode)
	}
	resp = ghPut(t, access+"/default-level", defaultToken, map[string]interface{}{"default_level": "internal"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("put default level: got %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, access, defaultToken)
	if got := decodeJSON(t, resp)["default_level"]; got != "internal" {
		t.Fatalf("default_level after put = %v, want internal", got)
	}

	// Revoke access.
	resp = ghPatch(t, access, defaultToken, map[string]interface{}{
		"repository_ids_to_remove": []int{repoID},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("patch remove: got %d, want 204", resp.StatusCode)
	}
	resp = ghGet(t, access, defaultToken)
	after := decodeJSON(t, resp)
	for _, entry := range after["accessible_repositories"].([]interface{}) {
		if m, _ := entry.(map[string]interface{}); m != nil && int(m["id"].(float64)) == repoID {
			t.Fatal("repository still accessible after removal")
		}
	}

	// The access surface is owner-only.
	memberTok := createEnterpriseTestUser(t, "ent-dep-member")
	resp = ghGet(t, access, memberTok)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-owner get access: got %d, want 403", resp.StatusCode)
	}
}

func TestEnterpriseDependabotAlerts(t *testing.T) {
	createEnterpriseTestOrg(t, "ent-dep-alerts-org")
	resp := ghPost(t, "/api/v3/orgs/ent-dep-alerts-org/repos", defaultToken, map[string]interface{}{"name": "alerts-repo"})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("create org repo: got %d, want 201", resp.StatusCode)
	}
	resp.Body.Close()

	seedDependabotAlert(t, "ent-dep-alerts-org", "alerts-repo", map[string]any{
		"package_name":             "left-pad",
		"package_ecosystem":        "npm",
		"manifest_path":            "package-lock.json",
		"severity":                 "high",
		"summary":                  "left-pad severity test",
		"vulnerable_version_range": "< 1.3.0",
	})

	// The enterprise alert list carries the alert with its repository.
	resp = ghGet(t, enterpriseAPI+"/dependabot/alerts", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list alerts: got %d, want 200", resp.StatusCode)
	}
	var seen bool
	for _, alert := range decodeJSONArray(t, resp) {
		repo, _ := alert["repository"].(map[string]interface{})
		if repo == nil {
			t.Fatalf("alert without repository member: %v", alert)
		}
		if repo["full_name"] == "ent-dep-alerts-org/alerts-repo" {
			seen = true
			if alert["state"] != "open" {
				t.Fatalf("alert state = %v, want open", alert["state"])
			}
		}
	}
	if !seen {
		t.Fatal("enterprise alert list does not contain the seeded alert")
	}

	// Filters narrow the list: matching severity keeps it, a different
	// state drops it.
	resp = ghGet(t, enterpriseAPI+"/dependabot/alerts?severity=high&ecosystem=npm&package=left-pad", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("filtered list: got %d, want 200", resp.StatusCode)
	}
	seen = false
	for _, alert := range decodeJSONArray(t, resp) {
		repo, _ := alert["repository"].(map[string]interface{})
		if repo != nil && repo["full_name"] == "ent-dep-alerts-org/alerts-repo" {
			seen = true
		}
	}
	if !seen {
		t.Fatal("matching filters dropped the seeded alert")
	}

	resp = ghGet(t, enterpriseAPI+"/dependabot/alerts?state=dismissed", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("state filter: got %d, want 200", resp.StatusCode)
	}
	for _, alert := range decodeJSONArray(t, resp) {
		if alert["state"] != "dismissed" {
			t.Fatalf("state=dismissed filter returned state %v", alert["state"])
		}
	}

	// Alerts only surface for organizations the caller owns: a plain
	// enterprise member who owns no organization sees none.
	memberTok := createEnterpriseTestUser(t, "ent-alerts-member")
	resp = ghGet(t, enterpriseAPI+"/dependabot/alerts", memberTok)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("member list: got %d, want 200", resp.StatusCode)
	}
	if got := len(decodeJSONArray(t, resp)); got != 0 {
		t.Fatalf("non-org-owner sees %d alerts, want 0", got)
	}
}
