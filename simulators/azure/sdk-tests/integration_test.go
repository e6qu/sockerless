package azure_sdk_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_ACAJobLifecycle(t *testing.T) {
	rg := "int-aca-rg"
	jobName := "int-aca-job"

	// 1. Create resource group
	rgReq, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"?api-version=2023-07-01",
		strings.NewReader(`{"location":"eastus"}`))
	rgReq.Header.Set("Content-Type", "application/json")
	rgReq.Header.Set("Authorization", "Bearer fake-token")
	rgResp, err := http.DefaultClient.Do(rgReq)
	require.NoError(t, err)
	rgResp.Body.Close()

	// 2. Create job
	acaCreateJob(t, rg, jobName)

	// 3. Start execution
	execName := acaStartExecution(t, rg, jobName)

	// 4. Verify Running state immediately
	exec := acaGetExecution(t, rg, jobName, execName)
	assert.Equal(t, "Running", exec["status"])

	// 5. Query ContainerAppConsoleLogs_CL for start entry
	kql := `ContainerAppConsoleLogs_CL | where ContainerGroupName_s == "` + jobName + `"`
	result := queryWorkspace(t, "default", kql)
	require.Len(t, result.Tables, 1)
	require.GreaterOrEqual(t, len(result.Tables[0].Rows), 1, "should have start log entry")

	// 6. Stop execution
	stopReq, _ := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.App/jobs/"+jobName+"/executions/"+execName+"/stop?api-version=2024-03-01",
		strings.NewReader("{}"))
	stopReq.Header.Set("Content-Type", "application/json")
	stopReq.Header.Set("Authorization", "Bearer fake-token")
	stopResp, err := http.DefaultClient.Do(stopReq)
	require.NoError(t, err)
	stopResp.Body.Close()
	require.Equal(t, http.StatusOK, stopResp.StatusCode)

	// 7. Verify Stopped state
	exec = acaGetExecution(t, rg, jobName, execName)
	assert.Equal(t, "Stopped", exec["status"])
	assert.NotEmpty(t, exec["endTime"])

	// 8. Delete job
	delReq, _ := http.NewRequestWithContext(ctx, "DELETE",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.App/jobs/"+jobName+"?api-version=2024-03-01",
		nil)
	delReq.Header.Set("Authorization", "Bearer fake-token")
	delResp, err := http.DefaultClient.Do(delReq)
	require.NoError(t, err)
	delResp.Body.Close()
	require.Equal(t, http.StatusOK, delResp.StatusCode)

	// 9. Verify 404 after delete
	getReq, _ := http.NewRequestWithContext(ctx, "GET",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.App/jobs/"+jobName+"?api-version=2024-03-01",
		nil)
	getReq.Header.Set("Authorization", "Bearer fake-token")
	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	getResp.Body.Close()
	assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
}

func TestIntegration_AzureFunctionsLifecycle(t *testing.T) {
	rg := "int-func-rg"
	siteName := "int-func-app"

	// 1. Create resource group
	rgReq, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"?api-version=2023-07-01",
		strings.NewReader(`{"location":"eastus"}`))
	rgReq.Header.Set("Content-Type", "application/json")
	rgReq.Header.Set("Authorization", "Bearer fake-token")
	rgResp, err := http.DefaultClient.Do(rgReq)
	require.NoError(t, err)
	rgResp.Body.Close()

	// 2. Create function app
	site := map[string]any{
		"location": "eastus",
		"kind":     "functionapp",
		"properties": map[string]any{
			"serverFarmId": "/subscriptions/" + subscriptionID + "/resourceGroups/" + rg + "/providers/Microsoft.Web/serverFarms/plan",
		},
	}
	siteBody, _ := json.Marshal(site)
	siteReq, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.Web/sites/"+siteName+"?api-version=2023-12-01",
		strings.NewReader(string(siteBody)))
	siteReq.Header.Set("Content-Type", "application/json")
	siteReq.Header.Set("Authorization", "Bearer fake-token")
	siteResp, err := http.DefaultClient.Do(siteReq)
	require.NoError(t, err)
	defer siteResp.Body.Close()
	require.Equal(t, http.StatusOK, siteResp.StatusCode)

	// 3. Verify DefaultHostName
	var siteResult map[string]any
	data, _ := io.ReadAll(siteResp.Body)
	require.NoError(t, json.Unmarshal(data, &siteResult))
	props := siteResult["properties"].(map[string]any)
	defaultHostName := props["defaultHostName"].(string)
	require.NotEmpty(t, defaultHostName)

	// 4. Invoke function via DefaultHostName
	invokeURL := "http://" + defaultHostName + "/api/function"
	invokeReq, _ := http.NewRequestWithContext(ctx, "POST", invokeURL, strings.NewReader("{}"))
	invokeReq.Header.Set("Content-Type", "application/json")
	invokeResp, err := http.DefaultClient.Do(invokeReq)
	require.NoError(t, err)
	invokeResp.Body.Close()
	require.Equal(t, http.StatusOK, invokeResp.StatusCode)

	// 5. Query AppTraces — verify at least one "Function invoked" entry exists.
	// Note: the invoke handler matches sites by Host header; when multiple
	// function apps share the same simulator host, the matched site may vary,
	// so we query all AppTraces with Message filter rather than AppRoleName.
	time.Sleep(100 * time.Millisecond)
	kql := `AppTraces | where Message == "Function invoked"`
	result := queryWorkspace(t, "default", kql)
	require.Len(t, result.Tables, 1)
	require.GreaterOrEqual(t, len(result.Tables[0].Rows), 1, "should have at least one Function invoked AppTraces entry")

	// 6. Delete function app
	delReq, _ := http.NewRequestWithContext(ctx, "DELETE",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.Web/sites/"+siteName+"?api-version=2023-12-01",
		nil)
	delReq.Header.Set("Authorization", "Bearer fake-token")
	delResp, err := http.DefaultClient.Do(delReq)
	require.NoError(t, err)
	delResp.Body.Close()
	require.Equal(t, http.StatusOK, delResp.StatusCode)

	// 7. Verify 404 after delete
	getReq, _ := http.NewRequestWithContext(ctx, "GET",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.Web/sites/"+siteName+"?api-version=2023-12-01",
		nil)
	getReq.Header.Set("Authorization", "Bearer fake-token")
	getResp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	getResp.Body.Close()
	assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
}
