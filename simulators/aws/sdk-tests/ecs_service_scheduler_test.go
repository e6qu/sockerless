package aws_sdk_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestECS_ServiceScheduler_ReconcilesDesiredCount exercises the background
// ECS service scheduler: a service with DesiredCount=2 converges to two
// RUNNING tasks, the scheduler replaces a manually stopped task, and a
// scale-to-zero drains every task.
func TestECS_ServiceScheduler_ReconcilesDesiredCount(t *testing.T) {
	c := ecsClient()
	cluster := "sched-cluster"
	_, err := c.CreateCluster(ctx, &ecs.CreateClusterInput{ClusterName: aws.String(cluster)})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteService(ctx, &ecs.DeleteServiceInput{
			Cluster: aws.String(cluster), Service: aws.String("sched-svc"), Force: aws.Bool(true),
		})
		_, _ = c.DeleteCluster(ctx, &ecs.DeleteClusterInput{Cluster: aws.String(cluster)})
	})

	_, err = c.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String("sched-task"),
		ContainerDefinitions: []ecstypes.ContainerDefinition{{
			Name:    aws.String("app"),
			Image:   aws.String(containerCommandImage),
			Command: []string{"hold"},
		}},
	})
	require.NoError(t, err)

	createOut, err := c.CreateService(ctx, &ecs.CreateServiceInput{
		Cluster:        aws.String(cluster),
		ServiceName:    aws.String("sched-svc"),
		TaskDefinition: aws.String("sched-task"),
		DesiredCount:   aws.Int32(2),
	})
	require.NoError(t, err)
	require.NotNil(t, createOut.Service)
	// Immediately after CreateService the scheduler has not yet placed tasks;
	// RunningCount must reflect that — never an assumed DesiredCount.
	assert.EqualValues(t, 0, createOut.Service.RunningCount,
		"RunningCount must be 0 immediately after create, before the scheduler places tasks")

	// Poll: scheduler must reach DesiredCount == 2 RUNNING tasks.
	runningArns := waitForRunningTaskCount(t, c, cluster, "sched-svc", 2, 60*time.Second)
	require.Len(t, runningArns, 2, "scheduler must place 2 RUNNING tasks for DesiredCount=2")

	// DescribeServices must now report runningCount == desiredCount.
	descSvc, err := c.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster: aws.String(cluster), Services: []string{"sched-svc"},
	})
	require.NoError(t, err)
	require.Len(t, descSvc.Services, 1)
	assert.EqualValues(t, 2, descSvc.Services[0].RunningCount,
		"DescribeServices runningCount must track the scheduler-placed task set")
	assert.EqualValues(t, 2, descSvc.Services[0].DesiredCount)

	// Stop one task manually. The scheduler must replace it on the next tick.
	stopped := runningArns[0]
	_, err = c.StopTask(ctx, &ecs.StopTaskInput{
		Cluster: aws.String(cluster), Task: aws.String(stopped), Reason: aws.String("test: manual stop"),
	})
	require.NoError(t, err)

	// After replacement, 2 RUNNING tasks must again be present. The replaced
	// task may differ from the original.
	_ = waitForRunningTaskCount(t, c, cluster, "sched-svc", 2, 60*time.Second)

	// Scale to zero. Scheduler must drain every task for the service.
	_, err = c.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster: aws.String(cluster), Service: aws.String("sched-svc"), DesiredCount: aws.Int32(0),
	})
	require.NoError(t, err)
	_ = waitForRunningTaskCount(t, c, cluster, "sched-svc", 0, 60*time.Second)

	descSvc, err = c.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster: aws.String(cluster), Services: []string{"sched-svc"},
	})
	require.NoError(t, err)
	require.Len(t, descSvc.Services, 1)
	assert.EqualValues(t, 0, descSvc.Services[0].RunningCount,
		"DescribeServices runningCount must be 0 after DesiredCount scale-to-zero")
}

// waitForRunningTaskCount polls ListTasks (desired=RUNNING, scoped to the
// service via the startedBy tag) until exactly n tasks are RUNNING for the
// service, returning their ARNs. Fails the test on timeout.
func waitForRunningTaskCount(t *testing.T, c *ecs.Client, cluster, service string, n int, timeout time.Duration) []string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastArns []string
	for time.Now().Before(deadline) {
		listOut, err := c.ListTasks(ctx, &ecs.ListTasksInput{
			Cluster:       aws.String(cluster),
			DesiredStatus: ecstypes.DesiredStatusRunning,
			StartedBy:     aws.String("ecs-svc/" + service),
		})
		if err == nil {
			lastArns = listOut.TaskArns
			if runningTaskCount(t, c, cluster, listOut.TaskArns) == n {
				return listOut.TaskArns
			}
		}
		time.Sleep(1 * time.Second)
	}
	if len(lastArns) == 0 {
		t.Fatalf("timed out waiting for %d RUNNING tasks for service %s (none observed)", n, service)
	} else {
		// Describe the last observed set for diagnostics.
		descOut, _ := c.DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: aws.String(cluster), Tasks: lastArns,
		})
		statuses := make([]string, 0, len(descOut.Tasks))
		for _, tk := range descOut.Tasks {
			statuses = append(statuses, fmt.Sprintf("%s=%s",
				aws.ToString(tk.TaskArn), aws.ToString(tk.LastStatus)))
		}
		t.Fatalf("timed out waiting for %d RUNNING tasks for service %s; last observed: %v",
			n, service, statuses)
	}
	return lastArns
}

// runningTaskCount describes the listed task ARNs and counts those whose
// LastStatus is RUNNING.
func runningTaskCount(t *testing.T, c *ecs.Client, cluster string, arns []string) int {
	t.Helper()
	if len(arns) == 0 {
		return 0
	}
	descOut, err := c.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(cluster), Tasks: arns,
	})
	require.NoError(t, err)
	count := 0
	for _, tk := range descOut.Tasks {
		if aws.ToString(tk.LastStatus) == "RUNNING" {
			count++
		}
	}
	return count
}
