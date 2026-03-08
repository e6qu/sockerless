package ecs

import (
	"context"
	"fmt"
	"net/http"
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

	tags := core.TagSet{
		ContainerID: containerID,
		Backend:     "ecs",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
	}

	runResult, err := s.aws.ECS.RunTask(s.ctx(), &awsecs.RunTaskInput{
		Cluster:        aws.String(s.config.Cluster),
		TaskDefinition: aws.String(taskDefARN),
		LaunchType:     ecstypes.LaunchTypeFargate,
		Count:          aws.Int32(1),
		Tags:           mapToECSTags(tags.AsMap()),
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				Subnets:        s.config.Subnets,
				SecurityGroups: s.config.SecurityGroups,
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

// waitForTaskRunning polls ECS until the task reaches RUNNING state.
// Returns the agent address (ip:9111).
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
				reason := aws.ToString(task.StoppedReason)
				return "", fmt.Errorf("task stopped: %s", reason)
			}
		}
	}
}

// waitForAgentHealth polls the agent's /health endpoint.
func (s *Server) waitForAgentHealth(ctx context.Context, healthURL string) error {
	timeout := time.After(s.config.AgentTimeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	client := &http.Client{Timeout: 2 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for agent health")
		case <-ticker.C:
			resp, err := client.Get(healthURL)
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
	}
}

// waitForTaskStopped blocks until the ECS task reaches STOPPED state or exitCh is closed.
// Used in reverse agent mode where the goroutine needs to wait for the cloud task to finish.
func (s *Server) waitForTaskStopped(taskARN string, exitCh chan struct{}) {
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
			if aws.ToString(result.Tasks[0].LastStatus) == "STOPPED" {
				return
			}
		}
	}
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
			if aws.ToString(task.LastStatus) == "STOPPED" {
				exitCode := 0
				for _, container := range task.Containers {
					if container.ExitCode != nil {
						exitCode = int(aws.ToInt32(container.ExitCode))
						break
					}
				}
				if c, ok := s.Store.Containers.Get(containerID); ok && c.State.Running {
					s.Store.StopContainer(containerID, exitCode)
				}
				return
			}
		}
	}
}

// refreshTaskStatus calls ecs:DescribeTasks for a single container's TaskARN
// and merges the real task status (running/stopped, exit code, IP) into the
// in-memory container. Returns nil if no TaskARN is stored or the DescribeTasks
// call fails (non-fatal — the in-memory state is unchanged).
func (s *Server) refreshTaskStatus(containerID string) {
	ecsState, ok := s.ECS.Get(containerID)
	if !ok || ecsState.TaskARN == "" {
		return
	}

	result, err := s.aws.ECS.DescribeTasks(s.ctx(), &awsecs.DescribeTasksInput{
		Cluster: aws.String(s.config.Cluster),
		Tasks:   []string{ecsState.TaskARN},
	})
	if err != nil || len(result.Tasks) == 0 {
		return
	}

	s.applyTaskStatus(containerID, result.Tasks[0])
}

// refreshTaskStatusBatch calls ecs:DescribeTasks for multiple containers at once
// (up to 100 per AWS API call) and merges real task status into each container.
func (s *Server) refreshTaskStatusBatch(ids []string) {
	// Build a map from taskARN → containerID and collect unique task ARNs.
	taskToContainer := make(map[string]string, len(ids))
	var taskARNs []string
	for _, id := range ids {
		ecsState, ok := s.ECS.Get(id)
		if !ok || ecsState.TaskARN == "" {
			continue
		}
		if _, exists := taskToContainer[ecsState.TaskARN]; !exists {
			taskARNs = append(taskARNs, ecsState.TaskARN)
			taskToContainer[ecsState.TaskARN] = id
		}
	}

	if len(taskARNs) == 0 {
		return
	}

	// DescribeTasks supports up to 100 ARNs per call.
	const batchSize = 100
	for i := 0; i < len(taskARNs); i += batchSize {
		end := i + batchSize
		if end > len(taskARNs) {
			end = len(taskARNs)
		}
		batch := taskARNs[i:end]

		result, err := s.aws.ECS.DescribeTasks(s.ctx(), &awsecs.DescribeTasksInput{
			Cluster: aws.String(s.config.Cluster),
			Tasks:   batch,
		})
		if err != nil {
			continue
		}

		for _, task := range result.Tasks {
			taskARN := aws.ToString(task.TaskArn)
			if cid, ok := taskToContainer[taskARN]; ok {
				s.applyTaskStatus(cid, task)
			}
		}
	}
}

// applyTaskStatus merges the ECS task description into the in-memory container state.
func (s *Server) applyTaskStatus(containerID string, task ecstypes.Task) {
	status := aws.ToString(task.LastStatus)

	switch status {
	case "STOPPED":
		exitCode := 0
		for _, container := range task.Containers {
			if container.ExitCode != nil {
				exitCode = int(aws.ToInt32(container.ExitCode))
				break
			}
		}
		if c, ok := s.Store.Containers.Get(containerID); ok && c.State.Running {
			s.Store.Containers.Update(containerID, func(c *api.Container) {
				c.State.Status = "exited"
				c.State.Running = false
				c.State.Pid = 0
				c.State.ExitCode = exitCode
				c.State.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
			})
			if ch, ok := s.Store.WaitChs.LoadAndDelete(containerID); ok {
				close(ch.(chan struct{}))
			}
		}
	case "RUNNING":
		// Update IP if we don't have it yet
		ip := extractENIIP(task)
		if ip != "" {
			s.Store.Containers.Update(containerID, func(c *api.Container) {
				for _, ep := range c.NetworkSettings.Networks {
					if ep != nil && ep.IPAddress == "" {
						ep.IPAddress = ip
					}
				}
			})
		}
	}
}
