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
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	shauthTransactionCookie  = "sockerless_admin_shauth_tx"
	shauthSessionCookie      = "sockerless_admin_shauth_session"
	shauthLogoutCompletePath = "/auth/shauth/logout/complete"
	shauthSignedOutPath      = "/auth/signed-out"
	shauthValidationPath     = "/auth/validation"
	shauthAdministratorRole  = "admin"
	shauthMaximumFormBytes   = 1 << 20
	shauthLogoutEvent        = "http://schemas.openid.net/event/backchannel-logout"
	shauthDiscoveryTimeout   = 10 * time.Second
)

var immutableSHAUTHApplicationRelease = regexp.MustCompile(`^(?:[0-9a-f]{12,64}|sha256:[0-9a-f]{64})$`)

// shauthConfig is optional so the local operator workflow remains available.
// A deployed administrator console must set every coordinate and is then
// guarded by Shauth; simulator cloud API endpoints are never wrapped here.
type shauthConfig struct {
	issuer          string
	clientID        string
	clientSecret    string
	sessionSecret   string
	publicURL       string
	releaseRevision string
	insecure        bool
	sessions        *shauthSessionStore
	providerCache   *shauthProviderCache
}

type shauthProviderCache struct {
	mu       sync.Mutex
	provider *oidc.Provider
}

func shauthConfigFromEnvironment() shauthConfig {
	return shauthConfig{
		issuer:          os.Getenv("SOCKERLESS_ADMIN_SHAUTH_ISSUER"),
		clientID:        os.Getenv("SOCKERLESS_ADMIN_SHAUTH_CLIENT_ID"),
		clientSecret:    os.Getenv("SOCKERLESS_ADMIN_SHAUTH_CLIENT_SECRET"),
		sessionSecret:   os.Getenv("SOCKERLESS_ADMIN_SESSION_SECRET"),
		publicURL:       strings.TrimRight(os.Getenv("SOCKERLESS_ADMIN_PUBLIC_URL"), "/"),
		releaseRevision: strings.TrimSpace(os.Getenv("APPLICATION_RELEASE_REVISION")),
		insecure:        os.Getenv("SOCKERLESS_ADMIN_INSECURE_COOKIES") == "true",
		sessions:        newSHAUTHSessionStore(),
		providerCache:   &shauthProviderCache{},
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
	if !immutableSHAUTHApplicationRelease.MatchString(c.releaseRevision) {
		return fmt.Errorf("APPLICATION_RELEASE_REVISION must identify an immutable deployed release")
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
	Email   string `json:"email"`
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

func (c shauthConfig) providerFor(ctx context.Context) (*oidc.Provider, error) {
	if c.providerCache == nil {
		return nil, errors.New("shauth provider cache is unavailable")
	}
	c.providerCache.mu.Lock()
	defer c.providerCache.mu.Unlock()
	if c.providerCache.provider != nil {
		return c.providerCache.provider, nil
	}
	discoveryContext, cancel := context.WithTimeout(ctx, shauthDiscoveryTimeout)
	defer cancel()
	provider, err := oidc.NewProvider(discoveryContext, c.issuer)
	if err != nil {
		return nil, err
	}
	c.providerCache.provider = provider
	return provider, nil
}

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
			if session.Role == shauthAdministratorRole {
				next.ServeHTTP(w, r)
				return
			}
			c.forbidden(w, r, session)
			return
		}
		http.Redirect(w, r, "/auth/shauth", http.StatusFound)
	})
}

func (c shauthConfig) forbidden(w http.ResponseWriter, r *http.Request, session shauthSession) {
	w.Header().Set("Cache-Control", "no-store")
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Shauth administrator role required"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'; form-action 'self' "+shauthOrigin(c.issuer))
	w.WriteHeader(http.StatusForbidden)
	if err := shauthForbiddenTemplate.Execute(w, struct {
		Name string
		Role string
	}{Name: session.Name, Role: session.Role}); err != nil {
		log.Printf("render Shauth authorization denial: %v", err)
	}
}

func isSHAUTHRoute(path string) bool {
	return path == "/auth/shauth" || path == "/auth/shauth/callback" || path == "/auth/shauth/frontchannel-logout" || path == "/auth/shauth/backchannel-logout" || path == "/auth/logout" || path == "/auth/session" || path == shauthLogoutCompletePath || path == shauthSignedOutPath || path == shauthValidationPath
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
	provider, err := c.providerFor(r.Context())
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
	provider, err := c.providerFor(r.Context())
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
		Email             string `json:"email"`
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
	session, err := c.sign(shauthSession{ID: sessionID, Subject: idToken.Subject, Name: name, Email: claims.Email, Role: claims.Role, Expires: expires})
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
	discoveryStarted := time.Now()
	provider, err := c.providerFor(r.Context())
	if err != nil {
		log.Printf("Shauth logout discovery failed for client %q after %s", c.clientID, time.Since(discoveryStarted).Round(time.Millisecond))
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
	log.Printf("initiated Shauth global logout for client %q after %s", c.clientID, time.Since(discoveryStarted).Round(time.Millisecond))
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
	query.Set("post_logout_redirect_uri", c.publicURL+shauthLogoutCompletePath)
	if rawIDToken != "" {
		query.Set("id_token_hint", rawIDToken)
	}
	logoutURL.RawQuery = query.Encode()
	return logoutURL, nil
}

// logoutComplete ignores request input and returns to Shauth's fixed
// completion endpoint.
func (c shauthConfig) logoutComplete(w http.ResponseWriter, r *http.Request) {
	target, err := url.Parse(c.issuer)
	if err != nil || !target.IsAbs() || target.Host == "" || target.User != nil {
		http.Error(w, "Shauth issuer is invalid", http.StatusInternalServerError)
		return
	}
	target.Path = "/oauth/logout/complete"
	target.RawPath = ""
	target.RawQuery = ""
	target.Fragment = ""
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, target.String(), http.StatusSeeOther)
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

func shauthOrigin(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "'none'"
	}
	return parsed.Scheme + "://" + parsed.Host
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
	provider, err := c.providerFor(r.Context())
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
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors "+shauthOrigin(c.issuer)+"; base-uri 'none'; form-action 'none'")
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

func (c shauthConfig) validation(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(shauthSessionCookie)
	var session shauthSession
	if err == nil {
		err = c.verify(cookie.Value, &session)
	}
	if err != nil || !c.sessionIsActive(session, time.Now()) {
		http.Redirect(w, r, shauthSignedOutPath, http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'; form-action 'self' "+shauthOrigin(c.issuer))
	_ = shauthValidationTemplate.Execute(w, map[string]string{
		"Username": session.Name, "Email": session.Email, "Role": session.Role, "Release": c.releaseRevision,
	})
}

var shauthSignedOutTemplate = template.Must(template.New("signed-out").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Signed out · Sockerless Admin</title><style>:root{color-scheme:light dark}body{margin:0;min-height:100vh;display:grid;place-items:center;background:#fff7ed;color:#21130f;font:16px system-ui,sans-serif}.card{max-width:34rem;padding:2.5rem;border:1px solid #fed7aa;border-radius:1.25rem;background:#fff;box-shadow:0 16px 40px #7c2d1214}a{display:inline-block;margin-top:1rem;padding:.7rem 1rem;border-radius:.6rem;background:#ea580c;color:#fff;font-weight:700;text-decoration:none}a:focus-visible{outline:3px solid #0ea5e9;outline-offset:3px}@media(prefers-color-scheme:dark){body{background:#160c09;color:#fff7ed}.card{background:#26130d;border-color:#7c2d12}}</style></head><body><main class="card" aria-labelledby="signed-out-title"><h1 id="signed-out-title">Signed out of Sockerless Admin</h1><p role="status">Your Sockerless Admin session and shared Shauth session have ended.</p><a href="/auth/shauth">Sign in with Shauth</a></main></body></html>`))

var shauthValidationTemplate = template.Must(template.New("validation").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Authentication validation · Sockerless Admin</title><style>:root{color-scheme:light dark}body{margin:0;min-height:100vh;display:grid;place-items:center;background:#fff7ed;color:#21130f;font:16px system-ui,sans-serif}.card{width:min(34rem,calc(100% - 2rem));padding:2.5rem;border:1px solid #fed7aa;border-radius:1.25rem;background:#fff;box-shadow:0 16px 40px #7c2d1214}dl{display:grid;grid-template-columns:max-content 1fr;gap:.65rem 1rem}dt{font-weight:700}dd{margin:0;overflow-wrap:anywhere}button{padding:.7rem 1rem;border:0;border-radius:.6rem;background:#ea580c;color:#fff;font:inherit;font-weight:700;cursor:pointer}button:focus-visible{outline:3px solid #0ea5e9;outline-offset:3px}@media(prefers-color-scheme:dark){body{background:#160c09;color:#fff7ed}.card{background:#26130d;border-color:#7c2d12}}</style></head><body><main class="card" aria-labelledby="validation-title"><h1 id="validation-title">Signed in to Sockerless Admin</h1><dl><dt>Username</dt><dd data-testid="validation-username">{{.Username}}</dd><dt>Email</dt><dd data-testid="validation-email">{{.Email}}</dd><dt>Role</dt><dd data-testid="validation-role">{{.Role}}</dd><dt>Release</dt><dd data-testid="validation-release">{{.Release}}</dd></dl><form method="post" action="/auth/logout"><button type="submit">Sign out</button></form></main></body></html>`))

var shauthForbiddenTemplate = template.Must(template.New("forbidden").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Administrator access required · Sockerless Admin</title><style>:root{color-scheme:light dark}body{margin:0;min-height:100vh;display:grid;place-items:center;background:#fff7ed;color:#21130f;font:16px system-ui,sans-serif}.card{max-width:36rem;padding:2.5rem;border:1px solid #fed7aa;border-radius:1.25rem;background:#fff;box-shadow:0 16px 40px #7c2d1214}.eyebrow{font-weight:800;letter-spacing:.08em;text-transform:uppercase;color:#c2410c}button{margin-top:1rem;padding:.7rem 1rem;border:0;border-radius:.6rem;background:#ea580c;color:#fff;font:inherit;font-weight:700;cursor:pointer}button:focus-visible{outline:3px solid #0ea5e9;outline-offset:3px}@media(prefers-color-scheme:dark){body{background:#160c09;color:#fff7ed}.card{background:#26130d;border-color:#7c2d12}.eyebrow{color:#fb923c}}</style></head><body><main class="card"><p class="eyebrow">Access denied</p><h1>Administrator access required</h1><p>{{if .Name}}{{.Name}} is{{else}}You are{{end}} signed in with the <strong>{{.Role}}</strong> role. Sockerless Admin contains operator controls and requires the Shauth administrator role.</p><form method="post" action="/auth/logout"><button type="submit">Sign out</button></form></main></body></html>`))

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
