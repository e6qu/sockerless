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
		fn["serviceConfig"] = map[string]any{
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
	gcpCreateFunction(t, fnID, []string{evalBinaryPath, "3 + 4 * 2"})
	body := gcpInvokeFunction(t, fnID)
	assert.Contains(t, body, "11")
}

func TestCloudFunctions_InvokeArithmeticParentheses(t *testing.T) {
	fnID := "arith-paren-cf"
	gcpCreateFunction(t, fnID, []string{evalBinaryPath, "(3 + 4) * 2"})
	body := gcpInvokeFunction(t, fnID)
	assert.Contains(t, body, "14")
}

func TestCloudFunctions_InvokeArithmeticInvalid(t *testing.T) {
	fnID := "arith-invalid-cf"
	gcpCreateFunction(t, fnID, []string{evalBinaryPath, "3 +"})
	gcpInvokeFunction(t, fnID)

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
	gcpCreateFunction(t, fnID, []string{evalBinaryPath, "((2+3)*4-1)/3"})
	body := gcpInvokeFunction(t, fnID)
	assert.Contains(t, body, "6.333")

	messages := gcpFunctionLogMessages(t, fnID)
	allLogs := strings.Join(messages, "\n")
	assert.Contains(t, allLogs, "Parsing expression:")
	assert.Contains(t, allLogs, "Result:")
}

// --- Cloud Run Jobs container tests ---

func TestCloudRun_JobArithmetic(t *testing.T) {
	execName := createAndRunJobWithCommand(t, "arith-crj", []string{evalBinaryPath, "(10 + 5) * 2"}, "10s")

	time.Sleep(2 * time.Second)

	exec := getExecution(t, execName)
	assert.Equal(t, float64(1), exec["succeededCount"])
	assert.Equal(t, float64(0), exec["failedCount"])

	client := logadminClient(t)
	filter := `resource.type="cloud_run_job" AND resource.labels.job_name="arith-crj"`
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
	assert.Contains(t, messages, "30", "expected output '30' in Cloud Logging")
}

func TestCloudRun_JobArithmeticInvalid(t *testing.T) {
	execName := createAndRunJobWithCommand(t, "arith-crj-fail", []string{evalBinaryPath, "3 +"}, "10s")

	time.Sleep(2 * time.Second)

	exec := getExecution(t, execName)
	assert.Equal(t, float64(1), exec["failedCount"])
	assert.Equal(t, float64(0), exec["succeededCount"])
}

func TestCloudRun_JobArithmeticLogs(t *testing.T) {
	_ = createAndRunJobWithCommand(t, "arith-crj-logs", []string{evalBinaryPath, "10 / 3"}, "10s")

	time.Sleep(2 * time.Second)

	client := logadminClient(t)
	filter := `resource.type="cloud_run_job" AND resource.labels.job_name="arith-crj-logs"`
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
	allLogs := strings.Join(messages, "\n")
	assert.Contains(t, allLogs, "3.333", "expected result '3.333...' in Cloud Logging")
	assert.Contains(t, allLogs, "Parsing expression:", "expected parsing log in Cloud Logging")
}
