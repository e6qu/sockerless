package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"mime"
	"net"
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
	shauthSignedOutPath     = "/auth/signed-out"
	shauthMaximumFormBytes  = 1 << 20
	shauthLogoutEvent       = "http://schemas.openid.net/event/backchannel-logout"
)

// shauthConfig is optional so the local operator workflow remains available.
// A deployed administrator console must set every coordinate and is then
// guarded by Shauth; simulator cloud API endpoints are never wrapped here.
type shauthConfig struct {
	issuer        string
	clientID      string
	clientSecret  string
	sessionSecret string
	publicURL     string
	insecure      bool
	sessions      *shauthSessionStore
}

func shauthConfigFromEnvironment() shauthConfig {
	return shauthConfig{
		issuer:        os.Getenv("SOCKERLESS_ADMIN_SHAUTH_ISSUER"),
		clientID:      os.Getenv("SOCKERLESS_ADMIN_SHAUTH_CLIENT_ID"),
		clientSecret:  os.Getenv("SOCKERLESS_ADMIN_SHAUTH_CLIENT_SECRET"),
		sessionSecret: os.Getenv("SOCKERLESS_ADMIN_SESSION_SECRET"),
		publicURL:     strings.TrimRight(os.Getenv("SOCKERLESS_ADMIN_PUBLIC_URL"), "/"),
		insecure:      os.Getenv("SOCKERLESS_ADMIN_INSECURE_COOKIES") == "true",
		sessions:      newSHAUTHSessionStore(),
	}
}

func (c shauthConfig) enabled() bool {
	return c.issuer != "" && c.clientID != "" && c.clientSecret != "" && c.publicURL != ""
}

func (c shauthConfig) validate() error {
	configured := 0
	for _, value := range []string{c.issuer, c.clientID, c.clientSecret, c.sessionSecret, c.publicURL} {
		if value != "" {
			configured++
		}
	}
	if configured == 0 {
		return nil
	}
	if configured != 5 {
		return fmt.Errorf("SOCKERLESS_ADMIN_SHAUTH_ISSUER, SOCKERLESS_ADMIN_SHAUTH_CLIENT_ID, SOCKERLESS_ADMIN_SHAUTH_CLIENT_SECRET, SOCKERLESS_ADMIN_SESSION_SECRET, and SOCKERLESS_ADMIN_PUBLIC_URL must be configured together")
	}
	if len(c.sessionSecret) < 32 {
		return fmt.Errorf("SOCKERLESS_ADMIN_SESSION_SECRET must contain at least 32 bytes")
	}
	for name, value := range map[string]string{"issuer": c.issuer, "public URL": c.publicURL} {
		parsed, err := url.ParseRequestURI(value)
		if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
			(parsed.Scheme != "https" && (!c.insecure || parsed.Scheme != "http" || !isSHAUTHLoopback(parsed.Hostname()))) {
			return fmt.Errorf("shauth issuer and public URL must be absolute HTTPS URLs")
		}
		if name == "public URL" && strings.TrimRight(parsed.Path, "/") != "" {
			return fmt.Errorf("shauth public URL must not contain a path")
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

func (s *shauthSessionStore) revoke(subject, upstreamSID string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(now)
	s.revokeLocked(subject, upstreamSID)
}

func (s *shauthSessionStore) revokeLocked(subject, upstreamSID string) {
	for id, session := range s.sessions {
		if (upstreamSID != "" && session.UpstreamSID == upstreamSID) ||
			(subject != "" && session.Subject == subject) {
			delete(s.sessions, id)
		}
	}
}

func (s *shauthSessionStore) consumeAndRevoke(id string, expiry time.Time, subject, upstreamSID string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune(now)
	if _, exists := s.logoutTokens[id]; exists {
		return false
	}
	s.logoutTokens[id] = expiry
	s.revokeLocked(subject, upstreamSID)
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
	mac := hmac.New(sha256.New, []byte(c.sessionSecret))
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
	mac := hmac.New(sha256.New, []byte(c.sessionSecret))
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
	return path == "/auth/shauth" || path == "/auth/shauth/callback" || path == "/auth/shauth/frontchannel-logout" || path == "/auth/shauth/backchannel-logout" || path == "/auth/logout" || path == "/auth/session" || path == shauthSignedOutPath
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
	oauthConfig := c.oauthConfig(provider)
	http.Redirect(w, r, oauthConfig.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

func (c shauthConfig) callback(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(shauthTransactionCookie)
	http.SetCookie(w, &http.Cookie{Name: shauthTransactionCookie, Path: "/auth/shauth", MaxAge: -1, HttpOnly: true, Secure: c.secureCookie(), SameSite: http.SameSiteLaxMode})
	if err != nil {
		http.Error(w, "Shauth transaction is missing", http.StatusBadRequest)
		return
	}
	var transaction shauthTransaction
	if err = c.verify(cookie.Value, &transaction); err != nil || transaction.Expires < time.Now().Unix() || subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("state")), []byte(transaction.State)) != 1 {
		http.Error(w, "Shauth transaction is invalid", http.StatusBadRequest)
		return
	}
	provider, err := oidc.NewProvider(r.Context(), c.issuer)
	if err != nil {
		http.Error(w, "Shauth discovery failed", http.StatusBadGateway)
		return
	}
	oauthConfig := c.oauthConfig(provider)
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
	if err := idToken.VerifyAccessToken(tokens.AccessToken); err != nil {
		http.Error(w, "Shauth access token verification failed", http.StatusUnauthorized)
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
	name := claims.PreferredUsername
	if name == "" {
		name = idToken.Subject
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
	expiresAt := time.Now().Add(8 * time.Hour)
	if idToken.Expiry.Before(expiresAt) {
		expiresAt = idToken.Expiry
	}
	expires := expiresAt.Unix()
	session, err := c.sign(shauthSession{ID: sessionID, Subject: idToken.Subject, Name: name, Role: claims.Role, Expires: expires})
	if err != nil {
		http.Error(w, "could not create Shauth session", http.StatusInternalServerError)
		return
	}
	c.sessions.put(sessionID, shauthSessionRecord{Subject: idToken.Subject, UpstreamSID: claims.SessionID, RawIDToken: rawIDToken, Expires: expires})
	http.SetCookie(w, &http.Cookie{Name: shauthSessionCookie, Value: session, Path: "/", HttpOnly: true, Secure: c.secureCookie(), SameSite: http.SameSiteLaxMode, MaxAge: int(time.Until(expiresAt).Seconds())})
	http.Redirect(w, r, "/ui/", http.StatusFound)
}

func (c shauthConfig) oauthConfig(provider *oidc.Provider) oauth2.Config {
	endpoint := provider.Endpoint()
	endpoint.AuthStyle = oauth2.AuthStyleInParams
	return oauth2.Config{
		ClientID: c.clientID, ClientSecret: c.clientSecret,
		Endpoint: endpoint, RedirectURL: c.publicURL + "/auth/shauth/callback",
		Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
	}
}

func (c shauthConfig) logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
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
	logoutURL, err := c.logoutURL(metadata.EndSessionEndpoint, rawIDToken)
	if err != nil {
		http.Error(w, "Shauth logout endpoint is invalid", http.StatusBadGateway)
		return
	}
	http.Redirect(w, r, logoutURL.String(), http.StatusFound)
}

func (c shauthConfig) logoutURL(endpoint, rawIDToken string) (*url.URL, error) {
	logoutURL, err := url.Parse(endpoint)
	if err != nil || !logoutURL.IsAbs() || !sameSHAUTHOrigin(logoutURL, c.issuer) ||
		(logoutURL.Scheme != "https" && (!c.insecure || logoutURL.Scheme != "http")) {
		return nil, errors.New("invalid Shauth logout endpoint")
	}
	query := logoutURL.Query()
	query.Set("client_id", c.clientID)
	query.Set("post_logout_redirect_uri", c.publicURL+shauthSignedOutPath)
	if rawIDToken != "" {
		query.Set("id_token_hint", rawIDToken)
	}
	logoutURL.RawQuery = query.Encode()
	return logoutURL, nil
}

func sameOriginRequest(r *http.Request, publicURL string) bool {
	want, err := url.Parse(publicURL)
	if err != nil {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		got, err := url.Parse(origin)
		return err == nil && sameSHAUTHParsedOrigin(got, want)
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		got, err := url.Parse(referer)
		return err == nil && sameSHAUTHParsedOrigin(got, want)
	}
	return r.Header.Get("Sec-Fetch-Site") == "same-origin" && r.Host == want.Host
}

func sameSHAUTHOrigin(got *url.URL, wantRaw string) bool {
	want, err := url.Parse(wantRaw)
	return err == nil && sameSHAUTHParsedOrigin(got, want)
}

func sameSHAUTHParsedOrigin(got, want *url.URL) bool {
	return got != nil && want != nil && got.IsAbs() && got.User == nil && got.Scheme == want.Scheme && got.Host == want.Host
}

func isSHAUTHLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c shauthConfig) backchannelLogout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !c.enabled() || c.sessions == nil {
		http.Error(w, "Shauth is not configured", http.StatusServiceUnavailable)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/x-www-form-urlencoded" {
		http.Error(w, "content type must be application/x-www-form-urlencoded", http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, shauthMaximumFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid logout request", http.StatusBadRequest)
		return
	}
	rawLogoutToken := r.PostForm.Get("logout_token")
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
	log.Printf("accepted Shauth back-channel logout for client %q", c.clientID)
	w.WriteHeader(http.StatusOK)
}

func (c shauthConfig) frontchannelLogout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors "+c.issuer+"; base-uri 'none'; form-action 'none'")
	if c.enabled() && c.sessions != nil && r.URL.Query().Get("iss") == c.issuer {
		c.sessions.revoke("", r.URL.Query().Get("sid"), time.Now())
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<!doctype html><html lang=en><title>Signed out</title><body>Signed out</body></html>"))
}

func (c shauthConfig) processBackchannelLogout(ctx context.Context, rawLogoutToken string, verifier *oidc.IDTokenVerifier, now time.Time) error {
	logoutToken, err := verifier.VerifyLogout(ctx, rawLogoutToken)
	if err != nil {
		return err
	}
	if logoutToken.IssuedAt.IsZero() {
		return fmt.Errorf("logout token is missing the iat claim")
	}
	if logoutToken.Expiry.IsZero() || !logoutToken.Expiry.After(now) {
		return fmt.Errorf("logout token is missing a valid exp claim")
	}
	var claims struct {
		Events map[string]json.RawMessage `json:"events"`
	}
	if err := logoutToken.Claims(&claims); err != nil || !validSHAUTHLogoutEvent(claims.Events[shauthLogoutEvent]) {
		return fmt.Errorf("logout token contains an invalid back-channel logout event")
	}
	if !c.sessions.consumeAndRevoke(logoutToken.TokenID, logoutToken.Expiry, logoutToken.Subject, logoutToken.SessionID, now) {
		return fmt.Errorf("logout token was already processed")
	}
	return nil
}

func validSHAUTHLogoutEvent(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var event map[string]json.RawMessage
	return json.Unmarshal(raw, &event) == nil && event != nil && len(event) == 0
}

func (c shauthConfig) signedOut(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'")
	_ = shauthSignedOutTemplate.Execute(w, nil)
}

var shauthSignedOutTemplate = template.Must(template.New("signed-out").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Signed out · Sockerless Admin</title><style>:root{color-scheme:light dark}body{margin:0;min-height:100vh;display:grid;place-items:center;background:#fff7ed;color:#21130f;font:16px system-ui,sans-serif}.card{max-width:34rem;padding:2.5rem;border:1px solid #fed7aa;border-radius:1.25rem;background:#fff;box-shadow:0 16px 40px #7c2d1214}a{display:inline-block;margin-top:1rem;padding:.7rem 1rem;border-radius:.6rem;background:#ea580c;color:#fff;font-weight:700;text-decoration:none}a:focus-visible{outline:3px solid #0ea5e9;outline-offset:3px}@media(prefers-color-scheme:dark){body{background:#160c09;color:#fff7ed}.card{background:#26130d;border-color:#7c2d12}}</style></head><body><main class="card"><h1>Signed out of Sockerless Admin</h1><p>Your Sockerless Admin session and shared Shauth session have ended.</p><a href="/auth/shauth">Sign in again</a></main></body></html>`))

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
