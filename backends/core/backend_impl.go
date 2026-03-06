package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// Compile-time check that BaseServer implements api.Backend.
var _ api.Backend = (*BaseServer)(nil)

// Info returns backend system information.
func (s *BaseServer) Info() (*api.BackendInfo, error) {
	running := 0
	paused := 0
	stopped := 0
	for _, c := range s.Store.Containers.List() {
		if c.State.Paused {
			paused++
			running++
		} else if c.State.Running {
			running++
		} else {
			stopped++
		}
	}
	return &api.BackendInfo{
		ID:                s.Desc.ID,
		Name:              s.Desc.Name,
		ServerVersion:     s.Desc.ServerVersion,
		Containers:        s.Store.Containers.Len(),
		ContainersRunning: running,
		ContainersPaused:  paused,
		ContainersStopped: stopped,
		Images:            s.Store.Images.Len(),
		Driver:            s.Desc.Driver,
		OperatingSystem:   s.Desc.OperatingSystem,
		OSType:            s.Desc.OSType,
		Architecture:      s.Desc.Architecture,
		NCPU:              s.Desc.NCPU,
		MemTotal:          s.Desc.MemTotal,
		KernelVersion:     "5.15.0-sockerless",
	}, nil
}

// ContainerCreate creates a container in the in-memory store.
func (s *BaseServer) ContainerCreate(req *api.ContainerCreateRequest) (*api.ContainerCreateResponse, error) {
	name := req.Name
	if name == "" {
		name = "/" + GenerateName()
	} else if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}

	if _, exists := s.Store.ContainerNames.Get(name); exists {
		return nil, &api.ConflictError{
			Message: fmt.Sprintf("Conflict. The container name \"%s\" is already in use", strings.TrimPrefix(name, "/")),
		}
	}

	id := GenerateID()

	config := api.ContainerConfig{}
	if req.ContainerConfig != nil {
		config = *req.ContainerConfig
	}

	if img, ok := s.Store.ResolveImage(config.Image); ok {
		config.Env = MergeEnvByKey(img.Config.Env, config.Env)
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

	container := s.buildContainerFromConfig(id, name, config, hostConfig, req.NetworkingConfig)
	s.Store.Containers.Put(id, container)
	s.Store.ContainerNames.Put(name, id)

	if img, ok := s.Store.ResolveImage(config.Image); ok {
		if ctxDir, ok := s.Store.BuildContexts.Load(img.ID); ok {
			s.Store.StagingDirs.Store(id, ctxDir.(string))
		}
	}

	// Implicit pod grouping for container: network mode
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

	// Implicit pod grouping for user-defined networks
	if _, inPod := s.Store.Pods.GetPodForContainer(id); !inPod {
		for netName := range container.NetworkSettings.Networks {
			if netName == "bridge" || netName == "host" || netName == "none" || netName == "default" {
				continue
			}
			if pod, exists := s.Store.Pods.GetPodForNetwork(netName); exists {
				_ = s.Store.Pods.AddContainer(pod.ID, id)
				break
			}
			if net, netExists := s.Store.ResolveNetwork(netName); netExists && len(net.Containers) > 1 {
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

	return &api.ContainerCreateResponse{
		ID:       id,
		Warnings: []string{},
	}, nil
}

// ContainerInspect returns container details.
func (s *BaseServer) ContainerInspect(ref string) (*api.Container, error) {
	c, ok := s.Store.ResolveContainer(ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &c, nil
}

// ContainerList lists containers matching options.
func (s *BaseServer) ContainerList(opts api.ContainerListOptions) ([]*api.ContainerSummary, error) {
	var result []*api.ContainerSummary
	for _, c := range s.Store.Containers.List() {
		if !opts.All && !c.State.Running {
			continue
		}
		if len(opts.Filters) > 0 && !MatchContainerFilters(c, opts.Filters) {
			continue
		}

		command := c.Path
		if len(c.Args) > 0 {
			command += " " + strings.Join(c.Args, " ")
		}
		created, _ := time.Parse(time.RFC3339Nano, c.Created)

		imageID := ""
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			imageID = img.ID
		} else {
			h := sha256.Sum256([]byte(c.Config.Image))
			imageID = fmt.Sprintf("sha256:%x", h)
		}

		labels := c.Config.Labels
		if labels == nil {
			labels = make(map[string]string)
		}
		mounts := c.Mounts
		if mounts == nil {
			mounts = []api.MountPoint{}
		}
		summary := &api.ContainerSummary{
			ID:      c.ID,
			Names:   []string{c.Name},
			Image:   c.Config.Image,
			ImageID: imageID,
			Command: command,
			Created: created.Unix(),
			State:   c.State.Status,
			Status:  FormatStatus(c.State),
			Ports:   buildPortList(c.HostConfig.PortBindings, c.Config.ExposedPorts),
			Labels:  labels,
			HostConfig: &api.HostConfigSummary{
				NetworkMode: c.HostConfig.NetworkMode,
			},
			Mounts: mounts,
			NetworkSettings: &api.SummaryNetworkSettings{
				Networks: c.NetworkSettings.Networks,
			},
		}
		result = append(result, summary)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Created > result[j].Created
	})

	if opts.Limit > 0 && opts.Limit < len(result) {
		result = result[:opts.Limit]
	}

	if result == nil {
		result = []*api.ContainerSummary{}
	}
	return result, nil
}

// ContainerStart starts a container.
func (s *BaseServer) ContainerStart(ref string) error {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	if c.State.Running {
		return &api.NotModifiedError{}
	}

	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod && len(pod.ContainerIDs) > 1 {
		return &api.InvalidParameterError{
			Message: "multi-container pods are not supported by the memory backend",
		}
	}

	// Inject env vars
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
	if len(c.HostConfig.ExtraHosts) > 0 {
		s.Store.Containers.Update(id, func(c *api.Container) {
			if !hasEnvKey(c.Config.Env, "SOCKERLESS_EXTRA_HOSTS") {
				c.Config.Env = append(c.Config.Env, "SOCKERLESS_EXTRA_HOSTS="+FormatExtraHostsEnv(c.HostConfig.ExtraHosts))
			}
		})
	}

	c, _ = s.Store.Containers.Get(id)

	tmpfs := resolveTmpfsMounts(c.HostConfig.Tmpfs)
	if len(tmpfs) > 0 {
		dirs := make([]string, 0, len(tmpfs))
		for _, v := range tmpfs {
			dirs = append(dirs, v)
		}
		s.Store.TmpfsDirs.Store(id, dirs)
	}
	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	pid := s.Store.NextPID()
	s.Store.Containers.Update(id, func(c *api.Container) {
		c.State.Status = "running"
		c.State.Running = true
		c.State.Pid = pid
		c.State.StartedAt = now
		c.State.FinishedAt = "0001-01-01T00:00:00Z"
		c.State.ExitCode = 0
	})

	c, _ = s.Store.Containers.Get(id)
	s.emitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})

	if c.Config.Healthcheck != nil && len(c.Config.Healthcheck.Test) > 0 &&
		(len(c.Config.Healthcheck.Test) != 1 || !strings.EqualFold(c.Config.Healthcheck.Test[0], "NONE")) {
		s.StartHealthCheck(id)
	}

	// Auto-spawn a local agent if configured (enables real exec for simulator backends)
	if err := s.SpawnAutoAgent(id); err != nil {
		s.Logger.Warn().Err(err).Str("container", id).Msg("auto-agent spawn failed")
	}

	return nil
}

// ContainerStop stops a running container.
func (s *BaseServer) ContainerStop(ref string, timeout *int) error {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	s.StopHealthCheck(id)
	StopAutoAgent(id)
	s.Store.ForceStopContainer(id, 0)
	s.emitEvent("container", "die", id, map[string]string{
		"exitCode": "0",
		"name":     strings.TrimPrefix(c.Name, "/"),
	})
	s.emitEvent("container", "stop", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	return nil
}

// ContainerKill sends a signal to a container.
func (s *BaseServer) ContainerKill(ref string, signal string) error {
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

	s.StopHealthCheck(id)
	StopAutoAgent(id)

	exitCode := signalToExitCode(signal)

	s.Store.Containers.Update(id, func(c *api.Container) {
		c.State.Status = "exited"
		c.State.Running = false
		c.State.Pid = 0
		c.State.ExitCode = exitCode
		c.State.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	})

	s.emitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	s.emitEvent("container", "die", id, map[string]string{
		"exitCode": fmt.Sprintf("%d", exitCode),
		"signal":   signal,
		"name":     strings.TrimPrefix(c.Name, "/"),
	})

	if ch, ok := s.Store.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
	return nil
}

// ContainerRemove removes a container.
func (s *BaseServer) ContainerRemove(ref string, force bool) error {
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

	if c.State.Running {
		s.emitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		s.emitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		s.Store.ForceStopContainer(id, 0)
	}

	s.StopHealthCheck(id)
	StopAutoAgent(id)

	ctx := context.Background()
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID != "" {
			_ = s.Drivers.Network.Disconnect(ctx, ep.NetworkID, id)
		}
	}

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
	return nil
}

// ContainerLogs returns container logs as a stream.
func (s *BaseServer) ContainerLogs(ref string, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
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

	logBytes := s.Drivers.Stream.LogBytes(id)
	if !opts.ShowStdout {
		logBytes = nil
	}

	var lines []string
	if len(logBytes) > 0 {
		ts := c.State.StartedAt
		if ts == "" || ts == "0001-01-01T00:00:00Z" {
			ts = time.Now().UTC().Format(time.RFC3339Nano)
		}
		raw := strings.Split(strings.TrimRight(string(logBytes), "\n"), "\n")
		for _, line := range raw {
			if line == "" {
				continue
			}
			lines = append(lines, ts+" "+line)
		}
	}

	if opts.Since != "" {
		if since, err := ParseDockerTimestamp(opts.Since); err == nil {
			lines = FilterLogSince(lines, since)
		}
	}
	if opts.Until != "" {
		if until, err := ParseDockerTimestamp(opts.Until); err == nil {
			lines = FilterLogUntil(lines, until)
		}
	}
	if opts.Tail != "" && opts.Tail != "all" {
		if n, err := fmt.Sscanf(opts.Tail, "%d", new(int)); err == nil && n > 0 {
			var tailN int
			fmt.Sscanf(opts.Tail, "%d", &tailN)
			lines = FilterLogTail(lines, tailN)
		}
	}

	if !opts.Timestamps {
		for i, line := range lines {
			if idx := strings.IndexByte(line, ' '); idx >= 0 {
				lines[i] = line[idx+1:]
			}
		}
	}

	var buf bytes.Buffer
	for _, line := range lines {
		buf.WriteString(line + "\n")
	}

	if opts.Follow {
		pr, pw := io.Pipe()
		go func() {
			if _, err := pw.Write(buf.Bytes()); err != nil {
				pw.CloseWithError(err)
				return
			}

			subID := GenerateID()[:16]
			ch := s.Drivers.Stream.LogSubscribe(id, subID)
			if ch == nil {
				_ = pw.Close()
				return
			}
			defer s.Drivers.Stream.LogUnsubscribe(id, subID)
			for chunk := range ch {
				if len(chunk) > 0 {
					if _, err := pw.Write(chunk); err != nil {
						pw.CloseWithError(err)
						return
					}
				}
			}
			_ = pw.Close()
		}()
		return pr, nil
	}

	return io.NopCloser(&buf), nil
}

// ContainerWait blocks until a container stops and returns its exit code.
func (s *BaseServer) ContainerWait(ref string, condition string) (*api.ContainerWaitResponse, error) {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		if condition == "removed" {
			return &api.ContainerWaitResponse{StatusCode: 0}, nil
		}
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}

	if condition == "" {
		condition = "not-running"
	}

	c, exists := s.Store.Containers.Get(id)
	if !exists {
		if condition == "removed" {
			return &api.ContainerWaitResponse{StatusCode: 0}, nil
		}
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}

	if condition != "next-exit" && (c.State.Status == "exited" || c.State.Status == "dead") {
		return &api.ContainerWaitResponse{StatusCode: c.State.ExitCode}, nil
	}

	ch, ok := s.Store.WaitChs.Load(id)
	if !ok {
		c, _ = s.Store.Containers.Get(id)
		return &api.ContainerWaitResponse{StatusCode: c.State.ExitCode}, nil
	}

	<-ch.(chan struct{})
	c, _ = s.Store.Containers.Get(id)
	if condition == "removed" {
		for i := 0; i < 50; i++ {
			if _, exists := s.Store.Containers.Get(id); !exists {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return &api.ContainerWaitResponse{StatusCode: c.State.ExitCode}, nil
}

// ContainerAttach establishes a bidirectional stream to the container.
func (s *BaseServer) ContainerAttach(ref string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, ok := s.Store.Containers.Get(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}

	tty := c.Config.Tty
	stdinPR, stdinPW := io.Pipe()
	stdoutPR, stdoutPW := io.Pipe()
	conn := &pipeConn{PipeReader: stdinPR, PipeWriter: stdoutPW}
	go func() {
		_ = s.Drivers.Stream.Attach(context.Background(), id, tty, conn)
		_ = stdoutPW.Close()
		_ = stdinPR.Close()
	}()

	return &pipeRWC{reader: stdoutPR, writer: stdinPW}, nil
}

// ContainerRestart restarts a container.
func (s *BaseServer) ContainerRestart(ref string, timeout *int) error {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	if c.State.Running {
		s.StopHealthCheck(id)
		StopAutoAgent(id)
		s.Store.ForceStopContainer(id, 0)
		s.emitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		s.emitEvent("container", "stop", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	}

	exitCh := make(chan struct{})
	s.Store.WaitChs.Store(id, exitCh)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	pid := s.Store.NextPID()
	s.Store.Containers.Update(id, func(c *api.Container) {
		c.State.Status = "running"
		c.State.Running = true
		c.State.Pid = pid
		c.State.StartedAt = now
		c.State.FinishedAt = "0001-01-01T00:00:00Z"
		c.State.ExitCode = 0
		c.RestartCount++
	})

	c, _ = s.Store.Containers.Get(id)

	if dirs, ok := s.Store.TmpfsDirs.LoadAndDelete(id); ok {
		for _, d := range dirs.([]string) {
			os.RemoveAll(d)
		}
	}

	tmpfs := resolveTmpfsMounts(c.HostConfig.Tmpfs)
	if len(tmpfs) > 0 {
		dirs := make([]string, 0, len(tmpfs))
		for _, v := range tmpfs {
			dirs = append(dirs, v)
		}
		s.Store.TmpfsDirs.Store(id, dirs)
	}
	s.emitEvent("container", "start", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
	s.emitEvent("container", "restart", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})

	c, _ = s.Store.Containers.Get(id)
	if c.Config.Healthcheck != nil && len(c.Config.Healthcheck.Test) > 0 &&
		(len(c.Config.Healthcheck.Test) != 1 || !strings.EqualFold(c.Config.Healthcheck.Test[0], "NONE")) {
		s.StartHealthCheck(id)
	}

	// Re-spawn auto-agent after restart
	if err := s.SpawnAutoAgent(id); err != nil {
		s.Logger.Warn().Err(err).Str("container", id).Msg("auto-agent spawn failed on restart")
	}

	return nil
}

// ContainerTop returns the running processes inside a container.
func (s *BaseServer) ContainerTop(ref string, psArgs string) (*api.ContainerTopResponse, error) {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		return nil, &api.ConflictError{
			Message: fmt.Sprintf("Container %s is not running", ref),
		}
	}

	cmd := c.Path
	if len(c.Args) > 0 {
		cmd += " " + strings.Join(c.Args, " ")
	}
	pid := fmt.Sprintf("%d", c.State.Pid)
	if c.State.Pid == 0 {
		pid = "1"
	}

	return &api.ContainerTopResponse{
		Titles: []string{"UID", "PID", "PPID", "C", "STIME", "TTY", "TIME", "CMD"},
		Processes: [][]string{
			{"root", pid, "0", "0", "00:00", "?", "00:00:00", cmd},
		},
	}, nil
}

// ContainerPrune removes stopped containers.
func (s *BaseServer) ContainerPrune(filters map[string][]string) (*api.ContainerPruneResponse, error) {
	labelFilters := filters["label"]
	untilFilters := filters["until"]

	pruned := s.Store.Containers.PruneIf(func(_ string, c api.Container) bool {
		if c.State.Status != "exited" && c.State.Status != "dead" {
			return false
		}
		if len(labelFilters) > 0 && !MatchLabels(c.Config.Labels, labelFilters) {
			return false
		}
		if len(untilFilters) > 0 && !MatchUntil(c.Created, untilFilters) {
			return false
		}
		return true
	})

	ctx := context.Background()
	var spaceReclaimed uint64
	deleted := make([]string, 0, len(pruned))
	for _, c := range pruned {
		if rootPath, err := s.Drivers.Filesystem.RootPath(c.ID); err == nil && rootPath != "" {
			spaceReclaimed += uint64(DirSize(rootPath))
		}
		s.StopHealthCheck(c.ID)
		s.Store.ContainerNames.Delete(c.Name)
		s.Store.LogBuffers.Delete(c.ID)
		if ch, ok := s.Store.WaitChs.LoadAndDelete(c.ID); ok {
			close(ch.(chan struct{}))
		}
		for _, ep := range c.NetworkSettings.Networks {
			if ep != nil && ep.NetworkID != "" {
				_ = s.Drivers.Network.Disconnect(ctx, ep.NetworkID, c.ID)
			}
		}
		if pod, inPod := s.Store.Pods.GetPodForContainer(c.ID); inPod {
			s.Store.Pods.RemoveContainer(pod.ID, c.ID)
		}
		s.Store.StagingDirs.Delete(c.ID)
		if dirs, ok := s.Store.TmpfsDirs.LoadAndDelete(c.ID); ok {
			for _, d := range dirs.([]string) {
				os.RemoveAll(d)
			}
		}
		for _, eid := range c.ExecIDs {
			s.Store.Execs.Delete(eid)
		}
		s.emitEvent("container", "destroy", c.ID, map[string]string{
			"name": strings.TrimPrefix(c.Name, "/"),
		})
		deleted = append(deleted, c.ID)
	}

	return &api.ContainerPruneResponse{
		ContainersDeleted: deleted,
		SpaceReclaimed:    spaceReclaimed,
	}, nil
}

// ContainerStats returns resource usage stats for a container.
func (s *BaseServer) ContainerStats(ref string, stream bool) (io.ReadCloser, error) {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	memLimit := int64(1073741824)
	if c.HostConfig.Memory > 0 {
		memLimit = c.HostConfig.Memory
	}

	if !stream || !c.State.Running {
		now := time.Now().UTC()
		entry := s.buildStatsEntry(id, now, "0001-01-01T00:00:00Z", memLimit)
		data, _ := json.Marshal(entry)
		return io.NopCloser(bytes.NewReader(data)), nil
	}

	pr, pw := io.Pipe()
	go func() {
		enc := json.NewEncoder(pw)
		preread := "0001-01-01T00:00:00Z"
		for {
			now := time.Now().UTC()
			entry := s.buildStatsEntry(id, now, preread, memLimit)
			if err := enc.Encode(entry); err != nil {
				pw.CloseWithError(err)
				return
			}
			preread = now.Format(time.RFC3339Nano)

			time.Sleep(1 * time.Second)
			if cur, ok := s.Store.Containers.Get(id); !ok || !cur.State.Running {
				_ = pw.Close()
				return
			}
		}
	}()
	return pr, nil
}

// ContainerRename renames a container.
func (s *BaseServer) ContainerRename(ref string, newName string) error {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	if newName == "" {
		return &api.InvalidParameterError{Message: "name is required"}
	}
	if !strings.HasPrefix(newName, "/") {
		newName = "/" + newName
	}

	s.Store.RenameMu.Lock()
	defer s.Store.RenameMu.Unlock()

	c, _ := s.Store.Containers.Get(id)
	oldName := c.Name

	if _, exists := s.Store.ContainerNames.Get(newName); exists {
		return &api.ConflictError{
			Message: fmt.Sprintf("Conflict. The container name \"%s\" is already in use", strings.TrimPrefix(newName, "/")),
		}
	}

	s.Store.ContainerNames.Delete(oldName)
	s.Store.ContainerNames.Put(newName, id)
	s.Store.Containers.Update(id, func(c *api.Container) {
		c.Name = newName
	})

	c, _ = s.Store.Containers.Get(id)
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID != "" {
			s.Store.Networks.Update(ep.NetworkID, func(n *api.Network) {
				if er, ok := n.Containers[id]; ok {
					er.Name = strings.TrimPrefix(newName, "/")
					n.Containers[id] = er
				}
			})
		}
	}

	s.emitEvent("container", "rename", id, map[string]string{
		"name":    strings.TrimPrefix(newName, "/"),
		"oldName": strings.TrimPrefix(oldName, "/"),
	})
	return nil
}

// ContainerPause pauses a container.
func (s *BaseServer) ContainerPause(ref string) error {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	var name string
	var conflict error
	s.Store.Containers.Update(id, func(c *api.Container) {
		if c.State.Paused {
			conflict = &api.ConflictError{Message: fmt.Sprintf("Container %s is already paused", ref)}
			return
		}
		if !c.State.Running {
			conflict = &api.ConflictError{Message: fmt.Sprintf("Container %s is not running", ref)}
			return
		}
		c.State.Paused = true
		c.State.Status = "paused"
		name = c.Name
	})
	if conflict != nil {
		return conflict
	}

	s.StopHealthCheck(id)
	s.emitEvent("container", "pause", id, map[string]string{"name": strings.TrimPrefix(name, "/")})
	return nil
}

// ContainerUnpause unpauses a container.
func (s *BaseServer) ContainerUnpause(ref string) error {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}

	var name string
	var hasHealthcheck bool
	var conflict error
	s.Store.Containers.Update(id, func(c *api.Container) {
		if !c.State.Paused {
			conflict = &api.ConflictError{Message: fmt.Sprintf("Container %s is not paused", ref)}
			return
		}
		c.State.Paused = false
		c.State.Status = "running"
		name = c.Name
		hasHealthcheck = c.Config.Healthcheck != nil && len(c.Config.Healthcheck.Test) > 0 &&
			(len(c.Config.Healthcheck.Test) != 1 || !strings.EqualFold(c.Config.Healthcheck.Test[0], "NONE"))
	})
	if conflict != nil {
		return conflict
	}

	if hasHealthcheck {
		s.StartHealthCheck(id)
	}
	s.emitEvent("container", "unpause", id, map[string]string{"name": strings.TrimPrefix(name, "/")})
	return nil
}

// ExecCreate creates an exec instance in a container.
func (s *BaseServer) ExecCreate(ref string, req *api.ExecCreateRequest) (*api.ExecCreateResponse, error) {
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		if c.AgentAddress != "" || c.State.Status == "" {
			return nil, &api.ConflictError{Message: "Container " + ref + " is not running"}
		}
	}

	if len(req.Cmd) == 0 {
		return nil, &api.InvalidParameterError{Message: "No exec command specified"}
	}

	execID := GenerateID()
	entrypoint := ""
	var arguments []string
	if len(req.Cmd) > 0 {
		entrypoint = req.Cmd[0]
		arguments = req.Cmd[1:]
	}

	exec := api.ExecInstance{
		ID:          execID,
		ContainerID: id,
		Running:     false,
		ExitCode:    0,
		OpenStdin:   req.AttachStdin,
		OpenStdout:  req.AttachStdout,
		OpenStderr:  req.AttachStderr,
		ProcessConfig: api.ExecProcessConfig{
			Tty:        req.Tty,
			Entrypoint: entrypoint,
			Arguments:  arguments,
			Privileged: &req.Privileged,
			User:       req.User,
			Env:        req.Env,
			WorkingDir: req.WorkingDir,
		},
	}

	s.Store.Execs.Put(execID, exec)
	s.Store.Containers.Update(id, func(c *api.Container) {
		c.ExecIDs = append(c.ExecIDs, execID)
	})

	return &api.ExecCreateResponse{ID: execID}, nil
}

// ExecStart starts an exec instance and returns a read-write stream.
func (s *BaseServer) ExecStart(id string, opts api.ExecStartRequest) (io.ReadWriteCloser, error) {
	exec, ok := s.Store.Execs.Get(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "exec instance", ID: id}
	}

	c, ok := s.Store.Containers.Get(exec.ContainerID)
	if !ok {
		return nil, &api.ConflictError{
			Message: fmt.Sprintf("Container %s has been removed", exec.ContainerID),
		}
	}

	execPid := s.Store.NextPID()
	s.Store.Execs.Update(id, func(e *api.ExecInstance) {
		e.Running = true
		e.Pid = execPid
	})

	tty := exec.ProcessConfig.Tty || opts.Tty
	cmd := append([]string{exec.ProcessConfig.Entrypoint}, exec.ProcessConfig.Arguments...)
	env := mergeEnv(c.Config.Env, exec.ProcessConfig.Env)
	workDir := exec.ProcessConfig.WorkingDir
	if workDir == "" {
		workDir = c.Config.WorkingDir
	}

	// Two pipes: stdin flows from caller to driver, stdout flows from driver to caller.
	stdinPR, stdinPW := io.Pipe()   // caller writes stdin → driver reads
	stdoutPR, stdoutPW := io.Pipe() // driver writes stdout → caller reads
	conn := &pipeConn{PipeReader: stdinPR, PipeWriter: stdoutPW}
	go func() {
		exitCode := s.Drivers.Exec.Exec(context.Background(), exec.ContainerID, id, cmd, env, workDir, tty, conn)

		s.Store.Execs.Update(id, func(e *api.ExecInstance) {
			e.Running = false
			e.Pid = 0
			e.ExitCode = exitCode
			e.CanRemove = true
		})

		_ = stdoutPW.Close()
		_ = stdinPR.Close() // unblock bridge's stdin reader goroutine
	}()

	return &pipeRWC{reader: stdoutPR, writer: stdinPW}, nil
}

// ExecInspect returns info about an exec instance.
func (s *BaseServer) ExecInspect(id string) (*api.ExecInstance, error) {
	exec, ok := s.Store.Execs.Get(id)
	if !ok {
		return nil, &api.NotFoundError{Resource: "exec instance", ID: id}
	}
	return &exec, nil
}

// ImagePull pulls an image (synthetic for memory backend).
func (s *BaseServer) ImagePull(ref string, auth string) (io.ReadCloser, error) {
	if ref == "" {
		return nil, &api.InvalidParameterError{Message: "image reference is required"}
	}

	if !strings.Contains(ref, ":") && !strings.Contains(ref, "@") {
		ref = ref + ":latest"
	}

	if _, exists := s.Store.ResolveImage(ref); exists {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		_ = enc.Encode(map[string]any{"status": fmt.Sprintf("Pulling from %s", strings.Split(ref, ":")[0])})
		_ = enc.Encode(map[string]any{"status": fmt.Sprintf("Status: Image is up to date for %s", ref)})
		return io.NopCloser(&buf), nil
	}

	hash := sha256.Sum256([]byte(ref))
	imageID := fmt.Sprintf("sha256:%x", hash)

	imgConfig := api.ContainerConfig{
		Env:    []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
		Cmd:    []string{"/bin/sh"},
		Labels: make(map[string]string),
	}

	h := fnv.New32a()
	h.Write([]byte(ref))
	imgSize := int64(10_000_000 + h.Sum32()%90_000_000)

	now := time.Now().UTC()
	img := api.Image{
		ID:       imageID,
		RepoTags: []string{ref},
		RepoDigests: []string{
			strings.Split(ref, ":")[0] + "@sha256:" + fmt.Sprintf("%x", hash)[:64],
		},
		Created:      now.Format(time.RFC3339Nano),
		Size:         imgSize,
		VirtualSize:  imgSize,
		Architecture: "amd64",
		Os:           "linux",
		Config:       imgConfig,
		RootFS: api.RootFS{
			Type:   "layers",
			Layers: []string{"sha256:" + GenerateID()},
		},
		GraphDriver: api.GraphDriverData{
			Name: "overlay2",
			Data: map[string]string{
				"MergedDir": "/var/lib/sockerless/overlay2/" + imageID[7:19] + "/merged",
				"UpperDir":  "/var/lib/sockerless/overlay2/" + imageID[7:19] + "/diff",
				"WorkDir":   "/var/lib/sockerless/overlay2/" + imageID[7:19] + "/work",
			},
		},
		Metadata: api.ImageMetadata{LastTagTime: now.Format(time.RFC3339Nano)},
	}

	StoreImageWithAliases(s.Store, ref, img)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	progress := []map[string]any{
		{"status": fmt.Sprintf("Pulling from %s", strings.Split(ref, ":")[0])},
		{"status": "Pulling fs layer", "id": "abc123"},
		{"status": "Download complete", "id": "abc123"},
		{"status": "Pull complete", "id": "abc123"},
		{"status": fmt.Sprintf("Digest: sha256:%x", hash)},
		{"status": fmt.Sprintf("Status: Downloaded newer image for %s", ref)},
	}
	for _, p := range progress {
		enc.Encode(p)
	}

	return io.NopCloser(&buf), nil
}

// ImageInspect returns detailed info about an image.
func (s *BaseServer) ImageInspect(name string) (*api.Image, error) {
	img, ok := s.Store.ResolveImage(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "image", ID: name}
	}
	return &img, nil
}

// ImageLoad loads an image from a tar archive.
func (s *BaseServer) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	repoTags, imgConfig := parseImageTar(r)

	if len(repoTags) == 0 {
		repoTags = []string{"loaded:latest"}
	}

	id := "sha256:" + GenerateID()
	layerID := GenerateID()
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	img := api.Image{
		ID:           id,
		RepoTags:     repoTags,
		Created:      nowStr,
		Size:         0,
		Architecture: "amd64",
		Os:           "linux",
		Config: api.ContainerConfig{
			Labels: make(map[string]string),
		},
		RootFS: api.RootFS{
			Type:   "layers",
			Layers: []string{"sha256:" + layerID},
		},
		GraphDriver: api.GraphDriverData{
			Name: "overlay2",
			Data: map[string]string{
				"MergedDir": "/var/lib/sockerless/overlay2/" + id[7:19] + "/merged",
				"UpperDir":  "/var/lib/sockerless/overlay2/" + id[7:19] + "/diff",
				"WorkDir":   "/var/lib/sockerless/overlay2/" + id[7:19] + "/work",
			},
		},
		Metadata: api.ImageMetadata{LastTagTime: nowStr},
	}

	if imgConfig != nil {
		if len(imgConfig.Env) > 0 {
			img.Config.Env = imgConfig.Env
		}
		if len(imgConfig.Cmd) > 0 {
			img.Config.Cmd = imgConfig.Cmd
		}
		if len(imgConfig.Entrypoint) > 0 {
			img.Config.Entrypoint = imgConfig.Entrypoint
		}
		if imgConfig.WorkingDir != "" {
			img.Config.WorkingDir = imgConfig.WorkingDir
		}
		if len(imgConfig.Labels) > 0 {
			img.Config.Labels = imgConfig.Labels
		}
	}

	for _, tag := range repoTags {
		StoreImageWithAliases(s.Store, tag, img)
	}
	s.Store.Images.Put(id, img)

	displayTag := repoTags[0]
	s.emitEvent("image", "load", id, map[string]string{"name": displayTag})

	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(map[string]string{
		"stream": fmt.Sprintf("Loaded image: %s\n", displayTag),
	})
	return io.NopCloser(&buf), nil
}

// ImageTag tags an image.
func (s *BaseServer) ImageTag(source string, repo string, tag string) error {
	if repo == "" {
		return &api.InvalidParameterError{Message: "repo name is required"}
	}

	img, ok := s.Store.ResolveImage(source)
	if !ok {
		return &api.NotFoundError{Resource: "image", ID: source}
	}

	ref := repo
	if tag != "" {
		ref = repo + ":" + tag
	}

	// Check for duplicate tag
	for _, t := range img.RepoTags {
		if t == ref {
			return nil
		}
	}

	img.RepoTags = append(img.RepoTags, ref)
	img.Metadata.LastTagTime = time.Now().UTC().Format(time.RFC3339Nano)

	StoreImageWithAliases(s.Store, ref, img)
	s.Store.Images.Put(img.ID, img)
	for _, existingTag := range img.RepoTags {
		s.Store.Images.Put(existingTag, img)
	}

	s.emitEvent("image", "tag", img.ID, map[string]string{"name": ref})
	return nil
}

// ImageList lists images.
func (s *BaseServer) ImageList(opts api.ImageListOptions) ([]*api.ImageSummary, error) {
	imgContainerCount := make(map[string]int64)
	for _, c := range s.Store.Containers.List() {
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			imgContainerCount[img.ID]++
		}
	}

	seen := make(map[string]bool)
	var result []*api.ImageSummary
	for _, img := range s.Store.Images.List() {
		if seen[img.ID] {
			continue
		}
		seen[img.ID] = true

		if len(opts.Filters) > 0 && !s.matchImageFilters(&img, opts.Filters) {
			continue
		}

		created, _ := time.Parse(time.RFC3339Nano, img.Created)
		result = append(result, &api.ImageSummary{
			ID:          img.ID,
			RepoTags:    img.RepoTags,
			RepoDigests: img.RepoDigests,
			Created:     created.Unix(),
			Size:        img.Size,
			VirtualSize: img.VirtualSize,
			Labels:      img.Config.Labels,
			Containers:  imgContainerCount[img.ID],
		})
	}

	if result == nil {
		result = []*api.ImageSummary{}
	}
	return result, nil
}

// ImageRemove removes an image.
func (s *BaseServer) ImageRemove(name string, force bool, prune bool) ([]*api.ImageDeleteResponse, error) {
	img, ok := s.Store.ResolveImage(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "image", ID: name}
	}

	// Check if any container uses this image
	if !force {
		for _, c := range s.Store.Containers.List() {
			cImg, _ := s.Store.ResolveImage(c.Config.Image)
			if cImg.ID == img.ID {
				return nil, &api.ConflictError{
					Message: fmt.Sprintf("conflict: unable to remove repository reference \"%s\" (must force)", name),
				}
			}
		}
	}

	var result []*api.ImageDeleteResponse
	for _, tag := range img.RepoTags {
		result = append(result, &api.ImageDeleteResponse{Untagged: tag})
		s.Store.Images.Delete(tag)
		parts := strings.SplitN(tag, ":", 2)
		s.Store.Images.Delete(parts[0])
		s.emitEvent("image", "untag", img.ID, map[string]string{"name": tag})
	}
	result = append(result, &api.ImageDeleteResponse{Deleted: img.ID})
	s.Store.Images.Delete(img.ID)

	if short := img.ID; len(short) > 19 {
		s.Store.Images.Delete(short[:19])
	}

	if ctxDir, ok := s.Store.BuildContexts.LoadAndDelete(img.ID); ok {
		os.RemoveAll(ctxDir.(string))
	}

	s.emitEvent("image", "delete", img.ID, map[string]string{"name": name})
	return result, nil
}

// ImageHistory returns the history of an image.
func (s *BaseServer) ImageHistory(name string) ([]*api.ImageHistoryEntry, error) {
	img, ok := s.Store.ResolveImage(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "image", ID: name}
	}

	var result []*api.ImageHistoryEntry
	created, _ := time.Parse(time.RFC3339Nano, img.Created)

	// Synthetic parent layers
	for i, layer := range img.RootFS.Layers {
		entry := &api.ImageHistoryEntry{
			ID:        layer,
			Created:   created.Unix() - int64(len(img.RootFS.Layers)-i),
			CreatedBy: "/bin/sh -c #(nop) ADD file:... in / ",
			Size:      img.Size / int64(len(img.RootFS.Layers)+1),
		}
		result = append(result, entry)
	}

	// Final layer
	cmd := ""
	if len(img.Config.Cmd) > 0 {
		cmd = fmt.Sprintf("/bin/sh -c #(nop)  CMD [%q]", strings.Join(img.Config.Cmd, " "))
	}
	result = append(result, &api.ImageHistoryEntry{
		ID:        img.ID,
		Created:   created.Unix(),
		CreatedBy: cmd,
		Tags:      img.RepoTags,
		Size:      0,
	})

	return result, nil
}

// ImagePrune removes unused images.
func (s *BaseServer) ImagePrune(filters map[string][]string) (*api.ImagePruneResponse, error) {
	inUseIDs := make(map[string]bool)
	for _, c := range s.Store.Containers.List() {
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			inUseIDs[img.ID] = true
		}
	}

	seen := make(map[string]bool)
	var deleted []*api.ImageDeleteResponse
	var spaceReclaimed uint64

	for _, img := range s.Store.Images.List() {
		if seen[img.ID] || inUseIDs[img.ID] {
			continue
		}
		seen[img.ID] = true

		// Skip images with real tags unless dangling filter
		if danglingVals, ok := filters["dangling"]; ok {
			if danglingVals[0] != "true" && danglingVals[0] != "1" {
				continue
			}
		}

		for _, tag := range img.RepoTags {
			deleted = append(deleted, &api.ImageDeleteResponse{Untagged: tag})
			s.Store.Images.Delete(tag)
			parts := strings.SplitN(tag, ":", 2)
			s.Store.Images.Delete(parts[0])
			s.emitEvent("image", "untag", img.ID, map[string]string{"name": tag})
		}
		deleted = append(deleted, &api.ImageDeleteResponse{Deleted: img.ID})
		spaceReclaimed += uint64(img.Size)
		s.Store.Images.Delete(img.ID)

		if ctxDir, ok := s.Store.BuildContexts.LoadAndDelete(img.ID); ok {
			os.RemoveAll(ctxDir.(string))
		}

		s.emitEvent("image", "delete", img.ID, nil)
	}

	if deleted == nil {
		deleted = []*api.ImageDeleteResponse{}
	}
	return &api.ImagePruneResponse{
		ImagesDeleted:  deleted,
		SpaceReclaimed: spaceReclaimed,
	}, nil
}

// AuthLogin authenticates with a registry.
func (s *BaseServer) AuthLogin(req *api.AuthRequest) (*api.AuthResponse, error) {
	if req.ServerAddress != "" {
		s.Store.Creds.Put(req.ServerAddress, *req)
	}
	return &api.AuthResponse{
		Status: "Login Succeeded",
	}, nil
}

// NetworkCreate creates a network.
func (s *BaseServer) NetworkCreate(req *api.NetworkCreateRequest) (*api.NetworkCreateResponse, error) {
	if req.Name == "" {
		return nil, &api.InvalidParameterError{Message: "network name is required"}
	}

	resp, err := s.Drivers.Network.Create(context.Background(), req.Name, req)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return nil, &api.ConflictError{Message: err.Error()}
		}
		return nil, &api.InvalidParameterError{Message: err.Error()}
	}

	s.emitEvent("network", "create", resp.ID, map[string]string{"name": req.Name})
	return resp, nil
}

// NetworkList lists networks.
func (s *BaseServer) NetworkList(f map[string][]string) ([]*api.Network, error) {
	result, err := s.Drivers.Network.List(context.Background(), f)
	if err != nil {
		return nil, &api.ServerError{Message: err.Error()}
	}
	return result, nil
}

// NetworkInspect returns details about a network.
func (s *BaseServer) NetworkInspect(ref string) (*api.Network, error) {
	n, err := s.Drivers.Network.Inspect(context.Background(), ref)
	if err != nil {
		return nil, &api.NotFoundError{Resource: "network", ID: ref}
	}
	return n, nil
}

// NetworkConnect connects a container to a network.
func (s *BaseServer) NetworkConnect(ref string, req *api.NetworkConnectRequest) error {
	net, ok := s.Store.ResolveNetwork(ref)
	if !ok {
		return &api.NotFoundError{Resource: "network", ID: ref}
	}

	containerID, ok := s.Store.ResolveContainerID(req.Container)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: req.Container}
	}

	if err := s.Drivers.Network.Connect(context.Background(), net.ID, containerID, req.EndpointConfig); err != nil {
		return &api.ServerError{Message: err.Error()}
	}

	s.emitEvent("network", "connect", net.ID, map[string]string{"container": containerID})

	if pod, exists := s.Store.Pods.GetPodForNetwork(net.Name); exists {
		if _, inPod := s.Store.Pods.GetPodForContainer(containerID); !inPod {
			_ = s.Store.Pods.AddContainer(pod.ID, containerID)
		}
	}
	return nil
}

// NetworkDisconnect disconnects a container from a network.
func (s *BaseServer) NetworkDisconnect(ref string, req *api.NetworkDisconnectRequest) error {
	net, ok := s.Store.ResolveNetwork(ref)
	if !ok {
		return &api.NotFoundError{Resource: "network", ID: ref}
	}

	containerID, found := s.Store.ResolveContainerID(req.Container)
	if !found {
		if req.Force {
			return nil
		}
		return &api.NotFoundError{Resource: "container", ID: req.Container}
	}

	_ = s.Drivers.Network.Disconnect(context.Background(), net.ID, containerID)
	s.emitEvent("network", "disconnect", net.ID, map[string]string{"container": containerID})
	return nil
}

// NetworkRemove removes a network.
func (s *BaseServer) NetworkRemove(ref string) error {
	n, ok := s.Store.ResolveNetwork(ref)
	if !ok {
		return &api.NotFoundError{Resource: "network", ID: ref}
	}

	if err := s.Drivers.Network.Remove(context.Background(), n.ID); err != nil {
		if strings.Contains(err.Error(), "pre-defined") {
			return &api.ConflictError{
				Message: fmt.Sprintf("%s is a pre-defined network and cannot be removed", n.Name),
			}
		}
		return &api.ServerError{Message: err.Error()}
	}

	s.emitEvent("network", "destroy", n.ID, map[string]string{"name": n.Name})
	return nil
}

// NetworkPrune removes unused networks.
func (s *BaseServer) NetworkPrune(f map[string][]string) (*api.NetworkPruneResponse, error) {
	resp, err := s.Drivers.Network.Prune(context.Background(), f)
	if err != nil {
		return nil, &api.ServerError{Message: err.Error()}
	}
	for _, nid := range resp.NetworksDeleted {
		s.emitEvent("network", "destroy", nid, map[string]string{"name": nid})
	}
	return resp, nil
}

// VolumeCreate creates a volume.
func (s *BaseServer) VolumeCreate(req *api.VolumeCreateRequest) (*api.Volume, error) {
	name := req.Name
	if name == "" {
		name = GenerateID()[:12]
	}

	if v, ok := s.Store.Volumes.Get(name); ok {
		return &v, nil
	}

	driver := req.Driver
	if driver == "" {
		driver = "local"
	}
	labels := req.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	options := req.DriverOpts
	if options == nil {
		options = make(map[string]string)
	}

	vol := api.Volume{
		Name:       name,
		Driver:     driver,
		Mountpoint: fmt.Sprintf("/var/lib/sockerless/volumes/%s/_data", name),
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		Labels:     labels,
		Scope:      "local",
		Options:    options,
	}

	dir, err := os.MkdirTemp("", "vol-"+name+"-*")
	if err == nil {
		s.Store.VolumeDirs.Store(name, dir)
		vol.Mountpoint = dir
	}

	s.Store.Volumes.Put(name, vol)
	s.emitEvent("volume", "create", name, map[string]string{"driver": driver})
	return &vol, nil
}

// VolumeList lists volumes.
func (s *BaseServer) VolumeList(f map[string][]string) (*api.VolumeListResponse, error) {
	var inUseNames map[string]bool
	if _, hasDangling := f["dangling"]; hasDangling {
		inUseNames = make(map[string]bool)
		for _, c := range s.Store.Containers.List() {
			for _, m := range c.Mounts {
				if m.Name != "" {
					inUseNames[m.Name] = true
				}
			}
		}
	}

	var vols []*api.Volume
	for _, v := range s.Store.Volumes.List() {
		if !MatchVolumeFilters(v, f) {
			continue
		}
		if danglingVals, ok := f["dangling"]; ok {
			wantDangling := danglingVals[0] == "true" || danglingVals[0] == "1"
			isDangling := !inUseNames[v.Name]
			if wantDangling != isDangling {
				continue
			}
		}
		v := v
		vols = append(vols, &v)
	}
	if vols == nil {
		vols = []*api.Volume{}
	}
	return &api.VolumeListResponse{
		Volumes:  vols,
		Warnings: []string{},
	}, nil
}

// VolumeInspect returns details about a volume.
func (s *BaseServer) VolumeInspect(name string) (*api.Volume, error) {
	vol, ok := s.Store.Volumes.Get(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "volume", ID: name}
	}
	return &vol, nil
}

// VolumeRemove removes a volume.
func (s *BaseServer) VolumeRemove(name string, force bool) error {
	if _, ok := s.Store.Volumes.Get(name); !ok {
		return &api.NotFoundError{Resource: "volume", ID: name}
	}

	if !force {
		for _, c := range s.Store.Containers.List() {
			for _, m := range c.Mounts {
				if m.Name == name {
					cShort := c.ID
					if len(cShort) > 12 {
						cShort = cShort[:12]
					}
					return &api.ConflictError{
						Message: fmt.Sprintf("volume is in use - [%s]", cShort),
					}
				}
			}
		}
	}

	vol, _ := s.Store.Volumes.Get(name)
	s.Store.Volumes.Delete(name)
	if dir, ok := s.Store.VolumeDirs.LoadAndDelete(name); ok {
		os.RemoveAll(dir.(string))
	}
	s.emitEvent("volume", "destroy", name, map[string]string{"driver": vol.Driver})
	return nil
}

// VolumePrune removes unused volumes.
func (s *BaseServer) VolumePrune(f map[string][]string) (*api.VolumePruneResponse, error) {
	labelFilters := f["label"]
	untilFilters := f["until"]

	inUseNames := make(map[string]bool)
	for _, c := range s.Store.Containers.List() {
		for _, m := range c.Mounts {
			if m.Name != "" {
				inUseNames[m.Name] = true
			}
		}
		for _, bind := range c.HostConfig.Binds {
			parts := strings.SplitN(bind, ":", 3)
			if len(parts) >= 2 && !strings.HasPrefix(parts[0], "/") {
				inUseNames[parts[0]] = true
			}
		}
	}

	pruned := s.Store.Volumes.PruneIf(func(_ string, v api.Volume) bool {
		if inUseNames[v.Name] {
			return false
		}
		if len(labelFilters) > 0 && !MatchLabels(v.Labels, labelFilters) {
			return false
		}
		if len(untilFilters) > 0 && !MatchUntil(v.CreatedAt, untilFilters) {
			return false
		}
		return true
	})

	var spaceReclaimed uint64
	deleted := make([]string, 0, len(pruned))
	for _, v := range pruned {
		if dir, ok := s.Store.VolumeDirs.LoadAndDelete(v.Name); ok {
			spaceReclaimed += uint64(DirSize(dir.(string)))
			os.RemoveAll(dir.(string))
		}
		s.emitEvent("volume", "destroy", v.Name, map[string]string{"driver": v.Driver})
		deleted = append(deleted, v.Name)
	}

	return &api.VolumePruneResponse{
		VolumesDeleted: deleted,
		SpaceReclaimed: spaceReclaimed,
	}, nil
}

// SystemEvents returns a stream of events.
func (s *BaseServer) SystemEvents(opts api.EventsOptions) (io.ReadCloser, error) {
	sinceTS := parseEventTimestamp(opts.Since)
	untilTS := parseEventTimestamp(opts.Until)

	typeFilter := opts.Filters["type"]
	actionFilter := opts.Filters["action"]
	containerFilter := opts.Filters["container"]
	labelFilter := opts.Filters["label"]

	matchEvent := func(event api.Event) bool {
		if !matchEventFilter(typeFilter, event.Type) {
			return false
		}
		if !matchEventFilter(actionFilter, event.Action) {
			return false
		}
		if len(containerFilter) > 0 {
			matched := false
			for _, cf := range containerFilter {
				if event.Actor.ID == cf || event.Actor.Attributes["name"] == cf {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
		if len(labelFilter) > 0 && !MatchLabels(event.Actor.Attributes, labelFilter) {
			return false
		}
		return true
	}

	pr, pw := io.Pipe()
	go func() {
		enc := json.NewEncoder(pw)

		// Replay historical events
		if sinceTS > 0 {
			for _, event := range s.EventBus.History(sinceTS, untilTS) {
				if matchEvent(event) {
					_ = enc.Encode(event)
				}
			}
			if untilTS > 0 && untilTS <= time.Now().Unix() {
				_ = pw.Close()
				return
			}
		}

		subID := GenerateID()[:16]
		ch := s.EventBus.Subscribe(subID)
		defer s.EventBus.Unsubscribe(subID)

		var untilCh <-chan time.Time
		if untilTS > 0 {
			d := time.Until(time.Unix(untilTS, 0))
			if d > 0 {
				untilCh = time.After(d)
			} else {
				_ = pw.Close()
				return
			}
		}

		for {
			select {
			case event, ok := <-ch:
				if !ok {
					_ = pw.Close()
					return
				}
				if matchEvent(event) {
					if err := enc.Encode(event); err != nil {
						pw.CloseWithError(err)
						return
					}
				}
			case <-func() <-chan time.Time {
				if untilCh != nil {
					return untilCh
				}
				return nil
			}():
				_ = pw.Close()
				return
			}
		}
	}()

	return pr, nil
}

// SystemDf returns disk usage information.
func (s *BaseServer) SystemDf() (*api.DiskUsageResponse, error) {
	imgContainerCount := make(map[string]int64)
	for _, c := range s.Store.Containers.List() {
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			imgContainerCount[img.ID]++
		}
	}

	var images []*api.ImageSummary
	seen := make(map[string]bool)
	for _, img := range s.Store.Images.List() {
		if seen[img.ID] {
			continue
		}
		seen[img.ID] = true
		created, _ := time.Parse(time.RFC3339Nano, img.Created)
		images = append(images, &api.ImageSummary{
			ID:          img.ID,
			RepoTags:    img.RepoTags,
			RepoDigests: img.RepoDigests,
			Created:     created.Unix(),
			Size:        img.Size,
			VirtualSize: img.VirtualSize,
			Labels:      img.Config.Labels,
			Containers:  imgContainerCount[img.ID],
		})
	}

	var containers []*api.ContainerSummary
	for _, c := range s.Store.Containers.List() {
		created, _ := time.Parse(time.RFC3339Nano, c.Created)
		command := c.Path
		if len(c.Args) > 0 {
			command += " " + strings.Join(c.Args, " ")
		}
		imageID := ""
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			imageID = img.ID
		} else {
			h := sha256.Sum256([]byte(c.Config.Image))
			imageID = fmt.Sprintf("sha256:%x", h)
		}
		labels := c.Config.Labels
		if labels == nil {
			labels = make(map[string]string)
		}
		mounts := c.Mounts
		if mounts == nil {
			mounts = []api.MountPoint{}
		}
		cs := &api.ContainerSummary{
			ID:      c.ID,
			Names:   []string{c.Name},
			Image:   c.Config.Image,
			ImageID: imageID,
			Command: command,
			Created: created.Unix(),
			State:   c.State.Status,
			Status:  FormatStatus(c.State),
			Labels:  labels,
			Ports:   buildPortList(c.HostConfig.PortBindings, c.Config.ExposedPorts),
			Mounts:  mounts,
			NetworkSettings: &api.SummaryNetworkSettings{Networks: c.NetworkSettings.Networks},
			HostConfig:      &api.HostConfigSummary{NetworkMode: c.HostConfig.NetworkMode},
		}
		if rootPath, err := s.Drivers.Filesystem.RootPath(c.ID); err == nil && rootPath != "" {
			cs.SizeRw = DirSize(rootPath)
		}
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			cs.SizeRootFs = img.Size
		}
		containers = append(containers, cs)
	}

	volRefCount := make(map[string]int64)
	for _, c := range s.Store.Containers.List() {
		for _, m := range c.Mounts {
			if m.Name != "" {
				volRefCount[m.Name]++
			}
		}
	}

	var volumes []*api.Volume
	for _, v := range s.Store.Volumes.List() {
		vCopy := v
		size := int64(-1)
		if dir, ok := s.Store.VolumeDirs.Load(v.Name); ok {
			size = DirSize(dir.(string))
			vCopy.Status = map[string]any{"Size": size}
		}
		vCopy.UsageData = &api.VolumeUsageData{
			RefCount: volRefCount[v.Name],
			Size:     size,
		}
		volumes = append(volumes, &vCopy)
	}

	return &api.DiskUsageResponse{
		Images:     images,
		Containers: containers,
		Volumes:    volumes,
		BuildCache: []*api.BuildCache{},
	}, nil
}

// matchImageFilters checks whether an image matches the given filter map.
func (s *BaseServer) matchImageFilters(img *api.Image, filters map[string][]string) bool {
	if refs := filters["reference"]; len(refs) > 0 {
		matched := false
		for _, ref := range refs {
			for _, tag := range img.RepoTags {
				if m, _ := path.Match(ref, tag); m {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}

	if df := filters["dangling"]; len(df) > 0 {
		isDangling := true
		for _, tag := range img.RepoTags {
			if !strings.Contains(tag, "<none>") {
				isDangling = false
				break
			}
		}
		wantDangling := df[0] == "true" || df[0] == "1"
		if wantDangling != isDangling {
			return false
		}
	}

	if lf := filters["label"]; len(lf) > 0 && !MatchLabels(img.Config.Labels, lf) {
		return false
	}

	if bf := filters["before"]; len(bf) > 0 {
		for _, val := range bf {
			if refImg, ok := s.Store.ResolveImage(val); ok {
				refTime, _ := time.Parse(time.RFC3339Nano, refImg.Created)
				imgTime, _ := time.Parse(time.RFC3339Nano, img.Created)
				if !imgTime.Before(refTime) {
					return false
				}
			}
		}
	}

	if sf := filters["since"]; len(sf) > 0 {
		for _, val := range sf {
			if refImg, ok := s.Store.ResolveImage(val); ok {
				refTime, _ := time.Parse(time.RFC3339Nano, refImg.Created)
				imgTime, _ := time.Parse(time.RFC3339Nano, img.Created)
				if !imgTime.After(refTime) {
					return false
				}
			}
		}
	}

	return true
}

// --- Helper types ---

// pipeRWC wraps an io.Pipe pair as an io.ReadWriteCloser.
type pipeRWC struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

func (p *pipeRWC) Read(b []byte) (int, error)  { return p.reader.Read(b) }
func (p *pipeRWC) Write(b []byte) (int, error) { return p.writer.Write(b) }
func (p *pipeRWC) Close() error {
	_ = p.writer.Close()
	return p.reader.Close()
}

// pipeConn wraps an io.Pipe pair as a net.Conn for use with driver Attach/Exec methods.
type pipeConn struct {
	*io.PipeReader
	*io.PipeWriter
}

func (p *pipeConn) Close() error {
	_ = p.PipeWriter.Close()
	return p.PipeReader.Close()
}
func (p *pipeConn) LocalAddr() net.Addr                { return pipeAddr{} }
func (p *pipeConn) RemoteAddr() net.Addr               { return pipeAddr{} }
func (p *pipeConn) SetDeadline(_ time.Time) error      { return nil }
func (p *pipeConn) SetReadDeadline(_ time.Time) error  { return nil }
func (p *pipeConn) SetWriteDeadline(_ time.Time) error { return nil }

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }
