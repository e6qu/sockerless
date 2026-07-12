package bleephub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func seedCodeScanningAlert(t *testing.T, owner, repo, ruleID, severity, toolName string) map[string]any {
	t.Helper()
	sarif := map[string]any{
		"version": "2.1.0",
		"runs": []map[string]any{
			{
				"tool": map[string]any{
					"driver": map[string]any{
						"name": toolName,
						"rules": []map[string]any{
							{
								"id": ruleID,
								"fullDescription": map[string]any{
									"text": "test description for " + ruleID,
								},
								"defaultConfiguration": map[string]any{
									"level": severity,
								},
								"properties": map[string]any{
									"problem.severity": severity,
								},
							},
						},
					},
				},
				"results": []map[string]any{
					{
						"ruleId":  ruleID,
						"message": map[string]any{"text": "problem here"},
						"locations": []map[string]any{
							{
								"physicalLocation": map[string]any{
									"artifactLocation": map[string]any{"uri": "src/index.js"},
									"region":           map[string]any{"startLine": 10, "endLine": 10, "startColumn": 5, "endColumn": 15},
								},
							},
						},
					},
				},
			},
		},
	}
	sarifBytes, _ := json.Marshal(sarif)
	commitSHA := putRepoFile(t, owner+"/"+repo, "src/"+ruleID+".js", "const finding = true;\n", "add "+ruleID+" source")
	body, _ := json.Marshal(map[string]any{
		"commit_sha": commitSHA,
		"ref":        "refs/heads/main",
		"sarif":      base64.StdEncoding.EncodeToString(sarifBytes),
	})
	resp, err := authedPost("/api/v3/repos/"+owner+"/"+repo+"/code-scanning/sarifs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("upload SARIF: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload SARIF: %d body=%s", resp.StatusCode, b)
	}
	alertsResp := authedGet(t, "/api/v3/repos/"+owner+"/"+repo+"/code-scanning/alerts?rule="+ruleID)
	if alertsResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(alertsResp.Body)
		alertsResp.Body.Close()
		t.Fatalf("list uploaded alert: %d body=%s", alertsResp.StatusCode, b)
	}
	var alerts []map[string]any
	if err := json.NewDecoder(alertsResp.Body).Decode(&alerts); err != nil {
		t.Fatalf("decode uploaded alerts: %v", err)
	}
	alertsResp.Body.Close()
	if len(alerts) != 1 {
		t.Fatalf("uploaded SARIF produced %d alerts for %s, want 1", len(alerts), ruleID)
	}
	return alerts[0]
}

func TestCodeScanningAlertTestsUsePublicSARIFUpload(t *testing.T) {
	needles := map[string]string{
		"gh_code_scanning_test.go": `authedPost("` + `/internal/repos/"+owner+"/"+repo+"/code-scanning/alerts"`,
		"gh_code_scanning.go":      `POST /internal/repos/{owner}/{repo}/code-scanning/alerts`,
	}
	for path, needle := range needles {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(source), needle) {
			t.Fatalf("%s must not create or register code scanning alerts through the internal operator route", path)
		}
	}
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
	commitSHA := putRepoFile(t, repo.FullName, "src/zip.js", "export const zip = true;\n", "add scanned source")
	body, _ := json.Marshal(map[string]any{
		"commit_sha": commitSHA,
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

	for name, coordinate := range map[string]map[string]any{
		"missing ref":    {"commit_sha": commitSHA, "ref": "", "sarif": base64.StdEncoding.EncodeToString(sarifBytes)},
		"unknown commit": {"commit_sha": strings.Repeat("f", 40), "ref": "refs/heads/main", "sarif": base64.StdEncoding.EncodeToString(sarifBytes)},
	} {
		t.Run(name, func(t *testing.T) {
			invalidBody, _ := json.Marshal(coordinate)
			invalid, err := authedPost("/api/v3/repos/admin/cs-sarif/code-scanning/sarifs", "application/json", bytes.NewReader(invalidBody))
			if err != nil {
				t.Fatal(err)
			}
			defer invalid.Body.Close()
			if invalid.StatusCode != http.StatusUnprocessableEntity {
				body, _ := io.ReadAll(invalid.Body)
				t.Fatalf("invalid coordinate = %d body=%s, want 422", invalid.StatusCode, body)
			}
		})
	}
}

func TestCodeScanning_SARIFUploadCreatesAnalysisWithoutFindings(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "cs-sarif-clean", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}
	commitSHA := putRepoFile(t, repo.FullName, "src/clean.js", "export const clean = true;\n", "add clean source")
	sarifBytes, err := json.Marshal(map[string]any{
		"version": "2.1.0",
		"runs": []map[string]any{{
			"tool":    map[string]any{"driver": map[string]any{"name": "CodeQL"}},
			"results": []map[string]any{},
		}},
	})
	if err != nil {
		t.Fatalf("marshal SARIF: %v", err)
	}
	body, err := json.Marshal(map[string]any{
		"commit_sha": commitSHA,
		"ref":        "refs/heads/main",
		"sarif":      base64.StdEncoding.EncodeToString(sarifBytes),
	})
	if err != nil {
		t.Fatalf("marshal upload: %v", err)
	}
	resp, err := authedPost("/api/v3/repos/admin/cs-sarif-clean/code-scanning/sarifs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("upload SARIF: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		responseBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload SARIF = %d body=%s, want 202", resp.StatusCode, responseBody)
	}

	resp = authedGet(t, "/api/v3/repos/admin/cs-sarif-clean/code-scanning/analyses")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("list analyses = %d body=%s, want 200", resp.StatusCode, responseBody)
	}
	var analyses []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&analyses); err != nil {
		t.Fatalf("decode analyses: %v", err)
	}
	if len(analyses) != 1 {
		t.Fatalf("analyses = %d, want one clean analysis", len(analyses))
	}
	if analyses[0]["results_count"] != float64(0) || analyses[0]["commit_sha"] != commitSHA {
		t.Fatalf("clean analysis = %+v, want zero findings at commit %s", analyses[0], commitSHA)
	}
}

func TestCodeScanning_SARIFUploadCreatesEveryRun(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "cs-sarif-runs", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}
	commitSHA := putRepoFile(t, repo.FullName, "src/multi.js", "export const multi = true;\n", "add multi-run source")
	sarifBytes, err := json.Marshal(map[string]any{
		"version": "2.1.0",
		"runs": []map[string]any{
			{
				"tool":    map[string]any{"driver": map[string]any{"name": "CodeQL-JavaScript"}},
				"results": []map[string]any{},
			},
			{
				"tool": map[string]any{"driver": map[string]any{"name": "CodeQL-Go"}},
				"results": []map[string]any{{
					"ruleId":  "go/path-injection",
					"message": map[string]any{"text": "Path injection"},
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal SARIF: %v", err)
	}
	body, err := json.Marshal(map[string]any{
		"commit_sha": commitSHA,
		"ref":        "refs/heads/main",
		"sarif":      base64.StdEncoding.EncodeToString(sarifBytes),
	})
	if err != nil {
		t.Fatalf("marshal upload: %v", err)
	}
	resp, err := authedPost("/api/v3/repos/admin/cs-sarif-runs/code-scanning/sarifs", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("upload SARIF: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		responseBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload SARIF = %d body=%s, want 202", resp.StatusCode, responseBody)
	}

	resp = authedGet(t, "/api/v3/repos/admin/cs-sarif-runs/code-scanning/analyses")
	defer resp.Body.Close()
	var analyses []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&analyses); err != nil {
		t.Fatalf("decode analyses: %v", err)
	}
	if len(analyses) != 2 {
		t.Fatalf("analyses = %d, want one analysis for each SARIF run", len(analyses))
	}
	tools := map[string]bool{}
	for _, analysis := range analyses {
		tool, _ := analysis["tool"].(map[string]any)
		tools[tool["name"].(string)] = true
	}
	if !tools["CodeQL-JavaScript"] || !tools["CodeQL-Go"] {
		t.Fatalf("analysis tools = %+v, want both SARIF runs", tools)
	}

	resp = authedGet(t, "/api/v3/repos/admin/cs-sarif-runs/code-scanning/alerts")
	defer resp.Body.Close()
	var alerts []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&alerts); err != nil {
		t.Fatalf("decode alerts: %v", err)
	}
	if len(alerts) != 1 || alerts[0]["rule"].(map[string]any)["id"] != "go/path-injection" {
		t.Fatalf("alerts = %+v, want the finding from the second SARIF run", alerts)
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

// patchDefaultSetup PATCHes a repo's default setup and returns the response.
func patchDefaultSetup(t *testing.T, repoKey string, body map[string]any) *http.Response {
	t.Helper()
	payload, _ := json.Marshal(body)
	req, _ := http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/"+repoKey+"/code-scanning/default-setup", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch default setup: %v", err)
	}
	return resp
}

func getDefaultSetup(t *testing.T, repoKey string) map[string]any {
	t.Helper()
	resp := authedGet(t, "/api/v3/repos/"+repoKey+"/code-scanning/default-setup")
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
	return got
}

func TestCodeScanning_DefaultSetup(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "cs-default", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}
	// Real repository content: languages are detected from the git tree.
	stor := testServer.store.GitStorages[repo.FullName]
	if _, err := initRepoWithFiles(stor, repo.DefaultBranch, "init", map[string]string{
		"main.go":                  "package main\n\nfunc main() {}\n",
		"tool/script.py":           "print('hi')\n",
		".github/workflows/ci.yml": "name: ci\non: push\njobs: {}\n",
	}, repoSignature("admin", "admin@bleephub.local")); err != nil {
		t.Fatalf("init repo files: %v", err)
	}

	// Before any configuration the repo reports not-configured.
	got := getDefaultSetup(t, "admin/cs-default")
	if got["state"] != "not-configured" {
		t.Fatalf("expected not-configured before enabling, got %v", got["state"])
	}
	if langs, ok := got["languages"].([]any); !ok || len(langs) != 0 {
		t.Fatalf("expected empty languages before enabling, got %v", got["languages"])
	}
	if got["updated_at"] != nil {
		t.Fatalf("expected null updated_at before enabling, got %v", got["updated_at"])
	}

	// Changing options while default setup is disabled is a state conflict.
	resp := patchDefaultSetup(t, "admin/cs-default", map[string]any{"query_suite": "extended"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("patch without enabling: %d want 409", resp.StatusCode)
	}

	// Enable with an explicit query suite; the 200 body is the documented
	// empty object.
	resp = patchDefaultSetup(t, "admin/cs-default", map[string]any{"state": "configured", "query_suite": "extended"})
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("enable default setup: %d body=%s", resp.StatusCode, b)
	}
	var patchBody map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&patchBody); err != nil {
		t.Fatalf("decode patch body: %v", err)
	}
	resp.Body.Close()
	if len(patchBody) != 0 {
		t.Fatalf("expected empty object body, got %v", patchBody)
	}

	// GET reads back the persisted configuration with the languages
	// derived from the repository's real content.
	got = getDefaultSetup(t, "admin/cs-default")
	if got["state"] != "configured" {
		t.Fatalf("expected configured, got %v", got["state"])
	}
	if got["query_suite"] != "extended" {
		t.Fatalf("expected query_suite extended, got %v", got["query_suite"])
	}
	if got["schedule"] != "weekly" {
		t.Fatalf("expected weekly schedule, got %v", got["schedule"])
	}
	updatedAt, _ := got["updated_at"].(string)
	if _, err := time.Parse(time.RFC3339, updatedAt); err != nil {
		t.Fatalf("updated_at %q is not RFC 3339: %v", got["updated_at"], err)
	}
	langs := make([]string, 0)
	for _, l := range got["languages"].([]any) {
		langs = append(langs, l.(string))
	}
	if want := []string{"actions", "go", "python"}; !reflect.DeepEqual(langs, want) {
		t.Fatalf("derived languages = %v want %v", langs, want)
	}

	// Explicit languages override detection.
	resp = patchDefaultSetup(t, "admin/cs-default", map[string]any{"languages": []string{"go"}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch languages: %d", resp.StatusCode)
	}
	got = getDefaultSetup(t, "admin/cs-default")
	if l, _ := got["languages"].([]any); len(l) != 1 || l[0] != "go" {
		t.Fatalf("expected languages [go], got %v", got["languages"])
	}
	// query_suite persisted across the partial update.
	if got["query_suite"] != "extended" {
		t.Fatalf("expected query_suite extended after partial update, got %v", got["query_suite"])
	}

	// A language outside the documented enum is rejected.
	resp = patchDefaultSetup(t, "admin/cs-default", map[string]any{"languages": []string{"cobol"}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid language: %d want 422", resp.StatusCode)
	}

	// Disable; GET reads back not-configured with the update timestamp.
	resp = patchDefaultSetup(t, "admin/cs-default", map[string]any{"state": "not-configured"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable default setup: %d", resp.StatusCode)
	}
	got = getDefaultSetup(t, "admin/cs-default")
	if got["state"] != "not-configured" {
		t.Fatalf("expected not-configured after disable, got %v", got["state"])
	}

	// Disabling again is a state conflict.
	resp = patchDefaultSetup(t, "admin/cs-default", map[string]any{"state": "not-configured"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("double disable: %d want 409", resp.StatusCode)
	}
}

// TestCodeScanning_DefaultSetupNoLanguages verifies enabling fails when
// the repository has no CodeQL-supported languages to analyze.
func TestCodeScanning_DefaultSetupNoLanguages(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := testServer.store.CreateRepo(admin, "cs-default-empty", "", false)
	if repo == nil {
		t.Fatal("create repo failed")
	}
	resp := patchDefaultSetup(t, "admin/cs-default-empty", map[string]any{"state": "configured"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("enable on empty repo: %d want 422", resp.StatusCode)
	}
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
