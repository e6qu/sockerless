package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
)

func testSHAUTHConfig() shauthConfig {
	return shauthConfig{
		issuer:          "https://auth.dev.e6qu.dev",
		clientID:        "client",
		clientSecret:    "this-is-a-test-secret",
		sessionSecret:   "0123456789abcdef0123456789abcdef",
		publicURL:       "https://admin.dev.e6qu.dev",
		releaseRevision: "0123456789ab",
		sessions:        newSHAUTHSessionStore(),
		providerCache:   &shauthProviderCache{},
	}
}

func TestSHAUTHProviderCacheReturnsDiscoveredProvider(t *testing.T) {
	config := testSHAUTHConfig()
	want := &oidc.Provider{}
	config.providerCache.provider = want
	got, err := config.providerFor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatal("cached OpenID Connect provider was not reused")
	}
}

func TestSHAUTHConfigRequiresCompleteHTTPSCoordinates(t *testing.T) {
	if err := (shauthConfig{}).validate(); err != nil {
		t.Fatalf("disabled configuration: %v", err)
	}
	if err := (shauthConfig{releaseRevision: "0123456789ab"}).validate(); err != nil {
		t.Fatalf("release metadata without optional browser authentication: %v", err)
	}
	if err := (shauthConfig{issuer: "https://auth.dev.e6qu.dev", clientID: "client"}).validate(); err == nil {
		t.Fatal("partial configuration was accepted")
	}
	if err := (shauthConfig{issuer: "http://auth.dev.e6qu.dev", clientID: "client", clientSecret: "secret", sessionSecret: "0123456789abcdef0123456789abcdef", publicURL: "https://admin.dev.e6qu.dev", releaseRevision: "0123456789ab"}).validate(); err == nil {
		t.Fatal("non-HTTPS issuer was accepted")
	}
	if err := (shauthConfig{issuer: "http://localhost:8080", clientID: "client", clientSecret: "secret", sessionSecret: "0123456789abcdef0123456789abcdef", publicURL: "http://localhost:29090", releaseRevision: "0123456789ab", insecure: true}).validate(); err != nil {
		t.Fatalf("explicit loopback test configuration: %v", err)
	}
	if err := (shauthConfig{issuer: "http://auth.dev.e6qu.dev", clientID: "client", clientSecret: "secret", sessionSecret: "0123456789abcdef0123456789abcdef", publicURL: "http://admin.dev.e6qu.dev", releaseRevision: "0123456789ab", insecure: true}).validate(); err == nil {
		t.Fatal("public HTTP coordinates were accepted in insecure test mode")
	}
	if err := (shauthConfig{issuer: "https://user@auth.dev.e6qu.dev", clientID: "client", clientSecret: "secret", sessionSecret: "0123456789abcdef0123456789abcdef", publicURL: "https://admin.dev.e6qu.dev", releaseRevision: "0123456789ab"}).validate(); err == nil {
		t.Fatal("issuer with user information was accepted")
	}
	if err := (shauthConfig{issuer: "https://auth.dev.e6qu.dev", clientID: "client", clientSecret: "secret", sessionSecret: "short", publicURL: "https://admin.dev.e6qu.dev", releaseRevision: "0123456789ab"}).validate(); err == nil {
		t.Fatal("short session secret was accepted")
	}
	config := testSHAUTHConfig()
	config.releaseRevision = ""
	if err := config.validate(); err == nil {
		t.Fatal("missing application release was accepted")
	}
	config = testSHAUTHConfig()
	config.releaseRevision = "latest"
	if err := config.validate(); err == nil {
		t.Fatal("moving application release was accepted")
	}
}

func TestSHAUTHConfigPreservesExactIssuer(t *testing.T) {
	t.Setenv("SOCKERLESS_ADMIN_SHAUTH_ISSUER", "https://auth.dev.e6qu.dev/tenant/")
	t.Setenv("SOCKERLESS_ADMIN_SHAUTH_CLIENT_ID", "client")
	t.Setenv("SOCKERLESS_ADMIN_SHAUTH_CLIENT_SECRET", "secret")
	t.Setenv("SOCKERLESS_ADMIN_SESSION_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("SOCKERLESS_ADMIN_PUBLIC_URL", "https://admin.dev.e6qu.dev/")
	t.Setenv("APPLICATION_RELEASE_REVISION", "0123456789ab")
	config := shauthConfigFromEnvironment()
	if config.issuer != "https://auth.dev.e6qu.dev/tenant/" {
		t.Fatalf("issuer = %q", config.issuer)
	}
	if config.publicURL != "https://admin.dev.e6qu.dev" {
		t.Fatalf("public URL = %q", config.publicURL)
	}
	if config.releaseRevision != "0123456789ab" {
		t.Fatalf("application release revision = %q", config.releaseRevision)
	}
}

func TestSHAUTHOriginDropsIssuerPath(t *testing.T) {
	if got := shauthOrigin("https://auth.dev.e6qu.dev/tenant/"); got != "https://auth.dev.e6qu.dev" {
		t.Fatalf("Shauth origin = %q", got)
	}
}

func TestSHAUTHSignedSessionGuardsOperatorSurface(t *testing.T) {
	config := testSHAUTHConfig()
	config.insecure = true
	handler := config.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/auth/shauth" {
		t.Fatalf("operator surface = %d %q, want Shauth redirect", response.Code, response.Header().Get("Location"))
	}

	expires := time.Now().Add(time.Hour).Unix()
	value, err := config.sign(shauthSession{ID: "session-1", Subject: "subject", Name: "operator", Role: "admin", Expires: expires})
	if err != nil {
		t.Fatalf("sign session: %v", err)
	}
	config.sessions.put("session-1", shauthSessionRecord{Subject: "subject", Expires: expires})
	request = httptest.NewRequest(http.MethodGet, "/api/v1/components", nil)
	request.AddCookie(&http.Cookie{Name: shauthSessionCookie, Value: value})
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("authenticated operator API = %d, want %d", response.Code, http.StatusNoContent)
	}

	request = httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("session endpoint = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestSHAUTHAdministratorRoleGuardsUIAndRealTopologyMutations(t *testing.T) {
	config := testSHAUTHConfig()
	config.insecure = true
	topology := NewTopologyManager(t.TempDir() + "/sockerless.yaml")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ui/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("operator console"))
	})
	registerTopologyAPI(mux, topology, nil)
	handler := config.middleware(mux)

	newSessionCookie := func(t *testing.T, id, role string) *http.Cookie {
		t.Helper()
		expires := time.Now().Add(time.Hour).Unix()
		value, err := config.sign(shauthSession{ID: id, Subject: id, Name: id, Role: role, Expires: expires})
		if err != nil {
			t.Fatalf("sign %s session: %v", role, err)
		}
		config.sessions.put(id, shauthSessionRecord{Subject: id, Expires: expires})
		return &http.Cookie{Name: shauthSessionCookie, Value: value}
	}
	developer := newSessionCookie(t, "developer-session", "developer")
	administrator := newSessionCookie(t, "administrator-session", shauthAdministratorRole)

	request := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	request.AddCookie(developer)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "Administrator access required") {
		t.Fatalf("developer UI response = %d %q", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("developer denial cache control = %q", response.Header().Get("Cache-Control"))
	}
	if csp := response.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "form-action 'self' https://auth.dev.e6qu.dev") {
		t.Fatalf("developer denial Content-Security-Policy omitted the exact Shauth origin: %q", csp)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/topology/projects", strings.NewReader(`{"name":"denied-project","instances":[]}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(developer)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "administrator role required") {
		t.Fatalf("developer mutation response = %d %q", response.Code, response.Body.String())
	}
	if len(topology.Get().Projects) != 0 {
		t.Fatalf("developer mutation changed topology: %#v", topology.Get().Projects)
	}

	request = httptest.NewRequest(http.MethodGet, "/ui/", nil)
	request.AddCookie(administrator)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "operator console" {
		t.Fatalf("administrator UI response = %d %q", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/topology/projects", strings.NewReader(`{"name":"authorized-project","instances":[]}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(administrator)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("administrator mutation response = %d %q", response.Code, response.Body.String())
	}
	projects := topology.Get().Projects
	if len(projects) != 1 || projects[0].Name != "authorized-project" {
		t.Fatalf("administrator mutation did not persist topology: %#v", projects)
	}
}

func TestSHAUTHSessionDoesNotAcceptTampering(t *testing.T) {
	config := testSHAUTHConfig()
	value, err := config.sign(shauthSession{Subject: "subject", Role: "admin", Expires: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatalf("sign session: %v", err)
	}
	var session shauthSession
	if err := config.verify(value+"x", &session); err == nil {
		t.Fatal("tampered session was accepted")
	}
}

func TestSHAUTHSessionKeyIsIndependentFromOIDCClientSecret(t *testing.T) {
	config := testSHAUTHConfig()
	value, err := config.sign(shauthSession{Subject: "subject", Expires: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatalf("sign session: %v", err)
	}
	rotatedClient := config
	rotatedClient.clientSecret = "rotated-confidential-client-secret"
	var session shauthSession
	if err := rotatedClient.verify(value, &session); err != nil {
		t.Fatalf("rotating OIDC client secret changed cookie verification: %v", err)
	}
	rotatedSession := config
	rotatedSession.sessionSecret = "fedcba9876543210fedcba9876543210"
	if err := rotatedSession.verify(value, &session); err == nil {
		t.Fatal("session signed with the previous cookie key was accepted")
	}
}

func TestSHAUTHSessionReportsSignedIdentity(t *testing.T) {
	config := testSHAUTHConfig()
	expires := time.Now().Add(time.Hour).Unix()
	value, err := config.sign(shauthSession{ID: "session-1", Subject: "subject", Name: "Taylor", Role: "admin", Expires: expires})
	if err != nil {
		t.Fatalf("sign session: %v", err)
	}
	config.sessions.put("session-1", shauthSessionRecord{Subject: "subject", Expires: expires})
	request := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	request.AddCookie(&http.Cookie{Name: shauthSessionCookie, Value: value})
	response := httptest.NewRecorder()
	config.session(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("session status = %d, want %d", response.Code, http.StatusOK)
	}
	var got struct {
		Authenticated bool   `json:"authenticated"`
		Name          string `json:"name"`
		Role          string `json:"role"`
	}
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if !got.Authenticated || got.Name != "Taylor" || got.Role != "admin" {
		t.Fatalf("session = %#v", got)
	}
}

func TestSHAUTHBackchannelLogoutRevokesMatchingSessionsAndRejectsReplay(t *testing.T) {
	config := testSHAUTHConfig()
	now := time.Now().UTC().Truncate(time.Second)
	config.sessions.put("target", shauthSessionRecord{Subject: "subject", UpstreamSID: "upstream-session", Expires: now.Add(time.Hour).Unix()})
	config.sessions.put("other", shauthSessionRecord{Subject: "other", UpstreamSID: "other-session", Expires: now.Add(time.Hour).Unix()})

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: privateKey}, nil)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	signClaims := func(claims map[string]any) string {
		t.Helper()
		payload, err := json.Marshal(claims)
		if err != nil {
			t.Fatalf("encode logout token: %v", err)
		}
		signed, err := signer.Sign(payload)
		if err != nil {
			t.Fatalf("sign logout token: %v", err)
		}
		raw, err := signed.CompactSerialize()
		if err != nil {
			t.Fatalf("serialize logout token: %v", err)
		}
		return raw
	}
	rawLogoutToken := signClaims(map[string]any{
		"iss":    config.issuer,
		"sub":    "subject",
		"sid":    "upstream-session",
		"aud":    config.clientID,
		"iat":    now.Unix(),
		"exp":    now.Add(5 * time.Minute).Unix(),
		"jti":    "logout-token-1",
		"events": map[string]any{"http://schemas.openid.net/event/backchannel-logout": map[string]any{}},
	})
	verifier := oidc.NewVerifier(config.issuer, &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{privateKey.Public()}}, &oidc.Config{ClientID: config.clientID, Now: func() time.Time { return now }})

	if err := config.processBackchannelLogout(context.Background(), rawLogoutToken, verifier, now); err != nil {
		t.Fatalf("process logout token: %v", err)
	}
	if _, active := config.sessions.get("target", now); active {
		t.Fatal("matching session remained active after back-channel logout")
	}
	if _, active := config.sessions.get("other", now); !active {
		t.Fatal("unrelated session was revoked")
	}
	if err := config.processBackchannelLogout(context.Background(), rawLogoutToken, verifier, now); err == nil {
		t.Fatal("replayed logout token was accepted")
	}
	missingIssuedAt := signClaims(map[string]any{
		"iss": config.issuer, "sub": "subject", "aud": config.clientID,
		"exp": now.Add(5 * time.Minute).Unix(), "jti": "logout-token-2",
		"events": map[string]any{shauthLogoutEvent: map[string]any{}},
	})
	if err := config.processBackchannelLogout(context.Background(), missingIssuedAt, verifier, now); err == nil {
		t.Fatal("logout token without iat was accepted")
	}
	missingExpiry := signClaims(map[string]any{
		"iss": config.issuer, "sub": "subject", "aud": config.clientID,
		"iat": now.Unix(), "jti": "logout-token-missing-expiry",
		"events": map[string]any{shauthLogoutEvent: map[string]any{}},
	})
	if err := config.processBackchannelLogout(context.Background(), missingExpiry, verifier, now); err == nil {
		t.Fatal("logout token without exp was accepted")
	}
	invalidEvent := signClaims(map[string]any{
		"iss": config.issuer, "sub": "subject", "aud": config.clientID,
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "jti": "logout-token-3",
		"events": map[string]any{shauthLogoutEvent: nil},
	})
	if err := config.processBackchannelLogout(context.Background(), invalidEvent, verifier, now); err == nil {
		t.Fatal("logout token with a null event was accepted")
	}
}

func TestSHAUTHMiddlewareRejectsSessionAfterServerRestart(t *testing.T) {
	config := testSHAUTHConfig()
	expires := time.Now().Add(time.Hour).Unix()
	value, err := config.sign(shauthSession{ID: "old-session", Subject: "subject", Name: "operator", Role: "developer", Expires: expires})
	if err != nil {
		t.Fatalf("sign session: %v", err)
	}

	restarted := testSHAUTHConfig()
	request := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	request.AddCookie(&http.Cookie{Name: shauthSessionCookie, Value: value})
	response := httptest.NewRecorder()
	restarted.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(response, request)
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/auth/shauth" {
		t.Fatalf("restarted server accepted stale session: status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
}

func TestSHAUTHMiddlewareAllowsFrontchannelLogoutWithoutLocalSession(t *testing.T) {
	config := testSHAUTHConfig()
	request := httptest.NewRequest(http.MethodGet, "/auth/shauth/frontchannel-logout?iss="+url.QueryEscape(config.issuer)+"&sid=provider-session", nil)
	response := httptest.NewRecorder()
	config.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("front-channel logout was redirected behind authentication: status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
}

func TestSHAUTHLogoutRequiresSameOriginBrowserEvidence(t *testing.T) {
	for name, testCase := range map[string]struct {
		configure func(*http.Request)
		want      bool
	}{
		"same origin":                {configure: func(r *http.Request) { r.Header.Set("Origin", "https://admin.dev.e6qu.dev") }, want: true},
		"same-origin referer":        {configure: func(r *http.Request) { r.Header.Set("Referer", "https://admin.dev.e6qu.dev/ui/") }, want: true},
		"same-origin fetch metadata": {configure: func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-origin") }, want: true},
		"missing evidence":           {configure: func(*http.Request) {}, want: false},
		"cross-origin referer":       {configure: func(r *http.Request) { r.Header.Set("Referer", "https://attacker.example/") }, want: false},
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "https://admin.dev.e6qu.dev/auth/logout", nil)
			testCase.configure(request)
			if got := sameOriginRequest(request, testSHAUTHConfig().publicURL); got != testCase.want {
				t.Fatalf("sameOriginRequest() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestSHAUTHLogoutRevokesLocalSessionBeforeProviderFailure(t *testing.T) {
	config := testSHAUTHConfig()
	config.issuer = "https://127.0.0.1:1"
	expires := time.Now().Add(time.Hour).Unix()
	session := shauthSession{ID: "session-1", Subject: "subject", Role: "developer", Expires: expires}
	value, err := config.sign(session)
	if err != nil {
		t.Fatal(err)
	}
	config.sessions.put(session.ID, shauthSessionRecord{Subject: session.Subject, RawIDToken: "id-token", Expires: expires})

	request := httptest.NewRequest(http.MethodPost, config.publicURL+"/auth/logout", nil)
	request.Header.Set("Origin", config.publicURL)
	request.AddCookie(&http.Cookie{Name: shauthSessionCookie, Value: value})
	recorder := httptest.NewRecorder()
	config.logout(recorder, request)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("provider failure status = %d, want %d", recorder.Code, http.StatusBadGateway)
	}
	if _, exists := config.sessions.get(session.ID, time.Now()); exists {
		t.Fatal("local session remained active after provider failure")
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	cleared := false
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == shauthSessionCookie && cookie.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("session cookie was not cleared before provider failure")
	}
}

func TestSHAUTHLogoutURLReturnsToSockerlessAdmin(t *testing.T) {
	config := testSHAUTHConfig()
	logoutURL, err := config.logoutURL("https://auth.dev.e6qu.dev/oauth2/sessions/logout", "signed-id-token")
	if err != nil {
		t.Fatal(err)
	}
	query := logoutURL.Query()
	if got := query.Get("client_id"); got != config.clientID {
		t.Fatalf("client_id = %q", got)
	}
	if got := query.Get("id_token_hint"); got != "signed-id-token" {
		t.Fatalf("id_token_hint = %q", got)
	}
	if got := query.Get("post_logout_redirect_uri"); got != "https://admin.dev.e6qu.dev"+shauthLogoutCompletePath {
		t.Fatalf("post_logout_redirect_uri = %q", got)
	}
}

func TestSHAUTHLogoutCompleteUsesOnlyConfiguredIssuer(t *testing.T) {
	config := testSHAUTHConfig()
	request := httptest.NewRequest(http.MethodGet, "https://admin.dev.e6qu.dev"+shauthLogoutCompletePath+"?next=https%3A%2F%2Fattacker.example&code=secret", nil)
	recorder := httptest.NewRecorder()
	config.middleware(http.HandlerFunc(config.logoutComplete)).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, location = %q", recorder.Code, recorder.Header().Get("Location"))
	}
	if location := recorder.Header().Get("Location"); location != "https://auth.dev.e6qu.dev/oauth/logout/complete" {
		t.Fatalf("bridge location = %q", location)
	}
	if recorder.Header().Get("Cache-Control") != "no-store" || recorder.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("logout completion bridge omitted no-store/no-referrer headers")
	}
}

func TestSHAUTHLogoutURLRejectsAnotherIssuerOrigin(t *testing.T) {
	if _, err := testSHAUTHConfig().logoutURL("https://attacker.example/oauth2/sessions/logout", ""); err == nil {
		t.Fatal("cross-origin logout endpoint was accepted")
	}
}

func TestSHAUTHLogoutURLAllowsHTTPOnlyInExplicitInsecureMode(t *testing.T) {
	config := testSHAUTHConfig()
	config.issuer = "http://localhost:8080"
	if _, err := config.logoutURL("http://localhost:8080/oauth2/sessions/logout", ""); err == nil {
		t.Fatal("HTTP logout endpoint was accepted without explicit insecure mode")
	}
	config.insecure = true
	if _, err := config.logoutURL("http://localhost:8080/oauth2/sessions/logout", ""); err != nil {
		t.Fatalf("HTTP logout endpoint was rejected in explicit insecure mode: %v", err)
	}
}

func TestSHAUTHBackchannelLogoutRequiresFormBody(t *testing.T) {
	config := testSHAUTHConfig()
	for name, testCase := range map[string]struct {
		target, contentType string
		wantStatus          int
	}{
		"missing media type": {target: "/auth/shauth/backchannel-logout", wantStatus: http.StatusUnsupportedMediaType},
		"JSON media type":    {target: "/auth/shauth/backchannel-logout", contentType: "application/json", wantStatus: http.StatusUnsupportedMediaType},
		"query token":        {target: "/auth/shauth/backchannel-logout?logout_token=query", contentType: "application/x-www-form-urlencoded", wantStatus: http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, testCase.target, strings.NewReader(""))
			if testCase.contentType != "" {
				request.Header.Set("Content-Type", testCase.contentType)
			}
			recorder := httptest.NewRecorder()
			config.backchannelLogout(recorder, request)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, testCase.wantStatus)
			}
			if recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("Cache-Control = %q", recorder.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestSHAUTHFrontchannelLogoutRevokesOnlyTrustedIssuerSession(t *testing.T) {
	config := testSHAUTHConfig()
	expires := time.Now().Add(time.Hour).Unix()
	config.sessions.put("session-1", shauthSessionRecord{Subject: "subject", UpstreamSID: "provider-session", Expires: expires})

	request := httptest.NewRequest(http.MethodGet, "/auth/shauth/frontchannel-logout?iss=https%3A%2F%2Fattacker.example&sid=provider-session", nil)
	recorder := httptest.NewRecorder()
	config.frontchannelLogout(recorder, request)
	if _, exists := config.sessions.get("session-1", time.Now()); !exists {
		t.Fatal("untrusted front-channel issuer revoked a session")
	}

	request = httptest.NewRequest(http.MethodGet, "/auth/shauth/frontchannel-logout?iss=https%3A%2F%2Fauth.dev.e6qu.dev&sid=provider-session", nil)
	recorder = httptest.NewRecorder()
	config.frontchannelLogout(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("front-channel response = %d Cache-Control=%q", recorder.Code, recorder.Header().Get("Cache-Control"))
	}
	if _, exists := config.sessions.get("session-1", time.Now()); exists {
		t.Fatal("trusted front-channel logout left the session active")
	}
}

func TestSHAUTHBackchannelLogoutEventMustBeJSONObject(t *testing.T) {
	for name, testCase := range map[string]struct {
		raw  string
		want bool
	}{
		"empty object":     {raw: `{}`, want: true},
		"non-empty object": {raw: `{"reason":"admin"}`, want: false},
		"missing":          {raw: ``, want: false},
		"null":             {raw: `null`, want: false},
		"string":           {raw: `"logout"`, want: false},
		"array":            {raw: `[]`, want: false},
		"invalid":          {raw: `{`, want: false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := validSHAUTHLogoutEvent(json.RawMessage(testCase.raw)); got != testCase.want {
				t.Fatalf("validSHAUTHLogoutEvent(%q) = %v, want %v", testCase.raw, got, testCase.want)
			}
		})
	}
}

func TestSHAUTHSignedOutResponseIsPublicAndNotCached(t *testing.T) {
	config := testSHAUTHConfig()
	recorder := httptest.NewRecorder()
	config.middleware(http.HandlerFunc(config.signedOut)).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, shauthSignedOutPath, nil))
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("signed-out response = %d Cache-Control=%q", recorder.Code, recorder.Header().Get("Cache-Control"))
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `href="/auth/shauth"`) || !strings.Contains(body, `>Sign in with Shauth</a>`) {
		t.Fatalf("signed-out response omitted the explicit Shauth sign-in control: %s", body)
	}
}

func TestSHAUTHValidationRequiresSessionAndExposesExactContract(t *testing.T) {
	config := testSHAUTHConfig()
	anonymous := httptest.NewRequest(http.MethodGet, shauthValidationPath, nil)
	anonymous.Header.Set("Authorization", "Bearer validator-material-must-not-authenticate")
	recorder := httptest.NewRecorder()
	config.validation(recorder, anonymous)
	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != shauthSignedOutPath {
		t.Fatalf("anonymous validation = %d %q", recorder.Code, recorder.Header().Get("Location"))
	}

	expires := time.Now().Add(time.Hour).Unix()
	session := shauthSession{ID: "validation-session", Subject: "subject", Name: "shauth-validator", Email: "validator@example.test", Role: "developer", Expires: expires}
	value, err := config.sign(session)
	if err != nil {
		t.Fatal(err)
	}
	config.sessions.put(session.ID, shauthSessionRecord{Subject: session.Subject, Expires: expires})
	request := httptest.NewRequest(http.MethodGet, shauthValidationPath, nil)
	request.AddCookie(&http.Cookie{Name: shauthSessionCookie, Value: value})
	recorder = httptest.NewRecorder()
	config.validation(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("authenticated validation = %d Cache-Control=%q", recorder.Code, recorder.Header().Get("Cache-Control"))
	}
	if csp := recorder.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "form-action 'self' https://auth.dev.e6qu.dev") {
		t.Fatalf("validation Content-Security-Policy omitted the exact Shauth origin: %q", csp)
	}
	for _, expected := range []string{
		`data-testid="validation-username">shauth-validator</dd>`,
		`data-testid="validation-email">validator@example.test</dd>`,
		`data-testid="validation-role">developer</dd>`,
		`data-testid="validation-release">0123456789ab</dd>`,
		`action="/auth/logout"`, `>Sign out</button>`,
	} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Fatalf("validation response omitted %q: %s", expected, recorder.Body.String())
		}
	}
}
