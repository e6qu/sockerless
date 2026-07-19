package uiauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	payload, err := json.Marshal(map[string]any{
		"iss": auth.config.Issuer, "sub": "subject", "sid": "upstream", "aud": auth.config.ClientID,
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "jti": "logout-1",
		"events": map[string]any{"http://schemas.openid.net/event/backchannel-logout": map[string]any{}},
	})
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
