package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func seedSecretAlert(t *testing.T, owner, repo, secretType string) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"secret_type": secretType,
		"locations": []map[string]any{
			{
				"type": "commit",
				"details": map[string]any{
					"path":         "config/secrets.txt",
					"start_line":   1,
					"end_line":     1,
					"start_column": 0,
					"end_column":   40,
					"blob_sha":     "af5626b4a114abcb82d63db7c8082c3c4756e51b",
					"blob_url":     "https://example.com/blob",
					"commit_sha":   "af5626b4a114abcb82d63db7c8082c3c4756e51b",
					"commit_url":   "https://example.com/commit",
					"html_url":     "https://example.com/html",
				},
			},
		},
	})
	resp, err := authedPost("/internal/repos/"+owner+"/"+repo+"/secret-scanning/alerts", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("seed alert: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("seed alert: %d body=%s", resp.StatusCode, b)
	}
	var created map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode seeded alert: %v", err)
	}
	return created
}

func TestSecretScanning_ListAndFilter(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "ss-list", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	seedSecretAlert(t, "admin", "ss-list", "github_personal_access_token")
	seedSecretAlert(t, "admin", "ss-list", "aws_access_key_id")

	resp := authedGet(t, "/api/v3/repos/admin/ss-list/secret-scanning/alerts")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list alerts: %d body=%s", resp.StatusCode, b)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	resp.Body.Close()
	if len(list) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(list))
	}

	resp = authedGet(t, "/api/v3/repos/admin/ss-list/secret-scanning/alerts?secret_type=aws_access_key_id")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("filter by secret_type: %d body=%s", resp.StatusCode, b)
	}
	var filtered []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&filtered); err != nil {
		t.Fatalf("decode filtered: %v", err)
	}
	resp.Body.Close()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered alert, got %d", len(filtered))
	}
}

func TestSecretScanning_GetAndLocations(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "ss-get", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	created := seedSecretAlert(t, "admin", "ss-get", "github_personal_access_token")
	number := int(created["number"].(float64))

	resp := authedGet(t, "/api/v3/repos/admin/ss-get/secret-scanning/alerts/"+itoa(number))
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get alert: %d body=%s", resp.StatusCode, b)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode alert: %v", err)
	}
	resp.Body.Close()
	if got["number"].(float64) != float64(number) {
		t.Fatalf("expected number %d, got %v", number, got["number"])
	}
	if got["state"] != "open" {
		t.Fatalf("expected state open, got %v", got["state"])
	}

	resp = authedGet(t, "/api/v3/repos/admin/ss-get/secret-scanning/alerts/"+itoa(number)+"/locations")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get locations: %d body=%s", resp.StatusCode, b)
	}
	var locs []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&locs); err != nil {
		t.Fatalf("decode locations: %v", err)
	}
	resp.Body.Close()
	if len(locs) != 1 {
		t.Fatalf("expected 1 location, got %d", len(locs))
	}
}

func TestSecretScanning_PatchResolution(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "ss-patch", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	created := seedSecretAlert(t, "admin", "ss-patch", "github_personal_access_token")
	number := int(created["number"].(float64))

	patch, _ := json.Marshal(map[string]any{"state": "resolved", "resolution": "false_positive"})
	req, _ := http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/admin/ss-patch/secret-scanning/alerts/"+itoa(number), bytes.NewReader(patch))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch alert: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("patch alert: %d body=%s", resp.StatusCode, b)
	}
	var updated map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode patched: %v", err)
	}
	resp.Body.Close()
	if updated["state"] != "resolved" {
		t.Fatalf("expected resolved, got %v", updated["state"])
	}
	if updated["resolution"] != "false_positive" {
		t.Fatalf("expected false_positive, got %v", updated["resolution"])
	}
}

func TestSecretScanning_InvalidResolution(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "ss-invalid", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	created := seedSecretAlert(t, "admin", "ss-invalid", "github_personal_access_token")
	number := int(created["number"].(float64))

	patch, _ := json.Marshal(map[string]any{"state": "resolved", "resolution": "not_a_real_resolution"})
	req, _ := http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/admin/ss-invalid/secret-scanning/alerts/"+itoa(number), bytes.NewReader(patch))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch alert: %v", err)
	}
	if resp.StatusCode != http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 422, got %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestSecretScanning_404(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "ss-404", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	resp := authedGet(t, "/api/v3/repos/admin/ss-404/secret-scanning/alerts/999")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/does-not-exist/secret-scanning/alerts")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for missing repo, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSecretScanning_BulkUpdate(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "ss-bulk", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	seedSecretAlert(t, "admin", "ss-bulk", "github_personal_access_token")
	seedSecretAlert(t, "admin", "ss-bulk", "github_personal_access_token")
	seedSecretAlert(t, "admin", "ss-bulk", "aws_access_key_id")

	patch, _ := json.Marshal(map[string]any{"state": "resolved", "resolution": "used_in_tests"})
	req, _ := http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/admin/ss-bulk/secret-scanning/alerts?secret_type=github_personal_access_token", bytes.NewReader(patch))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("bulk patch: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("bulk patch: %d body=%s", resp.StatusCode, b)
	}
	var updated []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode bulk response: %v", err)
	}
	resp.Body.Close()
	if len(updated) != 2 {
		t.Fatalf("expected 2 updated alerts, got %d", len(updated))
	}
	for _, a := range updated {
		if a["state"] != "resolved" || a["resolution"] != "used_in_tests" {
			t.Fatalf("unexpected updated alert: %+v", a)
		}
	}
}

func TestSecretScanning_OrgAlerts(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "ss-org-alerts", "SS Org Alerts", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	repo1 := testServer.store.CreateOrgRepo(org, admin, "ss-org-repo1", "", false)
	repo2 := testServer.store.CreateOrgRepo(org, admin, "ss-org-repo2", "", false)
	if repo1 == nil || repo2 == nil {
		t.Fatal("create org repo failed")
	}
	userRepo := testServer.store.CreateRepo(admin, "ss-user-repo", "", false)
	if userRepo == nil {
		t.Fatal("create repo failed")
	}

	seedSecretAlert(t, org.Login, repo1.Name, "github_personal_access_token")
	seedSecretAlert(t, org.Login, repo2.Name, "aws_access_key_id")
	seedSecretAlert(t, "admin", userRepo.Name, "slack_incoming_webhook_url")

	resp := authedGet(t, "/api/v3/orgs/ss-org-alerts/secret-scanning/alerts")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list org alerts: %d body=%s", resp.StatusCode, b)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode org alerts: %v", err)
	}
	resp.Body.Close()
	if len(list) != 2 {
		t.Fatalf("expected 2 org alerts, got %d", len(list))
	}
}

func TestSecretScanning_PatternConfigurations(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "ss-patterns", "SS Patterns", "")
	if org == nil {
		t.Fatal("create org failed")
	}

	resp := authedGet(t, "/api/v3/orgs/ss-patterns/secret-scanning/pattern-configurations")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list patterns: %d body=%s", resp.StatusCode, b)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode patterns: %v", err)
	}
	resp.Body.Close()
	overrides, ok := body["provider_pattern_overrides"].([]any)
	if !ok || len(overrides) == 0 {
		t.Fatalf("expected provider_pattern_overrides, got %+v", body)
	}
	found := false
	for _, po := range overrides {
		m, ok := po.(map[string]any)
		if !ok {
			continue
		}
		if m["token_type"] == "ghp" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected pattern ghp in %+v", overrides)
	}
}
