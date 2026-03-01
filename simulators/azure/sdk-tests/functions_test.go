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

// azureDeleteSite deletes a site, ignoring errors.
func azureDeleteSite(rg, name string) {
	url := baseURL + "/subscriptions/" + subscriptionID + "/resourceGroups/" + rg + "/providers/Microsoft.Web/sites/" + name + "?api-version=2023-12-01"
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("Authorization", "Bearer fake-token")
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// azureCreateSite creates a resource group and function app, optionally with SimCommand.
func azureCreateSite(t *testing.T, rg, name string, simCommand []string) {
	t.Helper()
	rgBody := `{"location":"eastus"}`
	rgReq, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"?api-version=2023-07-01",
		strings.NewReader(rgBody))
	rgReq.Header.Set("Content-Type", "application/json")
	rgReq.Header.Set("Authorization", "Bearer fake-token")
	rgResp, err := http.DefaultClient.Do(rgReq)
	require.NoError(t, err)
	rgResp.Body.Close()

	props := map[string]any{
		"serverFarmId": "/subscriptions/" + subscriptionID + "/resourceGroups/" + rg + "/providers/Microsoft.Web/serverFarms/test-plan",
	}
	if len(simCommand) > 0 {
		props["siteConfig"] = map[string]any{"simCommand": simCommand}
	}
	site := map[string]any{
		"location":   "eastus",
		"kind":       "functionapp",
		"properties": props,
	}
	siteBody, _ := json.Marshal(site)
	siteReq, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.Web/sites/"+name+"?api-version=2023-12-01",
		strings.NewReader(string(siteBody)))
	siteReq.Header.Set("Content-Type", "application/json")
	siteReq.Header.Set("Authorization", "Bearer fake-token")
	siteResp, err := http.DefaultClient.Do(siteReq)
	require.NoError(t, err)
	siteResp.Body.Close()
	require.Equal(t, http.StatusOK, siteResp.StatusCode)
}

// azureInvokeFunction invokes POST /api/function and returns the response body.
func azureInvokeFunction(t *testing.T) []byte {
	t.Helper()
	invokeReq, _ := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/api/function",
		strings.NewReader("{}"))
	invokeReq.Header.Set("Content-Type", "application/json")
	invokeReq.Host = baseURL[len("http://"):]
	invokeResp, err := http.DefaultClient.Do(invokeReq)
	require.NoError(t, err)
	defer invokeResp.Body.Close()
	require.Equal(t, http.StatusOK, invokeResp.StatusCode)
	body, _ := io.ReadAll(invokeResp.Body)
	return body
}

func TestAzureFunctions_InvokeInjectsLogEntries(t *testing.T) {
	rg, name := "func-log-rg", "log-func-app"
	azureCreateSite(t, rg, name, nil)
	defer azureDeleteSite(rg, name)

	azureInvokeFunction(t)

	kql := `AppTraces | where AppRoleName == "log-func-app"`
	result := queryWorkspace(t, "default", kql)

	require.Len(t, result.Tables, 1)
	table := result.Tables[0]
	require.GreaterOrEqual(t, len(table.Rows), 1, "should have at least one log entry from invocation")

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

	lastRow := table.Rows[len(table.Rows)-1]
	assert.Equal(t, "Function invoked", lastRow[msgIdx])
	assert.Equal(t, "log-func-app", lastRow[roleIdx])
}

func TestAzureFunctions_InvokeExecutesCommand(t *testing.T) {
	rg, name := "func-exec-rg", "exec-func-app"
	azureCreateSite(t, rg, name, []string{"echo", "hello-from-azure"})
	defer azureDeleteSite(rg, name)

	respBody := azureInvokeFunction(t)
	assert.Contains(t, string(respBody), "hello-from-azure")
}

func TestAzureFunctions_InvokeNonZeroExit(t *testing.T) {
	rg, name := "func-fail-rg", "fail-func-app"
	azureCreateSite(t, rg, name, []string{"sh", "-c", "exit 1"})
	defer azureDeleteSite(rg, name)

	azureInvokeFunction(t)

	kql := `AppTraces | where AppRoleName == "fail-func-app"`
	result := queryWorkspace(t, "default", kql)

	require.Len(t, result.Tables, 1)
	table := result.Tables[0]
	require.GreaterOrEqual(t, len(table.Rows), 1, "should have log entries from execution")

	msgIdx := -1
	for i, col := range table.Columns {
		if col.Name == "Message" {
			msgIdx = i
		}
	}
	require.GreaterOrEqual(t, msgIdx, 0, "Message column not found")

	found := false
	for _, row := range table.Rows {
		msg, ok := row[msgIdx].(string)
		if ok && strings.Contains(msg, "error") && strings.Contains(msg, "exit") {
			found = true
		}
	}
	assert.True(t, found, "expected error log entry about non-zero exit")
}

func TestAzureFunctions_InvokeLogsRealOutput(t *testing.T) {
	rg, name := "func-out-rg", "out-func-app"
	azureCreateSite(t, rg, name, []string{"echo", "real-azure-output"})
	defer azureDeleteSite(rg, name)

	azureInvokeFunction(t)

	kql := `AppTraces | where AppRoleName == "out-func-app"`
	result := queryWorkspace(t, "default", kql)

	require.Len(t, result.Tables, 1)
	table := result.Tables[0]
	require.GreaterOrEqual(t, len(table.Rows), 1, "should have log entries from execution")

	msgIdx := -1
	for i, col := range table.Columns {
		if col.Name == "Message" {
			msgIdx = i
		}
	}
	require.GreaterOrEqual(t, msgIdx, 0, "Message column not found")

	found := false
	for _, row := range table.Rows {
		msg, ok := row[msgIdx].(string)
		if ok && msg == "real-azure-output" {
			found = true
		}
	}
	assert.True(t, found, "process stdout should appear in AppTraces")
}

func TestAzureFunctions_DefaultHostNameReachability(t *testing.T) {
	rg, name := "func-host-rg", "host-func-app"
	azureCreateSite(t, rg, name, nil)
	defer azureDeleteSite(rg, name)

	// Get function app to extract DefaultHostName
	getReq, _ := http.NewRequestWithContext(ctx, "GET",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.Web/sites/"+name+"?api-version=2023-12-01",
		nil)
	getReq.Header.Set("Authorization", "Bearer fake-token")
	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer getResp.Body.Close()
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var result map[string]any
	data, _ := io.ReadAll(getResp.Body)
	require.NoError(t, json.Unmarshal(data, &result))
	props := result["properties"].(map[string]any)
	defaultHostName := props["defaultHostName"].(string)
	require.NotEmpty(t, defaultHostName, "DefaultHostName should be set")

	invokeURL := "http://" + defaultHostName + "/api/function"
	invokeReq, _ := http.NewRequestWithContext(ctx, "POST", invokeURL, strings.NewReader("{}"))
	invokeReq.Header.Set("Content-Type", "application/json")
	invokeResp, err := http.DefaultClient.Do(invokeReq)
	require.NoError(t, err)
	defer invokeResp.Body.Close()

	assert.Equal(t, http.StatusOK, invokeResp.StatusCode)
}
