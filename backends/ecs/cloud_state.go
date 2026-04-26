package ecs

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// ecsCloudState implements core.CloudStateProvider for ECS Fargate.
// All container state is derived from ECS tasks tagged with sockerless-managed=true.
type ecsCloudState struct {
	ecs      *awsecs.Client
	ecr      *ecr.Client
	cluster  string
	region   string
	config   Config
	registry *core.ResourceRegistry // used to hide user-removed tasks

	taskDefMu    sync.Mutex
	taskDefCache map[string]ecstypes.TaskDefinition
}

// describeTaskDefinition looks up a task definition by ARN, caching
// the result for the lifetime of the backend process (task-def ARNs
// are immutable — each revision has a unique ARN).
func (p *ecsCloudState) describeTaskDefinition(ctx context.Context, arn string) (ecstypes.TaskDefinition, bool) {
	if arn == "" {
		return ecstypes.TaskDefinition{}, false
	}
	p.taskDefMu.Lock()
	if p.taskDefCache == nil {
		p.taskDefCache = map[string]ecstypes.TaskDefinition{}
	}
	if td, ok := p.taskDefCache[arn]; ok {
		p.taskDefMu.Unlock()
		return td, true
	}
	p.taskDefMu.Unlock()

	out, err := p.ecs.DescribeTaskDefinition(ctx, &awsecs.DescribeTaskDefinitionInput{
		TaskDefinition: aws.String(arn),
	})
	if err != nil || out.TaskDefinition == nil {
		return ecstypes.TaskDefinition{}, false
	}
	td := *out.TaskDefinition
	p.taskDefMu.Lock()
	p.taskDefCache[arn] = td
	p.taskDefMu.Unlock()
	return td, true
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

	// Dedupe by container ID (same logical container can appear as one
	// STOPPED + one RUNNING task after a `docker restart`). Keep the
	// task with the latest CreatedAt (typically the running one).
	byID := map[string]ecstypes.Task{}
	for _, task := range descResult.Tasks {
		tags := tagsToMap(task.Tags)

		// Only include sockerless-managed tasks the user hasn't removed.
		// Removed-check has two sources of truth: a task-side tag
		// (when we could write it) and the local registry (when ECS
		// rejected the tag on a STOPPED task).
		if tags["sockerless-managed"] != "true" || tags["sockerless-removed"] == "true" {
			continue
		}
		if p.registry != nil && p.registry.IsCleanedUp(aws.ToString(task.TaskArn)) {
			continue
		}

		id := tags["sockerless-container-id"]
		if id == "" {
			continue
		}
		if existing, ok := byID[id]; ok {
			// Prefer the running one; otherwise the more-recent one.
			existingRunning := aws.ToString(existing.LastStatus) == "RUNNING"
			taskRunning := aws.ToString(task.LastStatus) == "RUNNING"
			switch {
			case taskRunning && !existingRunning:
				byID[id] = task
			case !taskRunning && existingRunning:
				// keep existing
			default:
				if task.CreatedAt != nil && existing.CreatedAt != nil && task.CreatedAt.After(*existing.CreatedAt) {
					byID[id] = task
				} else if existing.CreatedAt == nil && task.CreatedAt != nil {
					byID[id] = task
				}
			}
			continue
		}
		byID[id] = task
	}

	var containers []api.Container
	for _, task := range byID {
		tags := tagsToMap(task.Tags)
		td, _ := p.describeTaskDefinition(ctx, aws.ToString(task.TaskDefinitionArn))
		c := taskToContainer(task, tags, td)
		containers = append(containers, c)
	}

	return containers, nil
}

// resolveNetworkState returns NetworkState for the given docker network
// ID, deriving from cloud actuals when the in-memory cache is empty.
// Per, the cache is an optimization; the
// security group + Cloud Map namespace tagged with
// `sockerless:network-id=<id>` are the source of truth.
func (s *Server) resolveNetworkState(ctx context.Context, networkID string) (NetworkState, bool) {
	if state, ok := s.NetworkState.Get(networkID); ok && (state.SecurityGroupID != "" || state.NamespaceID != "") {
		return state, true
	}
	state := NetworkState{}
	// SG by tag.
	sgOut, err := s.aws.EC2.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("tag:sockerless:network-id"), Values: []string{networkID}},
		},
	})
	if err == nil && len(sgOut.SecurityGroups) > 0 {
		state.SecurityGroupID = aws.ToString(sgOut.SecurityGroups[0].GroupId)
	}
	// Cloud Map namespace by tag (added at create time per.
	nsOut, nsErr := s.aws.ServiceDiscovery.ListTagsForResource(ctx, &servicediscovery.ListTagsForResourceInput{
		ResourceARN: nil,
	})
	_ = nsOut
	_ = nsErr
	// ListTagsForResource on namespaces requires an ARN, which we don't
	// have. Instead enumerate namespaces and inspect their tags. Use a
	// limited list — sockerless networks are O(10), not O(thousands).
	listOut, listErr := s.aws.ServiceDiscovery.ListNamespaces(ctx, &servicediscovery.ListNamespacesInput{
		Filters: []sdtypes.NamespaceFilter{
			{
				Name:      sdtypes.NamespaceFilterNameType,
				Values:    []string{string(sdtypes.NamespaceTypeDnsPrivate)},
				Condition: sdtypes.FilterConditionEq,
			},
		},
	})
	if listErr == nil {
		for _, ns := range listOut.Namespaces {
			tagsOut, tErr := s.aws.ServiceDiscovery.ListTagsForResource(ctx, &servicediscovery.ListTagsForResourceInput{
				ResourceARN: ns.Arn,
			})
			if tErr != nil {
				continue
			}
			for _, t := range tagsOut.Tags {
				if aws.ToString(t.Key) == "sockerless:network-id" && aws.ToString(t.Value) == networkID {
					state.NamespaceID = aws.ToString(ns.Id)
					break
				}
			}
			if state.NamespaceID != "" {
				break
			}
		}
	}
	if state.SecurityGroupID == "" && state.NamespaceID == "" {
		return NetworkState{}, false
	}
	s.NetworkState.Update(networkID, func(ns *NetworkState) {
		if ns.SecurityGroupID == "" {
			ns.SecurityGroupID = state.SecurityGroupID
		}
		if ns.NamespaceID == "" {
			ns.NamespaceID = state.NamespaceID
		}
	})
	return state, true
}

// ListPods groups sockerless-managed tasks by `sockerless-pod` tag
// and returns one PodListEntry per unique pod name./
// cross-cloud sibling. Single-container tasks (no pod tag)
// are excluded; those show up as standalone containers in
// docker ps but not in docker pod ps.
func (p *ecsCloudState) ListPods(ctx context.Context) ([]*api.PodListEntry, error) {
	containers, err := p.queryTasks(ctx)
	if err != nil {
		return nil, err
	}
	// Re-query raw tasks to read `sockerless-pod` tag — queryTasks
	// drops the tag map after projecting to api.Container.
	var allArns []string
	for _, status := range []ecstypes.DesiredStatus{ecstypes.DesiredStatusRunning, ecstypes.DesiredStatusStopped} {
		out, lerr := p.ecs.ListTasks(ctx, &awsecs.ListTasksInput{
			Cluster:       aws.String(p.cluster),
			DesiredStatus: status,
		})
		if lerr == nil {
			allArns = append(allArns, out.TaskArns...)
		}
	}
	if len(allArns) == 0 {
		return nil, nil
	}
	desc, err := p.ecs.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
		Cluster: aws.String(p.cluster),
		Tasks:   allArns,
		Include: []ecstypes.TaskField{ecstypes.TaskFieldTags},
	})
	if err != nil {
		return nil, err
	}
	// Group container IDs by pod name.
	containerByID := make(map[string]api.Container, len(containers))
	for _, c := range containers {
		containerByID[c.ID] = c
	}
	groups := make(map[string][]api.PodContainerInfo)
	created := make(map[string]string)
	status := make(map[string]string)
	for _, task := range desc.Tasks {
		tags := tagsToMap(task.Tags)
		if tags["sockerless-managed"] != "true" {
			continue
		}
		podName := tags["sockerless-pod"]
		if podName == "" {
			continue
		}
		cid := tags["sockerless-container-id"]
		if cid == "" {
			continue
		}
		cont, ok := containerByID[cid]
		if !ok {
			continue
		}
		groups[podName] = append(groups[podName], api.PodContainerInfo{
			ID:    cont.ID,
			Name:  strings.TrimPrefix(cont.Name, "/"),
			State: cont.State.Status,
		})
		if task.CreatedAt != nil {
			ts := task.CreatedAt.Format(time.RFC3339Nano)
			if created[podName] == "" || ts < created[podName] {
				created[podName] = ts
			}
		}
		if cont.State.Running {
			status[podName] = "Running"
		} else if status[podName] == "" {
			status[podName] = cont.State.Status
		}
	}
	var out []*api.PodListEntry
	for name, cs := range groups {
		out = append(out, &api.PodListEntry{
			ID:         "pod-" + name,
			Name:       name,
			Status:     status[name],
			Created:    created[name],
			Containers: cs,
		})
	}
	return out, nil
}

// ListImages queries ECR for every image under every repository the
// backend's account has access to, projecting to api.ImageSummary so
// `docker images` returns the live cloud registry contents.
// step 2.
func (p *ecsCloudState) ListImages(ctx context.Context) ([]*api.ImageSummary, error) {
	if p.ecr == nil {
		return nil, nil
	}
	var result []*api.ImageSummary
	var nextToken *string
	for {
		reposOut, err := p.ecr.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return result, err
		}
		for _, repo := range reposOut.Repositories {
			repoName := aws.ToString(repo.RepositoryName)
			repoURI := aws.ToString(repo.RepositoryUri)
			var imgToken *string
			for {
				imgsOut, imErr := p.ecr.DescribeImages(ctx, &ecr.DescribeImagesInput{
					RepositoryName: aws.String(repoName),
					NextToken:      imgToken,
				})
				if imErr != nil {
					// Skip repos we can't read (permissions, empty, etc.)
					break
				}
				for _, img := range imgsOut.ImageDetails {
					tags := img.ImageTags
					var repoTags []string
					for _, t := range tags {
						repoTags = append(repoTags, repoURI+":"+t)
					}
					digest := aws.ToString(img.ImageDigest)
					repoDigests := []string{repoURI + "@" + digest}
					size := int64(0)
					if img.ImageSizeInBytes != nil {
						size = *img.ImageSizeInBytes
					}
					pushedAt := int64(0)
					if img.ImagePushedAt != nil {
						pushedAt = img.ImagePushedAt.Unix()
					}
					result = append(result, &api.ImageSummary{
						ID:          digest,
						RepoTags:    repoTags,
						RepoDigests: repoDigests,
						Created:     pushedAt,
						Size:        size,
						VirtualSize: size,
					})
				}
				if imgsOut.NextToken == nil {
					break
				}
				imgToken = imgsOut.NextToken
			}
		}
		if reposOut.NextToken == nil {
			break
		}
		nextToken = reposOut.NextToken
	}
	return result, nil
}

// resolveTaskState returns ECSState for the given container ID, deriving
// it from cloud actuals when the in-memory cache is empty. Per
// , the cache is an optimization; ECS tasks tagged with
// `sockerless-container-id=<id>` are the source of truth. Returns false
// only when no matching sockerless-managed task exists at all.
func (s *Server) resolveTaskState(ctx context.Context, containerID string) (ECSState, bool) {
	if state, ok := s.ECS.Get(containerID); ok && state.TaskARN != "" {
		return state, true
	}
	csp, ok := s.CloudState.(*ecsCloudState)
	if !ok {
		return ECSState{}, false
	}
	arn, clusterARN, err := csp.resolveTaskARN(ctx, containerID)
	if err != nil || arn == "" {
		return ECSState{}, false
	}
	state := ECSState{TaskARN: arn, ClusterARN: clusterARN}
	s.ECS.Update(containerID, func(st *ECSState) {
		st.TaskARN = arn
		st.ClusterARN = clusterARN
	})
	return state, true
}

// resolveTaskARN returns the ECS task ARN for a given container ID, or
// "" if no matching sockerless-managed task is found. Used to recover
// from in-memory state loss after backend restart.
func (p *ecsCloudState) resolveTaskARN(ctx context.Context, containerID string) (string, string, error) {
	var allArns []string
	for _, status := range []ecstypes.DesiredStatus{ecstypes.DesiredStatusRunning, ecstypes.DesiredStatusStopped} {
		out, err := p.ecs.ListTasks(ctx, &awsecs.ListTasksInput{
			Cluster:       aws.String(p.cluster),
			DesiredStatus: status,
		})
		if err == nil {
			allArns = append(allArns, out.TaskArns...)
		}
	}
	if len(allArns) == 0 {
		return "", "", nil
	}
	desc, err := p.ecs.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
		Cluster: aws.String(p.cluster),
		Tasks:   allArns,
		Include: []ecstypes.TaskField{ecstypes.TaskFieldTags},
	})
	if err != nil {
		return "", "", err
	}
	for _, task := range desc.Tasks {
		tags := tagsToMap(task.Tags)
		if tags["sockerless-container-id"] == containerID {
			return aws.ToString(task.TaskArn), aws.ToString(task.ClusterArn), nil
		}
	}
	return "", "", nil
}

// taskToContainer reconstructs an api.Container from an ECS task and its tags.
// The task-definition argument supplies entryPoint/command/environment
// metadata the task itself doesn't carry; pass a zero-value TaskDefinition
// when a description isn't available (caller-visible fields just stay blank).
func taskToContainer(task ecstypes.Task, tags map[string]string, td ecstypes.TaskDefinition) api.Container {
	containerID := tags["sockerless-container-id"]
	name := tags["sockerless-name"]
	if name == "" && containerID != "" {
		name = "/" + containerID[:12]
	}

	// Derive image from the live task's containers (so we see the
	// resolved ECR URI Fargate actually pulled); fall back to the
	// task-def's containerDefinitions when the task list is empty.
	image := ""
	var cmd []string
	var entrypoint []string
	var workingDir string
	var env []string
	if len(task.Containers) > 0 {
		for _, tc := range task.Containers {
			if aws.ToString(tc.Name) == "main" || len(task.Containers) == 1 {
				image = aws.ToString(tc.Image)
				break
			}
		}
	}
	if def, ok := containerDefFromTaskDef(td); ok {
		if image == "" {
			image = aws.ToString(def.Image)
		}
		if len(def.EntryPoint) > 0 {
			entrypoint = append([]string(nil), def.EntryPoint...)
		}
		if len(def.Command) > 0 {
			cmd = append([]string(nil), def.Command...)
		}
		if def.WorkingDirectory != nil {
			workingDir = aws.ToString(def.WorkingDirectory)
		}
		for _, kv := range def.Environment {
			env = append(env, aws.ToString(kv.Name)+"="+aws.ToString(kv.Value))
		}
	}

	// Map ECS status to Docker state
	state := mapTaskStatus(task, tags)

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

	var path string
	var args []string
	combined := append(append([]string(nil), entrypoint...), cmd...)
	if len(combined) > 0 {
		path = combined[0]
		args = combined[1:]
	}

	restartCount := 0
	if v := tags["sockerless-restart-count"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			restartCount = n
		}
	}

	// Map the task-def's CPU/Memory tier back to Docker's HostConfig
	// fields so `docker inspect` reflects the operator's `-m`/`--cpus`
	// request. Fargate stores `task.Memory` as MB (string) and
	// `task.Cpu` as 1024-unit shares (e.g. "256" = 0.25 vCPU). Docker's
	// HostConfig expects bytes and nanoCPUs respectively.
	var memBytes int64
	if task.Memory != nil {
		if mb, err := strconv.ParseInt(aws.ToString(task.Memory), 10, 64); err == nil && mb > 0 {
			memBytes = mb * 1024 * 1024
		}
	}
	var nanoCPUs int64
	if task.Cpu != nil {
		if shares, err := strconv.ParseInt(aws.ToString(task.Cpu), 10, 64); err == nil && shares > 0 {
			// Fargate's "cpu" is in 1024-share units; 1024 = 1 vCPU.
			// Docker's NanoCPUs is 1e9 = 1 CPU.
			nanoCPUs = shares * 1e9 / 1024
		}
	}

	return api.Container{
		ID:           containerID,
		Name:         name,
		Created:      created,
		Image:        image,
		Path:         path,
		Args:         args,
		State:        state,
		RestartCount: restartCount,
		Config: api.ContainerConfig{
			Image:      image,
			Cmd:        cmd,
			Entrypoint: entrypoint,
			Env:        env,
			Labels:     labels,
			WorkingDir: workingDir,
		},
		HostConfig: api.HostConfig{
			NetworkMode: networkName,
			Memory:      memBytes,
			NanoCPUs:    nanoCPUs,
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

// containerDefFromTaskDef returns the "main" container definition, or
// the only one if the task has a single container, or zero value.
func containerDefFromTaskDef(td ecstypes.TaskDefinition) (ecstypes.ContainerDefinition, bool) {
	if len(td.ContainerDefinitions) == 0 {
		return ecstypes.ContainerDefinition{}, false
	}
	if len(td.ContainerDefinitions) == 1 {
		return td.ContainerDefinitions[0], true
	}
	for _, d := range td.ContainerDefinitions {
		if aws.ToString(d.Name) == "main" {
			return d, true
		}
	}
	return td.ContainerDefinitions[0], true
}

// mapTaskStatus converts ECS task status to Docker container state.
func mapTaskStatus(task ecstypes.Task, tags map[string]string) api.ContainerState {
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
		// When the stop was initiated by `docker kill -s <sig>`, override
		// the exit code to the Docker convention (128+signum) so clients
		// see e.g. 143 for SIGTERM rather than whatever the container
		// process actually returned when ECS's grace period expired.
		if sig := tags["sockerless-kill-signal"]; sig != "" {
			exitCode = core.SignalToExitCode(sig)
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
