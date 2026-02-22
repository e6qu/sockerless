package ecs

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
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
		Driver:   "ecs-fargate",
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

	// Register ECS task definition (don't run yet — that's in Start)
	taskDefARN, err := s.registerTaskDefinition(s.ctx(), id, &container, agentToken)
	if err != nil {
		s.Logger.Error().Err(err).Msg("failed to register task definition")
		core.WriteError(w, fmt.Errorf("failed to register task definition: %w", err))
		return
	}

	s.Store.Containers.Put(id, container)
	s.Store.ContainerNames.Put(name, id)
	s.ECS.Put(id, ECSState{
		TaskDefARN: taskDefARN,
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

	ecsState, _ := s.ECS.Get(id)

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

	// Run ECS task
	assignPublicIP := ecstypes.AssignPublicIpDisabled
	if s.config.AssignPublicIP {
		assignPublicIP = ecstypes.AssignPublicIpEnabled
	}

	sgs := s.config.SecurityGroups
	runResult, err := s.aws.ECS.RunTask(s.ctx(), &awsecs.RunTaskInput{
		Cluster:        aws.String(s.config.Cluster),
		TaskDefinition: aws.String(ecsState.TaskDefARN),
		LaunchType:     ecstypes.LaunchTypeFargate,
		Count:          aws.Int32(1),
		NetworkConfiguration: &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				Subnets:        s.config.Subnets,
				SecurityGroups: sgs,
				AssignPublicIp: assignPublicIP,
			},
		},
	})
	if err != nil {
		core.WriteError(w, fmt.Errorf("failed to run task: %w", err))
		return
	}

	if len(runResult.Tasks) == 0 {
		msg := "no tasks launched"
		if len(runResult.Failures) > 0 {
			msg = aws.ToString(runResult.Failures[0].Reason)
		}
		core.WriteError(w, fmt.Errorf("failed to launch task: %s", msg))
		return
	}

	taskARN := aws.ToString(runResult.Tasks[0].TaskArn)
	clusterARN := aws.ToString(runResult.Tasks[0].ClusterArn)

	s.ECS.Update(id, func(state *ECSState) {
		state.TaskARN = taskARN
		state.ClusterARN = clusterARN
	})

	if s.config.CallbackURL != "" {
		// Reverse agent mode: wait for agent callback instead of polling task IP
		go func() {
			// Wait for task to complete
			s.waitForTaskStopped(taskARN, exitCh)

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
				c.AgentToken = ecsState.AgentToken
			})
		}
	} else {
		// Forward agent mode: poll for task RUNNING and health check
		agentAddr, err := s.waitForTaskRunning(s.ctx(), taskARN)
		if err != nil {
			s.Logger.Error().Err(err).Str("task", taskARN).Msg("task failed to reach RUNNING state")
			core.WriteError(w, fmt.Errorf("task failed to start: %w", err))
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
				c.AgentToken = ecsState.AgentToken
			})
		}

		s.ECS.Update(id, func(state *ECSState) {
			state.AgentAddress = agentAddr
		})

		// Start background poller to detect task exit
		go s.pollTaskExit(id, taskARN, exitCh)
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

	// Stop the ECS task (best-effort)
	ecsState, _ := s.ECS.Get(id)
	if ecsState.TaskARN != "" {
		_, _ = s.aws.ECS.StopTask(s.ctx(), &awsecs.StopTaskInput{
			Cluster: aws.String(s.config.Cluster),
			Task:    aws.String(ecsState.TaskARN),
			Reason:  aws.String("Container stopped via API"),
		})
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

	// Stop the ECS task (best-effort)
	ecsState, _ := s.ECS.Get(id)
	if ecsState.TaskARN != "" {
		_, _ = s.aws.ECS.StopTask(s.ctx(), &awsecs.StopTaskInput{
			Cluster: aws.String(s.config.Cluster),
			Task:    aws.String(ecsState.TaskARN),
			Reason:  aws.String("Container killed with " + signal),
		})
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
		ecsState, _ := s.ECS.Get(id)
		if ecsState.TaskARN != "" {
			_, _ = s.aws.ECS.StopTask(s.ctx(), &awsecs.StopTaskInput{
				Cluster: aws.String(s.config.Cluster),
				Task:    aws.String(ecsState.TaskARN),
				Reason:  aws.String("Container removed"),
			})
		}
		s.Store.StopContainer(id, 0)
	}

	// Deregister task definition (best-effort)
	ecsState, _ := s.ECS.Get(id)
	if ecsState.TaskDefARN != "" {
		_, _ = s.aws.ECS.DeregisterTaskDefinition(s.ctx(), &awsecs.DeregisterTaskDefinitionInput{
			TaskDefinition: aws.String(ecsState.TaskDefARN),
		})
	}

	s.Store.Containers.Delete(id)
	s.Store.ContainerNames.Delete(c.Name)
	s.ECS.Delete(id)
	s.Store.WaitChs.Delete(id)

	w.WriteHeader(http.StatusNoContent)
}

// waitForTaskRunning polls ECS until the task reaches RUNNING state.
// Returns the agent address (ip:9111).
func (s *Server) waitForTaskRunning(ctx context.Context, taskARN string) (string, error) {
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
			return "", fmt.Errorf("timeout waiting for task to reach RUNNING state")
		case <-ticker.C:
			result, err := s.aws.ECS.DescribeTasks(ctx, &awsecs.DescribeTasksInput{
				Cluster: aws.String(s.config.Cluster),
				Tasks:   []string{taskARN},
			})
			if err != nil {
				continue
			}
			if len(result.Tasks) == 0 {
				continue
			}

			task := result.Tasks[0]
			status := aws.ToString(task.LastStatus)

			switch status {
			case "RUNNING":
				ip := extractENIIP(task)
				if ip == "" {
					continue
				}
				return ip + ":9111", nil
			case "STOPPED":
				reason := aws.ToString(task.StoppedReason)
				return "", fmt.Errorf("task stopped: %s", reason)
			}
		}
	}
}

// waitForAgentHealth polls the agent's /health endpoint.
func (s *Server) waitForAgentHealth(ctx context.Context, healthURL string) error {
	agentTimeout := 60 * time.Second
	if s.config.EndpointURL != "" {
		agentTimeout = 2 * time.Second // simulator mode: no real agent
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

// waitForTaskStopped blocks until the ECS task reaches STOPPED state or exitCh is closed.
// Used in reverse agent mode where the goroutine needs to wait for the cloud task to finish.
func (s *Server) waitForTaskStopped(taskARN string, exitCh chan struct{}) {
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
			result, err := s.aws.ECS.DescribeTasks(s.ctx(), &awsecs.DescribeTasksInput{
				Cluster: aws.String(s.config.Cluster),
				Tasks:   []string{taskARN},
			})
			if err != nil {
				continue
			}
			if len(result.Tasks) == 0 {
				continue
			}
			if aws.ToString(result.Tasks[0].LastStatus) == "STOPPED" {
				return
			}
		}
	}
}

// pollTaskExit monitors an ECS task and updates container state when it stops.
func (s *Server) pollTaskExit(containerID, taskARN string, exitCh chan struct{}) {
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
			result, err := s.aws.ECS.DescribeTasks(s.ctx(), &awsecs.DescribeTasksInput{
				Cluster: aws.String(s.config.Cluster),
				Tasks:   []string{taskARN},
			})
			if err != nil {
				continue
			}
			if len(result.Tasks) == 0 {
				continue
			}

			task := result.Tasks[0]
			if aws.ToString(task.LastStatus) == "STOPPED" {
				exitCode := 0
				for _, container := range task.Containers {
					if container.ExitCode != nil {
						exitCode = int(aws.ToInt32(container.ExitCode))
						break
					}
				}
				s.Store.StopContainer(containerID, exitCode)
				return
			}
		}
	}
}
