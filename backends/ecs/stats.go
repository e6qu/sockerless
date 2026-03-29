package ecs

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	core "github.com/sockerless/backend-core"
)

// ecsStatsProvider fetches real container metrics from CloudWatch Container Insights.
// Replaces all-zero synthetic stats with real CloudWatch data.
type ecsStatsProvider struct {
	server *Server
}

func (p *ecsStatsProvider) ContainerMetrics(containerID string) (*core.ContainerMetrics, error) {
	ecsState, ok := p.server.ECS.Get(containerID)
	if !ok || ecsState.TaskARN == "" {
		return nil, nil
	}

	c, ok := p.server.Store.Containers.Get(containerID)
	if !ok || !c.State.Running {
		return nil, nil
	}

	// Extract task ID from ARN: arn:aws:ecs:region:account:task/cluster/taskid
	taskID := ecsState.TaskARN
	if parts := strings.Split(ecsState.TaskARN, "/"); len(parts) > 0 {
		taskID = parts[len(parts)-1]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC()
	start := now.Add(-5 * time.Minute)

	dimensions := []cwtypes.Dimension{
		{Name: aws.String("ClusterName"), Value: aws.String(p.server.config.Cluster)},
		{Name: aws.String("TaskId"), Value: aws.String(taskID)},
	}

	// Fetch CPU and memory utilization from ECS/ContainerInsights
	result, err := p.server.aws.CloudWatchMetrics.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
		StartTime: &start,
		EndTime:   &now,
		MetricDataQueries: []cwtypes.MetricDataQuery{
			{
				Id: aws.String("cpu"),
				MetricStat: &cwtypes.MetricStat{
					Metric: &cwtypes.Metric{
						Namespace:  aws.String("ECS/ContainerInsights"),
						MetricName: aws.String("CpuUtilized"),
						Dimensions: dimensions,
					},
					Period: aws.Int32(60),
					Stat:   aws.String("Average"),
				},
			},
			{
				Id: aws.String("mem"),
				MetricStat: &cwtypes.MetricStat{
					Metric: &cwtypes.Metric{
						Namespace:  aws.String("ECS/ContainerInsights"),
						MetricName: aws.String("MemoryUtilized"),
						Dimensions: dimensions,
					},
					Period: aws.Int32(60),
					Stat:   aws.String("Average"),
				},
			},
		},
	})
	if err != nil {
		// CloudWatch may not have data yet for new tasks — return zeros
		return &core.ContainerMetrics{PIDs: 1}, nil
	}

	var cpuUnits, memMB float64
	for _, r := range result.MetricDataResults {
		if len(r.Values) == 0 {
			continue
		}
		latest := r.Values[0]
		switch aws.ToString(r.Id) {
		case "cpu":
			cpuUnits = latest // CPU units (vCPU * 1024)
		case "mem":
			memMB = latest // Memory in megabytes
		}
	}

	return &core.ContainerMetrics{
		CPUNanos: int64(cpuUnits * 1e9 / 1024), // Convert CPU units to nanoseconds
		MemBytes: int64(memMB * 1024 * 1024),    // Convert MB to bytes
		PIDs:     1,
	}, nil
}
