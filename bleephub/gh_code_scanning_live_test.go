package bleephub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// Live-server shape test for code scanning. Exercises every route through the
// shared TestMain server so the OpenAPI response-shape validator observes them.
func TestLiveCodeScanning_FullFlow(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	testServer.store.CreateRepo(admin, "live-code-scanning", "", false)

	alert := seedCodeScanningAlert(t, "admin", "live-code-scanning", "live-rule", "error", "CodeQL")
	alertNumber := int(alert["number"].(float64))

	// List alerts
	resp := authedGet(t, "/api/v3/repos/admin/live-code-scanning/code-scanning/alerts")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list alerts: %d %s", resp.StatusCode, b)
	}
	json.NewDecoder(resp.Body).Decode(&[]map[string]any{})
	resp.Body.Close()

	// Get alert
	resp = authedGet(t, "/api/v3/repos/admin/live-code-scanning/code-scanning/alerts/"+itoa(alertNumber))
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get alert: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// List instances
	resp = authedGet(t, "/api/v3/repos/admin/live-code-scanning/code-scanning/alerts/"+itoa(alertNumber)+"/instances")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list instances: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Patch alert
	patchBody, _ := json.Marshal(map[string]any{"state": "dismissed", "dismissed_reason": "used_in_tests"})
	req, _ := http.NewRequest("PATCH", testBaseURL+"/api/v3/repos/admin/live-code-scanning/code-scanning/alerts/"+itoa(alertNumber), bytes.NewReader(patchBody))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch alert: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("patch alert: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// SARIF upload
	sarif := map[string]any{
		"version": "2.1.0",
		"runs": []map[string]any{
			{
				"tool": map[string]any{
					"driver": map[string]any{"name": "CodeQL"},
				},
				"results": []map[string]any{
					{
						"ruleId":  "live/sarif-rule",
						"message": map[string]any{"text": "SARIF-found issue"},
						"locations": []map[string]any{
							{
								"physicalLocation": map[string]any{
									"artifactLocation": map[string]any{"uri": "live.go"},
									"region":           map[string]any{"startLine": 1, "endLine": 1, "startColumn": 1, "endColumn": 5},
								},
							},
						},
					},
				},
			},
		},
	}
	sarifBytes, _ := json.Marshal(sarif)
	commitSHA := putRepoFile(t, "admin/live-code-scanning", "live.go", "package live\n", "add live source")
	sarifBody, _ := json.Marshal(map[string]any{
		"commit_sha": commitSHA,
		"ref":        "refs/heads/main",
		"sarif":      base64.StdEncoding.EncodeToString(sarifBytes),
	})
	resp, err = authedPost("/api/v3/repos/admin/live-code-scanning/code-scanning/sarifs", "application/json", bytes.NewReader(sarifBody))
	if err != nil {
		t.Fatalf("sarif upload: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("sarif upload: %d %s", resp.StatusCode, b)
	}
	var upload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&upload); err != nil {
		t.Fatalf("decode sarif upload: %v", err)
	}
	resp.Body.Close()
	uploadID := upload["id"].(string)

	// Get SARIF upload
	resp = authedGet(t, "/api/v3/repos/admin/live-code-scanning/code-scanning/sarifs/"+uploadID)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get sarif upload: %d %s", resp.StatusCode, b)
	}
	var gotUpload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&gotUpload); err != nil {
		t.Fatalf("decode sarif upload status: %v", err)
	}
	resp.Body.Close()
	if gotUpload["processing_status"] != "complete" {
		t.Fatalf("expected processing_status complete, got %v", gotUpload["processing_status"])
	}

	// List analyses
	resp = authedGet(t, "/api/v3/repos/admin/live-code-scanning/code-scanning/analyses")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list analyses: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	// Default setup
	resp = authedGet(t, "/api/v3/repos/admin/live-code-scanning/code-scanning/default-setup")
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("get default setup: %d %s", resp.StatusCode, b)
	}
	resp.Body.Close()
}
