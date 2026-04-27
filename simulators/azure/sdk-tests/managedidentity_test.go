package azure_sdk_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMSI_TokenEndpoint pins the IMDS-style metadata service. Real
// Azure exposes managed-identity tokens via two paths
// (`/metadata/identity/oauth2/token` for VMs, `/msi/token` via
// IDENTITY_ENDPOINT for App Service / Container Apps); both must
// return the standard token-payload shape so DefaultAzureCredential
// (Azure SDK + every SDK that wraps it) can mint tokens.
func TestMSI_TokenEndpoint_VMShape(t *testing.T) {
	resp, err := http.Get(baseURL + "/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https%3A%2F%2Fmanagement.azure.com%2F")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))

	tok, _ := payload["access_token"].(string)
	assert.True(t, strings.HasPrefix(tok, "sim-msi-"), "access_token must be present and sim-tagged")
	assert.Equal(t, "Bearer", payload["token_type"])
	assert.Equal(t, "https://management.azure.com/", payload["resource"])
	assert.NotEmpty(t, payload["expires_on"])
}

func TestMSI_TokenEndpoint_AppServiceShape(t *testing.T) {
	resp, err := http.Get(baseURL + "/msi/token?api-version=2019-08-01&resource=https%3A%2F%2Fstorage.azure.com%2F")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))

	assert.NotEmpty(t, payload["access_token"])
	assert.Equal(t, "https://storage.azure.com/", payload["resource"])
}

func TestMSI_TokenEndpoint_RejectsMissingResource(t *testing.T) {
	resp, err := http.Get(baseURL + "/metadata/identity/oauth2/token?api-version=2018-02-01")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"real IMDS rejects requests without 'resource'; sim must too")
}
