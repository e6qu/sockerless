package azure_cli_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerApps_CLI_ArithmeticEval(t *testing.T) {
	jobName := "cli-arith-aca-job"
	jobURL := acaURL("jobs/" + jobName)
	jobBody := fmt.Sprintf(`{
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
					"image": %q,
					"args": ["(3 + 4) * 2"]
				}]
			}
		}
	}`, evalImageName)
	runCLI(t, azRest("PUT", jobURL, jobBody))

	// Start execution
	rawStartURL := armURL("Microsoft.App", "jobs/"+jobName+"/start", acaAPIVersion)
	out := runCLI(t, azRest("POST", rawStartURL, ""))

	var startResult struct {
		Name string `json:"name"`
	}
	parseJSON(t, out, &startResult)
	require.NotEmpty(t, startResult.Name)

	// Wait for execution to complete
	time.Sleep(3 * time.Second)

	// GET execution status
	execURL := armURL("Microsoft.App", "jobs/"+jobName+"/executions/"+startResult.Name, acaAPIVersion)
	out = runCLI(t, azRest("GET", execURL, ""))

	var execResult struct {
		Properties struct {
			Status string `json:"status"`
		} `json:"properties"`
	}
	parseJSON(t, out, &execResult)
	assert.Equal(t, "Succeeded", execResult.Properties.Status)

	// Query Log Analytics for the output
	queryURL := baseURL + "/v1/workspaces/default/query"
	kqlBody := `{"query": "ContainerAppConsoleLogs_CL | where ContainerGroupName_s == \"` + jobName + `\""}`
	out = runCLI(t, azRest("POST", queryURL, kqlBody))
	assert.Contains(t, out, "14", "expected '14' in Log Analytics")

	// Cleanup
	runCLI(t, azRest("DELETE", jobURL, ""))
}

func TestContainerApps_CLI_ArithmeticInvalid(t *testing.T) {
	jobName := "cli-arith-aca-fail"
	jobURL := acaURL("jobs/" + jobName)
	jobBody := fmt.Sprintf(`{
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
					"image": %q,
					"args": ["3 +"]
				}]
			}
		}
	}`, evalImageName)
	runCLI(t, azRest("PUT", jobURL, jobBody))

	// Start execution
	rawStartURL := armURL("Microsoft.App", "jobs/"+jobName+"/start", acaAPIVersion)
	out := runCLI(t, azRest("POST", rawStartURL, ""))

	var startResult struct {
		Name string `json:"name"`
	}
	parseJSON(t, out, &startResult)
	require.NotEmpty(t, startResult.Name)

	// Wait for execution to complete
	time.Sleep(3 * time.Second)

	// GET execution status — should be Failed
	execURL := armURL("Microsoft.App", "jobs/"+jobName+"/executions/"+startResult.Name, acaAPIVersion)
	out = runCLI(t, azRest("GET", execURL, ""))

	var execResult struct {
		Properties struct {
			Status string `json:"status"`
		} `json:"properties"`
	}
	parseJSON(t, out, &execResult)
	assert.True(t, strings.Contains(execResult.Properties.Status, "Failed"),
		"expected status to be Failed, got: %s", execResult.Properties.Status)

	// Cleanup
	runCLI(t, azRest("DELETE", jobURL, ""))
}
