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
				"replicaTimeout": 1,
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
				"replicaTimeout": 1,
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

	// Wait for auto-completion (1s timeout + buffer)
	time.Sleep(2 * time.Second)

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

// acaCreateJob creates a Container Apps Job and returns its resource ID.
func acaCreateJob(t *testing.T, rg, jobName string) {
	t.Helper()

	// Ensure resource group exists
	rgBody := `{"location":"eastus"}`
	rgReq, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"?api-version=2023-07-01",
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
				"triggerType":    "Manual",
				"replicaTimeout": 1,
			},
			"template": map[string]any{
				"containers": []map[string]any{
					{"name": "worker", "image": "mcr.microsoft.com/test:latest"},
				},
			},
		},
	}
	body, _ := json.Marshal(job)
	req, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.App/jobs/"+jobName+"?api-version=2024-03-01",
		strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer fake-token")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Contains(t, []int{200, 201}, resp.StatusCode)
}

// acaStartExecution starts a job execution and returns the execution name from the response.
func acaStartExecution(t *testing.T, rg, jobName string) string {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.App/jobs/"+jobName+"/start?api-version=2024-03-01",
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer fake-token")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	var result map[string]any
	data, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(data, &result))
	return result["name"].(string)
}

// acaGetExecution returns the execution status.
func acaGetExecution(t *testing.T, rg, jobName, execName string) map[string]any {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, "GET",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.App/jobs/"+jobName+"/executions/"+execName+"?api-version=2024-03-01",
		nil)
	req.Header.Set("Authorization", "Bearer fake-token")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	data, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(data, &result))
	return result
}

func TestContainerApps_ExecutionRunningState(t *testing.T) {
	acaCreateJob(t, "status-rg", "running-job")
	execName := acaStartExecution(t, "status-rg", "running-job")

	// Immediately check — should be Running
	exec := acaGetExecution(t, "status-rg", "running-job", execName)
	assert.Equal(t, "Running", exec["status"])
	assert.NotEmpty(t, exec["startTime"])
}

func TestContainerApps_ExecutionSucceededState(t *testing.T) {
	acaCreateJob(t, "status-rg", "succeed-job")
	execName := acaStartExecution(t, "status-rg", "succeed-job")

	// Wait for auto-completion (1s timeout + buffer)
	time.Sleep(2 * time.Second)

	exec := acaGetExecution(t, "status-rg", "succeed-job", execName)
	assert.Equal(t, "Succeeded", exec["status"])
	assert.NotEmpty(t, exec["endTime"])
}

func TestContainerApps_ExecutionStoppedState(t *testing.T) {
	acaCreateJob(t, "status-rg", "stop-job")
	execName := acaStartExecution(t, "status-rg", "stop-job")

	// Stop the execution
	stopReq, _ := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/status-rg/providers/Microsoft.App/jobs/stop-job/executions/"+execName+"/stop?api-version=2024-03-01",
		strings.NewReader("{}"))
	stopReq.Header.Set("Content-Type", "application/json")
	stopReq.Header.Set("Authorization", "Bearer fake-token")
	stopResp, err := http.DefaultClient.Do(stopReq)
	require.NoError(t, err)
	stopResp.Body.Close()
	require.Equal(t, http.StatusOK, stopResp.StatusCode)

	exec := acaGetExecution(t, "status-rg", "stop-job", execName)
	assert.Equal(t, "Stopped", exec["status"])
	assert.NotEmpty(t, exec["endTime"])
}

// acaCreateJobWithCommand creates a Container Apps Job with a command.
func acaCreateJobWithCommand(t *testing.T, rg, jobName string, cmd []string) {
	t.Helper()

	// Ensure resource group exists
	rgBody := `{"location":"eastus"}`
	rgReq, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"?api-version=2023-07-01",
		strings.NewReader(rgBody))
	rgReq.Header.Set("Content-Type", "application/json")
	rgReq.Header.Set("Authorization", "Bearer fake-token")
	rgResp, err := http.DefaultClient.Do(rgReq)
	require.NoError(t, err)
	rgResp.Body.Close()

	container := map[string]any{
		"name":    "worker",
		"image":   "mcr.microsoft.com/test:latest",
		"command": cmd,
	}
	job := map[string]any{
		"location": "eastus",
		"properties": map[string]any{
			"configuration": map[string]any{
				"triggerType":    "Manual",
				"replicaTimeout": 5,
			},
			"template": map[string]any{
				"containers": []map[string]any{container},
			},
		},
	}
	body, _ := json.Marshal(job)
	req, _ := http.NewRequestWithContext(ctx, "PUT",
		baseURL+"/subscriptions/"+subscriptionID+"/resourceGroups/"+rg+"/providers/Microsoft.App/jobs/"+jobName+"?api-version=2024-03-01",
		strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer fake-token")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Contains(t, []int{200, 201}, resp.StatusCode)
}

func TestContainerApps_ExecutionRunsCommand(t *testing.T) {
	acaCreateJobWithCommand(t, "exec-rg", "exec-cmd-job", []string{"echo", "hello"})
	execName := acaStartExecution(t, "exec-rg", "exec-cmd-job")

	// Wait for process to complete
	time.Sleep(2 * time.Second)

	exec := acaGetExecution(t, "exec-rg", "exec-cmd-job", execName)
	assert.Equal(t, "Succeeded", exec["status"])
	assert.NotEmpty(t, exec["endTime"])
}

func TestContainerApps_ExecutionFailedStatus(t *testing.T) {
	acaCreateJobWithCommand(t, "exec-rg", "exec-fail-job", []string{"sh", "-c", "exit 1"})
	execName := acaStartExecution(t, "exec-rg", "exec-fail-job")

	// Wait for process to complete
	time.Sleep(2 * time.Second)

	exec := acaGetExecution(t, "exec-rg", "exec-fail-job", execName)
	assert.Equal(t, "Failed", exec["status"])
	assert.NotEmpty(t, exec["endTime"])
}

func TestContainerApps_ExecutionLogsRealOutput(t *testing.T) {
	acaCreateJobWithCommand(t, "exec-rg", "exec-log-job", []string{"echo", "real aca output"})
	_ = acaStartExecution(t, "exec-rg", "exec-log-job")

	// Wait for process to complete and logs to be written
	time.Sleep(2 * time.Second)

	// Query logs via KQL
	kql := `ContainerAppConsoleLogs_CL | where ContainerGroupName_s == "exec-log-job"`
	result := queryWorkspace(t, "default", kql)

	require.Len(t, result.Tables, 1)
	table := result.Tables[0]

	// Find Log_s column index
	logIdx := -1
	for i, col := range table.Columns {
		if col.Name == "Log_s" {
			logIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, logIdx, 0)

	// Should have at least "Container started" and "real aca output"
	var logs []string
	for _, row := range table.Rows {
		logs = append(logs, row[logIdx].(string))
	}
	assert.Contains(t, logs, "real aca output", "process stdout should appear in Log Analytics")
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
