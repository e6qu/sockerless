package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	shauthTransactionCookie = "sockerless_admin_shauth_tx"
	shauthSessionCookie     = "sockerless_admin_shauth_session"
)

// shauthConfig is optional so the local operator workflow remains available.
// A deployed administrator console must set every coordinate and is then
// guarded by Shauth; simulator cloud API endpoints are never wrapped here.
type shauthConfig struct {
	issuer       string
	clientID     string
	clientSecret string
	publicURL    string
	insecure     bool
}

func shauthConfigFromEnvironment() shauthConfig {
	return shauthConfig{
		issuer:       strings.TrimRight(os.Getenv("SOCKERLESS_ADMIN_SHAUTH_ISSUER"), "/"),
		clientID:     os.Getenv("SOCKERLESS_ADMIN_SHAUTH_CLIENT_ID"),
		clientSecret: os.Getenv("SOCKERLESS_ADMIN_SHAUTH_CLIENT_SECRET"),
		publicURL:    strings.TrimRight(os.Getenv("SOCKERLESS_ADMIN_PUBLIC_URL"), "/"),
		insecure:     os.Getenv("SOCKERLESS_ADMIN_INSECURE_COOKIES") == "true",
	}
}

func (c shauthConfig) enabled() bool {
	return c.issuer != "" && c.clientID != "" && c.clientSecret != "" && c.publicURL != ""
}

func (c shauthConfig) validate() error {
	configured := 0
	for _, value := range []string{c.issuer, c.clientID, c.clientSecret, c.publicURL} {
		if value != "" {
			configured++
		}
	}
	if configured == 0 {
		return nil
	}
	if configured != 4 {
		return fmt.Errorf("SOCKERLESS_ADMIN_SHAUTH_ISSUER, SOCKERLESS_ADMIN_SHAUTH_CLIENT_ID, SOCKERLESS_ADMIN_SHAUTH_CLIENT_SECRET, and SOCKERLESS_ADMIN_PUBLIC_URL must be configured together")
	}
	for _, value := range []string{c.issuer, c.publicURL} {
		parsed, err := url.ParseRequestURI(value)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return fmt.Errorf("shauth issuer and public URL must be absolute HTTPS URLs")
		}
	}
	return nil
}

type shauthTransaction struct {
	State    string `json:"state"`
	Nonce    string `json:"nonce"`
	Verifier string `json:"verifier"`
	Expires  int64  `json:"expires"`
}

type shauthSession struct {
	Subject string `json:"subject"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Expires int64  `json:"expires"`
}

func randomSHAUTHValue() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func (c shauthConfig) sign(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(c.clientSecret))
	_, _ = mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (c shauthConfig) verify(value string, destination any) error {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return fmt.Errorf("invalid signed value")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("invalid signed value")
	}
	mac := hmac.New(sha256.New, []byte(c.clientSecret))
	_, _ = mac.Write([]byte(parts[0]))
	if subtle.ConstantTimeCompare(signature, mac.Sum(nil)) != 1 {
		return fmt.Errorf("invalid signed value")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("invalid signed value")
	}
	return json.Unmarshal(payload, destination)
}

func (c shauthConfig) secureCookie() bool { return !c.insecure }

func (c shauthConfig) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !c.enabled() || isSHAUTHRoute(r.URL.Path) || r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(shauthSessionCookie)
		var session shauthSession
		if err == nil {
			err = c.verify(cookie.Value, &session)
		}
		if err == nil && session.Expires > time.Now().Unix() {
			next.ServeHTTP(w, r)
			return
		}
		http.Redirect(w, r, "/auth/shauth", http.StatusFound)
	})
}

func isSHAUTHRoute(path string) bool {
	return path == "/auth/shauth" || path == "/auth/shauth/callback" || path == "/auth/logout" || path == "/auth/session"
}

func (c shauthConfig) login(w http.ResponseWriter, r *http.Request) {
	if !c.enabled() {
		http.Error(w, "Shauth is not configured", http.StatusServiceUnavailable)
		return
	}
	provider, err := oidc.NewProvider(r.Context(), c.issuer)
	if err != nil {
		http.Error(w, "Shauth discovery failed", http.StatusBadGateway)
		return
	}
	state, err := randomSHAUTHValue()
	if err != nil {
		http.Error(w, "could not create Shauth transaction", http.StatusInternalServerError)
		return
	}
	nonce, err := randomSHAUTHValue()
	if err != nil {
		http.Error(w, "could not create Shauth transaction", http.StatusInternalServerError)
		return
	}
	verifier, err := randomSHAUTHValue()
	if err != nil {
		http.Error(w, "could not create Shauth transaction", http.StatusInternalServerError)
		return
	}
	transaction, err := c.sign(shauthTransaction{State: state, Nonce: nonce, Verifier: verifier, Expires: time.Now().Add(10 * time.Minute).Unix()})
	if err != nil {
		http.Error(w, "could not create Shauth transaction", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: shauthTransactionCookie, Value: transaction, Path: "/auth/shauth", HttpOnly: true, Secure: c.secureCookie(), SameSite: http.SameSiteLaxMode, MaxAge: 600})
	oauthConfig := oauth2.Config{ClientID: c.clientID, ClientSecret: c.clientSecret, Endpoint: provider.Endpoint(), RedirectURL: c.publicURL + "/auth/shauth/callback", Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}
	http.Redirect(w, r, oauthConfig.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

func (c shauthConfig) callback(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(shauthTransactionCookie)
	if err != nil {
		http.Error(w, "Shauth transaction is missing", http.StatusBadRequest)
		return
	}
	var transaction shauthTransaction
	if err = c.verify(cookie.Value, &transaction); err != nil || transaction.Expires < time.Now().Unix() || subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("state")), []byte(transaction.State)) != 1 {
		http.Error(w, "Shauth transaction is invalid", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: shauthTransactionCookie, Path: "/auth/shauth", MaxAge: -1, HttpOnly: true, Secure: c.secureCookie(), SameSite: http.SameSiteLaxMode})
	provider, err := oidc.NewProvider(r.Context(), c.issuer)
	if err != nil {
		http.Error(w, "Shauth discovery failed", http.StatusBadGateway)
		return
	}
	oauthConfig := oauth2.Config{ClientID: c.clientID, ClientSecret: c.clientSecret, Endpoint: provider.Endpoint(), RedirectURL: c.publicURL + "/auth/shauth/callback"}
	tokens, err := oauthConfig.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(transaction.Verifier))
	if err != nil {
		http.Error(w, "Shauth code exchange failed", http.StatusUnauthorized)
		return
	}
	rawIDToken, ok := tokens.Extra("id_token").(string)
	if !ok {
		http.Error(w, "Shauth did not return an ID token", http.StatusUnauthorized)
		return
	}
	idToken, err := provider.Verifier(&oidc.Config{ClientID: c.clientID}).Verify(r.Context(), rawIDToken)
	if err != nil {
		http.Error(w, "Shauth ID token verification failed", http.StatusUnauthorized)
		return
	}
	var claims struct {
		Nonce             string `json:"nonce"`
		PreferredUsername string `json:"preferred_username"`
		Role              string `json:"role"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Nonce != transaction.Nonce || (claims.Role != "admin" && claims.Role != "developer") {
		http.Error(w, "Shauth identity is not authorized", http.StatusForbidden)
		return
	}
	session, err := c.sign(shauthSession{Subject: idToken.Subject, Name: claims.PreferredUsername, Role: claims.Role, Expires: time.Now().Add(8 * time.Hour).Unix()})
	if err != nil {
		http.Error(w, "could not create Shauth session", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: shauthSessionCookie, Value: session, Path: "/", HttpOnly: true, Secure: c.secureCookie(), SameSite: http.SameSiteLaxMode, MaxAge: 8 * 60 * 60})
	http.Redirect(w, r, "/ui/", http.StatusFound)
}

func (c shauthConfig) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: shauthSessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: c.secureCookie(), SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/ui/", http.StatusFound)
}

func (c shauthConfig) session(w http.ResponseWriter, r *http.Request) {
	if !c.enabled() {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
		return
	}
	cookie, err := r.Cookie(shauthSessionCookie)
	var value shauthSession
	if err == nil {
		err = c.verify(cookie.Value, &value)
	}
	if err != nil || value.Expires <= time.Now().Unix() {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "name": value.Name, "role": value.Role})
}
