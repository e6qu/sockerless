package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// azureSimSigningKey is the per-process RS256 key used to sign Azure AD
// tokens the sim mints. Real Azure AD publishes public RSA signing keys
// via JWKS; the simulator does the same so downstream data-plane clients
// can verify bearer tokens without shared secrets.
var (
	azureSimSigningKeyOnce sync.Once
	azureSimSigningKeyVal  *rsa.PrivateKey
	azureSimSigningKeyErr  error

	azureAuthCodeMu    sync.Mutex
	azureAuthCodeStore = map[string]azureAuthCode{}

	azureRefreshTokenMu    sync.Mutex
	azureRefreshTokenStore = map[string]azureRefreshToken{}
)

const defaultAzureTokenAudience = "https://management.azure.com/"
const azureAuthCodeTTL = time.Minute

type azureAuthCode struct {
	TenantID            string
	ClientID            string
	RedirectURI         string
	Scope               string
	Nonce               string
	CodeChallenge       string
	CodeChallengeMethod string
	ExpiresAt           time.Time
}

type azureRefreshToken struct {
	TenantID string
	ClientID string
	Scope    string
	Nonce    string
}

var azureScopeAudienceOverrides = map[string]string{
	"https://management.azure.com": defaultAzureTokenAudience,
	"https://storage.azure.com":    "https://storage.azure.com/",
}

func azureSimSigningKey() (*rsa.PrivateKey, error) {
	azureSimSigningKeyOnce.Do(func() {
		azureSimSigningKeyVal, azureSimSigningKeyErr = rsa.GenerateKey(rand.Reader, 2048)
	})
	return azureSimSigningKeyVal, azureSimSigningKeyErr
}

// CleanPathMiddleware removes double slashes from request paths.
// The azurerm v3 provider (go-azure-sdk) constructs URLs by joining
// the resourceManager endpoint (with trailing slash) and the resource path
// (with leading slash), producing "//subscriptions/..." paths. Go's default
// mux 301-redirects these, which changes PUT→GET and breaks creates.
func CleanPathMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for strings.Contains(r.URL.Path, "//") {
			r.URL.Path = strings.ReplaceAll(r.URL.Path, "//", "/")
		}
		if r.URL.RawPath != "" {
			for strings.Contains(r.URL.RawPath, "//") {
				r.URL.RawPath = strings.ReplaceAll(r.URL.RawPath, "//", "/")
			}
		}
		next.ServeHTTP(w, r)
	})
}

// AzureARMAPIVersionMiddleware enforces the ARM control-plane api-version
// contract. Azure data planes and metadata endpoints have their own versioning
// rules, so only ARM resource paths are checked here.
func AzureARMAPIVersionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAzureARMPath(r.URL.Path) && r.URL.Query().Get("api-version") == "" {
			sim.AzureError(w, "InvalidApiVersionParameter",
				"The api-version query parameter (?api-version=) is required for all requests.",
				http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isAzureARMPath(path string) bool {
	return strings.HasPrefix(path, "/subscriptions/") || strings.HasPrefix(path, "/providers/")
}

// AzureAuthMiddleware intercepts OAuth2 and OpenID discovery requests needed
// by the Azure SDK for authentication. This is implemented as middleware
// rather than registered routes to avoid conflicts with ACR's /v2/{path...}.
func AzureAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Authorization endpoint: GET /{tenantId}/oauth2/v2.0/authorize
		if r.Method == http.MethodGet && strings.Contains(path, "/oauth2/v2.0/authorize") {
			handleAzureAuthorize(w, r, path)
			return
		}

		// Token endpoint: POST /{tenantId}/oauth2/v2.0/token
		if r.Method == http.MethodPost && strings.Contains(path, "/oauth2/v2.0/token") {
			handleAzureToken(w, r, path)
			return
		}
		// Token endpoint v1: POST /{tenantId}/oauth2/token. The Azure AD v1 endpoint
		// always carries a tenant prefix; the bare /oauth2/token (and /oauth2/exchange)
		// are ACR's registry-token endpoints, which must fall through to the ACR mux
		// routes rather than be handled as an AAD token request.
		if r.Method == http.MethodPost && strings.Contains(path, "/oauth2/token") &&
			path != "/oauth2/token" && path != "/oauth2/exchange" {
			handleAzureToken(w, r, path)
			return
		}

		// OpenID discovery endpoints
		if r.Method == http.MethodGet && strings.HasSuffix(path, "/.well-known/openid-configuration") {
			tenantId := extractTenantFromPath(path)
			baseURL := azureAuthBaseURL(r)
			// external: `issuer` is a JWT-spec claim convention,
			// not a routable URL. azidentity verifies token
			// signatures against the JWKS at `jwks_uri` below
			// (which IS sim-hosted) and compares `iss` against
			// the expected value rather than dereferencing it.
			sim.WriteJSON(w, http.StatusOK, map[string]any{
				"issuer":                                fmt.Sprintf("https://sts.windows.net/%s/", tenantId),
				"authorization_endpoint":                fmt.Sprintf("%s/%s/oauth2/v2.0/authorize", baseURL, tenantId),
				"token_endpoint":                        fmt.Sprintf("%s/%s/oauth2/v2.0/token", baseURL, tenantId),
				"jwks_uri":                              fmt.Sprintf("%s/%s/discovery/v2.0/keys", baseURL, tenantId),
				"response_types_supported":              []string{"code"},
				"response_modes_supported":              []string{"query", "fragment", "form_post"},
				"grant_types_supported":                 []string{"authorization_code", "client_credentials", "refresh_token", "password"},
				"code_challenge_methods_supported":      []string{"plain", "S256"},
				"id_token_signing_alg_values_supported": []string{"RS256"},
				"token_endpoint_auth_methods_supported": []string{"client_secret_post"},
				"subject_types_supported":               []string{"pairwise"},
				"scopes_supported":                      []string{"openid", "profile", "email", "offline_access"},
				"claims_supported":                      []string{"aud", "exp", "groups", "iat", "iss", "name", "nonce", "oid", "preferred_username", "sub", "tid", "ver"},
				"request_uri_parameter_supported":       false,
			})
			return
		}

		// JWKS endpoint — publish the public key that verifies freshly
		// minted RS256 tokens, matching Azure AD's verifier contract.
		if r.Method == http.MethodGet && (strings.HasSuffix(path, "/discovery/v2.0/keys") || strings.HasSuffix(path, "/discovery/keys")) {
			jwk, err := azureSimJWK()
			if err != nil {
				sim.AzureError(w, "InternalServerError", err.Error(), http.StatusInternalServerError)
				return
			}
			sim.WriteJSON(w, http.StatusOK, map[string]any{"keys": []map[string]any{jwk}})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func azureAuthBaseURL(r *http.Request) string {
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

func handleAzureAuthorize(w http.ResponseWriter, r *http.Request, path string) {
	tenantID := extractTenantFromPath(path)
	q := r.URL.Query()
	clientID := strings.TrimSpace(q.Get("client_id"))
	redirectURI := strings.TrimSpace(q.Get("redirect_uri"))
	responseType := strings.TrimSpace(q.Get("response_type"))
	scope := strings.TrimSpace(q.Get("scope"))
	state := q.Get("state")

	if clientID == "" {
		azureOAuthError(w, "invalid_request", "client_id is required", http.StatusBadRequest)
		return
	}
	if redirectURI == "" {
		azureOAuthError(w, "invalid_request", "redirect_uri is required", http.StatusBadRequest)
		return
	}
	if responseType != "code" {
		redirectAzureAuthError(w, redirectURI, q.Get("response_mode"), state, "unsupported_response_type", "response_type must be code")
		return
	}
	if scope == "" {
		redirectAzureAuthError(w, redirectURI, q.Get("response_mode"), state, "invalid_request", "scope is required")
		return
	}

	codeChallenge := strings.TrimSpace(q.Get("code_challenge"))
	codeChallengeMethod := strings.TrimSpace(q.Get("code_challenge_method"))
	if codeChallengeMethod != "" && codeChallenge == "" {
		redirectAzureAuthError(w, redirectURI, q.Get("response_mode"), state, "invalid_request", "code_challenge is required when code_challenge_method is set")
		return
	}
	if codeChallengeMethod == "" && codeChallenge != "" {
		codeChallengeMethod = "plain"
	}
	if codeChallengeMethod != "" && codeChallengeMethod != "plain" && codeChallengeMethod != "S256" {
		redirectAzureAuthError(w, redirectURI, q.Get("response_mode"), state, "invalid_request", "code_challenge_method must be plain or S256")
		return
	}

	code, err := newAzureAuthorizationCode()
	if err != nil {
		sim.AzureError(w, "InternalServerError", err.Error(), http.StatusInternalServerError)
		return
	}
	azureAuthCodeMu.Lock()
	azureAuthCodeStore[code] = azureAuthCode{
		TenantID:            tenantID,
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		Scope:               scope,
		Nonce:               q.Get("nonce"),
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		ExpiresAt:           time.Now().Add(azureAuthCodeTTL),
	}
	azureAuthCodeMu.Unlock()

	values := url.Values{"code": {code}}
	if state != "" {
		values.Set("state", state)
	}
	redirectAzureAuthorizeResponse(w, redirectURI, q.Get("response_mode"), values)
}

func newAzureAuthorizationCode() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate authorization code: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func extractTenantFromPath(path string) string {
	// Path is like /{tenantId}/v2.0/.well-known/openid-configuration
	parts := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return "unknown"
}

func handleAzureToken(w http.ResponseWriter, r *http.Request, path string) {
	tenantId := extractTenantFromPath(path)
	if err := r.ParseForm(); err != nil {
		azureOAuthError(w, "invalid_request", fmt.Sprintf("parse Azure token request form: %v", err), http.StatusBadRequest)
		return
	}
	if r.Form.Get("grant_type") == "authorization_code" {
		handleAzureAuthorizationCodeToken(w, r, tenantId)
		return
	}
	if r.Form.Get("grant_type") == "refresh_token" {
		handleAzureRefreshToken(w, r, tenantId)
		return
	}
	if r.Form.Get("grant_type") == "password" {
		handleAzureROPC(w, r, tenantId)
		return
	}
	if grantType := r.Form.Get("grant_type"); grantType != "" && grantType != "client_credentials" {
		azureOAuthError(w, "unsupported_grant_type", "grant_type is unsupported", http.StatusBadRequest)
		return
	}

	audience, err := azureTokenAudienceFromRequest(r)
	if err != nil {
		sim.AzureError(w, "InvalidRequest", err.Error(), http.StatusBadRequest)
		return
	}
	now := time.Now()
	token, err := mintAzureSimJWT(tenantId, audience, now, now.Add(1*time.Hour))
	if err != nil {
		sim.AzureError(w, "InternalServerError", err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"access_token":   token,
		"token_type":     "Bearer",
		"expires_in":     3600,
		"ext_expires_in": 3600,
	})
}

func handleAzureAuthorizationCodeToken(w http.ResponseWriter, r *http.Request, tenantID string) {
	code := strings.TrimSpace(r.Form.Get("code"))
	clientID := strings.TrimSpace(r.Form.Get("client_id"))
	redirectURI := strings.TrimSpace(r.Form.Get("redirect_uri"))
	if code == "" {
		azureOAuthError(w, "invalid_request", "code is required", http.StatusBadRequest)
		return
	}
	if clientID == "" {
		azureOAuthError(w, "invalid_request", "client_id is required", http.StatusBadRequest)
		return
	}
	if redirectURI == "" {
		azureOAuthError(w, "invalid_request", "redirect_uri is required", http.StatusBadRequest)
		return
	}

	authCode, ok := consumeAzureAuthorizationCode(code)
	if !ok {
		azureOAuthError(w, "invalid_grant", "authorization code is invalid or expired", http.StatusBadRequest)
		return
	}
	if authCode.TenantID != tenantID || authCode.ClientID != clientID || authCode.RedirectURI != redirectURI {
		azureOAuthError(w, "invalid_grant", "authorization code was issued for a different client or redirect URI", http.StatusBadRequest)
		return
	}
	if err := validateAzurePKCE(authCode, r.Form.Get("code_verifier")); err != nil {
		azureOAuthError(w, "invalid_grant", err.Error(), http.StatusBadRequest)
		return
	}

	scope := strings.TrimSpace(r.Form.Get("scope"))
	if scope == "" {
		scope = authCode.Scope
	}
	now := time.Now()
	accessToken, err := mintAzureSimJWT(tenantID, azureAudienceFromScope(scope), now, now.Add(time.Hour))
	if err != nil {
		sim.AzureError(w, "InternalServerError", err.Error(), http.StatusInternalServerError)
		return
	}
	body := map[string]any{
		"access_token":   accessToken,
		"token_type":     "Bearer",
		"expires_in":     3600,
		"ext_expires_in": 3600,
		"scope":          scope,
	}
	if azureScopeIncludes(scope, "openid") {
		idToken, err := mintAzureSimIDToken(tenantID, clientID, authCode.Nonce, scope, now, now.Add(time.Hour))
		if err != nil {
			sim.AzureError(w, "InternalServerError", err.Error(), http.StatusInternalServerError)
			return
		}
		body["id_token"] = idToken
	}
	if azureScopeIncludes(scope, "offline_access") {
		refreshToken, err := newAzureAuthorizationCode()
		if err != nil {
			sim.AzureError(w, "InternalServerError", err.Error(), http.StatusInternalServerError)
			return
		}
		azureRefreshTokenMu.Lock()
		azureRefreshTokenStore[refreshToken] = azureRefreshToken{
			TenantID: tenantID,
			ClientID: clientID,
			Scope:    scope,
			Nonce:    authCode.Nonce,
		}
		azureRefreshTokenMu.Unlock()
		body["refresh_token"] = refreshToken
	}
	sim.WriteJSON(w, http.StatusOK, body)
}

func handleAzureRefreshToken(w http.ResponseWriter, r *http.Request, tenantID string) {
	refreshToken := strings.TrimSpace(r.Form.Get("refresh_token"))
	clientID := strings.TrimSpace(r.Form.Get("client_id"))
	if refreshToken == "" {
		azureOAuthError(w, "invalid_request", "refresh_token is required", http.StatusBadRequest)
		return
	}
	if clientID == "" {
		azureOAuthError(w, "invalid_request", "client_id is required", http.StatusBadRequest)
		return
	}

	stored, ok := lookupAzureRefreshToken(refreshToken)
	if !ok || stored.TenantID != tenantID || stored.ClientID != clientID {
		azureOAuthError(w, "invalid_grant", "refresh token is invalid", http.StatusBadRequest)
		return
	}
	scope := strings.TrimSpace(r.Form.Get("scope"))
	if scope == "" {
		scope = stored.Scope
	}
	now := time.Now()
	accessToken, err := mintAzureSimJWT(tenantID, azureAudienceFromScope(scope), now, now.Add(time.Hour))
	if err != nil {
		sim.AzureError(w, "InternalServerError", err.Error(), http.StatusInternalServerError)
		return
	}
	body := map[string]any{
		"access_token":   accessToken,
		"token_type":     "Bearer",
		"expires_in":     3600,
		"ext_expires_in": 3600,
		"scope":          scope,
		"refresh_token":  refreshToken,
	}
	if azureScopeIncludes(scope, "openid") {
		idToken, err := mintAzureSimIDToken(tenantID, clientID, stored.Nonce, scope, now, now.Add(time.Hour))
		if err != nil {
			sim.AzureError(w, "InternalServerError", err.Error(), http.StatusInternalServerError)
			return
		}
		body["id_token"] = idToken
	}
	sim.WriteJSON(w, http.StatusOK, body)
}

func lookupAzureRefreshToken(refreshToken string) (azureRefreshToken, bool) {
	azureRefreshTokenMu.Lock()
	defer azureRefreshTokenMu.Unlock()
	stored, ok := azureRefreshTokenStore[refreshToken]
	return stored, ok
}

func consumeAzureAuthorizationCode(code string) (azureAuthCode, bool) {
	azureAuthCodeMu.Lock()
	defer azureAuthCodeMu.Unlock()
	authCode, ok := azureAuthCodeStore[code]
	if !ok {
		return azureAuthCode{}, false
	}
	delete(azureAuthCodeStore, code)
	if time.Now().After(authCode.ExpiresAt) {
		return azureAuthCode{}, false
	}
	return authCode, true
}

func validateAzurePKCE(authCode azureAuthCode, verifier string) error {
	if authCode.CodeChallenge == "" {
		return nil
	}
	if verifier == "" {
		return fmt.Errorf("code_verifier is required")
	}
	switch authCode.CodeChallengeMethod {
	case "", "plain":
		if verifier != authCode.CodeChallenge {
			return fmt.Errorf("code_verifier does not match code_challenge")
		}
	case "S256":
		digest := sha256.Sum256([]byte(verifier))
		if base64.RawURLEncoding.EncodeToString(digest[:]) != authCode.CodeChallenge {
			return fmt.Errorf("code_verifier does not match code_challenge")
		}
	default:
		return fmt.Errorf("code_challenge_method is unsupported")
	}
	return nil
}

func azureTokenAudienceFromRequest(r *http.Request) (string, error) {
	if r.Form == nil {
		if err := r.ParseForm(); err != nil {
			return "", fmt.Errorf("parse Azure token request form: %w", err)
		}
	}
	return azureTokenAudienceFromForm(r.Form), nil
}

func azureTokenAudienceFromForm(form url.Values) string {
	if scope := strings.TrimSpace(form.Get("scope")); scope != "" {
		return azureAudienceFromScope(scope)
	}
	if resource := strings.TrimSpace(form.Get("resource")); resource != "" {
		return resource
	}
	return defaultAzureTokenAudience
}

func azureAudienceFromScope(scope string) string {
	fields := strings.Fields(scope)
	for _, field := range fields {
		if azureScopeIsOIDC(field) {
			continue
		}
		audience := strings.TrimSuffix(field, "/.default")
		if idx := strings.LastIndex(audience, "/"); idx > len("https://") {
			audience = audience[:idx]
		}
		if override, ok := azureScopeAudienceOverrides[audience]; ok {
			return override
		}
		return audience
	}
	return defaultAzureTokenAudience
}

func azureScopeIncludes(scope, want string) bool {
	for _, field := range strings.Fields(scope) {
		if field == want {
			return true
		}
	}
	return false
}

func azureScopeIsOIDC(scope string) bool {
	switch scope {
	case "openid", "profile", "email", "offline_access":
		return true
	default:
		return false
	}
}

// mintAzureSimJWTForUser produces a real-shape Azure AD access token JWT for
// a specific user. mintAzureSimJWT uses the current sim-active user.
func mintAzureSimJWTForUser(u EntraUser, tenantId, audience string, issuedAt, expiresAt time.Time) (string, error) {
	return mintAzureSimSignedJWT(map[string]any{
		"tid":   tenantId,
		"oid":   u.OID,
		"sub":   u.Sub,
		"aud":   audience,
		"iss":   fmt.Sprintf("https://sts.windows.net/%s/", tenantId),
		"iat":   issuedAt.Unix(),
		"exp":   expiresAt.Unix(),
		"nbf":   issuedAt.Unix(),
		"ver":   "1.0",
		"appid": "sockerless-sim",
	})
}

func mintAzureSimJWT(tenantId, audience string, issuedAt, expiresAt time.Time) (string, error) {
	return mintAzureSimJWTForUser(getEntraSimActiveUser(), tenantId, audience, issuedAt, expiresAt)
}

// mintAzureSimIDTokenForUser produces a real-shape Azure AD id_token for a
// specific user. Groups are populated from both the inline sim-seed Groups
// field and the standard membership store. mintAzureSimIDToken uses the
// current sim-active user.
func mintAzureSimIDTokenForUser(u EntraUser, tenantID, clientID, nonce, scope string, issuedAt, expiresAt time.Time) (string, error) {
	email := u.Email
	if email == "" {
		email = u.PreferredUsername
	}
	claims := map[string]any{
		"tid":                tenantID,
		"oid":                u.OID,
		"sub":                u.Sub,
		"aud":                clientID,
		"iss":                fmt.Sprintf("https://sts.windows.net/%s/", tenantID),
		"iat":                issuedAt.Unix(),
		"exp":                expiresAt.Unix(),
		"nbf":                issuedAt.Unix(),
		"ver":                "2.0",
		"name":               u.Name,
		"preferred_username": u.PreferredUsername,
	}

	// Collect group IDs from both provisioning paths.
	groupIDSet := map[string]bool{}
	for _, g := range u.Groups {
		groupIDSet[g.ID] = true
	}
	memberships := entraGroupMembershipStore.Filter(func(m entraGroupMembership) bool {
		return m.UserID == u.OID
	})
	for _, m := range memberships {
		groupIDSet[m.GroupID] = true
	}
	if len(groupIDSet) > 0 {
		groupIDs := make([]string, 0, len(groupIDSet))
		for id := range groupIDSet {
			groupIDs = append(groupIDs, id)
		}
		claims["groups"] = groupIDs
	}

	if nonce != "" {
		claims["nonce"] = nonce
	}
	if azureScopeIncludes(scope, "email") {
		claims["email"] = email
	}
	return mintAzureSimSignedJWT(claims)
}

func mintAzureSimIDToken(tenantID, clientID, nonce, scope string, issuedAt, expiresAt time.Time) (string, error) {
	return mintAzureSimIDTokenForUser(getEntraSimActiveUser(), tenantID, clientID, nonce, scope, issuedAt, expiresAt)
}

// handleAzureROPC implements the Resource Owner Password Credentials grant
// (grant_type=password). Real Entra supports this for non-interactive test
// flows where a specific user's id_token is needed without a browser.
// The sim looks up the user by userPrincipalName (the username field) and
// mints tokens carrying that user's identity and group memberships.
func handleAzureROPC(w http.ResponseWriter, r *http.Request, tenantID string) {
	username := strings.TrimSpace(r.Form.Get("username"))
	clientID := strings.TrimSpace(r.Form.Get("client_id"))
	scope := strings.TrimSpace(r.Form.Get("scope"))
	if username == "" {
		azureOAuthError(w, "invalid_request", "username is required for grant_type=password", http.StatusBadRequest)
		return
	}

	users := entraUsersStore.Filter(func(u EntraUser) bool {
		return u.PreferredUsername == username
	})
	if len(users) == 0 {
		azureOAuthError(w, "invalid_grant", "user not found: "+username, http.StatusBadRequest)
		return
	}
	u := users[0]

	if scope == "" {
		scope = "openid profile"
	}
	now := time.Now()
	audience := azureAudienceFromScope(scope)
	accessToken, err := mintAzureSimJWTForUser(u, tenantID, audience, now, now.Add(time.Hour))
	if err != nil {
		sim.AzureError(w, "InternalServerError", err.Error(), http.StatusInternalServerError)
		return
	}
	body := map[string]any{
		"access_token":   accessToken,
		"token_type":     "Bearer",
		"expires_in":     3600,
		"ext_expires_in": 3600,
		"scope":          scope,
	}
	if azureScopeIncludes(scope, "openid") {
		idToken, err := mintAzureSimIDTokenForUser(u, tenantID, clientID, "", scope, now, now.Add(time.Hour))
		if err != nil {
			sim.AzureError(w, "InternalServerError", err.Error(), http.StatusInternalServerError)
			return
		}
		body["id_token"] = idToken
	}
	sim.WriteJSON(w, http.StatusOK, body)
}

func mintAzureSimSignedJWT(claims map[string]any) (string, error) {
	headerJSON, _ := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
		"kid": "sockerless-sim-key-1",
	})
	payloadJSON, _ := json.Marshal(claims)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := headerB64 + "." + payloadB64
	digest := sha256.Sum256([]byte(signingInput))
	key, err := azureSimSigningKey()
	if err != nil {
		return "", fmt.Errorf("generate Azure simulator signing key: %w", err)
	}
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign Azure simulator JWT: %w", err)
	}
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	return signingInput + "." + sigB64, nil
}

func redirectAzureAuthError(w http.ResponseWriter, redirectURI, responseMode, state, code, description string) {
	values := url.Values{
		"error":             {code},
		"error_description": {description},
	}
	if state != "" {
		values.Set("state", state)
	}
	redirectAzureAuthorizeResponse(w, redirectURI, responseMode, values)
}

func redirectAzureAuthorizeResponse(w http.ResponseWriter, redirectURI, responseMode string, values url.Values) {
	switch responseMode {
	case "", "query":
		u, err := url.Parse(redirectURI)
		if err != nil {
			azureOAuthError(w, "invalid_request", "redirect_uri is invalid", http.StatusBadRequest)
			return
		}
		q := u.Query()
		for k, vals := range values {
			for _, v := range vals {
				q.Add(k, v)
			}
		}
		u.RawQuery = q.Encode()
		w.Header().Set("Location", u.String())
		w.WriteHeader(http.StatusFound)
	case "fragment":
		u, err := url.Parse(redirectURI)
		if err != nil {
			azureOAuthError(w, "invalid_request", "redirect_uri is invalid", http.StatusBadRequest)
			return
		}
		fragment := url.Values{}
		if u.Fragment != "" {
			parsed, err := url.ParseQuery(u.Fragment)
			if err == nil {
				fragment = parsed
			}
		}
		for k, vals := range values {
			for _, v := range vals {
				fragment.Add(k, v)
			}
		}
		u.Fragment = fragment.Encode()
		w.Header().Set("Location", u.String())
		w.WriteHeader(http.StatusFound)
	case "form_post":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<!doctype html><html><body><form method="post" action="%s">`, html.EscapeString(redirectURI))
		for k, vals := range values {
			for _, v := range vals {
				fmt.Fprintf(w, `<input type="hidden" name="%s" value="%s">`, html.EscapeString(k), html.EscapeString(v))
			}
		}
		fmt.Fprint(w, `<noscript><button type="submit">Continue</button></noscript><script>document.forms[0].submit()</script></form></body></html>`)
	default:
		azureOAuthError(w, "invalid_request", "response_mode is unsupported", http.StatusBadRequest)
	}
}

func azureOAuthError(w http.ResponseWriter, code, description string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":             code,
		"error_description": description,
	})
}

func azureSimJWK() (map[string]any, error) {
	key, err := azureSimSigningKey()
	if err != nil {
		return nil, fmt.Errorf("generate Azure simulator signing key: %w", err)
	}
	pub := key.PublicKey
	return map[string]any{
		"kid": "sockerless-sim-key-1",
		"kty": "RSA",
		"alg": "RS256",
		"use": "sig",
		"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}, nil
}
