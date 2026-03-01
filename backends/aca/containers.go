package aca

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appcontainers/armappcontainers/v2"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

func (s *Server) handleContainerCreate(w http.ResponseWriter, r *http.Request) {
	var req api.ContainerCreateRequest
	if err := core.ReadJSON(r, &req); err != nil {
		core.WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		name = "/" + core.GenerateName()
	} else if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}

	if _, exists := s.Store.ContainerNames.Get(name); exists {
		core.WriteError(w, &api.ConflictError{
			Message: fmt.Sprintf("Conflict. The container name \"%s\" is already in use", strings.TrimPrefix(name, "/")),
		})
		return
	}

	id := core.GenerateID()

	config := api.ContainerConfig{}
	if req.ContainerConfig != nil {
		config = *req.ContainerConfig
	}

	// Merge image config if available
	if img, ok := s.Store.ResolveImage(config.Image); ok {
		if len(config.Env) == 0 {
			config.Env = img.Config.Env
		}
		if len(config.Cmd) == 0 {
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
		IPAddress:   fmt.Sprintf("172.17.0.%d", s.Store.Containers.Len()+2),
		IPPrefixLen: 16,
		MacAddress:  "02:42:ac:11:00:02",
	}

	agentToken := s.config.AgentToken
	if agentToken == "" {
		agentToken = core.GenerateToken()
	}

	s.Store.Containers.Put(id, container)
	s.Store.ContainerNames.Put(name, id)
	s.ACA.Put(id, ACAState{
		ResourceGroup: s.config.ResourceGroup,
		AgentToken:    agentToken,
	})

	core.WriteJSON(w, http.StatusCreated, api.ContainerCreateResponse{
		ID:       id,
		Warnings: []string{},
	})
}

func (s *Server) handleContainerStart(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)
	if c.State.Running {
		core.WriteError(w, &api.NotModifiedError{})
		return
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

	// Deferred start: if container is in a multi-container pod, wait for all siblings
	shouldDefer, podContainers := s.PodDeferredStart(id)
	if shouldDefer {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if len(podContainers) > 1 {
		// Multi-container pod: build combined job and run
		s.startMultiContainerJob(w, id, podContainers, exitCh)
		return
	}

	// Helper/cache containers (non-tail-dev-null commands like "chmod -R 777 /cache")
	// don't need the full invoke+agent flow. Auto-stop them after a brief delay.
	if s.config.CallbackURL != "" && !core.IsTailDevNull(c.Config.Entrypoint, c.Config.Cmd) {
		go func() {
			time.Sleep(500 * time.Millisecond)
			s.Store.StopContainer(id, 0)
		}()
		w.WriteHeader(http.StatusNoContent)
		return
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
		core.WriteError(w, fmt.Errorf("failed to create job: %w", err))
		return
	}

	// Wait for job creation to complete
	_, err = createPoller.PollUntilDone(s.ctx(), nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("job creation failed")
		core.WriteError(w, fmt.Errorf("job creation failed: %w", err))
		return
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
		core.WriteError(w, fmt.Errorf("failed to start job: %w", err))
		return
	}

	// Wait for start to return execution info
	startResp, err := startPoller.PollUntilDone(s.ctx(), nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("start job failed")
		s.deleteJob(jobName)
		core.WriteError(w, fmt.Errorf("start job failed: %w", err))
		return
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

			s.Store.StopContainer(id, 0)
		}()

		// Wait for reverse agent callback
		agentTimeout := 60 * time.Second
		if s.config.EndpointURL != "" {
			agentTimeout = 5 * time.Second
		}
		if err := s.AgentRegistry.WaitForAgent(id, agentTimeout); err != nil {
			s.Logger.Warn().Err(err).Msg("agent callback timeout, exec will use synthetic fallback")
		} else {
			s.Store.Containers.Update(id, func(c *api.Container) {
				c.AgentAddress = "reverse"
				c.AgentToken = acaState.AgentToken
			})
		}
	} else {
		// Forward agent mode: poll for execution RUNNING and health check
		agentAddr, completedExitCode, err := s.waitForExecutionRunning(s.ctx(), jobName, executionName)
		if err != nil {
			s.Logger.Error().Err(err).Str("execution", executionName).Msg("execution failed to reach RUNNING state")
			s.deleteJob(jobName)
			core.WriteError(w, fmt.Errorf("execution failed to start: %w", err))
			return
		}

		if completedExitCode >= 0 {
			// Execution completed before agent could be reached (short-lived command).
			// Treat as a fast exit — stop the container with the real exit code.
			go func() {
				s.Store.StopContainer(id, completedExitCode)
			}()
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

			// Start background poller to detect execution exit
			go s.pollExecutionExit(id, jobName, executionName, exitCh)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// startMultiContainerJob creates and runs an ACA Job with all pod containers.
// Called when the last container in a pod is started.
func (s *Server) startMultiContainerJob(w http.ResponseWriter, triggerID string, podContainers []api.Container, exitCh chan struct{}) {
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
		core.WriteError(w, fmt.Errorf("failed to create job: %w", err))
		return
	}

	_, err = createPoller.PollUntilDone(s.ctx(), nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("job creation failed")
		core.WriteError(w, fmt.Errorf("job creation failed: %w", err))
		return
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
		core.WriteError(w, fmt.Errorf("failed to start job: %w", err))
		return
	}

	startResp, err := startPoller.PollUntilDone(s.ctx(), nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("start job failed")
		s.deleteJob(jobName)
		core.WriteError(w, fmt.Errorf("start job failed: %w", err))
		return
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
			s.Store.StopContainer(mainID, 0)
		}()

		agentTimeout := 60 * time.Second
		if s.config.EndpointURL != "" {
			agentTimeout = 5 * time.Second
		}
		if err := s.AgentRegistry.WaitForAgent(mainID, agentTimeout); err != nil {
			s.Logger.Warn().Err(err).Msg("agent callback timeout, exec will use synthetic fallback")
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
			core.WriteError(w, fmt.Errorf("execution failed to start: %w", err))
			return
		}

		if completedExitCode >= 0 {
			go func() {
				s.Store.StopContainer(mainID, completedExitCode)
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

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerStop(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		core.WriteError(w, &api.NotModifiedError{})
		return
	}

	// Stop the ACA Job execution
	acaState, _ := s.ACA.Get(id)
	if acaState.JobName != "" && acaState.ExecutionName != "" {
		s.stopExecution(acaState.JobName, acaState.ExecutionName)
	}

	s.Store.StopContainer(id, 0)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerKill(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		core.WriteError(w, &api.ConflictError{
			Message: fmt.Sprintf("Container %s is not running", ref),
		})
		return
	}

	// Disconnect reverse agent if connected (unblocks invoke goroutine)
	s.AgentRegistry.Remove(id)

	signal := r.URL.Query().Get("signal")
	exitCode := 0
	if signal == "SIGKILL" || signal == "9" || signal == "KILL" {
		exitCode = 137
	}

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

	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerRemove(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		core.WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	c, _ := s.Store.Containers.Get(id)

	if c.State.Running && !force {
		core.WriteError(w, &api.ConflictError{
			Message: fmt.Sprintf("You cannot remove a running container %s. Stop the container before attempting removal or force remove", id[:12]),
		})
		return
	}

	// Disconnect reverse agent if connected (unblocks invoke goroutine)
	s.AgentRegistry.Remove(id)

	if c.State.Running {
		acaState, _ := s.ACA.Get(id)
		if acaState.JobName != "" && acaState.ExecutionName != "" {
			s.stopExecution(acaState.JobName, acaState.ExecutionName)
		}
		s.Store.StopContainer(id, 0)
	}

	// Delete ACA Job (best-effort)
	acaState, _ := s.ACA.Get(id)
	if acaState.JobName != "" {
		s.deleteJob(acaState.JobName)
		s.Registry.MarkCleanedUp(acaState.JobName)
	}

	s.Store.Containers.Delete(id)
	s.Store.ContainerNames.Delete(c.Name)
	s.ACA.Delete(id)
	s.Store.WaitChs.Delete(id)

	w.WriteHeader(http.StatusNoContent)
}

// waitForExecutionRunning polls until the execution reaches RUNNING state.
// Returns (agentAddr, -1, nil) if the execution is running.
// Returns ("", exitCode, nil) if the execution completed before the agent was reachable.
// Returns ("", -1, err) on failure.
func (s *Server) waitForExecutionRunning(ctx context.Context, jobName, executionName string) (string, int, error) {
	timeout := time.After(5 * time.Minute)
	pollInterval := 2 * time.Second
	if s.config.EndpointURL != "" {
		pollInterval = 500 * time.Millisecond
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", -1, ctx.Err()
		case <-timeout:
			return "", -1, fmt.Errorf("timeout waiting for execution to reach RUNNING state")
		case <-ticker.C:
			pager := s.azure.Executions.NewListPager(s.config.ResourceGroup, jobName, nil)
			for pager.More() {
				page, err := pager.NextPage(ctx)
				if err != nil {
					s.Logger.Debug().Err(err).Msg("polling execution status")
					break
				}
				for _, exec := range page.Value {
					if executionName != "" && (exec.Name == nil || *exec.Name != executionName) {
						continue
					}
					if exec.Status == nil {
						continue
					}
					name := executionName
					if name == "" && exec.Name != nil {
						name = *exec.Name
					}
					switch *exec.Status {
					case armappcontainers.JobExecutionRunningStateRunning:
						agentAddr := fmt.Sprintf("%s:9111", name)
						return agentAddr, -1, nil
					case armappcontainers.JobExecutionRunningStateFailed,
						armappcontainers.JobExecutionRunningStateDegraded:
						// Execution failed — treat as fast exit with code 1
						return "", 1, nil
					case armappcontainers.JobExecutionRunningStateStopped:
						return "", -1, fmt.Errorf("execution stopped")
					case armappcontainers.JobExecutionRunningStateSucceeded:
						// Execution completed before agent was reachable — fast exit with code 0
						return "", 0, nil
					}
				}
			}
		}
	}
}

// waitForAgentHealth polls the agent's /health endpoint.
func (s *Server) waitForAgentHealth(ctx context.Context, healthURL string) error {
	agentTimeout := 60 * time.Second
	if s.config.EndpointURL != "" {
		agentTimeout = 2 * time.Second
	}
	timeout := time.After(agentTimeout)
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

// waitForExecutionComplete blocks until the ACA Job execution completes or exitCh is closed.
// Used in reverse agent mode where the goroutine needs to wait for the cloud job to finish.
func (s *Server) waitForExecutionComplete(jobName, executionName string, exitCh chan struct{}) {
	pollInterval := 5 * time.Second
	if s.config.EndpointURL != "" {
		pollInterval = 1 * time.Second
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-exitCh:
			return
		case <-ticker.C:
			pager := s.azure.Executions.NewListPager(s.config.ResourceGroup, jobName, nil)
			for pager.More() {
				page, err := pager.NextPage(s.ctx())
				if err != nil {
					break
				}
				for _, exec := range page.Value {
					if exec.Name == nil || *exec.Name != executionName {
						continue
					}
					if exec.Status == nil {
						continue
					}
					switch *exec.Status {
					case armappcontainers.JobExecutionRunningStateSucceeded,
						armappcontainers.JobExecutionRunningStateFailed,
						armappcontainers.JobExecutionRunningStateDegraded,
						armappcontainers.JobExecutionRunningStateStopped:
						return
					}
				}
			}
		}
	}
}

// pollExecutionExit monitors an ACA Job execution and updates container state when it completes.
func (s *Server) pollExecutionExit(containerID, jobName, executionName string, exitCh chan struct{}) {
	pollInterval := 5 * time.Second
	if s.config.EndpointURL != "" {
		pollInterval = 1 * time.Second
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-exitCh:
			return
		case <-ticker.C:
			pager := s.azure.Executions.NewListPager(s.config.ResourceGroup, jobName, nil)
			for pager.More() {
				page, err := pager.NextPage(s.ctx())
				if err != nil {
					break
				}
				for _, exec := range page.Value {
					if exec.Name == nil || *exec.Name != executionName {
						continue
					}
					if exec.Status == nil {
						continue
					}
					switch *exec.Status {
					case armappcontainers.JobExecutionRunningStateSucceeded:
						s.Store.StopContainer(containerID, 0)
						return
					case armappcontainers.JobExecutionRunningStateFailed,
						armappcontainers.JobExecutionRunningStateDegraded:
						s.Store.StopContainer(containerID, 1)
						return
					case armappcontainers.JobExecutionRunningStateStopped:
						s.Store.StopContainer(containerID, 137)
						return
					}
				}
			}
		}
	}
}

// stopExecution stops an ACA Job execution (best-effort).
func (s *Server) stopExecution(jobName, executionName string) {
	poller, err := s.azure.Jobs.BeginStopExecution(s.ctx(), s.config.ResourceGroup, jobName, executionName, nil)
	if err != nil {
		s.Logger.Debug().Err(err).Str("execution", executionName).Msg("failed to stop execution")
		return
	}
	_ = poller
}

// deleteJob deletes an ACA Job (best-effort).
func (s *Server) deleteJob(jobName string) {
	poller, err := s.azure.Jobs.BeginDelete(s.ctx(), s.config.ResourceGroup, jobName, nil)
	if err != nil {
		s.Logger.Debug().Err(err).Str("job", jobName).Msg("failed to delete job")
		return
	}
	_ = poller
}
