package azf

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v4"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// Compile-time check that Server implements api.Backend.
var _ api.Backend = (*Server)(nil)

// ContainerCreate creates a container backed by an Azure Function App.
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
		Driver:   "azure-functions",
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
	} else {
		// Pass command via app setting (cloud-native) for short-lived containers
		cmd := core.BuildOriginalCommand(config.Entrypoint, config.Cmd)
		if len(cmd) > 0 {
			cmdJSON, _ := json.Marshal(cmd)
			appSettings = append(appSettings, &armappservice.NameValuePair{
				Name:  ptr("SOCKERLESS_CMD"),
				Value: ptr(base64.StdEncoding.EncodeToString(cmdJSON)),
			})
		}
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
		return nil, mapAzureError(err, "function app", funcAppName)
	}

	result, err := poller.PollUntilDone(s.ctx(), nil)
	if err != nil {
		// Best-effort: delete potentially-created Function App
		_, _ = s.azure.WebApps.Delete(s.ctx(), s.config.ResourceGroup, funcAppName, nil)
		s.Logger.Error().Err(err).Str("functionApp", funcAppName).Msg("Function App creation failed")
		return nil, mapAzureError(err, "function app", funcAppName)
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
		Metadata:     map[string]string{"image": container.Image, "name": container.Name, "functionAppName": funcAppName},
	})

	functionURL := ""
	if result.Properties != nil && result.Properties.DefaultHostName != nil {
		scheme := "https"
		if strings.HasPrefix(s.config.EndpointURL, "http://") {
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

	s.EmitEvent("container", "create", id, map[string]string{
		"name":  strings.TrimPrefix(name, "/"),
		"image": config.Image,
	})

	return &api.ContainerCreateResponse{
		ID:       id,
		Warnings: []string{},
	}, nil
}

// ContainerStart starts a Function App invocation for the container.
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
			Message: "multi-container pods are not supported by the azure-functions backend",
		}
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

	c, _ = s.Store.Containers.Get(id)
	s.EmitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})

	// Non-tail-dev-null containers: invoke the function with the container's command
	// to get real execution, then stop with the real exit code.
	if !core.IsTailDevNull(c.Config.Entrypoint, c.Config.Cmd) {
		cmd := core.BuildOriginalCommand(c.Config.Entrypoint, c.Config.Cmd)
		if len(cmd) > 0 && azfState.FunctionURL != "" {
			go func() {
				exitCode := 0
				client := &http.Client{Timeout: time.Duration(s.config.Timeout) * time.Second}
				resp, err := client.Post(azfState.FunctionURL, "application/json", nil)
				if err != nil {
					s.Logger.Error().Err(err).Str("functionApp", azfState.FunctionAppName).Msg("function invocation failed")
					exitCode = 1
				} else {
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					if len(body) > 0 && string(body) != "{}" {
						if c, ok := s.Store.Containers.Get(id); ok && c.State.Running {
							s.Store.LogBuffers.Store(id, body)
						}
					}
					if resp.StatusCode >= 400 {
						exitCode = 1
					}
				}
				if c, ok := s.Store.Containers.Get(id); ok && c.State.Running {
					s.Store.StopContainer(id, exitCode)
				}
			}()
		} else {
			// No command or no function URL: auto-stop after brief delay
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
				c.AgentToken = azfState.AgentToken
			})
		}
	}

	return nil
}

// ContainerStop stops a running Azure Functions container.
func (s *Server) ContainerStop(ref string, timeout *int) error {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	// Azure Functions run to completion — stop transitions state
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

// ContainerRemove removes a container and its associated Azure Functions resources.
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
	s.AZF.Delete(id)
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

// ContainerLogs streams container logs from Azure Monitor.
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

	azfState, _ := s.AZF.Get(id)
	functionAppName := azfState.FunctionAppName
	if functionAppName == "" {
		functionAppName = "skls-" + id[:12]
	}

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

		if s.config.LogAnalyticsWorkspace == "" {
			return
		}

		// Build KQL query for Azure Monitor -- query AppTraces for the function app
		query := fmt.Sprintf(
			`AppTraces | where AppRoleName == "%s"`,
			functionAppName,
		)
		// BUG-423, BUG-424: Apply since/until to initial query
		query += params.KQLSinceFilter()
		query += params.KQLUntilFilter()
		query += ` | order by TimeGenerated asc | project TimeGenerated, Message`

		resp, err := s.azure.Logs.QueryWorkspace(s.ctx(), s.config.LogAnalyticsWorkspace, azquery.Body{
			Query: &query,
		}, nil)
		if err != nil {
			s.Logger.Debug().Err(err).Msg("failed to query logs")
			return
		}

		type logEntry struct {
			line string
			ts   time.Time
		}
		var entries []logEntry

		for _, table := range resp.Tables {
			// Find column indices
			timeIdx := -1
			msgIdx := -1
			for i, col := range table.Columns {
				if col.Name != nil {
					switch *col.Name {
					case "TimeGenerated":
						timeIdx = i
					case "Message":
						msgIdx = i
					}
				}
			}

			for _, row := range table.Rows {
				if msgIdx < 0 || msgIdx >= len(row) {
					continue
				}

				line, ok := row[msgIdx].(string)
				if !ok || line == "" {
					continue
				}

				var ts time.Time
				if timeIdx >= 0 && timeIdx < len(row) {
					if tsStr, ok := row[timeIdx].(string); ok {
						ts, _ = time.Parse(time.RFC3339Nano, tsStr)
					}
				}

				entries = append(entries, logEntry{line: line, ts: ts})
			}
		}

		// BUG-425: Apply tail
		if params.Tail >= 0 && params.Tail < len(entries) {
			entries = entries[len(entries)-params.Tail:]
		}

		for _, e := range entries {
			formatted := params.FormatLine(e.line, e.ts)
			if _, err := pw.Write([]byte(formatted)); err != nil {
				return
			}
		}

		// BUG-430: Follow mode support
		if !params.Follow {
			return
		}

		// Track last timestamp for follow dedup
		var lastTimestamp time.Time
		if len(entries) > 0 {
			lastTimestamp = entries[len(entries)-1].ts
		}

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			c, _ := s.Store.Containers.Get(id)
			if !c.State.Running && c.State.Status != "created" {
				s.fetchFollowLogsPipe(pw, functionAppName, lastTimestamp, params, &lastTimestamp)
				return
			}
			s.fetchFollowLogsPipe(pw, functionAppName, lastTimestamp, params, &lastTimestamp)
		}
	}()

	return pr, nil
}

// fetchFollowLogsPipe queries Azure Monitor for entries after lastTS and writes to a pipe.
func (s *Server) fetchFollowLogsPipe(pw *io.PipeWriter, functionAppName string, after time.Time, params core.CloudLogParams, lastTS *time.Time) {
	query := fmt.Sprintf(
		`AppTraces | where AppRoleName == "%s"`,
		functionAppName,
	)
	if !after.IsZero() {
		query += fmt.Sprintf(` | where TimeGenerated > datetime(%s)`, after.UTC().Format(time.RFC3339Nano))
	}
	query += ` | order by TimeGenerated asc | project TimeGenerated, Message`

	resp, err := s.azure.Logs.QueryWorkspace(s.ctx(), s.config.LogAnalyticsWorkspace, azquery.Body{
		Query: &query,
	}, nil)
	if err != nil {
		s.Logger.Debug().Err(err).Msg("failed to query follow logs")
		return
	}

	for _, table := range resp.Tables {
		timeIdx := -1
		msgIdx := -1
		for i, col := range table.Columns {
			if col.Name != nil {
				switch *col.Name {
				case "TimeGenerated":
					timeIdx = i
				case "Message":
					msgIdx = i
				}
			}
		}

		for _, row := range table.Rows {
			if msgIdx < 0 || msgIdx >= len(row) {
				continue
			}

			line, ok := row[msgIdx].(string)
			if !ok || line == "" {
				continue
			}

			var ts time.Time
			if timeIdx >= 0 && timeIdx < len(row) {
				if tsStr, ok := row[timeIdx].(string); ok {
					ts, _ = time.Parse(time.RFC3339Nano, tsStr)
				}
			}

			formatted := params.FormatLine(line, ts)
			if _, err := pw.Write([]byte(formatted)); err != nil {
				return
			}

			if !ts.IsZero() && ts.After(*lastTS) {
				*lastTS = ts
			}
		}
	}
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

// ContainerPrune removes all stopped containers and their AZF state.
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
		// BUG-483: Sum image sizes for SpaceReclaimed
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			spaceReclaimed += uint64(img.Size)
		}
		// Clean up Azure Functions cloud resources
		azfState, _ := s.AZF.Get(c.ID)
		if azfState.FunctionAppName != "" {
			_, _ = s.azure.WebApps.Delete(s.ctx(), s.config.ResourceGroup, azfState.FunctionAppName, nil)
		}
		if azfState.ResourceID != "" {
			s.Registry.MarkCleanedUp(azfState.ResourceID)
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
		s.AZF.Delete(c.ID)
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

// ContainerPause is not supported by the Azure Functions backend.
func (s *Server) ContainerPause(ref string) error {
	_, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "Azure Functions backend does not support pause"}
}

// ContainerUnpause is not supported by the Azure Functions backend.
func (s *Server) ContainerUnpause(ref string) error {
	_, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "Azure Functions backend does not support unpause"}
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

// Info returns system information enriched with Azure-specific metadata.
func (s *Server) Info() (*api.BackendInfo, error) {
	info, err := s.BaseServer.Info()
	if err != nil {
		return nil, err
	}

	// Enrich with Azure-specific context
	info.OperatingSystem = fmt.Sprintf("Azure Functions (%s)", s.config.Location)
	info.Name = fmt.Sprintf("sockerless-azf/%s/%s", s.config.SubscriptionID, s.config.ResourceGroup)

	return info, nil
}

// ContainerExport is not supported by the Azure Functions backend.
// Azure Functions have no local filesystem to export.
func (s *Server) ContainerExport(id string) (io.ReadCloser, error) {
	if _, ok := s.Store.ResolveContainerID(id); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: id}
	}
	return nil, &api.NotImplementedError{
		Message: "Azure Functions backend does not support container export; functions have no local filesystem",
	}
}

// ContainerCommit is not supported by the Azure Functions backend.
// Azure Functions have no local filesystem to commit.
func (s *Server) ContainerCommit(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error) {
	if req.Container == "" {
		return nil, &api.InvalidParameterError{Message: "container query parameter is required"}
	}
	if _, ok := s.Store.ResolveContainerID(req.Container); !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: req.Container}
	}
	return nil, &api.NotImplementedError{
		Message: "Azure Functions backend does not support container commit; functions have no local filesystem",
	}
}

// ContainerAttach is not supported by the Azure Functions backend without a connected agent.
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
		Message: "Azure Functions backend does not support attach without a connected agent",
	}
}

// ImageBuild is not supported by the Azure Functions backend.
// Azure Functions require pre-built container images.
func (s *Server) ImageBuild(opts api.ImageBuildOptions, buildContext io.Reader) (io.ReadCloser, error) {
	return nil, &api.NotImplementedError{
		Message: "Azure Functions backend does not support image build; push pre-built images to Azure Container Registry",
	}
}

// ImagePush is not supported by the Azure Functions backend.
// Images should be pushed directly to Azure Container Registry using the az CLI or SDK.
func (s *Server) ImagePush(name string, tag string, auth string) (io.ReadCloser, error) {
	return nil, &api.NotImplementedError{
		Message: "Azure Functions backend does not support image push; push images directly to Azure Container Registry",
	}
}

// ImageLoad is not supported by the Azure Functions backend.
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	return nil, &api.NotImplementedError{Message: "image load is not supported by Azure Functions backend"}
}
