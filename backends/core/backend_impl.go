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
		KernelVersion:     s.kernelVersion(),
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
		refID, _ = s.ResolveContainerIDAuto(context.Background(), refID)
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
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}
	return &c, nil
}

// ContainerList lists containers matching options.
func (s *BaseServer) ContainerList(opts api.ContainerListOptions) ([]*api.ContainerSummary, error) {
	// Collect containers from CloudState or Store
	var containers []api.Container
	if s.CloudState != nil {
		cloudContainers, err := s.CloudState.ListContainers(context.Background(), opts.All, opts.Filters)
		if err == nil {
			containers = cloudContainers
		}
		// Include pending creates (not yet in cloud)
		if s.PendingCreates != nil {
			for _, pc := range s.PendingCreates.List() {
				if opts.All || pc.State.Running {
					containers = append(containers, pc)
				}
			}
		}
	} else {
		containers = s.Store.Containers.List()
	}

	var result []*api.ContainerSummary
	for _, c := range containers {
		if !opts.All && !c.State.Running {
			continue
		}
		if s.CloudState == nil && len(opts.Filters) > 0 && !MatchContainerFilters(c, opts.Filters) {
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
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID
	if c.State.Running {
		return &api.NotModifiedError{}
	}

	if pod, inPod := s.Store.Pods.GetPodForContainer(id); inPod && len(pod.ContainerIDs) > 1 {
		return &api.InvalidParameterError{
			Message: "multi-container pods are not supported by this backend",
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

	return nil
}

// ContainerStop stops a running container.
func (s *BaseServer) ContainerStop(ref string, timeout *int) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if !c.State.Running {
		return &api.NotModifiedError{}
	}

	s.StopHealthCheck(id)
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

	exitCode := SignalToExitCode(signal)

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
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
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
		s.emitEvent("container", "kill", id, map[string]string{"name": strings.TrimPrefix(c.Name, "/")})
		s.emitEvent("container", "die", id, map[string]string{
			"exitCode": "0",
			"name":     strings.TrimPrefix(c.Name, "/"),
		})
		s.Store.ForceStopContainer(id, 0)
	}

	s.StopHealthCheck(id)

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
	s.Store.PathMappings.Delete(id)
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
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID
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
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		if condition == "removed" {
			return &api.ContainerWaitResponse{StatusCode: 0}, nil
		}
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if condition == "" {
		condition = "not-running"
	}

	if condition != "next-exit" && (c.State.Status == "exited" || c.State.Status == "dead") {
		return &api.ContainerWaitResponse{StatusCode: c.State.ExitCode}, nil
	}

	// CloudState path: poll for exit via CloudState.WaitForExit
	if s.CloudState != nil {
		exitCode, err := s.CloudState.WaitForExit(context.Background(), id)
		if err != nil {
			return nil, err
		}
		return &api.ContainerWaitResponse{StatusCode: exitCode}, nil
	}

	// Store path (Docker passthrough): wait on channel
	ch, ok := s.Store.WaitChs.Load(id)
	if !ok {
		c, _ = s.ResolveContainerAuto(context.Background(), id)
		return &api.ContainerWaitResponse{StatusCode: c.State.ExitCode}, nil
	}

	<-ch.(chan struct{})
	c, _ = s.ResolveContainerAuto(context.Background(), id)
	if condition == "removed" {
		for i := 0; i < 50; i++ {
			if _, found := s.ResolveContainerAuto(context.Background(), id); !found {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return &api.ContainerWaitResponse{StatusCode: c.State.ExitCode}, nil
}

// ContainerAttach establishes a bidirectional stream to the container.
func (s *BaseServer) ContainerAttach(ref string, opts api.ContainerAttachOptions) (io.ReadWriteCloser, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

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
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID
	if c.State.Running {
		s.StopHealthCheck(id)
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

	return nil
}

// ContainerTop returns the running processes inside a container.
func (s *BaseServer) ContainerTop(ref string, psArgs string) (*api.ContainerTopResponse, error) {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if !c.State.Running {
		return nil, &api.ConflictError{
			Message: fmt.Sprintf("Container %s is not running", ref),
		}
	}

	// Try to get real process list via exec driver
	psCmd := "ps aux"
	if psArgs != "" {
		psCmd = "ps " + psArgs
	}
	if resp := s.execForOutput(id, []string{"/bin/sh", "-c", psCmd}); resp != "" {
		if titles, procs := parsePS(resp); len(procs) > 0 {
			return &api.ContainerTopResponse{Titles: titles, Processes: procs}, nil
		}
	}

	return nil, &api.NotImplementedError{Message: "container top is not supported by this backend"}
}

// execForOutput runs a command inside a container and captures stdout.
// Returns empty string on any error (non-fatal, used for best-effort data).
func (s *BaseServer) execForOutput(containerID string, cmd []string) string {
	execID := GenerateID()
	s.Store.Execs.Put(execID, api.ExecInstance{
		ID:          execID,
		ContainerID: containerID,
		Running:     true,
		OpenStdout:  true,
	})
	defer s.Store.Execs.Delete(execID)

	var buf bytes.Buffer
	pr, pw := io.Pipe()
	conn := &pipeConn{PipeReader: pr, PipeWriter: pw}

	go func() {
		s.Drivers.Exec.Exec(context.Background(), containerID, execID, cmd, nil, "", false, conn)
		_ = pw.Close()
	}()

	// Read with timeout
	readDone := make(chan struct{})
	go func() {
		io.Copy(&buf, pr)
		close(readDone)
	}()

	select {
	case <-readDone:
	case <-time.After(5 * time.Second):
		_ = pw.Close()
	}

	return buf.String()
}

// parsePS parses the output of `ps` into titles and process rows.
func parsePS(output string) ([]string, [][]string) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return nil, nil
	}

	// Parse header to determine column positions
	header := lines[0]
	titles := strings.Fields(header)
	if len(titles) == 0 {
		return nil, nil
	}

	var processes [][]string
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Split into at most len(titles) fields — last field may contain spaces (CMD)
		fields := strings.Fields(line)
		if len(fields) < len(titles) {
			continue
		}
		if len(fields) > len(titles) {
			// Join excess fields into the last column (CMD)
			row := make([]string, len(titles))
			copy(row, fields[:len(titles)-1])
			row[len(titles)-1] = strings.Join(fields[len(titles)-1:], " ")
			processes = append(processes, row)
		} else {
			processes = append(processes, fields)
		}
	}

	return titles, processes
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
		s.Store.PathMappings.Delete(c.ID)
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
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID
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
			// Check if container has stopped
			if ch, ok := s.Store.WaitChs.Load(id); ok {
				select {
				case <-ch.(chan struct{}):
					_ = pw.Close()
					return
				default:
					// Still running — continue streaming
				}
			} else if cur, ok := s.ResolveContainerAuto(context.Background(), id); ok && !cur.State.Running {
				// No WaitCh but CloudState confirms stopped
				_ = pw.Close()
				return
			}
			// If neither WaitCh nor CloudState says stopped, keep streaming
		}
	}()
	return pr, nil
}

// ContainerRename renames a container.
func (s *BaseServer) ContainerRename(ref string, newName string) error {
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if newName == "" {
		return &api.InvalidParameterError{Message: "name is required"}
	}
	if !strings.HasPrefix(newName, "/") {
		newName = "/" + newName
	}

	s.Store.RenameMu.Lock()
	defer s.Store.RenameMu.Unlock()

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
	rc, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := rc.ID

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
	rc, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := rc.ID

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
	c, ok := s.ResolveContainerAuto(context.Background(), ref)
	if !ok {
		return nil, &api.NotFoundError{Resource: "container", ID: ref}
	}
	id := c.ID

	if !c.State.Running {
		return nil, &api.ConflictError{Message: "Container " + ref + " is not running"}
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

	// Resolve via ResolveContainerAuto so stateless cloud backends
	// (ECS, Lambda, Cloud Run) that keep container state in the cloud
	// rather than Store.Containers can still run exec. Falls back to
	// Store for the local backends.
	c, ok := s.ResolveContainerAuto(context.Background(), exec.ContainerID)
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

// ImagePull pulls an image and stores it in the in-memory image store.
// Uses synthetic metadata. Prefer ImagePullWithMetadata when registry data is available.
func (s *BaseServer) ImagePull(ref string, auth string) (io.ReadCloser, error) {
	return s.ImagePullWithMetadata(ref, auth, nil)
}

// ImagePullWithMetadata pulls an image, using real registry metadata when available.
// When meta is nil, falls back to synthetic data for backward compatibility.
// Uses real sizes, digests, and layers from registry.
func (s *BaseServer) ImagePullWithMetadata(ref string, auth string, meta *ImageMetadataResult) (io.ReadCloser, error) {
	if ref == "" {
		return nil, &api.InvalidParameterError{Message: "image reference is required"}
	}

	if !strings.Contains(ref, ":") && !strings.Contains(ref, "@") {
		ref = ref + ":latest"
	}

	if _, exists := s.Store.ResolveImage(ref); exists {
		// Image already exists — re-merge metadata if available
		if meta != nil {
			if img, ok := s.Store.ResolveImage(ref); ok {
				mergeImageConfig(&img.Config, meta.Config)
				StoreImageWithAliases(s.Store, ref, img)
			}
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		_ = enc.Encode(map[string]any{"status": fmt.Sprintf("Pulling from %s", strings.Split(ref, ":")[0])})
		_ = enc.Encode(map[string]any{"status": fmt.Sprintf("Status: Image is up to date for %s", ref)})
		return io.NopCloser(&buf), nil
	}

	// Determine image ID, size, digests, layers from real metadata or synthetic fallback
	var imageID string
	var imgSize int64
	var repoDigests []string
	var layers []string
	var arch, osName string
	var imgConfig api.ContainerConfig
	var created string

	now := time.Now().UTC()

	if meta != nil {
		// Use real config digest as image ID
		imageID = meta.ConfigDigest
		// Use real total size from manifest layers
		imgSize = meta.TotalSize
		// Use real manifest digest for RepoDigests
		repoDigests = []string{strings.Split(ref, ":")[0] + "@" + meta.ManifestDigest}
		// Use real diff_ids from config blob
		layers = meta.LayerDigests
		arch = meta.Architecture
		osName = meta.OS
		imgConfig = *meta.Config
		if meta.Created != "" {
			created = meta.Created
		} else {
			created = now.Format(time.RFC3339Nano)
		}

		// Store real history
		if len(meta.History) > 0 {
			s.Store.ImageHistory.Store(imageID, meta.History)
		}
	} else {
		s.Logger.Warn().Str("ref", ref).Msg("using synthetic image metadata (registry fetch failed)")
		// Synthetic fallback
		hash := sha256.Sum256([]byte(ref))
		imageID = fmt.Sprintf("sha256:%x", hash)
		h := fnv.New32a()
		h.Write([]byte(ref))
		imgSize = int64(10_000_000 + h.Sum32()%90_000_000)
		repoDigests = []string{strings.Split(ref, ":")[0] + "@sha256:" + fmt.Sprintf("%x", hash)[:64]}
		layers = []string{"sha256:" + GenerateID()}
		arch = "amd64"
		osName = "linux"
		imgConfig = api.ContainerConfig{
			Env:    []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"},
			Cmd:    []string{"/bin/sh"},
			Labels: make(map[string]string),
		}
		created = now.Format(time.RFC3339Nano)
	}

	idShort := imageID
	if len(idShort) > 19 {
		idShort = idShort[7:19]
	}

	img := api.Image{
		ID:           imageID,
		RepoTags:     []string{ref},
		RepoDigests:  repoDigests,
		Created:      created,
		Size:         imgSize,
		VirtualSize:  imgSize,
		Architecture: arch,
		Os:           osName,
		Config:       imgConfig,
		RootFS: api.RootFS{
			Type:   "layers",
			Layers: layers,
		},
		GraphDriver: api.GraphDriverData{
			Name: "overlay2",
			Data: map[string]string{
				"MergedDir": "/var/lib/sockerless/overlay2/" + idShort + "/merged",
				"UpperDir":  "/var/lib/sockerless/overlay2/" + idShort + "/diff",
				"WorkDir":   "/var/lib/sockerless/overlay2/" + idShort + "/work",
			},
		},
		Metadata: api.ImageMetadata{LastTagTime: now.Format(time.RFC3339Nano)},
	}

	StoreImageWithAliases(s.Store, ref, img)

	// Use real layer digests for progress when available
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	_ = enc.Encode(map[string]any{"status": fmt.Sprintf("Pulling from %s", strings.Split(ref, ":")[0])})
	for _, layer := range layers {
		layerShort := layer
		if len(layer) > 18 {
			layerShort = layer[7:19]
		}
		_ = enc.Encode(map[string]any{"status": "Pulling fs layer", "id": layerShort})
		_ = enc.Encode(map[string]any{"status": "Download complete", "id": layerShort})
		_ = enc.Encode(map[string]any{"status": "Pull complete", "id": layerShort})
	}
	if meta != nil {
		_ = enc.Encode(map[string]any{"status": fmt.Sprintf("Digest: %s", meta.ManifestDigest)})
	} else {
		hash := sha256.Sum256([]byte(ref))
		_ = enc.Encode(map[string]any{"status": fmt.Sprintf("Digest: sha256:%x", hash)})
	}
	_ = enc.Encode(map[string]any{"status": fmt.Sprintf("Status: Downloaded newer image for %s", ref)})

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
// Preserves layer tarballs in LayerContent store for subsequent push.
func (s *BaseServer) ImageLoad(r io.Reader) (io.ReadCloser, error) {
	result := parseImageTarFull(r)

	var repoTags []string
	var imgConfig *api.ContainerConfig
	if result != nil {
		repoTags = result.RepoTags
		imgConfig = result.Config
	}

	if len(repoTags) == 0 {
		repoTags = []string{"loaded:latest"}
	}

	// Deduplicate: remove existing image with same tags
	for _, tag := range repoTags {
		if existing, ok := s.Store.ResolveImage(tag); ok {
			s.Store.Images.Delete(existing.ID)
		}
	}

	id := "sha256:" + GenerateID()
	nowStr := time.Now().UTC().Format(time.RFC3339Nano)

	// Compute real layer digests and sizes from preserved content.
	var layers []string
	var totalSize int64
	if result != nil && len(result.Layers) > 0 {
		for layerPath, content := range result.Layers {
			digest := fmt.Sprintf("sha256:%x", sha256.Sum256(content))
			layers = append(layers, digest)
			totalSize += int64(len(content))
			// Store layer content for subsequent OCI push
			s.Store.LayerContent.Store(digest, content)
			_ = layerPath
		}
	}
	if len(layers) == 0 {
		layers = []string{"sha256:" + GenerateID()}
	}

	img := api.Image{
		ID:           id,
		RepoTags:     repoTags,
		Created:      nowStr,
		Size:         totalSize,
		VirtualSize:  totalSize,
		Architecture: "amd64",
		Os:           "linux",
		Config: api.ContainerConfig{
			Labels: make(map[string]string),
		},
		RootFS: api.RootFS{
			Type:   "layers",
			Layers: layers,
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
		mergeImageConfig(&img.Config, imgConfig)
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

// ImageList lists images. Phase 89 / BUG-723 step 2: when the backend
// implements CloudImageLister, merge entries from the cloud registry
// (ECR, Artifact Registry, ACR) with the in-memory cache so the
// listing is accurate after a backend restart with an empty cache.
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

	// Merge entries from the cloud registry when the backend supports it.
	// Deduplicated by ID so we don't double-count anything already in the
	// in-memory cache.
	if lister, ok := s.CloudState.(CloudImageLister); ok {
		cloudImages, err := lister.ListImages(context.Background())
		if err != nil {
			s.Logger.Debug().Err(err).Msg("cloud image listing failed, returning cache-only result")
		}
		for _, img := range cloudImages {
			if img == nil || seen[img.ID] {
				continue
			}
			seen[img.ID] = true
			result = append(result, img)
		}
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
// Uses real build history from the registry config when available.
func (s *BaseServer) ImageHistory(name string) ([]*api.ImageHistoryEntry, error) {
	img, ok := s.Store.ResolveImage(name)
	if !ok {
		return nil, &api.NotFoundError{Resource: "image", ID: name}
	}

	// Check for real history data
	if histData, ok := s.Store.ImageHistory.Load(img.ID); ok {
		items := histData.([]ImageHistoryItem)
		var result []*api.ImageHistoryEntry
		layerIdx := len(img.RootFS.Layers) - 1
		for i := len(items) - 1; i >= 0; i-- {
			h := items[i]
			entry := &api.ImageHistoryEntry{
				CreatedBy: h.CreatedBy,
				Comment:   h.Comment,
			}
			if h.Created != "" {
				if t, err := time.Parse(time.RFC3339Nano, h.Created); err == nil {
					entry.Created = t.Unix()
				} else if t, err := time.Parse(time.RFC3339, h.Created); err == nil {
					entry.Created = t.Unix()
				}
			}
			if h.EmptyLayer {
				entry.ID = "<missing>"
				entry.Size = 0
			} else if layerIdx >= 0 {
				entry.ID = img.RootFS.Layers[layerIdx]
				if layerIdx < len(img.RootFS.Layers) {
					entry.Size = img.Size / int64(len(img.RootFS.Layers))
				}
				layerIdx--
			}
			if i == len(items)-1 {
				entry.Tags = img.RepoTags
			}
			result = append(result, entry)
		}
		return result, nil
	}

	// Synthetic fallback
	var result []*api.ImageHistoryEntry
	created, _ := time.Parse(time.RFC3339Nano, img.Created)

	for i, layer := range img.RootFS.Layers {
		entry := &api.ImageHistoryEntry{
			ID:        layer,
			Created:   created.Unix() - int64(len(img.RootFS.Layers)-i),
			CreatedBy: "/bin/sh -c #(nop) ADD file:... in / ",
			Size:      img.Size / int64(len(img.RootFS.Layers)+1),
		}
		result = append(result, entry)
	}

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

	containerID, ok := s.ResolveContainerIDAuto(context.Background(), req.Container)
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

	containerID, found := s.ResolveContainerIDAuto(context.Background(), req.Container)
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
	// Collect all containers: Store + CloudState + PendingCreates, deduplicated
	allContainers := s.collectAllContainers(context.Background())

	imgContainerCount := make(map[string]int64)
	for _, c := range allContainers {
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
	for _, c := range allContainers {
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
			ID:              c.ID,
			Names:           []string{c.Name},
			Image:           c.Config.Image,
			ImageID:         imageID,
			Command:         command,
			Created:         created.Unix(),
			State:           c.State.Status,
			Status:          FormatStatus(c.State),
			Labels:          labels,
			Ports:           buildPortList(c.HostConfig.PortBindings, c.Config.ExposedPorts),
			Mounts:          mounts,
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
	for _, c := range allContainers {
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
