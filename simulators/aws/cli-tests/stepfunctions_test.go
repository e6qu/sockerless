package aws_cli_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSFN_StateMachineCRUD_CLI(t *testing.T) {
	definition := `{"Comment":"cli test","StartAt":"Pass","States":{"Pass":{"Type":"Pass","End":true}}}`

	out := runCLI(t, awsCLI("stepfunctions", "create-state-machine",
		"--name", "sfn-cli-sm",
		"--definition", definition,
		"--role-arn", "arn:aws:iam::123456789012:role/sfn-role",
		"--type", "STANDARD",
	))
	var created struct {
		StateMachineArn string `json:"stateMachineArn"`
	}
	parseJSON(t, out, &created)
	require.NotEmpty(t, created.StateMachineArn)
	t.Cleanup(func() {
		runCLI(t, awsCLI("stepfunctions", "delete-state-machine",
			"--state-machine-arn", created.StateMachineArn))
	})

	out = runCLI(t, awsCLI("stepfunctions", "describe-state-machine",
		"--state-machine-arn", created.StateMachineArn))
	var described struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Type   string `json:"type"`
	}
	parseJSON(t, out, &described)
	assert.Equal(t, "sfn-cli-sm", described.Name)
	assert.Equal(t, "ACTIVE", described.Status)
	assert.Equal(t, "STANDARD", described.Type)

	out = runCLI(t, awsCLI("stepfunctions", "list-state-machines"))
	var list struct {
		StateMachines []struct {
			Name string `json:"name"`
		} `json:"stateMachines"`
	}
	parseJSON(t, out, &list)
	found := false
	for _, sm := range list.StateMachines {
		if sm.Name == "sfn-cli-sm" {
			found = true
		}
	}
	assert.True(t, found)

	// Tag / untag
	runCLI(t, awsCLI("stepfunctions", "tag-resource",
		"--resource-arn", created.StateMachineArn,
		"--tags", "key=env,value=cli"))
	out = runCLI(t, awsCLI("stepfunctions", "list-tags-for-resource",
		"--resource-arn", created.StateMachineArn))
	var tags struct {
		Tags []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"tags"`
	}
	parseJSON(t, out, &tags)
	require.Len(t, tags.Tags, 1)
	assert.Equal(t, "env", tags.Tags[0].Key)
	assert.Equal(t, "cli", tags.Tags[0].Value)

	runCLI(t, awsCLI("stepfunctions", "untag-resource",
		"--resource-arn", created.StateMachineArn,
		"--tag-keys", "env"))
	out = runCLI(t, awsCLI("stepfunctions", "list-tags-for-resource",
		"--resource-arn", created.StateMachineArn))
	parseJSON(t, out, &tags)
	assert.Empty(t, tags.Tags)
}

func TestSFN_ExecutionLifecycle_CLI(t *testing.T) {
	definition := `{"StartAt":"Pass","States":{"Pass":{"Type":"Pass","End":true}}}`

	out := runCLI(t, awsCLI("stepfunctions", "create-state-machine",
		"--name", "sfn-cli-exec-sm",
		"--definition", definition,
		"--role-arn", "arn:aws:iam::123456789012:role/sfn-role",
	))
	var created struct {
		StateMachineArn string `json:"stateMachineArn"`
	}
	parseJSON(t, out, &created)
	t.Cleanup(func() {
		runCLI(t, awsCLI("stepfunctions", "delete-state-machine",
			"--state-machine-arn", created.StateMachineArn))
	})

	inputJSON, _ := json.Marshal(map[string]string{"hello": "world"})
	out = runCLI(t, awsCLI("stepfunctions", "start-execution",
		"--state-machine-arn", created.StateMachineArn,
		"--name", "cli-exec-1",
		"--input", string(inputJSON),
	))
	var started struct {
		ExecutionArn string `json:"executionArn"`
	}
	parseJSON(t, out, &started)
	require.NotEmpty(t, started.ExecutionArn)

	var exec struct {
		Status string `json:"status"`
		Name   string `json:"name"`
	}
	require.Eventually(t, func() bool {
		out = runCLI(t, awsCLI("stepfunctions", "describe-execution",
			"--execution-arn", started.ExecutionArn))
		parseJSON(t, out, &exec)
		return exec.Status == "SUCCEEDED"
	}, 10*time.Second, 100*time.Millisecond)
	assert.Equal(t, "SUCCEEDED", exec.Status)
	assert.Equal(t, "cli-exec-1", exec.Name)

	out = runCLI(t, awsCLI("stepfunctions", "list-executions",
		"--state-machine-arn", created.StateMachineArn))
	var execList struct {
		Executions []struct {
			ExecutionArn string `json:"executionArn"`
			Status       string `json:"status"`
		} `json:"executions"`
	}
	parseJSON(t, out, &execList)
	require.Len(t, execList.Executions, 1)
	assert.Equal(t, started.ExecutionArn, execList.Executions[0].ExecutionArn)
	assert.Equal(t, "SUCCEEDED", execList.Executions[0].Status)
}
