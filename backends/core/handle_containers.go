package core

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sockerless/api"
)

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

	container := s.buildContainerFromConfig(id, name, config, hostConfig, req.NetworkingConfig)
	s.Store.Containers.Put(id, container)
	s.Store.ContainerNames.Put(name, id)

	// Inject build context files (from docker build COPY instructions)
	if img, ok := s.Store.ResolveImage(config.Image); ok {
		if ctxDir, ok := s.Store.BuildContexts.Load(img.ID); ok {
			s.Store.StagingDirs.Store(id, ctxDir.(string))
		}
	}

	// Pod association via query param (Podman convention: ?pod=<nameOrID>)
	if podRef := r.URL.Query().Get("pod"); podRef != "" {
		pod, ok := s.Store.Pods.GetPod(podRef)
		if !ok {
			WriteError(w, &api.NotFoundError{Resource: "pod", ID: podRef})
			return
		}
		s.Store.Pods.AddContainer(pod.ID, id)
	}

	// Implicit pod grouping: NetworkMode "container:<id>" joins the referenced
	// container's pod (or creates one if it doesn't exist yet)
	if strings.HasPrefix(hostConfig.NetworkMode, "container:") {
		refID := strings.TrimPrefix(hostConfig.NetworkMode, "container:")
		refID, _ = s.Store.ResolveContainerID(refID)
		if _, inPod := s.Store.Pods.GetPodForContainer(id); !inPod {
			if pod, exists := s.Store.Pods.GetPodForContainer(refID); exists {
				s.Store.Pods.AddContainer(pod.ID, id)
			} else {
				short := refID
				if len(short) > 12 {
					short = short[:12]
				}
				pod := s.Store.Pods.CreatePod("container-"+short, nil)
				s.Store.Pods.AddContainer(pod.ID, refID)
				s.Store.Pods.AddContainer(pod.ID, id)
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
				s.Store.Pods.AddContainer(pod.ID, id)
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
						s.Store.Pods.AddContainer(pod.ID, existingID)
					}
				}
				s.Store.Pods.AddContainer(pod.ID, id)
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

	// Inject HOSTNAME and user env vars into container config
	if c.Config.Hostname != "" {
		s.Store.Containers.Update(id, func(c *api.Container) {
			c.Config.Env = append(c.Config.Env, "HOSTNAME="+c.Config.Hostname)
		})
	}
	if c.Config.User != "" {
		parts := strings.SplitN(c.Config.User, ":", 2)
		s.Store.Containers.Update(id, func(c *api.Container) {
			c.Config.Env = append(c.Config.Env, "SOCKERLESS_UID="+parts[0])
			if len(parts) == 2 {
				c.Config.Env = append(c.Config.Env, "SOCKERLESS_GID="+parts[1])
			}
		})
	}

	// Inject ExtraHosts env var
	if len(c.HostConfig.ExtraHosts) > 0 {
		s.Store.Containers.Update(id, func(c *api.Container) {
			c.Config.Env = append(c.Config.Env, "SOCKERLESS_EXTRA_HOSTS="+FormatExtraHostsEnv(c.HostConfig.ExtraHosts))
		})
	}

	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	// Re-read container after env updates
	c, _ = s.Store.Containers.Get(id)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.Store.Containers.Update(id, func(c *api.Container) {
		c.State.Status = "running"
		c.State.Running = true
		c.State.Pid = 42
		c.State.StartedAt = now
		c.State.FinishedAt = "0001-01-01T00:00:00Z"
		c.State.ExitCode = 0
	})

	s.emitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})

	// Start health check if configured
	if c.Config.Healthcheck != nil && len(c.Config.Healthcheck.Test) > 0 &&
		(len(c.Config.Healthcheck.Test) != 1 || !strings.EqualFold(c.Config.Healthcheck.Test[0], "NONE")) {
		s.StartHealthCheck(id)
	}

	// Process execution: spawn via ProcessLifecycleDriver
	cmd := append([]string{c.Path}, c.Args...)
	binds := s.resolveBindMounts(c.HostConfig.Binds, c.HostConfig.Mounts)
	tmpfs := resolveTmpfsMounts(c.HostConfig.Tmpfs)
	for k, v := range tmpfs {
		if binds == nil {
			binds = make(map[string]string)
		}
		binds[k] = v
	}
	started, err := s.Drivers.ProcessLifecycle.Start(id, cmd, c.Config.Env, binds)
	if err != nil {
		s.Logger.Error().Err(err).Str("container", id).Msg("failed to start container process")
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
				os.MkdirAll(etcDir, 0o755)
				os.WriteFile(joinCleanPath(rootPath, "/etc/hosts"), hostsContent, 0o644)
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
		s.Store.StopContainer(id, 0)
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

	s.Drivers.ProcessLifecycle.Stop(id)
	s.Store.ForceStopContainer(id, 0)
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
	exitCode := 0
	if signal == "SIGKILL" || signal == "9" || signal == "KILL" {
		exitCode = 137
	}

	s.Store.Containers.Update(id, func(c *api.Container) {
		c.State.Status = "exited"
		c.State.Running = false
		c.State.Pid = 0
		c.State.ExitCode = exitCode
		c.State.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	})

	// Close wait channel
	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}

	s.emitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
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
		s.Store.ForceStopContainer(id, 0)
	}

	s.StopHealthCheck(id)

	// Clean up container process
	s.Drivers.ProcessLifecycle.Cleanup(id)

	s.Store.Containers.Delete(id)
	s.Store.ContainerNames.Delete(c.Name)
	s.Store.LogBuffers.Delete(id)
	s.Store.WaitChs.Delete(id)

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
		s.Drivers.ProcessLifecycle.Stop(id)
		s.Store.ForceStopContainer(id, 0)
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

	// Re-spawn process (wait-and-stop goroutine is handled by the driver)
	cmd := append([]string{c.Path}, c.Args...)
	binds := s.resolveBindMounts(c.HostConfig.Binds, c.HostConfig.Mounts)
	_, err := s.Drivers.ProcessLifecycle.Start(id, cmd, c.Config.Env, binds)
	if err != nil {
		s.Logger.Error().Err(err).Str("container", id).Msg("failed to restart container process")
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

	// If already exited, return immediately
	if c.State.Status == "exited" || c.State.Status == "dead" {
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

