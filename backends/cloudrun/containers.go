package cloudrun

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	runpb "cloud.google.com/go/run/apiv2/runpb"
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
	s.CloudRun.Put(id, CloudRunState{
		AgentToken: agentToken,
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

	// Clean up any existing Cloud Run Job from a previous start
	if crState.JobName != "" {
		s.deleteJob(crState.JobName)
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
		core.WriteError(w, fmt.Errorf("failed to create job: %w", err))
		return
	}

	// Wait for job creation to complete
	job, err := createOp.Wait(s.ctx())
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("job creation failed")
		core.WriteError(w, fmt.Errorf("job creation failed: %w", err))
		return
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
		core.WriteError(w, fmt.Errorf("failed to run job: %w", err))
		return
	}

	// Wait for RunJob LRO to return the execution
	execution, err := runOp.Wait(s.ctx())
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobFullName).Msg("run job failed")
		s.deleteJob(jobFullName)
		core.WriteError(w, fmt.Errorf("run job failed: %w", err))
		return
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
				c.AgentToken = crState.AgentToken
			})
		}
	} else {
		// Forward agent mode: poll for execution RUNNING and health check
		agentAddr, err := s.waitForExecutionRunning(s.ctx(), executionName)
		if err != nil {
			s.Logger.Error().Err(err).Str("execution", executionName).Msg("execution failed to reach RUNNING state")
			s.deleteJob(jobFullName)
			core.WriteError(w, fmt.Errorf("execution failed to start: %w", err))
			return
		}

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
		}

		s.CloudRun.Update(id, func(state *CloudRunState) {
			state.AgentAddress = agentAddr
		})

		// Start background poller to detect execution exit
		go s.pollExecutionExit(id, executionName, exitCh)
	}

	w.WriteHeader(http.StatusNoContent)
}

// startMultiContainerJob creates and runs a Cloud Run Job with all pod containers.
// Called when the last container in a pod is started.
func (s *Server) startMultiContainerJob(w http.ResponseWriter, triggerID string, podContainers []api.Container, exitCh chan struct{}) {
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
		core.WriteError(w, fmt.Errorf("failed to create job: %w", err))
		return
	}

	job, err := createOp.Wait(s.ctx())
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobName).Msg("job creation failed")
		core.WriteError(w, fmt.Errorf("job creation failed: %w", err))
		return
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
		core.WriteError(w, fmt.Errorf("failed to run job: %w", err))
		return
	}

	execution, err := runOp.Wait(s.ctx())
	if err != nil {
		s.Logger.Error().Err(err).Str("job", jobFullName).Msg("run job failed")
		s.deleteJob(jobFullName)
		core.WriteError(w, fmt.Errorf("run job failed: %w", err))
		return
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
		agentAddr, err := s.waitForExecutionRunning(s.ctx(), executionName)
		if err != nil {
			s.Logger.Error().Err(err).Str("execution", executionName).Msg("execution failed to reach RUNNING state")
			s.deleteJob(jobFullName)
			core.WriteError(w, fmt.Errorf("execution failed to start: %w", err))
			return
		}

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

	// Cancel the Cloud Run execution
	crState, _ := s.CloudRun.Get(id)
	if crState.ExecutionName != "" {
		s.cancelExecution(crState.ExecutionName)
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
		crState, _ := s.CloudRun.Get(id)
		if crState.ExecutionName != "" {
			s.cancelExecution(crState.ExecutionName)
		}
		s.Store.StopContainer(id, 0)
	}

	// Delete Cloud Run Job (best-effort)
	crState, _ := s.CloudRun.Get(id)
	if crState.JobName != "" {
		s.deleteJob(crState.JobName)
		s.Registry.MarkCleanedUp(crState.JobName)
	}

	s.Store.Containers.Delete(id)
	s.Store.ContainerNames.Delete(c.Name)
	s.CloudRun.Delete(id)
	s.Store.WaitChs.Delete(id)

	w.WriteHeader(http.StatusNoContent)
}

// waitForExecutionRunning polls a Cloud Run execution until it reaches RUNNING state.
func (s *Server) waitForExecutionRunning(ctx context.Context, executionName string) (string, error) {
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
			return "", ctx.Err()
		case <-timeout:
			return "", fmt.Errorf("timeout waiting for execution to reach RUNNING state")
		case <-ticker.C:
			exec, err := s.gcp.Executions.GetExecution(ctx, &runpb.GetExecutionRequest{
				Name: executionName,
			})
			if err != nil {
				s.Logger.Debug().Err(err).Msg("polling execution status")
				continue
			}

			if exec.RunningCount > 0 {
				agentAddr := fmt.Sprintf("%s:9111", executionName)
				return agentAddr, nil
			}

			if exec.FailedCount > 0 || exec.CancelledCount > 0 {
				return "", fmt.Errorf("execution failed or was cancelled")
			}

			if exec.SucceededCount > 0 {
				return "", fmt.Errorf("execution completed before agent could be reached")
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

// waitForExecutionComplete blocks until the Cloud Run execution completes or exitCh is closed.
// Used in reverse agent mode where the goroutine needs to wait for the cloud job to finish.
func (s *Server) waitForExecutionComplete(executionName string, exitCh chan struct{}) {
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
			exec, err := s.gcp.Executions.GetExecution(s.ctx(), &runpb.GetExecutionRequest{
				Name: executionName,
			})
			if err != nil {
				continue
			}
			if exec.CompletionTime != nil {
				return
			}
		}
	}
}

// pollExecutionExit monitors a Cloud Run execution and updates container state when it completes.
func (s *Server) pollExecutionExit(containerID, executionName string, exitCh chan struct{}) {
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
			exec, err := s.gcp.Executions.GetExecution(s.ctx(), &runpb.GetExecutionRequest{
				Name: executionName,
			})
			if err != nil {
				continue
			}

			if exec.CompletionTime != nil {
				exitCode := 0
				if exec.FailedCount > 0 {
					exitCode = 1
				}
				s.Store.StopContainer(containerID, exitCode)
				return
			}
		}
	}
}

// cancelExecution cancels a Cloud Run execution (best-effort).
func (s *Server) cancelExecution(executionName string) {
	op, err := s.gcp.Executions.CancelExecution(s.ctx(), &runpb.CancelExecutionRequest{
		Name: executionName,
	})
	if err != nil {
		s.Logger.Debug().Err(err).Str("execution", executionName).Msg("failed to cancel execution")
		return
	}
	_ = op
}

// deleteJob deletes a Cloud Run Job (best-effort).
func (s *Server) deleteJob(jobName string) {
	op, err := s.gcp.Jobs.DeleteJob(s.ctx(), &runpb.DeleteJobRequest{
		Name: jobName,
	})
	if err != nil {
		s.Logger.Debug().Err(err).Str("job", jobName).Msg("failed to delete job")
		return
	}
	_ = op
}
