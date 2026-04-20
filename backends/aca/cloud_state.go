package aca

import (
	"context"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v2"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// acaCloudState implements core.CloudStateProvider for Azure Container Apps.
// All container state is derived from ACA Jobs tagged with sockerless-managed=true.
// PendingCreates are merged for containers between create and start (not yet in cloud).
type acaCloudState struct {
	server *Server
}

func (p *acaCloudState) GetContainer(ctx context.Context, ref string) (api.Container, bool, error) {
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

func (p *acaCloudState) ListContainers(ctx context.Context, all bool, filters map[string][]string) ([]api.Container, error) {
	var result []api.Container

	// Include PendingCreates (containers between create and start, not yet in cloud)
	for _, c := range p.server.PendingCreates.List() {
		if !all && !c.State.Running {
			continue
		}
		if filters != nil && !core.MatchContainerFilters(c, filters) {
			continue
		}
		result = append(result, c)
	}

	// Track pending IDs so we don't duplicate them from the cloud query
	pendingIDs := make(map[string]bool)
	for _, c := range result {
		pendingIDs[c.ID] = true
	}

	// Query Azure Container Apps Jobs API for sockerless-managed resources
	cloudContainers, err := p.queryJobs(ctx)
	if err != nil {
		// Log but don't fail — return what we have from PendingCreates
		p.server.Logger.Debug().Err(err).Msg("failed to query ACA jobs for cloud state")
		return result, nil
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

func (p *acaCloudState) CheckNameAvailable(ctx context.Context, name string) (bool, error) {
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

func (p *acaCloudState) WaitForExit(ctx context.Context, containerID string) (int, error) {
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

// ListImages queries Azure Container Registry via the OCI distribution
// catalog + tags endpoints for every image in the configured ACR.
// Phase 89 / BUG-723 step 2 cross-cloud sibling. Registry host comes
// from `<ACRName>.azurecr.io`; bearer token comes from the
// ACRAuthProvider owned by the ImageManager.
func (p *acaCloudState) ListImages(ctx context.Context) ([]*api.ImageSummary, error) {
	if p.server.config.ACRName == "" {
		return nil, nil
	}
	if p.server.images == nil || p.server.images.Auth == nil {
		return nil, nil
	}
	registry := p.server.config.ACRName + ".azurecr.io"
	token, err := p.server.images.Auth.GetToken(registry)
	if err != nil {
		return nil, err
	}
	return core.OCIListImages(ctx, core.OCIListOptions{
		Registry:  registry,
		AuthToken: token,
	})
}

// resolveNetworkState returns NetworkState for the given docker
// network ID, deriving from cloud actuals when the in-memory cache is
// empty. Phase 89 / BUG-726 cross-cloud sibling. Both the Private DNS
// zone name (`skls-<net>.local`) and the NSG name
// (`nsg-<env>-<net>`) are deterministic from the network name, so a
// simple existence probe against each reconstitutes state.
func (s *Server) resolveNetworkState(ctx context.Context, networkID string) (NetworkState, bool) {
	if state, ok := s.NetworkState.Get(networkID); ok && state.DNSZoneName != "" {
		return state, true
	}
	if s.azure == nil || s.config.ResourceGroup == "" {
		return NetworkState{}, false
	}
	net, ok := s.Store.ResolveNetwork(networkID)
	if !ok {
		return NetworkState{}, false
	}
	zoneName := "skls-" + net.Name + ".local"
	nsgName := "nsg-" + s.config.Environment + "-" + net.Name
	state := NetworkState{}
	if s.azure.PrivateDNSZones != nil {
		if _, err := s.azure.PrivateDNSZones.Get(ctx, s.config.ResourceGroup, zoneName, nil); err == nil {
			state.DNSZoneName = zoneName
		}
	}
	if s.azure.NSG != nil {
		if _, err := s.azure.NSG.Get(ctx, s.config.ResourceGroup, nsgName, nil); err == nil {
			state.NSGName = nsgName
		}
	}
	if state.DNSZoneName == "" && state.NSGName == "" {
		return NetworkState{}, false
	}
	s.NetworkState.Update(networkID, func(ns *NetworkState) {
		if ns.DNSZoneName == "" {
			ns.DNSZoneName = state.DNSZoneName
		}
		if ns.NSGName == "" {
			ns.NSGName = state.NSGName
		}
	})
	return state, true
}

// resolveJobName returns the ACA Job name for a given container ID, or
// "" if no matching sockerless-managed job is found. Phase 89 / BUG-725
// cross-cloud sibling: state derived from cloud actuals (ACA Job tags).
func (p *acaCloudState) resolveJobName(ctx context.Context, containerID string) (string, error) {
	pager := p.server.azure.Jobs.NewListByResourceGroupPager(p.server.config.ResourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", err
		}
		for _, job := range page.Value {
			if job.Tags == nil {
				continue
			}
			tags := azureTagsToMap(job.Tags)
			if tags["sockerless-managed"] != "true" {
				continue
			}
			if tags["sockerless-container-id"] == containerID {
				if job.Name != nil {
					return *job.Name, nil
				}
			}
		}
	}
	return "", nil
}

// resolveActiveExecution returns the name of the latest non-terminal
// execution for an ACA Job, or "" if none. Used when the cache doesn't
// carry an ExecutionName (post-restart).
func (p *acaCloudState) resolveActiveExecution(ctx context.Context, jobName string) (string, error) {
	pager := p.server.azure.Executions.NewListPager(p.server.config.ResourceGroup, jobName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", err
		}
		for _, exec := range page.Value {
			if exec.Status == nil {
				continue
			}
			status := *exec.Status
			if status == armappcontainers.JobExecutionRunningStateRunning || status == armappcontainers.JobExecutionRunningStateProcessing {
				if exec.Name != nil {
					return *exec.Name, nil
				}
			}
		}
	}
	return "", nil
}

// resolveACAState returns ACAState for the given container ID, deriving
// from cloud actuals when the in-memory cache is empty. Phase 89 /
// BUG-725 cross-cloud sibling.
func (s *Server) resolveACAState(ctx context.Context, containerID string) (ACAState, bool) {
	if state, ok := s.ACA.Get(containerID); ok && state.JobName != "" {
		return state, true
	}
	csp, ok := s.CloudState.(*acaCloudState)
	if !ok {
		return ACAState{}, false
	}
	name, err := csp.resolveJobName(ctx, containerID)
	if err != nil || name == "" {
		return ACAState{}, false
	}
	state := ACAState{JobName: name}
	if exec, execErr := csp.resolveActiveExecution(ctx, name); execErr == nil && exec != "" {
		state.ExecutionName = exec
	}
	s.ACA.Update(containerID, func(st *ACAState) {
		if st.JobName == "" {
			st.JobName = name
		}
		if st.ExecutionName == "" && state.ExecutionName != "" {
			st.ExecutionName = state.ExecutionName
		}
	})
	return state, true
}

// queryJobs fetches all sockerless-managed ACA Jobs from the resource group and
// reconstructs api.Container from the job metadata, tags, and execution status.
func (p *acaCloudState) queryJobs(ctx context.Context) ([]api.Container, error) {
	var containers []api.Container

	pager := p.server.azure.Jobs.NewListByResourceGroupPager(p.server.config.ResourceGroup, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, job := range page.Value {
			if job.Tags == nil {
				continue
			}

			tags := azureTagsToMap(job.Tags)

			// Only include sockerless-managed jobs
			if tags["sockerless-managed"] != "true" {
				continue
			}

			c := p.jobToContainer(ctx, job, tags)
			containers = append(containers, c)
		}
	}

	return containers, nil
}

// jobToContainer reconstructs an api.Container from an ACA Job and its tags.
func (p *acaCloudState) jobToContainer(ctx context.Context, job *armappcontainers.Job, tags map[string]string) api.Container {
	containerID := tags["sockerless-container-id"]
	name := tags["sockerless-name"]
	if name == "" && containerID != "" {
		name = "/" + containerID[:12]
	}

	// Derive image, command, entrypoint, env from job template
	image := ""
	var cmd []string
	var entrypoint []string
	var env []string
	if job.Properties != nil && job.Properties.Template != nil {
		for _, tc := range job.Properties.Template.Containers {
			if tc.Name != nil && *tc.Name == "main" || len(job.Properties.Template.Containers) == 1 {
				if tc.Image != nil {
					image = *tc.Image
				}
				for _, a := range tc.Command {
					if a != nil {
						entrypoint = append(entrypoint, *a)
					}
				}
				for _, a := range tc.Args {
					if a != nil {
						cmd = append(cmd, *a)
					}
				}
				for _, ev := range tc.Env {
					if ev.Name != nil && ev.Value != nil {
						env = append(env, *ev.Name+"="+*ev.Value)
					}
				}
				break
			}
		}
	}

	// Determine container state from the latest execution
	state := p.resolveJobState(ctx, job, tags)

	// Parse creation time from tag
	created := tags["sockerless-created-at"]
	if created == "" && job.SystemData != nil && job.SystemData.CreatedAt != nil {
		created = job.SystemData.CreatedAt.Format(time.RFC3339Nano)
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

	path := ""
	var args []string
	if len(entrypoint) > 0 {
		path = entrypoint[0]
		args = append(entrypoint[1:], cmd...)
	} else if len(cmd) > 0 {
		path = cmd[0]
		args = cmd[1:]
	}

	return api.Container{
		ID:      containerID,
		Name:    name,
		Created: created,
		Image:   image,
		Path:    path,
		Args:    args,
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
					NetworkID: networkName,
					IPAddress: "",
				},
			},
		},
		Platform: "linux",
		Driver:   "aca-jobs",
	}
}

// resolveJobState determines Docker container state by querying the latest execution
// for the ACA Job.
func (p *acaCloudState) resolveJobState(ctx context.Context, job *armappcontainers.Job, tags map[string]string) api.ContainerState {
	// Check provisioning state first
	if job.Properties != nil && job.Properties.ProvisioningState != nil {
		switch *job.Properties.ProvisioningState {
		case armappcontainers.JobProvisioningStateInProgress:
			return api.ContainerState{
				Status: "created",
			}
		case armappcontainers.JobProvisioningStateFailed, armappcontainers.JobProvisioningStateCanceled:
			return api.ContainerState{
				Status:   "exited",
				ExitCode: 1,
				Error:    "job provisioning " + string(*job.Properties.ProvisioningState),
			}
		case armappcontainers.JobProvisioningStateDeleting:
			return api.ContainerState{
				Status: "removing",
			}
		}
	}

	// Query executions for the job to get running state
	if job.Name == nil {
		return api.ContainerState{Status: "created"}
	}

	jobName := *job.Name
	pager := p.server.azure.Executions.NewListPager(p.server.config.ResourceGroup, jobName, nil)

	// Find the latest execution
	var latest *armappcontainers.JobExecution
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			p.server.Logger.Debug().Err(err).Str("job", jobName).Msg("failed to list executions")
			break
		}
		for _, exec := range page.Value {
			if latest == nil {
				latest = exec
			} else if exec.StartTime != nil && latest.StartTime != nil && exec.StartTime.After(*latest.StartTime) {
				latest = exec
			}
		}
	}

	if latest == nil {
		// Job exists but no execution — it was created but not yet started
		return api.ContainerState{Status: "created"}
	}

	return mapExecutionStatus(latest)
}

// mapExecutionStatus converts an ACA Job execution to Docker container state.
func mapExecutionStatus(exec *armappcontainers.JobExecution) api.ContainerState {
	if exec.Status == nil {
		return api.ContainerState{Status: "created"}
	}

	switch *exec.Status {
	case armappcontainers.JobExecutionRunningStateRunning,
		armappcontainers.JobExecutionRunningStateProcessing:
		startedAt := ""
		if exec.StartTime != nil {
			startedAt = exec.StartTime.Format(time.RFC3339Nano)
		}
		return api.ContainerState{
			Status:    "running",
			Running:   true,
			StartedAt: startedAt,
		}

	case armappcontainers.JobExecutionRunningStateSucceeded:
		startedAt := ""
		if exec.StartTime != nil {
			startedAt = exec.StartTime.Format(time.RFC3339Nano)
		}
		finishedAt := ""
		if exec.EndTime != nil {
			finishedAt = exec.EndTime.Format(time.RFC3339Nano)
		}
		return api.ContainerState{
			Status:     "exited",
			ExitCode:   0,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
		}

	case armappcontainers.JobExecutionRunningStateFailed,
		armappcontainers.JobExecutionRunningStateDegraded:
		startedAt := ""
		if exec.StartTime != nil {
			startedAt = exec.StartTime.Format(time.RFC3339Nano)
		}
		finishedAt := ""
		if exec.EndTime != nil {
			finishedAt = exec.EndTime.Format(time.RFC3339Nano)
		}
		return api.ContainerState{
			Status:     "exited",
			ExitCode:   1,
			Error:      string(*exec.Status),
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
		}

	case armappcontainers.JobExecutionRunningStateStopped:
		startedAt := ""
		if exec.StartTime != nil {
			startedAt = exec.StartTime.Format(time.RFC3339Nano)
		}
		finishedAt := ""
		if exec.EndTime != nil {
			finishedAt = exec.EndTime.Format(time.RFC3339Nano)
		}
		return api.ContainerState{
			Status:     "exited",
			ExitCode:   137,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
		}

	default:
		return api.ContainerState{Status: "unknown"}
	}
}

// azureTagsToMap converts Azure SDK tag map to a plain map.
func azureTagsToMap(tags map[string]*string) map[string]string {
	m := make(map[string]string, len(tags))
	for k, v := range tags {
		if v != nil {
			m[k] = *v
		}
	}
	return m
}
