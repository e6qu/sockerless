package gcp_sdk_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"
)

// writeServiceAccountJSON stages a real-shape service-account credentials file
// with a real RSA keypair whose `token_uri` points at the simulator's OAuth2
// token endpoint — the same credential shape a Cloud Run / Cloud Functions
// backend runs with (GOOGLE_APPLICATION_CREDENTIALS). The
// google.golang.org/api/idtoken service-account flow signs a JWT-bearer
// assertion with this key, POSTs it to `token_uri`, and reads back the
// `id_token`.
func writeServiceAccountJSON(t *testing.T) string {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	sa := map[string]string{
		"type":            "service_account",
		"project_id":      "test-project",
		"private_key_id":  "sim-key",
		"private_key":     string(keyPEM),
		"client_email":    "sockerless-runner@test-project.iam.gserviceaccount.com",
		"client_id":       "111111111111111111111",
		"token_uri":       baseURL + "/token",
		"universe_domain": "googleapis.com",
	}
	body, err := json.Marshal(sa)
	require.NoError(t, err)

	f, err := os.CreateTemp(t.TempDir(), "sa-*.json")
	require.NoError(t, err)
	_, err = f.Write(body)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

// TestOAuth2IDToken_InvokeBearerAccepted reproduces the Cloud Run / Cloud
// Functions backend service-invoke path exactly: with a service-account
// credential whose token_uri is the simulator, the backend's
// gcpcommon.IDTokenAccess builds an idtoken client for the target service URL;
// idtoken runs the service-account JWT-bearer flow, POSTs to the sim's /token
// endpoint, and presents the returned id_token as its Authorization: Bearer
// when invoking the service.
//
// The id_token the sim returns must be an RS256 token the data-plane bearer
// middleware verifies — the regression this guards is the endpoint returning an
// HS256 id_token, which the middleware rejected with 401 "token algorithm HS256
// is not RS256".
func TestOAuth2IDToken_InvokeBearerAccepted(t *testing.T) {
	saPath := writeServiceAccountJSON(t)

	// A gated Cloud Run data-plane URL stands in for the target service; the
	// audience the backend requests is the service URL it will invoke.
	audience := baseURL + "/v2/projects/test-project/locations/us-central1/services"

	// The exact minting the invoke path performs: idtoken's service-account
	// flow exchanges the SA assertion at the sim /token endpoint for the
	// id_token it will bear.
	ts, err := idtoken.NewTokenSource(ctx, audience, option.WithCredentialsFile(saPath))
	require.NoError(t, err)
	tok, err := ts.Token()
	require.NoError(t, err)
	require.NotEmpty(t, tok.AccessToken)

	// The bearer must be RS256 — the header alg the data-plane middleware
	// enforces.
	parts := strings.Split(tok.AccessToken, ".")
	require.Len(t, parts, 3, "id_token must be a 3-segment JWT")
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	require.NoError(t, err)
	var hdr struct {
		Alg string `json:"alg"`
	}
	require.NoError(t, json.Unmarshal(header, &hdr))
	require.Equal(t, "RS256", hdr.Alg, "invoke bearer must be RS256, not HS256")

	// Present that exact bearer to the gated data plane, the way the backend's
	// idtoken client does when it invokes the service. A valid RS256 token the
	// sim minted is accepted (200); the pre-fix HS256 token was rejected (401).
	client, err := idtoken.NewClient(ctx, audience, option.WithCredentialsFile(saPath))
	require.NoError(t, err)
	resp, err := client.Get(audience)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"data plane must accept the RS256 invoke bearer the idtoken service-account flow mints")
}
