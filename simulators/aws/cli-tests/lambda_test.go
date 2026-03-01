package aws_cli_test

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createDummyZip(t *testing.T) string {
	t.Helper()
	zipPath := filepath.Join(tmpDir, "lambda-func.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)
	w := zip.NewWriter(f)
	fw, err := w.Create("index.js")
	require.NoError(t, err)
	fw.Write([]byte(`exports.handler = async () => ({ statusCode: 200, body: "hello" });`))
	w.Close()
	f.Close()
	return zipPath
}

func TestLambda_CreateAndGetFunction(t *testing.T) {
	zipPath := createDummyZip(t)

	out := runCLI(t, awsCLI("lambda", "create-function",
		"--function-name", "cli-test-func",
		"--runtime", "nodejs18.x",
		"--role", "arn:aws:iam::123456789012:role/test-role",
		"--handler", "index.handler",
		"--zip-file", "fileb://"+zipPath,
		"--output", "json",
	))

	var createResult struct {
		FunctionName string `json:"FunctionName"`
		FunctionArn  string `json:"FunctionArn"`
		Runtime      string `json:"Runtime"`
		State        string `json:"State"`
	}
	parseJSON(t, out, &createResult)
	assert.Equal(t, "cli-test-func", createResult.FunctionName)
	assert.Equal(t, "nodejs18.x", createResult.Runtime)

	// Get function
	out = runCLI(t, awsCLI("lambda", "get-function",
		"--function-name", "cli-test-func",
		"--output", "json",
	))

	var getResult struct {
		Configuration struct {
			FunctionName string `json:"FunctionName"`
			Runtime      string `json:"Runtime"`
		} `json:"Configuration"`
	}
	parseJSON(t, out, &getResult)
	assert.Equal(t, "cli-test-func", getResult.Configuration.FunctionName)

	// Cleanup
	runCLI(t, awsCLI("lambda", "delete-function", "--function-name", "cli-test-func"))
}

func TestLambda_ListFunctions(t *testing.T) {
	zipPath := createDummyZip(t)

	runCLI(t, awsCLI("lambda", "create-function",
		"--function-name", "list-test-func",
		"--runtime", "python3.12",
		"--role", "arn:aws:iam::123456789012:role/test-role",
		"--handler", "handler.handler",
		"--zip-file", "fileb://"+zipPath,
	))

	out := runCLI(t, awsCLI("lambda", "list-functions", "--output", "json"))

	var result struct {
		Functions []struct {
			FunctionName string `json:"FunctionName"`
		} `json:"Functions"`
	}
	parseJSON(t, out, &result)

	found := false
	for _, f := range result.Functions {
		if f.FunctionName == "list-test-func" {
			found = true
		}
	}
	assert.True(t, found, "Expected to find list-test-func in list")

	// Cleanup
	runCLI(t, awsCLI("lambda", "delete-function", "--function-name", "list-test-func"))
}

func TestLambda_InvokeFunction(t *testing.T) {
	zipPath := createDummyZip(t)

	runCLI(t, awsCLI("lambda", "create-function",
		"--function-name", "invoke-test-func",
		"--runtime", "nodejs18.x",
		"--role", "arn:aws:iam::123456789012:role/test-role",
		"--handler", "index.handler",
		"--zip-file", "fileb://"+zipPath,
	))

	outFile := filepath.Join(tmpDir, "invoke-output.json")
	out := runCLI(t, awsCLI("lambda", "invoke",
		"--function-name", "invoke-test-func",
		outFile,
		"--output", "json",
	))

	var invokeResult struct {
		StatusCode     int    `json:"StatusCode"`
		ExecutedVersion string `json:"ExecutedVersion"`
	}
	parseJSON(t, out, &invokeResult)
	assert.Equal(t, 200, invokeResult.StatusCode)

	// Cleanup
	runCLI(t, awsCLI("lambda", "delete-function", "--function-name", "invoke-test-func"))
}

func TestLambda_UpdateFunctionConfiguration(t *testing.T) {
	zipPath := createDummyZip(t)

	runCLI(t, awsCLI("lambda", "create-function",
		"--function-name", "update-test-func",
		"--runtime", "nodejs18.x",
		"--role", "arn:aws:iam::123456789012:role/test-role",
		"--handler", "index.handler",
		"--zip-file", "fileb://"+zipPath,
	))

	out := runCLI(t, awsCLI("lambda", "update-function-configuration",
		"--function-name", "update-test-func",
		"--memory-size", "256",
		"--timeout", "30",
		"--output", "json",
	))

	var result struct {
		MemorySize int `json:"MemorySize"`
		Timeout    int `json:"Timeout"`
	}
	parseJSON(t, out, &result)
	assert.Equal(t, 256, result.MemorySize)
	assert.Equal(t, 30, result.Timeout)

	// Cleanup
	runCLI(t, awsCLI("lambda", "delete-function", "--function-name", "update-test-func"))
}

func TestLambda_CLI_InvokeAndCheckLogs(t *testing.T) {
	zipPath := createDummyZip(t)

	fnName := "cli-log-invoke-func"

	runCLI(t, awsCLI("lambda", "create-function",
		"--function-name", fnName,
		"--runtime", "nodejs18.x",
		"--role", "arn:aws:iam::123456789012:role/test-role",
		"--handler", "index.handler",
		"--zip-file", "fileb://"+zipPath,
	))

	// Invoke the function
	outFile := filepath.Join(tmpDir, "cli-log-invoke-output.json")
	runCLI(t, awsCLI("lambda", "invoke",
		"--function-name", fnName,
		outFile,
		"--output", "json",
	))

	// Query CloudWatch logs for this function
	logGroupName := "/aws/lambda/" + fnName
	out := runCLI(t, awsCLI("logs", "filter-log-events",
		"--log-group-name", logGroupName,
		"--output", "json",
	))

	var logResult struct {
		Events []struct {
			Message string `json:"message"`
		} `json:"events"`
	}
	parseJSON(t, out, &logResult)
	require.NotEmpty(t, logResult.Events, "expected log events for Lambda invocation")

	// Verify START/END/REPORT log entries
	var messages []string
	for _, e := range logResult.Events {
		messages = append(messages, e.Message)
	}

	hasStart := false
	hasEnd := false
	for _, m := range messages {
		if strings.Contains(m, "START RequestId:") {
			hasStart = true
		}
		if strings.Contains(m, "END RequestId:") {
			hasEnd = true
		}
	}
	assert.True(t, hasStart, "expected START log entry, got: %v", messages)
	assert.True(t, hasEnd, "expected END log entry, got: %v", messages)

	// Cleanup
	runCLI(t, awsCLI("lambda", "delete-function", "--function-name", fnName))
}

func TestLambda_DeleteFunction(t *testing.T) {
	zipPath := createDummyZip(t)

	runCLI(t, awsCLI("lambda", "create-function",
		"--function-name", "delete-test-func",
		"--runtime", "nodejs18.x",
		"--role", "arn:aws:iam::123456789012:role/test-role",
		"--handler", "index.handler",
		"--zip-file", "fileb://"+zipPath,
	))

	runCLI(t, awsCLI("lambda", "delete-function", "--function-name", "delete-test-func"))

	// Verify deletion
	out := runCLI(t, awsCLI("lambda", "list-functions", "--output", "json"))
	var result struct {
		Functions []struct {
			FunctionName string `json:"FunctionName"`
		} `json:"Functions"`
	}
	parseJSON(t, out, &result)
	for _, f := range result.Functions {
		assert.NotEqual(t, "delete-test-func", f.FunctionName)
	}
}
