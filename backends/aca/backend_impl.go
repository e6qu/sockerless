package aca

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// Compile-time check that Server implements api.Backend.
var _ api.Backend = (*Server)(nil)

// ContainerCreate creates a container backed by an ACA Job.
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
		config.Env = mergeEnvByKey(img.Config.Env, config.Env)
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
		Driver:   "aca-jobs",
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

	s.ACA.Put(id, ACAState{
		ResourceGroup: s.config.ResourceGroup,
		AgentToken:    agentToken,
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

// ContainerStart starts an ACA Job for the container.
func (s *Server) ContainerStart(ref string) error {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	if c.State.Running {
		return &api.NotModifiedError{}
	}

	acaState, _ := s.ACA.Get(id)

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

	// Build ACA Job spec
	jobName := buildJobName(id)
	jobSpec := s.buildJobSpec([]containerInput{
		{ID: id, Container: &c, AgentToken: acaState.AgentToken, IsMain: true},
	})

	// Create the ACA Job
	createPoller, err := s.azure.Jobs.BeginCreateOrUpdate(s.ctx(), s.config.ResourceGroup, jobName, jobSpec, nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("failed to create ACA Job")
		s.AgentRegistry.Remove(id)
		s.Store.RevertToCreated(id)
		return mapAzureError(err, "job", id)
	}

	// Wait for job creation to complete
	_, err = createPoller.PollUntilDone(s.ctx(), nil)
	if err != nil {
		s.deleteJob(jobName)
		s.AgentRegistry.Remove(id)
		s.Store.RevertToCreated(id)
		s.Logger.Error().Err(err).Str("job", jobName).Msg("job creation failed")
		return mapAzureError(err, "job", id)
	}

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "aca",
		ResourceType: "job",
		ResourceID:   jobName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": c.Image, "name": c.Name, "jobName": jobName},
	})

	// Start the job (creates an execution)
	startPoller, err := s.azure.Jobs.BeginStart(s.ctx(), s.config.ResourceGroup, jobName, nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("failed to start ACA Job")
		s.deleteJob(jobName)
		s.AgentRegistry.Remove(id)
		s.Store.RevertToCreated(id)
		return mapAzureError(err, "execution", id)
	}

	// Wait for start to return execution info
	startResp, err := startPoller.PollUntilDone(s.ctx(), nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("start job failed")
		s.deleteJob(jobName)
		s.AgentRegistry.Remove(id)
		s.Store.RevertToCreated(id)
		return mapAzureError(err, "execution", id)
	}

	executionName := ""
	if startResp.Name != nil {
		executionName = *startResp.Name
	}

	s.ACA.Update(id, func(state *ACAState) {
		state.JobName = jobName
		state.ExecutionName = executionName
	})

	if s.config.CallbackURL != "" {
		// Reverse agent mode: wait for agent callback instead of polling execution IP
		go func() {
			// Wait for execution to complete
			s.waitForExecutionComplete(jobName, executionName, exitCh)

			// Wait for reverse agent to disconnect before stopping
			_ = s.AgentRegistry.WaitForDisconnect(id, 30*time.Minute)

			if _, ok := s.Store.Containers.Get(id); ok {
				s.Store.StopContainer(id, 0)
			}
		}()

		// Wait for reverse agent callback
		agentTimeout := s.config.AgentTimeout
		if err := s.AgentRegistry.WaitForAgent(id, agentTimeout); err != nil {
			s.Logger.Warn().Err(err).Msg("agent callback timeout, exec will use synthetic fallback")
			s.AgentRegistry.Remove(id)
		} else {
			s.Store.Containers.Update(id, func(c *api.Container) {
				c.AgentAddress = "reverse"
				c.AgentToken = acaState.AgentToken
			})
		}
	} else {
		// Forward agent mode: poll for execution RUNNING and health check
		isLongRunning := core.IsTailDevNull(c.Config.Entrypoint, c.Config.Cmd)

		if isLongRunning {
			// Long-running container: wait for RUNNING and check agent health
			agentAddr, completedExitCode, err := s.waitForExecutionRunning(s.ctx(), jobName, executionName)
			if err != nil {
				s.Logger.Error().Err(err).Str("execution", executionName).Msg("execution failed to reach RUNNING state")
				s.AgentRegistry.Remove(id)
				s.deleteJob(jobName)
				s.Store.RevertToCreated(id)
				return mapAzureError(err, "execution", id)
			}

			if completedExitCode >= 0 {
				// Execution completed before agent could be reached.
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
						c.AgentToken = acaState.AgentToken
					})
				}
				s.ACA.Update(id, func(state *ACAState) {
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
						c.AgentToken = acaState.AgentToken
					})
				}

				s.ACA.Update(id, func(state *ACAState) {
					state.AgentAddress = agentAddr
				})
			}
		}

		// Start background poller to detect execution exit
		go s.pollExecutionExit(id, jobName, executionName, exitCh)
	}

	return nil
}

// startMultiContainerJobTyped creates and runs an ACA Job with all pod containers.
// Called when the last container in a pod is started.
func (s *Server) startMultiContainerJobTyped(triggerID string, podContainers []api.Container, exitCh chan struct{}) error {
	// Build containerInput slice: first container is main (gets agent)
	var inputs []containerInput
	for i, pc := range podContainers {
		state, _ := s.ACA.Get(pc.ID)
		pcCopy := pc
		inputs = append(inputs, containerInput{
			ID:         pc.ID,
			Container:  &pcCopy,
			AgentToken: state.AgentToken,
			IsMain:     i == 0,
		})
	}

	mainID := podContainers[0].ID
	mainState, _ := s.ACA.Get(mainID)

	// Pre-create done channel for reverse agent on main container
	if s.config.CallbackURL != "" {
		s.AgentRegistry.Prepare(mainID)
	}

	// Build and create the combined job
	jobName := buildJobName(mainID)
	jobSpec := s.buildJobSpec(inputs)

	createPoller, err := s.azure.Jobs.BeginCreateOrUpdate(s.ctx(), s.config.ResourceGroup, jobName, jobSpec, nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("failed to create multi-container ACA Job")
		if s.config.CallbackURL != "" {
			s.AgentRegistry.Remove(mainID)
		}
		for _, pc := range podContainers {
			s.Store.RevertToCreated(pc.ID)
		}
		return mapAzureError(err, "job", mainID)
	}

	_, err = createPoller.PollUntilDone(s.ctx(), nil)
	if err != nil {
		s.deleteJob(jobName)
		if s.config.CallbackURL != "" {
			s.AgentRegistry.Remove(mainID)
		}
		for _, pc := range podContainers {
			s.Store.RevertToCreated(pc.ID)
		}
		s.Logger.Error().Err(err).Str("job", jobName).Msg("job creation failed")
		return mapAzureError(err, "job", mainID)
	}

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  mainID,
		Backend:      "aca",
		ResourceType: "job",
		ResourceID:   jobName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": podContainers[0].Image, "name": podContainers[0].Name, "jobName": jobName},
	})

	startPoller, err := s.azure.Jobs.BeginStart(s.ctx(), s.config.ResourceGroup, jobName, nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("failed to start ACA Job")
		s.deleteJob(jobName)
		if s.config.CallbackURL != "" {
			s.AgentRegistry.Remove(mainID)
		}
		for _, pc := range podContainers {
			s.Store.RevertToCreated(pc.ID)
		}
		return mapAzureError(err, "execution", mainID)
	}

	startResp, err := startPoller.PollUntilDone(s.ctx(), nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("start job failed")
		s.deleteJob(jobName)
		if s.config.CallbackURL != "" {
			s.AgentRegistry.Remove(mainID)
		}
		for _, pc := range podContainers {
			s.Store.RevertToCreated(pc.ID)
		}
		return mapAzureError(err, "execution", mainID)
	}

	executionName := ""
	if startResp.Name != nil {
		executionName = *startResp.Name
	}

	// Store cloud state on ALL pod containers
	for _, pc := range podContainers {
		s.ACA.Update(pc.ID, func(state *ACAState) {
			state.JobName = jobName
			state.ExecutionName = executionName
		})
	}

	if s.config.CallbackURL != "" {
		// Reverse agent mode
		go func() {
			s.waitForExecutionComplete(jobName, executionName, exitCh)
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
		agentAddr, completedExitCode, err := s.waitForExecutionRunning(s.ctx(), jobName, executionName)
		if err != nil {
			s.Logger.Error().Err(err).Str("execution", executionName).Msg("execution failed to reach RUNNING state")
			s.deleteJob(jobName)
			if s.config.CallbackURL != "" {
				s.AgentRegistry.Remove(mainID)
			}
			for _, pc := range podContainers {
				s.Store.RevertToCreated(pc.ID)
			}
			return mapAzureError(err, "execution", mainID)
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

			s.ACA.Update(mainID, func(state *ACAState) {
				state.AgentAddress = agentAddr
			})

			go s.pollExecutionExit(mainID, jobName, executionName, exitCh)
		}
	}

	return nil
}

// ContainerStop stops a running ACA container.
func (s *Server) ContainerStop(ref string, timeout *int) error {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	// Stop the ACA Job execution
	acaState, _ := s.ACA.Get(id)
	if acaState.JobName != "" && acaState.ExecutionName != "" {
		s.stopExecution(acaState.JobName, acaState.ExecutionName)
	}

	s.StopHealthCheck(id)
	s.AgentRegistry.Remove(id)
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

	exitCode := signalToExitCode(signal)

	// Stop the ACA Job execution
	acaState, _ := s.ACA.Get(id)
	if acaState.JobName != "" && acaState.ExecutionName != "" {
		s.stopExecution(acaState.JobName, acaState.ExecutionName)
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

	if c.State.Running {
		s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		acaState, _ := s.ACA.Get(id)
		if acaState.JobName != "" && acaState.ExecutionName != "" {
			s.stopExecution(acaState.JobName, acaState.ExecutionName)
		}
		s.Store.ForceStopContainer(id, 0)
	}

	s.StopHealthCheck(id)

	// Delete ACA Job (best-effort)
	acaState, _ := s.ACA.Get(id)
	if acaState.JobName != "" {
		s.deleteJob(acaState.JobName)
		s.Registry.MarkCleanedUp(acaState.JobName)
	}

	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod {
		s.Store.Pods.RemoveContainer(pod.ID, id)
	}

	// Clean up network associations
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID != "" {
			_ = s.Drivers.Network.Disconnect(context.Background(), ep.NetworkID, id)
		}
	}

	s.Store.Containers.Delete(id)
	s.Store.ContainerNames.Delete(c.Name)
	s.ACA.Delete(id)
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

// ContainerLogs streams container logs from Azure Monitor Log Analytics.
func (s *Server) ContainerLogs(ref string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	if c.State.Status == "created" {
		return nil, &api.InvalidParameterError{
			Message: "can not get logs from container which is dead or marked for removal",
		}
	}

	params := core.CloudLogParamsFromOpts(opts, c.Config.Labels)

	acaState, _ := s.ACA.Get(id)
	jobName := acaState.JobName
	if jobName == "" {
		jobName = buildJobName(id)
	}

	// Early return if stdout suppressed
	if !params.ShouldWrite() {
		return io.NopCloser(strings.NewReader("")), nil
	}

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		// Track latest timestamp to avoid duplicate entries
		var lastTimestamp time.Time

		// Fetch initial logs with since/until filtering
		s.fetchAndWriteLogsPipe(pw, jobName, lastTimestamp, params, &lastTimestamp, true)

		if !params.Follow {
			return
		}

		// Follow mode: poll for new events (1s interval)
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c, _ := s.Store.Containers.Get(id)
				if !c.State.Running && c.State.Status != "created" {
					s.fetchAndWriteLogsPipe(pw, jobName, lastTimestamp, params, &lastTimestamp, false)
					return
				}
				s.fetchAndWriteLogsPipe(pw, jobName, lastTimestamp, params, &lastTimestamp, false)
			}
		}
	}()

	return pr, nil
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
		acaState, _ := s.ACA.Get(id)
		if acaState.JobName != "" && acaState.ExecutionName != "" {
			s.stopExecution(acaState.JobName, acaState.ExecutionName)
		}
		s.Store.ForceStopContainer(id, 0)
		if acaState.JobName != "" {
			s.deleteJob(acaState.JobName)
			s.Registry.MarkCleanedUp(acaState.JobName)
		}
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
		// BUG-480: Sum image sizes for SpaceReclaimed
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			spaceReclaimed += uint64(img.Size)
		}
		// Clean up ACA resources
		acaState, _ := s.ACA.Get(c.ID)
		if acaState.JobName != "" {
			s.deleteJob(acaState.JobName)
			s.Registry.MarkCleanedUp(acaState.JobName)
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
		s.ACA.Delete(c.ID)
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

// ContainerPause is not supported by ACA backend.
func (s *Server) ContainerPause(ref string) error {
	if _, ok := s.Store.ResolveContainerID(ref); !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "container pause is not supported by ACA backend"}
}

// ContainerUnpause is not supported by ACA backend.
func (s *Server) ContainerUnpause(ref string) error {
	if _, ok := s.Store.ResolveContainerID(ref); !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "container unpause is not supported by ACA backend"}
}

// ImagePull pulls an image reference and stores it locally.
func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
	if ref == "" {
		return nil, &api.InvalidParameterError{Message: "image reference is required"}
	}

	if !strings.Contains(ref, ":") && !strings.Contains(ref, "@") {
		ref += ":latest"
	}

	imgConfig, err := s.fetchImageConfig(ref, auth)
	if err != nil {
		s.Logger.Warn().Err(err).Str("ref", ref).Msg("failed to fetch image config from registry, using synthetic")
	}

	hash := sha256.Sum256([]byte(ref))
	imageID := fmt.Sprintf("sha256:%x", hash)

	image := api.Image{
		ID:           imageID,
		RepoTags:     []string{ref},
		RepoDigests:  []string{},
		Created:      time.Now().UTC().Format(time.RFC3339Nano),
		Size:         0,
		VirtualSize:  0,
		Architecture: "amd64",
		Os:           "linux",
		RootFS:       api.RootFS{Type: "layers"},
	}

	if imgConfig != nil {
		image.Config = *imgConfig
	} else {
		image.Config = api.ContainerConfig{
			Image: ref,
		}
	}

	core.StoreImageWithAliases(s.Store, ref, image)

	// Stream progress via pipe
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		progress := []map[string]string{
			{"status": "Pulling from " + ref},
			{"status": "Digest: " + imageID[:19]},
			{"status": "Status: Downloaded newer image for " + ref},
		}
		for _, p := range progress {
			if err := json.NewEncoder(pw).Encode(p); err != nil {
				return
			}
		}
	}()

	return pr, nil
}

// ImageLoad is not supported by ACA backend.
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	return nil, &api.NotImplementedError{Message: "image load is not supported by ACA backend"}
}

// VolumeRemove removes a volume and its state.
func (s *Server) VolumeRemove(name string, force bool) error {
	if !s.Store.Volumes.Delete(name) {
		return &api.NotFoundError{Resource: "volume", ID: name}
	}
	s.VolumeState.Delete(name)
	return nil
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
			// BUG-486: Sum volume dir sizes for SpaceReclaimed
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
