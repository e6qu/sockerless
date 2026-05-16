package bleephub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Phase 153 (P153.6) — permission enforcement decorator.

func TestRequirePerm_GhsToken_PermsGate(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsRoutes()
	s.registerGHRepoRoutes()
	s.registerGHIssueRoutes()

	user := s.store.UsersByLogin["admin"]
	app := s.store.CreateApp(user.ID, "Perms App", "", map[string]string{
		"issues":   "read",
		"contents": "read",
	}, nil)
	inst := s.store.CreateInstallation(app.ID, "User", user.ID, user.Login, app.Permissions, nil)

	// Mint a ghs_ token carrying the installation's perms (contents:read, issues:read only).
	tok := s.store.CreateInstallationToken(inst.ID, app.ID, app.Permissions, nil)

	// Prep a target repo (created by the admin PAT path so we have something to mutate).
	repo := s.store.CreateRepo(user, "perms-target", "", false)
	_ = repo

	// Try to CREATE an issue via ghs_ token without issues:write → 403.
	body, _ := json.Marshal(map[string]string{"title": "test"})
	req := httptest.NewRequest("POST", "/api/v3/repos/admin/perms-target/issues", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 (issues:read can't create issues), got %d body=%s", w.Code, w.Body.String())
	}

	// Upgrade installation to issues:write; the ghs_ token's snapshot still
	// has the OLD perms (real GH behaviour — minted tokens are immutable).
	// Verify by minting a fresh ghs_ token after the upgrade.
	s.store.mu.Lock()
	inst.Permissions["issues"] = "write"
	s.store.mu.Unlock()
	fresh := s.store.CreateInstallationToken(inst.ID, app.ID, inst.Permissions, nil)

	req = httptest.NewRequest("POST", "/api/v3/repos/admin/perms-target/issues", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+fresh.Token)
	w = httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 with issues:write, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestRequirePerm_PATBypass(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHRepoRoutes()
	s.registerGHIssueRoutes()

	user := s.store.UsersByLogin["admin"]
	s.store.CreateRepo(user, "pat-target", "", false)

	// Use the seeded admin PAT (bph_-prefixed via Tokens map) — should bypass.
	body, _ := json.Marshal(map[string]string{"title": "test"})
	req := httptest.NewRequest("POST", "/api/v3/repos/admin/pat-target/issues", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer ghp_0000000000000000000000000000000000000000")
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("PAT bypass failed: %d body=%s", w.Code, w.Body.String())
	}
}

func TestRequirePerm_GhuToken_AppInstallationPerms(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHRepoRoutes()
	s.registerGHIssueRoutes()

	user := s.store.UsersByLogin["admin"]
	app := s.store.CreateApp(user.ID, "Ghu App", "", nil, nil)
	s.store.CreateInstallation(app.ID, "User", user.ID, user.Login, map[string]string{"issues": "write"}, nil)
	s.store.CreateRepo(user, "ghu-target", "", false)

	// Mint a ghu_ token for this app + user.
	tok, _ := s.store.CreateUserToServerToken(user.ID, app.ID, "", "", 8*time.Hour, false)

	body, _ := json.Marshal(map[string]string{"title": "test"})
	req := httptest.NewRequest("POST", "/api/v3/repos/admin/ghu-target/issues", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok.Token)
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("ghu_ token with issues:write installation expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestRequirePerm_GhoToken_ClassicScopesMap(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsOAuthMgmtRoutes()
	s.registerGHRepoRoutes()
	s.registerGHIssueRoutes()

	user := s.store.UsersByLogin["admin"]
	oapp := s.store.CreateOAuthApp(user.ID, "ScopeMap", "", "", "")
	tokRepo, _ := s.store.CreateUserToServerToken(user.ID, 0, oapp.ClientID, "repo", 8*time.Hour, false)
	tokReadOrg, _ := s.store.CreateUserToServerToken(user.ID, 0, oapp.ClientID, "read:org", 8*time.Hour, false)

	s.store.CreateRepo(user, "gho-target", "", false)
	body, _ := json.Marshal(map[string]string{"title": "test"})

	// "repo" classic scope covers issues:write → 201
	req := httptest.NewRequest("POST", "/api/v3/repos/admin/gho-target/issues", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokRepo.Token)
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("repo scope expected 201, got %d body=%s", w.Code, w.Body.String())
	}

	// "read:org" does NOT cover issues:write → 403
	req = httptest.NewRequest("POST", "/api/v3/repos/admin/gho-target/issues", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokReadOrg.Token)
	w = httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("read:org expected 403, got %d body=%s", w.Code, w.Body.String())
	}
}
