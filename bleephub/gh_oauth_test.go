package bleephub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// OAuth web flow — /login/oauth/authorize redirects + /login/oauth/access_token
// code exchange + device-code polling against the GitHub-compatible OAuth
// surface (uses RS256 client_assertion JWT, not client_secret).

func TestOAuth_AuthorizeRendersForm(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHOAuthRoutes()

	w := runRequest(s, "GET", "/login/oauth/authorize?client_id=Iv1.abc&redirect_uri=http://callback/&scope=repo&state=xyz")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<form") {
		t.Errorf("response missing <form> element: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Iv1.abc") {
		t.Errorf("response missing client_id")
	}
}

func TestOAuth_AuthorizeAuto1ImmediateRedirect(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHOAuthRoutes()

	w := runRequest(s, "GET", "/login/oauth/authorize?client_id=Iv1.x&redirect_uri=http://cb/&state=ST&auto=1")
	if w.Code != http.StatusFound {
		t.Fatalf("auto=1 status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	parsed, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	if parsed.Query().Get("code") == "" {
		t.Errorf("Location missing code: %s", loc)
	}
	if parsed.Query().Get("state") != "ST" {
		t.Errorf("Location state = %q, want ST", parsed.Query().Get("state"))
	}
}

func TestOAuth_AuthorizeRequiresClientIDAndRedirectURI(t *testing.T) {
	s := newTestServer()
	s.registerGHOAuthRoutes()

	w := runRequest(s, "GET", "/login/oauth/authorize?client_id=x")
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing redirect_uri: status = %d, want 400", w.Code)
	}
}

func TestOAuth_AuthorizePost_FormApproval(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHOAuthRoutes()

	form := url.Values{}
	form.Set("client_id", "Iv1.web")
	form.Set("redirect_uri", "http://cb/")
	form.Set("scope", "repo")
	form.Set("state", "STATE-1")

	req := httptest.NewRequest("POST", "/login/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc, _ := url.Parse(w.Header().Get("Location"))
	if loc.Query().Get("state") != "STATE-1" {
		t.Errorf("state lost in redirect: %v", loc)
	}
}

func TestOAuth_WebFlow_AccessTokenExchange(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHOAuthRoutes()

	// Step 1: authorize → get code from Location header.
	authW := runRequest(s, "GET", "/login/oauth/authorize?client_id=Iv1.test&redirect_uri=http://cb/&scope=repo&state=S&auto=1")
	if authW.Code != http.StatusFound {
		t.Fatalf("authorize status = %d", authW.Code)
	}
	loc, _ := url.Parse(authW.Header().Get("Location"))
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatal("no code in redirect")
	}

	// Step 2: exchange code for access token.
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", "Iv1.test")
	req := httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("token-exchange status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != "" {
		t.Fatalf("got error: %s", resp.Error)
	}
	if resp.AccessToken == "" {
		t.Errorf("access_token empty")
	}
	if resp.TokenType != "bearer" {
		t.Errorf("token_type = %q, want bearer", resp.TokenType)
	}
}

func TestOAuth_WebFlow_CodeIsOneTimeUse(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHOAuthRoutes()

	// First exchange — succeeds.
	authW := runRequest(s, "GET", "/login/oauth/authorize?client_id=x&redirect_uri=http://cb/&auto=1")
	loc, _ := url.Parse(authW.Header().Get("Location"))
	code := loc.Query().Get("code")

	form := url.Values{}
	form.Set("code", code)
	req := httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first exchange status = %d", w.Code)
	}

	// Second exchange with the SAME code — must fail.
	req2 := httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(form.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w2 := httptest.NewRecorder()
	s.mux.ServeHTTP(w2, req2)
	body := w2.Body.String()
	if !strings.Contains(body, "bad_verification_code") {
		t.Errorf("re-using code returned: %s", body)
	}
}

func TestOAuth_DeviceFlow_StillWorks(t *testing.T) {
	// Web-flow code-exchange must not regress the older device-code flow
	// (both routes share the /login/oauth/access_token endpoint).
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHOAuthRoutes()

	form := url.Values{}
	form.Set("scope", "repo")
	req := httptest.NewRequest("POST", "/login/device/code", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("device code status = %d", w.Code)
	}
	var dc struct {
		DeviceCode string `json:"device_code"`
	}
	json.Unmarshal(w.Body.Bytes(), &dc)
	if dc.DeviceCode == "" {
		t.Fatal("missing device_code in response")
	}

	form2 := url.Values{}
	form2.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form2.Set("device_code", dc.DeviceCode)
	req2 := httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(form2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w2 := httptest.NewRecorder()
	s.mux.ServeHTTP(w2, req2)

	var tokResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	json.Unmarshal(w2.Body.Bytes(), &tokResp)
	if tokResp.Error != "" {
		t.Errorf("device token error: %s", tokResp.Error)
	}
	if tokResp.AccessToken == "" {
		t.Errorf("device flow access_token empty")
	}
}
