package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
)

const defaultAzureTokenAudience = "https://management.azure.com/"

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
			// external: `issuer` is a JWT-spec claim convention,
			// not a routable URL. azidentity verifies token
			// signatures against the JWKS at `jwks_uri` below
			// (which IS sim-hosted) and compares `iss` against
			// the expected value rather than dereferencing it.
			sim.WriteJSON(w, http.StatusOK, map[string]any{
				"issuer":                 fmt.Sprintf("https://sts.windows.net/%s/", tenantId),
				"authorization_endpoint": fmt.Sprintf("/%s/oauth2/v2.0/authorize", tenantId),
				"token_endpoint":         fmt.Sprintf("/%s/oauth2/v2.0/token", tenantId),
				"jwks_uri":               fmt.Sprintf("/%s/discovery/v2.0/keys", tenantId),
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

func azureTokenAudienceFromRequest(r *http.Request) (string, error) {
	if err := r.ParseForm(); err != nil {
		return "", fmt.Errorf("parse Azure token request form: %w", err)
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
	if len(fields) == 0 {
		return defaultAzureTokenAudience
	}
	audience := strings.TrimSuffix(fields[0], "/.default")
	if override, ok := azureScopeAudienceOverrides[audience]; ok {
		return override
	}
	return audience
}

// mintAzureSimJWT produces a real-shape Azure AD access token JWT
// (`header.payload.signature`) signed with RS256 against the sim's
// per-process private key. The claims set matches what azure-identity / the
// Azure SDK round-trip on token introspection (tid, oid, sub, aud,
// iss, iat, exp, nbf, ver, appid).
func mintAzureSimJWT(tenantId, audience string, issuedAt, expiresAt time.Time) (string, error) {
	headerJSON, _ := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
		"kid": "sockerless-sim-key-1",
	})
	// external: `iss` claim — same JWT-spec convention as the
	// openid-configuration `issuer` above; not dereferenced.
	payloadJSON, _ := json.Marshal(map[string]any{
		"tid":   tenantId,
		"oid":   "test-oid",
		"sub":   "test-sub",
		"aud":   audience,
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
