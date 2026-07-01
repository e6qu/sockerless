package bleephub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func seedCodeScanningAlert(t *testing.T, owner, repo, ruleID, severity, toolName string) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"rule_id":          ruleID,
		"rule_severity":    severity,
		"rule_description": "test description for " + ruleID,
		"tool_name":        toolName,
		"instances": []map[string]any{
			{
				"ref":          "refs/heads/main",
				"analysis_key": ".github/workflows/codeql.yml:analyze",
				"category":     ".github/workflows/codeql.yml:analyze/language:javascript",
				"state":        "open",
				"commit_sha":   "af5626b4a114abcb82d63db7c8082c3c4756e51b",
				"path":         "src/index.js",
				"start_line":   10,
				"end_line":     10,
				"start_column": 5,
				"end_column":   15,
				"message":      "problem here",
			},
		},
	})
	resp, err := authedPost("/internal/repos/"+owner+"/"+repo+"/code-scanning/alerts", "application/json", bytes.NewReader(body))
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

func TestCodeScanning_ListAndFilter(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "cs-list", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	seedCodeScanningAlert(t, "admin", "cs-list", "rule-a", "error", "CodeQL")
	seedCodeScanningAlert(t, "admin", "cs-list", "rule-b", "warning", "Semgrep")

	resp := authedGet(t, "/api/v3/repos/admin/cs-list/code-scanning/alerts")
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

	resp = authedGet(t, "/api/v3/repos/admin/cs-list/code-scanning/alerts?severity=error")
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
		t.Fatalf("expected 1 filtered alert, got %d", len(filtered))
	}

	resp = authedGet(t, "/api/v3/repos/admin/cs-list/code-scanning/alerts?tool_name=Semgrep")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("filter by tool_name: %d body=%s", resp.StatusCode, b)
	}
	filtered = nil
	if err := json.NewDecoder(resp.Body).Decode(&filtered); err != nil {
		t.Fatalf("decode filtered: %v", err)
	}
	resp.Body.Close()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 Semgrep alert, got %d", len(filtered))
	}

	resp = authedGet(t, "/api/v3/repos/admin/cs-list/code-scanning/alerts?rule=rule-a")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("filter by rule: %d body=%s", resp.StatusCode, b)
	}
	filtered = nil
	if err := json.NewDecoder(resp.Body).Decode(&filtered); err != nil {
		t.Fatalf("decode filtered: %v", err)
	}
	resp.Body.Close()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 rule-a alert, got %d", len(filtered))
	}
}

func TestCodeScanning_GetAndInstances(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "cs-get", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	created := seedCodeScanningAlert(t, "admin", "cs-get", "rule-get", "error", "CodeQL")
	number := int(created["number"].(float64))

	resp := authedGet(t, "/api/v3/repos/admin/cs-get/code-scanning/alerts/"+itoa(number))
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

	resp = authedGet(t, "/api/v3/repos/admin/cs-get/code-scanning/alerts/"+itoa(number)+"/instances")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get instances: %d body=%s", resp.StatusCode, b)
	}
	var instances []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&instances); err != nil {
		t.Fatalf("decode instances: %v", err)
	}
	resp.Body.Close()
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
}

func TestCodeScanning_PatchDismiss(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "cs-patch", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	created := seedCodeScanningAlert(t, "admin", "cs-patch", "rule-patch", "error", "CodeQL")
	number := int(created["number"].(float64))

	patch, _ := json.Marshal(map[string]any{"state": "dismissed", "dismissed_reason": "false_positive"})
	req, _ := http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/admin/cs-patch/code-scanning/alerts/"+itoa(number), bytes.NewReader(patch))
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
	if updated["state"] != "dismissed" {
		t.Fatalf("expected dismissed, got %v", updated["state"])
	}
	if updated["dismissed_reason"] != "false_positive" {
		t.Fatalf("expected false_positive, got %v", updated["dismissed_reason"])
	}

	// Reopen
	patch, _ = json.Marshal(map[string]any{"state": "open"})
	req, _ = http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/admin/cs-patch/code-scanning/alerts/"+itoa(number), bytes.NewReader(patch))
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
	var reopened map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&reopened); err != nil {
		t.Fatalf("decode reopened: %v", err)
	}
	resp.Body.Close()
	if reopened["state"] != "open" {
		t.Fatalf("expected open after reopen, got %v", reopened["state"])
	}
}

func TestCodeScanning_InvalidDismissedReason(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "cs-invalid", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	created := seedCodeScanningAlert(t, "admin", "cs-invalid", "rule-invalid", "error", "CodeQL")
	number := int(created["number"].(float64))

	patch, _ := json.Marshal(map[string]any{"state": "dismissed", "dismissed_reason": "not_a_reason"})
	req, _ := http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/admin/cs-invalid/code-scanning/alerts/"+itoa(number), bytes.NewReader(patch))
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

func TestCodeScanning_SARIFUploadCreatesAlerts(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "cs-sarif", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	sarif := map[string]any{
		"version": "2.1.0",
		"runs": []map[string]any{
			{
				"tool": map[string]any{
					"driver": map[string]any{"name": "CodeQL"},
				},
				"results": []map[string]any{
					{
						"ruleId":  "js/zipslip",
						"message": map[string]any{"text": "Arbitrary file write during zip extraction"},
						"locations": []map[string]any{
							{
								"physicalLocation": map[string]any{
									"artifactLocation": map[string]any{"uri": "src/zip.js"},
									"region":           map[string]any{"startLine": 5, "endLine": 5, "startColumn": 1, "endColumn": 10},
								},
							},
						},
					},
					{
						"ruleId":  "js/sql-injection",
						"message": map[string]any{"text": "SQL injection risk"},
						"locations": []map[string]any{
							{
								"physicalLocation": map[string]any{
									"artifactLocation": map[string]any{"uri": "src/db.js"},
									"region":           map[string]any{"startLine": 12, "endLine": 12, "startColumn": 3, "endColumn": 20},
								},
							},
						},
					},
				},
			},
		},
	}
	sarifBytes, _ := json.Marshal(sarif)
	body, _ := json.Marshal(map[string]any{
		"commit_sha": "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		"ref":        "refs/heads/main",
		"sarif":      base64.StdEncoding.EncodeToString(sarifBytes),
	})

	resp, err := authedPost("/api/v3/repos/admin/cs-sarif/code-scanning/sarifs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("sarif upload: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("sarif upload: %d body=%s", resp.StatusCode, b)
	}
	var upload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&upload); err != nil {
		t.Fatalf("decode upload: %v", err)
	}
	resp.Body.Close()
	if upload["id"] == "" || upload["url"] == "" {
		t.Fatalf("expected upload id and url, got %+v", upload)
	}

	// List alerts
	resp = authedGet(t, "/api/v3/repos/admin/cs-sarif/code-scanning/alerts")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list after upload: %d body=%s", resp.StatusCode, b)
	}
	var alerts []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&alerts); err != nil {
		t.Fatalf("decode alerts: %v", err)
	}
	resp.Body.Close()
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts from SARIF, got %d", len(alerts))
	}

	// Get upload
	uploadID := upload["id"].(string)
	resp = authedGet(t, "/api/v3/repos/admin/cs-sarif/code-scanning/sarifs/"+uploadID)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get upload: %d body=%s", resp.StatusCode, b)
	}
	var gotUpload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&gotUpload); err != nil {
		t.Fatalf("decode upload: %v", err)
	}
	resp.Body.Close()
	if gotUpload["processing_status"] != "complete" {
		t.Fatalf("expected upload processing_status complete, got %v", gotUpload["processing_status"])
	}
}

func TestCodeScanning_Analyses(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "cs-analyses", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	analysis := testServer.store.CreateCodeScanningAnalysis(repo.FullName, "refs/heads/main", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "key", "cat", "CodeQL")

	resp := authedGet(t, "/api/v3/repos/admin/cs-analyses/code-scanning/analyses")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list analyses: %d body=%s", resp.StatusCode, b)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode analyses: %v", err)
	}
	resp.Body.Close()
	if len(list) != 1 {
		t.Fatalf("expected 1 analysis, got %d", len(list))
	}

	resp = authedGet(t, "/api/v3/repos/admin/cs-analyses/code-scanning/analyses/"+itoa(analysis.ID))
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get analysis: %d body=%s", resp.StatusCode, b)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode analysis: %v", err)
	}
	resp.Body.Close()
	if got["id"].(float64) != float64(analysis.ID) {
		t.Fatalf("expected analysis id %d, got %v", analysis.ID, got["id"])
	}

	req, _ := http.NewRequest("DELETE", testBaseURL+"/api/v3/repos/admin/cs-analyses/code-scanning/analyses/"+itoa(analysis.ID), nil)
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete analysis: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("delete analysis: %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/cs-analyses/code-scanning/analyses/"+itoa(analysis.ID))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCodeScanning_DefaultSetup(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "cs-default", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	resp := authedGet(t, "/api/v3/repos/admin/cs-default/code-scanning/default-setup")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get default setup: %d body=%s", resp.StatusCode, b)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode default setup: %v", err)
	}
	resp.Body.Close()
	if got["state"] != "configured" {
		t.Fatalf("expected configured, got %v", got["state"])
	}

	patch, _ := json.Marshal(map[string]any{"state": "configured"})
	req, _ := http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/admin/cs-default/code-scanning/default-setup", bytes.NewReader(patch))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch default setup: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("patch default setup: %d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

func TestCodeScanning_404(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "cs-404", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}

	resp := authedGet(t, "/api/v3/repos/admin/cs-404/code-scanning/alerts/999")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 alert, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/does-not-exist/code-scanning/alerts")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 repo, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/cs-404/code-scanning/analyses/999")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 analysis, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = authedGet(t, "/api/v3/repos/admin/cs-404/code-scanning/sarifs/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 sarif, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
