package gcf

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
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
		Driver:   "cloud-run-functions",
	}

	// Build function name from container ID
	funcName := "skls-" + id[:12]

	// Build environment variables
	envVars := make(map[string]string)
	for _, e := range config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}

	// Build fully qualified function name
	parent := fmt.Sprintf("projects/%s/locations/%s", s.config.Project, s.config.Region)
	fullFunctionName := fmt.Sprintf("%s/functions/%s", parent, funcName)

	// Generate agent token and build callback entrypoint if configured
	agentToken := ""
	if s.config.CallbackURL != "" {
		agentToken = core.GenerateToken()
		callbackURL := fmt.Sprintf("%s/internal/v1/agent/connect?id=%s&token=%s", s.config.CallbackURL, id, agentToken)
		agentEntrypoint := core.BuildAgentCallbackEntrypoint(config, callbackURL)
		config.Entrypoint = agentEntrypoint
		config.Cmd = nil

		envVars["SOCKERLESS_CONTAINER_ID"] = id
		envVars["SOCKERLESS_AGENT_TOKEN"] = agentToken
		envVars["SOCKERLESS_AGENT_CALLBACK_URL"] = callbackURL
	}

	// Build service config
	serviceConfig := &functionspb.ServiceConfig{
		AvailableMemory:      s.config.Memory,
		AvailableCpu:         s.config.CPU,
		TimeoutSeconds:       int32(s.config.Timeout),
		EnvironmentVariables: envVars,
	}

	if s.config.ServiceAccount != "" {
		serviceConfig.ServiceAccountEmail = s.config.ServiceAccount
	}

	// Build resource labels
	tags := core.TagSet{
		ContainerID: id,
		Backend:     "gcf",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
	}

	// Create the Cloud Run Function
	createReq := &functionspb.CreateFunctionRequest{
		Parent:     parent,
		FunctionId: funcName,
		Function: &functionspb.Function{
			Name: fullFunctionName,
			Labels: tags.AsGCPLabels(),
			BuildConfig: &functionspb.BuildConfig{
				Runtime:    "docker",
				EntryPoint: "",
			},
			ServiceConfig: serviceConfig,
		},
	}

	op, err := s.gcp.Functions.CreateFunction(s.ctx(), createReq)
	if err != nil {
		s.Logger.Error().Err(err).Str("function", funcName).Msg("failed to create Cloud Run Function")
		core.WriteError(w, mapGCPError(err, "function", funcName))
		return
	}

	result, err := op.Wait(s.ctx())
	if err != nil {
		s.Logger.Error().Err(err).Str("function", funcName).Msg("failed to wait for Cloud Run Function creation")
		core.WriteError(w, mapGCPError(err, "function", funcName))
		return
	}

	// Get function URL from the result
	functionURL := ""
	if result.ServiceConfig != nil {
		functionURL = result.ServiceConfig.Uri
	}

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "gcf",
		ResourceType: "function",
		ResourceID:   fullFunctionName,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": container.Image, "name": container.Name, "functionName": funcName},
	})

	s.Store.Containers.Put(id, container)
	s.Store.ContainerNames.Put(name, id)
	s.GCF.Put(id, GCFState{
		FunctionName: funcName,
		FunctionURL:  functionURL,
		LogResource:  funcName,
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
			Message: "multi-container pods are not supported by the cloudrun-functions backend",
		})
		return
	}

	gcfState, _ := s.GCF.Get(id)

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
	if !core.IsTailDevNull(c.Config.Entrypoint, c.Config.Cmd) {
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

	// Invoke function via HTTP trigger asynchronously
	go func() {
		exitCode := 0

		if gcfState.FunctionURL != "" {
			resp, err := http.Post(gcfState.FunctionURL, "application/json", nil)
			if err != nil {
				s.Logger.Error().Err(err).Str("function", gcfState.FunctionName).Msg("function invocation failed")
				exitCode = 1
			} else {
				resp.Body.Close()
				if resp.StatusCode >= 400 {
					s.Logger.Warn().Int("status", resp.StatusCode).Str("function", gcfState.FunctionName).Msg("function returned error status")
					exitCode = 1
				}
			}
		} else {
			s.Logger.Error().Str("function", gcfState.FunctionName).Msg("no function URL available for invocation")
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
				c.AgentToken = gcfState.AgentToken
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

	// Cloud Run Functions run to completion — stop is a no-op
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

	// Delete Cloud Run Function (best-effort)
	gcfState, _ := s.GCF.Get(id)
	if gcfState.FunctionName != "" {
		fullName := fmt.Sprintf("projects/%s/locations/%s/functions/%s", s.config.Project, s.config.Region, gcfState.FunctionName)
		op, err := s.gcp.Functions.DeleteFunction(s.ctx(), &functionspb.DeleteFunctionRequest{
			Name: fullName,
		})
		if err == nil {
			_ = op.Wait(s.ctx()) // best-effort wait
		}
		s.Registry.MarkCleanedUp(fullName)
	}

	s.Store.Containers.Delete(id)
	s.Store.ContainerNames.Delete(c.Name)
	s.GCF.Delete(id)
	s.Store.WaitChs.Delete(id)

	w.WriteHeader(http.StatusNoContent)
}
