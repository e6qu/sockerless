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

// TestECS_Service_ReconcilesRealTasks proves that Amazon ECS services own real
// task-definition workloads. It covers initial placement, replacement after a
// service task is stopped, rolling task-definition replacement, scale-out,
// scale-in, and delete-time draining through the official ECS client.
func TestECS_Service_ReconcilesRealTasks(t *testing.T) {
	client := ecsClient()
	cluster := "sched-cluster"
	const serviceName = "sched-svc"
	_, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{ClusterName: aws.String(cluster)})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = client.UpdateService(ctx, &ecs.UpdateServiceInput{
			Cluster: aws.String(cluster), Service: aws.String(serviceName), DesiredCount: aws.Int32(0),
		})
		_, _ = client.DeleteService(ctx, &ecs.DeleteServiceInput{
			Cluster: aws.String(cluster), Service: aws.String(serviceName), Force: aws.Bool(true),
		})
		_, _ = client.DeleteCluster(ctx, &ecs.DeleteClusterInput{Cluster: aws.String(cluster)})
	})

	register := func(command string) string {
		out, registerErr := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
			Family: aws.String("sched-task"),
			ContainerDefinitions: []ecstypes.ContainerDefinition{{
				Name:      aws.String("app"),
				Image:     aws.String(containerCommandImage),
				Command:   []string{command},
				Essential: aws.Bool(true),
			}},
		})
		require.NoError(t, registerErr)
		return aws.ToString(out.TaskDefinition.TaskDefinitionArn)
	}
	firstRevision := register("hold")

	created, err := client.CreateService(ctx, &ecs.CreateServiceInput{
		Cluster:        aws.String(cluster),
		ServiceName:    aws.String(serviceName),
		TaskDefinition: aws.String(firstRevision),
		DesiredCount:   aws.Int32(2),
	})
	require.NoError(t, err)
	require.NotNil(t, created.Service)
	assert.EqualValues(t, 2, created.Service.DesiredCount)

	runningTasks := func() []string {
		listed, listErr := client.ListTasks(ctx, &ecs.ListTasksInput{
			Cluster: aws.String(cluster), ServiceName: aws.String(serviceName),
			DesiredStatus: ecstypes.DesiredStatusRunning,
		})
		if listErr != nil {
			return nil
		}
		return listed.TaskArns
	}
	serviceIsSteady := func(desired int32) bool {
		described, describeErr := client.DescribeServices(ctx, &ecs.DescribeServicesInput{
			Cluster: aws.String(cluster), Services: []string{serviceName},
		})
		if describeErr != nil || len(described.Services) != 1 {
			return false
		}
		service := described.Services[0]
		return service.DesiredCount == desired &&
			service.RunningCount == desired &&
			service.PendingCount == 0 &&
			len(service.Deployments) == 1 &&
			service.Deployments[0].RolloutState == ecstypes.DeploymentRolloutStateCompleted
	}

	require.Eventually(t, func() bool {
		return serviceIsSteady(2) && len(runningTasks()) == 2
	}, 30*time.Second, 100*time.Millisecond, "service did not launch two real tasks")

	beforeStop := runningTasks()
	require.Len(t, beforeStop, 2)
	_, err = client.StopTask(ctx, &ecs.StopTaskInput{
		Cluster: aws.String(cluster), Task: aws.String(beforeStop[0]),
		Reason: aws.String("prove service replacement"),
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		after := runningTasks()
		return len(after) == 2 && !containsString(after, beforeStop[0]) && serviceIsSteady(2)
	}, 30*time.Second, 100*time.Millisecond, "service did not replace a stopped task")

	secondRevision := register("hold")
	_, err = client.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster: aws.String(cluster), Service: aws.String(serviceName),
		TaskDefinition: aws.String(secondRevision), DesiredCount: aws.Int32(3),
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		taskArns := runningTasks()
		if len(taskArns) != 3 || !serviceIsSteady(3) {
			return false
		}
		described, describeErr := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: aws.String(cluster), Tasks: taskArns,
		})
		if describeErr != nil || len(described.Tasks) != 3 {
			return false
		}
		for _, task := range described.Tasks {
			if aws.ToString(task.TaskDefinitionArn) != secondRevision {
				return false
			}
		}
		return true
	}, 30*time.Second, 100*time.Millisecond, "rolling deployment did not replace every task")

	_, err = client.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster: aws.String(cluster), Service: aws.String(serviceName), DesiredCount: aws.Int32(0),
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return serviceIsSteady(0) && len(runningTasks()) == 0
	}, 30*time.Second, 100*time.Millisecond, "scale-to-zero did not drain service tasks")

	deleted, err := client.DeleteService(ctx, &ecs.DeleteServiceInput{
		Cluster: aws.String(cluster), Service: aws.String(serviceName), Force: aws.Bool(true),
	})
	require.NoError(t, err)
	assert.Equal(t, "INACTIVE", aws.ToString(deleted.Service.Status))
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
