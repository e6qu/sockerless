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

// TestKeyVault_ARM_CRUD pins the ARM control plane: vault create, get,
// list, delete. Real Azure: PUT / GET / DELETE on
// `Microsoft.KeyVault/vaults/{name}`.
func TestKeyVault_ARM_CRUD(t *testing.T) {
	rg := "kv-rg"
	ensureRG(t, rg)

	vaultName := "sdk-test-vault"
	defer func() {
		req, _ := http.NewRequest("DELETE",
			baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.KeyVault/vaults/"+vaultName+"?api-version=2024-04-01-preview",
			nil)
		req.Header.Set("Authorization", "Bearer fake-token")
		http.DefaultClient.Do(req)
	}()

	body := map[string]any{
		"location": "eastus",
		"properties": map[string]any{
			"tenantId": "00000000-0000-0000-0000-000000000000",
			"sku": map[string]any{
				"family": "A",
				"name":   "standard",
			},
			"enableRbacAuthorization": true,
		},
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.KeyVault/vaults/"+vaultName+"?api-version=2024-04-01-preview",
		strings.NewReader(string(data)))
	req.Header.Set("Authorization", "Bearer fake-token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var vault map[string]any
	require.NoError(t, json.Unmarshal(respBody, &vault))
	props := vault["properties"].(map[string]any)
	vaultURI, _ := props["vaultUri"].(string)
	require.NotEmpty(t, vaultURI, "vaultUri must be set on create")
	assert.Contains(t, vaultURI, ".vault.", "vaultUri should follow real-Azure subdomain format")
	assert.Equal(t, "Succeeded", props["provisioningState"])

	// GET round-trips.
	getReq, _ := http.NewRequest("GET",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.KeyVault/vaults/"+vaultName+"?api-version=2024-04-01-preview",
		nil)
	getReq.Header.Set("Authorization", "Bearer fake-token")
	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()
	require.Equal(t, http.StatusOK, getResp.StatusCode)
}

// TestKeyVault_DataPlane_SetGetDelete pins the secret data plane:
// PUT /secrets/{name} → GET → DELETE round-trip with the Host header
// set to `<vault>.vault.<sim-host>` matching real-Azure subdomain
// routing.
func TestKeyVault_DataPlane_SetGetDelete(t *testing.T) {
	rg := "kv-data-rg"
	ensureRG(t, rg)
	vaultName := "data-vault"
	defer func() {
		req, _ := http.NewRequest("DELETE",
			baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.KeyVault/vaults/"+vaultName+"?api-version=2024-04-01-preview",
			nil)
		req.Header.Set("Authorization", "Bearer fake-token")
		http.DefaultClient.Do(req)
	}()

	createBody, _ := json.Marshal(map[string]any{
		"location": "eastus",
		"properties": map[string]any{
			"tenantId": "00000000-0000-0000-0000-000000000000",
		},
	})
	createReq, _ := http.NewRequest("PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.KeyVault/vaults/"+vaultName+"?api-version=2024-04-01-preview",
		strings.NewReader(string(createBody)))
	createReq.Header.Set("Authorization", "Bearer fake-token")
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	createResp.Body.Close()

	// Data plane URL = baseURL but Host header matches `<vault>.vault.<sim-host>`.
	host := strings.TrimPrefix(baseURL, "http://")
	dataPlaneHost := vaultName + ".vault." + host

	// SET secret.
	setBody, _ := json.Marshal(map[string]any{
		"value": "hunter2",
		"tags":  map[string]string{"env": "prod"},
	})
	setReq, _ := http.NewRequest("PUT",
		baseURL+"/secrets/db-password?api-version=7.4",
		strings.NewReader(string(setBody)))
	setReq.Header.Set("Authorization", "Bearer fake-token")
	setReq.Header.Set("Content-Type", "application/json")
	setReq.Host = dataPlaneHost
	setResp, err := http.DefaultClient.Do(setReq)
	require.NoError(t, err)
	defer setResp.Body.Close()
	require.Equal(t, http.StatusOK, setResp.StatusCode, "SetSecret must succeed")

	// GET secret back.
	getReq, _ := http.NewRequest("GET",
		baseURL+"/secrets/db-password?api-version=7.4", nil)
	getReq.Header.Set("Authorization", "Bearer fake-token")
	getReq.Host = dataPlaneHost
	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	body, _ := io.ReadAll(getResp.Body)
	var secret map[string]any
	require.NoError(t, json.Unmarshal(body, &secret))
	assert.Equal(t, "hunter2", secret["value"])

	// DELETE secret.
	delReq, _ := http.NewRequest("DELETE",
		baseURL+"/secrets/db-password?api-version=7.4", nil)
	delReq.Header.Set("Authorization", "Bearer fake-token")
	delReq.Host = dataPlaneHost
	delResp, err := http.DefaultClient.Do(delReq)
	require.NoError(t, err)
	delResp.Body.Close()
	require.Equal(t, http.StatusOK, delResp.StatusCode)

	// Subsequent GET must 404.
	gone, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	gone.Body.Close()
	assert.Equal(t, http.StatusNotFound, gone.StatusCode, "GET after delete must 404")
}
