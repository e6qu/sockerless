package aws_sdk_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestECS_Service_ControlPlaneConvergence exercises the ECS service control-plane
// state machine: the modeled service reaches runningCount == desiredCount with a
// COMPLETED primary deployment synchronously (no background scheduler launches
// ephemeral containers), scale up/down through UpdateService tracks desiredCount,
// scale-to-zero settles runningCount to 0, and DeleteService drains to INACTIVE.
func TestECS_Service_ControlPlaneConvergence(t *testing.T) {
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
	assert.EqualValues(t, 2, createOut.Service.DesiredCount)
	// The modeled service reaches steady state synchronously.
	assert.EqualValues(t, 2, createOut.Service.RunningCount,
		"CreateService must reach runningCount == desiredCount as a modeled control-plane state")
	assert.EqualValues(t, 0, createOut.Service.PendingCount)
	require.NotEmpty(t, createOut.Service.Deployments, "service must have a PRIMARY deployment")
	assert.Equal(t, "COMPLETED", string(createOut.Service.Deployments[0].RolloutState),
		"the primary deployment must report COMPLETED")
	assert.EqualValues(t, 2, createOut.Service.Deployments[0].RunningCount,
		"the primary deployment's runningCount must track desiredCount")

	// DescribeServices returns the same converged snapshot.
	descSvc, err := c.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster: aws.String(cluster), Services: []string{"sched-svc"},
	})
	require.NoError(t, err)
	require.Len(t, descSvc.Services, 1)
	assert.EqualValues(t, 2, descSvc.Services[0].RunningCount,
		"DescribeServices runningCount must equal desiredCount")
	assert.EqualValues(t, 2, descSvc.Services[0].DesiredCount)
	assert.Equal(t, "ACTIVE", aws.ToString(descSvc.Services[0].Status))

	// Scale up to 3 via UpdateService: runningCount tracks the new desiredCount.
	updOut, err := c.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster: aws.String(cluster), Service: aws.String("sched-svc"), DesiredCount: aws.Int32(3),
	})
	require.NoError(t, err)
	assert.EqualValues(t, 3, updOut.Service.DesiredCount)
	assert.EqualValues(t, 3, updOut.Service.RunningCount,
		"UpdateService must keep runningCount in lockstep with desiredCount")

	// Scale to zero: runningCount settles to 0.
	updOut, err = c.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster: aws.String(cluster), Service: aws.String("sched-svc"), DesiredCount: aws.Int32(0),
	})
	require.NoError(t, err)
	assert.EqualValues(t, 0, updOut.Service.DesiredCount)
	assert.EqualValues(t, 0, updOut.Service.RunningCount,
		"scale-to-zero must drop runningCount to 0")

	descSvc, err = c.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster: aws.String(cluster), Services: []string{"sched-svc"},
	})
	require.NoError(t, err)
	require.Len(t, descSvc.Services, 1)
	assert.EqualValues(t, 0, descSvc.Services[0].RunningCount,
		"DescribeServices runningCount must be 0 after DesiredCount scale-to-zero")

	// DeleteService settles the service to INACTIVE.
	delOut, err := c.DeleteService(ctx, &ecs.DeleteServiceInput{
		Cluster: aws.String(cluster), Service: aws.String("sched-svc"), Force: aws.Bool(true),
	})
	require.NoError(t, err)
	require.NotNil(t, delOut.Service)
	assert.Equal(t, "INACTIVE", aws.ToString(delOut.Service.Status),
		"DeleteService must settle the service to INACTIVE")
}
