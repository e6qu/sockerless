package azure_cli_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BUG-834 — sim was missing the v2 ContainerApps "Apps" routes
// (Microsoft.App/containerApps). Smoke-test the CRUD via az rest so a
// regression in the route registration is caught at the wire level.
func TestContainerAppsApps_CLI_CreateGetDelete(t *testing.T) {
	appURL := acaURL("containerApps/cli-test-app")
	body := `{
		"location": "eastus",
		"tags": { "sockerless-managed": "true" },
		"properties": {
			"environmentId": "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.App/managedEnvironments/sim-env",
			"configuration": {
				"activeRevisionsMode": "Single",
				"ingress": { "external": false, "targetPort": 8080, "transport": "auto" }
			},
			"template": {
				"containers": [{ "name": "main", "image": "alpine:latest" }],
				"scale": { "minReplicas": 1, "maxReplicas": 1 }
			}
		}
	}`
	out := runCLI(t, azRest("PUT", appURL, body))
	var created struct {
		Name       string `json:"name"`
		Properties struct {
			ProvisioningState       string `json:"provisioningState"`
			LatestReadyRevisionName string `json:"latestReadyRevisionName"`
			LatestRevisionFqdn      string `json:"latestRevisionFqdn"`
		} `json:"properties"`
	}
	parseJSON(t, out, &created)
	assert.Equal(t, "cli-test-app", created.Name)
	assert.Equal(t, "Succeeded", created.Properties.ProvisioningState,
		"backend reads provisioningState=Succeeded for state=running")
	assert.NotEmpty(t, created.Properties.LatestReadyRevisionName)
	assert.NotEmpty(t, created.Properties.LatestRevisionFqdn,
		"LatestRevisionFqdn drives Private DNS CNAME for peer discovery")

	out = runCLI(t, azRest("GET", appURL, ""))
	var got struct {
		Name string `json:"name"`
	}
	parseJSON(t, out, &got)
	assert.Equal(t, "cli-test-app", got.Name)

	runCLI(t, azRest("DELETE", appURL, ""))

	// GET after delete should 404 — verify via raw HTTP since
	// `az rest` exits non-zero on 404 and runCLI requires success.
	req, err := http.NewRequest("GET", baseURL+"/subscriptions/"+subscriptionID+
		"/resourceGroups/"+resourceGroup+
		"/providers/Microsoft.App/containerApps/cli-test-app?api-version="+acaAPIVersion, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer fake-token")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode, "GET after delete must be 404")
}

// BUG-835 — sim was missing WebApps.UpdateAzureStorageAccounts; the
// azure-functions backend uses it to bind named volumes to Azure Files.
func TestWebApps_CLI_UpdateAzureStorageAccounts(t *testing.T) {
	const azfAPIVersion = "2024-04-01"
	siteURL := armURL("Microsoft.Web", "sites/cli-storage-site", azfAPIVersion)
	siteBody := `{"location":"eastus","kind":"functionapp","properties":{"serverFarmId":"/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Web/serverFarms/test"}}`
	runCLI(t, azRest("PUT", siteURL, siteBody))

	storageURL := armURL("Microsoft.Web", "sites/cli-storage-site/config/azurestorageaccounts", azfAPIVersion)
	storageBody := `{
		"properties": {
			"data": {
				"type": "AzureFiles",
				"accountName": "simstorage",
				"shareName": "data-share",
				"accessKey": "fake-key",
				"mountPath": "/mnt/data"
			}
		}
	}`
	out := runCLI(t, azRest("PUT", storageURL, storageBody))
	var resp struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			AccountName string `json:"accountName"`
			ShareName   string `json:"shareName"`
			MountPath   string `json:"mountPath"`
		} `json:"properties"`
	}
	parseJSON(t, out, &resp)
	require.Contains(t, resp.Properties, "data")
	assert.Equal(t, "AzureFiles", resp.Properties["data"].Type)
	assert.Equal(t, "data-share", resp.Properties["data"].ShareName)
	assert.Equal(t, "/mnt/data", resp.Properties["data"].MountPath)
}
