package azure_sdk_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
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

func TestStorage_ARMListKeysCanonical512BitPerAccount(t *testing.T) {
	rg := "storage-keys-rg"
	ensureRG(t, rg)
	accountA := "keystoragea"
	accountB := "keystorageb"
	createStorageAccount(t, rg, accountA)
	createStorageAccount(t, rg, accountB)

	keysA := listStorageKeys(t, rg, accountA)
	keysAAgain := listStorageKeys(t, rg, accountA)
	keysB := listStorageKeys(t, rg, accountB)

	require.Len(t, keysA, 2)
	require.Len(t, keysB, 2)
	require.Equal(t, "key1", keysA[0].KeyName)
	require.Equal(t, "key2", keysA[1].KeyName)
	require.NotEqual(t, keysA[0].Value, keysA[1].Value, "key1 and key2 must differ")
	require.Equal(t, keysA[0].Value, keysAAgain[0].Value, "keys must be deterministic for the same account")
	require.NotEqual(t, keysA[0].Value, keysB[0].Value, "different accounts must receive different keys")

	for _, key := range append(keysA, keysB...) {
		raw, err := base64.StdEncoding.DecodeString(key.Value)
		require.NoError(t, err)
		assert.Len(t, raw, 64)
		assert.Equal(t, "FULL", key.Permissions)
	}
}

func TestStorage_ARMPatchAdvertisedEndpointsAndResourceEnumeration(t *testing.T) {
	rg := "storage-patch-rg"
	ensureRG(t, rg)
	accountName := "patchstorage"

	createBody := `{"location":"eastus","kind":"StorageV2","sku":{"name":"Standard_LRS"}}`
	createReq, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.Storage/storageAccounts/"+accountName+"?api-version=2023-05-01",
		strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer fake-token")
	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	createResp.Body.Close()
	require.Equal(t, http.StatusOK, createResp.StatusCode)

	patchBody := `{"tags":{"env":"sdk"},"sku":{"name":"Standard_GRS","tier":"Standard"}}`
	patchReq, _ := http.NewRequestWithContext(ctx, "PATCH",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.Storage/storageAccounts/"+accountName+"?api-version=2023-05-01",
		strings.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq.Header.Set("Authorization", "Bearer fake-token")
	patchResp, err := http.DefaultClient.Do(patchReq)
	require.NoError(t, err)
	defer patchResp.Body.Close()
	require.Equal(t, http.StatusOK, patchResp.StatusCode)

	var patched struct {
		Tags       map[string]string `json:"tags"`
		Sku        StorageSku        `json:"sku"`
		Properties struct {
			PrimaryEndpoints map[string]string `json:"primaryEndpoints"`
		} `json:"properties"`
	}
	require.NoError(t, json.NewDecoder(patchResp.Body).Decode(&patched))
	assert.Equal(t, "sdk", patched.Tags["env"])
	assert.Equal(t, "Standard_GRS", patched.Sku.Name)
	expectedBlob := fmt.Sprintf("http://%s.blob.shim.localhost:%s/", accountName, strings.TrimPrefix(strings.TrimPrefix(baseURL, "http://127.0.0.1:"), "https://127.0.0.1:"))
	assert.Equal(t, expectedBlob, patched.Properties.PrimaryEndpoints["blob"])

	listRGReq, _ := http.NewRequestWithContext(ctx, "GET",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups?api-version=2021-04-01", nil)
	listRGReq.Header.Set("Authorization", "Bearer fake-token")
	listRGResp, err := http.DefaultClient.Do(listRGReq)
	require.NoError(t, err)
	defer listRGResp.Body.Close()
	require.Equal(t, http.StatusOK, listRGResp.StatusCode)
	var rgList struct {
		Value []map[string]any `json:"value"`
	}
	require.NoError(t, json.NewDecoder(listRGResp.Body).Decode(&rgList))
	require.NotEmpty(t, rgList.Value)

	resourcesReq, _ := http.NewRequestWithContext(ctx, "GET",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/resources?api-version=2021-04-01", nil)
	resourcesReq.Header.Set("Authorization", "Bearer fake-token")
	resourcesResp, err := http.DefaultClient.Do(resourcesReq)
	require.NoError(t, err)
	defer resourcesResp.Body.Close()
	require.Equal(t, http.StatusOK, resourcesResp.StatusCode)
	var resources struct {
		Value []map[string]any `json:"value"`
	}
	require.NoError(t, json.NewDecoder(resourcesResp.Body).Decode(&resources))
	found := false
	for _, resource := range resources.Value {
		if resource["id"] == "/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.Storage/storageAccounts/"+accountName {
			found = true
			break
		}
	}
	require.True(t, found, "resource group resources list must include the storage account")
}

type StorageSku struct {
	Name string `json:"name"`
	Tier string `json:"tier,omitempty"`
}

type storageListKey struct {
	KeyName     string `json:"keyName"`
	Value       string `json:"value"`
	Permissions string `json:"permissions"`
}

func createStorageAccount(t *testing.T, rg, accountName string) {
	t.Helper()
	createBody := `{"location":"eastus","kind":"StorageV2","sku":{"name":"Standard_LRS"}}`
	createReq, err := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.Storage/storageAccounts/"+accountName+"?api-version=2023-05-01",
		strings.NewReader(createBody))
	require.NoError(t, err)
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer fake-token")
	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	defer createResp.Body.Close()
	require.Equal(t, http.StatusOK, createResp.StatusCode)
}

func listStorageKeys(t *testing.T, rg, accountName string) []storageListKey {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.Storage/storageAccounts/"+accountName+"/listKeys?api-version=2023-05-01",
		nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer fake-token")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		Keys []storageListKey `json:"keys"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return body.Keys
}
