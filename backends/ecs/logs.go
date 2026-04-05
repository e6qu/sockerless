package ecs

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// getTaskID extracts the ECS task ID for a container.
// First checks local ECS state, then queries the cloud if needed (stateless recovery).
func (s *Server) getTaskID(containerID string) string {
	// Try local state first (fast path for current session)
	if ecsState, ok := s.ECS.Get(containerID); ok && ecsState.TaskARN != "" {
		return extractTaskIDFromARN(ecsState.TaskARN)
	}

	// Query cloud — find the task by sockerless-container-id tag
	taskARN := s.findTaskARNByContainerID(containerID)
	if taskARN != "" {
		return extractTaskIDFromARN(taskARN)
	}

	return "unknown"
}

// findTaskARNByContainerID queries ECS for a task tagged with the given container ID.
func (s *Server) findTaskARNByContainerID(containerID string) string {
	ctx := context.Background()

	// Check both running and stopped tasks
	for _, status := range []ecstypes.DesiredStatus{ecstypes.DesiredStatusRunning, ecstypes.DesiredStatusStopped} {
		listResult, err := s.aws.ECS.ListTasks(ctx, &awsecs.ListTasksInput{
			Cluster:       aws.String(s.config.Cluster),
			DesiredStatus: status,
		})
		if err != nil || len(listResult.TaskArns) == 0 {
			continue
		}

		descResult, err := s.aws.ECS.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
			Cluster: aws.String(s.config.Cluster),
			Tasks:   listResult.TaskArns,
			Include: []ecstypes.TaskField{ecstypes.TaskFieldTags},
		})
		if err != nil {
			continue
		}

		for _, task := range descResult.Tasks {
			for _, tag := range task.Tags {
				if aws.ToString(tag.Key) == "sockerless-container-id" && aws.ToString(tag.Value) == containerID {
					return aws.ToString(task.TaskArn)
				}
			}
		}
	}

	return ""
}

func extractTaskIDFromARN(arn string) string {
	parts := strings.Split(arn, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return arn
}
