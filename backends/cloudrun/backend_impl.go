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

	if _, exists := s.Store.ContainerNames.Get(name); exists {
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

	// Set up default network
	netName := hostConfig.NetworkMode
	if netName == "default" {
		netName = "bridge"
	}
	container.NetworkSettings.Networks[netName] = &api.EndpointSettings{
		NetworkID:   netName,
		EndpointID:  core.GenerateID()[:16],
		Gateway:     "172.17.0.1",
		IPAddress:   fmt.Sprintf("172.17.0.%d", int(s.ipCounter.Add(1))),
		IPPrefixLen: 16,
		MacAddress:  "02:42:ac:11:00:02",
	}

	agentToken := s.config.AgentToken
	if agentToken == "" {
		agentToken = core.GenerateToken()
	}

	// Pod association is handled by the core HTTP handler layer (query param).
	s.Store.Containers.Put(id, container)
	s.Store.ContainerNames.Put(name, id)

	s.CloudRun.Put(id, CloudRunState{
		AgentToken: agentToken,
	})

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
	// When auto-agent is configured, skip cloud task launch entirely.
	// BaseServer.ContainerStart marks running and spawns a local agent.
	if os.Getenv("SOCKERLESS_AUTO_AGENT_BIN") != "" {
		return s.BaseServer.ContainerStart(ref)
	}

	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	if c.State.Running {
		return &api.NotModifiedError{}
	}

	crState, _ := s.CloudRun.Get(id)

	// Update container state to running
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.Store.Containers.Update(id, func(c *api.Container) {
		c.State.Status = "running"
		c.State.Running = true
		c.State.Pid = 1
		c.State.StartedAt = now
		c.State.FinishedAt = "0001-01-01T00:00:00Z"
		c.State.ExitCode = 0
	})

	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	c, _ = s.Store.Containers.Get(id)
	s.EmitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})

	// Deferred start: if container is in a multi-container pod, wait for all siblings
	shouldDefer, podContainers := s.PodDeferredStart(id)
	if shouldDefer {
		return nil
	}

	if len(podContainers) > 1 {
		// Multi-container pod: build combined job and run
		return s.startMultiContainerJobTyped(id, podContainers, exitCh)
	}

	// Pre-create done channel so invoke goroutine can wait for agent disconnect
	if s.config.CallbackURL != "" {
		s.AgentRegistry.Prepare(id)
	}

	// Clean up any existing Cloud Run Job from a previous start
	if crState.JobName != "" {
		s.deleteJob(crState.JobName)
		s.Registry.MarkCleanedUp(crState.JobName)
	}

	// Build Cloud Run Job spec
	jobName := buildJobName(id)
	jobSpec := s.buildJobSpec([]containerInput{
		{ID: id, Container: &c, AgentToken: crState.AgentToken, IsMain: true},
	})

	// Create the Cloud Run Job
	createOp, err := s.gcp.Jobs.CreateJob(s.ctx(), &runpb.CreateJobRequest{
		Parent: s.buildJobParent(),
		JobId:  jobName,
		Job:    jobSpec,
	})
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("failed to create Cloud Run Job")
		s.AgentRegistry.Remove(id)
		s.Store.RevertToCreated(id)
		return gcpcommon.MapGCPError(err, "job", id)
	}

	// Wait for job creation to complete
	job, err := createOp.Wait(s.ctx())
	if err != nil {
		s.deleteJob(fmt.Sprintf("%s/jobs/%s", s.buildJobParent(), jobName))
		s.AgentRegistry.Remove(id)
		s.Store.RevertToCreated(id)
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
		s.AgentRegistry.Remove(id)
		s.Store.RevertToCreated(id)
		return gcpcommon.MapGCPError(err, "execution", id)
	}

	// Wait for RunJob LRO to return the execution
	execution, err := runOp.Wait(s.ctx())
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobFullName).Msg("run job failed")
		s.deleteJob(jobFullName)
		s.AgentRegistry.Remove(id)
		s.Store.RevertToCreated(id)
		return gcpcommon.MapGCPError(err, "execution", id)
	}

	executionName := execution.Name

	s.CloudRun.Update(id, func(state *CloudRunState) {
		state.JobName = jobFullName
		state.ExecutionName = executionName
	})

	if s.config.CallbackURL != "" {
		// Reverse agent mode: wait for agent callback instead of polling execution IP
		go func() {
			// Wait for execution to complete
			s.waitForExecutionComplete(executionName, exitCh)

			// Wait for reverse agent to disconnect before stopping
			_ = s.AgentRegistry.WaitForDisconnect(id, 30*time.Minute)

			if _, ok := s.Store.Containers.Get(id); ok {
				s.Store.StopContainer(id, 0)
			}
		}()

		// Wait for reverse agent callback
		agentTimeout := s.config.AgentTimeout
		if err := s.AgentRegistry.WaitForAgent(id, agentTimeout); err != nil {
			s.Logger.Warn().Err(err).Msg("agent callback timeout, trying auto-agent")
			s.AgentRegistry.Remove(id)
			// Fallback to auto-agent if configured
			if autoErr := s.SpawnAutoAgent(id); autoErr != nil {
				s.Logger.Warn().Err(autoErr).Msg("auto-agent fallback failed")
			}
		} else {
			s.Store.Containers.Update(id, func(c *api.Container) {
				c.AgentAddress = "reverse"
				c.AgentToken = crState.AgentToken
			})
		}
	} else {
		// Forward agent mode: poll for execution RUNNING and health check
		isLongRunning := core.IsTailDevNull(c.Config.Entrypoint, c.Config.Cmd)
		skipPoller := false

		if isLongRunning {
			// Long-running container: wait for RUNNING and check agent health
			agentAddr, completedExitCode, err := s.waitForExecutionRunning(s.ctx(), executionName)
			if err != nil {
				s.Logger.Error().Err(err).Str("execution", executionName).Msg("execution failed to reach RUNNING state")
				s.AgentRegistry.Remove(id)
				s.deleteJob(jobFullName)
				s.Store.RevertToCreated(id)
				return gcpcommon.MapGCPError(err, "execution", id)
			}

			if completedExitCode >= 0 {
				skipPoller = true
				go func() {
					if _, ok := s.Store.Containers.Get(id); ok {
						s.Store.StopContainer(id, completedExitCode)
					}
				}()
			} else if agentAddr == "reverse" {
				// Use reverse agent callback
				s.AgentRegistry.Prepare(id)
				if err := s.AgentRegistry.WaitForAgent(id, s.config.AgentTimeout); err != nil {
					s.Logger.Warn().Err(err).Msg("agent callback timeout")
					s.AgentRegistry.Remove(id)
				} else {
					s.Store.Containers.Update(id, func(c *api.Container) {
						c.AgentAddress = "reverse"
						c.AgentToken = crState.AgentToken
					})
				}
				s.CloudRun.Update(id, func(state *CloudRunState) {
					state.AgentAddress = "reverse"
				})
			} else {
				// Wait for agent health
				agentURL := fmt.Sprintf("http://%s/health", agentAddr)
				agentHealthy := true
				if err := s.waitForAgentHealth(s.ctx(), agentURL); err != nil {
					s.Logger.Warn().Err(err).Str("agent", agentAddr).Msg("agent health check failed")
					agentHealthy = false
				}

				if agentHealthy {
					s.Store.Containers.Update(id, func(c *api.Container) {
						c.AgentAddress = agentAddr
						c.AgentToken = crState.AgentToken
					})
				} else {
					// Fallback to auto-agent if configured
					if autoErr := s.SpawnAutoAgent(id); autoErr != nil {
						s.Logger.Warn().Err(autoErr).Msg("auto-agent fallback failed")
					}
				}

				s.CloudRun.Update(id, func(state *CloudRunState) {
					state.AgentAddress = agentAddr
				})
			}
		} else {
			// Short-lived container without forward agent — try auto-agent
			if autoErr := s.SpawnAutoAgent(id); autoErr != nil {
				s.Logger.Warn().Err(autoErr).Msg("auto-agent fallback failed")
			}
		}

		if !skipPoller {
			go s.pollExecutionExit(id, executionName, exitCh)
		}
	}

	return nil
}

// startMultiContainerJobTyped creates and runs a Cloud Run Job with all pod containers.
// Called when the last container in a pod is started.
func (s *Server) startMultiContainerJobTyped(triggerID string, podContainers []api.Container, exitCh chan struct{}) error {
	// Build containerInput slice: first container is main (gets agent)
	var inputs []containerInput
	for i, pc := range podContainers {
		state, _ := s.CloudRun.Get(pc.ID)
		pcCopy := pc
		inputs = append(inputs, containerInput{
			ID:         pc.ID,
			Container:  &pcCopy,
			AgentToken: state.AgentToken,
			IsMain:     i == 0,
		})
	}

	mainID := podContainers[0].ID
	mainState, _ := s.CloudRun.Get(mainID)

	// Pre-create done channel for reverse agent on main container
	if s.config.CallbackURL != "" {
		s.AgentRegistry.Prepare(mainID)
	}

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
		if s.config.CallbackURL != "" {
			s.AgentRegistry.Remove(mainID)
		}
		for _, pc := range podContainers {
			s.Store.RevertToCreated(pc.ID)
		}
		return gcpcommon.MapGCPError(err, "job", mainID)
	}

	job, err := createOp.Wait(s.ctx())
	if err != nil {
		s.deleteJob(fmt.Sprintf("%s/jobs/%s", s.buildJobParent(), jobName))
		if s.config.CallbackURL != "" {
			s.AgentRegistry.Remove(mainID)
		}
		for _, pc := range podContainers {
			s.Store.RevertToCreated(pc.ID)
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
		if s.config.CallbackURL != "" {
			s.AgentRegistry.Remove(mainID)
		}
		for _, pc := range podContainers {
			s.Store.RevertToCreated(pc.ID)
		}
		return gcpcommon.MapGCPError(err, "execution", mainID)
	}

	execution, err := runOp.Wait(s.ctx())
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobFullName).Msg("run job failed")
		s.deleteJob(jobFullName)
		if s.config.CallbackURL != "" {
			s.AgentRegistry.Remove(mainID)
		}
		for _, pc := range podContainers {
			s.Store.RevertToCreated(pc.ID)
		}
		return gcpcommon.MapGCPError(err, "execution", mainID)
	}

	executionName := execution.Name

	// Store cloud state on ALL pod containers
	for _, pc := range podContainers {
		s.CloudRun.Update(pc.ID, func(state *CloudRunState) {
			state.JobName = jobFullName
			state.ExecutionName = executionName
		})
	}

	if s.config.CallbackURL != "" {
		// Reverse agent mode
		go func() {
			s.waitForExecutionComplete(executionName, exitCh)
			_ = s.AgentRegistry.WaitForDisconnect(mainID, 30*time.Minute)
			if _, ok := s.Store.Containers.Get(mainID); ok {
				s.Store.StopContainer(mainID, 0)
			}
		}()

		agentTimeout := s.config.AgentTimeout
		if err := s.AgentRegistry.WaitForAgent(mainID, agentTimeout); err != nil {
			s.Logger.Warn().Err(err).Msg("agent callback timeout, exec will use synthetic fallback")
			s.AgentRegistry.Remove(mainID)
		} else {
			s.Store.Containers.Update(mainID, func(c *api.Container) {
				c.AgentAddress = "reverse"
				c.AgentToken = mainState.AgentToken
			})
		}
	} else {
		// Forward agent mode
		agentAddr, completedExitCode, err := s.waitForExecutionRunning(s.ctx(), executionName)
		if err != nil {
			s.Logger.Error().Err(err).Str("execution", executionName).Msg("execution failed to reach RUNNING state")
			s.deleteJob(jobFullName)
			if s.config.CallbackURL != "" {
				s.AgentRegistry.Remove(mainID)
			}
			for _, pc := range podContainers {
				s.Store.RevertToCreated(pc.ID)
			}
			return gcpcommon.MapGCPError(err, "execution", mainID)
		}

		if completedExitCode >= 0 {
			go func() {
				if _, ok := s.Store.Containers.Get(mainID); ok {
					s.Store.StopContainer(mainID, completedExitCode)
				}
			}()
		} else {
			agentURL := fmt.Sprintf("http://%s/health", agentAddr)
			agentHealthy := true
			if err := s.waitForAgentHealth(s.ctx(), agentURL); err != nil {
				s.Logger.Warn().Err(err).Str("agent", agentAddr).Msg("agent health check failed")
				agentHealthy = false
			}

			if agentHealthy {
				s.Store.Containers.Update(mainID, func(c *api.Container) {
					c.AgentAddress = agentAddr
					c.AgentToken = mainState.AgentToken
				})
			}

			s.CloudRun.Update(mainID, func(state *CloudRunState) {
				state.AgentAddress = agentAddr
			})

			go s.pollExecutionExit(mainID, executionName, exitCh)
		}
	}

	return nil
}

// ContainerStop stops a running Cloud Run container.
func (s *Server) ContainerStop(ref string, timeout *int) error {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	// Cancel the Cloud Run execution
	crState, _ := s.CloudRun.Get(id)
	if crState.ExecutionName != "" {
		s.cancelExecution(crState.ExecutionName)
	}

	s.StopHealthCheck(id)
	s.AgentRegistry.Remove(id)
	core.StopAutoAgent(id)
	s.Store.ForceStopContainer(id, 0)
	c, _ = s.Store.Containers.Get(id)
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": fmt.Sprintf("%d", c.State.ExitCode), "name": strings.TrimPrefix(c.Name, "/")})
	s.EmitEvent("container", "stop", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	return nil
}

// ContainerKill kills a container with the given signal.
func (s *Server) ContainerKill(ref string, signal string) error {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		return &api.ConflictError{
			Message: fmt.Sprintf("Container %s is not running", ref),
		}
	}

	// Disconnect reverse agent if connected (unblocks invoke goroutine)
	s.StopHealthCheck(id)
	s.AgentRegistry.Remove(id)
	core.StopAutoAgent(id)

	exitCode := core.SignalToExitCode(signal)

	// Cancel the Cloud Run execution
	crState, _ := s.CloudRun.Get(id)
	if crState.ExecutionName != "" {
		s.cancelExecution(crState.ExecutionName)
	}

	s.Store.Containers.Update(id, func(c *api.Container) {
		c.State.Status = "exited"
		c.State.Running = false
		c.State.Pid = 0
		c.State.ExitCode = exitCode
		c.State.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	})

	s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": fmt.Sprintf("%d", exitCode), "name": strings.TrimPrefix(c.Name, "/")})

	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}

	return nil
}

// ContainerRemove removes a container.
func (s *Server) ContainerRemove(ref string, force bool) error {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)

	if c.State.Running && !force {
		return &api.ConflictError{
			Message: fmt.Sprintf("You cannot remove a running container %s. Stop the container before attempting removal or force remove", id[:12]),
		}
	}

	// Disconnect reverse agent if connected (unblocks invoke goroutine)
	s.AgentRegistry.Remove(id)
	core.StopAutoAgent(id)

	if c.State.Running {
		s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		crState, _ := s.CloudRun.Get(id)
		if crState.ExecutionName != "" {
			s.cancelExecution(crState.ExecutionName)
		}
		s.Store.ForceStopContainer(id, 0)
	}

	s.StopHealthCheck(id)

	// Delete Cloud Run Job (best-effort)
	crState, _ := s.CloudRun.Get(id)
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

	s.Store.Containers.Delete(id)
	s.Store.ContainerNames.Delete(c.Name)
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
	if id, ok := s.Store.ResolveContainerID(ref); ok {
		crState, _ := s.CloudRun.Get(id)
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

	return core.StreamCloudLogs(s.BaseServer, ref, opts, fetch, core.StreamCloudLogsOptions{
		CheckAutoAgent: true,
	})
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
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)

	// Stop if running
	if c.State.Running {
		s.StopHealthCheck(id)
		s.AgentRegistry.Remove(id)
		crState, _ := s.CloudRun.Get(id)
		if crState.ExecutionName != "" {
			s.cancelExecution(crState.ExecutionName)
		}
		if crState.JobName != "" {
			s.deleteJob(crState.JobName)
			s.Registry.MarkCleanedUp(crState.JobName)
		}
		s.Store.ForceStopContainer(id, 0)
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		s.EmitEvent("container", "stop", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	}

	s.Store.Containers.Update(id, func(c *api.Container) {
		c.RestartCount++
	})

	// Start the container directly via typed method
	if err := s.ContainerStart(id); err != nil {
		return err
	}

	s.EmitEvent("container", "restart", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	return nil
}

// ContainerPrune removes all stopped containers.
func (s *Server) ContainerPrune(filters map[string][]string) (*api.ContainerPruneResponse, error) {
	labelFilters := filters["label"]
	untilFilters := filters["until"]

	var deleted []string
	var spaceReclaimed uint64
	for _, c := range s.Store.Containers.List() {
		if c.State.Status != "exited" && c.State.Status != "dead" {
			continue
		}
		if len(labelFilters) > 0 && !core.MatchLabels(c.Config.Labels, labelFilters) {
			continue
		}
		if len(untilFilters) > 0 && !core.MatchUntil(c.Created, untilFilters) {
			continue
		}
		// BUG-479: Sum image sizes for SpaceReclaimed
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			spaceReclaimed += uint64(img.Size)
		}
		// Clean up Cloud Run resources
		crState, _ := s.CloudRun.Get(c.ID)
		if crState.JobName != "" {
			s.deleteJob(crState.JobName)
			s.Registry.MarkCleanedUp(crState.JobName)
		}
		s.StopHealthCheck(c.ID)
		s.AgentRegistry.Remove(c.ID)
		// Clean up network associations
		for _, ep := range c.NetworkSettings.Networks {
			if ep != nil && ep.NetworkID != "" {
				_ = s.Drivers.Network.Disconnect(context.Background(), ep.NetworkID, c.ID)
			}
		}
		if pod, inPod := s.Store.Pods.GetPodForContainer(c.ID); inPod {
			s.Store.Pods.RemoveContainer(pod.ID, c.ID)
		}
		s.Store.Containers.Delete(c.ID)
		s.Store.ContainerNames.Delete(c.Name)
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
	if _, ok := s.Store.ResolveContainerID(ref); !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "container pause is not supported by Cloud Run backend"}
}

// ContainerUnpause is not supported by Cloud Run backend.
func (s *Server) ContainerUnpause(ref string) error {
	if _, ok := s.Store.ResolveContainerID(ref); !ok {
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

// ExecStart checks for an agent connection before allowing exec.
// Cloud Run Jobs do not support native exec — there is no Cloud Run API for
// executing commands inside a running job container. An agent sidecar must be
// connected to proxy exec requests into the container.
func (s *Server) ExecStart(id string, opts api.ExecStartRequest) (io.ReadWriteCloser, error) {
	exec, ok := s.Store.Execs.Get(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "exec instance", ID: id}
	}

	c, ok := s.Store.Containers.Get(exec.ContainerID)
	if !ok {
		return nil, &api.ConflictError{
			Message: fmt.Sprintf("Container %s has been removed", exec.ContainerID),
		}
	}

	// If the container has an agent connected, delegate to BaseServer which
	// will proxy through the agent's exec driver.
	if c.AgentAddress != "" {
		return s.BaseServer.ExecStart(id, opts)
	}

	return nil, &api.NotImplementedError{
		Message: "exec requires an agent connection; Cloud Run Jobs do not support local exec",
	}
}

// ContainerExport is not supported by Cloud Run backend.
func (s *Server) ContainerExport(ref string) (io.ReadCloser, error) {
	if _, ok := s.Store.ResolveContainerID(ref); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}
	return nil, &api.NotImplementedError{Message: "container export is not supported by Cloud Run backend: no container filesystem access"}
}

// ContainerCommit is not supported by Cloud Run backend.
func (s *Server) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	if _, ok := s.Store.ResolveContainerID(req.Container); !ok {
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
		c, ok := s.Store.Containers.Get(cid)
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
		c, ok := s.Store.Containers.Get(cid)
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
		c, ok := s.Store.Containers.Get(cid)
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
			c, ok := s.Store.Containers.Get(cid)
			if ok && c.State.Running {
				return &api.ConflictError{
					Message: fmt.Sprintf("pod %s has running containers, cannot remove without force", name),
				}
			}
		}
	}

	// Remove each container via the typed method (cleans up Cloud Run resources)
	for _, cid := range pod.ContainerIDs {
		if _, ok := s.Store.Containers.Get(cid); !ok {
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

// ContainerAttach attaches to a container's IO streams.
// Cloud Run Jobs do not support local attach — an agent must be connected.
func (s *Server) ContainerAttach(id string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	cid, ok := s.Store.ResolveContainerID(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	c, ok := s.Store.Containers.Get(cid)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	if c.AgentAddress != "" {
		return s.BaseServer.ContainerAttach(id, opts)
	}

	return nil, &api.NotImplementedError{
		Message: "attach requires an agent connection; Cloud Run Jobs do not support local attach",
	}
}

// ContainerTop lists processes running inside a container.
// Cloud Run Jobs do not support local ps — delegates to BaseServer which returns a synthetic response.
func (s *Server) ContainerTop(id string, psArgs string) (*api.ContainerTopResponse, error) {
	cid, ok := s.Store.ResolveContainerID(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	c, ok := s.Store.Containers.Get(cid)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	if c.AgentAddress != "" {
		return s.BaseServer.ContainerTop(id, psArgs)
	}

	// BaseServer returns a reasonable synthetic response for containers without agents.
	return s.BaseServer.ContainerTop(id, psArgs)
}

// ContainerGetArchive gets an archive of a path in a container's filesystem.
// Cloud Run Jobs do not support local filesystem access — an agent must be connected.
func (s *Server) ContainerGetArchive(id string, path string) (*api.ContainerArchiveResponse, error) {
	cid, ok := s.Store.ResolveContainerID(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	c, ok := s.Store.Containers.Get(cid)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	if c.AgentAddress != "" {
		return s.BaseServer.ContainerGetArchive(id, path)
	}

	return nil, &api.NotImplementedError{
		Message: "archive get requires an agent connection; Cloud Run Jobs do not support local filesystem access",
	}
}

// ContainerPutArchive extracts an archive to a path in a container's filesystem.
// Cloud Run Jobs do not support local filesystem access — an agent must be connected.
func (s *Server) ContainerPutArchive(id string, path string, noOverwriteDirNonDir bool, body io.Reader) error {
	cid, ok := s.Store.ResolveContainerID(id)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: id}
	}

	c, ok := s.Store.Containers.Get(cid)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: id}
	}

	if c.AgentAddress != "" {
		return s.BaseServer.ContainerPutArchive(id, path, noOverwriteDirNonDir, body)
	}

	return &api.NotImplementedError{
		Message: "archive put requires an agent connection; Cloud Run Jobs do not support local filesystem access",
	}
}

// ContainerStatPath stats a path in a container's filesystem.
// Cloud Run Jobs do not support local filesystem access — an agent must be connected.
func (s *Server) ContainerStatPath(id string, path string) (*api.ContainerPathStat, error) {
	cid, ok := s.Store.ResolveContainerID(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	c, ok := s.Store.Containers.Get(cid)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}

	if c.AgentAddress != "" {
		return s.BaseServer.ContainerStatPath(id, path)
	}

	return nil, &api.NotImplementedError{
		Message: "stat requires an agent connection; Cloud Run Jobs do not support local filesystem access",
	}
}

// ContainerUpdate updates container resource constraints.
// Cloud Run does not support live resource updates — changes are stored in-memory
// and take effect on the next container restart.
func (s *Server) ContainerUpdate(id string, req *api.ContainerUpdateRequest) (*api.ContainerUpdateResponse, error) {
	s.Logger.Warn().Str("container", id).Msg("ContainerUpdate: resource changes are stored in-memory only and take effect on restart")
	return s.BaseServer.ContainerUpdate(id, req)
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
		for _, c := range s.Store.Containers.List() {
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
			// BUG-485: Sum volume dir sizes for SpaceReclaimed
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
