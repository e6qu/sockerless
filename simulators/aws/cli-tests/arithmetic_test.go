package aws_cli_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestECS_CLI_ArithmeticEval(t *testing.T) {
	// Create cluster
	runCLI(t, awsCLI("ecs", "create-cluster", "--cluster-name", "cli-arith-cluster"))

	// Register task definition with eval-arithmetic command
	out := runCLI(t, awsCLI("ecs", "register-task-definition",
		"--family", "cli-arith-task",
		"--requires-compatibilities", "FARGATE",
		"--network-mode", "awsvpc",
		"--cpu", "256",
		"--memory", "512",
		"--container-definitions", fmt.Sprintf(`[{
			"name": "app",
			"image": "alpine:latest",
			"command": [%q, "(3 + 4) * 2"],
			"logConfiguration": {
				"logDriver": "awslogs",
				"options": {
					"awslogs-group": "/ecs/cli-arith-task",
					"awslogs-stream-prefix": "ecs"
				}
			}
		}]`, evalBinaryPath),
		"--output", "json",
	))

	var tdResult struct {
		TaskDefinition struct {
			TaskDefinitionArn string `json:"taskDefinitionArn"`
		} `json:"taskDefinition"`
	}
	parseJSON(t, out, &tdResult)
	require.NotEmpty(t, tdResult.TaskDefinition.TaskDefinitionArn)

	// Run task
	out = runCLI(t, awsCLI("ecs", "run-task",
		"--cluster", "cli-arith-cluster",
		"--task-definition", tdResult.TaskDefinition.TaskDefinitionArn,
		"--launch-type", "FARGATE",
		"--count", "1",
		"--network-configuration", `awsvpcConfiguration={subnets=[subnet-12345]}`,
		"--output", "json",
	))

	var runResult struct {
		Tasks []struct {
			TaskArn string `json:"taskArn"`
		} `json:"tasks"`
	}
	parseJSON(t, out, &runResult)
	require.Len(t, runResult.Tasks, 1)
	taskArn := runResult.Tasks[0].TaskArn

	// Wait for process to complete
	time.Sleep(3 * time.Second)

	// Describe task — should be STOPPED with exit code 0
	out = runCLI(t, awsCLI("ecs", "describe-tasks",
		"--cluster", "cli-arith-cluster",
		"--tasks", taskArn,
		"--output", "json",
	))

	var descResult struct {
		Tasks []struct {
			LastStatus string `json:"lastStatus"`
			Containers []struct {
				ExitCode *int `json:"exitCode"`
			} `json:"containers"`
		} `json:"tasks"`
	}
	parseJSON(t, out, &descResult)
	require.Len(t, descResult.Tasks, 1)
	assert.Equal(t, "STOPPED", descResult.Tasks[0].LastStatus)
	require.NotEmpty(t, descResult.Tasks[0].Containers)
	require.NotNil(t, descResult.Tasks[0].Containers[0].ExitCode)
	assert.Equal(t, 0, *descResult.Tasks[0].Containers[0].ExitCode)

	// Verify CloudWatch logs contain the result
	out = runCLI(t, awsCLI("logs", "filter-log-events",
		"--log-group-name", "/ecs/cli-arith-task",
		"--output", "json",
	))

	var logResult struct {
		Events []struct {
			Message string `json:"message"`
		} `json:"events"`
	}
	parseJSON(t, out, &logResult)
	require.NotEmpty(t, logResult.Events)

	found := false
	for _, e := range logResult.Events {
		if strings.Contains(e.Message, "14") {
			found = true
		}
	}
	assert.True(t, found, "expected '14' in CloudWatch logs")
}

func TestECS_CLI_ArithmeticInvalid(t *testing.T) {
	// Create cluster
	runCLI(t, awsCLI("ecs", "create-cluster", "--cluster-name", "cli-arith-fail-cluster"))

	// Register task definition with invalid expression
	out := runCLI(t, awsCLI("ecs", "register-task-definition",
		"--family", "cli-arith-fail-task",
		"--requires-compatibilities", "FARGATE",
		"--network-mode", "awsvpc",
		"--cpu", "256",
		"--memory", "512",
		"--container-definitions", fmt.Sprintf(`[{
			"name": "app",
			"image": "alpine:latest",
			"command": [%q, "3 +"],
			"logConfiguration": {
				"logDriver": "awslogs",
				"options": {
					"awslogs-group": "/ecs/cli-arith-fail-task",
					"awslogs-stream-prefix": "ecs"
				}
			}
		}]`, evalBinaryPath),
		"--output", "json",
	))

	var tdResult struct {
		TaskDefinition struct {
			TaskDefinitionArn string `json:"taskDefinitionArn"`
		} `json:"taskDefinition"`
	}
	parseJSON(t, out, &tdResult)

	// Run task
	out = runCLI(t, awsCLI("ecs", "run-task",
		"--cluster", "cli-arith-fail-cluster",
		"--task-definition", tdResult.TaskDefinition.TaskDefinitionArn,
		"--launch-type", "FARGATE",
		"--count", "1",
		"--network-configuration", `awsvpcConfiguration={subnets=[subnet-12345]}`,
		"--output", "json",
	))

	var runResult struct {
		Tasks []struct {
			TaskArn string `json:"taskArn"`
		} `json:"tasks"`
	}
	parseJSON(t, out, &runResult)
	require.Len(t, runResult.Tasks, 1)
	taskArn := runResult.Tasks[0].TaskArn

	// Wait for process to complete
	time.Sleep(3 * time.Second)

	// Describe task — should be STOPPED with exit code 1
	out = runCLI(t, awsCLI("ecs", "describe-tasks",
		"--cluster", "cli-arith-fail-cluster",
		"--tasks", taskArn,
		"--output", "json",
	))

	var descResult struct {
		Tasks []struct {
			LastStatus string `json:"lastStatus"`
			Containers []struct {
				ExitCode *int `json:"exitCode"`
			} `json:"containers"`
		} `json:"tasks"`
	}
	parseJSON(t, out, &descResult)
	require.Len(t, descResult.Tasks, 1)
	assert.Equal(t, "STOPPED", descResult.Tasks[0].LastStatus)
	require.NotEmpty(t, descResult.Tasks[0].Containers)
	require.NotNil(t, descResult.Tasks[0].Containers[0].ExitCode)
	assert.Equal(t, 1, *descResult.Tasks[0].Containers[0].ExitCode)
}
