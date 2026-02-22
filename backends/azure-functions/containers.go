package azf

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v4"
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
		Driver:   "azure-functions",
	}

	// Function App names must be globally unique -- use skls- prefix + truncated container ID
	funcAppName := "skls-" + id[:12]

	// Build environment variables for App Settings
	envVars := make(map[string]string)
	for _, e := range config.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envVars[parts[0]] = parts[1]
		}
	}

	// Build App Settings
	appSettings := []*armappservice.NameValuePair{
		{Name: ptr("FUNCTIONS_EXTENSION_VERSION"), Value: ptr("~4")},
		{Name: ptr("WEBSITES_ENABLE_APP_SERVICE_STORAGE"), Value: ptr("false")},
		{Name: ptr("AzureWebJobsStorage"), Value: ptr(fmt.Sprintf("DefaultEndpointsProtocol=https;AccountName=%s;EndpointSuffix=core.windows.net", s.config.StorageAccount))},
	}

	if s.config.Registry != "" {
		appSettings = append(appSettings, &armappservice.NameValuePair{
			Name: ptr("DOCKER_REGISTRY_SERVER_URL"), Value: ptr(s.config.Registry),
		})
	}

	// Add user environment variables as App Settings
	for k, v := range envVars {
		appSettings = append(appSettings, &armappservice.NameValuePair{
			Name: ptr(k), Value: ptr(v),
		})
	}

	// Generate agent token and build callback entrypoint if configured
	agentToken := ""
	var startupCommand string
	if s.config.CallbackURL != "" {
		agentToken = core.GenerateToken()
		callbackURL := fmt.Sprintf("%s/internal/v1/agent/connect?id=%s&token=%s", s.config.CallbackURL, id, agentToken)
		agentEntrypoint := core.BuildAgentCallbackEntrypoint(config, callbackURL)
		startupCommand = strings.Join(agentEntrypoint, " ")

		appSettings = append(appSettings,
			&armappservice.NameValuePair{Name: ptr("SOCKERLESS_CONTAINER_ID"), Value: ptr(id)},
			&armappservice.NameValuePair{Name: ptr("SOCKERLESS_AGENT_TOKEN"), Value: ptr(agentToken)},
			&armappservice.NameValuePair{Name: ptr("SOCKERLESS_AGENT_CALLBACK_URL"), Value: ptr(callbackURL)},
		)
	}

	// Build the Function App Site resource
	siteConfig := &armappservice.SiteConfig{
		LinuxFxVersion: ptr("DOCKER|" + config.Image),
		AppSettings:    appSettings,
	}
	if startupCommand != "" {
		siteConfig.AppCommandLine = ptr(startupCommand)
	}

	// Build resource tags
	tags := core.TagSet{
		ContainerID: id,
		Backend:     "azf",
		InstanceID:  s.Desc.InstanceID,
		CreatedAt:   time.Now(),
	}

	site := armappservice.Site{
		Location: ptr(s.config.Location),
		Kind:     ptr("functionapp,linux,container"),
		Tags:     tags.AsAzurePtrMap(),
		Properties: &armappservice.SiteProperties{
			SiteConfig: siteConfig,
		},
	}

	if s.config.AppServicePlan != "" {
		site.Properties.ServerFarmID = ptr(s.config.AppServicePlan)
	}

	// Create Function App
	poller, err := s.azure.WebApps.BeginCreateOrUpdate(s.ctx(), s.config.ResourceGroup, funcAppName, site, nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("functionApp", funcAppName).Msg("failed to create Function App")
		core.WriteError(w, mapAzureError(err, "function app", funcAppName))
		return
	}

	result, err := poller.PollUntilDone(s.ctx(), nil)
	if err != nil {
		s.Logger.Error().Err(err).Str("functionApp", funcAppName).Msg("Function App creation failed")
		core.WriteError(w, mapAzureError(err, "function app", funcAppName))
		return
	}

	resourceID := ""
	if result.ID != nil {
		resourceID = *result.ID
	}

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "azf",
		ResourceType: "site",
		ResourceID:   resourceID,
		InstanceID:   s.Desc.InstanceID,
		CreatedAt:    time.Now(),
	})

	functionURL := ""
	if result.Properties != nil && result.Properties.DefaultHostName != nil {
		scheme := "https"
		if s.config.EndpointURL != "" {
			scheme = "http"
		}
		functionURL = fmt.Sprintf("%s://%s/api/function", scheme, *result.Properties.DefaultHostName)
	}

	s.Store.Containers.Put(id, container)
	s.Store.ContainerNames.Put(name, id)
	s.AZF.Put(id, AZFState{
		FunctionAppName: funcAppName,
		ResourceID:      resourceID,
		FunctionURL:     functionURL,
		AgentToken:      agentToken,
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

	azfState, _ := s.AZF.Get(id)

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

	// Trigger the Function App via HTTP POST in a goroutine
	go func() {
		exitCode := 0

		if azfState.FunctionURL != "" {
			client := &http.Client{Timeout: time.Duration(s.config.Timeout) * time.Second}
			resp, err := client.Post(azfState.FunctionURL, "application/json", strings.NewReader("{}"))
			if err != nil {
				s.Logger.Error().Err(err).Str("functionApp", azfState.FunctionAppName).Msg("Function App invocation failed")
				exitCode = 1
			} else {
				resp.Body.Close()
				if resp.StatusCode >= 400 {
					s.Logger.Warn().Int("status", resp.StatusCode).Str("functionApp", azfState.FunctionAppName).Msg("Function App returned error")
					exitCode = 1
				}
			}
		} else {
			s.Logger.Warn().Str("functionApp", azfState.FunctionAppName).Msg("no function URL available, cannot invoke")
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
				c.AgentToken = azfState.AgentToken
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

	// Azure Functions run to completion -- stop is a no-op
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

	// Delete Function App (best-effort)
	azfState, _ := s.AZF.Get(id)
	if azfState.FunctionAppName != "" {
		_, err := s.azure.WebApps.Delete(s.ctx(), s.config.ResourceGroup, azfState.FunctionAppName, nil)
		if err != nil {
			s.Logger.Debug().Err(err).Str("functionApp", azfState.FunctionAppName).Msg("failed to delete Function App")
		}
	}

	if azfState.ResourceID != "" {
		s.Registry.MarkCleanedUp(azfState.ResourceID)
	}

	s.Store.Containers.Delete(id)
	s.Store.ContainerNames.Delete(c.Name)
	s.AZF.Delete(id)
	s.Store.WaitChs.Delete(id)

	w.WriteHeader(http.StatusNoContent)
}

func ptr(s string) *string {
	return &s
}
