package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestPKCEChallengeRFC7636Vector pins the S256 derivation to the RFC 7636
// Appendix B example: the published verifier must produce the published
// challenge.
func TestPKCEChallengeRFC7636Vector(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := pkceChallenge(verifier); got != want {
		t.Fatalf("pkceChallenge(%q) = %q, want %q", verifier, got, want)
	}
}

// testProvider is a minimal but real OpenID Connect provider: it serves
// discovery, issues single-use authorization codes bound to the request's
// PKCE challenge, verifies the S256 code verifier on exchange exactly as
// RFC 7636 §4.6 requires, and signs ID tokens with RS256. The browser is the
// only absent party — tests play the user agent by following the authorize
// redirect to the loopback callback.
type testProvider struct {
	server *httptest.Server
	key    *rsa.PrivateKey

	mu    sync.Mutex
	codes map[string]testAuthCode

	// authorizeState optionally rewrites the state echoed back on the
	// redirect, to exercise the client's state check.
	authorizeState func(state string) string
	// authorizeError makes /authorize return an OAuth error redirect.
	authorizeError string
}

type testAuthCode struct {
	challenge   string
	redirectURI string
	clientID    string
	nonce       string
}

func newTestProvider(t *testing.T) *testProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate provider key: %v", err)
	}
	provider := &testProvider{key: key, codes: make(map[string]testAuthCode), authorizeState: func(s string) string { return s }}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 provider.server.URL,
			"authorization_endpoint": provider.server.URL + "/authorize",
			"token_endpoint":         provider.server.URL + "/token",
		})
	})
	mux.HandleFunc("GET /authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		redirectURI, err := url.Parse(q.Get("redirect_uri"))
		if err != nil || redirectURI.Hostname() != "127.0.0.1" {
			http.Error(w, "redirect_uri must target loopback", http.StatusBadRequest)
			return
		}
		out := url.Values{"state": {provider.authorizeState(q.Get("state"))}}
		if provider.authorizeError != "" {
			out.Set("error", provider.authorizeError)
			out.Set("error_description", "the resource owner denied the request")
		} else {
			if q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
				http.Error(w, "authorization request lacks code+S256 PKCE", http.StatusBadRequest)
				return
			}
			code := fmt.Sprintf("code-%d", time.Now().UnixNano())
			provider.mu.Lock()
			provider.codes[code] = testAuthCode{
				challenge:   q.Get("code_challenge"),
				redirectURI: q.Get("redirect_uri"),
				clientID:    q.Get("client_id"),
				nonce:       q.Get("nonce"),
			}
			provider.mu.Unlock()
			out.Set("code", code)
		}
		redirectURI.RawQuery = out.Encode()
		http.Redirect(w, r, redirectURI.String(), http.StatusFound)
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		writeOAuthError := func(code, description string) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": description})
		}
		if r.Form.Get("grant_type") != "authorization_code" {
			writeOAuthError("unsupported_grant_type", "only authorization_code is supported")
			return
		}
		provider.mu.Lock()
		grant, ok := provider.codes[r.Form.Get("code")]
		delete(provider.codes, r.Form.Get("code"))
		provider.mu.Unlock()
		if !ok {
			writeOAuthError("invalid_grant", "authorization code is unknown or already used")
			return
		}
		if grant.redirectURI != r.Form.Get("redirect_uri") || grant.clientID != r.Form.Get("client_id") {
			writeOAuthError("invalid_grant", "redirect_uri or client_id does not match the authorization request")
			return
		}
		digest := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(digest[:]) != grant.challenge {
			writeOAuthError("invalid_grant", "the PKCE code challenge did not match the code verifier")
			return
		}
		idToken, err := provider.signIDToken(map[string]any{
			"iss":                provider.server.URL,
			"sub":                "user-1",
			"aud":                grant.clientID,
			"nonce":              grant.nonce,
			"preferred_username": "tester",
			"email":              "tester@example.test",
			"iat":                time.Now().Unix(),
			"exp":                time.Now().Add(15 * time.Minute).Unix(),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id_token":     idToken,
			"access_token": "at-" + r.Form.Get("code"),
			"token_type":   "bearer",
			"expires_in":   900,
		})
	})

	provider.server = httptest.NewServer(mux)
	t.Cleanup(provider.server.Close)
	return provider
}

func (p *testProvider) signIDToken(claims map[string]any) (string, error) {
	encode := func(v any) string {
		raw, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	signingInput := encode(map[string]string{"alg": "RS256", "typ": "JWT"}) + "." + encode(claims)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// followAuthorize acts as the user agent: it fetches the authorization URL
// and follows redirects, including the final redirect to the flow's loopback
// callback listener.
func followAuthorize(t *testing.T) func(string) error {
	t.Helper()
	return func(authorizeURL string) error {
		resp, err := http.Get(authorizeURL)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return nil
	}
}

func TestBrowserLoginEndToEnd(t *testing.T) {
	provider := newTestProvider(t)
	idToken, err := browserLogin(loginFlowOptions{
		Issuer:   provider.server.URL,
		ClientID: "sockerless-cli",
		Timeout:  10 * time.Second,
		Browse:   followAuthorize(t),
		Printf:   t.Logf,
	})
	if err != nil {
		t.Fatalf("browserLogin: %v", err)
	}
	claims, err := idTokenClaims(idToken)
	if err != nil {
		t.Fatalf("idTokenClaims: %v", err)
	}
	if got := stringClaim(claims, "preferred_username"); got != "tester" {
		t.Errorf("preferred_username = %q, want %q", got, "tester")
	}
	if got := stringClaim(claims, "iss"); got != provider.server.URL {
		t.Errorf("iss = %q, want %q", got, provider.server.URL)
	}
	if got := stringClaim(claims, "nonce"); got == "" {
		t.Error("ID token carries no nonce")
	}
}

func TestBrowserLoginRejectsStateMismatch(t *testing.T) {
	provider := newTestProvider(t)
	provider.authorizeState = func(string) string { return "tampered-state-value" }
	_, err := browserLogin(loginFlowOptions{
		Issuer:   provider.server.URL,
		ClientID: "sockerless-cli",
		Timeout:  10 * time.Second,
		Browse:   followAuthorize(t),
		Printf:   t.Logf,
	})
	if err == nil || !strings.Contains(err.Error(), "state") {
		t.Fatalf("browserLogin error = %v, want a state-mismatch error", err)
	}
}

func TestBrowserLoginReportsProviderError(t *testing.T) {
	provider := newTestProvider(t)
	provider.authorizeError = "access_denied"
	_, err := browserLogin(loginFlowOptions{
		Issuer:   provider.server.URL,
		ClientID: "sockerless-cli",
		Timeout:  10 * time.Second,
		Browse:   followAuthorize(t),
		Printf:   t.Logf,
	})
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("browserLogin error = %v, want access_denied", err)
	}
}

func TestIDTokenClaimsRejectsNonJWT(t *testing.T) {
	if _, err := idTokenClaims("not-a-jwt"); err == nil {
		t.Fatal("idTokenClaims accepted a non-JWT value")
	}
}
