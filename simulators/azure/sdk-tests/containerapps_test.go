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

// Container Apps Jobs use the Microsoft.App provider.
// The armappcontainers SDK requires specific API versions that may differ.
// Using direct HTTP calls for more reliable testing against the simulator.

func TestContainerApps_CreateJob(t *testing.T) {
	// Ensure resource group exists
	rgBody := `{"location":"eastus"}`
	rgReq, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/aca-rg?api-version=2023-07-01",
		strings.NewReader(rgBody))
	rgReq.Header.Set("Content-Type", "application/json")
	rgReq.Header.Set("Authorization", "Bearer fake-token")
	rgResp, err := http.DefaultClient.Do(rgReq)
	require.NoError(t, err)
	rgResp.Body.Close()

	job := map[string]any{
		"location": "eastus",
		"properties": map[string]any{
			"configuration": map[string]any{
				"triggerType":  "Manual",
				"replicaTimeout": 300,
			},
			"template": map[string]any{
				"containers": []map[string]any{
					{
						"name":  "worker",
						"image": "mcr.microsoft.com/azuredocs/containerapps-helloworld:latest",
					},
				},
			},
		},
	}
	body, _ := json.Marshal(job)

	req, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/aca-rg/providers/Microsoft.App/jobs/test-job?api-version=2024-03-01",
		strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer fake-token")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Contains(t, []int{200, 201}, resp.StatusCode)

	var result map[string]any
	data, _ := io.ReadAll(resp.Body)
	json.Unmarshal(data, &result)
	assert.Equal(t, "test-job", result["name"])
}

func TestContainerApps_StartJobInjectsLogs(t *testing.T) {
	// Ensure resource group exists
	rgBody := `{"location":"eastus"}`
	rgReq, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/aca-log-rg?api-version=2023-07-01",
		strings.NewReader(rgBody))
	rgReq.Header.Set("Content-Type", "application/json")
	rgReq.Header.Set("Authorization", "Bearer fake-token")
	rgResp, err := http.DefaultClient.Do(rgReq)
	require.NoError(t, err)
	rgResp.Body.Close()

	// Create job
	job := map[string]any{
		"location": "eastus",
		"properties": map[string]any{
			"configuration": map[string]any{
				"triggerType":    "Manual",
				"replicaTimeout": 300,
			},
			"template": map[string]any{
				"containers": []map[string]any{
					{"name": "worker", "image": "mcr.microsoft.com/test:latest"},
				},
			},
		},
	}
	body, _ := json.Marshal(job)
	createReq, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/aca-log-rg/providers/Microsoft.App/jobs/log-test-job?api-version=2024-03-01",
		strings.NewReader(string(body)))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer fake-token")
	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	createResp.Body.Close()

	// Start execution
	startReq, _ := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/aca-log-rg/providers/Microsoft.App/jobs/log-test-job/start?api-version=2024-03-01",
		strings.NewReader("{}"))
	startReq.Header.Set("Content-Type", "application/json")
	startReq.Header.Set("Authorization", "Bearer fake-token")
	startResp, err := http.DefaultClient.Do(startReq)
	require.NoError(t, err)
	startResp.Body.Close()
	require.Equal(t, http.StatusAccepted, startResp.StatusCode)

	// Wait for auto-completion (3s + buffer)
	time.Sleep(4 * time.Second)

	// Query logs via KQL
	kql := `ContainerAppConsoleLogs_CL | where ContainerGroupName_s == "log-test-job"`
	result := queryWorkspace(t, "default", kql)

	require.Len(t, result.Tables, 1)
	table := result.Tables[0]
	require.GreaterOrEqual(t, len(table.Rows), 2, "should have at least start and completion entries")

	// Find Log_s column index
	logIdx := -1
	for i, col := range table.Columns {
		if col.Name == "Log_s" {
			logIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, logIdx, 0)

	assert.Equal(t, "Container started", table.Rows[0][logIdx])
	assert.Equal(t, "Execution completed successfully", table.Rows[1][logIdx])
}

func TestContainerApps_GetJob(t *testing.T) {
	req, _ := http.NewRequestWithContext(ctx, "GET",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/aca-rg/providers/Microsoft.App/jobs/test-job?api-version=2024-03-01",
		nil)
	req.Header.Set("Authorization", "Bearer fake-token")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	data, _ := io.ReadAll(resp.Body)
	json.Unmarshal(data, &result)
	assert.Equal(t, "test-job", result["name"])
}
