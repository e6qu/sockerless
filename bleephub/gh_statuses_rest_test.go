package bleephub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Commit Statuses REST API — create, list, and combined status.

func TestCommitStatuses_CreateListCombined(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHStatusesRoutes()

	user := s.store.UsersByLogin["admin"]
	_ = s.store.CreateRepo(user, "status-repo", "", false)

	post := func(sha string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/v3/repos/admin/status-repo/statuses/"+sha, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
		return w
	}

	sha := "deadbeef"

	// First status: pending.
	b1, _ := json.Marshal(map[string]string{
		"state":       "pending",
		"target_url":  "https://ci.example.com/1",
		"description": "build running",
		"context":     "ci/build",
	})
	w := post(sha, b1)
	if w.Code != http.StatusCreated {
		t.Fatalf("create pending: %d body=%s", w.Code, w.Body.String())
	}
	var pending map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &pending)
	if pending["state"] != "pending" || pending["context"] != "ci/build" {
		t.Errorf("pending status shape: %v", pending)
	}
	if _, ok := pending["avatar_url"].(string); !ok {
		t.Errorf("pending status avatar_url = %T, want string", pending["avatar_url"])
	}

	// Second status for same context: success (overrides pending in combined).
	b2, _ := json.Marshal(map[string]string{
		"state":       "success",
		"target_url":  "https://ci.example.com/2",
		"description": "build ok",
		"context":     "ci/build",
	})
	w = post(sha, b2)
	if w.Code != http.StatusCreated {
		t.Fatalf("create success: %d body=%s", w.Code, w.Body.String())
	}

	// Third status: failure in a different context.
	b3, _ := json.Marshal(map[string]string{
		"state":       "failure",
		"target_url":  "https://tests.example.com/1",
		"description": "tests failed",
		"context":     "ci/tests",
	})
	w = post(sha, b3)
	if w.Code != http.StatusCreated {
		t.Fatalf("create failure: %d body=%s", w.Code, w.Body.String())
	}

	// List statuses should be newest-first and contain all three.
	req := httptest.NewRequest("GET", "/api/v3/repos/admin/status-repo/commits/"+sha+"/statuses", nil)
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w = httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list statuses: %d body=%s", w.Code, w.Body.String())
	}
	var list []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 3 {
		t.Fatalf("list len = %d, want 3", len(list))
	}
	if list[0]["state"] != "failure" {
		t.Errorf("newest status state = %v, want failure", list[0]["state"])
	}

	// Combined status should reflect failure (worst of latest per context).
	req = httptest.NewRequest("GET", "/api/v3/repos/admin/status-repo/commits/"+sha+"/status", nil)
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w = httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("combined status: %d body=%s", w.Code, w.Body.String())
	}
	var combined map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &combined)
	if combined["state"] != "failure" {
		t.Errorf("combined state = %v, want failure", combined["state"])
	}
	if combined["total_count"] != float64(2) {
		t.Errorf("combined total_count = %v, want 2", combined["total_count"])
	}
	if combined["sha"] != sha {
		t.Errorf("combined sha = %v, want %s", combined["sha"], sha)
	}
	statuses, _ := combined["statuses"].([]any)
	if len(statuses) != 2 {
		t.Errorf("combined statuses len = %d, want 2", len(statuses))
	}
}

func TestCommitStatuses_MissingState422(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHStatusesRoutes()

	user := s.store.UsersByLogin["admin"]
	_ = s.store.CreateRepo(user, "status-repo2", "", false)

	body, _ := json.Marshal(map[string]string{"description": "missing state"})
	req := httptest.NewRequest("POST", "/api/v3/repos/admin/status-repo2/statuses/abc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("missing state: %d", w.Code)
	}
}
