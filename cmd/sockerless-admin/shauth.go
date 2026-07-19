package main

import (
	"context"
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
	"sync"
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
	sessions     *shauthSessionStore
}

func shauthConfigFromEnvironment() shauthConfig {
	return shauthConfig{
		issuer:       strings.TrimRight(os.Getenv("SOCKERLESS_ADMIN_SHAUTH_ISSUER"), "/"),
		clientID:     os.Getenv("SOCKERLESS_ADMIN_SHAUTH_CLIENT_ID"),
		clientSecret: os.Getenv("SOCKERLESS_ADMIN_SHAUTH_CLIENT_SECRET"),
		publicURL:    strings.TrimRight(os.Getenv("SOCKERLESS_ADMIN_PUBLIC_URL"), "/"),
		insecure:     os.Getenv("SOCKERLESS_ADMIN_INSECURE_COOKIES") == "true",
		sessions:     newSHAUTHSessionStore(),
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
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Expires int64  `json:"expires"`
}

type shauthSessionRecord struct {
	Subject     string
	UpstreamSID string
	RawIDToken  string
	Expires     int64
}

type shauthSessionStore struct {
	mu           sync.Mutex
	sessions     map[string]shauthSessionRecord
	logoutTokens map[string]time.Time
}

func newSHAUTHSessionStore() *shauthSessionStore {
	return &shauthSessionStore{
		sessions:     make(map[string]shauthSessionRecord),
		logoutTokens: make(map[string]time.Time),
	}
}

func (s *shauthSessionStore) put(id string, record shauthSessionRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = record
}

func (s *shauthSessionStore) get(id string, now time.Time) (shauthSessionRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(now)
	record, ok := s.sessions[id]
	return record, ok
}

func (s *shauthSessionStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *shauthSessionStore) revoke(subject, upstreamSID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, session := range s.sessions {
		if (upstreamSID != "" && session.UpstreamSID == upstreamSID) ||
			(subject != "" && session.Subject == subject) {
			delete(s.sessions, id)
		}
	}
}

func (s *shauthSessionStore) consumeLogoutToken(id string, expiry, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(now)
	if _, exists := s.logoutTokens[id]; exists {
		return false
	}
	s.logoutTokens[id] = expiry
	return true
}

func (s *shauthSessionStore) prune(now time.Time) {
	for id, session := range s.sessions {
		if session.Expires <= now.Unix() {
			delete(s.sessions, id)
		}
	}
	for id, expiry := range s.logoutTokens {
		if !expiry.After(now) {
			delete(s.logoutTokens, id)
		}
	}
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
		if err == nil && c.sessionIsActive(session, time.Now()) {
			next.ServeHTTP(w, r)
			return
		}
		http.Redirect(w, r, "/auth/shauth", http.StatusFound)
	})
}

func isSHAUTHRoute(path string) bool {
	return path == "/auth/shauth" || path == "/auth/shauth/callback" || path == "/auth/shauth/backchannel-logout" || path == "/auth/logout" || path == "/auth/session"
}

func (c shauthConfig) sessionIsActive(session shauthSession, now time.Time) bool {
	if c.sessions == nil || session.ID == "" || session.Expires <= now.Unix() {
		return false
	}
	record, ok := c.sessions.get(session.ID, now)
	return ok && record.Subject == session.Subject && record.Expires == session.Expires
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
		SessionID         string `json:"sid"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Nonce != transaction.Nonce || (claims.Role != "admin" && claims.Role != "developer") {
		http.Error(w, "Shauth identity is not authorized", http.StatusForbidden)
		return
	}
	if c.sessions == nil {
		http.Error(w, "Shauth session storage is unavailable", http.StatusInternalServerError)
		return
	}
	sessionID, err := randomSHAUTHValue()
	if err != nil {
		http.Error(w, "could not create Shauth session", http.StatusInternalServerError)
		return
	}
	expires := time.Now().Add(8 * time.Hour).Unix()
	session, err := c.sign(shauthSession{ID: sessionID, Subject: idToken.Subject, Name: claims.PreferredUsername, Role: claims.Role, Expires: expires})
	if err != nil {
		http.Error(w, "could not create Shauth session", http.StatusInternalServerError)
		return
	}
	c.sessions.put(sessionID, shauthSessionRecord{Subject: idToken.Subject, UpstreamSID: claims.SessionID, RawIDToken: rawIDToken, Expires: expires})
	http.SetCookie(w, &http.Cookie{Name: shauthSessionCookie, Value: session, Path: "/", HttpOnly: true, Secure: c.secureCookie(), SameSite: http.SameSiteLaxMode, MaxAge: 8 * 60 * 60})
	http.Redirect(w, r, "/ui/", http.StatusFound)
}

func (c shauthConfig) logout(w http.ResponseWriter, r *http.Request) {
	if !sameOriginRequest(r, c.publicURL) {
		http.Error(w, "cross-origin request denied", http.StatusForbidden)
		return
	}
	var rawIDToken string
	if cookie, err := r.Cookie(shauthSessionCookie); err == nil {
		var session shauthSession
		if c.verify(cookie.Value, &session) == nil && c.sessions != nil {
			if record, ok := c.sessions.get(session.ID, time.Now()); ok {
				rawIDToken = record.RawIDToken
			}
			c.sessions.delete(session.ID)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: shauthSessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: c.secureCookie(), SameSite: http.SameSiteLaxMode})
	if !c.enabled() {
		http.Redirect(w, r, "/ui/", http.StatusFound)
		return
	}
	provider, err := oidc.NewProvider(r.Context(), c.issuer)
	if err != nil {
		http.Error(w, "Shauth discovery failed", http.StatusBadGateway)
		return
	}
	var metadata struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := provider.Claims(&metadata); err != nil || metadata.EndSessionEndpoint == "" {
		http.Error(w, "Shauth logout endpoint is unavailable", http.StatusBadGateway)
		return
	}
	logoutURL, err := url.Parse(metadata.EndSessionEndpoint)
	if err != nil || !logoutURL.IsAbs() || logoutURL.Scheme != "https" {
		http.Error(w, "Shauth logout endpoint is invalid", http.StatusBadGateway)
		return
	}
	query := logoutURL.Query()
	query.Set("post_logout_redirect_uri", c.publicURL+"/")
	if rawIDToken != "" {
		query.Set("id_token_hint", rawIDToken)
	}
	logoutURL.RawQuery = query.Encode()
	http.Redirect(w, r, logoutURL.String(), http.StatusFound)
}

func sameOriginRequest(r *http.Request, publicURL string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	want, err := url.Parse(publicURL)
	if err != nil {
		return false
	}
	got, err := url.Parse(origin)
	return err == nil && got.Scheme == want.Scheme && got.Host == want.Host
}

func (c shauthConfig) backchannelLogout(w http.ResponseWriter, r *http.Request) {
	if !c.enabled() || c.sessions == nil {
		http.Error(w, "Shauth is not configured", http.StatusServiceUnavailable)
		return
	}
	rawLogoutToken := r.PostFormValue("logout_token")
	if rawLogoutToken == "" {
		http.Error(w, "logout_token is required", http.StatusBadRequest)
		return
	}
	provider, err := oidc.NewProvider(r.Context(), c.issuer)
	if err != nil {
		http.Error(w, "Shauth discovery failed", http.StatusBadGateway)
		return
	}
	if err := c.processBackchannelLogout(r.Context(), rawLogoutToken, provider.Verifier(&oidc.Config{ClientID: c.clientID}), time.Now()); err != nil {
		http.Error(w, "invalid logout token", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (c shauthConfig) processBackchannelLogout(ctx context.Context, rawLogoutToken string, verifier *oidc.IDTokenVerifier, now time.Time) error {
	logoutToken, err := verifier.VerifyLogout(ctx, rawLogoutToken)
	if err != nil {
		return err
	}
	if !c.sessions.consumeLogoutToken(logoutToken.TokenID, logoutToken.Expiry, now) {
		return fmt.Errorf("logout token was already processed")
	}
	c.sessions.revoke(logoutToken.Subject, logoutToken.SessionID)
	return nil
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
	if err != nil || !c.sessionIsActive(value, time.Now()) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "name": value.Name, "role": value.Role})
}
