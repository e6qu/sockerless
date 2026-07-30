package aws_sdk_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/require"
)

// TestAmazonECSServiceAdoptsItsTaskAcrossSimulatorRestart_SDK proves that the
// durable service and task stores remain authoritative across a hard
// control-plane replacement. The restarted simulator adopts the original
// workload container and does not launch a duplicate replacement task.
func TestAmazonECSServiceAdoptsItsTaskAcrossSimulatorRestart_SDK(t *testing.T) {
	stateDir := t.TempDir()
	tcpPort, udpPort := persistentSimulatorPorts(t)
	endpoint := fmt.Sprintf("http://127.0.0.1:%d", tcpPort)
	cmd := startPersistentSimulator(t, stateDir, tcpPort, udpPort, "docker")
	t.Cleanup(func() { shutdownSimulator(cmd) })

	cfg := persistentSDKConfig()
	client := ecs.NewFromConfig(cfg, func(options *ecs.Options) {
		options.BaseEndpoint = aws.String(endpoint)
	})
	testCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	const (
		cluster = "persistent-service-cluster"
		service = "persistent-service"
	)
	_, err := client.CreateCluster(testCtx, &ecs.CreateClusterInput{ClusterName: aws.String(cluster)})
	require.NoError(t, err)
	_, err = client.RegisterTaskDefinition(testCtx, &ecs.RegisterTaskDefinitionInput{
		Family: aws.String("persistent-service-task"),
		ContainerDefinitions: []ecstypes.ContainerDefinition{{
			Name: aws.String("application"), Image: aws.String(containerCommandImage),
			Command: []string{"hold"}, Essential: aws.Bool(true),
		}},
	})
	require.NoError(t, err)
	_, err = client.CreateService(testCtx, &ecs.CreateServiceInput{
		Cluster: aws.String(cluster), ServiceName: aws.String(service),
		TaskDefinition: aws.String("persistent-service-task"), DesiredCount: aws.Int32(1),
	})
	require.NoError(t, err)

	var originalTaskARN string
	require.Eventually(t, func() bool {
		listed, listErr := client.ListTasks(testCtx, &ecs.ListTasksInput{
			Cluster: aws.String(cluster), ServiceName: aws.String(service),
			DesiredStatus: ecstypes.DesiredStatusRunning,
		})
		if listErr != nil || len(listed.TaskArns) != 1 {
			return false
		}
		originalTaskARN = listed.TaskArns[0]
		return originalTaskARN != ""
	}, 30*time.Second, 100*time.Millisecond, "service task did not reach RUNNING")

	shutdownSimulator(cmd)
	cmd = startPersistentSimulator(t, stateDir, tcpPort, udpPort, "docker")
	client = ecs.NewFromConfig(cfg, func(options *ecs.Options) {
		options.BaseEndpoint = aws.String(endpoint)
	})

	require.Eventually(t, func() bool {
		listed, listErr := client.ListTasks(testCtx, &ecs.ListTasksInput{
			Cluster: aws.String(cluster), ServiceName: aws.String(service),
			DesiredStatus: ecstypes.DesiredStatusRunning,
		})
		if listErr != nil || len(listed.TaskArns) != 1 || listed.TaskArns[0] != originalTaskARN {
			return false
		}
		described, describeErr := client.DescribeServices(testCtx, &ecs.DescribeServicesInput{
			Cluster: aws.String(cluster), Services: []string{service},
		})
		return describeErr == nil && len(described.Services) == 1 &&
			described.Services[0].RunningCount == 1 &&
			described.Services[0].PendingCount == 0
	}, 30*time.Second, 100*time.Millisecond, "restart did not adopt exactly the original service task")

	_, err = client.UpdateService(testCtx, &ecs.UpdateServiceInput{
		Cluster: aws.String(cluster), Service: aws.String(service), DesiredCount: aws.Int32(0),
	})
	require.NoError(t, err)
	_, err = client.DeleteService(testCtx, &ecs.DeleteServiceInput{
		Cluster: aws.String(cluster), Service: aws.String(service), Force: aws.Bool(true),
	})
	require.NoError(t, err)
}
