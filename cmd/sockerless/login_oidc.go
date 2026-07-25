package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// oidcProviderMetadata is the subset of the OpenID Connect discovery document
// the login flow needs.
type oidcProviderMetadata struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

// oidcHTTPClient bounds every request of the login flow so a wrong issuer
// coordinate fails instead of hanging the terminal.
var oidcHTTPClient = &http.Client{Timeout: 30 * time.Second}

// discoverOIDC fetches the issuer's OpenID Connect discovery document.
func discoverOIDC(issuer string) (oidcProviderMetadata, error) {
	discoveryURL := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"
	resp, err := oidcHTTPClient.Get(discoveryURL)
	if err != nil {
		return oidcProviderMetadata{}, fmt.Errorf("discover %s: %w", discoveryURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return oidcProviderMetadata{}, fmt.Errorf("discover %s: HTTP %d: %s", discoveryURL, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var metadata oidcProviderMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return oidcProviderMetadata{}, fmt.Errorf("discover %s: %w", discoveryURL, err)
	}
	if metadata.AuthorizationEndpoint == "" || metadata.TokenEndpoint == "" {
		return oidcProviderMetadata{}, fmt.Errorf("discover %s: document lacks authorization_endpoint or token_endpoint", discoveryURL)
	}
	return metadata, nil
}

// pkceChallenge derives the RFC 7636 S256 code challenge from a verifier.
func pkceChallenge(verifier string) string {
	digest := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

// randomURLToken returns a URL-safe random string from n bytes of entropy.
func randomURLToken(n int) (string, error) {
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// tokenResponse is the OAuth 2.0 token endpoint response the flow consumes.
type tokenResponse struct {
	IDToken          string `json:"id_token"`
	AccessToken      string `json:"access_token"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// loginFlowOptions configures one RFC 8252 authorization-code + PKCE sign-in
// through the system browser with a loopback redirect.
type loginFlowOptions struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	Timeout      time.Duration
	// Browse presents the authorization URL to the user. The default opens
	// the system browser; --no-browser only prints the URL; tests follow it
	// with an HTTP client acting as the user agent.
	Browse func(authorizeURL string) error
	// Printf reports flow progress (the authorization URL, completion).
	Printf func(format string, args ...any)
}

// browserLogin runs the OpenID Connect authorization-code flow with PKCE
// (RFC 7636) and a loopback redirect (RFC 8252 §7.3): it starts an ephemeral
// listener on 127.0.0.1, sends the user to the issuer's authorization
// endpoint, receives the code on the loopback callback, and exchanges it at
// the token endpoint. It returns the raw ID token.
func browserLogin(opts loginFlowOptions) (string, error) {
	if opts.Printf == nil {
		opts.Printf = func(string, ...any) {}
	}
	metadata, err := discoverOIDC(opts.Issuer)
	if err != nil {
		return "", err
	}

	verifier, err := randomURLToken(32)
	if err != nil {
		return "", err
	}
	state, err := randomURLToken(32)
	if err != nil {
		return "", err
	}
	nonce, err := randomURLToken(32)
	if err != nil {
		return "", err
	}

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("start loopback listener: %w", err)
	}
	defer func() { _ = listener.Close() }()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return "", fmt.Errorf("loopback listener has unexpected address type %T", listener.Addr())
	}
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", addr.Port)

	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {opts.ClientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {"openid"},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	authorizeURL := metadata.AuthorizationEndpoint + "?" + query.Encode()

	type callbackResult struct {
		code string
		err  error
	}
	results := make(chan callbackResult, 1)
	deliver := func(result callbackResult) {
		select {
		case results <- result:
		default:
		}
	}

	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		params := r.URL.Query()
		if params.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			deliver(callbackResult{err: errors.New("authorization response state does not match the request")})
			return
		}
		if errCode := params.Get("error"); errCode != "" {
			description := params.Get("error_description")
			http.Error(w, fmt.Sprintf("sign-in failed: %s: %s", errCode, description), http.StatusBadRequest)
			deliver(callbackResult{err: fmt.Errorf("authorization failed: %s: %s", errCode, description)})
			return
		}
		code := params.Get("code")
		if code == "" {
			http.Error(w, "missing authorization code", http.StatusBadRequest)
			deliver(callbackResult{err: errors.New("authorization response carried no code")})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<!doctype html><title>Signed in</title><body style=\"font-family:system-ui;margin:4rem\"><h1>Signed in</h1><p>You can close this window and return to the terminal.</p></body>")
		deliver(callbackResult{code: code})
	})}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			deliver(callbackResult{err: fmt.Errorf("loopback callback server: %w", serveErr)})
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	opts.Printf("Sign in to %s in your browser:\n\n  %s\n\n", opts.Issuer, authorizeURL)
	if opts.Browse != nil {
		if err := opts.Browse(authorizeURL); err != nil {
			return "", err
		}
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	var code string
	select {
	case result := <-results:
		if result.err != nil {
			return "", result.err
		}
		code = result.code
	case <-time.After(timeout):
		return "", fmt.Errorf("timed out after %s waiting for the browser sign-in", timeout)
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {opts.ClientID},
		"code_verifier": {verifier},
	}
	if opts.ClientSecret != "" {
		form.Set("client_secret", opts.ClientSecret)
	}
	resp, err := oidcHTTPClient.PostForm(metadata.TokenEndpoint, form)
	if err != nil {
		return "", fmt.Errorf("exchange authorization code: %w", err)
	}
	defer resp.Body.Close()
	var tokens tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return "", fmt.Errorf("exchange authorization code: parse response: %w", err)
	}
	if tokens.Error != "" {
		return "", fmt.Errorf("exchange authorization code: %s: %s", tokens.Error, tokens.ErrorDescription)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("exchange authorization code: HTTP %d", resp.StatusCode)
	}
	if tokens.IDToken == "" {
		return "", errors.New("token endpoint returned no ID token")
	}
	return tokens.IDToken, nil
}

// idTokenClaims decodes a JWT's claims WITHOUT verifying its signature — for
// terminal display only. The relying parties are the clouds: every federation
// endpoint (AssumeRoleWithWebIdentity, the Google Cloud Security Token
// Service, Microsoft Entra) verifies the token against Shauth's JSON Web Key
// Set itself before trusting it.
func idTokenClaims(rawToken string) (map[string]any, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return nil, errors.New("ID token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode ID token claims: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("parse ID token claims: %w", err)
	}
	return claims, nil
}

// stringClaim returns a string claim or "" when absent.
func stringClaim(claims map[string]any, name string) string {
	value, _ := claims[name].(string)
	return value
}

// openBrowser opens the URL in the platform's default browser.
func openBrowser(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w (use --no-browser and open the printed URL yourself)", err)
	}
	return nil
}
