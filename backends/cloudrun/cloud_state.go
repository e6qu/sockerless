package cloudrun

import (
	"context"
	"strings"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	"google.golang.org/api/iterator"
)

// cloudRunCloudState implements core.CloudStateProvider for Cloud Run.
// All container state is derived from Cloud Run Jobs tagged with sockerless_managed=true.
type cloudRunCloudState struct {
	server *Server
}

func (p *cloudRunCloudState) GetContainer(ctx context.Context, ref string) (api.Container, bool, error) {
	containers, err := p.ListContainers(ctx, true, nil)
	if err != nil {
		return api.Container{}, false, err
	}

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

func (p *cloudRunCloudState) ListContainers(ctx context.Context, all bool, filters map[string][]string) ([]api.Container, error) {
	var result []api.Container

	// Include PendingCreates (containers between create and start)
	for _, c := range p.server.PendingCreates.List() {
		if !all && !c.State.Running {
			continue
		}
		if filters != nil && !core.MatchContainerFilters(c, filters) {
			continue
		}
		result = append(result, c)
	}

	// Query Cloud Run Jobs API for all sockerless-managed jobs
	cloudContainers, err := p.queryJobs(ctx)
	if err != nil {
		return result, err
	}

	// Deduplicate: skip cloud containers that are already in PendingCreates
	pendingIDs := make(map[string]bool)
	for _, c := range p.server.PendingCreates.List() {
		pendingIDs[c.ID] = true
	}

	for _, c := range cloudContainers {
		if pendingIDs[c.ID] {
			continue
		}
		if !all && !c.State.Running {
			continue
		}
		if filters != nil && !core.MatchContainerFilters(c, filters) {
			continue
		}
		result = append(result, c)
	}

	return result, nil
}

func (p *cloudRunCloudState) CheckNameAvailable(ctx context.Context, name string) (bool, error) {
	containers, err := p.ListContainers(ctx, true, nil)
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

func (p *cloudRunCloudState) WaitForExit(ctx context.Context, containerID string) (int, error) {
	ticker := time.NewTicker(p.server.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-ticker.C:
			containers, err := p.ListContainers(ctx, true, nil)
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

// queryJobs fetches all sockerless-managed Cloud Run Jobs and reconstructs containers.
func (p *cloudRunCloudState) queryJobs(ctx context.Context) ([]api.Container, error) {
	var containers []api.Container

	it := p.server.gcp.Jobs.ListJobs(ctx, &runpb.ListJobsRequest{
		Parent: p.server.buildJobParent(),
	})

	for {
		job, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		// Only include sockerless-managed jobs
		if job.Labels["sockerless_managed"] != "true" {
			continue
		}

		c, err := p.jobToContainer(ctx, job)
		if err != nil {
			// Log but skip problematic jobs
			p.server.Logger.Debug().Err(err).Str("job", job.Name).Msg("skipping job in cloud state query")
			continue
		}
		containers = append(containers, c)
	}

	return containers, nil
}

// jobToContainer reconstructs an api.Container from a Cloud Run Job and its execution status.
func (p *cloudRunCloudState) jobToContainer(ctx context.Context, job *runpb.Job) (api.Container, error) {
	labels := job.Labels

	containerID := labels["sockerless_container_id"]
	name := labels["sockerless_name"]
	if name == "" && containerID != "" {
		if len(containerID) >= 12 {
			name = "/" + containerID[:12]
		} else {
			name = "/" + containerID
		}
	}

	// Extract image from job template
	image := ""
	var cmd []string
	var entrypoint []string
	var env []string
	if job.Template != nil && job.Template.Template != nil && len(job.Template.Template.Containers) > 0 {
		mainContainer := job.Template.Template.Containers[0]
		image = mainContainer.Image
		entrypoint = mainContainer.Command
		cmd = mainContainer.Args
		for _, e := range mainContainer.Env {
			env = append(env, e.Name+"="+e.Values.(*runpb.EnvVar_Value).Value)
		}
	}

	// Determine state from latest execution
	state := p.resolveExecutionState(ctx, job)

	// Parse creation time
	created := ""
	if job.CreateTime != nil {
		created = job.CreateTime.AsTime().Format(time.RFC3339Nano)
	}

	// Parse Docker labels from GCP labels (convert underscores back to dashes)
	gcpTags := gcpLabelsToTags(labels)
	dockerLabels := core.ParseLabelsFromTags(gcpTags)
	if dockerLabels == nil {
		dockerLabels = make(map[string]string)
	}

	networkName := labels["sockerless_network"]
	if networkName == "" {
		networkName = "bridge"
	}

	return api.Container{
		ID:      containerID,
		Name:    name,
		Created: created,
		Image:   image,
		State:   state,
		Config: api.ContainerConfig{
			Image:      image,
			Cmd:        cmd,
			Entrypoint: entrypoint,
			Env:        env,
			Labels:     dockerLabels,
		},
		HostConfig: api.HostConfig{
			NetworkMode: networkName,
		},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				networkName: {
					NetworkID: networkName,
				},
			},
		},
	}, nil
}

// resolveExecutionState determines container state from the job's latest execution.
func (p *cloudRunCloudState) resolveExecutionState(ctx context.Context, job *runpb.Job) api.ContainerState {
	latestExec := job.LatestCreatedExecution
	if latestExec == nil || latestExec.Name == "" {
		// No execution created yet — container is in "created" state
		return api.ContainerState{
			Status: "created",
		}
	}

	// Fetch execution details
	exec, err := p.server.gcp.Executions.GetExecution(ctx, &runpb.GetExecutionRequest{
		Name: latestExec.Name,
	})
	if err != nil {
		// If we can't fetch, use the reference's completion info
		if latestExec.CompletionTime != nil {
			return api.ContainerState{
				Status:   "exited",
				ExitCode: 0,
			}
		}
		return api.ContainerState{
			Status: "created",
		}
	}

	// Check execution status
	if exec.RunningCount > 0 {
		startedAt := ""
		if exec.StartTime != nil {
			startedAt = exec.StartTime.AsTime().Format(time.RFC3339Nano)
		}
		return api.ContainerState{
			Status:    "running",
			Running:   true,
			StartedAt: startedAt,
		}
	}

	if exec.CompletionTime != nil {
		// Execution completed
		exitCode := 0
		stateError := ""
		if exec.FailedCount > 0 {
			exitCode = 1
			stateError = "execution failed"
		}
		if exec.CancelledCount > 0 {
			exitCode = 137
			stateError = "execution cancelled"
		}

		startedAt := ""
		if exec.StartTime != nil {
			startedAt = exec.StartTime.AsTime().Format(time.RFC3339Nano)
		}
		finishedAt := ""
		if exec.CompletionTime != nil {
			finishedAt = exec.CompletionTime.AsTime().Format(time.RFC3339Nano)
		}

		return api.ContainerState{
			Status:     "exited",
			ExitCode:   exitCode,
			Error:      stateError,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
		}
	}

	// Execution exists but not yet running or completed — provisioning
	return api.ContainerState{
		Status: "created",
	}
}

// gcpLabelsToTags converts GCP label keys (underscores) back to standard tag keys (dashes).
func gcpLabelsToTags(labels map[string]string) map[string]string {
	m := make(map[string]string, len(labels))
	for k, v := range labels {
		dashKey := strings.ReplaceAll(k, "_", "-")
		m[dashKey] = v
	}
	return m
}
