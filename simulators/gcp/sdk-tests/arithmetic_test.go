package gcp_sdk_test

import (
	"encoding/json"
	"fmt"
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

// --- Cloud Functions FaaS tests ---

func gcpCreateFunction(t *testing.T, fnID string, simCommand []string) {
	t.Helper()
	fn := map[string]any{
		"buildConfig": map[string]any{
			"runtime":    "go121",
			"entryPoint": "Handler",
		},
	}
	if len(simCommand) > 0 {
		// Workloads dispatch through Docker — never os/exec on the sim
		// host. Tests carry the image alongside the command.
		// evalImageName is built in TestMain and contains eval-arithmetic
		// at /usr/local/bin/eval-arithmetic as ENTRYPOINT.
		fn["serviceConfig"] = map[string]any{
			"simImage":   evalImageName,
			"simCommand": simCommand,
		}
	}
	body, _ := json.Marshal(fn)
	req, _ := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/v2/projects/test-project/locations/us-central1/functions?functionId="+fnID,
		strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func gcpInvokeFunction(t *testing.T, fnID string) string {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/v2-functions-invoke/"+fnID,
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	data, _ := io.ReadAll(resp.Body)
	return string(data)
}

func gcpInvokeFunctionExpectError(t *testing.T, fnID string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, "POST",
		baseURL+"/v2-functions-invoke/"+fnID,
		strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func gcpFunctionLogMessages(t *testing.T, fnID string) []string {
	t.Helper()
	client := logadminClient(t)
	filter := `resource.type="cloud_run_revision" AND resource.labels.service_name="` + fnID + `"`
	it := client.Entries(ctx, logadmin.Filter(filter))
	var messages []string
	for {
		entry, err := it.Next()
		if err == iterator.Done {
			break
		}
		require.NoError(t, err)
		if s, ok := entry.Payload.(string); ok {
			messages = append(messages, s)
		}
	}
	return messages
}

func TestCloudFunctions_InvokeArithmetic(t *testing.T) {
	fnID := "arith-basic-cf"
	gcpCreateFunction(t, fnID, []string{"3 + 4 * 2"})
	body := gcpInvokeFunction(t, fnID)
	assert.Contains(t, body, "11")
}

func TestCloudFunctions_InvokeArithmeticParentheses(t *testing.T) {
	fnID := "arith-paren-cf"
	gcpCreateFunction(t, fnID, []string{"(3 + 4) * 2"})
	body := gcpInvokeFunction(t, fnID)
	assert.Contains(t, body, "14")
}

func TestCloudFunctions_InvokeArithmeticInvalid(t *testing.T) {
	fnID := "arith-invalid-cf"
	gcpCreateFunction(t, fnID, []string{"3 +"})
	gcpInvokeFunctionExpectError(t, fnID)

	messages := gcpFunctionLogMessages(t, fnID)
	found := false
	for _, m := range messages {
		if strings.Contains(m, "error") && strings.Contains(m, "exit") {
			found = true
		}
	}
	assert.True(t, found, "expected error log entry for invalid expression, got: %v", messages)
}

func TestCloudFunctions_InvokeArithmeticLogs(t *testing.T) {
	fnID := "arith-logs-cf"
	gcpCreateFunction(t, fnID, []string{"((2+3)*4-1)/3"})
	body := gcpInvokeFunction(t, fnID)
	assert.Contains(t, body, "6.333")

	messages := gcpFunctionLogMessages(t, fnID)
	allLogs := strings.Join(messages, "\n")
	assert.Contains(t, allLogs, "Parsing expression:")
	assert.Contains(t, allLogs, "Result:")
}

// --- Cloud Run Jobs container tests ---

func TestCloudRun_JobArithmetic(t *testing.T) {
	execName := createAndRunJobWithImageAndCommand(t, "arith-crj", evalImageName, []string{"(10 + 5) * 2"}, "10s")

	exec := waitExecutionDone(t, execName)
	assert.Equal(t, float64(1), exec["succeededCount"])
	assert.Equal(t, float64(0), exec["failedCount"])

	// Poll until the arithmetic result is ingested into Cloud Logging.
	require.Eventually(t, func() bool {
		return strings.Contains(jobLogs(t, "arith-crj"), "Result: 30")
	}, 60*time.Second, 200*time.Millisecond)
}

func TestCloudRun_JobArithmeticInvalid(t *testing.T) {
	execName := createAndRunJobWithImageAndCommand(t, "arith-crj-fail", evalImageName, []string{"3 +"}, "10s")

	exec := waitExecutionDone(t, execName)
	assert.Equal(t, float64(1), exec["failedCount"])
	assert.Equal(t, float64(0), exec["succeededCount"])
}

func TestCloudRun_JobArithmeticLogs(t *testing.T) {
	_ = createAndRunJobWithImageAndCommand(t, "arith-crj-logs", evalImageName, []string{"10 / 3"}, "10s")

	// Poll until both the result + parsing logs are ingested.
	require.Eventually(t, func() bool {
		l := jobLogs(t, "arith-crj-logs")
		return strings.Contains(l, "3.333") && strings.Contains(l, "Parsing expression:")
	}, 60*time.Second, 200*time.Millisecond)
}

// jobLogs returns the joined Cloud Logging messages for a Cloud Run job.
func jobLogs(t *testing.T, jobName string) string {
	t.Helper()
	client := logadminClient(t)
	filter := fmt.Sprintf(`resource.type="cloud_run_job" AND resource.labels.job_name=%q`, jobName)
	it := client.Entries(ctx, logadmin.Filter(filter))
	var messages []string
	for {
		entry, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return strings.Join(messages, "\n")
		}
		if s, ok := entry.Payload.(string); ok {
			messages = append(messages, s)
		}
	}
	return strings.Join(messages, "\n")
}
