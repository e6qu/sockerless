package azure_sdk_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Azure Functions FaaS tests ---

func TestAzureFunctions_InvokeArithmetic(t *testing.T) {
	rg, name := "arith-func-rg", "arith-basic-app"
	azureCreateSite(t, rg, name, []string{evalBinaryPath, "3 + 4 * 2"})
	defer azureDeleteSite(rg, name)

	body := azureInvokeFunction(t)
	assert.Contains(t, string(body), "11")
}

func TestAzureFunctions_InvokeArithmeticParentheses(t *testing.T) {
	rg, name := "arith-paren-rg", "arith-paren-app"
	azureCreateSite(t, rg, name, []string{evalBinaryPath, "(3 + 4) * 2"})
	defer azureDeleteSite(rg, name)

	body := azureInvokeFunction(t)
	assert.Contains(t, string(body), "14")
}

func TestAzureFunctions_InvokeArithmeticInvalid(t *testing.T) {
	rg, name := "arith-inv-rg", "arith-invalid-app"
	azureCreateSite(t, rg, name, []string{evalBinaryPath, "3 +"})
	defer azureDeleteSite(rg, name)

	azureInvokeFunction(t)

	kql := `AppTraces | where AppRoleName == "arith-invalid-app"`
	result := queryWorkspace(t, "default", kql)

	require.Len(t, result.Tables, 1)
	table := result.Tables[0]
	require.GreaterOrEqual(t, len(table.Rows), 1)

	msgIdx := -1
	for i, col := range table.Columns {
		if col.Name == "Message" {
			msgIdx = i
		}
	}
	require.GreaterOrEqual(t, msgIdx, 0)

	found := false
	for _, row := range table.Rows {
		msg, ok := row[msgIdx].(string)
		if ok && strings.Contains(msg, "error") && strings.Contains(msg, "exit") {
			found = true
		}
	}
	assert.True(t, found, "expected error log entry for invalid expression")
}

func TestAzureFunctions_InvokeArithmeticLogs(t *testing.T) {
	rg, name := "arith-log-rg", "arith-logs-app"
	azureCreateSite(t, rg, name, []string{evalBinaryPath, "((2+3)*4-1)/3"})
	defer azureDeleteSite(rg, name)

	body := azureInvokeFunction(t)
	assert.Contains(t, string(body), "6.333")

	kql := `AppTraces | where AppRoleName == "arith-logs-app"`
	result := queryWorkspace(t, "default", kql)

	require.Len(t, result.Tables, 1)
	table := result.Tables[0]

	msgIdx := -1
	for i, col := range table.Columns {
		if col.Name == "Message" {
			msgIdx = i
		}
	}
	require.GreaterOrEqual(t, msgIdx, 0)

	var allMsgs []string
	for _, row := range table.Rows {
		if msg, ok := row[msgIdx].(string); ok {
			allMsgs = append(allMsgs, msg)
		}
	}
	allLogs := strings.Join(allMsgs, "\n")
	assert.Contains(t, allLogs, "Parsing expression:")
	assert.Contains(t, allLogs, "Result:")
}

// --- Container Apps Jobs container tests ---

func TestContainerApps_JobArithmetic(t *testing.T) {
	rg, jobName := "arith-aca-rg", "arith-aca-job"
	acaCreateJobWithCommand(t, rg, jobName, []string{evalBinaryPath, "(10 + 5) * 2"})
	execName := acaStartExecution(t, rg, jobName)

	time.Sleep(2 * time.Second)

	exec := acaGetExecution(t, rg, jobName, execName)
	assert.Equal(t, "Succeeded", exec["status"])

	kql := `ContainerAppConsoleLogs_CL | where ContainerGroupName_s == "arith-aca-job"`
	result := queryWorkspace(t, "default", kql)

	require.Len(t, result.Tables, 1)
	table := result.Tables[0]

	logIdx := -1
	for i, col := range table.Columns {
		if col.Name == "Log_s" {
			logIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, logIdx, 0)

	var logs []string
	for _, row := range table.Rows {
		logs = append(logs, row[logIdx].(string))
	}
	assert.Contains(t, logs, "30", "expected output '30' in Log Analytics")
}

func TestContainerApps_JobArithmeticInvalid(t *testing.T) {
	rg, jobName := "arith-aca-rg", "arith-aca-fail-job"
	acaCreateJobWithCommand(t, rg, jobName, []string{evalBinaryPath, "3 +"})
	execName := acaStartExecution(t, rg, jobName)

	time.Sleep(2 * time.Second)

	exec := acaGetExecution(t, rg, jobName, execName)
	assert.Equal(t, "Failed", exec["status"])
}

func TestContainerApps_JobArithmeticLogs(t *testing.T) {
	rg, jobName := "arith-aca-rg", "arith-aca-log-job"
	acaCreateJobWithCommand(t, rg, jobName, []string{evalBinaryPath, "10 / 3"})
	_ = acaStartExecution(t, rg, jobName)

	time.Sleep(2 * time.Second)

	kql := `ContainerAppConsoleLogs_CL | where ContainerGroupName_s == "arith-aca-log-job"`
	result := queryWorkspace(t, "default", kql)

	require.Len(t, result.Tables, 1)
	table := result.Tables[0]

	logIdx := -1
	for i, col := range table.Columns {
		if col.Name == "Log_s" {
			logIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, logIdx, 0)

	var logs []string
	for _, row := range table.Rows {
		logs = append(logs, row[logIdx].(string))
	}
	allLogs := strings.Join(logs, "\n")
	assert.Contains(t, allLogs, "3.333", "expected result '3.333...' in Log Analytics")
	assert.Contains(t, allLogs, "Parsing expression:", "expected parsing log in Log Analytics")
}
