package bleephub

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// /applications/{client_id}/token family + OAuth App management — token
// inspection, revocation, refresh, and OAuth App create/update against the
// GitHub-compatible app-management surface (uses Basic auth with the OAuth
// App credentials, not bearer tokens).

func TestOAuthAppCreate_AndCheckTokenWithBasicAuth(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsRoutes()
	s.registerGHAppsOAuthMgmtRoutes()

	user := s.store.UsersByLogin["admin"]
	oapp := s.store.CreateOAuthApp(user.ID, "Test OAuth App", "", "https://example.test", "https://example.test/cb")
	tok, _ := s.store.CreateUserToServerToken(user.ID, 0, oapp.ClientID, "repo", 8*time.Hour, false)

	body, _ := json.Marshal(map[string]string{"access_token": tok.Token})
	req := httptest.NewRequest("POST", "/api/v3/applications/"+oapp.ClientID+"/token", bytes.NewReader(body))
	req.Header.Set("Authorization", "Basic "+basicHeader(oapp.ClientID, oapp.ClientSecret))
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["token"] != tok.Token {
		t.Errorf("inspection echoed wrong token: %v", got["token"])
	}
}

func TestOAuthCheckToken_BadCreds(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsRoutes()
	s.registerGHAppsOAuthMgmtRoutes()

	user := s.store.UsersByLogin["admin"]
	oapp := s.store.CreateOAuthApp(user.ID, "T", "", "", "")
	tok, _ := s.store.CreateUserToServerToken(user.ID, 0, oapp.ClientID, "repo", 8*time.Hour, false)
	body, _ := json.Marshal(map[string]string{"access_token": tok.Token})

	// Wrong secret → 401
	req := httptest.NewRequest("POST", "/api/v3/applications/"+oapp.ClientID+"/token", bytes.NewReader(body))
	req.Header.Set("Authorization", "Basic "+basicHeader(oapp.ClientID, "wrong"))
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("wrong-secret status = %d, want 401", w.Code)
	}

	// No Authorization header → 401
	req = httptest.NewRequest("POST", "/api/v3/applications/"+oapp.ClientID+"/token", bytes.NewReader(body))
	w = httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("no-auth status = %d, want 401", w.Code)
	}
}

func TestOAuthResetToken_RotatesAndRevokesOld(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsRoutes()
	s.registerGHAppsOAuthMgmtRoutes()

	user := s.store.UsersByLogin["admin"]
	oapp := s.store.CreateOAuthApp(user.ID, "Rotate", "", "", "")
	tok, _ := s.store.CreateUserToServerToken(user.ID, 0, oapp.ClientID, "repo", 8*time.Hour, false)
	oldToken := tok.Token

	body, _ := json.Marshal(map[string]string{"access_token": oldToken})
	req := httptest.NewRequest("PATCH", "/api/v3/applications/"+oapp.ClientID+"/token", bytes.NewReader(body))
	req.Header.Set("Authorization", "Basic "+basicHeader(oapp.ClientID, oapp.ClientSecret))
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	newToken, _ := resp["token"].(string)
	if newToken == "" || newToken == oldToken {
		t.Errorf("reset returned same/empty token: old=%q new=%q", oldToken, newToken)
	}
	// Old token is revoked.
	if got, _ := s.store.LookupUserToServerToken(oldToken); got != nil {
		t.Error("old token still valid after reset")
	}
}

func TestOAuthRevokeToken_204(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsRoutes()
	s.registerGHAppsOAuthMgmtRoutes()

	user := s.store.UsersByLogin["admin"]
	oapp := s.store.CreateOAuthApp(user.ID, "Revoke", "", "", "")
	tok, _ := s.store.CreateUserToServerToken(user.ID, 0, oapp.ClientID, "repo", 8*time.Hour, false)

	body, _ := json.Marshal(map[string]string{"access_token": tok.Token})
	req := httptest.NewRequest("DELETE", "/api/v3/applications/"+oapp.ClientID+"/token", bytes.NewReader(body))
	req.Header.Set("Authorization", "Basic "+basicHeader(oapp.ClientID, oapp.ClientSecret))
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if got, _ := s.store.LookupUserToServerToken(tok.Token); got != nil {
		t.Error("token still valid after revoke")
	}
}

func TestOAuthRevokeGrant_KillsAllUserToServerTokensForClient(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsRoutes()
	s.registerGHAppsOAuthMgmtRoutes()

	user := s.store.UsersByLogin["admin"]
	oapp := s.store.CreateOAuthApp(user.ID, "Grant", "", "", "")
	a, _ := s.store.CreateUserToServerToken(user.ID, 0, oapp.ClientID, "repo", 8*time.Hour, true)
	b, _ := s.store.CreateUserToServerToken(user.ID, 0, oapp.ClientID, "read:org", 8*time.Hour, true)

	body, _ := json.Marshal(map[string]string{"access_token": a.Token})
	req := httptest.NewRequest("DELETE", "/api/v3/applications/"+oapp.ClientID+"/grant", bytes.NewReader(body))
	req.Header.Set("Authorization", "Basic "+basicHeader(oapp.ClientID, oapp.ClientSecret))
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	// Both A and B revoked (entire grant for clientID/userID).
	if got, _ := s.store.LookupUserToServerToken(a.Token); got != nil {
		t.Error("A still valid after grant revoke")
	}
	if got, _ := s.store.LookupUserToServerToken(b.Token); got != nil {
		t.Error("B still valid after grant revoke")
	}
}

func TestOAuthAppManagement(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHAppsOAuthMgmtRoutes()

	// Create via management endpoint (PAT auth).
	body, _ := json.Marshal(map[string]string{
		"name":         "My OAuth App",
		"description":  "test",
		"url":          "https://example.test",
		"callback_url": "https://example.test/cb",
	})
	req := httptest.NewRequest("POST", "/api/v3/bleephub/oauth-apps", bytes.NewReader(body))
	user := s.store.UsersByLogin["admin"]
	req = req.WithContext(setUserCtx(req, user))
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", w.Code, w.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created["client_id"] == "" || created["client_secret"] == "" {
		t.Error("missing client_id or client_secret in response")
	}

	// List
	req = httptest.NewRequest("GET", "/api/v3/bleephub/oauth-apps", nil)
	req = req.WithContext(setUserCtx(req, user))
	w = httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d", w.Code)
	}
	var list []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Errorf("expected 1 oauth app, got %d", len(list))
	}
	// List view must NOT include client_secret.
	if _, hasSecret := list[0]["client_secret"]; hasSecret {
		t.Error("list view leaked client_secret")
	}
}

func basicHeader(clientID, clientSecret string) string {
	return base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
}

func setUserCtx(r *http.Request, u *User) context.Context {
	return context.WithValue(r.Context(), ctxUser, u)
}
