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

// Azure Storage account management uses the ARM API.
// Using direct HTTP calls for simpler testing against the simulator.

func TestStorage_CreateAccount(t *testing.T) {
	// Ensure resource group
	rgBody := `{"location":"eastus"}`
	rgReq, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/storage-rg?api-version=2023-07-01",
		strings.NewReader(rgBody))
	rgReq.Header.Set("Content-Type", "application/json")
	rgReq.Header.Set("Authorization", "Bearer fake-token")
	rgResp, _ := http.DefaultClient.Do(rgReq)
	rgResp.Body.Close()

	account := map[string]any{
		"location": "eastus",
		"kind":     "StorageV2",
		"sku":      map[string]string{"name": "Standard_LRS"},
		"properties": map[string]any{
			"accessTier": "Hot",
		},
	}
	body, _ := json.Marshal(account)

	req, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/storage-rg/providers/Microsoft.Storage/storageAccounts/teststorage?api-version=2023-05-01",
		strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer fake-token")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Contains(t, []int{200, 201, 202}, resp.StatusCode)
}

func TestStorage_GetAccount(t *testing.T) {
	req, _ := http.NewRequestWithContext(ctx, "GET",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/storage-rg/providers/Microsoft.Storage/storageAccounts/teststorage?api-version=2023-05-01",
		nil)
	req.Header.Set("Authorization", "Bearer fake-token")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	data, _ := io.ReadAll(resp.Body)
	json.Unmarshal(data, &result)
	assert.Equal(t, "teststorage", result["name"])
}
