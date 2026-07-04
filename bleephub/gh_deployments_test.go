package bleephub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Deployments + Environments + repository_dispatch APIs — CRUD against the
// /repos/{}/deployments + /repos/{}/environments surfaces and the
// repository_dispatch trigger that emits a custom-event webhook.

func do(s *Server, method, path string, body []byte) *httptest.ResponseRecorder {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	return w
}

func TestDeployments_Lifecycle(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHDeploymentsRoutes()

	user := s.store.UsersByLogin["admin"]
	_ = s.store.CreateRepo(user, "dep-repo", "", false)

	body, _ := json.Marshal(map[string]any{
		"ref":                    "main",
		"environment":            "staging",
		"description":            "smoke deploy",
		"production_environment": false,
	})
	w := do(s, "POST", "/api/v3/repos/admin/dep-repo/deployments", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d body=%s", w.Code, w.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	depID := int(created["id"].(float64))
	if created["environment"] != "staging" {
		t.Errorf("env = %v", created["environment"])
	}

	// List → 1
	w = do(s, "GET", "/api/v3/repos/admin/dep-repo/deployments", nil)
	var list []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Errorf("list len = %d", len(list))
	}

	// Status: pending → in_progress → success.
	for _, state := range []string{"pending", "in_progress", "success"} {
		statusBody, _ := json.Marshal(map[string]any{"state": state, "description": state + " step"})
		w = do(s, "POST", "/api/v3/repos/admin/dep-repo/deployments/"+itoa(depID)+"/statuses", statusBody)
		if w.Code != http.StatusCreated {
			t.Errorf("status %s: %d body=%s", state, w.Code, w.Body.String())
		}
	}

	// List statuses → 3
	w = do(s, "GET", "/api/v3/repos/admin/dep-repo/deployments/"+itoa(depID)+"/statuses", nil)
	var statusList []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &statusList)
	if len(statusList) != 3 {
		t.Errorf("statuses len = %d", len(statusList))
	}

	// Environment auto-created
	w = do(s, "GET", "/api/v3/repos/admin/dep-repo/environments", nil)
	var envs map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &envs)
	if envs["total_count"].(float64) < 1 {
		t.Errorf("env count = %v", envs["total_count"])
	}

	// PUT a new environment
	w = do(s, "PUT", "/api/v3/repos/admin/dep-repo/environments/production", []byte("{}"))
	if w.Code != http.StatusOK {
		t.Fatalf("put env: %d", w.Code)
	}
	w = do(s, "GET", "/api/v3/repos/admin/dep-repo/environments/production", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get env: %d", w.Code)
	}

	// Delete env
	w = do(s, "DELETE", "/api/v3/repos/admin/dep-repo/environments/production", nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("del env: %d", w.Code)
	}

	// Delete deployment
	w = do(s, "DELETE", "/api/v3/repos/admin/dep-repo/deployments/"+itoa(depID), nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("del dep: %d", w.Code)
	}
}

// TestUpsertEnvironmentBodyValidation: PUT /environments/{name} accepts an
// absent body (environment with no protection config) but rejects malformed
// JSON with 400 like real GitHub.
func TestUpsertEnvironmentBodyValidation(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHDeploymentsRoutes()

	user := s.store.UsersByLogin["admin"]
	_ = s.store.CreateRepo(user, "env-body-repo", "", false)

	w := do(s, "PUT", "/api/v3/repos/admin/env-body-repo/environments/staging", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("put env with no body: %d body=%s", w.Code, w.Body.String())
	}

	w = do(s, "PUT", "/api/v3/repos/admin/env-body-repo/environments/staging", []byte(`{"wait_timer": `))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("put env with malformed JSON: %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestRepositoryDispatch(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHActionsExtrasRoutes()

	user := s.store.UsersByLogin["admin"]
	_ = s.store.CreateRepo(user, "dispatch-repo", "", false)

	body, _ := json.Marshal(map[string]any{
		"event_type":     "deploy",
		"client_payload": map[string]any{"version": "1.2.3"},
	})
	w := do(s, "POST", "/api/v3/repos/admin/dispatch-repo/dispatches", body)
	if w.Code != http.StatusNoContent {
		t.Fatalf("dispatch: %d body=%s", w.Code, w.Body.String())
	}

	// Missing event_type → 422
	bad, _ := json.Marshal(map[string]any{})
	w = do(s, "POST", "/api/v3/repos/admin/dispatch-repo/dispatches", bad)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("missing event_type: %d", w.Code)
	}
}
