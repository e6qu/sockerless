package ecs

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// runECSTask runs a single ECS task with the given task definition.
// Returns the task ARN and cluster ARN.
func (s *Server) runECSTask(containerID, taskDefARN string, c *api.Container) (taskARN, clusterARN string, err error) {
	assignPublicIP := ecstypes.AssignPublicIpDisabled
	if s.config.AssignPublicIP {
		assignPublicIP = ecstypes.AssignPublicIpEnabled
	}

	restartCount := 0
	if ecsState, ok := s.ECS.Get(containerID); ok {
		restartCount = ecsState.RestartCount
	}
	tags := core.TagSet{
		ContainerID:  containerID,
		Backend:      "ecs",
		Cluster:      s.config.Cluster,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Name:         c.Name,
		Network:      c.HostConfig.NetworkMode,
		Labels:       c.Config.Labels,
		Tty:          c.Config.Tty,
		RestartCount: restartCount,
	}
	// Set pod tag if container is in a pod
	if pod, _ := s.Store.Pods.GetPodForContainer(containerID); pod != nil {
		tags.Pod = pod.Name
	}

	// Merge per-container security groups from network associations.
	securityGroups := append([]string{}, s.config.SecurityGroups...)
	if ecsState, ok := s.ECS.Get(containerID); ok && len(ecsState.SecurityGroupIDs) > 0 {
		securityGroups = append(securityGroups, ecsState.SecurityGroupIDs...)
	}

	runResult, err := s.aws.ECS.RunTask(s.ctx(), &awsecs.RunTaskInput{
		Cluster:        aws.String(s.config.Cluster),
		TaskDefinition: aws.String(taskDefARN),
		LaunchType:     ecstypes.LaunchTypeFargate,
		Count:          aws.Int32(1),
		Tags:           mapToECSTags(tags.AsMap()),
		// ECS Exec must be enabled at task launch time for
		// docker exec to work. Combined with task-role ssmmessages:*
		// permissions this allows the in-task SSM agent to
		// dial back to Session Manager.
		EnableExecuteCommand: true,
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				Subnets:        s.config.Subnets,
				SecurityGroups: securityGroups,
				AssignPublicIp: assignPublicIP,
			},
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to run task: %w", err)
	}

	if len(runResult.Tasks) == 0 {
		msg := "no tasks launched"
		if len(runResult.Failures) > 0 {
			msg = aws.ToString(runResult.Failures[0].Reason)
		}
		return "", "", fmt.Errorf("failed to launch task: %s", msg)
	}

	taskARN = aws.ToString(runResult.Tasks[0].TaskArn)
	clusterARN = aws.ToString(runResult.Tasks[0].ClusterArn)

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  containerID,
		Backend:      "ecs",
		ResourceType: "task",
		ResourceID:   taskARN,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": c.Image, "name": c.Name, "taskArn": taskARN},
	})

	return taskARN, clusterARN, nil
}

// waitForTaskRunning polls ECS until the task reaches RUNNING state
// or exits successfully before we observe RUNNING. A short-lived
// command (`alpine echo hello`) can transition PENDING→STOPPED within
// a single poll interval; treating that as a start failure would be
// wrong — the task ran, produced output, and exited 0, which is what
// docker semantics call a successful run. Returns the task IP
// (`ip:9111`) when the task is observed RUNNING, or empty string when
// the task already completed successfully.
func (s *Server) waitForTaskRunning(ctx context.Context, taskARN string) (string, error) {
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(s.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timeout:
			return "", fmt.Errorf("timeout waiting for task to reach RUNNING state")
		case <-ticker.C:
			result, err := s.aws.ECS.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
				Cluster: aws.String(s.config.Cluster),
				Tasks:   []string{taskARN},
			})
			if err != nil {
				continue
			}
			if len(result.Tasks) == 0 {
				continue
			}

			task := result.Tasks[0]
			status := aws.ToString(task.LastStatus)

			switch status {
			case "RUNNING":
				ip := extractENIIP(task)
				if ip == "" {
					continue
				}
				return ip + ":9111", nil
			case "STOPPED":
				if taskExitedSuccessfully(task) {
					return "", nil
				}
				reason := aws.ToString(task.StoppedReason)
				return "", fmt.Errorf("task stopped: %s", reason)
			}
		}
	}
}

// taskExitedSuccessfully reports whether a STOPPED task's essential
// container exited with status 0.
func taskExitedSuccessfully(task ecstypes.Task) bool {
	if len(task.Containers) == 0 {
		return false
	}
	for _, c := range task.Containers {
		if c.ExitCode == nil || *c.ExitCode != 0 {
			return false
		}
	}
	return true
}

// mapToECSTags converts a map[string]string to ECS SDK tag format.
func mapToECSTags(m map[string]string) []ecstypes.Tag {
	tags := make([]ecstypes.Tag, 0, len(m))
	for k, v := range m {
		tags = append(tags, ecstypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return tags
}

// pollTaskExit monitors an ECS task and updates container state when it stops.
func (s *Server) pollTaskExit(containerID, taskARN string, exitCh chan struct{}) {
	ticker := time.NewTicker(s.config.PollInterval * 2)
	defer ticker.Stop()

	for {
		select {
		case <-exitCh:
			return
		case <-ticker.C:
			result, err := s.aws.ECS.DescribeTasks(s.ctx(), &awsecs.DescribeTasksInput{
				Cluster: aws.String(s.config.Cluster),
				Tasks:   []string{taskARN},
			})
			if err != nil {
				continue
			}
			if len(result.Tasks) == 0 {
				continue
			}

			task := result.Tasks[0]
			// Apply task status on every poll (updates IP for RUNNING, exits for STOPPED)
			s.applyTaskStatus(containerID, task)
			if aws.ToString(task.LastStatus) == "STOPPED" {
				return
			}
		}
	}
}

// applyTaskStatus processes ECS task status changes.
// Cloud is the source of truth for container state — no local Store.Containers writes.
// Only closes wait channels when the task stops (needed for ContainerWait).
func (s *Server) applyTaskStatus(containerID string, task ecstypes.Task) {
	status := aws.ToString(task.LastStatus)

	switch status {
	case "STOPPED":
		// Close wait channel so ContainerWait unblocks
		if ch, ok := s.Store.WaitChs.LoadAndDelete(containerID); ok {
			close(ch.(chan struct{}))
		}
	case "RUNNING":
		// No-op: cloud is the source of truth for IP/MAC.
	}
}
