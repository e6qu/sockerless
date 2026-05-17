package bleephub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Checks API parity — check-run + check-suite CRUD against the
// GitHub-compatible /repos/{}/check-runs surface.

func TestCheckRunLifecycle(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsRoutes()
	s.registerGHRepoRoutes()
	s.registerGHChecksRoutes()

	user := s.store.UsersByLogin["admin"]
	app := s.store.CreateApp(user.ID, "Checks App", "", map[string]string{"checks": "write"}, nil)
	inst := s.store.CreateInstallation(app.ID, "User", user.ID, user.Login, map[string]string{"checks": "write"}, nil)
	tok := s.store.CreateInstallationToken(inst.ID, app.ID, map[string]string{"checks": "write"}, nil)

	s.store.CreateRepo(user, "checks-target", "", false)
	headSHA := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

	doReq := func(method, path string, body []byte) *httptest.ResponseRecorder {
		var bodyR *bytes.Reader
		if body != nil {
			bodyR = bytes.NewReader(body)
		}
		var req *http.Request
		if bodyR != nil {
			req = httptest.NewRequest(method, path, bodyR)
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("Authorization", "Bearer "+tok.Token)
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
		return w
	}

	// CREATE check run
	body, _ := json.Marshal(map[string]any{
		"name":        "go test",
		"head_sha":    headSHA,
		"status":      "in_progress",
		"details_url": "https://example.test/run/1",
	})
	w := doReq("POST", "/api/v3/repos/admin/checks-target/check-runs", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", w.Code, w.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	runID := int(created["id"].(float64))
	suiteID := created["check_suite"].(map[string]any)["id"]
	if suiteID.(float64) == 0 {
		t.Error("create did not associate a check_suite")
	}

	// GET check run
	w = doReq("GET", fmt.Sprintf("/api/v3/repos/admin/checks-target/check-runs/%d", runID), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d body = %s", w.Code, w.Body.String())
	}

	// PATCH check run → completed
	completedAt := time.Now()
	body, _ = json.Marshal(map[string]any{
		"status":       "completed",
		"conclusion":   "success",
		"completed_at": completedAt,
		"output": map[string]any{
			"title":   "all green",
			"summary": "5 passed, 0 failed",
		},
	})
	w = doReq("PATCH", fmt.Sprintf("/api/v3/repos/admin/checks-target/check-runs/%d", runID), body)
	if w.Code != http.StatusOK {
		t.Fatalf("patch status = %d body = %s", w.Code, w.Body.String())
	}
	var patched map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &patched)
	if patched["status"] != "completed" || patched["conclusion"] != "success" {
		t.Errorf("patch did not update status/conclusion: %v / %v", patched["status"], patched["conclusion"])
	}

	// LIST by commit
	w = doReq("GET", fmt.Sprintf("/api/v3/repos/admin/checks-target/commits/%s/check-runs", headSHA), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list by commit status = %d body = %s", w.Code, w.Body.String())
	}
	var listResp struct {
		TotalCount int              `json:"total_count"`
		CheckRuns  []map[string]any `json:"check_runs"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &listResp)
	if listResp.TotalCount != 1 {
		t.Errorf("expected 1 check run, got %d", listResp.TotalCount)
	}

	// LIST suites by commit
	w = doReq("GET", fmt.Sprintf("/api/v3/repos/admin/checks-target/commits/%s/check-suites", headSHA), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list suites status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestCheckRunRequiresChecksScope(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsRoutes()
	s.registerGHRepoRoutes()
	s.registerGHChecksRoutes()

	user := s.store.UsersByLogin["admin"]
	// Installation has issues:write but NOT checks.
	app := s.store.CreateApp(user.ID, "Wrong Scope App", "", map[string]string{"issues": "write"}, nil)
	inst := s.store.CreateInstallation(app.ID, "User", user.ID, user.Login, map[string]string{"issues": "write"}, nil)
	tok := s.store.CreateInstallationToken(inst.ID, app.ID, map[string]string{"issues": "write"}, nil)

	s.store.CreateRepo(user, "scope-target", "", false)

	body, _ := json.Marshal(map[string]any{
		"name":     "t",
		"head_sha": "0000000000000000000000000000000000000000",
	})
	req := httptest.NewRequest("POST", "/api/v3/repos/admin/scope-target/check-runs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without checks:write, got %d body=%s", w.Code, w.Body.String())
	}
}
