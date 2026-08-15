package azurecommon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	core "github.com/sockerless/backend-core"
)

// Azure Container Registry data-plane authentication.
//
// A registry's Docker Registry HTTP API v2 surface does not accept a Microsoft
// Entra token. Entra tokens are exchanged for the registry's own tokens at the
// registry's token service, and only the token service's output is a valid
// Bearer credential on /v2/:
//
//  1. Acquire a Microsoft Entra access token for the Azure Container Registry
//     service audience, "https://containerregistry.azure.net" — the audience of
//     the *service*, not of an individual registry host.
//  2. POST it to https://<loginServer>/oauth2/exchange with
//     grant_type=access_token, service=<loginServer> and access_token=<entra
//     token> (plus tenant, where the identity's tenant has to be named), and
//     read the ACR refresh token out of the response's `refresh_token`.
//  3. POST that refresh token to https://<loginServer>/oauth2/token with
//     grant_type=refresh_token, service=<loginServer> and one or more
//     scope=<type>:<name>:<actions> parameters, and read the scoped ACR access
//     token out of the response's `access_token`.
//  4. Present that access token as `Authorization: Bearer <access token>` on
//     the /v2/ request.
//
// Both hops are the ones the Azure SDK for Go's azcontainerregistry
// authentication policy performs, and the scope grammar is the Docker Registry
// HTTP API v2 one the registry's own Bearer challenge names.

// acrEntraScope is the Microsoft Entra scope an Azure Container Registry
// data-plane token is acquired with. The audience is the container-registry
// *service*; a scope built from a registry's own host yields a token the
// registry's token service refuses.
const acrEntraScope = "https://containerregistry.azure.net/.default"

// acrTokenAPIVersion is the data-plane API version the registry's token
// service is called with, the one the Azure SDK for Go's azcontainerregistry
// authentication client sends on both /oauth2 routes.
const acrTokenAPIVersion = "2021-07-01"

// acrTokenRefreshMargin is how long before its expiry a cached token stops
// being reused, so an operation is never started with a credential that
// expires while it is in flight.
const acrTokenRefreshMargin = 5 * time.Minute

// ACRRegistryScope renders the access an operation needs as the Docker Registry
// HTTP API v2 token scope the ACR token service is asked for. An empty
// repository addresses the registry itself — its catalog — and a registry-level
// resource carries only the "*" action, even for a read.
func ACRRegistryScope(repository string, actions ...string) string {
	if repository == "" {
		return "registry:catalog:" + core.ActionAll
	}
	if len(actions) == 0 {
		actions = []string{core.ActionPull}
	}
	return "repository:" + repository + ":" + strings.Join(actions, ",")
}

// acrCachedToken is one token the registry's token service issued, with the
// expiry read out of it. A token whose expiry could not be read is never
// cached, so it is re-acquired for every operation rather than reused past a
// lifetime nothing established.
type acrCachedToken struct {
	value     string
	expiresAt time.Time
}

func (t acrCachedToken) usable() bool {
	return t.value != "" && time.Now().Add(acrTokenRefreshMargin).Before(t.expiresAt)
}

// acrTokenService mints Azure Container Registry access tokens through the
// registry's token service, caching each hop for as long as the token it
// issued says it is valid.
type acrTokenService struct {
	// credential is the Microsoft Entra credential the exchange authenticates
	// with. It is built on first use from the ambient Azure environment when
	// the owner supplied none.
	credential azcore.TokenCredential
	credOnce   sync.Once
	credErr    error

	client *http.Client

	mu      sync.Mutex
	refresh map[string]acrCachedToken // keyed by login server
	access  map[string]acrCachedToken // keyed by login server + "|" + scope
}

func newACRTokenService(credential azcore.TokenCredential) *acrTokenService {
	return &acrTokenService{
		credential: credential,
		client:     &http.Client{Timeout: 30 * time.Second},
		refresh:    map[string]acrCachedToken{},
		access:     map[string]acrCachedToken{},
	}
}

// entraCredential resolves the Microsoft Entra credential once. The ambient
// environment is what selects the identity (a managed identity's endpoint, a
// service principal's secret, a developer's signed-in session) — the same
// coordinates every other Azure client in this backend is configured by.
func (s *acrTokenService) entraCredential() (azcore.TokenCredential, error) {
	s.credOnce.Do(func() {
		if s.credential != nil {
			return
		}
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			s.credErr = fmt.Errorf("resolve the Microsoft Entra credential: %w", err)
			return
		}
		s.credential = cred
	})
	if s.credErr != nil {
		return nil, s.credErr
	}
	return s.credential, nil
}

// AccessToken returns the ACR access token that authorizes `scope` on the
// registry whose login server is `service`, reached over `baseURL`.
//
// `baseURL` is the network coordinate the token service is dialed at — the
// registry's own https://<loginServer> unless an endpoint coordinate relocates
// it — while `service` always names the registry, both in the form parameter
// the token service matches and in the Host header that routes the request to
// it.
func (s *acrTokenService) AccessToken(ctx context.Context, baseURL, service, scope string) (string, error) {
	key := service + "|" + scope
	s.mu.Lock()
	cached, ok := s.access[key]
	s.mu.Unlock()
	if ok && cached.usable() {
		return cached.value, nil
	}

	refreshToken, err := s.refreshToken(ctx, baseURL, service)
	if err != nil {
		return "", err
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"service":       {service},
		"refresh_token": {refreshToken},
		"scope":         {scope},
	}
	body, err := s.postForm(ctx, baseURL+"/oauth2/token", service, form)
	if err != nil {
		return "", fmt.Errorf("ACR token service (%s, scope %q): %w", service, scope, err)
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("ACR token service (%s): decode access token: %w", service, err)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("ACR token service (%s): response carried no access_token", service)
	}
	s.store(s.access, key, out.AccessToken)
	return out.AccessToken, nil
}

// refreshToken returns the ACR refresh token a Microsoft Entra identity
// exchanges for at the registry's token service. It is registry-wide, so one
// serves every scope that registry issues.
func (s *acrTokenService) refreshToken(ctx context.Context, baseURL, service string) (string, error) {
	s.mu.Lock()
	cached, ok := s.refresh[service]
	s.mu.Unlock()
	if ok && cached.usable() {
		return cached.value, nil
	}

	cred, err := s.entraCredential()
	if err != nil {
		return "", err
	}
	entra, err := cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{acrEntraScope}})
	if err != nil {
		return "", fmt.Errorf("acquire a Microsoft Entra token for %s: %w", acrEntraScope, err)
	}

	form := url.Values{
		"grant_type":   {"access_token"},
		"service":      {service},
		"access_token": {entra.Token},
	}
	// The tenant the identity belongs to, named when the environment says
	// which one it is. The token service needs it to resolve an identity whose
	// tenant its own directory lookup cannot infer.
	if tenant := strings.TrimSpace(os.Getenv("AZURE_TENANT_ID")); tenant != "" {
		form.Set("tenant", tenant)
	}
	body, err := s.postForm(ctx, baseURL+"/oauth2/exchange", service, form)
	if err != nil {
		return "", fmt.Errorf("ACR token exchange (%s): %w", service, err)
	}
	var out struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("ACR token exchange (%s): decode refresh token: %w", service, err)
	}
	if out.RefreshToken == "" {
		return "", fmt.Errorf("ACR token exchange (%s): response carried no refresh_token", service)
	}
	s.store(s.refresh, service, out.RefreshToken)
	return out.RefreshToken, nil
}

// postForm sends one form-encoded request to the registry's token service.
// `service` is set as the Host so the request routes to the registry it names
// even when baseURL relocates the network destination.
func (s *acrTokenService) postForm(ctx context.Context, endpoint, service string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		endpoint+"?api-version="+acrTokenAPIVersion, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	core.SetOCIHost(req, service)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

// store caches a token under `key` until the `exp` claim of the JWT the
// registry issued says it expires. The token service reports no lifetime
// alongside the token — its responses carry the token and nothing else — so the
// token itself is the only statement of when it stops working, which is what
// the Azure SDK for Go reads to decide when to renew. A token that carries no
// readable expiry is not cached, and is re-acquired for every operation.
func (s *acrTokenService) store(into map[string]acrCachedToken, key, token string) {
	expiresAt, ok := acrTokenExpiry(token)
	if !ok {
		return
	}
	s.mu.Lock()
	into[key] = acrCachedToken{value: token, expiresAt: expiresAt}
	s.mu.Unlock()
}

// Invalidate drops every token cached for a registry, so the next operation
// re-runs the exchange. A registry that refuses a token it previously issued
// is the authority on it having gone stale.
func (s *acrTokenService) Invalidate(service string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.refresh, service)
	for key := range s.access {
		if strings.HasPrefix(key, service+"|") {
			delete(s.access, key)
		}
	}
}

// acrTokenExpiry reads the `exp` claim out of a JWT the registry's token
// service issued. It reports false for a token that is not a JWT or carries no
// expiry, which is the caller's signal not to cache it.
func acrTokenExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}
