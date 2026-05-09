package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// azureSimSignKey is the per-process HS256 key used to sign Azure AD
// tokens the sim mints. Generated lazily on first call so tests that
// don't invoke handleMockToken aren't affected. Real Azure AD signs
// with rotated RSA keys; HS256 is sufficient here because the sim
// doesn't verify inbound tokens — the only requirement is that SDKs
// that parse the token (e.g. azure-identity's confidential-client
// path) accept the structure.
var (
	azureSimSignKeyOnce sync.Once
	azureSimSignKeyVal  []byte
)

func azureSimSignKey() []byte {
	azureSimSignKeyOnce.Do(func() {
		azureSimSignKeyVal = []byte(fmt.Sprintf("sockerless-sim-azure-%d", time.Now().UnixNano()))
	})
	return azureSimSignKeyVal
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

// AzureAuthMiddleware intercepts OAuth2 and OpenID discovery requests needed
// by the Azure SDK for authentication. This is implemented as middleware
// rather than registered routes to avoid conflicts with ACR's /v2/{path...}.
func AzureAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Token endpoint: POST /{tenantId}/oauth2/v2.0/token
		if r.Method == http.MethodPost && strings.Contains(path, "/oauth2/v2.0/token") {
			handleMockToken(w, r, path)
			return
		}
		// Token endpoint v1: POST /{tenantId}/oauth2/token
		if r.Method == http.MethodPost && strings.Contains(path, "/oauth2/token") {
			handleMockToken(w, r, path)
			return
		}

		// OpenID discovery endpoints
		if r.Method == http.MethodGet && strings.HasSuffix(path, "/.well-known/openid-configuration") {
			tenantId := extractTenantFromPath(path)
			sim.WriteJSON(w, http.StatusOK, map[string]any{
				"issuer":                 fmt.Sprintf("https://sts.windows.net/%s/", tenantId),
				"authorization_endpoint": fmt.Sprintf("/%s/oauth2/v2.0/authorize", tenantId),
				"token_endpoint":         fmt.Sprintf("/%s/oauth2/v2.0/token", tenantId),
				"jwks_uri":               fmt.Sprintf("/%s/discovery/v2.0/keys", tenantId),
			})
			return
		}

		// JWKS endpoint — publish the kid the sim stamps into freshly
		// minted tokens. Body is intentionally minimal: real Azure
		// publishes RS256 public keys, the sim signs with HS256, so
		// the kid is the only field clients can usefully cross-check.
		if r.Method == http.MethodGet && (strings.HasSuffix(path, "/discovery/v2.0/keys") || strings.HasSuffix(path, "/discovery/keys")) {
			sim.WriteJSON(w, http.StatusOK, map[string]any{
				"keys": []map[string]any{{
					"kid": "sockerless-sim-key-1",
					"kty": "oct",
					"alg": "HS256",
					"use": "sig",
				}},
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func extractTenantFromPath(path string) string {
	// Path is like /{tenantId}/v2.0/.well-known/openid-configuration
	parts := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return "unknown"
}

func handleMockToken(w http.ResponseWriter, r *http.Request, path string) {
	tenantId := extractTenantFromPath(path)
	now := time.Now()
	token := mintAzureSimJWT(tenantId, now, now.Add(1*time.Hour))

	w.Header().Set("Content-Type", "application/json")
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"access_token":   token,
		"token_type":     "Bearer",
		"expires_in":     3600,
		"ext_expires_in": 3600,
	})
}

// mintAzureSimJWT produces a real-shape Azure AD access token JWT
// (`header.payload.signature`) signed with HS256 against the sim's
// per-process key. The claims set matches what azure-identity / the
// Azure SDK round-trip on token introspection (tid, oid, sub, aud,
// iss, iat, exp, nbf, ver, appid).
func mintAzureSimJWT(tenantId string, issuedAt, expiresAt time.Time) string {
	headerJSON, _ := json.Marshal(map[string]string{
		"alg": "HS256",
		"typ": "JWT",
		"kid": "sockerless-sim-key-1",
	})
	payloadJSON, _ := json.Marshal(map[string]any{
		"tid":   tenantId,
		"oid":   "test-oid",
		"sub":   "test-sub",
		"aud":   "https://management.azure.com/",
		"iss":   fmt.Sprintf("https://sts.windows.net/%s/", tenantId),
		"iat":   issuedAt.Unix(),
		"exp":   expiresAt.Unix(),
		"nbf":   issuedAt.Unix(),
		"ver":   "1.0",
		"appid": "sockerless-sim",
	})
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := headerB64 + "." + payloadB64
	mac := hmac.New(sha256.New, azureSimSignKey())
	mac.Write([]byte(signingInput))
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sigB64
}
