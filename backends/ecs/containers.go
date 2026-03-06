package ecs

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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

// signalToExitCode maps a signal name or number to the corresponding
// exit code (128 + signal number), matching Docker's behavior.
func signalToExitCode(signal string) int {
	signalMap := map[string]int{
		"SIGHUP": 129, "HUP": 129, "1": 129,
		"SIGINT": 130, "INT": 130, "2": 130,
		"SIGQUIT": 131, "QUIT": 131, "3": 131,
		"SIGABRT": 134, "ABRT": 134, "6": 134,
		"SIGKILL": 137, "KILL": 137, "9": 137,
		"SIGUSR1": 138, "USR1": 138, "10": 138,
		"SIGUSR2": 140, "USR2": 140, "12": 140,
		"SIGTERM": 143, "TERM": 143, "15": 143,
	}
	signal = strings.ToUpper(strings.TrimSpace(signal))
	if code, ok := signalMap[signal]; ok {
		return code
	}
	return 137 // default to SIGKILL
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

// mergeEnvByKey merges base env vars with override env vars by key.
// Override values replace base values with the same key; order is preserved.
func mergeEnvByKey(base, override []string) []string {
	if len(override) == 0 {
		return base
	}
	if len(base) == 0 {
		return override
	}
	keys := make(map[string]string)
	order := make([]string, 0, len(base)+len(override))
	for _, e := range base {
		k, _, _ := strings.Cut(e, "=")
		keys[k] = e
		order = append(order, k)
	}
	for _, e := range override {
		k, _, _ := strings.Cut(e, "=")
		if _, exists := keys[k]; !exists {
			order = append(order, k)
		}
		keys[k] = e
	}
	result := make([]string, 0, len(order))
	for _, k := range order {
		result = append(result, keys[k])
	}
	return result
}
