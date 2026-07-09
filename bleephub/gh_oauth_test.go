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
// surface using registered OAuth App and GitHub App client credentials.

// doLogin posts a real stored personal access token to POST /login and returns
// a cookie jar carrying the browser session.
func doLogin(t *testing.T, s *Server, login string) http.CookieJar {
	t.Helper()
	user := s.store.LookupUserByLogin(login)
	if user == nil {
		t.Fatalf("test user %q not found", login)
	}
	credential := s.store.CreateToken(user.ID, "repo,read:org").Value
	form := url.Values{}
	form.Set("login", login)
	form.Set("password", credential)
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

func authorizeOAuthWebFlow(t *testing.T, s *Server, login, clientID, redirectURI, scope, state string) string {
	t.Helper()
	jar := doLogin(t, s, login)
	authorizeURL := "/login/oauth/authorize?client_id=" + url.QueryEscape(clientID) +
		"&redirect_uri=" + url.QueryEscape(redirectURI) +
		"&scope=" + url.QueryEscape(scope) +
		"&state=" + url.QueryEscape(state)
	w := requestWithJar(s, "GET", authorizeURL, "", "", jar)
	if w.Code != http.StatusOK {
		t.Fatalf("GET authorize status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	csrf := extractCSRF(t, w.Body.String())
	form := url.Values{}
	form.Set("authenticity_token", csrf)
	form.Set("client_id", clientID)
	form.Set("redirect_uri", redirectURI)
	form.Set("scope", scope)
	form.Set("state", state)
	w2 := requestWithJar(s, "POST", "/login/oauth/authorize", form.Encode(), "application/x-www-form-urlencoded", jar)
	if w2.Code != http.StatusFound {
		t.Fatalf("POST authorize status = %d, want 302; body=%s", w2.Code, w2.Body.String())
	}
	loc, err := url.Parse(w2.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse authorize redirect: %v", err)
	}
	if got := loc.Query().Get("state"); got != state {
		t.Fatalf("authorize redirect state = %q, want %q", got, state)
	}
	code := loc.Query().Get("code")
	if code == "" {
		t.Fatalf("authorize redirect missing code: %s", loc.String())
	}
	return code
}

func issueDeviceCode(t *testing.T, s *Server, clientID, scope string) (deviceCode, userCode string) {
	t.Helper()
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("scope", scope)
	req := httptest.NewRequest("POST", "/login/device/code", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("device code status = %d", w.Code)
	}
	var dc struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &dc); err != nil {
		t.Fatalf("decode device code: %v", err)
	}
	if dc.DeviceCode == "" || dc.UserCode == "" {
		t.Fatalf("missing device/user code in response: %s", w.Body.String())
	}
	return dc.DeviceCode, dc.UserCode
}

func pollDeviceToken(t *testing.T, s *Server, deviceCode, accept string) *httptest.ResponseRecorder {
	t.Helper()
	var clientID string
	s.store.mu.RLock()
	if dc := s.store.DeviceCodes[deviceCode]; dc != nil {
		clientID = dc.ClientID
	}
	s.store.mu.RUnlock()
	form := url.Values{
		"client_id":   {clientID},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"device_code": {deviceCode},
	}
	req := httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	return w
}

func approveDeviceCode(t *testing.T, s *Server, login, userCode string) {
	t.Helper()
	jar := doLogin(t, s, login)
	form := url.Values{}
	form.Set("user_code", userCode)
	w := requestWithJar(s, "POST", "/login/device", form.Encode(), "application/x-www-form-urlencoded", jar)
	if w.Code != http.StatusOK {
		t.Fatalf("device approve status = %d, body=%s", w.Code, w.Body.String())
	}
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

func postLogin(t *testing.T, s *Server, login, credential string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{}
	form.Set("login", login)
	form.Set("password", credential)
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	return w
}

func seedOAuthTestUser(t *testing.T, s *Server, login string) *User {
	t.Helper()
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	user := &User{ID: s.store.NextUser, Login: login, Type: "User"}
	s.store.NextUser++
	s.store.Users[user.ID] = user
	s.store.UsersByLogin[user.Login] = user
	return user
}

func createOAuthTestApp(t *testing.T, s *Server, callbackURL string) *OAuthApp {
	t.Helper()
	admin := s.store.LookupUserByLogin("admin")
	if admin == nil {
		t.Fatal("admin user not seeded")
	}
	return s.store.CreateOAuthApp(admin.ID, "test OAuth App", "", "https://example.test", callbackURL)
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

func TestOAuth_LoginPost_RequiresStoredPersonalAccessToken(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	alice := seedOAuthTestUser(t, s, "login-alice")
	aliceToken := s.store.CreateToken(alice.ID, "repo").Value
	adminToken := defaultToken
	s.registerGHOAuthRoutes()

	w := postLogin(t, s, "admin", "anything")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("arbitrary password status = %d, want 401", w.Code)
	}

	w = postLogin(t, s, "admin", aliceToken)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("mismatched token status = %d, want 401", w.Code)
	}

	w = postLogin(t, s, "admin", adminToken)
	if w.Code != http.StatusOK && w.Code != http.StatusFound {
		t.Fatalf("stored token status = %d, want 200 or 302", w.Code)
	}
}

func TestOAuth_LoginPost_RejectsSuspendedUser(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	u := seedOAuthTestUser(t, s, "login-suspended")
	token := s.store.CreateToken(u.ID, "repo").Value
	s.store.mu.Lock()
	u.Suspended = true
	s.store.mu.Unlock()
	s.registerGHOAuthRoutes()

	w := postLogin(t, s, u.Login, token)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("suspended user login status = %d, want 401", w.Code)
	}
}

func TestOAuth_LoginPost_UnknownUserReturns401(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHOAuthRoutes()

	w := postLogin(t, s, "nobody", "anything")
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
	app := createOAuthTestApp(t, s, "http://callback/")
	s.registerGHOAuthRoutes()

	jar := doLogin(t, s, "admin")
	w := requestWithJar(s, "GET", "/login/oauth/authorize?client_id="+url.QueryEscape(app.ClientID)+"&redirect_uri=http://callback/&scope=repo&state=xyz", "", "", jar)
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
	if !strings.Contains(body, app.ClientID) {
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
	app := createOAuthTestApp(t, s, "http://cb/")
	s.registerGHOAuthRoutes()

	// Step 1: login as alice.
	jar := doLogin(t, s, "alice")

	// Step 2: GET authorize → consent form with CSRF.
	authorizeURL := "/login/oauth/authorize?client_id=" + url.QueryEscape(app.ClientID) + "&redirect_uri=http://cb/&scope=repo&state=S"
	w := requestWithJar(s, "GET", authorizeURL, "", "", jar)
	if w.Code != http.StatusOK {
		t.Fatalf("GET authorize status = %d, want 200", w.Code)
	}
	csrf := extractCSRF(t, w.Body.String())

	// Step 3: POST authorize with CSRF → 302 with code.
	form := url.Values{}
	form.Set("authenticity_token", csrf)
	form.Set("client_id", app.ClientID)
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
	exchForm.Set("client_id", app.ClientID)
	exchForm.Set("client_secret", app.ClientSecret)
	req := httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(exchForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json") // real clients (Go oauth2/gh) negotiate JSON
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

	// Step 5: verify the web flow yields a user-to-server token (gho_ for an
	// OAuth App client_id) belonging to alice, not admin.
	if !strings.HasPrefix(tokResp.AccessToken, "gho_") {
		t.Errorf("web flow token = %q, want gho_ prefix", tokResp.AccessToken)
	}
	_, user := s.store.LookupUserToServerToken(tokResp.AccessToken)
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

func TestOAuth_AuthorizeAutoParamDoesNotBypassSession(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHOAuthRoutes()

	w := runRequest(s, "GET", "/login/oauth/authorize?client_id=Iv1.x&redirect_uri=http://cb/&state=ST&auto=1")
	if w.Code != http.StatusFound {
		t.Fatalf("auto=1 without session status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "/login") {
		t.Fatalf("auto=1 bypassed session requirement, redirected to %q", loc)
	}
	if strings.Contains(loc, "code=") {
		t.Fatalf("auto=1 minted an authorization code without consent: %q", loc)
	}
}

func TestOAuth_WebFlow_AccessTokenExchange(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	app := createOAuthTestApp(t, s, "http://cb/")
	s.registerGHOAuthRoutes()

	// Use the conformant flow: login → consent form → POST with CSRF → exchange.
	jar := doLogin(t, s, "admin")
	w := requestWithJar(s, "GET", "/login/oauth/authorize?client_id="+url.QueryEscape(app.ClientID)+"&redirect_uri=http://cb/&scope=repo&state=S", "", "", jar)
	if w.Code != http.StatusOK {
		t.Fatalf("GET authorize status = %d", w.Code)
	}
	csrf := extractCSRF(t, w.Body.String())

	form := url.Values{}
	form.Set("authenticity_token", csrf)
	form.Set("client_id", app.ClientID)
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
	exchForm.Set("client_id", app.ClientID)
	exchForm.Set("client_secret", app.ClientSecret)
	req := httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(exchForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json") // real clients (Go oauth2/gh) negotiate JSON
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
	app := createOAuthTestApp(t, s, "http://cb/")
	s.registerGHOAuthRoutes()

	jar := doLogin(t, s, "admin")
	w := requestWithJar(s, "GET", "/login/oauth/authorize?client_id="+url.QueryEscape(app.ClientID)+"&redirect_uri=http://cb/&scope=repo", "", "", jar)
	csrf := extractCSRF(t, w.Body.String())

	form := url.Values{}
	form.Set("authenticity_token", csrf)
	form.Set("client_id", app.ClientID)
	form.Set("redirect_uri", "http://cb/")
	w2 := requestWithJar(s, "POST", "/login/oauth/authorize", form.Encode(), "application/x-www-form-urlencoded", jar)
	loc, _ := url.Parse(w2.Header().Get("Location"))
	code := loc.Query().Get("code")

	exchForm := url.Values{}
	exchForm.Set("code", code)
	exchForm.Set("client_id", app.ClientID)
	exchForm.Set("client_secret", app.ClientSecret)

	req := httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(exchForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json") // real clients (Go oauth2/gh) negotiate JSON
	w3 := httptest.NewRecorder()
	s.mux.ServeHTTP(w3, req)
	if w3.Code != http.StatusOK {
		t.Fatalf("first exchange status = %d", w3.Code)
	}

	// Second exchange with the SAME code — must fail.
	req2 := httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(exchForm.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.Header.Set("Accept", "application/json")
	w4 := httptest.NewRecorder()
	s.mux.ServeHTTP(w4, req2)
	if !strings.Contains(w4.Body.String(), "bad_verification_code") {
		t.Errorf("re-using code returned: %s", w4.Body.String())
	}
}

func TestOAuth_WebFlow_RejectsMissingOrWrongClientSecret(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	app := createOAuthTestApp(t, s, "http://cb/")
	s.registerGHOAuthRoutes()

	code := authorizeOAuthWebFlow(t, s, "admin", app.ClientID, "http://cb/", "repo", "S")

	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", app.ClientID)
	req := httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "incorrect_client_credentials") {
		t.Fatalf("missing client_secret response = %s, want incorrect_client_credentials", w.Body.String())
	}

	code = authorizeOAuthWebFlow(t, s, "admin", app.ClientID, "http://cb/", "repo", "S2")
	form.Set("code", code)
	form.Set("client_secret", "wrong")
	req = httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	w = httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "incorrect_client_credentials") {
		t.Fatalf("wrong client_secret response = %s, want incorrect_client_credentials", w.Body.String())
	}
}

func TestOAuth_DeviceFlow_StillWorks(t *testing.T) {
	// Web-flow code-exchange must not regress the older device-code flow
	// (both routes share the /login/oauth/access_token endpoint).
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.store.mu.Lock()
	alice := &User{ID: s.store.NextUser, Login: "device-alice", Type: "User"}
	s.store.NextUser++
	s.store.Users[alice.ID] = alice
	s.store.UsersByLogin[alice.Login] = alice
	s.store.mu.Unlock()
	app := createOAuthTestApp(t, s, "http://device-callback/")
	s.registerGHOAuthRoutes()

	deviceCode, userCode := issueDeviceCode(t, s, app.ClientID, "repo")

	pending := pollDeviceToken(t, s, deviceCode, "application/json")
	if !strings.Contains(pending.Body.String(), "authorization_pending") {
		t.Fatalf("unapproved device poll = %s, want authorization_pending", pending.Body.String())
	}
	if s.store.DeviceCodes[deviceCode].Token != "" {
		t.Fatal("device code minted a token before browser approval")
	}

	approveDeviceCode(t, s, "device-alice", userCode)
	w2 := pollDeviceToken(t, s, deviceCode, "application/json")

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
	if tok := s.store.UserToServerTokens[tokResp.AccessToken]; tok == nil || tok.UserID != alice.ID {
		t.Fatalf("device token = %+v, want user %d", tok, alice.ID)
	}
	if _, ok := s.store.DeviceCodes[deviceCode]; ok {
		t.Fatal("device code remained reusable after token grant")
	}
}

func TestOAuth_DeviceFlow_RequiresRegisteredMatchingClient(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	app := createOAuthTestApp(t, s, "http://device-callback/")
	other := createOAuthTestApp(t, s, "http://other-callback/")
	s.registerGHOAuthRoutes()

	form := url.Values{}
	form.Set("client_id", "unregistered")
	form.Set("scope", "repo")
	req := httptest.NewRequest("POST", "/login/device/code", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "incorrect_client_credentials") {
		t.Fatalf("unregistered device client response = %s, want incorrect_client_credentials", w.Body.String())
	}

	deviceCode, userCode := issueDeviceCode(t, s, app.ClientID, "repo")
	approveDeviceCode(t, s, "admin", userCode)
	poll := url.Values{
		"client_id":   {other.ClientID},
		"device_code": {deviceCode},
	}
	req = httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(poll.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	w = httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "incorrect_client_credentials") {
		t.Fatalf("mismatched device client response = %s, want incorrect_client_credentials", w.Body.String())
	}
}

// TestOAuth_TokenResponse_ContentNegotiation pins the #494 contract: real
// GitHub's POST /login/oauth/access_token returns form-encoded by default and
// JSON only when the client sends Accept: application/json.
func TestOAuth_TokenResponse_ContentNegotiation(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	app := createOAuthTestApp(t, s, "http://device-callback/")
	s.registerGHOAuthRoutes()

	// Default (no Accept) → form-encoded, matching real GitHub.
	deviceCode, userCode := issueDeviceCode(t, s, app.ClientID, "repo")
	approveDeviceCode(t, s, "admin", userCode)
	def := pollDeviceToken(t, s, deviceCode, "")
	if ct := def.Header().Get("Content-Type"); !strings.Contains(ct, "application/x-www-form-urlencoded") {
		t.Errorf("default Content-Type = %q, want application/x-www-form-urlencoded", ct)
	}
	vals, err := url.ParseQuery(def.Body.String())
	if err != nil {
		t.Fatalf("default body not form-encoded: %v (%s)", err, def.Body.String())
	}
	if vals.Get("access_token") == "" {
		t.Errorf("default form body missing access_token: %s", def.Body.String())
	}
	if vals.Get("token_type") != "bearer" {
		t.Errorf("default token_type = %q, want bearer", vals.Get("token_type"))
	}

	// Accept: application/json → JSON.
	deviceCode, userCode = issueDeviceCode(t, s, app.ClientID, "repo")
	approveDeviceCode(t, s, "admin", userCode)
	js := pollDeviceToken(t, s, deviceCode, "application/json")
	if ct := js.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("json Content-Type = %q, want application/json", ct)
	}
	var obj struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(js.Body.Bytes(), &obj); err != nil {
		t.Fatalf("json body not JSON: %v", err)
	}
	if obj.AccessToken == "" {
		t.Errorf("json body missing access_token")
	}
}
