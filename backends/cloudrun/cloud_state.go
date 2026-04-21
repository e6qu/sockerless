package cloudrun

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
		// Prefix match (either direction — handles GCP label truncation)
		if len(ref) >= 3 && (strings.HasPrefix(c.ID, ref) || strings.HasPrefix(ref, c.ID)) {
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

	// Query Cloud Run Jobs API for all sockerless-managed jobs. When
	// Config.UseService is true, also include Services —
	// this lets mixed deployments surface both tracks during the
	// migration window. Jobs and Services live in distinct GCP
	// resource namespaces, so there's no double-counting.
	cloudContainers, err := p.queryJobs(ctx)
	if err != nil {
		return result, err
	}
	if p.server.config.UseService {
		services, sErr := p.queryServices(ctx)
		if sErr != nil {
			return result, sErr
		}
		cloudContainers = append(cloudContainers, services...)
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

// ListPods groups sockerless-managed Cloud Run Jobs (and Services
// when UseService is set) by their `sockerless_pod` label and returns
// one PodListEntry per unique pod name. Jobs/Services without a pod
// label are excluded — they surface as standalone containers in
// `docker ps` but not in `docker pod ps`.
func (p *cloudRunCloudState) ListPods(ctx context.Context) ([]*api.PodListEntry, error) {
	containers, err := p.ListContainers(ctx, true, nil)
	if err != nil {
		return nil, err
	}
	containerByID := make(map[string]api.Container, len(containers))
	for _, c := range containers {
		containerByID[c.ID] = c
	}

	groups := make(map[string][]api.PodContainerInfo)
	created := make(map[string]string)
	status := make(map[string]string)

	walkJob := func(labels map[string]string, annotations map[string]string, createdAt string) {
		if labels["sockerless_managed"] != "true" {
			return
		}
		podName := labels["sockerless_pod"]
		if podName == "" {
			return
		}
		cid := annotations["sockerless_container_id"]
		if cid == "" {
			cid = labels["sockerless_container_id"]
		}
		cont, ok := containerByID[cid]
		if !ok {
			return
		}
		groups[podName] = append(groups[podName], api.PodContainerInfo{
			ID:    cont.ID,
			Name:  strings.TrimPrefix(cont.Name, "/"),
			State: cont.State.Status,
		})
		if createdAt != "" && (created[podName] == "" || createdAt < created[podName]) {
			created[podName] = createdAt
		}
		if cont.State.Running {
			status[podName] = "Running"
		} else if status[podName] == "" {
			status[podName] = cont.State.Status
		}
	}

	// Walk Jobs.
	jobIt := p.server.gcp.Jobs.ListJobs(ctx, &runpb.ListJobsRequest{
		Parent: p.server.buildJobParent(),
	})
	for {
		job, err := jobIt.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		ca := ""
		if job.CreateTime != nil {
			ca = job.CreateTime.AsTime().Format(time.RFC3339Nano)
		}
		walkJob(job.Labels, job.Annotations, ca)
	}

	// Walk Services too when UseService is on — mixed deployments
	// surface both tracks, and Services carry the same tag layout.
	if p.server.config.UseService && p.server.gcp.Services != nil {
		svcIt := p.server.gcp.Services.ListServices(ctx, &runpb.ListServicesRequest{
			Parent: p.server.buildServiceParent(),
		})
		for {
			svc, err := svcIt.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, err
			}
			ca := ""
			if svc.CreateTime != nil {
				ca = svc.CreateTime.AsTime().Format(time.RFC3339Nano)
			}
			walkJob(svc.Labels, svc.Annotations, ca)
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

// ListImages queries GCP Artifact Registry via the OCI distribution
// catalog + tags endpoints for every image under the backend's
// configured registry./step 2 cross-cloud sibling.
// Registry host is derived from project+region: `<region>-docker.pkg.dev`.
// Bearer token comes from the ARAuthProvider that the ImageManager
// already owns.
func (p *cloudRunCloudState) ListImages(ctx context.Context) ([]*api.ImageSummary, error) {
	if p.server.config.Region == "" || p.server.config.Project == "" {
		return nil, nil
	}
	if p.server.images == nil || p.server.images.Auth == nil {
		return nil, nil
	}
	registry := p.server.config.Region + "-docker.pkg.dev"
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
// empty./cross-cloud sibling. Looks up the Cloud
// DNS managed zone whose sanitized name matches the network.
// (Zones are created per-network in `cloudNetworkCreate`.)
func (s *Server) resolveNetworkState(ctx context.Context, networkID string) (NetworkState, bool) {
	if state, ok := s.NetworkState.Get(networkID); ok && state.ManagedZoneName != "" {
		return state, true
	}
	if s.gcp == nil || s.gcp.DNS == nil || s.config.Project == "" {
		return NetworkState{}, false
	}
	net, ok := s.Store.ResolveNetwork(networkID)
	if !ok {
		return NetworkState{}, false
	}
	zoneName := sanitizeZoneName(net.Name)
	zone, err := s.gcp.DNS.ManagedZones.Get(s.config.Project, zoneName).Context(ctx).Do()
	if err != nil || zone == nil {
		return NetworkState{}, false
	}
	state := NetworkState{
		ManagedZoneName: zone.Name,
		DNSName:         zone.DnsName,
	}
	s.NetworkState.Update(networkID, func(ns *NetworkState) {
		if ns.ManagedZoneName == "" {
			ns.ManagedZoneName = zone.Name
		}
		if ns.DNSName == "" {
			ns.DNSName = zone.DnsName
		}
	})
	return state, true
}

// resolveJobName returns the Cloud Run Job name for a given container
// ID, or "" if no matching sockerless-managed job is found./
// cross-cloud sibling: state derived from cloud actuals (Cloud
// Run Job labels), not from the in-memory cache.
func (p *cloudRunCloudState) resolveJobName(ctx context.Context, containerID string) (string, error) {
	it := p.server.gcp.Jobs.ListJobs(ctx, &runpb.ListJobsRequest{
		Parent: p.server.buildJobParent(),
	})
	for {
		job, err := it.Next()
		if err == iterator.Done {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if job.Labels["sockerless_managed"] != "true" {
			continue
		}
		// Full ID lives in annotations (labels truncate at 63 chars); fall
		// back to the label for older jobs.
		jobContainerID := job.Annotations["sockerless_container_id"]
		if jobContainerID == "" {
			jobContainerID = job.Labels["sockerless_container_id"]
		}
		if jobContainerID == containerID {
			return job.Name, nil
		}
	}
}

// resolveActiveExecution returns the name of the latest non-terminal
// execution for a Cloud Run Job, or "" if none. Used when the cache
// doesn't carry an ExecutionName (post-restart) and the caller still
// needs to cancel a running execution.
func (p *cloudRunCloudState) resolveActiveExecution(ctx context.Context, jobName string) (string, error) {
	it := p.server.gcp.Executions.ListExecutions(ctx, &runpb.ListExecutionsRequest{
		Parent: jobName,
	})
	for {
		exec, err := it.Next()
		if err == iterator.Done {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		// CompletionTime is set when the execution is no longer running.
		if exec.CompletionTime == nil {
			return exec.Name, nil
		}
	}
}

// resolveCloudRunState returns CloudRunState for the given container
// ID, deriving from cloud actuals when the in-memory cache is empty.
// cross-cloud sibling.
func (s *Server) resolveCloudRunState(ctx context.Context, containerID string) (CloudRunState, bool) {
	if state, ok := s.CloudRun.Get(containerID); ok && state.JobName != "" {
		return state, true
	}
	csp, ok := s.CloudState.(*cloudRunCloudState)
	if !ok {
		return CloudRunState{}, false
	}
	name, err := csp.resolveJobName(ctx, containerID)
	if err != nil || name == "" {
		return CloudRunState{}, false
	}
	state := CloudRunState{JobName: name}
	// Best-effort: fetch the active execution so Stop/Kill have a
	// target. If the job isn't running, ExecutionName stays empty.
	if exec, execErr := csp.resolveActiveExecution(ctx, name); execErr == nil && exec != "" {
		state.ExecutionName = exec
	}
	s.CloudRun.Update(containerID, func(st *CloudRunState) {
		if st.JobName == "" {
			st.JobName = name
		}
		if st.ExecutionName == "" && state.ExecutionName != "" {
			st.ExecutionName = state.ExecutionName
		}
	})
	return state, true
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

	// Full container ID from annotations (labels truncate at 63 chars, IDs are 64)
	containerID := job.Annotations["sockerless_container_id"]
	if containerID == "" {
		containerID = labels["sockerless_container_id"]
	}
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

	// Phase 97 (BUG-746): Docker labels round-trip via three places, in
	// priority order:
	//   1. SOCKERLESS_LABELS env var on the main container (robust against
	//      control-plane or sim-side annotation stripping).
	//   2. Job.Annotations (GCP annotations for labels whose JSON blob
	//      fails the label-value charset).
	//   3. Job.Labels (legacy split-across-chunks fallback).
	dockerLabels := decodeLabelsFromEnv(env)
	if len(dockerLabels) == 0 {
		merged := mergeLabelsAndAnnotations(labels, job.Annotations)
		gcpTags := gcpLabelsToTags(merged)
		if parsed := core.ParseLabelsFromTags(gcpTags); parsed != nil {
			dockerLabels = parsed
		}
	}
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

// decodeLabelsFromEnv extracts the Docker labels map from the
// `SOCKERLESS_LABELS` env var (base64-encoded JSON) injected by
// jobspec.go / servicespec.go. Returns nil if the var is missing or
// malformed. Phase 97.
func decodeLabelsFromEnv(env []string) map[string]string {
	for _, e := range env {
		if !strings.HasPrefix(e, "SOCKERLESS_LABELS=") {
			continue
		}
		b64 := strings.TrimPrefix(e, "SOCKERLESS_LABELS=")
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil
		}
		var out map[string]string
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil
		}
		return out
	}
	return nil
}

// mergeLabelsAndAnnotations combines GCP labels + annotations into a
// single map (annotations win on key collision since they carry the
// unmutated full-fidelity values). Phase 97: needed because Docker
// labels land in annotations but identity fields land in labels, and
// ParseLabelsFromTags needs both to reconstruct a container's metadata.
func mergeLabelsAndAnnotations(labels, annotations map[string]string) map[string]string {
	out := make(map[string]string, len(labels)+len(annotations))
	for k, v := range labels {
		out[k] = v
	}
	for k, v := range annotations {
		out[k] = v
	}
	return out
}
