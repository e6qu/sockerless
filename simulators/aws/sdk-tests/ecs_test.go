package aws_sdk_test

import (
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ecsClient() *ecs.Client {
	return ecs.NewFromConfig(sdkConfig(), func(o *ecs.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func TestECS_CreateCluster(t *testing.T) {
	client := ecsClient()
	out, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String("test-cluster"),
	})
	require.NoError(t, err)
	assert.Equal(t, "test-cluster", *out.Cluster.ClusterName)
	assert.Contains(t, *out.Cluster.ClusterArn, "test-cluster")
}

func TestECS_DescribeClusters(t *testing.T) {
	client := ecsClient()

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String("describe-cluster"),
	})
	require.NoError(t, err)

	out, err := client.DescribeClusters(ctx, &ecs.DescribeClustersInput{
		Clusters: []string{"describe-cluster"},
	})
	require.NoError(t, err)
	require.Len(t, out.Clusters, 1)
	assert.Equal(t, "describe-cluster", *out.Clusters[0].ClusterName)
}

func TestECS_RegisterTaskDefinition(t *testing.T) {
	client := ecsClient()
	out, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String("test-task"),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{
				Name:  aws.String("app"),
				Image: aws.String("alpine:latest"),
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "test-task", *out.TaskDefinition.Family)
	assert.Equal(t, int32(1), out.TaskDefinition.Revision)
}

func TestECS_MultiContainerTaskSharesLocalhost(t *testing.T) {
	client := ecsClient()

	clusterName := "pod-localhost"
	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	logGroupName := "/ecs/pod-localhost"
	cw := cwLogsClient()
	_, _ = cw.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{LogGroupName: aws.String(logGroupName)})
	t.Cleanup(func() {
		_, _ = cw.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{LogGroupName: aws.String(logGroupName)})
	})

	tdOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String("pod-localhost"),
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
		Cpu:                     aws.String("256"),
		Memory:                  aws.String("512"),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{
				Name:       aws.String("main"),
				Image:      aws.String(evalImageName),
				EntryPoint: []string{"sh", "-c"},
				Command: []string{`for i in $(seq 1 50); do
if nc -z 127.0.0.1 9090; then echo sidecar-ok; exit 0; fi
sleep 0.1
done
echo sidecar-missing
exit 1`},
				LogConfiguration: &ecstypes.LogConfiguration{
					LogDriver: ecstypes.LogDriverAwslogs,
					Options: map[string]string{
						"awslogs-group":         logGroupName,
						"awslogs-stream-prefix": "ecs",
					},
				},
			},
			{
				Name:       aws.String("sidecar"),
				Image:      aws.String(evalImageName),
				EntryPoint: []string{"sh", "-c"},
				Command:    []string{`while true; do { printf 'HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok'; } | nc -l -p 9090; done`},
			},
		},
	})
	require.NoError(t, err)

	runOut, err := client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        aws.String(clusterName),
		TaskDefinition: tdOut.TaskDefinition.TaskDefinitionArn,
		LaunchType:     ecstypes.LaunchTypeFargate,
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				Subnets: []string{"subnet-0123456789abcdef0"},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, runOut.Tasks, 1)
	taskArn := *runOut.Tasks[0].TaskArn
	cleanupECSTask(t, client, clusterName, taskArn)

	require.Eventually(t, func() bool {
		desc, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: aws.String(clusterName),
			Tasks:   []string{taskArn},
		})
		if err != nil || len(desc.Tasks) != 1 {
			return false
		}
		return desc.Tasks[0].LastStatus != nil && *desc.Tasks[0].LastStatus == "STOPPED"
	}, 20*time.Second, 500*time.Millisecond)

	events, err := cw.FilterLogEvents(ctx, &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String(logGroupName),
	})
	require.NoError(t, err)
	var messages []string
	for _, e := range events.Events {
		messages = append(messages, aws.ToString(e.Message))
	}
	assert.Contains(t, strings.Join(messages, "\n"), "sidecar-ok")
}

func TestECS_ManagedEBSVolumeSnapshotRoundTripSDK(t *testing.T) {
	client := ecsClient()
	ec2c := ec2Client()
	cw := cwLogsClient()

	clusterName := "managed-ebs-roundtrip"
	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{ClusterName: aws.String(clusterName)})
	require.NoError(t, err)

	logGroupName := "/ecs/managed-ebs-roundtrip"
	_, _ = cw.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{LogGroupName: aws.String(logGroupName)})
	t.Cleanup(func() {
		_, _ = cw.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{LogGroupName: aws.String(logGroupName)})
	})

	tdOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String("managed-ebs-roundtrip"),
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
		Cpu:                     aws.String("256"),
		Memory:                  aws.String("512"),
		Volumes: []ecstypes.Volume{{
			Name:               aws.String("workspace"),
			ConfiguredAtLaunch: aws.Bool(true),
		}},
		ContainerDefinitions: []ecstypes.ContainerDefinition{{
			Name:       aws.String("writer"),
			Image:      aws.String(evalImageName),
			EntryPoint: []string{"sh", "-c"},
			Command:    []string{"printf 'sockerless-ebs-roundtrip' > /workspace/state.txt"},
			MountPoints: []ecstypes.MountPoint{{
				SourceVolume:  aws.String("workspace"),
				ContainerPath: aws.String("/workspace"),
			}},
		}},
	})
	require.NoError(t, err)

	keepVolume := false
	runOut, err := client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        aws.String(clusterName),
		TaskDefinition: tdOut.TaskDefinition.TaskDefinitionArn,
		LaunchType:     ecstypes.LaunchTypeFargate,
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{Subnets: []string{"subnet-0123456789abcdef0"}},
		},
		VolumeConfigurations: []ecstypes.TaskVolumeConfiguration{{
			Name: aws.String("workspace"),
			ManagedEBSVolume: &ecstypes.TaskManagedEBSVolumeConfiguration{
				RoleArn:    aws.String("arn:aws:iam::123456789012:role/ecsInfrastructureRole"),
				SizeInGiB:  aws.Int32(1),
				VolumeType: aws.String("gp3"),
				TerminationPolicy: &ecstypes.TaskManagedEBSVolumeTerminationPolicy{
					DeleteOnTermination: aws.Bool(keepVolume),
				},
				TagSpecifications: []ecstypes.EBSTagSpecification{{
					ResourceType: ecstypes.EBSResourceTypeVolume,
					Tags:         []ecstypes.Tag{{Key: aws.String("purpose"), Value: aws.String("roundtrip")}},
				}},
			},
		}},
	})
	require.NoError(t, err)
	require.Len(t, runOut.Tasks, 1)
	writerTaskArn := aws.ToString(runOut.Tasks[0].TaskArn)
	waitForECSTaskStatus(t, client, clusterName, writerTaskArn, "STOPPED")
	writerDesc, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(clusterName),
		Tasks:   []string{writerTaskArn},
	})
	require.NoError(t, err)
	volumeID := ebsVolumeIDFromTask(t, writerDesc.Tasks[0])
	t.Cleanup(func() {
		_, _ = ec2c.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: aws.String(volumeID)})
	})

	snapshotOut, err := ec2c.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
		VolumeId:    aws.String(volumeID),
		Description: aws.String("ecs managed ebs roundtrip"),
	})
	require.NoError(t, err)
	snapshotID := aws.ToString(snapshotOut.SnapshotId)
	require.NotEmpty(t, snapshotID)
	waitForEC2SnapshotState(t, ec2c, snapshotID, "completed")
	t.Cleanup(func() {
		_, _ = ec2c.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{SnapshotId: aws.String(snapshotID)})
	})

	readerTD, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String("managed-ebs-reader"),
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
		Cpu:                     aws.String("256"),
		Memory:                  aws.String("512"),
		Volumes: []ecstypes.Volume{{
			Name:               aws.String("workspace"),
			ConfiguredAtLaunch: aws.Bool(true),
		}},
		ContainerDefinitions: []ecstypes.ContainerDefinition{{
			Name:       aws.String("reader"),
			Image:      aws.String(evalImageName),
			EntryPoint: []string{"sh", "-c"},
			Command: []string{`test "$(cat /workspace/state.txt)" = "sockerless-ebs-roundtrip"
echo EBS_ROUNDTRIP_OK`},
			MountPoints: []ecstypes.MountPoint{{
				SourceVolume:  aws.String("workspace"),
				ContainerPath: aws.String("/workspace"),
			}},
			LogConfiguration: &ecstypes.LogConfiguration{
				LogDriver: ecstypes.LogDriverAwslogs,
				Options: map[string]string{
					"awslogs-group":         logGroupName,
					"awslogs-stream-prefix": "ecs",
				},
			},
		}},
	})
	require.NoError(t, err)

	runReader, err := client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        aws.String(clusterName),
		TaskDefinition: readerTD.TaskDefinition.TaskDefinitionArn,
		LaunchType:     ecstypes.LaunchTypeFargate,
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{Subnets: []string{"subnet-0123456789abcdef0"}},
		},
		VolumeConfigurations: []ecstypes.TaskVolumeConfiguration{{
			Name: aws.String("workspace"),
			ManagedEBSVolume: &ecstypes.TaskManagedEBSVolumeConfiguration{
				RoleArn:    aws.String("arn:aws:iam::123456789012:role/ecsInfrastructureRole"),
				SnapshotId: aws.String(snapshotID),
				VolumeType: aws.String("gp3"),
			},
		}},
	})
	require.NoError(t, err)
	require.Len(t, runReader.Tasks, 1)
	readerTaskArn := aws.ToString(runReader.Tasks[0].TaskArn)
	waitForECSTaskStatus(t, client, clusterName, readerTaskArn, "STOPPED")

	events, err := cw.FilterLogEvents(ctx, &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String(logGroupName),
	})
	require.NoError(t, err)
	var messages []string
	for _, e := range events.Events {
		messages = append(messages, aws.ToString(e.Message))
	}
	assert.Contains(t, strings.Join(messages, "\n"), "EBS_ROUNDTRIP_OK")
}

func TestECS_ExitCodeNilWhileRunning(t *testing.T) {
	client := ecsClient()

	// Setup: cluster + task definition
	clusterName := "exitcode-test-cluster"
	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	tdOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String("exitcode-task"),
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		Cpu:                     aws.String("256"),
		Memory:                  aws.String("512"),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{
				Name:    aws.String("app"),
				Image:   aws.String("alpine:latest"),
				Command: []string{"sleep", "30"}, // long-running so RUNNING window is real
			},
		},
	})
	require.NoError(t, err)

	// Run task
	runOut, err := client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        aws.String(clusterName),
		TaskDefinition: aws.String(*tdOut.TaskDefinition.TaskDefinitionArn),
		Count:          aws.Int32(1),
		LaunchType:     ecstypes.LaunchTypeFargate,
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				Subnets: []string{"subnet-0123456789abcdef0"},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, runOut.Tasks, 1)
	taskArn := *runOut.Tasks[0].TaskArn
	cleanupECSTask(t, client, clusterName, taskArn)

	// Wait briefly for task to transition to RUNNING (500ms in simulator)
	time.Sleep(800 * time.Millisecond)

	// Describe task while RUNNING — ExitCode should be nil
	descOut, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(clusterName),
		Tasks:   []string{taskArn},
	})
	require.NoError(t, err)
	require.Len(t, descOut.Tasks, 1)
	require.NotEmpty(t, descOut.Tasks[0].Containers)

	runningTask := descOut.Tasks[0]
	assert.Equal(t, "RUNNING", *runningTask.LastStatus)
	for _, c := range runningTask.Containers {
		assert.Nil(t, c.ExitCode, "ExitCode should be nil while task is RUNNING")
	}

	// Stop task explicitly (real ECS has no task timeout — tasks run until stopped)
	_, err = client.StopTask(ctx, &ecs.StopTaskInput{
		Cluster: aws.String(clusterName),
		Task:    aws.String(taskArn),
	})
	require.NoError(t, err)

	// Describe task after STOPPED — ExitCode should be set
	descOut2, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(clusterName),
		Tasks:   []string{taskArn},
	})
	require.NoError(t, err)
	require.Len(t, descOut2.Tasks, 1)

	stoppedTask := descOut2.Tasks[0]
	assert.Equal(t, "STOPPED", *stoppedTask.LastStatus)
	assert.Equal(t, ecstypes.TaskStopCodeUserInitiated, stoppedTask.StopCode)
	for _, c := range stoppedTask.Containers {
		require.NotNil(t, c.ExitCode, "ExitCode should be set when task is STOPPED")
		assert.Equal(t, int32(0), *c.ExitCode)
	}
}

func TestECS_StopCodeUserInitiated(t *testing.T) {
	client := ecsClient()

	clusterName := "stopcode-user-cluster"
	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	tdOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String("stopcode-task"),
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		Cpu:                     aws.String("256"),
		Memory:                  aws.String("512"),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{
				Name:  aws.String("app"),
				Image: aws.String("alpine:latest"),
			},
		},
	})
	require.NoError(t, err)

	runOut, err := client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        aws.String(clusterName),
		TaskDefinition: aws.String(*tdOut.TaskDefinition.TaskDefinitionArn),
		Count:          aws.Int32(1),
		LaunchType:     ecstypes.LaunchTypeFargate,
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				Subnets: []string{"subnet-0123456789abcdef0"},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, runOut.Tasks, 1)
	taskArn := *runOut.Tasks[0].TaskArn
	cleanupECSTask(t, client, clusterName, taskArn)

	// Wait for RUNNING
	time.Sleep(800 * time.Millisecond)

	// Stop task via API
	_, err = client.StopTask(ctx, &ecs.StopTaskInput{
		Cluster: aws.String(clusterName),
		Task:    aws.String(taskArn),
		Reason:  aws.String("testing stop"),
	})
	require.NoError(t, err)

	// Describe — StopCode should be UserInitiated
	descOut, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(clusterName),
		Tasks:   []string{taskArn},
	})
	require.NoError(t, err)
	require.Len(t, descOut.Tasks, 1)

	task := descOut.Tasks[0]
	assert.Equal(t, "STOPPED", *task.LastStatus)
	assert.Equal(t, ecstypes.TaskStopCodeUserInitiated, task.StopCode)
	assert.Equal(t, "testing stop", *task.StoppedReason)
}

// ecsRunTaskHelper creates a cluster, registers a task definition, and runs a task.
// Returns the ECS client, cluster name, and task ARN.
func ecsRunTaskHelper(t *testing.T, name string, containerDef ecstypes.ContainerDefinition) (*ecs.Client, string, string) {
	t.Helper()
	client := ecsClient()
	clusterName := name + "-cluster"

	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: aws.String(clusterName),
	})
	require.NoError(t, err)

	tdOut, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String(name + "-task"),
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		Cpu:                     aws.String("256"),
		Memory:                  aws.String("512"),
		ContainerDefinitions:    []ecstypes.ContainerDefinition{containerDef},
	})
	require.NoError(t, err)

	runOut, err := client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        aws.String(clusterName),
		TaskDefinition: aws.String(*tdOut.TaskDefinition.TaskDefinitionArn),
		Count:          aws.Int32(1),
		LaunchType:     ecstypes.LaunchTypeFargate,
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				Subnets: []string{"subnet-0123456789abcdef0"},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, runOut.Tasks, 1)

	taskArn := *runOut.Tasks[0].TaskArn
	cleanupECSTask(t, client, clusterName, taskArn)

	return client, clusterName, taskArn
}

func cleanupECSTask(t *testing.T, client *ecs.Client, clusterName, taskArn string) {
	t.Helper()
	t.Cleanup(func() {
		_, err := client.StopTask(ctx, &ecs.StopTaskInput{
			Cluster: aws.String(clusterName),
			Task:    aws.String(taskArn),
			Reason:  aws.String("test cleanup"),
		})
		require.NoError(t, err)
	})
}

func waitForECSTaskStatus(t *testing.T, client *ecs.Client, clusterName, taskArn, want string) {
	t.Helper()
	require.Eventually(t, func() bool {
		desc, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: aws.String(clusterName),
			Tasks:   []string{taskArn},
		})
		if err != nil || len(desc.Tasks) != 1 || desc.Tasks[0].LastStatus == nil {
			return false
		}
		return aws.ToString(desc.Tasks[0].LastStatus) == want
	}, 20*time.Second, 500*time.Millisecond)
}

func ebsVolumeIDFromTask(t *testing.T, task ecstypes.Task) string {
	t.Helper()
	for _, att := range task.Attachments {
		if aws.ToString(att.Type) != "AmazonElasticBlockStorage" {
			continue
		}
		for _, detail := range att.Details {
			if aws.ToString(detail.Name) == "volumeId" {
				return aws.ToString(detail.Value)
			}
		}
	}
	t.Fatalf("task %s did not include an AmazonElasticBlockStorage volume attachment", aws.ToString(task.TaskArn))
	return ""
}

func TestECS_TaskExecutesCommand(t *testing.T) {
	client, cluster, taskArn := ecsRunTaskHelper(t, "exec-cmd", ecstypes.ContainerDefinition{
		Name:    aws.String("app"),
		Image:   aws.String("alpine:latest"),
		Command: []string{"echo", "hello"},
		LogConfiguration: &ecstypes.LogConfiguration{
			LogDriver: ecstypes.LogDriverAwslogs,
			Options: map[string]string{
				"awslogs-group":         "/ecs/exec-cmd",
				"awslogs-stream-prefix": "ecs",
			},
		},
	})

	// Wait for container to complete (image pull + start + command execution)
	time.Sleep(10 * time.Second)

	descOut, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(cluster),
		Tasks:   []string{taskArn},
	})
	require.NoError(t, err)
	require.Len(t, descOut.Tasks, 1)

	task := descOut.Tasks[0]
	assert.Equal(t, "STOPPED", *task.LastStatus)
	require.NotEmpty(t, task.Containers)
	require.NotNil(t, task.Containers[0].ExitCode)
	assert.Equal(t, int32(0), *task.Containers[0].ExitCode)
}

func TestECS_TaskExitCodeNonZero(t *testing.T) {
	client, cluster, taskArn := ecsRunTaskHelper(t, "exec-fail", ecstypes.ContainerDefinition{
		Name:    aws.String("app"),
		Image:   aws.String("alpine:latest"),
		Command: []string{"sh", "-c", "exit 1"},
		LogConfiguration: &ecstypes.LogConfiguration{
			LogDriver: ecstypes.LogDriverAwslogs,
			Options: map[string]string{
				"awslogs-group":         "/ecs/exec-fail",
				"awslogs-stream-prefix": "ecs",
			},
		},
	})

	time.Sleep(2 * time.Second)

	descOut, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(cluster),
		Tasks:   []string{taskArn},
	})
	require.NoError(t, err)
	require.Len(t, descOut.Tasks, 1)

	task := descOut.Tasks[0]
	assert.Equal(t, "STOPPED", *task.LastStatus)
	require.NotEmpty(t, task.Containers)
	require.NotNil(t, task.Containers[0].ExitCode)
	assert.Equal(t, int32(1), *task.Containers[0].ExitCode)
}

func TestECS_TaskLogsToCloudWatch(t *testing.T) {
	_, _, _ = ecsRunTaskHelper(t, "exec-logs", ecstypes.ContainerDefinition{
		Name:    aws.String("app"),
		Image:   aws.String("alpine:latest"),
		Command: []string{"echo", "hello from process"},
		LogConfiguration: &ecstypes.LogConfiguration{
			LogDriver: ecstypes.LogDriverAwslogs,
			Options: map[string]string{
				"awslogs-group":         "/ecs/exec-logs",
				"awslogs-stream-prefix": "ecs",
			},
		},
	})

	cw := cwLogsClient()

	// Poll until the process stdout reaches CloudWatch. Image pull +
	// container start latency on slow CI runners can exceed any fixed sleep.
	var messages []string
	require.Eventually(t, func() bool {
		streams, serr := cw.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
			LogGroupName: aws.String("/ecs/exec-logs"),
		})
		if serr != nil || len(streams.LogStreams) == 0 {
			return false
		}
		out, err := cw.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
			LogGroupName:  aws.String("/ecs/exec-logs"),
			LogStreamName: streams.LogStreams[0].LogStreamName,
		})
		if err != nil {
			return false
		}
		messages = messages[:0]
		for _, e := range out.Events {
			messages = append(messages, *e.Message)
			if *e.Message == "hello from process" {
				return true
			}
		}
		return false
	}, 30*time.Second, 250*time.Millisecond, "process stdout should reach CloudWatch logs; saw=%v", messages)
}

func TestECS_TaskNoCommandStaysRunning(t *testing.T) {
	client, cluster, taskArn := ecsRunTaskHelper(t, "exec-nocmd", ecstypes.ContainerDefinition{
		Name:    aws.String("app"),
		Image:   aws.String("alpine:latest"),
		Command: []string{"tail", "-f", "/dev/null"}, // Long-running — stays RUNNING
		LogConfiguration: &ecstypes.LogConfiguration{
			LogDriver: ecstypes.LogDriverAwslogs,
			Options: map[string]string{
				"awslogs-group":         "/ecs/exec-nocmd",
				"awslogs-stream-prefix": "ecs",
			},
		},
	})

	// Wait a bit and verify still RUNNING
	time.Sleep(1500 * time.Millisecond)

	descOut, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(cluster),
		Tasks:   []string{taskArn},
	})
	require.NoError(t, err)
	require.Len(t, descOut.Tasks, 1)

	task := descOut.Tasks[0]
	assert.Equal(t, "RUNNING", *task.LastStatus, "task with no command should stay RUNNING")
	for _, c := range task.Containers {
		assert.Nil(t, c.ExitCode, "ExitCode should be nil while RUNNING")
	}
}

// TagResource/UntagResource contract: tag a running task, list tags,
// untag, and confirm STOPPED tasks reject tagging.
func TestECS_TagResource_OnRunningTask(t *testing.T) {
	client, cluster, taskArn := ecsRunTaskHelper(t, "tag-task", ecstypes.ContainerDefinition{
		Name:    aws.String("app"),
		Image:   aws.String("alpine:latest"),
		Command: []string{"tail", "-f", "/dev/null"},
	})
	_ = cluster

	_, err := client.TagResource(ctx, &ecs.TagResourceInput{
		ResourceArn: aws.String(taskArn),
		Tags: []ecstypes.Tag{
			{Key: aws.String("sockerless-name"), Value: aws.String("my-task")},
			{Key: aws.String("sockerless-restart-count"), Value: aws.String("0")},
		},
	})
	require.NoError(t, err)

	listOut, err := client.ListTagsForResource(ctx, &ecs.ListTagsForResourceInput{
		ResourceArn: aws.String(taskArn),
	})
	require.NoError(t, err)

	got := map[string]string{}
	for _, tag := range listOut.Tags {
		got[*tag.Key] = *tag.Value
	}
	assert.Equal(t, "my-task", got["sockerless-name"])
	assert.Equal(t, "0", got["sockerless-restart-count"])

	// Overwrite an existing key — merge-by-key semantics.
	_, err = client.TagResource(ctx, &ecs.TagResourceInput{
		ResourceArn: aws.String(taskArn),
		Tags: []ecstypes.Tag{
			{Key: aws.String("sockerless-restart-count"), Value: aws.String("3")},
		},
	})
	require.NoError(t, err)

	listOut, err = client.ListTagsForResource(ctx, &ecs.ListTagsForResourceInput{
		ResourceArn: aws.String(taskArn),
	})
	require.NoError(t, err)
	got = map[string]string{}
	for _, tag := range listOut.Tags {
		got[*tag.Key] = *tag.Value
	}
	assert.Equal(t, "my-task", got["sockerless-name"], "existing key should persist after partial update")
	assert.Equal(t, "3", got["sockerless-restart-count"], "matching key should be overwritten")

	// Untag one key.
	_, err = client.UntagResource(ctx, &ecs.UntagResourceInput{
		ResourceArn: aws.String(taskArn),
		TagKeys:     []string{"sockerless-restart-count"},
	})
	require.NoError(t, err)

	listOut, err = client.ListTagsForResource(ctx, &ecs.ListTagsForResourceInput{
		ResourceArn: aws.String(taskArn),
	})
	require.NoError(t, err)
	got = map[string]string{}
	for _, tag := range listOut.Tags {
		got[*tag.Key] = *tag.Value
	}
	_, ok := got["sockerless-restart-count"]
	assert.False(t, ok, "untagged key should be gone")
	assert.Equal(t, "my-task", got["sockerless-name"], "non-untagged key should remain")
}

func TestECS_TagResource_RejectsStoppedTask(t *testing.T) {
	client, cluster, taskArn := ecsRunTaskHelper(t, "tag-stopped", ecstypes.ContainerDefinition{
		Name:    aws.String("app"),
		Image:   aws.String("alpine:latest"),
		Command: []string{"sh", "-c", "exit 0"},
	})

	// Poll for STOPPED — podman lifecycle (image pull + start + exit + sim
	// state update) can take >8s under CI contention; a fixed sleep flakes.
	var descOut *ecs.DescribeTasksOutput
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		descOut, err = client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: aws.String(cluster),
			Tasks:   []string{taskArn},
		})
		require.NoError(t, err)
		require.Len(t, descOut.Tasks, 1)
		if *descOut.Tasks[0].LastStatus == "STOPPED" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.Equal(t, "STOPPED", *descOut.Tasks[0].LastStatus, "task should be STOPPED before this assertion")

	// Real ECS rejects TagResource on STOPPED tasks; sim must too.
	_, err := client.TagResource(ctx, &ecs.TagResourceInput{
		ResourceArn: aws.String(taskArn),
		Tags: []ecstypes.Tag{
			{Key: aws.String("sockerless-name"), Value: aws.String("late-tag")},
		},
	})
	require.Error(t, err, "TagResource on a STOPPED task should fail with InvalidParameterException")
}

func TestECS_ListTasks_Pagination(t *testing.T) {
	client := ecsClient()
	cluster := "pag-cluster"
	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{ClusterName: aws.String(cluster)})
	require.NoError(t, err)
	t.Cleanup(func() {
		client.DeleteCluster(ctx, &ecs.DeleteClusterInput{Cluster: aws.String(cluster)})
	})

	td, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String("pag-family"),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Name: aws.String("app"), Image: aws.String("alpine:latest")},
		},
		NetworkMode: ecstypes.NetworkModeAwsvpc,
	})
	require.NoError(t, err)
	tdArn := aws.ToString(td.TaskDefinition.TaskDefinitionArn)

	// Run 3 tasks.
	for i := 0; i < 3; i++ {
		_, err = client.RunTask(ctx, &ecs.RunTaskInput{
			Cluster:        aws.String(cluster),
			TaskDefinition: aws.String(tdArn),
		})
		require.NoError(t, err)
	}

	// Page with MaxResults=1 — should need 3 pages to see all tasks.
	seen := map[string]bool{}
	var token *string
	for {
		out, err := client.ListTasks(ctx, &ecs.ListTasksInput{
			Cluster:    aws.String(cluster),
			MaxResults: aws.Int32(1),
			NextToken:  token,
		})
		require.NoError(t, err)
		for _, arn := range out.TaskArns {
			seen[arn] = true
		}
		if out.NextToken == nil {
			break
		}
		token = out.NextToken
	}
	assert.Equal(t, 3, len(seen), "should see all 3 task ARNs via pagination")
}
