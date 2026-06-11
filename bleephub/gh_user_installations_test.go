package bleephub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// /user/installations + /installation/token surface — listing the
// installations a user has access to + minting an installation token.
//
// The handlers go through ghHeadersMiddleware to populate ctxUser /
// ctxInstallation; the middleware reads the Authorization header. To
// avoid wiring the full middleware chain in unit tests, we inject the
// context value directly via the request's WithContext.

func runWithUser(s *Server, method, path string, user *User) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxUser, user))
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	return w
}

func TestUserInstallations_RequiresAuth(t *testing.T) {
	s := newTestServer()
	s.registerGHAppsRoutes()
	w := runRequest(s, "GET", "/api/v3/user/installations")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

// TestUserInstallations_List asserts the user-scoping contract: the list
// covers installations on the user's own account and on orgs where the user
// is an ACTIVE member — never installations on unrelated accounts.
func TestUserInstallations_List(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsRoutes()
	user := s.store.UsersByLogin["admin"]

	s.store.mu.Lock()
	other := &User{ID: s.store.NextUser, Login: "outsider", Type: "User"}
	s.store.NextUser++
	s.store.Users[other.ID] = other
	s.store.UsersByLogin[other.Login] = other
	s.store.mu.Unlock()

	memberOrg := s.store.CreateOrg(user, "octo-org", "Octo", "")
	otherOrg := s.store.CreateOrg(other, "other-org", "Other", "")
	if memberOrg == nil || otherOrg == nil {
		t.Fatal("CreateOrg failed")
	}

	app := s.store.CreateApp(user.ID, "test-app", "", nil, nil)
	s.store.CreateInstallation(app.ID, "Organization", memberOrg.ID, "octo-org", nil, nil)
	s.store.CreateInstallation(app.ID, "Organization", otherOrg.ID, "other-org", nil, nil)
	s.store.CreateInstallation(app.ID, "User", user.ID, user.Login, nil, nil)

	w := runWithUser(s, "GET", "/api/v3/user/installations", user)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		TotalCount    int              `json:"total_count"`
		Installations []map[string]any `json:"installations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalCount != 2 {
		t.Errorf("total_count = %d, want 2 (own account + member org; never the unrelated org)", resp.TotalCount)
	}
	for _, inst := range resp.Installations {
		if acct, _ := inst["account"].(map[string]any); acct != nil && acct["login"] == "other-org" {
			t.Error("installation on an org the user is not a member of leaked into /user/installations")
		}
	}
}

func TestUserInstallationRepos_List(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsRoutes()
	user := s.store.UsersByLogin["admin"]
	app := s.store.CreateApp(user.ID, "test-app", "", nil, nil)

	// Create a separate user "octocat" + a repo owned by them.
	s.store.mu.Lock()
	octo := &User{ID: s.store.NextUser, Login: "octocat", Type: "User"}
	s.store.NextUser++
	s.store.Users[octo.ID] = octo
	s.store.UsersByLogin[octo.Login] = octo
	s.store.mu.Unlock()
	s.store.CreateRepo(octo, "test-repo", "", false)

	inst := s.store.CreateInstallation(app.ID, "User", octo.ID, "octocat", nil, nil)

	w := runWithUser(s, "GET", fmt.Sprintf("/api/v3/user/installations/%d/repositories", inst.ID), user)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		TotalCount          int              `json:"total_count"`
		RepositorySelection string           `json:"repository_selection"`
		Repositories        []map[string]any `json:"repositories"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalCount != 1 {
		t.Errorf("total_count = %d, want 1", resp.TotalCount)
	}
	if resp.RepositorySelection != "all" {
		t.Errorf("repository_selection = %q, want 'all'", resp.RepositorySelection)
	}
	if len(resp.Repositories) > 0 && resp.Repositories[0]["name"] != "test-repo" {
		t.Errorf("repo name = %v", resp.Repositories[0]["name"])
	}
}

func TestUserInstallationRepos_NotFound(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsRoutes()
	user := s.store.UsersByLogin["admin"]
	w := runWithUser(s, "GET", "/api/v3/user/installations/9999/repositories", user)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestRevokeInstallationToken_DeletesAndReturns204(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsRoutes()
	app := s.store.CreateApp(1, "test-app", "", nil, nil)
	inst := s.store.CreateInstallation(app.ID, "User", 1, "admin", nil, nil)
	tok := s.store.CreateInstallationToken(inst.ID, app.ID, nil, nil)

	req := httptest.NewRequest("DELETE", "/api/v3/installation/token", nil)
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body = %s", w.Code, w.Body.String())
	}
	// Token must be gone.
	if got, _ := s.store.LookupInstallationToken(tok.Token); got != nil {
		t.Errorf("token still present after revoke")
	}
}

func TestRevokeInstallationToken_RejectsUnknownToken(t *testing.T) {
	s := newTestServer()
	s.registerGHAppsRoutes()
	req := httptest.NewRequest("DELETE", "/api/v3/installation/token", nil)
	req.Header.Set("Authorization", "Bearer ghs_unknown")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestRevokeInstallationToken_RejectsNonGhsToken(t *testing.T) {
	s := newTestServer()
	s.registerGHAppsRoutes()
	req := httptest.NewRequest("DELETE", "/api/v3/installation/token", nil)
	req.Header.Set("Authorization", "Bearer bph_some_user_pat")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (only ghs_* tokens revocable)", w.Code)
	}
}
