package aws_cli_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sfnSyncEndpoint returns the simulator's Step Functions endpoint coordinate.
// It has the same shape as the real endpoint a CLI resolves
// (`states.us-east-1.amazonaws.com`), so botocore prepends the `sync-` host
// prefix that StartSyncExecution and TestState carry and sends the request to
// `sync-states.localhost`. Callers pair it with awsCLIHostPrefixed, which is
// what makes that name reach the loopback sim.
func sfnSyncEndpoint(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("http://states.localhost:%s", portFromBaseURL(t))
}

// portFromBaseURL extracts the port from the suite baseURL (http://127.0.0.1:PORT).
func portFromBaseURL(t *testing.T) string {
	t.Helper()
	i := strings.LastIndex(baseURL, ":")
	require.GreaterOrEqual(t, i, 0)
	return baseURL[i+1:]
}

// TestSFNCLI_Activities covers create-activity / describe-activity /
// list-activities / get-activity-task / send-task-success /
// send-task-failure / send-task-heartbeat / delete-activity.
func TestSFNCLI_Activities(t *testing.T) {
	out := runCLI(t, awsCLI("stepfunctions", "create-activity", "--name", "sfn-cli-activity"))
	var created struct {
		ActivityArn string `json:"activityArn"`
	}
	parseJSON(t, out, &created)
	require.NotEmpty(t, created.ActivityArn)
	t.Cleanup(func() {
		runCLI(t, awsCLI("stepfunctions", "delete-activity", "--activity-arn", created.ActivityArn))
	})

	out = runCLI(t, awsCLI("stepfunctions", "describe-activity", "--activity-arn", created.ActivityArn))
	var desc struct {
		Name string `json:"name"`
	}
	parseJSON(t, out, &desc)
	assert.Equal(t, "sfn-cli-activity", desc.Name)

	out = runCLI(t, awsCLI("stepfunctions", "list-activities"))
	var list struct {
		Activities []struct {
			Name string `json:"name"`
		} `json:"activities"`
	}
	parseJSON(t, out, &list)
	found := false
	for _, a := range list.Activities {
		if a.Name == "sfn-cli-activity" {
			found = true
		}
	}
	assert.True(t, found)

	// No work scheduled — get-activity-task returns an empty token.
	out = runCLI(t, awsCLI("stepfunctions", "get-activity-task",
		"--activity-arn", created.ActivityArn, "--worker-name", "w1"))
	var task struct {
		TaskToken string `json:"taskToken"`
	}
	parseJSON(t, out, &task)
	assert.Empty(t, task.TaskToken)

	// SendTask* against unknown tokens raise TaskDoesNotExist.
	runCLIExpectError(t, awsCLI("stepfunctions", "send-task-heartbeat", "--task-token", "nope"))
	runCLIExpectError(t, awsCLI("stepfunctions", "send-task-success", "--task-token", "nope", "--output", "{}"))
	runCLIExpectError(t, awsCLI("stepfunctions", "send-task-failure", "--task-token", "nope", "--error", "E"))
}

// TestSFNCLI_VersionsAndAliases covers publish-state-machine-version,
// create-state-machine-alias, describe-state-machine-alias,
// list-state-machine-aliases, update-state-machine-alias,
// delete-state-machine-alias, delete-state-machine-version.
func TestSFNCLI_VersionsAndAliases(t *testing.T) {
	definition := `{"StartAt":"P","States":{"P":{"Type":"Pass","End":true}}}`
	out := runCLI(t, awsCLI("stepfunctions", "create-state-machine",
		"--name", "sfn-cli-ver-sm",
		"--definition", definition,
		"--role-arn", "arn:aws:iam::123456789012:role/sfn-role"))
	var sm struct {
		StateMachineArn string `json:"stateMachineArn"`
	}
	parseJSON(t, out, &sm)
	t.Cleanup(func() {
		runCLI(t, awsCLI("stepfunctions", "delete-state-machine", "--state-machine-arn", sm.StateMachineArn))
	})

	out = runCLI(t, awsCLI("stepfunctions", "publish-state-machine-version",
		"--state-machine-arn", sm.StateMachineArn, "--description", "v1"))
	var pub struct {
		StateMachineVersionArn string `json:"stateMachineVersionArn"`
	}
	parseJSON(t, out, &pub)
	require.NotEmpty(t, pub.StateMachineVersionArn)

	out = runCLI(t, awsCLI("stepfunctions", "create-state-machine-alias",
		"--name", "PROD",
		"--routing-configuration", "stateMachineVersionArn="+pub.StateMachineVersionArn+",weight=100"))
	var alias struct {
		StateMachineAliasArn string `json:"stateMachineAliasArn"`
	}
	parseJSON(t, out, &alias)
	require.NotEmpty(t, alias.StateMachineAliasArn)
	t.Cleanup(func() {
		runCLI(t, awsCLI("stepfunctions", "delete-state-machine-alias",
			"--state-machine-alias-arn", alias.StateMachineAliasArn))
	})

	out = runCLI(t, awsCLI("stepfunctions", "describe-state-machine-alias",
		"--state-machine-alias-arn", alias.StateMachineAliasArn))
	var da struct {
		Name                 string `json:"name"`
		RoutingConfiguration []struct {
			StateMachineVersionArn string `json:"stateMachineVersionArn"`
			Weight                 int    `json:"weight"`
		} `json:"routingConfiguration"`
	}
	parseJSON(t, out, &da)
	assert.Equal(t, "PROD", da.Name)
	require.Len(t, da.RoutingConfiguration, 1)
	assert.Equal(t, 100, da.RoutingConfiguration[0].Weight)

	out = runCLI(t, awsCLI("stepfunctions", "list-state-machine-aliases",
		"--state-machine-arn", sm.StateMachineArn))
	var la struct {
		StateMachineAliases []struct {
			StateMachineAliasArn string `json:"stateMachineAliasArn"`
		} `json:"stateMachineAliases"`
	}
	parseJSON(t, out, &la)
	require.Len(t, la.StateMachineAliases, 1)

	runCLI(t, awsCLI("stepfunctions", "update-state-machine-alias",
		"--state-machine-alias-arn", alias.StateMachineAliasArn, "--description", "updated"))

	runCLI(t, awsCLI("stepfunctions", "delete-state-machine-version",
		"--state-machine-version-arn", pub.StateMachineVersionArn))
}

// TestSFNCLI_TestState runs a single Pass state synchronously. test-state
// carries a `sync-` host prefix, so it targets the sync endpoint.
func TestSFNCLI_TestState(t *testing.T) {
	endpoint := sfnSyncEndpoint(t)
	out := runCLI(t, awsCLIHostPrefixed("stepfunctions", "test-state",
		"--definition", `{"Type":"Pass","Result":{"hello":"world"},"End":true}`,
		"--input", `{"x":1}`,
		"--role-arn", "arn:aws:iam::123456789012:role/sfn-role",
		"--endpoint-url", endpoint))
	var res struct {
		Status string `json:"status"`
		Output string `json:"output"`
	}
	parseJSON(t, out, &res)
	assert.Equal(t, "SUCCEEDED", res.Status)
	assert.JSONEq(t, `{"hello":"world"}`, res.Output)
}

// TestSFNCLI_StartSyncExecution runs a whole EXPRESS state machine
// synchronously. start-sync-execution carries a `sync-` host prefix, so it
// targets the sync endpoint.
func TestSFNCLI_StartSyncExecution(t *testing.T) {
	endpoint := sfnSyncEndpoint(t)
	definition := `{"StartAt":"P","States":{"P":{"Type":"Pass","Result":{"ok":true},"End":true}}}`
	out := runCLI(t, awsCLI("stepfunctions", "create-state-machine",
		"--name", "sfn-cli-sync-sm",
		"--definition", definition,
		"--role-arn", "arn:aws:iam::123456789012:role/sfn-role",
		"--type", "EXPRESS"))
	var sm struct {
		StateMachineArn string `json:"stateMachineArn"`
	}
	parseJSON(t, out, &sm)
	t.Cleanup(func() {
		runCLI(t, awsCLI("stepfunctions", "delete-state-machine", "--state-machine-arn", sm.StateMachineArn))
	})

	out = runCLI(t, awsCLIHostPrefixed("stepfunctions", "start-sync-execution",
		"--state-machine-arn", sm.StateMachineArn, "--input", "{}",
		"--endpoint-url", endpoint))
	var res struct {
		Status string `json:"status"`
		Output string `json:"output"`
	}
	parseJSON(t, out, &res)
	assert.Equal(t, "SUCCEEDED", res.Status)
	assert.JSONEq(t, `{"ok":true}`, res.Output)
}

// TestSFNCLI_DescribeForExecutionAndRedrive covers
// describe-state-machine-for-execution and redrive-execution, plus
// list-map-runs (empty).
func TestSFNCLI_DescribeForExecutionAndRedrive(t *testing.T) {
	definition := `{"StartAt":"F","States":{"F":{"Type":"Fail","Error":"E","Cause":"boom"}}}`
	out := runCLI(t, awsCLI("stepfunctions", "create-state-machine",
		"--name", "sfn-cli-redrive-sm",
		"--definition", definition,
		"--role-arn", "arn:aws:iam::123456789012:role/sfn-role"))
	var sm struct {
		StateMachineArn string `json:"stateMachineArn"`
	}
	parseJSON(t, out, &sm)
	t.Cleanup(func() {
		runCLI(t, awsCLI("stepfunctions", "delete-state-machine", "--state-machine-arn", sm.StateMachineArn))
	})

	out = runCLI(t, awsCLI("stepfunctions", "start-execution",
		"--state-machine-arn", sm.StateMachineArn, "--input", "{}"))
	var started struct {
		ExecutionArn string `json:"executionArn"`
	}
	parseJSON(t, out, &started)

	out = runCLI(t, awsCLI("stepfunctions", "describe-state-machine-for-execution",
		"--execution-arn", started.ExecutionArn))
	var dfe struct {
		Name       string `json:"name"`
		Definition string `json:"definition"`
	}
	parseJSON(t, out, &dfe)
	assert.Equal(t, "sfn-cli-redrive-sm", dfe.Name)
	assert.Contains(t, dfe.Definition, "Fail")

	var exec struct {
		Status string `json:"status"`
	}
	require.Eventually(t, func() bool {
		o := runCLI(t, awsCLI("stepfunctions", "describe-execution", "--execution-arn", started.ExecutionArn))
		parseJSON(t, o, &exec)
		return exec.Status == "FAILED"
	}, 10*time.Second, 100*time.Millisecond)

	out = runCLI(t, awsCLI("stepfunctions", "redrive-execution", "--execution-arn", started.ExecutionArn))
	// The CLI renders the epoch redriveDate as an ISO-8601 timestamp string.
	var rd struct {
		RedriveDate string `json:"redriveDate"`
	}
	parseJSON(t, out, &rd)
	assert.NotEmpty(t, rd.RedriveDate)

	out = runCLI(t, awsCLI("stepfunctions", "list-map-runs", "--execution-arn", started.ExecutionArn))
	var lmr struct {
		MapRuns []any `json:"mapRuns"`
	}
	parseJSON(t, out, &lmr)
	assert.Empty(t, lmr.MapRuns)
}
