package bleephub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func seedDependabotAlert(t *testing.T, owner, repo string, overrides map[string]any) map[string]any {
	t.Helper()
	body := map[string]any{
		"package_name":             "lodash",
		"package_ecosystem":        "npm",
		"manifest_path":            "package-lock.json",
		"vulnerability_id":         "GHSA-1234-5678-9012",
		"cve_id":                   "CVE-2026-0001",
		"severity":                 "high",
		"state":                    "open",
		"summary":                  "Prototype pollution in lodash",
		"description":              "A vulnerability allows prototype pollution.",
		"vulnerable_version_range": "< 4.17.21",
		"first_patched_version":    "4.17.21",
	}
	for k, v := range overrides {
		body[k] = v
	}
	resp, err := authedPost("/internal/repos/"+owner+"/"+repo+"/dependabot/alerts", "application/json", bytes.NewReader(mustJSON(body)))
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

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func TestDependabot_ListAndFilter(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "dependabot-list", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	seedDependabotAlert(t, "admin", "dependabot-list", map[string]any{
		"package_name":      "lodash",
		"package_ecosystem": "npm",
		"manifest_path":     "package-lock.json",
		"severity":          "high",
	})
	seedDependabotAlert(t, "admin", "dependabot-list", map[string]any{
		"package_name":      "django",
		"package_ecosystem": "pip",
		"manifest_path":     "requirements.txt",
		"severity":          "critical",
	})

	resp := authedGet(t, "/api/v3/repos/admin/dependabot-list/dependabot/alerts")
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

	resp = authedGet(t, "/api/v3/repos/admin/dependabot-list/dependabot/alerts?severity=critical")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("filter by severity: %d body=%s", resp.StatusCode, b)
	}
	var filtered []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&filtered); err != nil {
		t.Fatalf("decode filtered: %v", err)
	}
	resp.Body.Close()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 critical alert, got %d", len(filtered))
	}

	resp = authedGet(t, "/api/v3/repos/admin/dependabot-list/dependabot/alerts?ecosystem=pip")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("filter by ecosystem: %d body=%s", resp.StatusCode, b)
	}
	json.NewDecoder(resp.Body).Decode(&filtered)
	resp.Body.Close()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 pip alert, got %d", len(filtered))
	}
}

func TestDependabot_GetAndPatch(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "dependabot-patch", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	created := seedDependabotAlert(t, "admin", "dependabot-patch", map[string]any{
		"package_name": "axios",
		"severity":     "medium",
	})
	number := int(created["number"].(float64))

	resp := authedGet(t, "/api/v3/repos/admin/dependabot-patch/dependabot/alerts/"+itoa(number))
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
	if got["state"] != "open" {
		t.Fatalf("expected state open, got %v", got["state"])
	}

	patch, _ := json.Marshal(map[string]any{"state": "dismissed", "dismissed_reason": "tolerable_risk", "dismissed_comment": "accepted risk"})
	req, _ := http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/admin/dependabot-patch/dependabot/alerts/"+itoa(number), bytes.NewReader(patch))
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
	if updated["state"] != "dismissed" || updated["dismissed_reason"] != "tolerable_risk" {
		t.Fatalf("unexpected patched alert: %+v", updated)
	}

	// Reopen
	patch, _ = json.Marshal(map[string]any{"state": "open"})
	req, _ = http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/admin/dependabot-patch/dependabot/alerts/"+itoa(number), bytes.NewReader(patch))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("reopen alert: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("reopen alert: %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestDependabot_InvalidDismissedReason(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "dependabot-invalid", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}
	created := seedDependabotAlert(t, "admin", "dependabot-invalid", map[string]any{})
	number := int(created["number"].(float64))

	patch, _ := json.Marshal(map[string]any{"state": "dismissed", "dismissed_reason": "not_a_reason"})
	req, _ := http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/admin/dependabot-invalid/dependabot/alerts/"+itoa(number), bytes.NewReader(patch))
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

func TestDependabot_RepoSecretsCRUD(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "dependabot-repo-secrets", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	resp := authedGet(t, "/api/v3/repos/admin/dependabot-repo-secrets/dependabot/secrets/public-key")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("public key: %d body=%s", resp.StatusCode, b)
	}
	var pk map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&pk); err != nil {
		t.Fatalf("decode public key: %v", err)
	}
	resp.Body.Close()
	keyID := pk["key_id"].(string)

	put := func(name string, status int) {
		body := map[string]any{
			"encrypted_value": base64.StdEncoding.EncodeToString([]byte("encrypted-" + name)),
			"key_id":          keyID,
		}
		req, _ := http.NewRequest("PUT", testBaseURL+"/api/v3/repos/admin/dependabot-repo-secrets/dependabot/secrets/"+name, bytes.NewReader(mustJSON(body)))
		req.Header.Set("Authorization", "Bearer "+defaultToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("put secret %s: %v", name, err)
		}
		if resp.StatusCode != status {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("put secret %s: expected %d got %d body=%s", name, status, resp.StatusCode, b)
		}
		resp.Body.Close()
	}

	put("MY_SECRET", http.StatusCreated)
	put("MY_SECRET", http.StatusNoContent)

	resp = authedGet(t, "/api/v3/repos/admin/dependabot-repo-secrets/dependabot/secrets")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list secrets: %d body=%s", resp.StatusCode, b)
	}
	var list map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	resp.Body.Close()
	secrets := list["secrets"].([]any)
	if len(secrets) != 1 {
		t.Fatalf("expected 1 secret, got %d", len(secrets))
	}

	resp = authedGet(t, "/api/v3/repos/admin/dependabot-repo-secrets/dependabot/secrets/MY_SECRET")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get secret: %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()

	req, _ := http.NewRequest("DELETE", testBaseURL+"/api/v3/repos/admin/dependabot-repo-secrets/dependabot/secrets/MY_SECRET", nil)
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete secret: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete secret: %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/dependabot-repo-secrets/dependabot/secrets/MY_SECRET")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDependabot_OrgSecretsCRUD(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "dependabot-org", "Dependabot Org", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	repo := testServer.store.CreateOrgRepo(org, admin, "dependabot-org-repo", "", false)
	if repo == nil {
		t.Fatal("create org repo failed")
	}

	resp := authedGet(t, "/api/v3/orgs/dependabot-org/dependabot/secrets/public-key")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("public key: %d body=%s", resp.StatusCode, b)
	}
	var pk map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&pk); err != nil {
		t.Fatalf("decode public key: %v", err)
	}
	resp.Body.Close()
	keyID := pk["key_id"].(string)

	putBody := map[string]any{
		"encrypted_value":         base64.StdEncoding.EncodeToString([]byte("encrypted-org-secret")),
		"key_id":                  keyID,
		"visibility":              "selected",
		"selected_repository_ids": []int{repo.ID},
	}
	req, _ := http.NewRequest("PUT", testBaseURL+"/api/v3/orgs/dependabot-org/dependabot/secrets/ORG_SECRET", bytes.NewReader(mustJSON(putBody)))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("put org secret: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("put org secret: %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/orgs/dependabot-org/dependabot/secrets")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list org secrets: %d body=%s", resp.StatusCode, b)
	}
	var list map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	resp.Body.Close()
	secrets := list["secrets"].([]any)
	if len(secrets) != 1 {
		t.Fatalf("expected 1 org secret, got %d", len(secrets))
	}

	resp = authedGet(t, "/api/v3/orgs/dependabot-org/dependabot/secrets/ORG_SECRET")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get org secret: %d body=%s", resp.StatusCode, b)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode org secret: %v", err)
	}
	resp.Body.Close()
	if got["visibility"] != "selected" {
		t.Fatalf("expected visibility selected, got %v", got["visibility"])
	}

	resp = authedGet(t, "/api/v3/orgs/dependabot-org/dependabot/secrets/ORG_SECRET/repositories")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get selected repos: %d body=%s", resp.StatusCode, b)
	}
	var repos map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		t.Fatalf("decode repos: %v", err)
	}
	resp.Body.Close()
	if repos["total_count"] != float64(1) {
		t.Fatalf("expected 1 selected repo, got %v", repos["total_count"])
	}

	// Replace selected repos
	setBody := map[string]any{"selected_repository_ids": []int{}}
	req, _ = http.NewRequest("PUT", testBaseURL+"/api/v3/orgs/dependabot-org/dependabot/secrets/ORG_SECRET/repositories", bytes.NewReader(mustJSON(setBody)))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("set selected repos: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("set selected repos: %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()

	req, _ = http.NewRequest("DELETE", testBaseURL+"/api/v3/orgs/dependabot-org/dependabot/secrets/ORG_SECRET", nil)
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete org secret: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete org secret: %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestDependabot_404(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "dependabot-404", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	resp := authedGet(t, "/api/v3/repos/admin/dependabot-404/dependabot/alerts/999")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 alert, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/does-not-exist/dependabot/alerts")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 missing repo, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/dependabot-404/dependabot/secrets/NOPE")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 secret, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/orgs/does-not-exist/dependabot/secrets")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 org, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
