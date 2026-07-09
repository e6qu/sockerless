package bleephub

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// unauthedRequest drives a route through the /api middleware chain WITHOUT
// any Authorization header, so requirePerm sees no user. Mirrors
// runAuthedRequest (gh_actions_test.go) minus the token.
func unauthedRequest(s *Server, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	return w
}

// TestActionsWriteEndpointsRequireAuth guards BUG-1591: the workflow-run
// write endpoints were registered without requirePerm, so an anonymous
// caller could dispatch / cancel / rerun / delete runs and delete runners.
// Real GitHub requires actions:write (administration:write for runners);
// every one of these must reject an unauthenticated request with 401.
func TestActionsWriteEndpointsRequireAuth(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	s.registerAgentRoutes()
	s.registerGHWorkflowsRoutes()
	wf, _ := seedRun(t, s, "octo/repo", "running", "")
	file := s.store.RegisterWorkflowFile("octo/repo", ".github/workflows/ci.yml", "ci", sampleWorkflowYAML, "submitted")

	cases := []struct {
		name, method, path string
	}{
		{"dispatch", "POST", fmt.Sprintf("/api/v3/repos/octo/repo/actions/workflows/%d/dispatches", file.ID)},
		{"cancel", "POST", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d/cancel", wf.RunID)},
		{"rerun", "POST", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d/rerun", wf.RunID)},
		{"delete-run", "DELETE", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d", wf.RunID)},
		{"delete-runner", "DELETE", "/api/v3/repos/octo/repo/actions/runners/1"},
		// Runner setup + listing require administration (read/write); an
		// anonymous caller must never mint a registration/removal/JIT token
		// or enumerate the repo's runners.
		{"registration-token", "POST", "/api/v3/repos/octo/repo/actions/runners/registration-token"},
		{"remove-token", "POST", "/api/v3/repos/octo/repo/actions/runners/remove-token"},
		{"generate-jitconfig", "POST", "/api/v3/repos/octo/repo/actions/runners/generate-jitconfig"},
		{"list-runners", "GET", "/api/v3/repos/octo/repo/actions/runners"},
		{"get-runner", "GET", "/api/v3/repos/octo/repo/actions/runners/1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := unauthedRequest(s, c.method, c.path)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s unauthenticated = %d, want 401", c.method, c.path, w.Code)
			}
		})
	}

	// The run must survive every rejected mutation.
	if s.findWorkflowByRunID(wf.RunID) == nil {
		t.Error("run was mutated/deleted by an unauthenticated request")
	}
}

// TestInternalEndpointsRequireAuth guards BUG-1593: the /internal/* operator
// surface was served with no server-side token check (the UI login was a
// client-side guard only). The middleware must 401 anonymous callers and
// admit the admin token; /health stays open for liveness probes.
func TestInternalEndpointsRequireAuth(t *testing.T) {
	s := newTestServer()
	s.metrics = NewMetrics()
	s.registerMgmtRoutes()
	s.mux.HandleFunc("GET /internal/metrics", s.handleInternalMetrics)
	s.mux.HandleFunc("GET /internal/status", s.handleInternalStatus)
	s.mux.HandleFunc("GET /internal/storage", s.handleInternalStorage)
	s.mux.HandleFunc("GET /health", s.handleHealth)
	handler := s.internalAuthMiddleware(s.mux)

	paths := []string{
		"/internal/metrics", "/internal/status", "/internal/storage",
		"/internal/workflows", "/internal/sessions", "/internal/repos",
		"/internal/oauth/state",
	}
	for _, p := range paths {
		t.Run("anon"+p, func(t *testing.T) {
			req := httptest.NewRequest("GET", p, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("GET %s anonymous = %d, want 401", p, w.Code)
			}
		})
		t.Run("authed"+p, func(t *testing.T) {
			req := httptest.NewRequest("GET", p, nil)
			req.Header.Set("Authorization", "Bearer "+defaultToken)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code == http.StatusUnauthorized {
				t.Errorf("GET %s with admin token = 401, want admitted", p)
			}
		})
	}

	// Liveness probe stays open.
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /health = %d, want 200 (must not require auth)", w.Code)
	}

	// A bogus token is rejected.
	req = httptest.NewRequest("GET", "/internal/metrics", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("GET /internal/metrics with bogus token = %d, want 401", w.Code)
	}
}
