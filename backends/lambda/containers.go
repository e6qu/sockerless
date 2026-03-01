package lambda

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
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
		Driver:   "lambda",
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
		core.WriteError(w, mapAWSError(err, "function", funcName))
		return
	}

	functionARN := aws.ToString(result.FunctionArn)

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "lambda",
		ResourceType: "function",
		ResourceID:   functionARN,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": container.Image, "name": container.Name, "functionName": funcName},
	})

	s.Store.Containers.Put(id, container)
	s.Store.ContainerNames.Put(name, id)
	s.Lambda.Put(id, LambdaState{
		FunctionName: funcName,
		FunctionARN:  functionARN,
		AgentToken:   agentToken,
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

	// Multi-container pods are not supported by FaaS backends
	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod && len(pod.ContainerIDs) > 1 {
		core.WriteError(w, &api.InvalidParameterError{
			Message: "multi-container pods are not supported by the lambda backend",
		})
		return
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

	// Non-tail-dev-null containers: invoke the function with the container's command
	// to get real execution, then stop with the real exit code.
	if !core.IsTailDevNull(c.Config.Entrypoint, c.Config.Cmd) {
		cmd := core.BuildOriginalCommand(c.Config.Entrypoint, c.Config.Cmd)
		if len(cmd) > 0 && s.config.EndpointURL != "" {
			// Simulator mode: invoke the function (already has ImageConfig.Command from create)
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
						s.Store.LogBuffers.Store(id, result.Payload)
					}
				}
				s.Store.StopContainer(id, exitCode)
			}()
		} else {
			// No command or production mode: auto-stop after brief delay
			go func() {
				time.Sleep(500 * time.Millisecond)
				s.Store.StopContainer(id, 0)
			}()
		}
		w.WriteHeader(http.StatusNoContent)
		return
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

		s.Store.StopContainer(id, exitCode)
	}()

	// Wait for reverse agent callback if configured
	if s.config.CallbackURL != "" {
		agentTimeout := 60 * time.Second
		if s.config.EndpointURL != "" {
			// Simulator mode: agent subprocess needs startup time
			agentTimeout = 5 * time.Second
		}
		if err := s.AgentRegistry.WaitForAgent(id, agentTimeout); err != nil {
			s.Logger.Warn().Err(err).Msg("agent callback timeout, exec will use synthetic fallback")
		} else {
			s.Store.Containers.Update(id, func(c *api.Container) {
				c.AgentAddress = "reverse"
				c.AgentToken = lambdaState.AgentToken
			})
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

	// Lambda functions run to completion — stop is a no-op
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
		s.Store.StopContainer(id, 0)
	}

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

	s.Store.Containers.Delete(id)
	s.Store.ContainerNames.Delete(c.Name)
	s.Lambda.Delete(id)
	s.Store.WaitChs.Delete(id)

	w.WriteHeader(http.StatusNoContent)
}
