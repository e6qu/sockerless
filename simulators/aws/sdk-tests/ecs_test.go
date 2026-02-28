package aws_sdk_test

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
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
				Image: aws.String("nginx:latest"),
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "test-task", *out.TaskDefinition.Family)
	assert.Equal(t, int32(1), out.TaskDefinition.Revision)
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
				Name:  aws.String("app"),
				Image: aws.String("alpine:latest"),
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
				Subnets: []string{"subnet-12345"},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, runOut.Tasks, 1)
	taskArn := *runOut.Tasks[0].TaskArn

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
				Subnets: []string{"subnet-12345"},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, runOut.Tasks, 1)
	taskArn := *runOut.Tasks[0].TaskArn

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
