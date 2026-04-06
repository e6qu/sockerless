package ecs

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	core "github.com/sockerless/backend-core"
)

// ScanOrphanedResources discovers Sockerless-managed ECS tasks.
func (s *Server) ScanOrphanedResources(ctx context.Context, instanceID string) ([]core.ResourceEntry, error) {
	listResult, err := s.aws.ECS.ListTasks(ctx, &awsecs.ListTasksInput{
		Cluster: aws.String(s.config.Cluster),
	})
	if err != nil {
		return nil, err
	}

	if len(listResult.TaskArns) == 0 {
		return nil, nil
	}

	descResult, err := s.aws.ECS.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
		Cluster: aws.String(s.config.Cluster),
		Tasks:   listResult.TaskArns,
	})
	if err != nil {
		return nil, err
	}

	var orphans []core.ResourceEntry
	for _, task := range descResult.Tasks {
		taskARN := aws.ToString(task.TaskArn)
		tagsResult, err := s.aws.ECS.ListTagsForResource(ctx, &awsecs.ListTagsForResourceInput{
			ResourceArn: aws.String(taskARN),
		})
		if err != nil {
			continue
		}

		managed := false
		matchesInstance := false
		for _, tag := range tagsResult.Tags {
			if aws.ToString(tag.Key) == "sockerless-managed" && aws.ToString(tag.Value) == "true" {
				managed = true
			}
			if aws.ToString(tag.Key) == "sockerless-instance" && aws.ToString(tag.Value) == instanceID {
				matchesInstance = true
			}
		}

		if managed && matchesInstance {
			orphans = append(orphans, core.ResourceEntry{
				Backend:      "ecs",
				ResourceType: "task",
				ResourceID:   taskARN,
				InstanceID:   instanceID,
				CreatedAt:    time.Now(),
			})
		}
	}

	return orphans, nil
}

// SyncResources queries ECS for the current status of all tracked tasks
// and updates the registry (mark stopped tasks, remove deleted ones).
func (s *Server) SyncResources(ctx context.Context, registry *core.ResourceRegistry) error {
	active := registry.ListActive()
	if len(active) == 0 {
		return nil
	}

	// Collect task ARNs to check
	var arns []string
	for _, entry := range active {
		if entry.ResourceType == "task" {
			arns = append(arns, entry.ResourceID)
		}
	}
	if len(arns) == 0 {
		return nil
	}

	// DescribeTasks supports up to 100 ARNs per call
	for i := 0; i < len(arns); i += 100 {
		end := i + 100
		if end > len(arns) {
			end = len(arns)
		}
		batch := arns[i:end]

		result, err := s.aws.ECS.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
			Cluster: aws.String(s.config.Cluster),
			Tasks:   batch,
		})
		if err != nil {
			s.Logger.Warn().Err(err).Msg("resync: DescribeTasks failed")
			continue
		}

		// Build set of found tasks
		found := make(map[string]string) // arn → lastStatus
		for _, task := range result.Tasks {
			found[aws.ToString(task.TaskArn)] = aws.ToString(task.LastStatus)
		}

		// Mark tasks that are stopped or gone
		for _, arn := range batch {
			status, exists := found[arn]
			if !exists || status == "STOPPED" || status == "DEPROVISIONING" {
				registry.MarkCleanedUp(arn)
			}
		}
	}

	return nil
}

// CleanupResource stops an ECS task.
func (s *Server) CleanupResource(ctx context.Context, entry core.ResourceEntry) error {
	_, err := s.aws.ECS.StopTask(ctx, &awsecs.StopTaskInput{
		Cluster: aws.String(s.config.Cluster),
		Task:    aws.String(entry.ResourceID),
		Reason:  aws.String("Sockerless orphan cleanup"),
	})
	return err
}
