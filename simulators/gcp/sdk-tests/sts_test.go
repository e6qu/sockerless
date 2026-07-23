package gcp_sdk_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2/google/externalaccount"
	"google.golang.org/api/iam/v1"
)

// oidcIssuer is a minimal but real OpenID Connect issuer: it serves the
// discovery document and JSON Web Key Set the Security Token Service fetches to
// verify a subject token, and signs assertions with the matching key. It stands
// in for the external identity provider a workforce pool federates.
type oidcIssuer struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	keyID  string
}

func newOIDCIssuer(t *testing.T) *oidcIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	issuer := &oidcIssuer{key: key, keyID: "test-key-1"}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuer.server.URL,
			"jwks_uri":                              issuer.server.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key:       key.Public(),
			KeyID:     issuer.keyID,
			Algorithm: string(jose.RS256),
			Use:       "sig",
		}}})
	})
	issuer.server = httptest.NewServer(mux)
	t.Cleanup(issuer.server.Close)
	return issuer
}

// mintAssertion signs an OpenID Connect assertion the way the external identity
// provider would for a signed-in user.
func (i *oidcIssuer) mintAssertion(t *testing.T, subject, audience string, expiry time.Time) string {
	t.Helper()
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: i.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", i.keyID),
	)
	require.NoError(t, err)
	raw, err := jwt.Signed(signer).Claims(jwt.Claims{
		Issuer:   i.server.URL,
		Subject:  subject,
		Audience: jwt.Audience{audience},
		IssuedAt: jwt.NewNumericDate(time.Now()),
		Expiry:   jwt.NewNumericDate(expiry),
	}).Serialize()
	require.NoError(t, err)
	return raw
}

// createWorkforceProvider stands up a workforce pool and an OpenID Connect
// provider beneath it that federates the given issuer, and returns the
// provider's resource name.
func createWorkforceProvider(t *testing.T, issuerURI, audience string) string {
	t.Helper()
	svc := iamService(t)
	location := "locations/global"
	poolID := fmt.Sprintf("sts-pool-%d", time.Now().UnixNano())
	_, err := svc.Locations.WorkforcePools.Create(location,
		&iam.WorkforcePool{DisplayName: "STS Pool", Parent: "organizations/123"}).
		WorkforcePoolId(poolID).Do()
	require.NoError(t, err)

	poolName := location + "/workforcePools/" + poolID
	_, err = svc.Locations.WorkforcePools.Providers.Create(poolName,
		&iam.WorkforcePoolProvider{
			DisplayName: "Test IdP",
			Oidc: &iam.GoogleIamAdminV1WorkforcePoolProviderOidc{
				IssuerUri: issuerURI,
				ClientId:  audience,
			},
			AttributeMapping: map[string]string{"google.subject": "assertion.sub"},
		}).WorkforcePoolProviderId("idp").Do()
	require.NoError(t, err)
	return poolName + "/providers/idp"
}

// staticSubjectToken supplies a fixed assertion to the external-account
// credential, standing in for a live subject-token source.
type staticSubjectToken struct{ token string }

func (s staticSubjectToken) SubjectToken(context.Context, externalaccount.SupplierOptions) (string, error) {
	return s.token, nil
}

// TestSTS_WorkforceFederationExchange drives the real external-account
// credential the Workforce Identity Federation SDK uses: it exchanges a signed
// OpenID Connect assertion for a federated access token at the simulator's
// Security Token Service, exactly as a client would against Google.
func TestSTS_WorkforceFederationExchange(t *testing.T) {
	issuer := newOIDCIssuer(t)
	const audience = "sockerless-console"
	providerName := createWorkforceProvider(t, issuer.server.URL, audience)
	stsAudience := "//iam.googleapis.com/" + providerName

	assertion := issuer.mintAssertion(t, "operator@example.test", audience, time.Now().Add(10*time.Minute))

	source, err := externalaccount.NewTokenSource(context.Background(), externalaccount.Config{
		Audience:                 stsAudience,
		SubjectTokenType:         "urn:ietf:params:oauth:token-type:jwt",
		TokenURL:                 baseURL + "/v1/token",
		SubjectTokenSupplier:     staticSubjectToken{token: assertion},
		Scopes:                   []string{"https://www.googleapis.com/auth/cloud-platform"},
		WorkforcePoolUserProject: "sts-user-project",
	})
	require.NoError(t, err)

	token, err := source.Token()
	require.NoError(t, err)
	assert.NotEmpty(t, token.AccessToken)
	assert.Equal(t, "Bearer", token.TokenType)
	assert.True(t, token.Expiry.After(time.Now()), "federated token must have a future expiry")
}

// TestSTS_RejectsUnknownProvider proves the exchange fails when the audience
// names a provider that was never created, rather than issuing a token for an
// unconfigured federation.
func TestSTS_RejectsUnknownProvider(t *testing.T) {
	issuer := newOIDCIssuer(t)
	assertion := issuer.mintAssertion(t, "operator@example.test", "sockerless-console", time.Now().Add(10*time.Minute))

	resp := postTokenExchange(t, map[string]string{
		"grant_type":         "urn:ietf:params:oauth:grant-type:token-exchange",
		"audience":           "//iam.googleapis.com/locations/global/workforcePools/absent/providers/absent",
		"subject_token":      assertion,
		"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
	})
	assert.Equal(t, http.StatusBadRequest, resp.status)
	assert.Equal(t, "invalid_target", resp.body["error"])
}

// TestSTS_RejectsExpiredAssertion proves a subject token past its expiry is
// refused, so a stale assertion cannot be exchanged for a live token.
func TestSTS_RejectsExpiredAssertion(t *testing.T) {
	issuer := newOIDCIssuer(t)
	const audience = "sockerless-console"
	providerName := createWorkforceProvider(t, issuer.server.URL, audience)
	expired := issuer.mintAssertion(t, "operator@example.test", audience, time.Now().Add(-time.Minute))

	resp := postTokenExchange(t, map[string]string{
		"grant_type":         "urn:ietf:params:oauth:grant-type:token-exchange",
		"audience":           "//iam.googleapis.com/" + providerName,
		"subject_token":      expired,
		"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
	})
	assert.Equal(t, http.StatusBadRequest, resp.status)
	assert.Equal(t, "invalid_grant", resp.body["error"])
}

// TestSTS_RejectsDisallowedAudience proves an assertion minted for an audience
// the provider does not permit is refused.
func TestSTS_RejectsDisallowedAudience(t *testing.T) {
	issuer := newOIDCIssuer(t)
	providerName := createWorkforceProvider(t, issuer.server.URL, "sockerless-console")
	wrongAudience := issuer.mintAssertion(t, "operator@example.test", "some-other-client", time.Now().Add(10*time.Minute))

	resp := postTokenExchange(t, map[string]string{
		"grant_type":         "urn:ietf:params:oauth:grant-type:token-exchange",
		"audience":           "//iam.googleapis.com/" + providerName,
		"subject_token":      wrongAudience,
		"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
	})
	assert.Equal(t, http.StatusBadRequest, resp.status)
	assert.Equal(t, "invalid_grant", resp.body["error"])
}

type tokenExchangeResponse struct {
	status int
	body   map[string]string
}

func postTokenExchange(t *testing.T, form map[string]string) tokenExchangeResponse {
	t.Helper()
	values := url.Values{}
	for key, value := range form {
		values.Set(key, value)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/token", strings.NewReader(values.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return tokenExchangeResponse{status: resp.StatusCode, body: body}
}
