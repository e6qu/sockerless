package main

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

func testSHAUTHConfig() shauthConfig {
	return shauthConfig{
		issuer:       "https://auth.dev.e6qu.dev",
		clientID:     "client",
		clientSecret: "this-is-a-test-secret",
		publicURL:    "https://admin.dev.e6qu.dev",
		sessions:     newSHAUTHSessionStore(),
	}
}

func TestSHAUTHConfigRequiresCompleteHTTPSCoordinates(t *testing.T) {
	if err := (shauthConfig{}).validate(); err != nil {
		t.Fatalf("disabled configuration: %v", err)
	}
	if err := (shauthConfig{issuer: "https://auth.dev.e6qu.dev", clientID: "client"}).validate(); err == nil {
		t.Fatal("partial configuration was accepted")
	}
	if err := (shauthConfig{issuer: "http://auth.dev.e6qu.dev", clientID: "client", clientSecret: "secret", publicURL: "https://admin.dev.e6qu.dev"}).validate(); err == nil {
		t.Fatal("non-HTTPS issuer was accepted")
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
	value, err := config.sign(shauthSession{ID: "session-1", Subject: "subject", Name: "operator", Role: "developer", Expires: expires})
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
	payload, err := json.Marshal(map[string]any{
		"iss":    config.issuer,
		"sub":    "subject",
		"sid":    "upstream-session",
		"aud":    config.clientID,
		"iat":    now.Unix(),
		"exp":    now.Add(5 * time.Minute).Unix(),
		"jti":    "logout-token-1",
		"events": map[string]any{"http://schemas.openid.net/event/backchannel-logout": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("encode logout token: %v", err)
	}
	signed, err := signer.Sign(payload)
	if err != nil {
		t.Fatalf("sign logout token: %v", err)
	}
	rawLogoutToken, err := signed.CompactSerialize()
	if err != nil {
		t.Fatalf("serialize logout token: %v", err)
	}
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
