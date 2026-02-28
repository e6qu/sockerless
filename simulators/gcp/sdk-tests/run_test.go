package gcp_sdk_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/logging/logadmin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iterator"
)

// Cloud Run Jobs v2 uses REST API.
// The cloud.google.com/go/run package uses gRPC by default,
// so we use direct HTTP calls against the REST API.

func TestCloudRun_CreateJob(t *testing.T) {
	job := map[string]any{
		"template": map[string]any{
			"template": map[string]any{
				"containers": []map[string]any{
					{"image": "gcr.io/test/worker:latest"},
				},
			},
		},
	}
	body, _ := json.Marshal(job)

	req, _ := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/v2/projects/test-project/locations/us-central1/jobs?jobId=test-job",
		strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	data, _ := io.ReadAll(resp.Body)
	json.Unmarshal(data, &result)
	// Response is an LRO; the job is in the "response" field
	assert.True(t, result["done"].(bool), "LRO should be done")
	if response, ok := result["response"].(map[string]any); ok {
		assert.Contains(t, response["name"], "test-job")
	}
}

func TestCloudRun_GetJob(t *testing.T) {
	// Create job first
	job := map[string]any{
		"template": map[string]any{
			"template": map[string]any{
				"containers": []map[string]any{
					{"image": "gcr.io/test/app:latest"},
				},
			},
		},
	}
	body, _ := json.Marshal(job)
	createReq, _ := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/v2/projects/test-project/locations/us-central1/jobs?jobId=get-job",
		strings.NewReader(string(body)))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	createResp.Body.Close()

	// Get job
	getReq, _ := http.NewRequestWithContext(ctx, "GET",
		baseURL+"/v2/projects/test-project/locations/us-central1/jobs/get-job", nil)
	resp, err := http.DefaultClient.Do(getReq)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	data, _ := io.ReadAll(resp.Body)
	json.Unmarshal(data, &result)
	assert.Contains(t, result["name"], "get-job")
}

func TestCloudRun_ListJobs(t *testing.T) {
	req, _ := http.NewRequestWithContext(ctx, "GET",
		baseURL+"/v2/projects/test-project/locations/us-central1/jobs", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestCloudRun_DeleteJob(t *testing.T) {
	// Create job
	job := map[string]any{
		"template": map[string]any{
			"template": map[string]any{
				"containers": []map[string]any{
					{"image": "gcr.io/test/temp:latest"},
				},
			},
		},
	}
	body, _ := json.Marshal(job)
	createReq, _ := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/v2/projects/test-project/locations/us-central1/jobs?jobId=del-job",
		strings.NewReader(string(body)))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	createResp.Body.Close()

	// Delete
	delReq, _ := http.NewRequestWithContext(ctx, "DELETE",
		baseURL+"/v2/projects/test-project/locations/us-central1/jobs/del-job", nil)
	resp, err := http.DefaultClient.Do(delReq)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestCloudRun_RunJobInjectsLogEntries(t *testing.T) {
	// Create a job with a unique name for this test
	job := map[string]any{
		"template": map[string]any{
			"template": map[string]any{
				"timeout": "1s",
				"containers": []map[string]any{
					{"image": "gcr.io/test/logtest:latest"},
				},
			},
		},
	}
	body, _ := json.Marshal(job)
	createReq, _ := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/v2/projects/test-project/locations/us-central1/jobs?jobId=log-inject-job",
		strings.NewReader(string(body)))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	createResp.Body.Close()

	// Run the job
	runReq, _ := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/v2/projects/test-project/locations/us-central1/jobs/log-inject-job:run",
		strings.NewReader("{}"))
	runReq.Header.Set("Content-Type", "application/json")
	runResp, err := http.DefaultClient.Do(runReq)
	require.NoError(t, err)
	runResp.Body.Close()
	require.Equal(t, http.StatusOK, runResp.StatusCode)

	// Wait for execution to complete (1s timeout + buffer)
	time.Sleep(2 * time.Second)

	// Query log entries using logadmin with the same filter the backend uses
	client := logadminClient(t)
	filter := `resource.type="cloud_run_job" AND resource.labels.job_name="log-inject-job"`
	it := client.Entries(ctx, logadmin.Filter(filter))

	var messages []string
	for {
		entry, err := it.Next()
		if err == iterator.Done {
			break
		}
		require.NoError(t, err)

		// Verify resource type and label
		assert.Equal(t, "cloud_run_job", entry.Resource.Type)
		assert.Equal(t, "log-inject-job", entry.Resource.Labels["job_name"])

		if entry.Payload != nil {
			if s, ok := entry.Payload.(string); ok {
				messages = append(messages, s)
			}
		}
	}

	require.GreaterOrEqual(t, len(messages), 2, "should have at least start and completion log entries")
	assert.Equal(t, "Container started", messages[0])
	assert.Equal(t, "Execution completed successfully", messages[1])
}

// createAndRunJob creates a job and runs it, returning the execution name from the LRO response.
func createAndRunJob(t *testing.T, jobID string) string {
	t.Helper()
	job := map[string]any{
		"template": map[string]any{
			"template": map[string]any{
				"timeout": "1s",
				"containers": []map[string]any{
					{"image": "gcr.io/test/status:latest"},
				},
			},
		},
	}
	body, _ := json.Marshal(job)
	createReq, _ := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/v2/projects/test-project/locations/us-central1/jobs?jobId="+jobID,
		strings.NewReader(string(body)))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	createResp.Body.Close()

	runReq, _ := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/v2/projects/test-project/locations/us-central1/jobs/"+jobID+":run",
		strings.NewReader("{}"))
	runReq.Header.Set("Content-Type", "application/json")
	runResp, err := http.DefaultClient.Do(runReq)
	require.NoError(t, err)
	defer runResp.Body.Close()
	require.Equal(t, http.StatusOK, runResp.StatusCode)

	var lro map[string]any
	data, _ := io.ReadAll(runResp.Body)
	require.NoError(t, json.Unmarshal(data, &lro))
	response := lro["response"].(map[string]any)
	return response["name"].(string)
}

// getExecution fetches an execution and returns it as a map.
func getExecution(t *testing.T, execName string) map[string]any {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/v2/"+execName, nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	data, _ := io.ReadAll(resp.Body)
	require.NoError(t, json.Unmarshal(data, &result))
	return result
}

func TestCloudRun_ExecutionRunningState(t *testing.T) {
	execName := createAndRunJob(t, "status-running-job")

	// Immediately check — should be running
	exec := getExecution(t, execName)
	assert.Equal(t, float64(1), exec["runningCount"])
	assert.Equal(t, float64(0), exec["succeededCount"])
	assert.Equal(t, float64(0), exec["failedCount"])
	assert.Empty(t, exec["completionTime"])
}

func TestCloudRun_ExecutionSucceededState(t *testing.T) {
	execName := createAndRunJob(t, "status-succeeded-job")

	// Wait for auto-completion (1s timeout + buffer)
	time.Sleep(2 * time.Second)

	exec := getExecution(t, execName)
	assert.Equal(t, float64(0), exec["runningCount"])
	assert.Equal(t, float64(1), exec["succeededCount"])
	assert.Equal(t, float64(0), exec["failedCount"])
	assert.NotEmpty(t, exec["completionTime"])
}

func TestCloudRun_ExecutionCancelledState(t *testing.T) {
	execName := createAndRunJob(t, "status-cancel-job")

	// Cancel immediately
	parts := strings.SplitN(execName, "/executions/", 2)
	cancelURL := baseURL + "/v2/" + parts[0] + "/executions/" + parts[1] + ":cancel"
	cancelReq, _ := http.NewRequestWithContext(ctx, "POST", cancelURL, strings.NewReader("{}"))
	cancelReq.Header.Set("Content-Type", "application/json")
	cancelResp, err := http.DefaultClient.Do(cancelReq)
	require.NoError(t, err)
	cancelResp.Body.Close()
	require.Equal(t, http.StatusOK, cancelResp.StatusCode)

	exec := getExecution(t, execName)
	assert.Equal(t, float64(0), exec["runningCount"])
	assert.Equal(t, float64(1), exec["cancelledCount"])
	assert.Equal(t, float64(0), exec["failedCount"])
	assert.NotEmpty(t, exec["completionTime"])
}
