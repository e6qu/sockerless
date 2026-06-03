package bleephub

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// OAuth web flow — /login/oauth/authorize redirects + /login/oauth/access_token
// code exchange + device-code polling against the GitHub-compatible OAuth
// surface (uses RS256 client_assertion JWT, not client_secret).

// doLogin posts to POST /login and returns a cookie jar carrying the session.
func doLogin(t *testing.T, s *Server, login string) http.CookieJar {
	t.Helper()
	form := url.Values{}
	form.Set("login", login)
	form.Set("password", "anything")
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusFound && w.Code != http.StatusOK {
		t.Fatalf("POST /login status = %d, want 200 or 302", w.Code)
	}
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse("http://bleephub.test")
	jar.SetCookies(u, w.Result().Cookies())
	return jar
}

// requestWithJar sends a request through s.mux carrying cookies from jar.
func requestWithJar(s *Server, method, path string, body string, contentType string, jar http.CookieJar) *httptest.ResponseRecorder {
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	var req *http.Request
	if bodyReader != nil {
		req = httptest.NewRequest(method, path, bodyReader)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if jar != nil {
		u, _ := url.Parse("http://bleephub.test")
		for _, c := range jar.Cookies(u) {
			req.AddCookie(c)
		}
	}
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	return w
}

// extractCSRF reads the authenticity_token from a consent form body.
func extractCSRF(t *testing.T, body string) string {
	t.Helper()
	const marker = `name="authenticity_token" value="`
	idx := strings.Index(body, marker)
	if idx == -1 {
		t.Fatalf("authenticity_token not found in form body:\n%s", body)
	}
	rest := body[idx+len(marker):]
	end := strings.Index(rest, `"`)
	if end == -1 {
		t.Fatalf("authenticity_token value not terminated")
	}
	return rest[:end]
}

func TestOAuth_LoginPage_RendersForm(t *testing.T) {
	s := newTestServer()
	s.registerGHOAuthRoutes()

	w := runRequest(s, "GET", "/login")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /login status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<form") {
		t.Errorf("login page missing <form>")
	}
}

func TestOAuth_LoginPost_SetsSessionCookie(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHOAuthRoutes()

	jar := doLogin(t, s, "admin")
	u, _ := url.Parse("http://bleephub.test")
	cookies := jar.Cookies(u)
	found := false
	for _, c := range cookies {
		if c.Name == "_gh_sess" && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("_gh_sess cookie not set after POST /login")
	}
}

func TestOAuth_LoginPost_UnknownUserReturns401(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHOAuthRoutes()

	form := url.Values{}
	form.Set("login", "nobody")
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown user: status = %d, want 401", w.Code)
	}
}

func TestOAuth_AuthorizeRedirectsToLoginWithoutSession(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHOAuthRoutes()

	w := runRequest(s, "GET", "/login/oauth/authorize?client_id=Iv1.abc&redirect_uri=http://callback/&scope=repo&state=xyz")
	if w.Code != http.StatusFound {
		t.Fatalf("no-session authorize: status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "/login") {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

func TestOAuth_AuthorizeRendersFormWithCSRF(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHOAuthRoutes()

	jar := doLogin(t, s, "admin")
	w := requestWithJar(s, "GET", "/login/oauth/authorize?client_id=Iv1.abc&redirect_uri=http://callback/&scope=repo&state=xyz", "", "", jar)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<form") {
		t.Errorf("consent form missing <form> element")
	}
	if !strings.Contains(body, "authenticity_token") {
		t.Errorf("consent form missing authenticity_token field")
	}
	if !strings.Contains(body, "Iv1.abc") {
		t.Errorf("consent form missing client_id")
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

func TestOAuth_ConformantWebFlow_BindsCodeToSessionUser(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	// Seed a second non-admin user.
	s.store.mu.Lock()
	alice := &User{ID: s.store.NextUser, Login: "alice", Type: "User", SiteAdmin: false}
	s.store.NextUser++
	s.store.Users[alice.ID] = alice
	s.store.UsersByLogin[alice.Login] = alice
	s.store.mu.Unlock()
	s.registerGHOAuthRoutes()

	// Step 1: login as alice.
	jar := doLogin(t, s, "alice")

	// Step 2: GET authorize → consent form with CSRF.
	authorizeURL := "/login/oauth/authorize?client_id=Iv1.test&redirect_uri=http://cb/&scope=repo&state=S"
	w := requestWithJar(s, "GET", authorizeURL, "", "", jar)
	if w.Code != http.StatusOK {
		t.Fatalf("GET authorize status = %d, want 200", w.Code)
	}
	csrf := extractCSRF(t, w.Body.String())

	// Step 3: POST authorize with CSRF → 302 with code.
	form := url.Values{}
	form.Set("authenticity_token", csrf)
	form.Set("client_id", "Iv1.test")
	form.Set("redirect_uri", "http://cb/")
	form.Set("scope", "repo")
	form.Set("state", "S")
	w2 := requestWithJar(s, "POST", "/login/oauth/authorize", form.Encode(), "application/x-www-form-urlencoded", jar)
	if w2.Code != http.StatusFound {
		t.Fatalf("POST authorize status = %d, want 302 (body: %s)", w2.Code, w2.Body.String())
	}
	loc, _ := url.Parse(w2.Header().Get("Location"))
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatal("no code in redirect Location")
	}
	if loc.Query().Get("state") != "S" {
		t.Errorf("state lost in redirect: %v", loc)
	}

	// Step 4: exchange code for token.
	exchForm := url.Values{}
	exchForm.Set("code", code)
	exchForm.Set("client_id", "Iv1.test")
	req := httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(exchForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w3 := httptest.NewRecorder()
	s.mux.ServeHTTP(w3, req)
	if w3.Code != http.StatusOK {
		t.Fatalf("token exchange status = %d", w3.Code)
	}
	var tokResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(w3.Body.Bytes(), &tokResp); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if tokResp.Error != "" {
		t.Fatalf("token error: %s", tokResp.Error)
	}
	if tokResp.AccessToken == "" {
		t.Errorf("access_token empty")
	}

	// Step 5: verify the token belongs to alice, not admin.
	_, user := s.store.LookupToken(tokResp.AccessToken)
	if user == nil {
		t.Fatal("token not found in store")
	}
	if user.Login != "alice" {
		t.Errorf("token user = %q, want alice", user.Login)
	}
}

func TestOAuth_AuthorizeApprove_RejectsNoSession(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHOAuthRoutes()

	form := url.Values{}
	form.Set("authenticity_token", "any")
	form.Set("client_id", "Iv1.x")
	form.Set("redirect_uri", "http://cb/")
	req := httptest.NewRequest("POST", "/login/oauth/authorize", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no-session POST authorize: status = %d, want 401", w.Code)
	}
}

func TestOAuth_AuthorizeApprove_RejectsWrongCSRF(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHOAuthRoutes()

	jar := doLogin(t, s, "admin")

	form := url.Values{}
	form.Set("authenticity_token", "wrong-token")
	form.Set("client_id", "Iv1.x")
	form.Set("redirect_uri", "http://cb/")
	w := requestWithJar(s, "POST", "/login/oauth/authorize", form.Encode(), "application/x-www-form-urlencoded", jar)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong CSRF POST authorize: status = %d, want 422", w.Code)
	}
}

func TestOAuth_AuthorizeAuto1ImmediateRedirect(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHOAuthRoutes()

	// auto=1 without a session still works (non-standard shortcut, uses seed admin).
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

func TestOAuth_WebFlow_AccessTokenExchange(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHOAuthRoutes()

	// Use the conformant flow: login → consent form → POST with CSRF → exchange.
	jar := doLogin(t, s, "admin")
	w := requestWithJar(s, "GET", "/login/oauth/authorize?client_id=Iv1.test&redirect_uri=http://cb/&scope=repo&state=S", "", "", jar)
	if w.Code != http.StatusOK {
		t.Fatalf("GET authorize status = %d", w.Code)
	}
	csrf := extractCSRF(t, w.Body.String())

	form := url.Values{}
	form.Set("authenticity_token", csrf)
	form.Set("client_id", "Iv1.test")
	form.Set("redirect_uri", "http://cb/")
	form.Set("scope", "repo")
	form.Set("state", "S")
	w2 := requestWithJar(s, "POST", "/login/oauth/authorize", form.Encode(), "application/x-www-form-urlencoded", jar)
	if w2.Code != http.StatusFound {
		t.Fatalf("POST authorize status = %d", w2.Code)
	}
	loc, _ := url.Parse(w2.Header().Get("Location"))
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatal("no code in redirect")
	}

	// Exchange code for access token.
	exchForm := url.Values{}
	exchForm.Set("grant_type", "authorization_code")
	exchForm.Set("code", code)
	exchForm.Set("client_id", "Iv1.test")
	req := httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(exchForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w3 := httptest.NewRecorder()
	s.mux.ServeHTTP(w3, req)
	if w3.Code != http.StatusOK {
		t.Fatalf("token-exchange status = %d, body = %s", w3.Code, w3.Body.String())
	}
	var resp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(w3.Body.Bytes(), &resp); err != nil {
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

	jar := doLogin(t, s, "admin")
	w := requestWithJar(s, "GET", "/login/oauth/authorize?client_id=x&redirect_uri=http://cb/&scope=repo", "", "", jar)
	csrf := extractCSRF(t, w.Body.String())

	form := url.Values{}
	form.Set("authenticity_token", csrf)
	form.Set("client_id", "x")
	form.Set("redirect_uri", "http://cb/")
	w2 := requestWithJar(s, "POST", "/login/oauth/authorize", form.Encode(), "application/x-www-form-urlencoded", jar)
	loc, _ := url.Parse(w2.Header().Get("Location"))
	code := loc.Query().Get("code")

	exchForm := url.Values{}
	exchForm.Set("code", code)

	req := httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(exchForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w3 := httptest.NewRecorder()
	s.mux.ServeHTTP(w3, req)
	if w3.Code != http.StatusOK {
		t.Fatalf("first exchange status = %d", w3.Code)
	}

	// Second exchange with the SAME code — must fail.
	req2 := httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(exchForm.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w4 := httptest.NewRecorder()
	s.mux.ServeHTTP(w4, req2)
	if !strings.Contains(w4.Body.String(), "bad_verification_code") {
		t.Errorf("re-using code returned: %s", w4.Body.String())
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
