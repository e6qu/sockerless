package core

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// hasEnvKey checks whether an env slice already contains a variable with the given key.
func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}

// --- Default overridable container handlers (memory-like behavior) ---

func (s *BaseServer) handleContainerCreate(w http.ResponseWriter, r *http.Request) {
	var req api.ContainerCreateRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		name = "/" + GenerateName()
	} else if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}

	// Check name conflict
	if _, exists := s.Store.ContainerNames.Get(name); exists {
		WriteError(w, &api.ConflictError{
			Message: fmt.Sprintf("Conflict. The container name \"%s\" is already in use", strings.TrimPrefix(name, "/")),
		})
		return
	}

	id := GenerateID()

	// Build config from the embedded ContainerConfig
	config := api.ContainerConfig{}
	if req.ContainerConfig != nil {
		config = *req.ContainerConfig
	}

	// Merge image config if we have it
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

	// Pod association via query param (Podman convention: ?pod=<nameOrID>)
	// Validate BEFORE storing the container to avoid leaking on failure.
	if podRef := r.URL.Query().Get("pod"); podRef != "" {
		if _, ok := s.Store.Pods.GetPod(podRef); !ok {
			WriteError(w, &api.NotFoundError{Resource: "pod", ID: podRef})
			return
		}
	}

	container := s.buildContainerFromConfig(id, name, config, hostConfig, req.NetworkingConfig)
	s.Store.Containers.Put(id, container)
	s.Store.ContainerNames.Put(name, id)

	// Inject build context files (from docker build COPY instructions)
	if img, ok := s.Store.ResolveImage(config.Image); ok {
		if ctxDir, ok := s.Store.BuildContexts.Load(img.ID); ok {
			s.Store.StagingDirs.Store(id, ctxDir.(string))
		}
	}

	// Pod association — pod was validated above
	if podRef := r.URL.Query().Get("pod"); podRef != "" {
		pod, _ := s.Store.Pods.GetPod(podRef)
		_ = s.Store.Pods.AddContainer(pod.ID, id)
	}

	// Implicit pod grouping: NetworkMode "container:<id>" joins the referenced
	// container's pod (or creates one if it doesn't exist yet)
	if strings.HasPrefix(hostConfig.NetworkMode, "container:") {
		refID := strings.TrimPrefix(hostConfig.NetworkMode, "container:")
		refID, _ = s.Store.ResolveContainerID(refID)
		if _, inPod := s.Store.Pods.GetPodForContainer(id); !inPod {
			if pod, exists := s.Store.Pods.GetPodForContainer(refID); exists {
				_ = s.Store.Pods.AddContainer(pod.ID, id)
			} else {
				short := refID
				if len(short) > 12 {
					short = short[:12]
				}
				pod := s.Store.Pods.CreatePod("container-"+short, nil)
				_ = s.Store.Pods.AddContainer(pod.ID, refID)
				_ = s.Store.Pods.AddContainer(pod.ID, id)
			}
		}
	}

	// Implicit pod grouping: containers sharing a user-defined network form a pod
	if _, inPod := s.Store.Pods.GetPodForContainer(id); !inPod {
		for netName := range container.NetworkSettings.Networks {
			if netName == "bridge" || netName == "host" || netName == "none" || netName == "default" {
				continue
			}
			if pod, exists := s.Store.Pods.GetPodForNetwork(netName); exists {
				_ = s.Store.Pods.AddContainer(pod.ID, id)
				break
			}
			// Check if the network already has other containers
			if net, netExists := s.Store.ResolveNetwork(netName); netExists && len(net.Containers) > 1 {
				// More than just this container on the network — create implicit pod
				podName := "net-" + netName
				pod := s.Store.Pods.CreatePod(podName, nil)
				s.Store.Pods.SetNetwork(pod.ID, netName)
				for existingID := range net.Containers {
					if existingID == id {
						continue
					}
					if _, alreadyInPod := s.Store.Pods.GetPodForContainer(existingID); !alreadyInPod {
						_ = s.Store.Pods.AddContainer(pod.ID, existingID)
					}
				}
				_ = s.Store.Pods.AddContainer(pod.ID, id)
				break
			}
		}
	}

	s.emitEvent("container", "create", id, map[string]string{
		"name":  strings.TrimPrefix(name, "/"),
		"image": config.Image,
	})

	WriteJSON(w, http.StatusCreated, api.ContainerCreateResponse{
		ID:       id,
		Warnings: []string{},
	})
}

func (s *BaseServer) handleContainerStart(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)
	if c.State.Running {
		WriteError(w, &api.NotModifiedError{})
		return
	}

	// Multi-container pods are not supported by the memory backend
	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod && len(pod.ContainerIDs) > 1 {
		WriteError(w, &api.InvalidParameterError{
			Message: "multi-container pods are not supported by the memory backend",
		})
		return
	}

	// Inject HOSTNAME and user env vars into container config (guard against duplicates on re-start)
	if c.Config.Hostname != "" {
		s.Store.Containers.Update(id, func(c *api.Container) {
			if !hasEnvKey(c.Config.Env, "HOSTNAME") {
				c.Config.Env = append(c.Config.Env, "HOSTNAME="+c.Config.Hostname)
			}
		})
	}
	if c.Config.User != "" {
		parts := strings.SplitN(c.Config.User, ":", 2)
		s.Store.Containers.Update(id, func(c *api.Container) {
			if !hasEnvKey(c.Config.Env, "SOCKERLESS_UID") {
				c.Config.Env = append(c.Config.Env, "SOCKERLESS_UID="+parts[0])
				if len(parts) == 2 {
					c.Config.Env = append(c.Config.Env, "SOCKERLESS_GID="+parts[1])
				}
			}
		})
	}

	// Inject ExtraHosts env var
	if len(c.HostConfig.ExtraHosts) > 0 {
		s.Store.Containers.Update(id, func(c *api.Container) {
			if !hasEnvKey(c.Config.Env, "SOCKERLESS_EXTRA_HOSTS") {
				c.Config.Env = append(c.Config.Env, "SOCKERLESS_EXTRA_HOSTS="+FormatExtraHostsEnv(c.HostConfig.ExtraHosts))
			}
		})
	}

	// Re-read container after env updates
	c, _ = s.Store.Containers.Get(id)

	// Process execution: spawn via ProcessLifecycleDriver
	cmd := append([]string{c.Path}, c.Args...)
	binds := s.resolveBindMounts(c.HostConfig.Binds, c.HostConfig.Mounts)
	tmpfs := resolveTmpfsMounts(c.HostConfig.Tmpfs)
	if len(tmpfs) > 0 {
		dirs := make([]string, 0, len(tmpfs))
		for _, v := range tmpfs {
			dirs = append(dirs, v)
		}
		s.Store.TmpfsDirs.Store(id, dirs)
	}
	for k, v := range tmpfs {
		if binds == nil {
			binds = make(map[string]string)
		}
		binds[k] = v
	}
	started, err := s.Drivers.ProcessLifecycle.Start(id, cmd, c.Config.Env, binds)
	if err != nil {
		s.Logger.Error().Err(err).Str("container", id).Msg("failed to start container process")
		WriteError(w, &api.ServerError{Message: "failed to start container process: " + err.Error()})
		return
	}

	// Start() succeeded — now create wait channel and update state
	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.Store.Containers.Update(id, func(c *api.Container) {
		c.State.Status = "running"
		c.State.Running = true
		c.State.Pid = 42
		c.State.StartedAt = now
		c.State.FinishedAt = "0001-01-01T00:00:00Z"
		c.State.ExitCode = 0
	})

	// Re-fetch after state update for accurate event data (BUG-208)
	c, _ = s.Store.Containers.Get(id)

	s.emitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})

	// Start health check if configured
	if c.Config.Healthcheck != nil && len(c.Config.Healthcheck.Test) > 0 &&
		(len(c.Config.Healthcheck.Test) != 1 || !strings.EqualFold(c.Config.Healthcheck.Test[0], "NONE")) {
		s.StartHealthCheck(id)
	}

	if started {
		// Merge pre-start archive files (from docker cp before start)
		if rootPath, err := s.Drivers.Filesystem.RootPath(id); err == nil && rootPath != "" {
			s.mergeStagingDir(id, rootPath)

			// Write /etc/hosts with extra hosts + peer DNS
			extraHosts := c.HostConfig.ExtraHosts
			peerHosts := ResolvePeerHosts(s.Store, id)
			allHosts := append(extraHosts, peerHosts...)
			if len(allHosts) > 0 || c.Config.Hostname != "" {
				hostsContent := BuildHostsFile(c.Config.Hostname, allHosts)
				etcDir := joinCleanPath(rootPath, "/etc")
				_ = os.MkdirAll(etcDir, 0o755)
				_ = os.WriteFile(joinCleanPath(rootPath, "/etc/hosts"), hostsContent, 0o644)
			}
		}

		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Store synthetic log output
	logMsg := ""
	if c.Path != "" {
		logMsg = fmt.Sprintf("executing: %s %s\n", c.Path, strings.Join(c.Args, " "))
	}
	s.Store.LogBuffers.Store(id, []byte(logMsg))

	// Synthetic mode: no process was spawned, so auto-exit the container.
	// Non-interactive containers exit immediately; interactive (OpenStdin)
	// containers exit after a brief delay to allow attach to send log data.
	delay := 50 * time.Millisecond
	if c.Config.OpenStdin {
		delay = 200 * time.Millisecond
	}
	go func() {
		time.Sleep(delay)
		if c, ok := s.Store.Containers.Get(id); ok && c.State.Running {
			s.emitEvent("container", "die", id, map[string]string{
				"exitCode": "0",
				"name":     strings.TrimPrefix(c.Name, "/"),
			})
			s.Store.StopContainer(id, 0)
		}
	}()

	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleContainerStop(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		WriteError(w, &api.NotModifiedError{})
		return
	}

	s.StopHealthCheck(id)
	s.Drivers.ProcessLifecycle.Stop(id)
	s.Drivers.ProcessLifecycle.Cleanup(id)
	s.Store.ForceStopContainer(id, 0)
	s.emitEvent("container", "die", id, map[string]string{
		"exitCode": "0",
		"name":     strings.TrimPrefix(c.Name, "/"),
	})
	s.emitEvent("container", "stop", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleContainerKill(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		WriteError(w, &api.ConflictError{
			Message: fmt.Sprintf("Container %s is not running", ref),
		})
		return
	}

	s.Drivers.ProcessLifecycle.Kill(id)
	s.StopHealthCheck(id)

	signal := r.URL.Query().Get("signal")
	exitCode := signalToExitCode(signal)

	s.Store.Containers.Update(id, func(c *api.Container) {
		c.State.Status = "exited"
		c.State.Running = false
		c.State.Pid = 0
		c.State.ExitCode = exitCode
		c.State.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	})

	s.Drivers.ProcessLifecycle.Cleanup(id) // BUG-159: clean up WASM resources
	s.emitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	s.emitEvent("container", "die", id, map[string]string{
		"exitCode": fmt.Sprintf("%d", exitCode),
		"signal":   signal,
		"name":     strings.TrimPrefix(c.Name, "/"),
	})

	// Close wait channel after emitting events so watchers see events first
	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleContainerRemove(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	c, _ := s.Store.Containers.Get(id)

	if c.State.Running && !force {
		WriteError(w, &api.ConflictError{
			Message: fmt.Sprintf("You cannot remove a running container %s. Stop the container before attempting removal or force remove", id[:12]),
		})
		return
	}

	if c.State.Running {
		s.emitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		s.emitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		s.Store.ForceStopContainer(id, 0)
	}

	s.StopHealthCheck(id)

	// Clean up container process
	s.Drivers.ProcessLifecycle.Cleanup(id)

	// Clean up network associations via driver (allows Linux driver to remove veth pairs)
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID != "" {
			_ = s.Drivers.Network.Disconnect(r.Context(), ep.NetworkID, id)
		}
	}

	// Clean up pod registry
	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod {
		s.Store.Pods.RemoveContainer(pod.ID, id)
	}

	s.Store.Containers.Delete(id)
	s.Store.ContainerNames.Delete(c.Name)
	s.Store.LogBuffers.Delete(id)
	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
	s.Store.StagingDirs.Delete(id)
	if dirs, ok := s.Store.TmpfsDirs.LoadAndDelete(id); ok {
		for _, d := range dirs.([]string) {
			os.RemoveAll(d)
		}
	}
	for _, eid := range c.ExecIDs {
		s.Store.Execs.Delete(eid)
	}

	s.emitEvent("container", "destroy", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleContainerRestart(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)
	if c.State.Running {
		s.StopHealthCheck(id)
		s.Drivers.ProcessLifecycle.Stop(id)
		s.Store.ForceStopContainer(id, 0)
		s.emitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		s.emitEvent("container", "stop", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	}

	// Clean up old process
	s.Drivers.ProcessLifecycle.Cleanup(id)

	// Re-start
	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.Store.Containers.Update(id, func(c *api.Container) {
		c.State.Status = "running"
		c.State.Running = true
		c.State.Pid = 42
		c.State.StartedAt = now
		c.State.FinishedAt = "0001-01-01T00:00:00Z"
		c.State.ExitCode = 0
		c.RestartCount++
	})

	// Re-fetch fresh container after state update
	c, _ = s.Store.Containers.Get(id)

	// Clean up old tmpfs dirs before creating new ones
	if dirs, ok := s.Store.TmpfsDirs.LoadAndDelete(id); ok {
		for _, d := range dirs.([]string) {
			os.RemoveAll(d)
		}
	}

	// Re-spawn process (wait-and-stop goroutine is handled by the driver)
	cmd := append([]string{c.Path}, c.Args...)
	binds := s.resolveBindMounts(c.HostConfig.Binds, c.HostConfig.Mounts)
	tmpfs := resolveTmpfsMounts(c.HostConfig.Tmpfs)
	if len(tmpfs) > 0 {
		dirs := make([]string, 0, len(tmpfs))
		for _, v := range tmpfs {
			dirs = append(dirs, v)
		}
		s.Store.TmpfsDirs.Store(id, dirs)
	}
	for k, v := range tmpfs {
		if binds == nil {
			binds = make(map[string]string)
		}
		binds[k] = v
	}
	_, err := s.Drivers.ProcessLifecycle.Start(id, cmd, c.Config.Env, binds)
	if err != nil {
		s.Logger.Error().Err(err).Str("container", id).Msg("failed to restart container process")
		if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
			close(ch.(chan struct{}))
		}
		s.StopHealthCheck(id)
		s.Store.RevertToCreated(id)
		WriteError(w, &api.ServerError{Message: "failed to restart container process: " + err.Error()})
		return
	}

	s.emitEvent("container", "start", id, map[string]string{
		"name": strings.TrimPrefix(c.Name, "/"),
	})
	s.emitEvent("container", "restart", id, map[string]string{
		"name": strings.TrimPrefix(c.Name, "/"),
	})

	// Re-start health check if configured
	c, _ = s.Store.Containers.Get(id)
	if c.Config.Healthcheck != nil && len(c.Config.Healthcheck.Test) > 0 &&
		(len(c.Config.Healthcheck.Test) != 1 || !strings.EqualFold(c.Config.Healthcheck.Test[0], "NONE")) {
		s.StartHealthCheck(id)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleContainerWait(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)

	// BUG-384: Read condition query parameter (not-running, next-exit, removed)
	condition := r.URL.Query().Get("condition")
	if condition == "" {
		condition = "not-running"
	}

	// If already exited, return immediately (unless next-exit which waits for a new exit)
	if condition != "next-exit" && (c.State.Status == "exited" || c.State.Status == "dead") {
		WriteJSON(w, http.StatusOK, api.ContainerWaitResponse{
			StatusCode: c.State.ExitCode,
		})
		return
	}

	// Block until exit. Auto-stop is handled by:
	// - handleContainerStart: 50ms for non-interactive (!Tty && !OpenStdin)
	// - below: 2s for interactive containers with no execs (predefined/helper)
	// - handleExecStart: 500ms after all synthetic execs complete (build)
	ch, ok := s.Store.WaitChs.Load(id)
	if !ok {
		c, _ = s.Store.Containers.Get(id)
		WriteJSON(w, http.StatusOK, api.ContainerWaitResponse{
			StatusCode: c.State.ExitCode,
		})
		return
	}

	// For synthetic (agentless) interactive containers, auto-stop after 2s
	// if no execs have been created. This handles CI runner "predefined"
	// containers whose helper process would exit quickly in real Docker.
	// Build containers get execs within ms, so they won't be affected.
	if c.AgentAddress == "" && (c.Config.Tty || c.Config.OpenStdin) {
		go func() {
			time.Sleep(2 * time.Second)
			c2, ok := s.Store.Containers.Get(id)
			if !ok || !c2.State.Running || len(c2.ExecIDs) > 0 {
				return
			}
			s.Store.StopContainer(id, 0)
		}()
	}

	select {
	case <-ch.(chan struct{}):
		c, _ = s.Store.Containers.Get(id)
		WriteJSON(w, http.StatusOK, api.ContainerWaitResponse{
			StatusCode: c.State.ExitCode,
		})
	case <-r.Context().Done():
		return
	}
}

// signalToExitCode maps a signal name or number to the corresponding
// exit code (128 + signal number), matching Docker's behavior.
func signalToExitCode(signal string) int {
	signalMap := map[string]int{
		"SIGHUP": 129, "HUP": 129, "1": 129,
		"SIGINT": 130, "INT": 130, "2": 130,
		"SIGQUIT": 131, "QUIT": 131, "3": 131,
		"SIGABRT": 134, "ABRT": 134, "6": 134,
		"SIGKILL": 137, "KILL": 137, "9": 137,
		"SIGUSR1": 138, "USR1": 138, "10": 138,
		"SIGUSR2": 140, "USR2": 140, "12": 140,
		"SIGTERM": 143, "TERM": 143, "15": 143,
	}
	if code, ok := signalMap[signal]; ok {
		return code
	}
	if n, err := strconv.Atoi(signal); err == nil && n > 0 {
		return 128 + n
	}
	return 137 // default to SIGKILL
}

