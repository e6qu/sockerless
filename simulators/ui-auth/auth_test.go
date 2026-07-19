package uiauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
)

func testConfig() Config {
	return Config{
		Issuer: "https://auth.example.test", ClientID: "simulator-test",
		ClientSecret: "client-secret", PublicURL: "https://sim.example.test",
		SessionSecret: "0123456789abcdef0123456789abcdef", CookieName: "sim_session",
		ApplicationName: "Simulator", SessionLifetime: time.Hour,
	}
}

func TestConfigRequiresCompleteSecureCoordinates(t *testing.T) {
	if _, err := New(Config{}); err != nil {
		t.Fatalf("disabled authentication: %v", err)
	}
	for name, mutate := range map[string]func(*Config){
		"missing client secret": func(c *Config) { c.ClientSecret = "" },
		"short session secret":  func(c *Config) { c.SessionSecret = "short" },
		"insecure issuer":       func(c *Config) { c.Issuer = "http://auth.example.test" },
		"issuer user info":      func(c *Config) { c.Issuer = "https://user@auth.example.test" },
		"public path":           func(c *Config) { c.PublicURL += "/sim" },
	} {
		t.Run(name, func(t *testing.T) {
			config := testConfig()
			mutate(&config)
			if _, err := New(config); err == nil {
				t.Fatal("invalid OpenID Connect configuration was accepted")
			}
		})
	}
}

func TestConfigPreservesExactIssuer(t *testing.T) {
	config := testConfig()
	config.Issuer = "https://auth.example.test/tenant/"
	auth, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	if auth.config.Issuer != config.Issuer {
		t.Fatalf("issuer = %q, want exact %q", auth.config.Issuer, config.Issuer)
	}
}

func TestProtectRedirectsOnlyTheUserInterfaceToLogin(t *testing.T) {
	auth, err := New(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	protected := auth.Protect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	recorder := httptest.NewRecorder()
	protected.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "https://sim.example.test/ui/tasks?next=1", nil))
	if recorder.Code != http.StatusFound || recorder.Header().Get("Location") != LoginPath+"?return_to=%2Fui%2Ftasks%3Fnext%3D1" {
		t.Fatalf("redirect = %d %q", recorder.Code, recorder.Header().Get("Location"))
	}

	disabled, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	disabled.Protect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ui/", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("disabled auth status = %d", recorder.Code)
	}
}

func TestSignedSessionIdentityAndRevocation(t *testing.T) {
	auth, err := New(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour).Unix()
	session := browserSession{ID: "session-1", Subject: "subject", Name: "Ada", Email: "ada@example.test", Role: "admin", Expires: expires}
	value, err := auth.sign(session)
	if err != nil {
		t.Fatal(err)
	}
	auth.store.put(session.ID, sessionRecord{Subject: session.Subject, Expires: expires})

	request := httptest.NewRequest(http.MethodGet, SessionPath, nil)
	request.AddCookie(&http.Cookie{Name: auth.config.CookieName, Value: value})
	recorder := httptest.NewRecorder()
	auth.session(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("session status = %d", recorder.Code)
	}
	var identity map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	if identity["name"] != "Ada" || identity["role"] != "admin" {
		t.Fatalf("identity = %#v", identity)
	}

	auth.store.delete(session.ID)
	recorder = httptest.NewRecorder()
	auth.session(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("revoked status = %d", recorder.Code)
	}
}

func TestBackchannelLogoutRevokesMatchingSessionAndRejectsReplay(t *testing.T) {
	auth, err := New(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	auth.store.put("target", sessionRecord{Subject: "subject", UpstreamSID: "upstream", Expires: now.Add(time.Hour).Unix()})
	auth.store.put("other", sessionRecord{Subject: "other", UpstreamSID: "other", Expires: now.Add(time.Hour).Unix()})
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: privateKey}, nil)
	if err != nil {
		t.Fatal(err)
	}
	signClaims := func(claims map[string]any) string {
		t.Helper()
		payload, err := json.Marshal(claims)
		if err != nil {
			t.Fatal(err)
		}
		signed, err := signer.Sign(payload)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := signed.CompactSerialize()
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	raw := signClaims(map[string]any{
		"iss": auth.config.Issuer, "sub": "subject", "sid": "upstream", "aud": auth.config.ClientID,
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "jti": "logout-1",
		"events": map[string]any{"http://schemas.openid.net/event/backchannel-logout": map[string]any{}},
	})
	verifier := oidc.NewVerifier(auth.config.Issuer, &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{privateKey.Public()}}, &oidc.Config{ClientID: auth.config.ClientID, Now: func() time.Time { return now }})
	if err := auth.processBackchannelLogout(context.Background(), raw, verifier, now); err != nil {
		t.Fatal(err)
	}
	if _, ok := auth.store.get("target", now); ok {
		t.Fatal("target session remained active")
	}
	if _, ok := auth.store.get("other", now); !ok {
		t.Fatal("unrelated session was revoked")
	}
	if err := auth.processBackchannelLogout(context.Background(), raw, verifier, now); err == nil {
		t.Fatal("replayed logout token was accepted")
	}
	missingIssuedAt := signClaims(map[string]any{
		"iss": auth.config.Issuer, "sub": "subject", "aud": auth.config.ClientID,
		"exp": now.Add(5 * time.Minute).Unix(), "jti": "logout-2",
		"events": map[string]any{backchannelLogoutEvent: map[string]any{}},
	})
	if err := auth.processBackchannelLogout(context.Background(), missingIssuedAt, verifier, now); err == nil {
		t.Fatal("logout token without iat was accepted")
	}
	invalidEvent := signClaims(map[string]any{
		"iss": auth.config.Issuer, "sub": "subject", "aud": auth.config.ClientID,
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "jti": "logout-3",
		"events": map[string]any{backchannelLogoutEvent: nil},
	})
	if err := auth.processBackchannelLogout(context.Background(), invalidEvent, verifier, now); err == nil {
		t.Fatal("logout token with a null event was accepted")
	}
}

func TestLogoutRejectsCrossOriginBeforeProviderAccess(t *testing.T) {
	auth, err := New(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, LogoutPath, nil)
	request.Header.Set("Origin", "https://attacker.example")
	recorder := httptest.NewRecorder()
	auth.logout(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-origin logout status = %d", recorder.Code)
	}
}

func TestLogoutRequiresSameOriginBrowserEvidence(t *testing.T) {
	for name, testCase := range map[string]struct {
		configure func(*http.Request)
		want      bool
	}{
		"same origin":                {configure: func(r *http.Request) { r.Header.Set("Origin", "https://sim.example.test") }, want: true},
		"same-origin referer":        {configure: func(r *http.Request) { r.Header.Set("Referer", "https://sim.example.test/ui/") }, want: true},
		"same-origin fetch metadata": {configure: func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-origin") }, want: true},
		"missing evidence":           {configure: func(*http.Request) {}, want: false},
		"cross-origin referer":       {configure: func(r *http.Request) { r.Header.Set("Referer", "https://attacker.example/") }, want: false},
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "https://sim.example.test"+LogoutPath, nil)
			testCase.configure(request)
			if got := sameOrigin(request, testConfig().PublicURL); got != testCase.want {
				t.Fatalf("sameOrigin() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestBackchannelLogoutRequiresFormBody(t *testing.T) {
	auth, err := New(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	for name, testCase := range map[string]struct {
		target, contentType string
		wantStatus          int
	}{
		"missing media type": {target: BackchannelLogoutPath, wantStatus: http.StatusUnsupportedMediaType},
		"JSON media type":    {target: BackchannelLogoutPath, contentType: "application/json", wantStatus: http.StatusUnsupportedMediaType},
		"query token":        {target: BackchannelLogoutPath + "?logout_token=query", contentType: "application/x-www-form-urlencoded", wantStatus: http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, testCase.target, strings.NewReader(""))
			if testCase.contentType != "" {
				request.Header.Set("Content-Type", testCase.contentType)
			}
			recorder := httptest.NewRecorder()
			auth.backchannelLogout(recorder, request)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, testCase.wantStatus)
			}
			if recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("Cache-Control = %q", recorder.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestBackchannelLogoutEventMustBeJSONObject(t *testing.T) {
	for name, testCase := range map[string]struct {
		raw  string
		want bool
	}{
		"empty object":     {raw: `{}`, want: true},
		"non-empty object": {raw: `{"reason":"admin"}`, want: true},
		"missing":          {raw: ``, want: false},
		"null":             {raw: `null`, want: false},
		"string":           {raw: `"logout"`, want: false},
		"array":            {raw: `[]`, want: false},
		"invalid":          {raw: `{`, want: false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := validLogoutEvent(json.RawMessage(testCase.raw)); got != testCase.want {
				t.Fatalf("validLogoutEvent(%q) = %v, want %v", testCase.raw, got, testCase.want)
			}
		})
	}
}

func TestSignedOutResponseIsNotCached(t *testing.T) {
	auth, err := New(testConfig())
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	auth.signedOut(recorder, httptest.NewRequest(http.MethodGet, SignedOutPath, nil))
	if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("signed-out response = %d Cache-Control=%q", recorder.Code, recorder.Header().Get("Cache-Control"))
	}
}
