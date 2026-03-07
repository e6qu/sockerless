package lambda

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
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// Compile-time check that Server implements api.Backend.
var _ api.Backend = (*Server)(nil)

// ContainerCreate creates a container backed by an AWS Lambda function.
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
		// BUG-541: Merge ENV by key — image provides defaults, container overrides
		config.Env = core.MergeEnvByKey(img.Config.Env, config.Env)
		// BUG-542: Docker clears image Cmd when Entrypoint is overridden in create
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
		Driver:   "lambda",
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

	// Build function name from container name
	funcName := "skls-" + id[:12]

	// Build environment variables
	envVars := make(map[string]string)
	for _, e := range config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}

	// Build resource tags
	tags := core.TagSet{
		ContainerID: id,
		Backend:     "lambda",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
	}

	// Create Lambda function
	createInput := &awslambda.CreateFunctionInput{
		FunctionName: aws.String(funcName),
		Role:         aws.String(s.config.RoleARN),
		PackageType:  lambdatypes.PackageTypeImage,
		Code: &lambdatypes.FunctionCode{
			ImageUri: aws.String(config.Image),
		},
		MemorySize: aws.Int32(int32(s.config.MemorySize)),
		Timeout:    aws.Int32(int32(s.config.Timeout)),
		Tags:       tags.AsMap(),
	}

	if len(envVars) > 0 {
		createInput.Environment = &lambdatypes.Environment{
			Variables: envVars,
		}
	}

	// Add VPC config if subnets are specified
	if len(s.config.SubnetIDs) > 0 {
		createInput.VpcConfig = &lambdatypes.VpcConfig{
			SubnetIds:        s.config.SubnetIDs,
			SecurityGroupIds: s.config.SecurityGroupIDs,
		}
	}

	// Generate agent token and build callback entrypoint if configured
	agentToken := ""
	if s.config.CallbackURL != "" {
		agentToken = core.GenerateToken()
		callbackURL := fmt.Sprintf("%s/internal/v1/agent/connect?id=%s&token=%s", s.config.CallbackURL, id, agentToken)
		agentEntrypoint := core.BuildAgentCallbackEntrypoint(config, callbackURL)

		// Add agent env vars
		envVars["SOCKERLESS_CONTAINER_ID"] = id
		envVars["SOCKERLESS_AGENT_TOKEN"] = agentToken
		envVars["SOCKERLESS_AGENT_CALLBACK_URL"] = callbackURL
		if createInput.Environment == nil {
			createInput.Environment = &lambdatypes.Environment{Variables: envVars}
		} else {
			createInput.Environment.Variables = envVars
		}

		// Override entrypoint with agent wrapper
		createInput.ImageConfig = &lambdatypes.ImageConfig{
			EntryPoint: agentEntrypoint,
		}
		if config.WorkingDir != "" {
			createInput.ImageConfig.WorkingDirectory = aws.String(config.WorkingDir)
		}
	} else {
		// Set image config overrides if cmd/entrypoint specified
		if len(config.Cmd) > 0 || len(config.Entrypoint) > 0 || config.WorkingDir != "" {
			imgConfig := &lambdatypes.ImageConfig{}
			if len(config.Entrypoint) > 0 {
				imgConfig.EntryPoint = config.Entrypoint
			}
			if len(config.Cmd) > 0 {
				imgConfig.Command = config.Cmd
			}
			if config.WorkingDir != "" {
				imgConfig.WorkingDirectory = aws.String(config.WorkingDir)
			}
			createInput.ImageConfig = imgConfig
		}
	}

	result, err := s.aws.Lambda.CreateFunction(s.ctx(), createInput)
	if err != nil {
		s.Logger.Error().Err(err).Str("function", funcName).Msg("failed to create Lambda function")
		return nil, mapAWSError(err, "function", funcName)
	}

	functionARN := aws.ToString(result.FunctionArn)

	s.Store.Containers.Put(id, container)
	s.Store.ContainerNames.Put(name, id)

	s.Lambda.Put(id, LambdaState{
		FunctionName: funcName,
		FunctionARN:  functionARN,
		AgentToken:   agentToken,
	})

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "lambda",
		ResourceType: "function",
		ResourceID:   functionARN,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": container.Image, "name": container.Name, "functionName": funcName},
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

// ContainerStart starts a Lambda function invocation for the container.
func (s *Server) ContainerStart(ref string) error {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	if c.State.Running {
		return &api.NotModifiedError{}
	}

	// Multi-container pods are not supported by FaaS backends
	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod && len(pod.ContainerIDs) > 1 {
		return &api.InvalidParameterError{
			Message: "multi-container pods are not supported by the lambda backend",
		}
	}

	lambdaState, _ := s.Lambda.Get(id)

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

	// Non-tail-dev-null containers: invoke the function with the container's command
	// to get real execution, then stop with the real exit code.
	if !core.IsTailDevNull(c.Config.Entrypoint, c.Config.Cmd) {
		cmd := core.BuildOriginalCommand(c.Config.Entrypoint, c.Config.Cmd)
		if len(cmd) > 0 {
			// Invoke the function (already has ImageConfig.Command from create)
			go func() {
				exitCode := 0
				result, err := s.aws.Lambda.Invoke(s.ctx(), &awslambda.InvokeInput{
					FunctionName: aws.String(lambdaState.FunctionName),
				})
				if err != nil {
					s.Logger.Error().Err(err).Str("function", lambdaState.FunctionName).Msg("Lambda invocation failed")
					exitCode = 1
				} else {
					if result.FunctionError != nil {
						exitCode = 1
					}
					// Store response payload in log buffer for container logs
					if len(result.Payload) > 0 && string(result.Payload) != "{}" {
						if c, ok := s.Store.Containers.Get(id); ok && c.State.Running {
							s.Store.LogBuffers.Store(id, result.Payload)
						}
					}
				}
				if c, ok := s.Store.Containers.Get(id); ok && c.State.Running {
					s.Store.StopContainer(id, exitCode)
				}
			}()
		} else {
			// No command: auto-stop after brief delay
			go func() {
				time.Sleep(500 * time.Millisecond)
				if c, ok := s.Store.Containers.Get(id); ok && c.State.Running {
					s.Store.StopContainer(id, 0)
				}
			}()
		}
		return nil
	}

	// Pre-create done channel so invoke goroutine can wait for agent disconnect
	if s.config.CallbackURL != "" {
		s.AgentRegistry.Prepare(id)
	}

	// Invoke Lambda function asynchronously
	go func() {
		result, err := s.aws.Lambda.Invoke(s.ctx(), &awslambda.InvokeInput{
			FunctionName: aws.String(lambdaState.FunctionName),
		})

		exitCode := 0
		if err != nil {
			s.Logger.Error().Err(err).Str("function", lambdaState.FunctionName).Msg("Lambda invocation failed")
			exitCode = 1
		} else if result.FunctionError != nil {
			s.Logger.Warn().Str("error", aws.ToString(result.FunctionError)).Msg("Lambda function returned error")
			exitCode = 1
		}

		// Wait for reverse agent to disconnect before stopping.
		// In production, agent exits when function returns (near-instant wait).
		// In simulator mode, agent stays connected until runner finishes execs.
		if s.config.CallbackURL != "" {
			_ = s.AgentRegistry.WaitForDisconnect(id, 30*time.Minute)
		}

		if _, ok := s.Store.Containers.Get(id); ok {
			s.Store.StopContainer(id, exitCode)
		}
	}()

	// Wait for reverse agent callback if configured
	if s.config.CallbackURL != "" {
		if err := s.AgentRegistry.WaitForAgent(id, 30*time.Second); err != nil {
			s.Logger.Warn().Err(err).Msg("agent callback timeout, exec will use synthetic fallback")
			s.AgentRegistry.Remove(id)
		} else {
			s.Store.Containers.Update(id, func(c *api.Container) {
				c.AgentAddress = "reverse"
				c.AgentToken = lambdaState.AgentToken
			})
		}
	}

	return nil
}

// ContainerStop stops a running Lambda container.
func (s *Server) ContainerStop(ref string, timeout *int) error {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	// Lambda functions run to completion — stop transitions state
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

	// Parse signal and transition container to exited state
	exitCode := signalToExitCode(signal)

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

// ContainerRemove removes a container and its associated Lambda resources.
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
		s.Store.ForceStopContainer(id, 0)
	}

	s.StopHealthCheck(id)

	// Delete Lambda function (best-effort)
	lambdaState, _ := s.Lambda.Get(id)
	if lambdaState.FunctionName != "" {
		_, _ = s.aws.Lambda.DeleteFunction(s.ctx(), &awslambda.DeleteFunctionInput{
			FunctionName: aws.String(lambdaState.FunctionName),
		})
	}

	if lambdaState.FunctionARN != "" {
		s.Registry.MarkCleanedUp(lambdaState.FunctionARN)
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
	s.Lambda.Delete(id)
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

	lambdaState, _ := s.Lambda.Get(id)
	logStreamPrefix := lambdaState.FunctionName

	// Early return if stdout suppressed
	if !params.ShouldWrite() {
		return io.NopCloser(strings.NewReader("")), nil
	}

	pr, pw := io.Pipe()

	go func() {
		defer func() { _ = pw.Close() }()

		// BUG-435: Filter LogBuffers output through params (raw text, no mux framing)
		if buf, ok := s.Store.LogBuffers.Load(id); ok {
			data := buf.([]byte)
			if len(data) > 0 {
				raw := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
				now := time.Now().UTC()
				var filtered []string
				for _, line := range raw {
					if line == "" {
						continue
					}
					if !params.FilterByTime(now) {
						continue
					}
					filtered = append(filtered, line)
				}
				filtered = params.ApplyTail(filtered)
				for _, line := range filtered {
					formatted := params.FormatLine(line, now)
					if _, err := pw.Write([]byte(formatted)); err != nil {
						return
					}
				}
			}
		}

		// Query CloudWatch Logs using the function name as log group prefix
		logGroupName := fmt.Sprintf("/aws/lambda/%s", logStreamPrefix)

		// Try to find a log stream for this function
		streamsResult, err := s.aws.CloudWatch.DescribeLogStreams(s.ctx(), &cloudwatchlogs.DescribeLogStreamsInput{
			LogGroupName: aws.String(logGroupName),
			OrderBy:      "LastEventTime",
			Descending:   aws.Bool(true),
			Limit:        aws.Int32(1),
		})
		if err != nil {
			s.Logger.Debug().Err(err).Str("logGroup", logGroupName).Msg("failed to describe log streams")
			return
		}

		if len(streamsResult.LogStreams) == 0 {
			return
		}

		logStreamName := streamsResult.LogStreams[0].LogStreamName

		input := &cloudwatchlogs.GetLogEventsInput{
			LogGroupName:  aws.String(logGroupName),
			LogStreamName: logStreamName,
			StartFromHead: aws.Bool(params.CloudLogTailInt32() == nil),
		}
		if limit := params.CloudLogTailInt32(); limit != nil {
			input.Limit = limit
		}
		// BUG-423: Apply since as StartTime
		if ms := params.SinceMillis(); ms != nil {
			input.StartTime = ms
		}
		// BUG-424: Apply until as EndTime
		if ms := params.UntilMillis(); ms != nil {
			input.EndTime = ms
		}

		result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), input)
		if err != nil {
			s.Logger.Debug().Err(err).Msg("failed to get log events")
			return
		}

		for _, event := range result.Events {
			line := s.formatLogEventText(event.Message, event.Timestamp, params)
			if line == "" {
				continue
			}
			if _, err := pw.Write([]byte(line)); err != nil {
				return
			}
		}

		// BUG-428: Follow mode support
		if !params.Follow {
			return
		}

		nextToken := result.NextForwardToken

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			c, _ := s.Store.Containers.Get(id)
			if !c.State.Running && c.State.Status != "created" {
				// Fetch any remaining logs
				followInput := &cloudwatchlogs.GetLogEventsInput{
					LogGroupName:  aws.String(logGroupName),
					LogStreamName: logStreamName,
					StartFromHead: aws.Bool(true),
					NextToken:     nextToken,
				}
				result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), followInput)
				if err == nil {
					for _, event := range result.Events {
						line := s.formatLogEventText(event.Message, event.Timestamp, params)
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

			followInput := &cloudwatchlogs.GetLogEventsInput{
				LogGroupName:  aws.String(logGroupName),
				LogStreamName: logStreamName,
				StartFromHead: aws.Bool(true),
				NextToken:     nextToken,
			}
			result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), followInput)
			if err != nil {
				continue
			}

			for _, event := range result.Events {
				line := s.formatLogEventText(event.Message, event.Timestamp, params)
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

// formatLogEventText formats a single CloudWatch log event as raw text.
// Returns empty string if message is nil.
func (s *Server) formatLogEventText(message *string, timestamp *int64, params core.CloudLogParams) string {
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
	if c.State.Running {
		s.StopHealthCheck(id)
		s.AgentRegistry.Remove(id)
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
		// BUG-481: Sum image sizes for SpaceReclaimed
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			spaceReclaimed += uint64(img.Size)
		}
		// Clean up Lambda cloud resources
		lambdaState, _ := s.Lambda.Get(c.ID)
		if lambdaState.FunctionName != "" {
			_, _ = s.aws.Lambda.DeleteFunction(s.ctx(), &awslambda.DeleteFunctionInput{
				FunctionName: aws.String(lambdaState.FunctionName),
			})
		}
		if lambdaState.FunctionARN != "" {
			s.Registry.MarkCleanedUp(lambdaState.FunctionARN)
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
		s.Lambda.Delete(c.ID)
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

// ContainerPause is not supported by the Lambda backend.
func (s *Server) ContainerPause(ref string) error {
	_, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "Lambda backend does not support pause"}
}

// ContainerUnpause is not supported by the Lambda backend.
func (s *Server) ContainerUnpause(ref string) error {
	_, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "Lambda backend does not support unpause"}
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

	// Generate image ID
	hash := sha256.Sum256([]byte(ref))
	imageID := fmt.Sprintf("sha256:%x", hash)

	imgConfig := api.ContainerConfig{
		Image: ref,
	}

	// Try to fetch real config from registry
	if realConfig, _ := core.FetchImageConfig(ref, ""); realConfig != nil {
		if len(realConfig.Env) > 0 {
			imgConfig.Env = realConfig.Env
		}
		if len(realConfig.Cmd) > 0 {
			imgConfig.Cmd = realConfig.Cmd
		}
		if len(realConfig.Entrypoint) > 0 {
			imgConfig.Entrypoint = realConfig.Entrypoint
		}
		if realConfig.WorkingDir != "" {
			imgConfig.WorkingDir = realConfig.WorkingDir
		}
		if len(realConfig.Labels) > 0 {
			imgConfig.Labels = realConfig.Labels
		}
	}

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
		Config:       imgConfig,
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

// ImageLoad is not supported by the Lambda backend.
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	return nil, &api.NotImplementedError{Message: "image load is not supported by Lambda backend"}
}

// ImageBuild is not supported by the Lambda backend.
// Lambda requires pre-built images stored in ECR.
func (s *Server) ImageBuild(opts api.ImageBuildOptions, buildContext io.Reader) (io.ReadCloser, error) {
	return nil, &api.NotImplementedError{
		Message: "Lambda backend does not support image build; push pre-built images to ECR and use the ECR image URI",
	}
}

// AuthLogin validates login credentials.
// For ECR registries, logs a warning that credentials should be obtained via
// `aws ecr get-login-password`. For all other registries, delegates to BaseServer.
func (s *Server) AuthLogin(req *api.AuthRequest) (*api.AuthResponse, error) {
	if strings.HasSuffix(req.ServerAddress, ".amazonaws.com") &&
		strings.Contains(req.ServerAddress, ".dkr.ecr.") {
		// ECR registry — store credentials via BaseServer but warn that
		// ECR auth tokens should be obtained via `aws ecr get-login-password`.
		s.Logger.Warn().
			Str("registry", req.ServerAddress).
			Msg("ECR login: credentials stored locally; use `aws ecr get-login-password` for production")
		return s.BaseServer.AuthLogin(req)
	}
	return s.BaseServer.AuthLogin(req)
}

// Info returns system information with Lambda-appropriate values.
func (s *Server) Info() (*api.BackendInfo, error) {
	info, err := s.BaseServer.Info()
	if err != nil {
		return nil, err
	}
	info.KernelVersion = "5.10.0-aws-lambda"
	info.OperatingSystem = "AWS Lambda"
	return info, nil
}

// ContainerAttach attaches to a container's streams.
// Only supported when a reverse agent is connected; otherwise Lambda functions
// are not interactive.
func (s *Server) ContainerAttach(id string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	cid, ok := s.Store.ResolveContainerID(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	c, _ := s.Store.Containers.Get(cid)
	if c.AgentAddress != "" {
		return s.BaseServer.ContainerAttach(id, opts)
	}
	return nil, &api.NotImplementedError{
		Message: "Lambda backend does not support attach without a connected agent",
	}
}

// ImagePush is not supported by the Lambda backend.
// Images should be pushed directly to ECR using the AWS CLI or SDK.
func (s *Server) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
	return nil, &api.NotImplementedError{
		Message: "Lambda backend does not support image push; push images directly to ECR",
	}
}
