package azure_sdk_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azcertificates"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kvVaultURL returns the data-plane URL the Azure KV SDK clients
// connect to: `https://<vault>.vault.<sim-host>:<port>`. The KV
// SDKs construct an internal BearerTokenPolicy that refuses non-
// HTTPS scheme regardless of `InsecureAllowCredentialWithHTTP`, so
// the client URL must say `https`; httpToHTTPSRewriter (the
// transport below) rewrites scheme + dial target down to the sim's
// actual HTTP listener at request time.
func kvVaultURL(vault string) string {
	host := strings.TrimPrefix(baseURL, "http://")
	return "https://" + vault + ".vault." + host
}

// kvSchemeRewritingTransport rewrites every request's URL scheme
// from `https` to `http` before dispatching it via the default
// transport. The KV SDK's internal BearerTokenPolicy checks
// `req.URL.Scheme == "https"` before letting the request through,
// so the client URL must advertise HTTPS. The actual sim listener
// is plain HTTP, so we drop the TLS before dialing. This is the
// sim's analogue of the InsecureSkipVerify + self-signed-cert
// pattern other sim tests use, with the simpler "no TLS at all"
// shape the sim's listener supports.
type kvSchemeRewritingTransport struct {
	inner *http.Client
}

func (t kvSchemeRewritingTransport) Do(req *http.Request) (*http.Response, error) {
	rewritten := req.Clone(req.Context())
	newURL := *req.URL
	// `<vault>.vault.localhost` doesn't resolve via DNS on macOS / CI
	// runners; keep that host on the Host header (so the sim's
	// subdomain dispatcher matches) but dial the sim's actual
	// loopback address.
	rewritten.Host = req.URL.Host
	simHost := strings.TrimPrefix(baseURL, "http://")
	simHost = strings.TrimPrefix(simHost, "https://")
	newURL.Host = simHost
	if req.URL.Scheme == "https" {
		newURL.Scheme = "http"
	}
	rewritten.URL = &newURL
	client := t.inner
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(rewritten)
}

// kvSchemeRewritingTransport wraps an *http.Client to satisfy the
// Azure SDK's `policy.Transporter` interface (which requires `Do`,
// not `RoundTrip` — the SDK never speaks to the lower http
// `RoundTripper` directly).

// kvClientOptions returns the canonical Azure KV client options
// for talking to the sim. `DisableChallengeResourceVerification`
// is set because the sim's data-plane listens on
// `<vault>.vault.localhost:<port>` while the challenge's
// `resource="https://vault.azure.net"` would otherwise fail the
// SDK's `HasSuffix(req.Host, "."+resource.Host)` check. The
// scheme-rewriting transport carries the matching HTTPS→HTTP
// allowed-diff for the sim's plain-HTTP listener.
func kvClientOptions() azcore.ClientOptions {
	return azcore.ClientOptions{
		Transport: kvSchemeRewritingTransport{},
	}
}

// createKVViaARM creates a key vault through the ARM control plane.
// Raw HTTP is fine here — ARM endpoints don't issue the data-plane
// `WWW-Authenticate` challenge that the KV SDK packages exercise.
func createKVViaARM(t *testing.T, rg, vault string) {
	t.Helper()
	ensureRG(t, rg)
	createBody, _ := json.Marshal(map[string]any{
		"location": "eastus",
		"properties": map[string]any{
			"tenantId": "00000000-0000-0000-0000-000000000000",
		},
	})
	req, _ := http.NewRequest("PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+
			"/providers/Microsoft.KeyVault/vaults/"+vault+"?api-version=2024-04-01-preview",
		strings.NewReader(string(createBody)))
	req.Header.Set("Authorization", "Bearer fake-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "ARM vault create must succeed")
	t.Cleanup(func() {
		del, _ := http.NewRequest("DELETE",
			baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+
				"/providers/Microsoft.KeyVault/vaults/"+vault+"?api-version=2024-04-01-preview",
			nil)
		del.Header.Set("Authorization", "Bearer fake-token")
		resp, _ := http.DefaultClient.Do(del)
		if resp != nil {
			resp.Body.Close()
		}
	})
}

// TestKeyVault_SDK_Secrets_ChallengeRoundTrip exercises the full
// canonical Azure SDK flow against the sim — including the
// challenge-then-retry handshake that real consumers depend on.
//
// `azsecrets.NewClient` issues the initial request without an
// Authorization header. The sim responds 401 + `WWW-Authenticate:
// Bearer authorization=..., resource=...`. The SDK's challenge
// policy parses the URL, asks the configured credential for a
// token, then retries the request with `Authorization: Bearer ...`
// set.
//
// The `authorization` URL must split on `/` into ≥ 4 segments
// because the SDK's `parseTenant` indexes `parts[3]` without a
// bounds check. A URL like `http://<host>/<tenant-uuid>` works
// (4 segments: ["http:", "", "<host>", "<tenant>"]); the bare
// `http://<host>` form panics. This test fails — by SDK panic —
// against any sim build that emits the 3-segment form.
func TestKeyVault_SDK_Secrets_ChallengeRoundTrip(t *testing.T) {
	rg := "kv-sdk-secrets-rg"
	vault := "kv-sdk-secrets"
	createKVViaARM(t, rg, vault)

	client, err := azsecrets.NewClient(kvVaultURL(vault), &fakeCredential{},
		&azsecrets.ClientOptions{
			ClientOptions:                        kvClientOptions(),
			DisableChallengeResourceVerification: true,
		})
	require.NoError(t, err)

	setResp, err := client.SetSecret(ctx, "db-password",
		azsecrets.SetSecretParameters{Value: stringPtr("hunter2")}, nil)
	require.NoError(t, err, "SetSecret over SDK must succeed (challenge round-trip)")
	require.NotNil(t, setResp.Value)
	assert.Equal(t, "hunter2", *setResp.Value)

	getResp, err := client.GetSecret(ctx, "db-password", "", nil)
	require.NoError(t, err)
	require.NotNil(t, getResp.Value)
	assert.Equal(t, "hunter2", *getResp.Value)

	_, err = client.DeleteSecret(ctx, "db-password", nil)
	require.NoError(t, err)
}

// TestKeyVault_SDK_Keys_ChallengeRoundTrip covers the same
// challenge-then-retry handshake for the keys client. The SDK
// shares the challenge-policy code with secrets, so the same
// `parts[3]` parse panic surfaces here too if the authorization
// URL is malformed.
func TestKeyVault_SDK_Keys_ChallengeRoundTrip(t *testing.T) {
	rg := "kv-sdk-keys-rg"
	vault := "kv-sdk-keys"
	createKVViaARM(t, rg, vault)

	client, err := azkeys.NewClient(kvVaultURL(vault), &fakeCredential{},
		&azkeys.ClientOptions{
			ClientOptions:                        kvClientOptions(),
			DisableChallengeResourceVerification: true,
		})
	require.NoError(t, err)

	createResp, err := client.CreateKey(ctx, "signing-key",
		azkeys.CreateKeyParameters{Kty: keyTypePtr(azkeys.KeyTypeRSA)}, nil)
	require.NoError(t, err, "CreateKey over SDK must succeed")
	require.NotNil(t, createResp.Key)
	require.NotNil(t, createResp.Key.KID)

	getResp, err := client.GetKey(ctx, "signing-key", "", nil)
	require.NoError(t, err)
	require.NotNil(t, getResp.Key.KID)
	assert.Equal(t, *createResp.Key.KID, *getResp.Key.KID,
		"GetKey must return the same KID emitted by CreateKey")
}

// TestKeyVault_SDK_Certificates_ChallengeRoundTrip covers the
// challenge-then-retry handshake for the certificates client. The
// test exercises GetCertificate (not CreateCertificate, which is a
// real Long-Running Operation on real Azure and a separate sim
// surface tracked elsewhere) — that's still enough to put the
// challenge policy through its paces; the surface that's load-
// bearing here is the challenge handshake, not the cert lifecycle.
func TestKeyVault_SDK_Certificates_ChallengeRoundTrip(t *testing.T) {
	rg := "kv-sdk-certs-rg"
	vault := "kv-sdk-certs"
	createKVViaARM(t, rg, vault)

	client, err := azcertificates.NewClient(kvVaultURL(vault), &fakeCredential{},
		&azcertificates.ClientOptions{
			ClientOptions:                        kvClientOptions(),
			DisableChallengeResourceVerification: true,
		})
	require.NoError(t, err)

	// GetCertificate on a non-existent cert: the canonical error is
	// 404 + `CertificateNotFound`; the SDK surfaces it as a non-nil
	// error. The challenge handshake fires regardless of the
	// downstream 404, which is what we're locking in.
	_, err = client.GetCertificate(ctx, "missing-cert", "", nil)
	require.Error(t, err, "GetCertificate must surface the 404 (challenge still fired)")
	assert.Contains(t, err.Error(), "CertificateNotFound",
		"sim must emit the canonical CertificateNotFound error code")
}

// Helper to express &"..." inline — Azure SDKs take *string everywhere.
func stringPtr(s string) *string { return &s }

// Helper to express &keyType inline for azkeys.CreateKeyParameters.
func keyTypePtr(k azkeys.KeyType) *azkeys.KeyType { return &k }
