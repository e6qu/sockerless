package aws_sdk_test

import (
	"testing"

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
