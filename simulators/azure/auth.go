package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

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

		// JWKS endpoint
		if r.Method == http.MethodGet && (strings.HasSuffix(path, "/discovery/v2.0/keys") || strings.HasSuffix(path, "/discovery/keys")) {
			sim.WriteJSON(w, http.StatusOK, map[string]any{"keys": []any{}})
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

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(
		`{"tid":"%s","oid":"test-oid","sub":"test-sub","aud":"https://management.azure.com/","iss":"https://sts.windows.net/%s/","iat":%d,"exp":%d,"nbf":%d}`,
		tenantId, tenantId,
		time.Now().Unix(),
		time.Now().Add(1*time.Hour).Unix(),
		time.Now().Unix(),
	)))
	token := header + "." + payload + "."

	w.Header().Set("Content-Type", "application/json")
	sim.WriteJSON(w, http.StatusOK, map[string]any{
		"access_token":   token,
		"token_type":     "Bearer",
		"expires_in":     3600,
		"ext_expires_in": 3600,
	})
}
