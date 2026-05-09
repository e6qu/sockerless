// azure-ad access driver shared by every Azure-product backend.
//
// Wraps an azcore.TokenCredential (DefaultAzureCredential picks up the
// MSI inside Azure, falls back through the standard credential chain
// outside it). Each AuthenticatedClient call returns an http.Client
// whose Transport mints a fresh Bearer token per request — scope is
// derived from the audience parameter (`<audience>/.default`).
//
// Per-backend wiring: backend startup constructs with the per-backend
// principal label (the workload's MSI client ID or service principal
// app ID) so /v1/info reports the right WorkloadPrincipal value. The
// principal label is informational only — the actual signing identity
// comes from the credential the backend supplies.

package azurecommon

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/sockerless/api"
)

// AzureADAccess implements core.AccessDriver via Azure AD bearer tokens.
type AzureADAccess struct {
	// Credential mints OAuth2 access tokens. Pass an azidentity.*
	// credential constructed at backend startup (typically
	// DefaultAzureCredential on the Cred field of AzureClients).
	Credential azcore.TokenCredential
	// Principal is the workload's identity label reported by
	// WorkloadPrincipal — the MSI client ID or service principal
	// AppId the deployed workload runs as.
	Principal string
}

// NewAzureADAccess constructs the driver with a credential + principal label.
func NewAzureADAccess(cred azcore.TokenCredential, principal string) *AzureADAccess {
	return &AzureADAccess{Credential: cred, Principal: principal}
}

func (a *AzureADAccess) Mechanism() api.AccessMechanism { return api.AccessMechanismAzureAD }

func (a *AzureADAccess) WorkloadPrincipal() string { return a.Principal }

// AuthenticatedClient returns an http.Client whose Transport attaches
// `Authorization: Bearer <token>` where the token's scope is derived
// from `audience` (`<audience>/.default`). The token is minted per
// request via the underlying credential — azidentity caches tokens
// internally so the cost is amortised across calls within the
// credential's TTL.
func (a *AzureADAccess) AuthenticatedClient(ctx context.Context, audience string) (*http.Client, error) {
	if a.Credential == nil {
		return nil, fmt.Errorf("AzureADAccess: nil credential")
	}
	if audience == "" {
		return nil, fmt.Errorf("AzureADAccess: empty audience")
	}
	scope := scopeFromAudience(audience)
	return &http.Client{
		Transport: &aadBearerTransport{
			cred:  a.Credential,
			scope: scope,
			next:  http.DefaultTransport,
		},
	}, nil
}

// scopeFromAudience converts an HTTP URL audience to the AAD scope
// format expected by the v2 token endpoint: `<resource>/.default`.
// Strips trailing slash + path so the scope is just `<scheme>://<host>/.default`.
func scopeFromAudience(audience string) string {
	resource := audience
	if i := strings.Index(strings.TrimPrefix(strings.TrimPrefix(resource, "https://"), "http://"), "/"); i >= 0 {
		// Strip path (everything after the host).
		schemePrefix := ""
		switch {
		case strings.HasPrefix(resource, "https://"):
			schemePrefix = "https://"
		case strings.HasPrefix(resource, "http://"):
			schemePrefix = "http://"
		}
		hostAndPath := strings.TrimPrefix(resource, schemePrefix)
		host := hostAndPath
		if j := strings.Index(hostAndPath, "/"); j >= 0 {
			host = hostAndPath[:j]
		}
		resource = schemePrefix + host
	}
	resource = strings.TrimSuffix(resource, "/")
	return resource + "/.default"
}

type aadBearerTransport struct {
	cred  azcore.TokenCredential
	scope string
	next  http.RoundTripper
}

func (t *aadBearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tok, err := t.cred.GetToken(req.Context(), policy.TokenRequestOptions{Scopes: []string{t.scope}})
	if err != nil {
		return nil, fmt.Errorf("AzureAD token mint for %s: %w", t.scope, err)
	}
	req2 := req.Clone(req.Context())
	req2.Header.Set("Authorization", "Bearer "+tok.Token)
	return t.next.RoundTrip(req2)
}
