package bleephub

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSecurityAdvisory_CRUD(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "sa-crud", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	body := map[string]interface{}{
		"summary":     "Test advisory",
		"description": "A detailed description",
		"severity":    "high",
		"cwe_ids":     []string{"CWE-79"},
	}
	resp := ghPost(t, "/api/v3/repos/admin/sa-crud/security-advisories", defaultToken, body)
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create advisory: %d body=%s", resp.StatusCode, b)
	}
	var created map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	resp.Body.Close()
	ghsa, _ := created["ghsa_id"].(string)
	if ghsa == "" {
		t.Fatalf("expected ghsa_id, got %v", created["ghsa_id"])
	}
	if created["state"] != "draft" {
		t.Fatalf("expected state draft, got %v", created["state"])
	}

	resp = authedGet(t, "/api/v3/repos/admin/sa-crud/security-advisories")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list advisories: %d body=%s", resp.StatusCode, b)
	}
	var list []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	resp.Body.Close()
	if len(list) != 1 {
		t.Fatalf("expected 1 advisory, got %d", len(list))
	}

	resp = authedGet(t, "/api/v3/repos/admin/sa-crud/security-advisories/"+ghsa)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get advisory: %d body=%s", resp.StatusCode, b)
	}
	var got map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	resp.Body.Close()
	if got["summary"] != "Test advisory" {
		t.Fatalf("expected summary, got %v", got["summary"])
	}

	patch, _ := json.Marshal(map[string]interface{}{
		"summary":  "Updated advisory",
		"severity": "critical",
	})
	req, _ := http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/admin/sa-crud/security-advisories/"+ghsa, bytes.NewReader(patch))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch advisory: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("patch advisory: %d body=%s", resp.StatusCode, b)
	}
	var updated map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode updated: %v", err)
	}
	resp.Body.Close()
	if updated["summary"] != "Updated advisory" || updated["severity"] != "critical" {
		t.Fatalf("unexpected updated advisory: %+v", updated)
	}
}

func TestSecurityAdvisory_RequestCVE(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "sa-cve", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	adv := testServer.store.CreateSecurityAdvisory(repo.ID, admin.ID, CreateAdvisoryReq{
		Summary:  "CVE advisory",
		Severity: "medium",
	})
	if adv == nil {
		t.Fatal("create advisory failed")
	}

	resp, err := authedPost("/api/v3/repos/admin/sa-cve/security-advisories/"+adv.GHSAID+"/cve", "application/json", nil)
	if err != nil {
		t.Fatalf("request CVE: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("request CVE: %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/sa-cve/security-advisories/"+adv.GHSAID)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get advisory: %d body=%s", resp.StatusCode, b)
	}
	var got map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	resp.Body.Close()
	if got["cve_id"] == nil {
		t.Fatalf("expected cve_id after request, got nil")
	}
	cve, _ := got["cve_id"].(string)
	if !strings.HasPrefix(cve, "CVE-") {
		t.Fatalf("expected CVE prefix, got %v", cve)
	}
}

func TestSecurityAdvisory_Report(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "sa-report", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	body := map[string]interface{}{
		"summary":     "Reported vulnerability",
		"description": "I found a bug",
		"severity":    "low",
	}
	resp, err := authedPost("/api/v3/repos/admin/sa-report/security-advisories/reports", "application/json", bytes.NewReader(mustJSON(body)))
	if err != nil {
		t.Fatalf("report vulnerability: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("report vulnerability: %d body=%s", resp.StatusCode, b)
	}
	var created map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	resp.Body.Close()
	if created["state"] != "triage" {
		t.Fatalf("expected state triage, got %v", created["state"])
	}
	sub, _ := created["submission"].(map[string]interface{})
	if sub == nil || sub["accepted"] != true {
		t.Fatalf("expected submission accepted true, got %v", created["submission"])
	}
}

func TestSecurityAdvisory_TemporaryFork(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "sa-fork", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	adv := testServer.store.CreateSecurityAdvisory(repo.ID, admin.ID, CreateAdvisoryReq{
		Summary:  "Fork advisory",
		Severity: "high",
	})
	if adv == nil {
		t.Fatal("create advisory failed")
	}

	resp, err := authedPost("/api/v3/repos/admin/sa-fork/security-advisories/"+adv.GHSAID+"/forks", "application/json", nil)
	if err != nil {
		t.Fatalf("create temporary fork: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create temporary fork: %d body=%s", resp.StatusCode, b)
	}
	var fork map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&fork); err != nil {
		t.Fatalf("decode fork: %v", err)
	}
	resp.Body.Close()
	forkName, _ := fork["full_name"].(string)
	if forkName == "" || !strings.Contains(forkName, "sa-fork") {
		t.Fatalf("expected temporary fork full_name, got %v", forkName)
	}
	if fork["private"] != true {
		t.Fatalf("expected private fork, got %v", fork["private"])
	}

	resp = authedGet(t, "/api/v3/repos/admin/sa-fork/security-advisories/"+adv.GHSAID)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get advisory: %d body=%s", resp.StatusCode, b)
	}
	var got map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	resp.Body.Close()
	if got["private_fork"] == nil {
		t.Fatalf("expected private_fork set, got nil")
	}
}

func TestSecurityAdvisories_OrgWideList(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	org := testServer.store.CreateOrg(admin, "sa-org-list", "SA Org List", "")
	if org == nil {
		t.Fatal("create org failed")
	}
	r1 := testServer.store.CreateOrgRepo(org, admin, "sa-org-repo-1", "", false)
	r2 := testServer.store.CreateOrgRepo(org, admin, "sa-org-repo-2", "", false)
	if r1 == nil || r2 == nil {
		t.Fatal("create org repos failed")
	}

	// Honest empty list before any advisories.
	resp := ghGet(t, "/api/v3/orgs/sa-org-list/security-advisories", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list org advisories (empty): %d", resp.StatusCode)
	}
	if list := decodeJSONArray(t, resp); len(list) != 0 {
		t.Fatalf("expected empty advisory list, got %v", list)
	}

	for _, repoName := range []string{"sa-org-repo-1", "sa-org-repo-2"} {
		resp := ghPost(t, "/api/v3/repos/sa-org-list/"+repoName+"/security-advisories", defaultToken, map[string]interface{}{
			"summary":     "org-wide advisory in " + repoName,
			"description": "details",
			"severity":    "high",
		})
		if resp.StatusCode != http.StatusCreated {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("create advisory in %s: %d body=%s", repoName, resp.StatusCode, b)
		}
		resp.Body.Close()
	}

	resp = ghGet(t, "/api/v3/orgs/sa-org-list/security-advisories", defaultToken)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("list org advisories: %d", resp.StatusCode)
	}
	list := decodeJSONArray(t, resp)
	if len(list) != 2 {
		t.Fatalf("expected 2 org advisories, got %v", list)
	}
	for _, a := range list {
		if ghsa, _ := a["ghsa_id"].(string); !strings.HasPrefix(ghsa, "GHSA-") {
			t.Fatalf("advisory missing ghsa_id: %v", a)
		}
	}

	// State filter.
	resp = ghGet(t, "/api/v3/orgs/sa-org-list/security-advisories?state=draft", defaultToken)
	if drafts := decodeJSONArray(t, resp); len(drafts) != 2 {
		t.Fatalf("draft filter = %d advisories, want 2", len(drafts))
	}
	resp = ghGet(t, "/api/v3/orgs/sa-org-list/security-advisories?state=published", defaultToken)
	if published := decodeJSONArray(t, resp); len(published) != 0 {
		t.Fatalf("published filter = %d advisories, want 0", len(published))
	}

	// Unknown org.
	resp = ghGet(t, "/api/v3/orgs/no-such-sa-org/security-advisories", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown org advisories: %d, want 404", resp.StatusCode)
	}
}
