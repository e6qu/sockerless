package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
	config := shauthConfig{issuer: "https://auth.dev.e6qu.dev", clientID: "client", clientSecret: "this-is-a-test-secret", publicURL: "https://admin.dev.e6qu.dev", insecure: true}
	handler := config.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/auth/shauth" {
		t.Fatalf("operator surface = %d %q, want Shauth redirect", response.Code, response.Header().Get("Location"))
	}

	value, err := config.sign(shauthSession{Subject: "subject", Name: "operator", Role: "developer", Expires: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatalf("sign session: %v", err)
	}
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
	config := shauthConfig{issuer: "https://auth.dev.e6qu.dev", clientID: "client", clientSecret: "this-is-a-test-secret", publicURL: "https://admin.dev.e6qu.dev"}
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
	config := shauthConfig{issuer: "https://auth.dev.e6qu.dev", clientID: "client", clientSecret: "this-is-a-test-secret", publicURL: "https://admin.dev.e6qu.dev"}
	value, err := config.sign(shauthSession{Subject: "subject", Name: "Taylor", Role: "admin", Expires: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatalf("sign session: %v", err)
	}
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
