package gcf

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/logging/logadmin"
	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	"google.golang.org/api/iterator"
)

// Compile-time check that Server implements api.Backend.
var _ api.Backend = (*Server)(nil)

// ContainerCreate creates a container backed by a Cloud Run Function.
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
		Driver:   "cloud-run-functions",
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
	} else {
		// Pass command via environment variable (cloud-native) for short-lived containers
		cmd := core.BuildOriginalCommand(config.Entrypoint, config.Cmd)
		if len(cmd) > 0 {
			cmdJSON, _ := json.Marshal(cmd)
			envVars["SOCKERLESS_CMD"] = base64.StdEncoding.EncodeToString(cmdJSON)
		}
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
			Name:   fullFunctionName,
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
		return nil, mapGCPError(err, "function", funcName)
	}

	result, err := op.Wait(s.ctx())
	if err != nil {
		// Best-effort: delete potentially-created function
		if delOp, delErr := s.gcp.Functions.DeleteFunction(s.ctx(), &functionspb.DeleteFunctionRequest{
			Name: fullFunctionName,
		}); delErr == nil {
			_ = delOp.Wait(s.ctx())
		}
		s.Logger.Error().Err(err).Str("function", funcName).Msg("failed to wait for Cloud Run Function creation")
		return nil, mapGCPError(err, "function", funcName)
	}

	// Get function URL from the result
	functionURL := ""
	if result.ServiceConfig != nil {
		functionURL = result.ServiceConfig.Uri
	}

	s.Store.Containers.Put(id, container)
	s.Store.ContainerNames.Put(name, id)

	s.GCF.Put(id, GCFState{
		FunctionName: funcName,
		FunctionURL:  functionURL,
		LogResource:  funcName,
		AgentToken:   agentToken,
	})

	s.Registry.Register(core.ResourceEntry{
		ContainerID:  id,
		Backend:      "gcf",
		ResourceType: "function",
		ResourceID:   fullFunctionName,
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

// ContainerStart starts a Cloud Run Function invocation for the container.
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
			Message: "multi-container pods are not supported by the cloudrun-functions backend",
		}
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

	c, _ = s.Store.Containers.Get(id)
	s.EmitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})

	// Non-tail-dev-null containers: invoke the function with the container's command
	// to get real execution, then stop with the real exit code.
	if !core.IsTailDevNull(c.Config.Entrypoint, c.Config.Cmd) {
		cmd := core.BuildOriginalCommand(c.Config.Entrypoint, c.Config.Cmd)
		if len(cmd) > 0 && gcfState.FunctionURL != "" {
			go func() {
				exitCode := 0
				resp, err := gcfHTTPClient.Post(gcfState.FunctionURL, "application/json", nil)
				if err != nil {
					s.Logger.Error().Err(err).Str("function", gcfState.FunctionName).Msg("function invocation failed")
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

	// Invoke function via HTTP trigger asynchronously
	go func() {
		exitCode := 0

		if gcfState.FunctionURL != "" {
			resp, err := gcfHTTPClient.Post(gcfState.FunctionURL, "application/json", nil)
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
				c.AgentToken = gcfState.AgentToken
			})
		}
	}

	return nil
}

// ContainerStop stops a running Cloud Run Function container.
func (s *Server) ContainerStop(ref string, timeout *int) error {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	// Cloud Run Functions run to completion — stop transitions state
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

// ContainerRemove removes a container and its associated Cloud Run Function resources.
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
	s.GCF.Delete(id)
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

// ContainerLogs streams container logs from Cloud Logging.
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

	gcfState, _ := s.GCF.Get(id)
	funcName := gcfState.FunctionName

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

		// Build filter for Cloud Logging — Cloud Run Functions 2nd gen run as Cloud Run services
		baseFilter := fmt.Sprintf(
			`resource.type="cloud_run_revision" AND resource.labels.service_name="%s"`,
			funcName,
		)

		// BUG-423, BUG-424: Apply since/until to initial query
		initialFilter := baseFilter
		initialFilter += params.CloudLoggingSinceFilter()
		initialFilter += params.CloudLoggingUntilFilter()

		ctx, cancel := context.WithTimeout(s.ctx(), s.config.LogTimeout)
		defer cancel()

		it := s.gcp.LogAdmin.Entries(ctx, logadmin.Filter(initialFilter))

		// BUG-425: Collect entries for tail support
		type logEntry struct {
			line string
			ts   time.Time
		}
		var entries []logEntry
		var lastTimestamp time.Time

		for {
			entry, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				s.Logger.Debug().Err(err).Msg("failed to read log entry")
				break
			}

			line := extractLogLine(entry)
			if line == "" {
				continue
			}

			entries = append(entries, logEntry{line: line, ts: entry.Timestamp})

			if entry.Timestamp.After(lastTimestamp) {
				lastTimestamp = entry.Timestamp
			}
		}

		// BUG-425: Apply tail
		if params.Tail >= 0 && params.Tail < len(entries) {
			entries = entries[len(entries)-params.Tail:]
		}

		for _, e := range entries {
			formatted := params.FormatLine(e.line, e.ts) // BUG-427: details + timestamps
			if _, err := pw.Write([]byte(formatted)); err != nil {
				return
			}
		}

		// BUG-429: Follow mode support
		if !params.Follow {
			return
		}

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			c, _ := s.Store.Containers.Get(id)
			if !c.State.Running && c.State.Status != "created" {
				s.fetchFollowLogsPipe(pw, baseFilter, lastTimestamp, params, &lastTimestamp)
				return
			}
			s.fetchFollowLogsPipe(pw, baseFilter, lastTimestamp, params, &lastTimestamp)
		}
	}()

	return pr, nil
}

// fetchFollowLogsPipe queries Cloud Logging for entries after lastTS, writing raw text to a pipe writer.
func (s *Server) fetchFollowLogsPipe(pw *io.PipeWriter, baseFilter string, after time.Time, params core.CloudLogParams, lastTS *time.Time) {
	logFilter := baseFilter
	if !after.IsZero() {
		logFilter += fmt.Sprintf(` AND timestamp>"%s"`, after.UTC().Format(time.RFC3339Nano))
	}

	ctx, cancel := context.WithTimeout(s.ctx(), s.config.LogTimeout)
	defer cancel()

	it := s.gcp.LogAdmin.Entries(ctx, logadmin.Filter(logFilter))

	for {
		entry, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			s.Logger.Debug().Err(err).Msg("failed to read log entry")
			break
		}

		line := extractLogLine(entry)
		if line == "" {
			continue
		}

		formatted := params.FormatLine(line, entry.Timestamp)
		if _, err := pw.Write([]byte(formatted)); err != nil {
			return
		}

		if entry.Timestamp.After(*lastTS) {
			*lastTS = entry.Timestamp
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

// ContainerPrune removes all stopped containers and their GCF state.
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
		// BUG-482: Sum image sizes for SpaceReclaimed
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			spaceReclaimed += uint64(img.Size)
		}
		// Clean up Cloud Run Functions cloud resources
		gcfState, _ := s.GCF.Get(c.ID)
		if gcfState.FunctionName != "" {
			fullName := fmt.Sprintf("projects/%s/locations/%s/functions/%s", s.config.Project, s.config.Region, gcfState.FunctionName)
			if op, err := s.gcp.Functions.DeleteFunction(s.ctx(), &functionspb.DeleteFunctionRequest{
				Name: fullName,
			}); err == nil {
				_ = op.Wait(s.ctx())
			}
			s.Registry.MarkCleanedUp(fullName)
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
		s.GCF.Delete(c.ID)
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

// ContainerPause is not supported by the Cloud Run Functions backend.
func (s *Server) ContainerPause(ref string) error {
	_, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "Cloud Run Functions backend does not support pause"}
}

// ContainerUnpause is not supported by the Cloud Run Functions backend.
func (s *Server) ContainerUnpause(ref string) error {
	_, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &api.NotImplementedError{Message: "Cloud Run Functions backend does not support unpause"}
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

// ImageLoad is not supported by the Cloud Run Functions backend.
func (s *Server) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	return nil, &api.NotImplementedError{Message: "image load is not supported by Cloud Run Functions backend"}
}
