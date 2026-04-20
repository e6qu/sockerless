package cloudrun

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/logging/logadmin"
	runpb "cloud.google.com/go/run/apiv2/runpb"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	gcpcommon "github.com/sockerless/gcp-common"
)

// Compile-time check that Server implements api.Backend.
var _ api.Backend = (*Server)(nil)

// ContainerCreate creates a container backed by a Cloud Run Job.
func (s *Server) ContainerCreate(req *api.ContainerCreateRequest) (*api.ContainerCreateResponse, error) {
	name := req.Name
	if name == "" {
		name = "/" + core.GenerateName()
	} else if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}

	if avail, _ := s.CloudState.CheckNameAvailable(context.Background(), name); !avail {
		return nil, &api.ConflictError{
			Message: fmt.Sprintf("Conflict. The container name \"%s\" is already in use", strings.TrimPrefix(name, "/")),
		}
	}

	id := core.GenerateID()

	config := api.ContainerConfig{}
	if req.ContainerConfig != nil {
		config = *req.ContainerConfig
	}

	// Merge image config if available
	if img, ok := s.Store.ResolveImage(config.Image); ok {
		config.Env = core.MergeEnvByKey(img.Config.Env, config.Env)
		if len(config.Cmd) == 0 && len(config.Entrypoint) == 0 {
			config.Cmd = img.Config.Cmd
		}
		if len(config.Entrypoint) == 0 {
			config.Entrypoint = img.Config.Entrypoint
		}
		if config.WorkingDir == "" {
			config.WorkingDir = img.Config.WorkingDir
		}
	}
	if config.Labels == nil {
		config.Labels = make(map[string]string)
	}

	// Resolve Docker Hub images to Artifact Registry remote repository URIs
	config.Image = gcpcommon.ResolveGCPImageURI(config.Image, s.config.Project, s.config.Region)

	hostConfig := api.HostConfig{NetworkMode: "default"}
	if req.HostConfig != nil {
		hostConfig = *req.HostConfig
	}
	if hostConfig.NetworkMode == "" {
		hostConfig.NetworkMode = "default"
	}

	path := ""
	var args []string
	if len(config.Entrypoint) > 0 {
		path = config.Entrypoint[0]
		args = append(config.Entrypoint[1:], config.Cmd...)
	} else if len(config.Cmd) > 0 {
		path = config.Cmd[0]
		args = config.Cmd[1:]
	}

	container := api.Container{
		ID:      id,
		Name:    name,
		Created: time.Now().UTC().Format(time.RFC3339Nano),
		Path:    path,
		Args:    args,
		State: api.ContainerState{
			Status:     "created",
			FinishedAt: "0001-01-01T00:00:00Z",
			StartedAt:  "0001-01-01T00:00:00Z",
		},
		Image:      config.Image,
		Config:     config,
		HostConfig: hostConfig,
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
		Mounts:   make([]api.MountPoint, 0),
		Platform: "linux",
		Driver:   "cloudrun-jobs",
	}

	// Set up default network — resolve via store for correct ID and Containers map
	netName := hostConfig.NetworkMode
	if netName == "default" {
		netName = "bridge"
	}
	networkID := netName
	if net, ok := s.Store.ResolveNetwork(netName); ok {
		networkID = net.ID
		// Register container in the network's Containers map
		s.Store.Networks.Update(net.ID, func(n *api.Network) {
			if n.Containers == nil {
				n.Containers = make(map[string]api.EndpointResource)
			}
			n.Containers[id] = api.EndpointResource{
				Name:       strings.TrimPrefix(name, "/"),
				EndpointID: core.GenerateID()[:16],
			}
		})
	}
	container.NetworkSettings.Networks[netName] = &api.EndpointSettings{
		NetworkID:   networkID,
		EndpointID:  core.GenerateID()[:16],
		Gateway:     "",
		IPAddress:   "0.0.0.0",
		IPPrefixLen: 16,
		MacAddress:  "",
	}

	// Pod association is handled by the core HTTP handler layer (query param).
	s.PendingCreates.Put(id, container)

	s.CloudRun.Put(id, CloudRunState{})

	s.EmitEvent("container", "create", id, map[string]string{
		"name":  strings.TrimPrefix(name, "/"),
		"image": config.Image,
	})

	return &api.ContainerCreateResponse{
		ID:       id,
		Warnings: []string{},
	}, nil
}

// ContainerStart starts a Cloud Run Job for the container.
func (s *Server) ContainerStart(ref string) error {
	// Resolve from PendingCreates (containers between create and start)
	c, ok := s.PendingCreates.Get(ref)
	if !ok {
		// Try name/short-ID match in PendingCreates
		for _, pc := range s.PendingCreates.List() {
			if pc.Name == ref || pc.Name == "/"+ref || (len(ref) >= 3 && strings.HasPrefix(pc.ID, ref)) {
				c = pc
				ok = true
				break
			}
		}
	}
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if c.State.Running {
		return &api.NotModifiedError{}
	}

	crState, _ := s.resolveCloudRunState(s.ctx(), id)

	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	s.EmitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})

	// Deferred start: if container is in a multi-container pod, wait for all siblings
	shouldDefer, podContainers := s.PodDeferredStart(id)
	if shouldDefer {
		return nil
	}

	if len(podContainers) > 1 {
		// Multi-container pod: build combined resource and run
		if s.config.UseService {
			return s.startMultiContainerServiceTyped(id, podContainers, exitCh)
		}
		return s.startMultiContainerJobTyped(id, podContainers, exitCh)
	}

	// Phase 87 — Services path. Separate function so the Jobs branch
	// below can be deleted when Jobs support is sunset.
	if s.config.UseService {
		return s.startSingleContainerService(id, c, crState, exitCh)
	}

	// Clean up any existing Cloud Run Job from a previous start
	if crState.JobName != "" {
		s.deleteJob(crState.JobName)
		s.Registry.MarkCleanedUp(crState.JobName)
	}

	// Build Cloud Run Job spec
	jobName := buildJobName(id)
	jobSpec := s.buildJobSpec([]containerInput{
		{ID: id, Container: &c, IsMain: true},
	})

	// Create the Cloud Run Job
	createOp, err := s.gcp.Jobs.CreateJob(s.ctx(), &runpb.CreateJobRequest{
		Parent: s.buildJobParent(),
		JobId:  jobName,
		Job:    jobSpec,
	})
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("failed to create Cloud Run Job")
		s.Store.WaitChs.Delete(id)
		return gcpcommon.MapGCPError(err, "job", id)
	}

	// Wait for job creation to complete
	job, err := createOp.Wait(s.ctx())
	if err != nil {
		s.deleteJob(fmt.Sprintf("%s/jobs/%s", s.buildJobParent(), jobName))
		s.Store.WaitChs.Delete(id)
		s.Logger.Error().Err(err).Str("job", jobName).Msg("job creation failed")
		return gcpcommon.MapGCPError(err, "job", id)
	}

	jobFullName := job.Name

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "cloudrun",
		ResourceType: "job",
		ResourceID:   jobFullName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": c.Image, "name": c.Name, "jobName": jobName},
	})

	// Run the job (creates an execution)
	runOp, err := s.gcp.Jobs.RunJob(s.ctx(), &runpb.RunJobRequest{
		Name: jobFullName,
	})
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobFullName).Msg("failed to run job")
		s.deleteJob(jobFullName)
		s.Store.WaitChs.Delete(id)
		return gcpcommon.MapGCPError(err, "execution", id)
	}

	// Wait for RunJob LRO to return the execution
	execution, err := runOp.Wait(s.ctx())
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobFullName).Msg("run job failed")
		s.deleteJob(jobFullName)
		s.Store.WaitChs.Delete(id)
		return gcpcommon.MapGCPError(err, "execution", id)
	}

	executionName := execution.Name

	// Remove from PendingCreates now that the job is launched in the cloud.
	s.PendingCreates.Delete(id)

	s.CloudRun.Update(id, func(state *CloudRunState) {
		state.JobName = jobFullName
		state.ExecutionName = executionName
	})

	// Start background poller to detect execution exit
	go s.pollExecutionExit(id, executionName, exitCh)

	return nil
}

// startMultiContainerJobTyped creates and runs a Cloud Run Job with all pod containers.
// Called when the last container in a pod is started.
func (s *Server) startMultiContainerJobTyped(triggerID string, podContainers []api.Container, exitCh chan struct{}) error {
	// Build containerInput slice
	var inputs []containerInput
	for i, pc := range podContainers {
		pcCopy := pc
		inputs = append(inputs, containerInput{
			ID:        pc.ID,
			Container: &pcCopy,
			IsMain:    i == 0,
		})
	}

	mainID := podContainers[0].ID

	// Build and create the combined job
	jobName := buildJobName(mainID)
	jobSpec := s.buildJobSpec(inputs)

	createOp, err := s.gcp.Jobs.CreateJob(s.ctx(), &runpb.CreateJobRequest{
		Parent: s.buildJobParent(),
		JobId:  jobName,
		Job:    jobSpec,
	})
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("failed to create multi-container Cloud Run Job")
		for _, pc := range podContainers {
			s.Store.WaitChs.Delete(pc.ID)
		}
		return gcpcommon.MapGCPError(err, "job", mainID)
	}

	job, err := createOp.Wait(s.ctx())
	if err != nil {
		s.deleteJob(fmt.Sprintf("%s/jobs/%s", s.buildJobParent(), jobName))
		for _, pc := range podContainers {
			s.Store.WaitChs.Delete(pc.ID)
		}
		s.Logger.Error().Err(err).Str("job", jobName).Msg("job creation failed")
		return gcpcommon.MapGCPError(err, "job", mainID)
	}

	jobFullName := job.Name

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  mainID,
		Backend:      "cloudrun",
		ResourceType: "job",
		ResourceID:   jobFullName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": podContainers[0].Image, "name": podContainers[0].Name, "jobName": jobName},
	})

	runOp, err := s.gcp.Jobs.RunJob(s.ctx(), &runpb.RunJobRequest{
		Name: jobFullName,
	})
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobFullName).Msg("failed to run job")
		s.deleteJob(jobFullName)
		for _, pc := range podContainers {
			s.Store.WaitChs.Delete(pc.ID)
		}
		return gcpcommon.MapGCPError(err, "execution", mainID)
	}

	execution, err := runOp.Wait(s.ctx())
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobFullName).Msg("run job failed")
		s.deleteJob(jobFullName)
		for _, pc := range podContainers {
			s.Store.WaitChs.Delete(pc.ID)
		}
		return gcpcommon.MapGCPError(err, "execution", mainID)
	}

	executionName := execution.Name

	// Remove all pod containers from PendingCreates now that the job is launched.
	for _, pc := range podContainers {
		s.PendingCreates.Delete(pc.ID)
	}

	// Store cloud state on ALL pod containers
	for _, pc := range podContainers {
		s.CloudRun.Update(pc.ID, func(state *CloudRunState) {
			state.JobName = jobFullName
			state.ExecutionName = executionName
		})
	}

	// Start background poller to detect execution exit
	go s.pollExecutionExit(mainID, executionName, exitCh)

	return nil
}

// ContainerStop stops a running Cloud Run container.
func (s *Server) ContainerStop(ref string, timeout *int) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	// Phase 89 / BUG-725: cloud-fallback lookup so stop works post-restart.
	if crState, ok := s.resolveCloudRunState(s.ctx(), id); ok && crState.ExecutionName != "" {
		s.cancelExecution(crState.ExecutionName)
	}

	s.StopHealthCheck(id)

	// Close wait channel so ContainerWait unblocks
	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": "0", "name": strings.TrimPrefix(c.Name, "/")})
	s.EmitEvent("container", "stop", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	return nil
}

// ContainerKill kills a container with the given signal.
func (s *Server) ContainerKill(ref string, signal string) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if !c.State.Running {
		return &api.ConflictError{
			Message: fmt.Sprintf("Container %s is not running", ref),
		}
	}

	// Disconnect reverse agent if connected (unblocks invoke goroutine)
	s.StopHealthCheck(id)

	exitCode := core.SignalToExitCode(signal)

	// Phase 89 / BUG-725: cloud-fallback lookup so kill works post-restart.
	if crState, ok := s.resolveCloudRunState(s.ctx(), id); ok && crState.ExecutionName != "" {
		s.cancelExecution(crState.ExecutionName)
	}

	s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": fmt.Sprintf("%d", exitCode), "name": strings.TrimPrefix(c.Name, "/")})

	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}

	return nil
}

// ContainerRemove removes a container.
func (s *Server) ContainerRemove(ref string, force bool) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		// Also check PendingCreates (container created but never started)
		if pc, pcOK := s.PendingCreates.Get(ref); pcOK {
			c = pc
			ok = true
		}
	}
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if c.State.Running && !force {
		return &api.ConflictError{
			Message: fmt.Sprintf("You cannot remove a running container %s. Stop the container before attempting removal or force remove", id[:12]),
		}
	}

	// Disconnect reverse agent if connected (unblocks invoke goroutine)

	if c.State.Running {
		s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		crState, _ := s.resolveCloudRunState(s.ctx(), id)
		if crState.ExecutionName != "" {
			s.cancelExecution(crState.ExecutionName)
		}
	}

	s.StopHealthCheck(id)

	// Delete Cloud Run Job (best-effort)
	crState, _ := s.resolveCloudRunState(s.ctx(), id)
	if crState.JobName != "" {
		s.deleteJob(crState.JobName)
		s.Registry.MarkCleanedUp(crState.JobName)
	}

	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod {
		s.Store.Pods.RemoveContainer(pod.ID, id)
	}

	// Deregister from Cloud DNS
	hostname := strings.TrimPrefix(c.Name, "/")
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID != "" {
			if err := s.cloudServiceDeregister(id, hostname, ep.NetworkID); err != nil {
				s.Logger.Warn().Err(err).Str("container", id[:12]).Msg("failed to deregister from Cloud DNS")
			}
		}
	}

	// Clean up network associations
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID != "" {
			_ = s.Drivers.Network.Disconnect(context.Background(), ep.NetworkID, id)
		}
	}

	s.PendingCreates.Delete(id)
	s.CloudRun.Delete(id)
	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
	s.Store.LogBuffers.Delete(id)
	s.Store.StagingDirs.Delete(id)
	if dirs, ok := s.Store.TmpfsDirs.LoadAndDelete(id); ok {
		for _, d := range dirs.([]string) {
			os.RemoveAll(d)
		}
	}
	for _, eid := range c.ExecIDs {
		s.Store.Execs.Delete(eid)
	}

	s.EmitEvent("container", "destroy", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	return nil
}

// ContainerLogs streams container logs from Cloud Logging.
func (s *Server) ContainerLogs(ref string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	// Resolve cloud resource name for the log filter.
	var shortJobName string
	if id, ok := s.ResolveContainerIDAuto(context.Background(), ref); ok {
		crState, _ := s.resolveCloudRunState(s.ctx(), id)
		jobName := crState.JobName
		if jobName == "" {
			jobName = buildJobName(id)
		}
		parts := strings.Split(jobName, "/")
		shortJobName = parts[len(parts)-1]
	}

	baseFilter := fmt.Sprintf(
		`resource.type="cloud_run_job" AND resource.labels.job_name="%s"`,
		shortJobName,
	)

	fetch := s.cloudLoggingFetch(baseFilter)

	return core.StreamCloudLogs(s.BaseServer, ref, opts, fetch, core.StreamCloudLogsOptions{})
}

// cloudLoggingFetch returns a CloudLogFetchFunc that queries Cloud Logging.
// cursor is a *time.Time tracking the latest seen timestamp for dedup.
func (s *Server) cloudLoggingFetch(baseFilter string) core.CloudLogFetchFunc {
	return func(ctx context.Context, params core.CloudLogParams, cursor any) ([]core.CloudLogEntry, any, error) {
		logFilter := baseFilter

		var lastTS time.Time
		if cursor != nil {
			lastTS = cursor.(time.Time)
		}

		if !lastTS.IsZero() {
			// Follow mode: only entries after last seen.
			logFilter += fmt.Sprintf(` AND timestamp>"%s"`, lastTS.UTC().Format(time.RFC3339Nano))
		} else {
			// Initial fetch: apply since/until.
			logFilter += params.CloudLoggingSinceFilter()
			logFilter += params.CloudLoggingUntilFilter()
		}

		fetchCtx, cancel := context.WithTimeout(s.ctx(), s.config.LogTimeout)
		defer cancel()

		it := s.gcp.LogAdmin.Entries(fetchCtx, logadmin.Filter(logFilter))

		var entries []core.CloudLogEntry
		for {
			entry, err := it.Next()
			if err != nil {
				break
			}
			line := extractLogLine(entry)
			if line == "" {
				continue
			}
			entries = append(entries, core.CloudLogEntry{Timestamp: entry.Timestamp, Message: line})
			if entry.Timestamp.After(lastTS) {
				lastTS = entry.Timestamp
			}
		}

		return entries, lastTS, nil
	}
}

// ContainerRestart stops and then starts a container.
func (s *Server) ContainerRestart(ref string, timeout *int) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	// Stop if running
	if c.State.Running {
		s.StopHealthCheck(id)

		crState, _ := s.resolveCloudRunState(s.ctx(), id)
		if crState.ExecutionName != "" {
			s.cancelExecution(crState.ExecutionName)
		}
		if crState.JobName != "" {
			s.deleteJob(crState.JobName)
			s.Registry.MarkCleanedUp(crState.JobName)
		}
		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
		}
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		s.EmitEvent("container", "stop", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	}

	// Re-add to PendingCreates so ContainerStart can find it
	c.State.Status = "created"
	c.State.Running = false
	c.State.Pid = 0
	c.State.StartedAt = "0001-01-01T00:00:00Z"
	c.RestartCount++
	s.PendingCreates.Put(id, c)

	// Start the container directly via typed method
	if err := s.ContainerStart(id); err != nil {
		return err
	}

	s.EmitEvent("container", "restart", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	return nil
}

// ContainerPrune removes all stopped containers.
// In the stateless model, only PendingCreates (never-started) containers are local.
func (s *Server) ContainerPrune(filters map[string][]string) (*api.ContainerPruneResponse, error) {
	labelFilters := filters["label"]
	untilFilters := filters["until"]

	var deleted []string
	var spaceReclaimed uint64

	// Check PendingCreates for containers that were created but never started
	for _, c := range s.PendingCreates.List() {
		if len(labelFilters) > 0 && !core.MatchLabels(c.Config.Labels, labelFilters) {
			continue
		}
		if len(untilFilters) > 0 && !core.MatchUntil(c.Created, untilFilters) {
			continue
		}
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			spaceReclaimed += uint64(img.Size)
		}
		// Clean up Cloud Run resources
		crState, _ := s.resolveCloudRunState(s.ctx(), c.ID)
		if crState.JobName != "" {
			s.deleteJob(crState.JobName)
			s.Registry.MarkCleanedUp(crState.JobName)
		}
		s.StopHealthCheck(c.ID)

		for _, ep := range c.NetworkSettings.Networks {
			if ep != nil && ep.NetworkID != "" {
				_ = s.Drivers.Network.Disconnect(context.Background(), ep.NetworkID, c.ID)
			}
		}
		if pod, inPod := s.Store.Pods.GetPodForContainer(c.ID); inPod {
			s.Store.Pods.RemoveContainer(pod.ID, c.ID)
		}
		s.PendingCreates.Delete(c.ID)
		s.CloudRun.Delete(c.ID)
		if ch, ok := s.Store.WaitChs.LoadAndDelete(c.ID); ok {
			close(ch.(chan struct{}))
		}
		s.Store.LogBuffers.Delete(c.ID)
		s.Store.StagingDirs.Delete(c.ID)
		if dirs, ok := s.Store.TmpfsDirs.LoadAndDelete(c.ID); ok {
			for _, d := range dirs.([]string) {
				os.RemoveAll(d)
			}
		}
		for _, eid := range c.ExecIDs {
			s.Store.Execs.Delete(eid)
		}
		s.EmitEvent("container", "destroy", c.ID, map[string]string{
			"name": strings.TrimPrefix(c.Name, "/"),
		})
		deleted = append(deleted, c.ID)
	}
	if deleted == nil {
		deleted = []string{}
	}
	return &api.ContainerPruneResponse{
		ContainersDeleted: deleted,
		SpaceReclaimed:    spaceReclaimed,
	}, nil
}

// ContainerPause is not supported by Cloud Run backend.
func (s *Server) ContainerPause(ref string) error {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), ref); !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "container pause is not supported by Cloud Run backend"}
}

// ContainerUnpause is not supported by Cloud Run backend.
func (s *Server) ContainerUnpause(ref string) error {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), ref); !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "container unpause is not supported by Cloud Run backend"}
}

// ImagePull delegates to ImageManager which handles cloud auth and config fetching.
func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
	return s.images.Pull(ref, auth)
}

// ImageLoad delegates to ImageManager.
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	return s.images.Load(r)
}

// VolumeRemove removes a volume and its state.
func (s *Server) VolumeRemove(name string, force bool) error {
	if !s.Store.Volumes.Delete(name) {
		return &api.NotFoundError{Resource: "volume", ID: name}
	}
	s.VolumeState.Delete(name)
	return nil
}

// ExecStart is not supported by the Cloud Run backend.
// Cloud Run Jobs do not support native exec — there is no Cloud Run API for
// executing commands inside a running job container.
func (s *Server) ExecStart(id string, opts api.ExecStartRequest) (io.ReadWriteCloser, error) {
	exec, ok := s.Store.Execs.Get(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "exec instance", ID: id}
	}

	if _, ok := s.ResolveContainerAuto(context.Background(), exec.ContainerID); !ok {
		return nil, &api.ConflictError{
			Message: fmt.Sprintf("Container %s has been removed", exec.ContainerID),
		}
	}

	return nil, &api.NotImplementedError{
		Message: "exec is not supported by Cloud Run backend; Cloud Run Jobs do not support native exec",
	}
}

// ContainerExport is not supported by Cloud Run backend.
func (s *Server) ContainerExport(ref string) (io.ReadCloser, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), ref); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}
	return nil, &api.NotImplementedError{Message: "container export is not supported by Cloud Run backend: no container filesystem access"}
}

// ContainerCommit is not supported by Cloud Run backend.
func (s *Server) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), req.Container); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: req.Container}
	}
	return nil, &api.NotImplementedError{Message: "container commit is not supported by Cloud Run backend: cannot create images from running Cloud Run containers"}
}

// PodStart starts all containers in a pod by calling ContainerStart for each.
// This triggers the Cloud Run Job creation via the deferred-start mechanism.
func (s *Server) PodStart(name string) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}

	var errs []string
	for _, cid := range pod.ContainerIDs {
		c, ok := s.ResolveContainerAuto(context.Background(), cid)
		if !ok || c.State.Running {
			continue
		}
		if err := s.ContainerStart(cid); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", cid, err))
		}
	}

	s.Store.Pods.SetStatus(pod.ID, "running")
	if errs == nil {
		errs = []string{}
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// PodStop stops all containers in a pod by calling ContainerStop for each.
// This cancels the Cloud Run executions.
func (s *Server) PodStop(name string, timeout *int) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}

	var errs []string
	for _, cid := range pod.ContainerIDs {
		c, ok := s.ResolveContainerAuto(context.Background(), cid)
		if !ok || !c.State.Running {
			continue
		}
		if err := s.ContainerStop(cid, timeout); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", cid, err))
		}
	}

	s.Store.Pods.SetStatus(pod.ID, "stopped")
	if errs == nil {
		errs = []string{}
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// PodKill sends a signal to all containers in a pod by calling ContainerKill for each.
// This cancels the Cloud Run executions.
func (s *Server) PodKill(name string, signal string) (*api.PodActionResponse, error) {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "pod", ID: name}
	}

	if signal == "" {
		signal = "SIGKILL"
	}

	var errs []string
	for _, cid := range pod.ContainerIDs {
		c, ok := s.ResolveContainerAuto(context.Background(), cid)
		if !ok || !c.State.Running {
			continue
		}
		if err := s.ContainerKill(cid, signal); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", cid, err))
		}
	}

	s.Store.Pods.SetStatus(pod.ID, "exited")
	if errs == nil {
		errs = []string{}
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// PodRemove removes a pod and its containers by calling ContainerRemove for each.
// This deletes the Cloud Run Jobs.
func (s *Server) PodRemove(name string, force bool) error {
	pod, ok := s.Store.Pods.GetPod(name)
	if !ok {
		return &api.NotFoundError{Resource: "pod", ID: name}
	}

	// Without force, reject if any containers are running
	if !force {
		for _, cid := range pod.ContainerIDs {
			c, ok := s.ResolveContainerAuto(context.Background(), cid)
			if ok && c.State.Running {
				return &api.ConflictError{
					Message: fmt.Sprintf("pod %s has running containers, cannot remove without force", name),
				}
			}
		}
	}

	// Remove each container via the typed method (cleans up Cloud Run resources)
	for _, cid := range pod.ContainerIDs {
		if _, ok := s.ResolveContainerAuto(context.Background(), cid); !ok {
			continue
		}
		_ = s.ContainerRemove(cid, force)
	}

	s.Store.Pods.DeletePod(pod.ID)
	return nil
}

// Info returns system information enriched with GCP project/region metadata.
func (s *Server) Info() (*api.BackendInfo, error) {
	info, err := s.BaseServer.Info()
	if err != nil {
		return nil, err
	}

	// Enrich with GCP-specific information
	info.OperatingSystem = fmt.Sprintf("Google Cloud Run (%s/%s)", s.config.Project, s.config.Region)
	info.Driver = "cloudrun-jobs"
	info.KernelVersion = fmt.Sprintf("cloudrun/%s/%s", s.config.Project, s.config.Region)

	return info, nil
}

// ContainerAttach is not supported by the Cloud Run backend.
func (s *Server) ContainerAttach(id string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	if _, ok := s.ResolveContainerAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	return nil, &api.NotImplementedError{
		Message: "attach is not supported by Cloud Run backend",
	}
}

// ContainerTop is not supported by the Cloud Run backend.
func (s *Server) ContainerTop(id string, psArgs string) (*api.ContainerTopResponse, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	if !c.State.Running {
		return nil, &api.ConflictError{Message: fmt.Sprintf("Container %s is not running", id)}
	}

	return nil, &api.NotImplementedError{Message: "container top is not supported by Cloud Run backend"}
}

// ContainerGetArchive is not supported by the Cloud Run backend.
func (s *Server) ContainerGetArchive(id string, path string) (*api.ContainerArchiveResponse, error) {
	if _, ok := s.ResolveContainerAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	return nil, &api.NotImplementedError{
		Message: "archive get is not supported by Cloud Run backend; no container filesystem access",
	}
}

// ContainerPutArchive is not supported by the Cloud Run backend.
func (s *Server) ContainerPutArchive(id string, path string, noOverwriteDirNonDir bool, body io.Reader) error {
	if _, ok := s.ResolveContainerAuto(context.Background(), id); !ok {
		return &api.NotFoundError{Resource: "container", ID: id}
	}

	return &api.NotImplementedError{
		Message: "archive put is not supported by Cloud Run backend; no container filesystem access",
	}
}

// ContainerStatPath is not supported by the Cloud Run backend.
func (s *Server) ContainerStatPath(id string, path string) (*api.ContainerPathStat, error) {
	if _, ok := s.ResolveContainerAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	return nil, &api.NotImplementedError{
		Message: "stat is not supported by Cloud Run backend; no container filesystem access",
	}
}

// ContainerUpdate updates container resource constraints.
// Cloud Run does not support live resource updates.
func (s *Server) ContainerUpdate(id string, req *api.ContainerUpdateRequest) (*api.ContainerUpdateResponse, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	s.Logger.Warn().Str("container", c.ID[:12]).Msg("ContainerUpdate: Cloud Run does not support live resource updates")
	return s.BaseServer.ContainerUpdate(c.ID, req)
}

// ImagePush delegates to ImageManager which handles cloud auth and OCI push.
func (s *Server) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
	return s.images.Push(name, tag, auth)
}

// ImageTag delegates to ImageManager which handles cloud sync.
func (s *Server) ImageTag(source string, repo string, tag string) error {
	return s.images.Tag(source, repo, tag)
}

// ImageRemove delegates to ImageManager which handles cloud sync.
func (s *Server) ImageRemove(name string, force bool, prune bool) ([]*api.ImageDeleteResponse, error) {
	return s.images.Remove(name, force, prune)
}

// AuthLogin handles Docker registry authentication.
// For GCR/Artifact Registry addresses, logs a warning and delegates to BaseServer.
func (s *Server) AuthLogin(req *api.AuthRequest) (*api.AuthResponse, error) {
	addr := req.ServerAddress
	if strings.HasSuffix(addr, ".gcr.io") ||
		strings.HasSuffix(addr, "-docker.pkg.dev") ||
		strings.Contains(addr, ".pkg.dev") {
		s.Logger.Warn().
			Str("registry", addr).
			Msg("GCR/Artifact Registry login: credentials stored locally; use `gcloud auth configure-docker` for production")
	}
	return s.BaseServer.AuthLogin(req)
}

// VolumePrune removes unused volumes.
func (s *Server) VolumePrune(filters map[string][]string) (*api.VolumePruneResponse, error) {
	var deleted []string
	var spaceReclaimed uint64
	for _, v := range s.Store.Volumes.List() {
		inUse := false
		for _, c := range s.PendingCreates.List() {
			for _, m := range c.Mounts {
				if m.Name == v.Name {
					inUse = true
					break
				}
			}
			if inUse {
				break
			}
		}
		if !inUse {
			// Sum volume dir sizes for SpaceReclaimed
			if dir, ok := s.Store.VolumeDirs.Load(v.Name); ok {
				spaceReclaimed += uint64(core.DirSize(dir.(string)))
			}
			s.Store.Volumes.Delete(v.Name)
			s.VolumeState.Delete(v.Name)
			deleted = append(deleted, v.Name)
		}
	}
	if deleted == nil {
		deleted = []string{}
	}
	return &api.VolumePruneResponse{
		VolumesDeleted: deleted,
		SpaceReclaimed: spaceReclaimed,
	}, nil
}
