package ecs

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// Compile-time check that Server implements api.Backend.
var _ api.Backend = (*Server)(nil)

// ContainerCreate creates a container backed by an ECS task definition.
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
		IPAddress:   fmt.Sprintf("172.17.0.%d", int(s.ipCounter.Add(1))),
		IPPrefixLen: 16,
		MacAddress:  "02:42:ac:11:00:02",
	}

	agentToken := s.config.AgentToken
	if agentToken == "" {
		agentToken = core.GenerateToken()
	}

	s.Store.Containers.Put(id, container)
	s.Store.ContainerNames.Put(name, id)

	// Store ECS state without task definition — defer registration to ContainerStart.
	s.ECS.Put(id, ECSState{
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

// ContainerStart starts an ECS task for the container.
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

	ecsState, _ := s.ECS.Get(id)

	// Deferred task definition registration: if not yet registered, do it now
	if ecsState.TaskDefARN == "" {
		taskDefARN, err := s.registerTaskDefinition(s.ctx(), []containerInput{
			{ID: id, Container: &c, AgentToken: ecsState.AgentToken, IsMain: true},
		})
		if err != nil {
			s.Logger.Error().Err(err).Msg("failed to register task definition")
			return mapAWSError(err, "task-definition", id)
		}
		s.ECS.Update(id, func(state *ECSState) {
			state.TaskDefARN = taskDefARN
		})
		ecsState.TaskDefARN = taskDefARN
	}

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
		// Multi-container pod: register combined task definition and run a single task
		return s.startMultiContainerTaskTyped(id, podContainers, exitCh)
	}

	// Pre-create done channel so invoke goroutine can wait for agent disconnect
	if s.config.CallbackURL != "" {
		s.AgentRegistry.Prepare(id)
	}

	// Run ECS task
	taskDefARN := ecsState.TaskDefARN
	taskARN, clusterARN, err := s.runECSTask(id, taskDefARN, &c)
	if err != nil {
		// Best-effort cleanup of orphaned task definition
		_, _ = s.aws.ECS.DeregisterTaskDefinition(s.ctx(), &awsecs.DeregisterTaskDefinitionInput{
			TaskDefinition: aws.String(taskDefARN),
		})
		s.AgentRegistry.Remove(id)
		s.Store.RevertToCreated(id)
		return mapAWSError(err, "task", id)
	}

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
				c.AgentToken = ecsState.AgentToken
			})
		}
	} else {
		// Forward agent mode: poll for task RUNNING and health check
		isLongRunning := core.IsTailDevNull(c.Config.Entrypoint, c.Config.Cmd)

		if isLongRunning {
			// Long-running container: wait for RUNNING and check agent health
			agentAddr, err := s.waitForTaskRunning(s.ctx(), taskARN)
			if err != nil {
				s.Logger.Error().Err(err).Str("task", taskARN).Msg("task failed to reach RUNNING state")
				// Stop the ECS task that was already launched
				_, _ = s.aws.ECS.StopTask(s.ctx(), &awsecs.StopTaskInput{
					Cluster: aws.String(clusterARN),
					Task:    aws.String(taskARN),
					Reason:  aws.String("Task failed to reach RUNNING state"),
				})
				s.Registry.MarkCleanedUp(taskARN)
				s.AgentRegistry.Remove(id)
				s.Store.RevertToCreated(id)
				return mapAWSError(err, "task", id)
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
			} else {
				// Fallback to auto-agent if configured
				if autoErr := s.SpawnAutoAgent(id); autoErr != nil {
					s.Logger.Warn().Err(autoErr).Msg("auto-agent fallback failed")
				}
			}

			s.ECS.Update(id, func(state *ECSState) {
				state.AgentAddress = agentAddr
			})
		} else {
			// Short-lived container without forward agent — try auto-agent
			if autoErr := s.SpawnAutoAgent(id); autoErr != nil {
				s.Logger.Warn().Err(autoErr).Msg("auto-agent fallback failed")
			}
		}

		// Start background poller to detect task exit
		go s.pollTaskExit(id, taskARN, exitCh)
	}

	return nil
}

// startMultiContainerTaskTyped registers a combined task definition for all pod containers
// and runs a single ECS task. Returns error instead of writing to http.ResponseWriter.
func (s *Server) startMultiContainerTaskTyped(triggerID string, podContainers []api.Container, exitCh chan struct{}) error {
	// Build containerInput slice: first container is main (gets agent)
	var inputs []containerInput
	for i, pc := range podContainers {
		state, _ := s.ECS.Get(pc.ID)
		pcCopy := pc
		inputs = append(inputs, containerInput{
			ID:         pc.ID,
			Container:  &pcCopy,
			AgentToken: state.AgentToken,
			IsMain:     i == 0,
		})
	}

	// Register combined task definition
	taskDefARN, err := s.registerTaskDefinition(s.ctx(), inputs)
	if err != nil {
		s.Logger.Error().Err(err).Msg("failed to register multi-container task definition")
		return mapAWSError(err, "task-definition", triggerID)
	}

	// Use the main (first) container for the task
	mainContainer := &podContainers[0]
	mainID := mainContainer.ID
	mainState, _ := s.ECS.Get(mainID)

	// Pre-create done channel for reverse agent on main container
	if s.config.CallbackURL != "" {
		s.AgentRegistry.Prepare(mainID)
	}

	// Run the combined task
	taskARN, clusterARN, err := s.runECSTask(mainID, taskDefARN, mainContainer)
	if err != nil {
		// Best-effort cleanup of orphaned task definition
		_, _ = s.aws.ECS.DeregisterTaskDefinition(s.ctx(), &awsecs.DeregisterTaskDefinitionInput{
			TaskDefinition: aws.String(taskDefARN),
		})
		if s.config.CallbackURL != "" {
			s.AgentRegistry.Remove(mainID)
		}
		for _, pc := range podContainers {
			s.Store.RevertToCreated(pc.ID)
		}
		return mapAWSError(err, "task", mainID)
	}

	// Store cloud state on ALL pod containers (so stop/remove works for any)
	for _, pc := range podContainers {
		s.ECS.Update(pc.ID, func(state *ECSState) {
			state.TaskDefARN = taskDefARN
			state.TaskARN = taskARN
			state.ClusterARN = clusterARN
		})
	}

	if s.config.CallbackURL != "" {
		// Reverse agent mode
		go func() {
			s.waitForTaskStopped(taskARN, exitCh)
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
		agentAddr, err := s.waitForTaskRunning(s.ctx(), taskARN)
		if err != nil {
			s.Logger.Error().Err(err).Str("task", taskARN).Msg("task failed to reach RUNNING state")
			if s.config.CallbackURL != "" {
				s.AgentRegistry.Remove(mainID)
			}
			for _, pc := range podContainers {
				s.Store.RevertToCreated(pc.ID)
			}
			return mapAWSError(err, "task", mainID)
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

		s.ECS.Update(mainID, func(state *ECSState) {
			state.AgentAddress = agentAddr
		})

		go s.pollTaskExit(mainID, taskARN, exitCh)
	}

	return nil
}

// ContainerStop stops a running container by stopping its ECS task.
func (s *Server) ContainerStop(ref string, timeout *int) error {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	// Stop the ECS task (best-effort)
	ecsState, _ := s.ECS.Get(id)
	if ecsState.TaskARN != "" {
		cluster := s.config.Cluster
		if ecsState.ClusterARN != "" {
			cluster = ecsState.ClusterARN
		}
		_, _ = s.aws.ECS.StopTask(s.ctx(), &awsecs.StopTaskInput{
			Cluster: aws.String(cluster),
			Task:    aws.String(ecsState.TaskARN),
			Reason:  aws.String("Container stopped via API"),
		})
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

	exitCode := signalToExitCode(signal)

	// Stop the ECS task (best-effort)
	ecsState, _ := s.ECS.Get(id)
	if ecsState.TaskARN != "" {
		cluster := s.config.Cluster
		if ecsState.ClusterARN != "" {
			cluster = ecsState.ClusterARN
		}
		_, _ = s.aws.ECS.StopTask(s.ctx(), &awsecs.StopTaskInput{
			Cluster: aws.String(cluster),
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

	s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": fmt.Sprintf("%d", exitCode), "name": strings.TrimPrefix(c.Name, "/")})

	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}

	return nil
}

// ContainerRemove removes a container and its associated ECS resources.
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
		ecsState, _ := s.ECS.Get(id)
		if ecsState.TaskARN != "" {
			cluster := s.config.Cluster
			if ecsState.ClusterARN != "" {
				cluster = ecsState.ClusterARN
			}
			_, _ = s.aws.ECS.StopTask(s.ctx(), &awsecs.StopTaskInput{
				Cluster: aws.String(cluster),
				Task:    aws.String(ecsState.TaskARN),
				Reason:  aws.String("Container removed"),
			})
		}
		s.Store.ForceStopContainer(id, 0)
	}

	s.StopHealthCheck(id)

	// Deregister task definition (best-effort)
	ecsState, _ := s.ECS.Get(id)
	if ecsState.TaskDefARN != "" {
		_, _ = s.aws.ECS.DeregisterTaskDefinition(s.ctx(), &awsecs.DeregisterTaskDefinitionInput{
			TaskDefinition: aws.String(ecsState.TaskDefARN),
		})
	}

	if ecsState.TaskARN != "" {
		s.Registry.MarkCleanedUp(ecsState.TaskARN)
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
	s.ECS.Delete(id)
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

// ContainerLogs streams container logs from CloudWatch.
func (s *Server) ContainerLogs(ref string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	// Auto-agent containers have logs captured by the agent, not CloudWatch
	if os.Getenv("SOCKERLESS_AUTO_AGENT_BIN") != "" {
		return s.BaseServer.ContainerLogs(ref, opts)
	}

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

	logStreamPrefix := id[:12]
	logStreamName := fmt.Sprintf("%s/main/%s", logStreamPrefix, s.getTaskID(id))

	// Early return if stdout suppressed (all cloud logs are stdout)
	if !params.ShouldWrite() {
		return io.NopCloser(strings.NewReader("")), nil
	}

	pr, pw := io.Pipe()

	go func() {
		defer func() { _ = pw.Close() }()

		input := &cloudwatchlogs.GetLogEventsInput{
			LogGroupName:  aws.String(s.config.LogGroup),
			LogStreamName: aws.String(logStreamName),
			StartFromHead: aws.Bool(params.CloudLogTailInt32() == nil),
		}
		if limit := params.CloudLogTailInt32(); limit != nil {
			input.Limit = limit
		}
		if ms := params.SinceMillis(); ms != nil {
			input.StartTime = ms
		}
		if ms := params.UntilMillis(); ms != nil {
			input.EndTime = ms
		}

		// Fetch initial logs
		result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), input)
		if err != nil {
			s.Logger.Debug().Err(err).Str("stream", logStreamName).Msg("failed to get log events")
			return
		}

		for _, event := range result.Events {
			line := s.formatLogEvent(event.Message, event.Timestamp, params)
			if line == "" {
				continue
			}
			if _, err := pw.Write([]byte(line)); err != nil {
				return
			}
		}

		nextToken := result.NextForwardToken

		if !params.Follow {
			return
		}

		// Follow mode: poll for new events
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			// Check if container has stopped
			c, _ := s.Store.Containers.Get(id)
			if !c.State.Running && c.State.Status != "created" {
				// Fetch any remaining logs
				followInput := &cloudwatchlogs.GetLogEventsInput{
					LogGroupName:  aws.String(s.config.LogGroup),
					LogStreamName: aws.String(logStreamName),
					StartFromHead: aws.Bool(true),
					NextToken:     nextToken,
				}
				result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), followInput)
				if err == nil {
					for _, event := range result.Events {
						line := s.formatLogEvent(event.Message, event.Timestamp, params)
						if line == "" {
							continue
						}
						if _, err := pw.Write([]byte(line)); err != nil {
							return
						}
					}
				}
				return
			}

			// Follow queries use NextToken only, no since/until
			followInput := &cloudwatchlogs.GetLogEventsInput{
				LogGroupName:  aws.String(s.config.LogGroup),
				LogStreamName: aws.String(logStreamName),
				StartFromHead: aws.Bool(true),
				NextToken:     nextToken,
			}
			result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), followInput)
			if err != nil {
				continue
			}

			for _, event := range result.Events {
				line := s.formatLogEvent(event.Message, event.Timestamp, params)
				if line == "" {
					continue
				}
				if _, err := pw.Write([]byte(line)); err != nil {
					return
				}
			}
			nextToken = result.NextForwardToken
		}
	}()

	return pr, nil
}

// formatLogEvent formats a single CloudWatch log event as a raw text line.
// Returns empty string if message is nil.
func (s *Server) formatLogEvent(message *string, timestamp *int64, params core.CloudLogParams) string {
	if message == nil {
		return ""
	}
	var ts time.Time
	if timestamp != nil {
		ts = time.UnixMilli(*timestamp)
	}
	return params.FormatLine(*message, ts)
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
		ecsState, _ := s.ECS.Get(id)
		if ecsState.TaskARN != "" {
			cluster := s.config.Cluster
			if ecsState.ClusterARN != "" {
				cluster = ecsState.ClusterARN
			}
			_, _ = s.aws.ECS.StopTask(s.ctx(), &awsecs.StopTaskInput{
				Cluster: aws.String(cluster),
				Task:    aws.String(ecsState.TaskARN),
				Reason:  aws.String("Container restarted via API"),
			})
			s.Registry.MarkCleanedUp(ecsState.TaskARN)
		}
		if ecsState.TaskDefARN != "" {
			_, _ = s.aws.ECS.DeregisterTaskDefinition(s.ctx(), &awsecs.DeregisterTaskDefinitionInput{
				TaskDefinition: aws.String(ecsState.TaskDefARN),
			})
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
		// BUG-478: Sum image sizes for SpaceReclaimed
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			spaceReclaimed += uint64(img.Size)
		}
		ecsState, _ := s.ECS.Get(c.ID)
		if ecsState.TaskDefARN != "" {
			_, _ = s.aws.ECS.DeregisterTaskDefinition(s.ctx(), &awsecs.DeregisterTaskDefinitionInput{
				TaskDefinition: aws.String(ecsState.TaskDefARN),
			})
		}
		if ecsState.TaskARN != "" {
			s.Registry.MarkCleanedUp(ecsState.TaskARN)
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
		s.ECS.Delete(c.ID)
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

// ContainerPause is not supported by the ECS backend.
func (s *Server) ContainerPause(ref string) error {
	_, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "ECS backend does not support pause"}
}

// ContainerUnpause is not supported by the ECS backend.
func (s *Server) ContainerUnpause(ref string) error {
	_, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "ECS backend does not support unpause"}
}

// ImagePull pulls an image reference and stores it locally.
func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
	if ref == "" {
		return nil, &api.InvalidParameterError{Message: "image reference is required"}
	}

	// Add :latest if no tag or digest
	if !strings.Contains(ref, ":") && !strings.Contains(ref, "@") {
		ref += ":latest"
	}

	// Attempt to fetch image config from registry
	imgConfig, err := s.fetchImageConfig(ref, auth)
	if err != nil {
		s.Logger.Warn().Err(err).Str("ref", ref).Msg("failed to fetch image config from registry, using synthetic")
	}

	// Generate image ID
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

	// Merge config from registry if available
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
		defer func() { _ = pw.Close() }()

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

// ImageLoad is not supported by the ECS backend.
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	return nil, &api.NotImplementedError{Message: "image load is not supported by ECS backend"}
}

// VolumeRemove removes a volume by name.
func (s *Server) VolumeRemove(name string, force bool) error {
	if !s.Store.Volumes.Delete(name) {
		return &api.NotFoundError{Resource: "volume", ID: name}
	}
	s.VolumeState.Delete(name)
	return nil
}

// ExecStart starts an exec instance. For ECS, if no agent is connected,
// we cannot execute commands inside the remote Fargate task without the
// ECS ExecuteCommand API (SSM), which is not yet implemented. In that case,
// return a clear error. If an agent is connected, delegate to BaseServer
// which routes through the agent driver.
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

	// If an agent is connected, delegate to BaseServer (agent driver handles it)
	if c.AgentAddress != "" {
		return s.BaseServer.ExecStart(id, opts)
	}

	// No agent connected — cannot exec into a remote Fargate task without
	// ECS ExecuteCommand + SSM Session Manager (not yet implemented).
	return nil, &api.NotImplementedError{
		Message: "exec requires an agent connection for ECS containers; ECS ExecuteCommand (SSM) is not yet supported",
	}
}

// PodStart starts all containers in a pod by calling ContainerStart for each.
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
			errs = append(errs, fmt.Sprintf("container %s: %v", cid[:12], err))
		}
	}
	if len(errs) == 0 {
		s.Store.Pods.SetStatus(pod.ID, "running")
		errs = []string{}
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// PodStop stops all containers in a pod by calling ContainerStop for each.
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
			// NotModifiedError is not a real error — container was already stopped
			if _, ok := err.(*api.NotModifiedError); !ok {
				errs = append(errs, fmt.Sprintf("container %s: %v", cid[:12], err))
			}
		}
	}
	s.Store.Pods.SetStatus(pod.ID, "stopped")
	if errs == nil {
		errs = []string{}
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// PodKill sends a signal to all containers in a pod by calling ContainerKill for each.
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
			errs = append(errs, fmt.Sprintf("container %s: %v", cid[:12], err))
		}
	}
	s.Store.Pods.SetStatus(pod.ID, "exited")
	if errs == nil {
		errs = []string{}
	}
	return &api.PodActionResponse{ID: pod.ID, Errs: errs}, nil
}

// PodRemove removes a pod and all its containers by calling ContainerRemove for each.
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

	// Remove each container through our ContainerRemove (handles ECS cleanup)
	for _, cid := range pod.ContainerIDs {
		if _, ok := s.Store.Containers.Get(cid); !ok {
			continue
		}
		_ = s.ContainerRemove(cid, force)
	}

	s.Store.Pods.DeletePod(pod.ID)
	return nil
}

// Info returns system information, enriched with real ECS cluster stats.
func (s *Server) Info() (*api.BackendInfo, error) {
	// Get base info from BaseServer (in-memory container/image counts)
	info, err := s.BaseServer.Info()
	if err != nil {
		return nil, err
	}

	// Enrich with real cluster data from ECS DescribeClusters
	result, err := s.aws.ECS.DescribeClusters(s.ctx(), &awsecs.DescribeClustersInput{
		Clusters: []string{s.config.Cluster},
	})
	if err != nil {
		// Non-fatal: return base info if AWS call fails
		s.Logger.Warn().Err(err).Msg("failed to describe ECS cluster for Info")
		return info, nil
	}

	if len(result.Clusters) > 0 {
		cluster := result.Clusters[0]
		info.ContainersRunning = int(cluster.RunningTasksCount)
	}

	return info, nil
}

// VolumePrune removes all unused volumes.
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
			// BUG-484: Sum volume dir sizes for SpaceReclaimed
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
