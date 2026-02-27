package azure_sdk_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAzureFunctions_InvokeInjectsLogEntries(t *testing.T) {
	// Ensure resource group exists
	rgBody := `{"location":"eastus"}`
	rgReq, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/func-log-rg?api-version=2023-07-01",
		strings.NewReader(rgBody))
	rgReq.Header.Set("Content-Type", "application/json")
	rgReq.Header.Set("Authorization", "Bearer fake-token")
	rgResp, err := http.DefaultClient.Do(rgReq)
	require.NoError(t, err)
	rgResp.Body.Close()

	// Create function app
	site := map[string]any{
		"location": "eastus",
		"kind":     "functionapp",
		"properties": map[string]any{
			"serverFarmId": "/subscriptions/" + subscriptionID + "/resourceGroups/func-log-rg/providers/Microsoft.Web/serverFarms/test-plan",
		},
	}
	siteBody, _ := json.Marshal(site)
	siteReq, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/func-log-rg/providers/Microsoft.Web/sites/log-func-app?api-version=2023-12-01",
		strings.NewReader(string(siteBody)))
	siteReq.Header.Set("Content-Type", "application/json")
	siteReq.Header.Set("Authorization", "Bearer fake-token")
	siteResp, err := http.DefaultClient.Do(siteReq)
	require.NoError(t, err)
	siteResp.Body.Close()
	require.Equal(t, http.StatusOK, siteResp.StatusCode)

	// Invoke the function via POST /api/function
	invokeReq, _ := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/api/function",
		strings.NewReader("{}"))
	invokeReq.Header.Set("Content-Type", "application/json")
	invokeReq.Host = baseURL[len("http://"):] // strip scheme to match DefaultHostName
	invokeResp, err := http.DefaultClient.Do(invokeReq)
	require.NoError(t, err)
	invokeResp.Body.Close()
	require.Equal(t, http.StatusOK, invokeResp.StatusCode)

	// Query AppTraces for the function app
	kql := `AppTraces | where AppRoleName == "log-func-app"`
	result := queryWorkspace(t, "default", kql)

	require.Len(t, result.Tables, 1)
	table := result.Tables[0]
	require.GreaterOrEqual(t, len(table.Rows), 1, "should have at least one log entry from invocation")

	// Find Message column index
	msgIdx := -1
	roleIdx := -1
	for i, col := range table.Columns {
		if col.Name == "Message" {
			msgIdx = i
		}
		if col.Name == "AppRoleName" {
			roleIdx = i
		}
	}
	require.GreaterOrEqual(t, msgIdx, 0, "Message column not found")
	require.GreaterOrEqual(t, roleIdx, 0, "AppRoleName column not found")

	// Verify the invocation log entry
	lastRow := table.Rows[len(table.Rows)-1]
	assert.Equal(t, "Function invoked", lastRow[msgIdx])
	assert.Equal(t, "log-func-app", lastRow[roleIdx])
}
