package aws_sdk_test

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rdsClient() *rds.Client {
	return rds.NewFromConfig(sdkConfig(), func(o *rds.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func ecClient() *elasticache.Client {
	return elasticache.NewFromConfig(sdkConfig(), func(o *elasticache.Options) {
		o.BaseEndpoint = aws.String(baseURL)
	})
}

func TestRDS_DBInstanceLifecycle(t *testing.T) {
	c := rdsClient()
	id := "test-pg-db"
	_, err := c.CreateDBInstance(ctx, &rds.CreateDBInstanceInput{
		DBInstanceIdentifier: aws.String(id),
		DBInstanceClass:      aws.String("db.t3.micro"),
		Engine:               aws.String("postgres"),
		EngineVersion:        aws.String("15.4"),
		MasterUsername:       aws.String("admin"),
		MasterUserPassword:   aws.String("password123!"),
		AllocatedStorage:     aws.Int32(20),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteDBInstance(ctx, &rds.DeleteDBInstanceInput{
			DBInstanceIdentifier: aws.String(id),
			SkipFinalSnapshot:    aws.Bool(true),
		})
	})

	desc, err := c.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(id),
	})
	require.NoError(t, err)
	require.Len(t, desc.DBInstances, 1)
	got := desc.DBInstances[0]
	assert.Equal(t, "postgres", aws.ToString(got.Engine))
	assert.Equal(t, "available", aws.ToString(got.DBInstanceStatus))
	assert.NotNil(t, got.Endpoint)
	assert.Equal(t, int32(5432), aws.ToInt32(got.Endpoint.Port),
		"postgres engine should map to port 5432")

	_, err = c.ModifyDBInstance(ctx, &rds.ModifyDBInstanceInput{
		DBInstanceIdentifier: aws.String(id),
		DBInstanceClass:      aws.String("db.t3.small"),
	})
	require.NoError(t, err)

	desc2, err := c.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(id),
	})
	require.NoError(t, err)
	require.Len(t, desc2.DBInstances, 1)
	assert.Equal(t, "db.t3.small", aws.ToString(desc2.DBInstances[0].DBInstanceClass))

	_, err = c.DeleteDBInstance(ctx, &rds.DeleteDBInstanceInput{
		DBInstanceIdentifier: aws.String(id),
		SkipFinalSnapshot:    aws.Bool(true),
	})
	require.NoError(t, err)

	_, err = c.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(id),
	})
	assertAWSAPIErrorCode(t, err, "DBInstanceNotFound")
}

func assertAWSAPIErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	require.Error(t, err)
	var apiErr smithy.APIError
	require.True(t, errors.As(err, &apiErr), "expected smithy.APIError")
	assert.Equal(t, code, apiErr.ErrorCode())
}

func TestElastiCache_ClusterLifecycle(t *testing.T) {
	c := ecClient()
	id := "test-redis-cluster"
	_, err := c.CreateCacheCluster(ctx, &elasticache.CreateCacheClusterInput{
		CacheClusterId: aws.String(id),
		CacheNodeType:  aws.String("cache.t3.micro"),
		Engine:         aws.String("redis"),
		EngineVersion:  aws.String("7.0"),
		NumCacheNodes:  aws.Int32(1),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = c.DeleteCacheCluster(ctx, &elasticache.DeleteCacheClusterInput{
			CacheClusterId: aws.String(id),
		})
	})

	desc, err := c.DescribeCacheClusters(ctx, &elasticache.DescribeCacheClustersInput{
		CacheClusterId: aws.String(id),
	})
	require.NoError(t, err)
	require.Len(t, desc.CacheClusters, 1)
	got := desc.CacheClusters[0]
	assert.Equal(t, "redis", aws.ToString(got.Engine))
	assert.Equal(t, "available", aws.ToString(got.CacheClusterStatus))
	require.NotNil(t, got.ConfigurationEndpoint)
	assert.Equal(t, int32(6379), aws.ToInt32(got.ConfigurationEndpoint.Port),
		"redis engine should map to port 6379")

	_, err = c.ModifyCacheCluster(ctx, &elasticache.ModifyCacheClusterInput{
		CacheClusterId: aws.String(id),
		CacheNodeType:  aws.String("cache.t3.small"),
	})
	require.NoError(t, err)

	desc2, err := c.DescribeCacheClusters(ctx, &elasticache.DescribeCacheClustersInput{
		CacheClusterId: aws.String(id),
	})
	require.NoError(t, err)
	require.Len(t, desc2.CacheClusters, 1)
	assert.Equal(t, "cache.t3.small", aws.ToString(desc2.CacheClusters[0].CacheNodeType))

	_, err = c.DeleteCacheCluster(ctx, &elasticache.DeleteCacheClusterInput{
		CacheClusterId: aws.String(id),
	})
	require.NoError(t, err)
	_, err = c.DescribeCacheClusters(ctx, &elasticache.DescribeCacheClustersInput{
		CacheClusterId: aws.String(id),
	})
	assertAWSAPIErrorCode(t, err, "CacheClusterNotFound")
}
