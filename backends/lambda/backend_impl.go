package lambda

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/sockerless/api"
	awscommon "github.com/sockerless/aws-common"
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
		// Merge ENV by key — image provides defaults, container overrides
		config.Env = core.MergeEnvByKey(img.Config.Env, config.Env)
		// Docker clears image Cmd when Entrypoint is overridden in create
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

	// Resolve image to ECR URI (Lambda only supports ECR images)
	imageURI, err := s.resolveImageURI(s.ctx(), config.Image)
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("failed to resolve image %q to ECR URI: %v", config.Image, err)}
	}

	// Create Lambda function
	createInput := &awslambda.CreateFunctionInput{
		FunctionName: aws.String(funcName),
		Role:         aws.String(s.config.RoleARN),
		PackageType:  lambdatypes.PackageTypeImage,
		Code: &lambdatypes.FunctionCode{
			ImageUri: aws.String(imageURI),
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
		return nil, awscommon.MapAWSError(err, "function", funcName)
	}

	functionARN := aws.ToString(result.FunctionArn)

	s.PendingCreates.Put(id, container)

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

	// Multi-container pods are not supported by FaaS backends
	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod && len(pod.ContainerIDs) > 1 {
		return &api.InvalidParameterError{
			Message: "multi-container pods are not supported by the lambda backend",
		}
	}

	lambdaState, _ := s.Lambda.Get(id)

	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	s.EmitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})

	// Remove from PendingCreates now that the function is being invoked.
	s.PendingCreates.Delete(id)

	// Non-tail-dev-null containers: invoke the function with the container's command
	// to get real execution, then stop with the real exit code.
	if !core.IsTailDevNull(c.Config.Entrypoint, c.Config.Cmd) {
		cmd := core.BuildOriginalCommand(c.Config.Entrypoint, c.Config.Cmd)
		if len(cmd) > 0 {
			// Invoke the function (already has ImageConfig.Command from create)
			go func() {
				result, err := s.aws.Lambda.Invoke(s.ctx(), &awslambda.InvokeInput{
					FunctionName: aws.String(lambdaState.FunctionName),
				})
				if err != nil {
					s.Logger.Error().Err(err).Str("function", lambdaState.FunctionName).Msg("Lambda invocation failed")
				} else {
					if result.FunctionError != nil {
						s.Logger.Warn().Str("error", aws.ToString(result.FunctionError)).Msg("Lambda function returned error")
					}
					// Store response payload in log buffer for container logs
					if len(result.Payload) > 0 && string(result.Payload) != "{}" {
						s.Store.LogBuffers.Store(id, result.Payload)
					}
				}
				if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
					close(ch.(chan struct{}))
				}
			}()
		} else {
			// No command: auto-stop after brief delay
			go func() {
				time.Sleep(500 * time.Millisecond)
				if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
					close(ch.(chan struct{}))
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

		if err != nil {
			s.Logger.Error().Err(err).Str("function", lambdaState.FunctionName).Msg("Lambda invocation failed")
		} else if result.FunctionError != nil {
			s.Logger.Warn().Str("error", aws.ToString(result.FunctionError)).Msg("Lambda function returned error")
		}

		// Wait for reverse agent to disconnect before stopping.
		// In production, agent exits when function returns (near-instant wait).
		// In simulator mode, agent stays connected until runner finishes execs.
		if s.config.CallbackURL != "" {
			_ = s.AgentRegistry.WaitForDisconnect(id, 30*time.Minute)
		}

		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
		}
	}()

	// Wait for reverse agent callback if configured
	if s.config.CallbackURL != "" {
		if err := s.AgentRegistry.WaitForAgent(id, s.config.AgentTimeout); err != nil {
			s.Logger.Warn().Err(err).Msg("agent callback timeout, exec will use synthetic fallback")
			s.AgentRegistry.Remove(id)
		} else {
			s.Lambda.Update(id, func(state *LambdaState) {
				state.AgentAddress = "reverse"
			})
		}
	}

	return nil
}

// ContainerStop stops a running Lambda container.
func (s *Server) ContainerStop(ref string, timeout *int) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	// Lambda functions run to completion — stop transitions state
	s.StopHealthCheck(id)
	s.AgentRegistry.Remove(id)
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
	s.AgentRegistry.Remove(id)

	exitCode := core.SignalToExitCode(signal)

	s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": fmt.Sprintf("%d", exitCode), "name": strings.TrimPrefix(c.Name, "/")})

	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}

	return nil
}

// ContainerRemove removes a container and its associated Lambda resources.
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
	s.AgentRegistry.Remove(id)

	if c.State.Running {
		s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
	}

	s.StopHealthCheck(id)
	s.AgentRegistry.Remove(id)

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

	s.PendingCreates.Delete(id)
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
	// Resolve state upfront so the closure can capture it.
	var logGroupName, logStreamName *string
	if id, ok := s.ResolveContainerIDAuto(context.Background(), ref); ok {
		lambdaState, _ := s.Lambda.Get(id)
		group := fmt.Sprintf("/aws/lambda/%s", lambdaState.FunctionName)
		logGroupName = &group

		// Find the most recent log stream.
		streamsResult, err := s.aws.CloudWatch.DescribeLogStreams(s.ctx(), &cloudwatchlogs.DescribeLogStreamsInput{
			LogGroupName: aws.String(group),
			OrderBy:      "LastEventTime",
			Descending:   aws.Bool(true),
			Limit:        aws.Int32(1),
		})
		if err == nil && len(streamsResult.LogStreams) > 0 {
			logStreamName = streamsResult.LogStreams[0].LogStreamName
		}
	}

	fetch := func(ctx context.Context, params core.CloudLogParams, cursor any) ([]core.CloudLogEntry, any, error) {
		if logGroupName == nil || logStreamName == nil {
			return nil, nil, nil
		}

		input := &cloudwatchlogs.GetLogEventsInput{
			LogGroupName:  logGroupName,
			LogStreamName: logStreamName,
			StartFromHead: aws.Bool(true),
		}

		if cursor != nil {
			input.NextToken = cursor.(*string)
		} else {
			input.StartFromHead = aws.Bool(params.CloudLogTailInt32() == nil)
			if limit := params.CloudLogTailInt32(); limit != nil {
				input.Limit = limit
			}
			if ms := params.SinceMillis(); ms != nil {
				input.StartTime = ms
			}
			if ms := params.UntilMillis(); ms != nil {
				input.EndTime = ms
			}
		}

		result, err := s.aws.CloudWatch.GetLogEvents(s.ctx(), input)
		if err != nil {
			return nil, cursor, err
		}

		var entries []core.CloudLogEntry
		for _, event := range result.Events {
			if event.Message == nil {
				continue
			}
			var ts time.Time
			if event.Timestamp != nil {
				ts = time.UnixMilli(*event.Timestamp)
			}
			entries = append(entries, core.CloudLogEntry{Timestamp: ts, Message: *event.Message})
		}
		return entries, result.NextForwardToken, nil
	}

	return core.StreamCloudLogs(s.BaseServer, ref, opts, fetch, core.StreamCloudLogsOptions{
		CheckLogBuffers: true,
	})
}

// ContainerRestart stops and then starts a container.
func (s *Server) ContainerRestart(ref string, timeout *int) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if c.State.Running {
		s.StopHealthCheck(id)
		s.AgentRegistry.Remove(id)
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
// Lambda functions that have already run are cleaned up via ContainerRemove.
func (s *Server) ContainerPrune(filters map[string][]string) (*api.ContainerPruneResponse, error) {
	labelFilters := filters["label"]
	untilFilters := filters["until"]
	var deleted []string
	var spaceReclaimed uint64

	// Check PendingCreates for containers that were created but never started
	for _, c := range s.PendingCreates.List() {
		// PendingCreates containers are in "created" state — treat as pruneable
		if len(labelFilters) > 0 && !core.MatchLabels(c.Config.Labels, labelFilters) {
			continue
		}
		if len(untilFilters) > 0 && !core.MatchUntil(c.Created, untilFilters) {
			continue
		}
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
		for _, ep := range c.NetworkSettings.Networks {
			if ep != nil && ep.NetworkID != "" {
				_ = s.Drivers.Network.Disconnect(context.Background(), ep.NetworkID, c.ID)
			}
		}
		if pod, inPod := s.Store.Pods.GetPodForContainer(c.ID); inPod {
			s.Store.Pods.RemoveContainer(pod.ID, c.ID)
		}
		s.PendingCreates.Delete(c.ID)
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
	if _, ok := s.ResolveContainerIDAuto(context.Background(), ref); !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "Lambda backend does not support pause"}
}

// ContainerUnpause is not supported by the Lambda backend.
func (s *Server) ContainerUnpause(ref string) error {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), ref); !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "Lambda backend does not support unpause"}
}

// ImagePull pulls an image, using ECR cloud auth when available.
func (s *Server) ImagePull(ref string, auth string) (io.ReadCloser, error) {
	return s.images.Pull(ref, auth)
}

// ImageTag tags an image and syncs the new tag to ECR.
func (s *Server) ImageTag(source string, repo string, tag string) error {
	return s.images.Tag(source, repo, tag)
}

// ImageRemove removes an image and syncs the removal to ECR.
func (s *Server) ImageRemove(name string, force bool, prune bool) ([]*api.ImageDeleteResponse, error) {
	return s.images.Remove(name, force, prune)
}

// ImageLoad loads an image from a tar archive.
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	return s.images.Load(r)
}

// ImageBuild delegates to the shared ImageManager.
func (s *Server) ImageBuild(opts api.ImageBuildOptions, buildContext io.Reader) (io.ReadCloser, error) {
	return s.images.Build(opts, buildContext)
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
	c, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	if c.AgentAddress != "" {
		return s.BaseServer.ContainerAttach(id, opts)
	}
	return nil, &api.NotImplementedError{
		Message: "Lambda backend does not support attach without a connected agent",
	}
}

// ContainerExport is not supported by the Lambda backend.
// Lambda functions have no local filesystem to export.
func (s *Server) ContainerExport(id string) (io.ReadCloser, error) {
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return nil, &api.NotImplementedError{
		Message: "Lambda backend does not support container export; functions have no local filesystem",
	}
}

// ContainerCommit is not supported by the Lambda backend.
// Lambda functions have no local filesystem to commit.
func (s *Server) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	if req.Container == "" {
		return nil, &api.InvalidParameterError{Message: "container query parameter is required"}
	}
	if _, ok := s.ResolveContainerIDAuto(context.Background(), req.Container); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: req.Container}
	}
	return nil, &api.NotImplementedError{
		Message: "Lambda backend does not support container commit; functions have no local filesystem",
	}
}

// ImagePush pushes an image, syncing to ECR when applicable.
func (s *Server) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
	return s.images.Push(name, tag, auth)
}
