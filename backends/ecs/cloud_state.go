package ecs

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// ecsCloudState implements core.CloudStateProvider for ECS Fargate.
// All container state is derived from ECS tasks tagged with sockerless-managed=true.
type ecsCloudState struct {
	ecs     *awsecs.Client
	cluster string
	config  Config
}

func (p *ecsCloudState) GetContainer(ctx context.Context, ref string) (api.Container, bool, error) {
	containers, err := p.queryTasks(ctx)
	if err != nil {
		return api.Container{}, false, err
	}

	// Match by full ID, name, or short ID prefix
	for _, c := range containers {
		if c.ID == ref {
			return c, true, nil
		}
		if c.Name == ref || c.Name == "/"+ref || strings.TrimPrefix(c.Name, "/") == ref {
			return c, true, nil
		}
		if len(ref) >= 3 && strings.HasPrefix(c.ID, ref) {
			return c, true, nil
		}
	}
	return api.Container{}, false, nil
}

func (p *ecsCloudState) ListContainers(ctx context.Context, all bool, filters map[string][]string) ([]api.Container, error) {
	containers, err := p.queryTasks(ctx)
	if err != nil {
		return nil, err
	}

	var result []api.Container
	for _, c := range containers {
		if !all && !c.State.Running {
			continue
		}
		if !core.MatchContainerFilters(c, filters) {
			continue
		}
		result = append(result, c)
	}
	return result, nil
}

func (p *ecsCloudState) CheckNameAvailable(ctx context.Context, name string) (bool, error) {
	containers, err := p.queryTasks(ctx)
	if err != nil {
		return false, err
	}
	for _, c := range containers {
		if c.Name == name || c.Name == "/"+name {
			return false, nil
		}
	}
	return true, nil
}

func (p *ecsCloudState) WaitForExit(ctx context.Context, containerID string) (int, error) {
	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-ticker.C:
			containers, err := p.queryTasks(ctx)
			if err != nil {
				continue
			}
			for _, c := range containers {
				if c.ID == containerID && !c.State.Running && c.State.Status == "exited" {
					return c.State.ExitCode, nil
				}
			}
		}
	}
}

// queryTasks fetches all sockerless-managed tasks from ECS and reconstructs containers.
// Queries both RUNNING and STOPPED tasks (ECS keeps stopped tasks for ~1 hour).
func (p *ecsCloudState) queryTasks(ctx context.Context) ([]api.Container, error) {
	var allTaskArns []string

	// List running tasks
	runningResult, err := p.ecs.ListTasks(ctx, &awsecs.ListTasksInput{
		Cluster:       aws.String(p.cluster),
		DesiredStatus: ecstypes.DesiredStatusRunning,
	})
	if err == nil {
		allTaskArns = append(allTaskArns, runningResult.TaskArns...)
	}

	// List stopped tasks (kept by ECS for ~1 hour after exit)
	stoppedResult, err := p.ecs.ListTasks(ctx, &awsecs.ListTasksInput{
		Cluster:       aws.String(p.cluster),
		DesiredStatus: ecstypes.DesiredStatusStopped,
	})
	if err == nil {
		allTaskArns = append(allTaskArns, stoppedResult.TaskArns...)
	}

	if len(allTaskArns) == 0 {
		return nil, nil
	}

	// Describe tasks with tags included (batch up to 100)
	descResult, err := p.ecs.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
		Cluster: aws.String(p.cluster),
		Tasks:   allTaskArns,
		Include: []ecstypes.TaskField{ecstypes.TaskFieldTags},
	})
	if err != nil {
		return nil, err
	}

	var containers []api.Container
	for _, task := range descResult.Tasks {
		tags := tagsToMap(task.Tags)

		// Only include sockerless-managed tasks
		if tags["sockerless-managed"] != "true" {
			continue
		}

		c := taskToContainer(task, tags)
		containers = append(containers, c)
	}

	return containers, nil
}

// taskToContainer reconstructs an api.Container from an ECS task and its tags.
func taskToContainer(task ecstypes.Task, tags map[string]string) api.Container {
	containerID := tags["sockerless-container-id"]
	name := tags["sockerless-name"]
	if name == "" && containerID != "" {
		name = "/" + containerID[:12]
	}

	// Derive image and command from task definition containers
	image := ""
	var cmd []string
	var entrypoint []string
	var env []string
	if len(task.Containers) > 0 {
		// The "main" container or first container
		for _, tc := range task.Containers {
			if aws.ToString(tc.Name) == "main" || len(task.Containers) == 1 {
				image = aws.ToString(tc.Image)
				break
			}
		}
	}

	// Map ECS status to Docker state
	state := mapTaskStatus(task)

	// Extract real IP from ENI
	ip := extractENIIP(task)
	mac := ""
	if ip != "" {
		mac = deriveMACFromIP(ip)
	}

	// Parse creation time
	created := ""
	if task.CreatedAt != nil {
		created = task.CreatedAt.Format(time.RFC3339Nano)
	}

	// Parse Docker labels from tags
	labels := core.ParseLabelsFromTags(tags)
	if labels == nil {
		labels = make(map[string]string)
	}

	networkName := tags["sockerless-network"]
	if networkName == "" {
		networkName = "bridge"
	}

	return api.Container{
		ID:      containerID,
		Name:    name,
		Created: created,
		Image:   image,
		Path:    "",
		Args:    nil,
		State:   state,
		Config: api.ContainerConfig{
			Image:      image,
			Cmd:        cmd,
			Entrypoint: entrypoint,
			Env:        env,
			Labels:     labels,
		},
		HostConfig: api.HostConfig{
			NetworkMode: networkName,
		},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				networkName: {
					NetworkID:  networkName,
					IPAddress:  ip,
					MacAddress: mac,
					Gateway:    "",
				},
			},
		},
	}
}

// mapTaskStatus converts ECS task status to Docker container state.
func mapTaskStatus(task ecstypes.Task) api.ContainerState {
	lastStatus := aws.ToString(task.LastStatus)

	switch lastStatus {
	case "PROVISIONING", "PENDING":
		return api.ContainerState{
			Status: "created",
		}
	case "RUNNING":
		startedAt := ""
		if task.StartedAt != nil {
			startedAt = task.StartedAt.Format(time.RFC3339Nano)
		}
		return api.ContainerState{
			Status:    "running",
			Running:   true,
			StartedAt: startedAt,
		}
	case "DEPROVISIONING", "STOPPED":
		exitCode := 0
		stateError := ""
		for _, c := range task.Containers {
			if c.ExitCode != nil {
				exitCode = int(aws.ToInt32(c.ExitCode))
				break
			}
			if c.Reason != nil && *c.Reason != "" {
				stateError = *c.Reason
			}
		}
		if exitCode == 0 && stateError == "" {
			reason := aws.ToString(task.StoppedReason)
			if reason != "" && reason != "Essential container in task exited" {
				stateError = reason
				exitCode = 1
			}
		}
		stoppedAt := ""
		if task.StoppedAt != nil {
			stoppedAt = task.StoppedAt.Format(time.RFC3339Nano)
		}
		startedAt := ""
		if task.StartedAt != nil {
			startedAt = task.StartedAt.Format(time.RFC3339Nano)
		}
		return api.ContainerState{
			Status:     "exited",
			ExitCode:   exitCode,
			Error:      stateError,
			StartedAt:  startedAt,
			FinishedAt: stoppedAt,
		}
	default:
		return api.ContainerState{Status: "unknown"}
	}
}

// tagsToMap converts ECS tag slice to a map.
func tagsToMap(tags []ecstypes.Tag) map[string]string {
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return m
}
