package aws_cli_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestECS_CLI_ServiceFamily(t *testing.T) {
	cluster := "cli-ecs-svc-cluster"
	runCLI(t, awsCLI("ecs", "create-cluster", "--cluster-name", cluster))
	t.Cleanup(func() { _ = awsCLI("ecs", "delete-cluster", "--cluster", cluster).Run() })
	runCLI(t, awsCLI("ecs", "register-task-definition",
		"--family", "cli-svc-task",
		"--container-definitions", `[{"name":"app","image":"alpine:latest","essential":true}]`))

	// PutClusterCapacityProviders → DescribeClusters echoes them.
	runCLI(t, awsCLI("ecs", "put-cluster-capacity-providers",
		"--cluster", cluster,
		"--capacity-providers", "FARGATE", "FARGATE_SPOT",
		"--default-capacity-provider-strategy", "capacityProvider=FARGATE,weight=1,base=1"))
	descCl := runCLI(t, awsCLI("ecs", "describe-clusters", "--clusters", cluster, "--output", "json"))
	var cl struct {
		Clusters []struct {
			CapacityProviders []string `json:"capacityProviders"`
		} `json:"clusters"`
	}
	parseJSON(t, descCl, &cl)
	require.Len(t, cl.Clusters, 1)
	assert.ElementsMatch(t, []string{"FARGATE", "FARGATE_SPOT"}, cl.Clusters[0].CapacityProviders)

	// CreateService → ACTIVE.
	createOut := runCLI(t, awsCLI("ecs", "create-service",
		"--cluster", cluster, "--service-name", "cli-svc",
		"--task-definition", "cli-svc-task", "--desired-count", "2",
		"--launch-type", "FARGATE", "--output", "json"))
	var created struct {
		Service struct {
			Status       string `json:"status"`
			RunningCount int    `json:"runningCount"`
		} `json:"service"`
	}
	parseJSON(t, createOut, &created)
	assert.Equal(t, "ACTIVE", created.Service.Status)
	assert.Equal(t, 2, created.Service.RunningCount)

	descOut := runCLI(t, awsCLI("ecs", "describe-services",
		"--cluster", cluster, "--services", "cli-svc", "--output", "json"))
	var desc struct {
		Services []struct {
			Status string `json:"status"`
		} `json:"services"`
	}
	parseJSON(t, descOut, &desc)
	require.Len(t, desc.Services, 1)
	assert.Equal(t, "ACTIVE", desc.Services[0].Status)

	delOut := runCLI(t, awsCLI("ecs", "delete-service",
		"--cluster", cluster, "--service", "cli-svc", "--force", "--output", "json"))
	var del struct {
		Service struct {
			Status string `json:"status"`
		} `json:"service"`
	}
	parseJSON(t, delOut, &del)
	assert.Equal(t, "INACTIVE", del.Service.Status)
}

func TestECS_CLI_RunTaskAndCheckLogs(t *testing.T) {
	subnetID := createCLIECSTestSubnet(t, 142)

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
		"--network-configuration", `awsvpcConfiguration={subnets=[`+subnetID+`]}`,
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
	cleanupCLIECSTask(t, "cli-ecs-cluster", taskArn)

	// Poll until the task reaches STOPPED; netns setup on CI can make a fixed
	// sleep race the real container lifecycle.
	out = pollECSTaskStopped(t, "cli-ecs-cluster", taskArn)

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

func TestECS_CLI_ManagedEBSVolumeSnapshotRoundTrip(t *testing.T) {
	subnetID := createCLIECSTestSubnet(t, 143)

	runCLI(t, awsCLI("ecs", "create-cluster", "--cluster-name", "cli-ebs-roundtrip"))
	runCLI(t, awsCLI("logs", "create-log-group", "--log-group-name", "/ecs/cli-ebs-roundtrip"))

	out := runCLI(t, awsCLI("ecs", "register-task-definition",
		"--family", "cli-ebs-writer",
		"--requires-compatibilities", "FARGATE",
		"--network-mode", "awsvpc",
		"--cpu", "256",
		"--memory", "512",
		"--volumes", `[{"name":"workspace","configuredAtLaunch":true}]`,
		"--container-definitions", `[{
			"name": "writer",
			"image": "`+evalImageName+`",
			"entryPoint": ["sh", "-c"],
			"command": ["printf 'cli-ebs-roundtrip' > /workspace/state.txt"],
			"mountPoints": [{"sourceVolume":"workspace","containerPath":"/workspace"}]
		}]`,
		"--output", "json",
	))
	var writerTD struct {
		TaskDefinition struct {
			TaskDefinitionArn string `json:"taskDefinitionArn"`
		} `json:"taskDefinition"`
	}
	parseJSON(t, out, &writerTD)

	out = runCLI(t, awsCLI("ecs", "run-task",
		"--cluster", "cli-ebs-roundtrip",
		"--task-definition", writerTD.TaskDefinition.TaskDefinitionArn,
		"--launch-type", "FARGATE",
		"--network-configuration", `awsvpcConfiguration={subnets=[`+subnetID+`]}`,
		"--volume-configurations", `[{"name":"workspace","managedEBSVolume":{"roleArn":"arn:aws:iam::123456789012:role/ecsInfrastructureRole","sizeInGiB":1,"volumeType":"gp3","terminationPolicy":{"deleteOnTermination":false},"tagSpecifications":[{"resourceType":"volume","tags":[{"key":"purpose","value":"cli-roundtrip"}]}]}}]`,
		"--output", "json",
	))
	var runWriter struct {
		Tasks []struct {
			TaskArn string `json:"taskArn"`
		} `json:"tasks"`
	}
	parseJSON(t, out, &runWriter)
	require.Len(t, runWriter.Tasks, 1)
	writerTaskArn := runWriter.Tasks[0].TaskArn
	waitCLITaskStatus(t, "cli-ebs-roundtrip", writerTaskArn, "STOPPED")

	out = runCLI(t, awsCLI("ecs", "describe-tasks",
		"--cluster", "cli-ebs-roundtrip",
		"--tasks", writerTaskArn,
		"--output", "json",
	))
	var writerDesc struct {
		Tasks []struct {
			Attachments []struct {
				Type    string `json:"type"`
				Details []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"details"`
			} `json:"attachments"`
		} `json:"tasks"`
	}
	parseJSON(t, out, &writerDesc)
	require.Len(t, writerDesc.Tasks, 1)
	volumeID := cliEBSVolumeID(t, writerDesc.Tasks[0].Attachments)
	t.Cleanup(func() {
		runCLI(t, awsCLI("ec2", "delete-volume", "--volume-id", volumeID))
	})

	out = runCLI(t, awsCLI("ec2", "create-snapshot",
		"--volume-id", volumeID,
		"--description", "cli ebs roundtrip",
		"--output", "json",
	))
	var snapResult struct {
		SnapshotId string `json:"SnapshotId"`
	}
	parseJSON(t, out, &snapResult)
	require.NotEmpty(t, snapResult.SnapshotId)
	waitCLISnapshotStatus(t, snapResult.SnapshotId, "completed")
	t.Cleanup(func() {
		runCLI(t, awsCLI("ec2", "delete-snapshot", "--snapshot-id", snapResult.SnapshotId))
	})

	out = runCLI(t, awsCLI("ecs", "register-task-definition",
		"--family", "cli-ebs-reader",
		"--requires-compatibilities", "FARGATE",
		"--network-mode", "awsvpc",
		"--cpu", "256",
		"--memory", "512",
		"--volumes", `[{"name":"workspace","configuredAtLaunch":true}]`,
		"--container-definitions", `[{
			"name": "reader",
			"image": "`+evalImageName+`",
			"entryPoint": ["sh", "-c"],
			"command": ["test \"$(cat /workspace/state.txt)\" = \"cli-ebs-roundtrip\" && echo CLI_EBS_ROUNDTRIP_OK"],
			"mountPoints": [{"sourceVolume":"workspace","containerPath":"/workspace"}],
			"logConfiguration": {"logDriver":"awslogs","options":{"awslogs-group":"/ecs/cli-ebs-roundtrip","awslogs-stream-prefix":"ecs"}}
		}]`,
		"--output", "json",
	))
	var readerTD struct {
		TaskDefinition struct {
			TaskDefinitionArn string `json:"taskDefinitionArn"`
		} `json:"taskDefinition"`
	}
	parseJSON(t, out, &readerTD)

	out = runCLI(t, awsCLI("ecs", "run-task",
		"--cluster", "cli-ebs-roundtrip",
		"--task-definition", readerTD.TaskDefinition.TaskDefinitionArn,
		"--launch-type", "FARGATE",
		"--network-configuration", `awsvpcConfiguration={subnets=[`+subnetID+`]}`,
		"--volume-configurations", `[{"name":"workspace","managedEBSVolume":{"roleArn":"arn:aws:iam::123456789012:role/ecsInfrastructureRole","snapshotId":"`+snapResult.SnapshotId+`","volumeType":"gp3"}}]`,
		"--output", "json",
	))
	var runReader struct {
		Tasks []struct {
			TaskArn string `json:"taskArn"`
		} `json:"tasks"`
	}
	parseJSON(t, out, &runReader)
	require.Len(t, runReader.Tasks, 1)
	waitCLITaskStatus(t, "cli-ebs-roundtrip", runReader.Tasks[0].TaskArn, "STOPPED")

	out = runCLI(t, awsCLI("logs", "filter-log-events",
		"--log-group-name", "/ecs/cli-ebs-roundtrip",
		"--output", "json",
	))
	var logs struct {
		Events []struct {
			Message string `json:"message"`
		} `json:"events"`
	}
	parseJSON(t, out, &logs)
	var messages []string
	for _, event := range logs.Events {
		messages = append(messages, event.Message)
	}
	assert.Contains(t, strings.Join(messages, "\n"), "CLI_EBS_ROUNDTRIP_OK")
}

func TestECS_CLI_RunTaskNonZeroExit(t *testing.T) {
	subnetID := createCLIECSTestSubnet(t, 144)

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
		"--network-configuration", `awsvpcConfiguration={subnets=[`+subnetID+`]}`,
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
	cleanupCLIECSTask(t, "cli-ecs-fail-cluster", taskArn)

	// Poll until the task reaches STOPPED; netns setup on CI can make a fixed
	// sleep race the real container lifecycle.
	out = pollECSTaskStopped(t, "cli-ecs-fail-cluster", taskArn)

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

// CLI-level coverage for the ECS Tag/Untag handlers.
func TestECS_CLI_TagAndUntagTask(t *testing.T) {
	runCLI(t, awsCLI("ecs", "create-cluster", "--cluster-name", "cli-tag-cluster"))

	out := runCLI(t, awsCLI("ecs", "register-task-definition",
		"--family", "cli-tag-task",
		"--requires-compatibilities", "FARGATE",
		"--network-mode", "awsvpc",
		"--cpu", "256",
		"--memory", "512",
		"--container-definitions", `[{
				"name": "app",
				"image": "alpine:latest",
				"entryPoint": ["sh", "-c"],
				"command": ["sleep 30"]
			}]`,
		"--output", "json",
	))
	var tdResult struct {
		TaskDefinition struct {
			TaskDefinitionArn string `json:"taskDefinitionArn"`
		} `json:"taskDefinition"`
	}
	parseJSON(t, out, &tdResult)

	resourceArn := tdResult.TaskDefinition.TaskDefinitionArn

	runCLI(t, awsCLI("ecs", "tag-resource",
		"--resource-arn", resourceArn,
		"--tags", "key=sockerless-name,value=cli-task",
	))

	out = runCLI(t, awsCLI("ecs", "list-tags-for-resource",
		"--resource-arn", resourceArn,
		"--output", "json",
	))
	var listResult struct {
		Tags []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"tags"`
	}
	parseJSON(t, out, &listResult)

	found := false
	for _, tag := range listResult.Tags {
		if tag.Key == "sockerless-name" && tag.Value == "cli-task" {
			found = true
		}
	}
	assert.True(t, found, "tag must be visible via list-tags-for-resource after tag-resource")

	runCLI(t, awsCLI("ecs", "untag-resource",
		"--resource-arn", resourceArn,
		"--tag-keys", "sockerless-name",
	))

	out = runCLI(t, awsCLI("ecs", "list-tags-for-resource",
		"--resource-arn", resourceArn,
		"--output", "json",
	))
	listResult.Tags = nil
	parseJSON(t, out, &listResult)
	for _, tag := range listResult.Tags {
		assert.NotEqual(t, "sockerless-name", tag.Key, "untagged key should not be present")
	}
}

func waitCLITaskStatus(t *testing.T, clusterName, taskArn, want string) {
	t.Helper()
	require.Eventually(t, func() bool {
		out := runCLI(t, awsCLI("ecs", "describe-tasks",
			"--cluster", clusterName,
			"--tasks", taskArn,
			"--output", "json",
		))
		var desc struct {
			Tasks []struct {
				LastStatus string `json:"lastStatus"`
			} `json:"tasks"`
		}
		parseJSON(t, out, &desc)
		return len(desc.Tasks) == 1 && desc.Tasks[0].LastStatus == want
	}, 20*time.Second, 500*time.Millisecond)
}

func cliEBSVolumeID(t *testing.T, attachments []struct {
	Type    string `json:"type"`
	Details []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"details"`
}) string {
	t.Helper()
	for _, attachment := range attachments {
		if attachment.Type != "AmazonElasticBlockStorage" {
			continue
		}
		for _, detail := range attachment.Details {
			if detail.Name == "volumeId" {
				return detail.Value
			}
		}
	}
	t.Fatal("task did not include an AmazonElasticBlockStorage volume attachment")
	return ""
}
