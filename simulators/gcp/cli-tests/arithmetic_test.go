package gcp_cli_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloudRun_CLI_ArithmeticEval(t *testing.T) {
	jobID := "cli-arith-crj"
	createBody := fmt.Sprintf(`{
		"template": {
			"taskCount": 1,
			"template": {
				"containers": [{
					"name": "app",
					"image": %q,
					"args": ["(3 + 4) * 2"]
				}],
				"maxRetries": 0,
				"timeout": "10s"
			}
		}
	}`, evalImageName)
	httpDoJSON(t, "POST", jobsBaseURL()+"?jobId="+jobID, createBody)

	// Run the job
	out := httpDoJSON(t, "POST", jobURL(jobID+":run"), "")

	var lro struct {
		Response struct {
			Name string `json:"name"`
		} `json:"response"`
	}
	parseJSON(t, out, &lro)
	require.NotEmpty(t, lro.Response.Name)

	// Wait for execution to complete
	time.Sleep(3 * time.Second)

	// Check execution status
	out = httpDoJSON(t, "GET", baseURL+"/v2/"+lro.Response.Name, "")
	var exec struct {
		SucceededCount int `json:"succeededCount"`
		FailedCount    int `json:"failedCount"`
	}
	parseJSON(t, out, &exec)
	assert.Equal(t, 1, exec.SucceededCount, "expected job to succeed")
	assert.Equal(t, 0, exec.FailedCount)

	// Query Cloud Logging for job output
	out = runCLI(t, gcloudCLI("logging", "read",
		`resource.type="cloud_run_job" AND resource.labels.job_name="`+jobID+`"`,
		"--format", "json",
	))
	assert.Contains(t, out, "14", "expected '14' in Cloud Logging")

	// Cleanup
	httpDoJSON(t, "DELETE", jobURL(jobID), "")
}

func TestCloudRun_CLI_ArithmeticInvalid(t *testing.T) {
	jobID := "cli-arith-crj-fail"
	createBody := fmt.Sprintf(`{
		"template": {
			"taskCount": 1,
			"template": {
				"containers": [{
					"name": "app",
					"image": %q,
					"args": ["3 +"]
				}],
				"maxRetries": 0,
				"timeout": "10s"
			}
		}
	}`, evalImageName)
	httpDoJSON(t, "POST", jobsBaseURL()+"?jobId="+jobID, createBody)

	// Run the job
	out := httpDoJSON(t, "POST", jobURL(jobID+":run"), "")

	var lro struct {
		Response struct {
			Name string `json:"name"`
		} `json:"response"`
	}
	parseJSON(t, out, &lro)
	require.NotEmpty(t, lro.Response.Name)

	// Wait for execution to complete
	time.Sleep(3 * time.Second)

	// Check execution status — should be failed
	out = httpDoJSON(t, "GET", baseURL+"/v2/"+lro.Response.Name, "")
	var exec struct {
		SucceededCount int `json:"succeededCount"`
		FailedCount    int `json:"failedCount"`
	}
	parseJSON(t, out, &exec)
	assert.Equal(t, 0, exec.SucceededCount)
	assert.Equal(t, 1, exec.FailedCount, "expected job to fail")

	// Cleanup
	httpDoJSON(t, "DELETE", jobURL(jobID), "")
}
