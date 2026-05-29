package azure_sdk_test

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAzureEntra_TokenRS256VerifiesWithJWKS(t *testing.T) {
	tenant := "tenant-rs256"
	tokenBody := requestAzureToken(t, tenant, "oauth2/v2.0/token", url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"client"},
		"client_secret": {"secret"},
		"scope":         {"https://management.azure.com/.default"},
	})

	jwksResp, err := http.Get(baseURL + "/" + tenant + "/discovery/v2.0/keys")
	require.NoError(t, err)
	defer jwksResp.Body.Close()
	require.Equal(t, http.StatusOK, jwksResp.StatusCode)
	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Alg string `json:"alg"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	require.NoError(t, json.NewDecoder(jwksResp.Body).Decode(&jwks))
	require.Len(t, jwks.Keys, 1)
	key := jwks.Keys[0]
	assert.Equal(t, "sockerless-sim-key-1", key.Kid)
	assert.Equal(t, "RSA", key.Kty)
	assert.Equal(t, "RS256", key.Alg)
	assert.Equal(t, "sig", key.Use)
	require.NotEmpty(t, key.N)
	require.NotEmpty(t, key.E)

	parts := strings.Split(tokenBody.AccessToken, ".")
	require.Len(t, parts, 3)
	var header map[string]string
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(headerJSON, &header))
	assert.Equal(t, "RS256", header["alg"])
	assert.Equal(t, key.Kid, header["kid"])

	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	require.NoError(t, err)
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	require.NoError(t, err)
	pub := rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: int(new(big.Int).SetBytes(eBytes).Int64()),
	}

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	require.NoError(t, err)
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	require.NoError(t, rsa.VerifyPKCS1v15(&pub, crypto.SHA256, digest[:], sig))

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(payloadJSON, &payload))
	assert.Equal(t, tenant, payload["tid"])
	assert.Equal(t, "https://management.azure.com/", payload["aud"])
	assert.Equal(t, "https://sts.windows.net/"+tenant+"/", payload["iss"])
}

func TestAzureEntra_TokenAudienceFollowsScopeAndResource(t *testing.T) {
	tests := []struct {
		name     string
		tenant   string
		tokenURL string
		values   url.Values
		wantAud  string
	}{
		{
			name:     "v2 vault scope",
			tenant:   "tenant-vault",
			tokenURL: "oauth2/v2.0/token",
			values: url.Values{
				"grant_type":    {"client_credentials"},
				"client_id":     {"client"},
				"client_secret": {"secret"},
				"scope":         {"https://vault.azure.net/.default"},
			},
			wantAud: "https://vault.azure.net",
		},
		{
			name:     "v2 storage scope",
			tenant:   "tenant-storage",
			tokenURL: "oauth2/v2.0/token",
			values: url.Values{
				"grant_type":    {"client_credentials"},
				"client_id":     {"client"},
				"client_secret": {"secret"},
				"scope":         {"https://storage.azure.com/.default"},
			},
			wantAud: "https://storage.azure.com/",
		},
		{
			name:     "v1 service bus resource",
			tenant:   "tenant-servicebus",
			tokenURL: "oauth2/token",
			values: url.Values{
				"grant_type":    {"client_credentials"},
				"client_id":     {"client"},
				"client_secret": {"secret"},
				"resource":      {"https://servicebus.azure.net"},
			},
			wantAud: "https://servicebus.azure.net",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenBody := requestAzureToken(t, tt.tenant, tt.tokenURL, tt.values)
			payload := azureTokenPayload(t, tokenBody.AccessToken)
			assert.Equal(t, tt.tenant, payload["tid"])
			assert.Equal(t, tt.wantAud, payload["aud"])
		})
	}
}

type azureTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

func requestAzureToken(t *testing.T, tenant, tokenURL string, values url.Values) azureTokenResponse {
	t.Helper()
	tokenResp, err := http.PostForm(baseURL+"/"+tenant+"/"+tokenURL, values)
	require.NoError(t, err)
	defer tokenResp.Body.Close()
	require.Equal(t, http.StatusOK, tokenResp.StatusCode)
	var tokenBody azureTokenResponse
	require.NoError(t, json.NewDecoder(tokenResp.Body).Decode(&tokenBody))
	require.Equal(t, "Bearer", tokenBody.TokenType)
	require.NotEmpty(t, tokenBody.AccessToken)
	return tokenBody
}

func azureTokenPayload(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	require.Len(t, parts, 3)
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(payloadJSON, &payload))
	return payload
}
