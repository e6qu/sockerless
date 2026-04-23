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
		Cluster:     s.config.Region,
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
		Name:        name,
		Labels:      config.Labels,
	}

	// Resolve the image URI. If the operator shipped a pre-baked
	// overlay (PrebuiltOverlayImage) and we're in reverse-agent mode
	// (CallbackURL set), the user's Image is irrelevant — the Lambda
	// function runs the operator's image, and the user's original
	// entrypoint+cmd are passed via env vars. Skip ECR resolution
	// entirely in that case; the prebuilt image is expected to be
	// already pushed to ECR (or resolvable locally in integration).
	var imageURI string
	switch {
	case s.config.CallbackURL != "" && s.config.PrebuiltOverlayImage != "":
		imageURI = s.config.PrebuiltOverlayImage
		envVars["SOCKERLESS_CALLBACK_URL"] = s.config.CallbackURL
		envVars["SOCKERLESS_CONTAINER_ID"] = id
		if len(config.Entrypoint) > 0 {
			envVars["SOCKERLESS_USER_ENTRYPOINT"] = strings.Join(config.Entrypoint, ":")
		}
		if len(config.Cmd) > 0 {
			envVars["SOCKERLESS_USER_CMD"] = strings.Join(config.Cmd, ":")
		}
	case s.config.CallbackURL != "":
		// Build + push an overlay on top of the user's image. Resolve
		// the base image first so the Dockerfile's FROM can reference
		// something Lambda can pull.
		base, err := s.resolveImageURI(s.ctx(), config.Image)
		if err != nil {
			return nil, &api.ServerError{Message: fmt.Sprintf("failed to resolve image %q to ECR URI: %v", config.Image, err)}
		}
		imageURI = base
		spec := OverlayImageSpec{
			BaseImageRef:        imageURI,
			AgentBinaryPath:     s.config.AgentBinaryPath,
			BootstrapBinaryPath: s.config.BootstrapBinaryPath,
			UserEntrypoint:      config.Entrypoint,
			UserCmd:             config.Cmd,
		}
		destRef := fmt.Sprintf("%s-overlay-%s", strings.TrimSuffix(imageURI, ":latest"), id[:12])
		overlay, buildErr := BuildAndPushOverlayImage(s.ctx(), spec, destRef)
		if buildErr != nil {
			s.Logger.Warn().Err(buildErr).Str("image", imageURI).
				Msg("overlay build failed; falling back to base image (docker exec will not work)")
		} else {
			imageURI = overlay.ImageURI
			envVars["SOCKERLESS_CALLBACK_URL"] = s.config.CallbackURL
			envVars["SOCKERLESS_CONTAINER_ID"] = id
		}
	default:
		var resolveErr error
		imageURI, resolveErr = s.resolveImageURI(s.ctx(), config.Image)
		if resolveErr != nil {
			return nil, &api.ServerError{Message: fmt.Sprintf("failed to resolve image %q to ECR URI: %v", config.Image, resolveErr)}
		}
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
		Tags:       func() map[string]string { m := tags.AsMap(); m["sockerless-image"] = config.Image; return m }(),
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

	// Phase 94b: attach named-volume binds as EFS FileSystemConfigs.
	// Reject host-path binds; require VPC + subnets (enforced by
	// fileSystemConfigsForBinds). Access points are sockerless-managed
	// via awscommon.EFSManager (shared with ECS).
	if len(hostConfig.Binds) > 0 {
		fsConfigs, err := s.fileSystemConfigsForBinds(s.ctx(), hostConfig.Binds)
		if err != nil {
			return nil, &api.InvalidParameterError{Message: fmt.Sprintf("resolve Lambda file-system configs: %v", err)}
		}
		createInput.FileSystemConfigs = fsConfigs
	}

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

	lambdaState, _ := s.resolveLambdaState(s.ctx(), id)

	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	s.EmitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})

	// Remove from PendingCreates now that the function is being invoked.
	s.PendingCreates.Delete(id)

	// Invoke Lambda function asynchronously. Phase 95: capture the
	// outcome in Store.InvocationResults so CloudState reflects the
	// container as exited with the real exit code.
	go func() {
		result, err := s.aws.Lambda.Invoke(s.ctx(), &awslambda.InvokeInput{
			FunctionName: aws.String(lambdaState.FunctionName),
		})

		inv := core.InvocationResult{}
		switch {
		case err != nil:
			s.Logger.Error().Err(err).Str("function", lambdaState.FunctionName).Msg("Lambda invocation failed")
			inv.ExitCode = 1
			inv.Error = err.Error()
		case result.FunctionError != nil:
			fnErr := aws.ToString(result.FunctionError)
			s.Logger.Warn().Str("error", fnErr).Msg("Lambda function returned error")
			inv.ExitCode = 1
			inv.Error = fnErr
			if len(result.Payload) > 0 {
				s.Store.LogBuffers.Store(id, result.Payload)
			}
		default:
			// Successful invocation — exit code 0.
			if len(result.Payload) > 0 && string(result.Payload) != "{}" {
				s.Store.LogBuffers.Store(id, result.Payload)
			}
		}
		s.Store.PutInvocationResult(id, inv)

		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
		}
	}()

	return nil
}

// ContainerStop stops a running Lambda container. AWS Lambda exposes no
// "cancel invoke" API and UpdateFunctionConfiguration only applies to
// future invocations — an in-flight invoke cannot be aborted from the
// control plane. Stop therefore does three things, in order:
// 1. Clamps the function timeout to the minimum (1s) so any subsequent
// invocations of this container are short-lived. Best-effort — a
// failure here is logged but non-fatal (the invocation may already
// be finishing and the function may already be gone).
// 2. Requests the reverse agent (if connected) to exit, which causes
// the agent-as-handler Lambda invocation to return immediately.
// This is the only path that actually cuts short an in-flight
// invocation. Containers without the bundled agent will keep
// running until natural completion or the 15-min AWS hard cap.
// 3. Closes the local wait channel so `docker wait` unblocks.
// Exit code 137 matches Docker's convention for force-stopped containers.
func (s *Server) ContainerStop(ref string, timeout *int) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	s.StopHealthCheck(id)

	// cloud-fallback lookup so stop works post-restart.
	if lambdaState, ok := s.resolveLambdaState(s.ctx(), id); ok && lambdaState.FunctionName != "" {
		_, err := s.aws.Lambda.UpdateFunctionConfiguration(s.ctx(),
			&awslambda.UpdateFunctionConfigurationInput{
				FunctionName: aws.String(lambdaState.FunctionName),
				Timeout:      aws.Int32(1),
			},
		)
		if err != nil {
			s.Logger.Debug().Err(err).Str("function", lambdaState.FunctionName).
				Msg("UpdateFunctionConfiguration(Timeout=1) failed during stop")
		}
	}

	s.disconnectReverseAgent(id)

	// Record the stop outcome so CloudState reports the container as
	// exited with code 137 (SIGKILL equivalent) even though Lambda has no
	// invocation-cancel API. Phase 95.
	s.Store.PutInvocationResult(id, core.InvocationResult{ExitCode: 137})

	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
	s.EmitEvent("container", "die", id, map[string]string{"exitCode": "137", "name": strings.TrimPrefix(c.Name, "/")})
	s.EmitEvent("container", "stop", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	return nil
}

// ContainerKill kills a container with the given signal. Lambda delivers
// no POSIX signals to invocations; termination follows the same path as
// ContainerStop (clamp future timeout, disconnect reverse agent, close
// wait channel). The supplied signal is reflected only in the reported
// exit code via SignalToExitCode.
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

	s.StopHealthCheck(id)

	exitCode := core.SignalToExitCode(signal)

	// cloud-fallback lookup so kill works post-restart.
	lambdaState, _ := s.resolveLambdaState(s.ctx(), id)
	if lambdaState.FunctionName != "" {
		_, err := s.aws.Lambda.UpdateFunctionConfiguration(s.ctx(),
			&awslambda.UpdateFunctionConfigurationInput{
				FunctionName: aws.String(lambdaState.FunctionName),
				Timeout:      aws.Int32(1),
			},
		)
		if err != nil {
			s.Logger.Debug().Err(err).Str("function", lambdaState.FunctionName).
				Msg("UpdateFunctionConfiguration(Timeout=1) failed during kill")
		}
	}

	s.disconnectReverseAgent(id)

	// Record the kill outcome so CloudState reports the container as
	// exited with the signal-derived code. Phase 95.
	s.Store.PutInvocationResult(id, core.InvocationResult{ExitCode: exitCode})

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

	if c.State.Running {
		s.EmitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		s.EmitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
	}

	s.StopHealthCheck(id)

	// Delete Lambda function (best-effort)
	lambdaState, _ := s.resolveLambdaState(s.ctx(), id)
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
	s.Store.DeleteInvocationResult(id)
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

// ContainerLogs streams container logs from CloudWatch. The log stream
// is resolved lazily on each fetch: Lambda creates the stream when the
// function is invoked for the first time, so if ContainerLogs is called
// before the invocation has produced output the stream lookup would
// return empty. In follow mode the fetch closure keeps checking until
// the stream appears.
func (s *Server) ContainerLogs(ref string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	var logGroupName *string
	if id, ok := s.ResolveContainerIDAuto(context.Background(), ref); ok {
		lambdaState, _ := s.resolveLambdaState(s.ctx(), id)
		if lambdaState.FunctionName != "" {
			group := fmt.Sprintf("/aws/lambda/%s", lambdaState.FunctionName)
			logGroupName = &group
		}
	}

	// resolveStream looks up the most recent log stream in the group;
	// returns nil if the stream doesn't exist yet.
	resolveStream := func() *string {
		if logGroupName == nil {
			return nil
		}
		out, err := s.aws.CloudWatch.DescribeLogStreams(s.ctx(), &cloudwatchlogs.DescribeLogStreamsInput{
			LogGroupName: logGroupName,
			OrderBy:      "LastEventTime",
			Descending:   aws.Bool(true),
			Limit:        aws.Int32(1),
		})
		if err != nil || len(out.LogStreams) == 0 {
			return nil
		}
		return out.LogStreams[0].LogStreamName
	}

	// Cache the resolved stream once it appears so we don't pay
	// DescribeLogStreams on every poll tick in follow mode.
	var cachedStream *string

	fetch := func(ctx context.Context, params core.CloudLogParams, cursor any) ([]core.CloudLogEntry, any, error) {
		if logGroupName == nil {
			return nil, nil, nil
		}
		if cachedStream == nil {
			cachedStream = resolveStream()
			if cachedStream == nil {
				return nil, cursor, nil
			}
		}

		input := &cloudwatchlogs.GetLogEventsInput{
			LogGroupName:  logGroupName,
			LogStreamName: cachedStream,
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
		lambdaState, _ := s.resolveLambdaState(s.ctx(), c.ID)
		if lambdaState.FunctionName != "" {
			_, _ = s.aws.Lambda.DeleteFunction(s.ctx(), &awslambda.DeleteFunctionInput{
				FunctionName: aws.String(lambdaState.FunctionName),
			})
		}
		if lambdaState.FunctionARN != "" {
			s.Registry.MarkCleanedUp(lambdaState.FunctionARN)
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

// ContainerPause sends SIGSTOP to the user subprocess via the reverse-
// agent.
func (s *Server) ContainerPause(ref string) error {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return core.MapPauseErr(core.RunContainerPauseViaAgent(s.reverseAgents, cid))
}

// ContainerUnpause sends SIGCONT to the user subprocess via the
// reverse-agent.
func (s *Server) ContainerUnpause(ref string) error {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return core.MapPauseErr(core.RunContainerUnpauseViaAgent(s.reverseAgents, cid))
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
	if _, ok := s.ResolveContainerIDAuto(context.Background(), id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return nil, &api.NotImplementedError{
		Message: "Lambda backend does not support attach",
	}
}

// ContainerExport streams a tar archive of the Lambda container's
// rootfs via the reverse-agent. Phase 98 (BUG-751). Buffered in memory;
// see core.RunContainerExportViaAgent for the size caveat.
func (s *Server) ContainerExport(id string) (io.ReadCloser, error) {
	cid, ok := s.ResolveContainerIDAuto(context.Background(), id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	rc, err := core.RunContainerExportViaAgent(s.reverseAgents, cid)
	if err == core.ErrNoReverseAgent {
		return nil, &api.NotImplementedError{Message: "docker export requires a reverse-agent bootstrap inside the container (SOCKERLESS_CALLBACK_URL); no session registered"}
	}
	if err != nil {
		return nil, &api.ServerError{Message: fmt.Sprintf("export via reverse-agent: %v", err)}
	}
	return rc, nil
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
