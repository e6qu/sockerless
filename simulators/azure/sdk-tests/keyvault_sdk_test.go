package azure_sdk_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

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
	assertSecretRecoveryAttrs(t, setResp.Attributes)

	getResp, err := client.GetSecret(ctx, "db-password", "", nil)
	require.NoError(t, err)
	require.NotNil(t, getResp.Value)
	assert.Equal(t, "hunter2", *getResp.Value)
	assertSecretRecoveryAttrs(t, getResp.Attributes)

	version := getResp.ID.Version()
	updated, err := client.UpdateSecretProperties(ctx, "db-password", version,
		azsecrets.UpdateSecretPropertiesParameters{
			ContentType: stringPtr("text/plain"),
			Tags:        map[string]*string{"scope": stringPtr("sdk")},
		}, nil)
	require.NoError(t, err)
	require.NotNil(t, updated.ContentType)
	assert.Equal(t, "text/plain", *updated.ContentType)

	pager := client.NewListSecretPropertiesPager(nil)
	var listed bool
	for pager.More() {
		page, err := pager.NextPage(ctx)
		require.NoError(t, err)
		for _, item := range page.Value {
			if item.ID != nil && item.ID.Name() == "db-password" {
				listed = true
			}
		}
	}
	require.True(t, listed, "ListSecretProperties must include the SDK-created secret")

	backup, err := client.BackupSecret(ctx, "db-password", nil)
	require.NoError(t, err)
	require.NotEmpty(t, backup.Value)

	_, err = client.DeleteSecret(ctx, "db-password", nil)
	require.NoError(t, err)
	_, err = client.PurgeDeletedSecret(ctx, "db-password", nil)
	require.NoError(t, err)

	restored, err := client.RestoreSecret(ctx, azsecrets.RestoreSecretParameters{SecretBackup: backup.Value}, nil)
	require.NoError(t, err)
	require.NotNil(t, restored.Value)
	assert.Equal(t, "hunter2", *restored.Value)
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
	assertKeyRecoveryAttrs(t, createResp.Attributes)
	version := createResp.Key.KID.Version()

	getResp, err := client.GetKey(ctx, "signing-key", "", nil)
	require.NoError(t, err)
	require.NotNil(t, getResp.Key.KID)
	assertKeyRecoveryAttrs(t, getResp.Attributes)
	assert.Equal(t, *createResp.Key.KID, *getResp.Key.KID,
		"GetKey must return the same KID emitted by CreateKey")

	updated, err := client.UpdateKey(ctx, "signing-key", version,
		azkeys.UpdateKeyParameters{Tags: map[string]*string{"scope": stringPtr("sdk")}}, nil)
	require.NoError(t, err)
	require.NotNil(t, updated.Key.KID)

	versionPager := client.NewListKeyPropertiesVersionsPager("signing-key", nil)
	require.True(t, versionPager.More())
	versionPage, err := versionPager.NextPage(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, versionPage.Value)

	digest := sha256.Sum256([]byte("payload"))
	signResp, err := client.Sign(ctx, "signing-key", version,
		azkeys.SignParameters{Algorithm: signatureAlgPtr(azkeys.SignatureAlgorithmRS256), Value: digest[:]}, nil)
	require.NoError(t, err)
	require.NotEmpty(t, signResp.Result)
	verifyResp, err := client.Verify(ctx, "signing-key", version,
		azkeys.VerifyParameters{Algorithm: signatureAlgPtr(azkeys.SignatureAlgorithmRS256), Digest: digest[:], Signature: signResp.Result}, nil)
	require.NoError(t, err)
	require.NotNil(t, verifyResp.Value)
	assert.True(t, *verifyResp.Value)

	encryptResp, err := client.Encrypt(ctx, "signing-key", version,
		azkeys.KeyOperationParameters{Algorithm: encryptionAlgPtr(azkeys.EncryptionAlgorithmRSAOAEP256), Value: []byte("secret")}, nil)
	require.NoError(t, err)
	decryptResp, err := client.Decrypt(ctx, "signing-key", version,
		azkeys.KeyOperationParameters{Algorithm: encryptionAlgPtr(azkeys.EncryptionAlgorithmRSAOAEP256), Value: encryptResp.Result}, nil)
	require.NoError(t, err)
	assert.Equal(t, []byte("secret"), decryptResp.Result)

	wrapResp, err := client.WrapKey(ctx, "signing-key", version,
		azkeys.KeyOperationParameters{Algorithm: encryptionAlgPtr(azkeys.EncryptionAlgorithmRSAOAEP256), Value: []byte("wrapme")}, nil)
	require.NoError(t, err)
	unwrapResp, err := client.UnwrapKey(ctx, "signing-key", version,
		azkeys.KeyOperationParameters{Algorithm: encryptionAlgPtr(azkeys.EncryptionAlgorithmRSAOAEP256), Value: wrapResp.Result}, nil)
	require.NoError(t, err)
	assert.Equal(t, []byte("wrapme"), unwrapResp.Result)

	backup, err := client.BackupKey(ctx, "signing-key", nil)
	require.NoError(t, err)
	require.NotEmpty(t, backup.Value)
	_, err = client.DeleteKey(ctx, "signing-key", nil)
	require.NoError(t, err)
	_, err = client.PurgeDeletedKey(ctx, "signing-key", nil)
	require.NoError(t, err)
	restored, err := client.RestoreKey(ctx, azkeys.RestoreKeyParameters{KeyBackup: backup.Value}, nil)
	require.NoError(t, err)
	require.NotNil(t, restored.Key.KID)
}

// TestKeyVault_SDK_Certificates_ChallengeRoundTrip covers the
// challenge-then-retry handshake and the certificate lifecycle routes
// the official Azure SDK uses for create, pending operation polling,
// update, import, merge, backup, restore, and list pagers.
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

	createResp, err := client.CreateCertificate(ctx, "tls-cert",
		azcertificates.CreateCertificateParameters{
			CertificatePolicy: selfSignedCertPolicy(),
			Tags:              map[string]*string{"scope": stringPtr("sdk")},
		}, nil)
	require.NoError(t, err)
	require.NotNil(t, createResp.Status)
	assert.Equal(t, "completed", *createResp.Status)
	require.NotNil(t, createResp.Target)

	op, err := client.GetCertificateOperation(ctx, "tls-cert", nil)
	require.NoError(t, err)
	require.NotNil(t, op.Status)
	assert.Equal(t, "completed", *op.Status)

	cert, err := client.GetCertificate(ctx, "tls-cert", "", nil)
	require.NoError(t, err)
	require.NotNil(t, cert.ID)
	assertCertificateRecoveryAttrs(t, cert.Attributes)
	version := cert.ID.Version()
	require.NotEmpty(t, cert.CER)
	require.NotEmpty(t, cert.X509Thumbprint)

	updated, err := client.UpdateCertificate(ctx, "tls-cert", version,
		azcertificates.UpdateCertificateParameters{Tags: map[string]*string{"scope": stringPtr("updated")}}, nil)
	require.NoError(t, err)
	require.NotNil(t, updated.ID)

	pager := client.NewListCertificatePropertiesPager(nil)
	var listed bool
	for pager.More() {
		page, err := pager.NextPage(ctx)
		require.NoError(t, err)
		for _, item := range page.Value {
			if item.ID != nil && item.ID.Name() == "tls-cert" {
				listed = true
			}
		}
	}
	require.True(t, listed, "ListCertificateProperties must include the SDK-created certificate")

	versionPager := client.NewListCertificatePropertiesVersionsPager("tls-cert", nil)
	require.True(t, versionPager.More())
	versionPage, err := versionPager.NextPage(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, versionPage.Value)

	certDER := testCertificateDER(t, "imported-cert")
	imported, err := client.ImportCertificate(ctx, "imported-cert",
		azcertificates.ImportCertificateParameters{
			Base64EncodedCertificate: stringPtr(base64.StdEncoding.EncodeToString(certDER)),
			CertificatePolicy:        selfSignedCertPolicy(),
		}, nil)
	require.NoError(t, err)
	require.NotNil(t, imported.ID)
	assertCertificateRecoveryAttrs(t, imported.Attributes)

	_, err = client.CreateCertificate(ctx, "merged-cert",
		azcertificates.CreateCertificateParameters{CertificatePolicy: unknownIssuerCertPolicy()}, nil)
	require.NoError(t, err)
	merged, err := client.MergeCertificate(ctx, "merged-cert",
		azcertificates.MergeCertificateParameters{X509Certificates: [][]byte{testCertificateDER(t, "merged-cert")}}, nil)
	require.NoError(t, err)
	require.NotNil(t, merged.ID)
	assertCertificateRecoveryAttrs(t, merged.Attributes)

	backup, err := client.BackupCertificate(ctx, "tls-cert", nil)
	require.NoError(t, err)
	require.NotEmpty(t, backup.Value)
	_, err = client.DeleteCertificate(ctx, "tls-cert", nil)
	require.NoError(t, err)
	_, err = client.PurgeDeletedCertificate(ctx, "tls-cert", nil)
	require.NoError(t, err)
	restored, err := client.RestoreCertificate(ctx,
		azcertificates.RestoreCertificateParameters{CertificateBackup: backup.Value}, nil)
	require.NoError(t, err)
	require.NotNil(t, restored.ID)
}

// Helper to express &"..." inline — Azure SDKs take *string everywhere.
func stringPtr(s string) *string { return &s }

// Helper to express &keyType inline for azkeys.CreateKeyParameters.
func keyTypePtr(k azkeys.KeyType) *azkeys.KeyType { return &k }

func signatureAlgPtr(a azkeys.SignatureAlgorithm) *azkeys.SignatureAlgorithm { return &a }

func encryptionAlgPtr(a azkeys.EncryptionAlgorithm) *azkeys.EncryptionAlgorithm { return &a }

func certKeyTypePtr(k azcertificates.KeyType) *azcertificates.KeyType { return &k }

func int32Ptr(v int32) *int32 { return &v }

func assertSecretRecoveryAttrs(t *testing.T, attrs *azsecrets.SecretAttributes) {
	t.Helper()
	require.NotNil(t, attrs)
	require.NotNil(t, attrs.RecoveryLevel)
	assert.Equal(t, "Recoverable+Purgeable", *attrs.RecoveryLevel)
	require.NotNil(t, attrs.RecoverableDays)
	assert.Equal(t, int32(90), *attrs.RecoverableDays)
}

func assertKeyRecoveryAttrs(t *testing.T, attrs *azkeys.KeyAttributes) {
	t.Helper()
	require.NotNil(t, attrs)
	require.NotNil(t, attrs.RecoveryLevel)
	assert.Equal(t, "Recoverable+Purgeable", *attrs.RecoveryLevel)
	require.NotNil(t, attrs.RecoverableDays)
	assert.Equal(t, int32(90), *attrs.RecoverableDays)
}

func assertCertificateRecoveryAttrs(t *testing.T, attrs *azcertificates.CertificateAttributes) {
	t.Helper()
	require.NotNil(t, attrs)
	require.NotNil(t, attrs.RecoveryLevel)
	assert.Equal(t, "Recoverable+Purgeable", *attrs.RecoveryLevel)
	require.NotNil(t, attrs.RecoverableDays)
	assert.Equal(t, int32(90), *attrs.RecoverableDays)
}

func selfSignedCertPolicy() *azcertificates.CertificatePolicy {
	return &azcertificates.CertificatePolicy{
		IssuerParameters: &azcertificates.IssuerParameters{Name: stringPtr("Self")},
		KeyProperties: &azcertificates.KeyProperties{
			KeyType: certKeyTypePtr(azcertificates.KeyTypeRSA),
			KeySize: int32Ptr(2048),
		},
		SecretProperties: &azcertificates.SecretProperties{ContentType: stringPtr("application/x-pkcs12")},
		X509CertificateProperties: &azcertificates.X509CertificateProperties{
			Subject:          stringPtr("CN=sockerless.local"),
			ValidityInMonths: int32Ptr(12),
		},
	}
}

func unknownIssuerCertPolicy() *azcertificates.CertificatePolicy {
	p := selfSignedCertPolicy()
	p.IssuerParameters.Name = stringPtr("Unknown")
	return p
}

func testCertificateDER(t *testing.T, commonName string) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{commonName},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return der
}
