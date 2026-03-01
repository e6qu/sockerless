package aws_cli_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestECS_CLI_RunTaskAndCheckLogs(t *testing.T) {
	// Create cluster
	runCLI(t, awsCLI("ecs", "create-cluster", "--cluster-name", "cli-ecs-cluster"))

	// Register task definition with echo command and awslogs
	out := runCLI(t, awsCLI("ecs", "register-task-definition",
		"--family", "cli-ecs-task",
		"--requires-compatibilities", "FARGATE",
		"--network-mode", "awsvpc",
		"--cpu", "256",
		"--memory", "512",
		"--container-definitions", `[{
			"name": "app",
			"image": "alpine:latest",
			"command": ["echo", "hello-from-ecs"],
			"logConfiguration": {
				"logDriver": "awslogs",
				"options": {
					"awslogs-group": "/ecs/cli-task",
					"awslogs-stream-prefix": "ecs"
				}
			}
		}]`,
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
		"--cluster", "cli-ecs-cluster",
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
		"--cluster", "cli-ecs-cluster",
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

	// Verify CloudWatch logs contain the real output
	out = runCLI(t, awsCLI("logs", "filter-log-events",
		"--log-group-name", "/ecs/cli-task",
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
		if strings.Contains(e.Message, "hello-from-ecs") {
			found = true
		}
	}
	assert.True(t, found, "expected 'hello-from-ecs' in CloudWatch logs")
}

func TestECS_CLI_RunTaskNonZeroExit(t *testing.T) {
	// Create cluster
	runCLI(t, awsCLI("ecs", "create-cluster", "--cluster-name", "cli-ecs-fail-cluster"))

	// Register task definition with exit 1
	out := runCLI(t, awsCLI("ecs", "register-task-definition",
		"--family", "cli-ecs-fail-task",
		"--requires-compatibilities", "FARGATE",
		"--network-mode", "awsvpc",
		"--cpu", "256",
		"--memory", "512",
		"--container-definitions", `[{
			"name": "app",
			"image": "alpine:latest",
			"command": ["sh", "-c", "exit 1"],
			"logConfiguration": {
				"logDriver": "awslogs",
				"options": {
					"awslogs-group": "/ecs/cli-fail-task",
					"awslogs-stream-prefix": "ecs"
				}
			}
		}]`,
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
		"--cluster", "cli-ecs-fail-cluster",
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
		"--cluster", "cli-ecs-fail-cluster",
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
