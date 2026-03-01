package azure_cli_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const acaAPIVersion = "2023-05-01"

func acaURL(path string) string {
	return armURL("Microsoft.App", path, acaAPIVersion)
}

func TestContainerApps_CLI_StartAndCheckLogs(t *testing.T) {
	// Create a Container Apps Job with echo command
	jobURL := acaURL("jobs/cli-aca-job")
	jobBody := `{
		"location": "eastus",
		"properties": {
			"environmentId": "",
			"configuration": {
				"replicaTimeout": 30,
				"triggerType": "Manual",
				"manualTriggerConfig": { "parallelism": 1, "replicaCompletionCount": 1 }
			},
			"template": {
				"containers": [{
					"name": "app",
					"image": "alpine:latest",
					"command": ["echo", "hello-from-aca"]
				}]
			}
		}
	}`
	runCLI(t, azRest("PUT", jobURL, jobBody))

	// Start execution
	rawStartURL := armURL("Microsoft.App", "jobs/cli-aca-job/start", acaAPIVersion)
	out := runCLI(t, azRest("POST", rawStartURL, ""))

	var startResult struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	}
	parseJSON(t, out, &startResult)
	require.NotEmpty(t, startResult.Name)

	// Wait for execution to complete
	time.Sleep(3 * time.Second)

	// GET execution status
	execURL := armURL("Microsoft.App", "jobs/cli-aca-job/executions/"+startResult.Name, acaAPIVersion)
	out = runCLI(t, azRest("GET", execURL, ""))

	var execResult struct {
		Status string `json:"status"`
	}
	parseJSON(t, out, &execResult)
	assert.Equal(t, "Succeeded", execResult.Status)

	// Query Log Analytics for the output
	queryURL := baseURL + "/v1/workspaces/default/query"
	kqlBody := `{"query": "ContainerAppConsoleLogs_CL | where ContainerGroupName_s == \"cli-aca-job\""}`
	out = runCLI(t, azRest("POST", queryURL, kqlBody))

	// Verify real output in logs
	assert.Contains(t, out, "hello-from-aca", "expected real process output in Log Analytics")

	// Cleanup
	runCLI(t, azRest("DELETE", jobURL, ""))
}

func TestContainerApps_CLI_StartFailure(t *testing.T) {
	jobURL := acaURL("jobs/cli-aca-fail-job")
	jobBody := `{
		"location": "eastus",
		"properties": {
			"environmentId": "",
			"configuration": {
				"replicaTimeout": 30,
				"triggerType": "Manual",
				"manualTriggerConfig": { "parallelism": 1, "replicaCompletionCount": 1 }
			},
			"template": {
				"containers": [{
					"name": "app",
					"image": "alpine:latest",
					"command": ["sh", "-c", "exit 1"]
				}]
			}
		}
	}`
	runCLI(t, azRest("PUT", jobURL, jobBody))

	// Start execution
	rawStartURL := armURL("Microsoft.App", "jobs/cli-aca-fail-job/start", acaAPIVersion)
	out := runCLI(t, azRest("POST", rawStartURL, ""))

	var startResult struct {
		Name string `json:"name"`
	}
	parseJSON(t, out, &startResult)
	require.NotEmpty(t, startResult.Name)

	// Wait for execution to complete
	time.Sleep(3 * time.Second)

	// GET execution status — should be Failed
	execURL := armURL("Microsoft.App", "jobs/cli-aca-fail-job/executions/"+startResult.Name, acaAPIVersion)
	out = runCLI(t, azRest("GET", execURL, ""))

	var execResult struct {
		Status string `json:"status"`
	}
	parseJSON(t, out, &execResult)

	// Accept either "Failed" or check that it's not "Running"/"Succeeded"
	assert.True(t, strings.Contains(execResult.Status, "Failed"),
		"expected status to be Failed, got: %s", execResult.Status)

	// Cleanup
	runCLI(t, azRest("DELETE", jobURL, ""))
}
