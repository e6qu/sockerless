package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sockerless/api"
)

func (s *Server) handleContainerCreate(w http.ResponseWriter, r *http.Request) {
	var req api.ContainerCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	name := r.URL.Query().Get("name")

	config := &container.Config{}
	if req.ContainerConfig != nil {
		cc := req.ContainerConfig
		config.Image = cc.Image
		config.Cmd = cc.Cmd
		config.Env = cc.Env
		config.Labels = cc.Labels
		config.Tty = cc.Tty
		config.OpenStdin = cc.OpenStdin
		config.StdinOnce = cc.StdinOnce
		config.AttachStdin = cc.AttachStdin
		config.AttachStdout = cc.AttachStdout
		config.AttachStderr = cc.AttachStderr
		config.WorkingDir = cc.WorkingDir
		config.Entrypoint = cc.Entrypoint
		config.User = cc.User
		config.Hostname = cc.Hostname
		config.Domainname = cc.Domainname
		config.StopSignal = cc.StopSignal
		config.StopTimeout = cc.StopTimeout
		config.Shell = cc.Shell
		config.Volumes = cc.Volumes
		if len(cc.ExposedPorts) > 0 {
			config.ExposedPorts = make(nat.PortSet, len(cc.ExposedPorts))
			for p := range cc.ExposedPorts {
				config.ExposedPorts[nat.Port(p)] = struct{}{}
			}
		}
		if cc.Healthcheck != nil {
			config.Healthcheck = &container.HealthConfig{
				Test:        cc.Healthcheck.Test,
				Interval:    time.Duration(cc.Healthcheck.Interval),
				Timeout:     time.Duration(cc.Healthcheck.Timeout),
				StartPeriod: time.Duration(cc.Healthcheck.StartPeriod),
				Retries:     cc.Healthcheck.Retries,
			}
		}
	}

	var hostConfig *container.HostConfig
	if req.HostConfig != nil {
		hostConfig = &container.HostConfig{
			NetworkMode: container.NetworkMode(req.HostConfig.NetworkMode),
			Binds:       req.HostConfig.Binds,
			AutoRemove:  req.HostConfig.AutoRemove,
			Privileged:  req.HostConfig.Privileged,
			CapAdd:      req.HostConfig.CapAdd,
			CapDrop:     req.HostConfig.CapDrop,
			Init:        req.HostConfig.Init,
			UsernsMode:  container.UsernsMode(req.HostConfig.UsernsMode),
			ShmSize:     req.HostConfig.ShmSize,
			Tmpfs:       req.HostConfig.Tmpfs,
			SecurityOpt: req.HostConfig.SecurityOpt,
			ExtraHosts:  req.HostConfig.ExtraHosts,
			Isolation:   container.Isolation(req.HostConfig.Isolation),
			RestartPolicy: container.RestartPolicy{
				Name:              container.RestartPolicyMode(req.HostConfig.RestartPolicy.Name),
				MaximumRetryCount: req.HostConfig.RestartPolicy.MaximumRetryCount,
			},
		}
		if len(req.HostConfig.PortBindings) > 0 {
			hostConfig.PortBindings = make(nat.PortMap, len(req.HostConfig.PortBindings))
			for port, bindings := range req.HostConfig.PortBindings {
				var nb []nat.PortBinding
				for _, b := range bindings {
					nb = append(nb, nat.PortBinding{HostIP: b.HostIP, HostPort: b.HostPort})
				}
				hostConfig.PortBindings[nat.Port(port)] = nb
			}
		}
		if req.HostConfig.LogConfig != nil {
			hostConfig.LogConfig = container.LogConfig{
				Type:   req.HostConfig.LogConfig.Type,
				Config: req.HostConfig.LogConfig.Config,
			}
		}
		for _, m := range req.HostConfig.Mounts {
			dm := mount.Mount{
				Type:     mount.Type(m.Type),
				Source:   m.Source,
				Target:   m.Target,
				ReadOnly: m.ReadOnly,
			}
			if m.BindOptions != nil {
				dm.BindOptions = &mount.BindOptions{
					Propagation: mount.Propagation(m.BindOptions.Propagation),
				}
			}
			hostConfig.Mounts = append(hostConfig.Mounts, dm)
		}
	}

	// Auto-pull image if needed
	_, _, err := s.docker.ImageInspectWithRaw(r.Context(), config.Image)
	if err != nil {
		rc, pullErr := s.docker.ImagePull(r.Context(), config.Image, dockerimage.PullOptions{})
		if pullErr != nil {
			writeError(w, &api.NotFoundError{Resource: "image", ID: config.Image})
			return
		}
		io.Copy(io.Discard, rc)
		rc.Close()
	}

	resp, err := s.docker.ContainerCreate(r.Context(), config, hostConfig, nil, (*ocispec.Platform)(nil), name)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	warnings := resp.Warnings
	if warnings == nil {
		warnings = []string{}
	}

	writeJSON(w, http.StatusCreated, api.ContainerCreateResponse{
		ID:       resp.ID,
		Warnings: warnings,
	})
}

func (s *Server) handleContainerList(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"

	containers, err := s.docker.ContainerList(r.Context(), container.ListOptions{All: all})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	result := make([]*api.ContainerSummary, 0, len(containers))
	for _, c := range containers {
		summary := &api.ContainerSummary{
			ID:      c.ID,
			Names:   c.Names,
			Image:   c.Image,
			ImageID: c.ImageID,
			Command: c.Command,
			Created: c.Created,
			State:   c.State,
			Status:  c.Status,
			Labels:  c.Labels,
			SizeRw:  c.SizeRw,
			Mounts:  mapMountsFromSummary(c.Mounts),
		}
		for _, p := range c.Ports {
			summary.Ports = append(summary.Ports, api.Port{
				IP:          p.IP,
				PrivatePort: p.PrivatePort,
				PublicPort:  p.PublicPort,
				Type:        p.Type,
			})
		}
		if c.NetworkSettings != nil && len(c.NetworkSettings.Networks) > 0 {
			nets := make(map[string]*api.EndpointSettings, len(c.NetworkSettings.Networks))
			for name, ep := range c.NetworkSettings.Networks {
				nets[name] = &api.EndpointSettings{
					NetworkID:   ep.NetworkID,
					EndpointID:  ep.EndpointID,
					Gateway:     ep.Gateway,
					IPAddress:   ep.IPAddress,
					IPPrefixLen: ep.IPPrefixLen,
					MacAddress:  ep.MacAddress,
				}
			}
			summary.NetworkSettings = &api.SummaryNetworkSettings{Networks: nets}
		}
		result = append(result, summary)
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleContainerInspect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	info, err := s.docker.ContainerInspect(r.Context(), id)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	c := mapContainerFromDocker(info)
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleContainerStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.docker.ContainerStart(r.Context(), id, container.StartOptions{})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var timeout *int
	if t := r.URL.Query().Get("t"); t != "" {
		v, _ := strconv.Atoi(t)
		timeout = &v
	}
	err := s.docker.ContainerStop(r.Context(), id, container.StopOptions{Timeout: timeout})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerKill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	signal := r.URL.Query().Get("signal")
	if signal == "" {
		signal = "SIGKILL"
	}
	err := s.docker.ContainerKill(r.Context(), id, signal)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	removeVolumes := r.URL.Query().Get("v") == "1" || r.URL.Query().Get("v") == "true"

	err := s.docker.ContainerRemove(r.Context(), id, container.RemoveOptions{
		Force:         force,
		RemoveVolumes: removeVolumes,
	})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rc, err := s.docker.ContainerLogs(r.Context(), id, container.LogsOptions{
		ShowStdout: r.URL.Query().Get("stdout") == "1" || r.URL.Query().Get("stdout") == "true",
		ShowStderr: r.URL.Query().Get("stderr") == "1" || r.URL.Query().Get("stderr") == "true",
		Follow:     r.URL.Query().Get("follow") == "1" || r.URL.Query().Get("follow") == "true",
		Timestamps: r.URL.Query().Get("timestamps") == "1" || r.URL.Query().Get("timestamps") == "true",
		Tail:       r.URL.Query().Get("tail"),
	})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, rc)
}

func (s *Server) handleContainerWait(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	condition := r.URL.Query().Get("condition")
	if condition == "" {
		condition = "not-running"
	}

	waitCh, errCh := s.docker.ContainerWait(r.Context(), id, container.WaitCondition(condition))
	select {
	case result := <-waitCh:
		resp := api.ContainerWaitResponse{StatusCode: int(result.StatusCode)}
		if result.Error != nil {
			resp.Error = &api.WaitError{Message: result.Error.Message}
		}
		writeJSON(w, http.StatusOK, resp)
	case err := <-errCh:
		writeError(w, mapDockerError(err))
	case <-r.Context().Done():
		return
	}
}

func (s *Server) handleContainerAttach(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	resp, err := s.docker.ContainerAttach(r.Context(), id, container.AttachOptions{
		Stream: true,
		Stdin:  r.URL.Query().Get("stdin") == "1",
		Stdout: r.URL.Query().Get("stdout") != "0",
		Stderr: r.URL.Query().Get("stderr") != "0",
	})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		resp.Close()
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	conn, buf, err := hj.Hijack()
	if err != nil {
		resp.Close()
		return
	}
	defer conn.Close()
	defer resp.Close()

	buf.WriteString("HTTP/1.1 101 UPGRADED\r\n")
	buf.WriteString("Content-Type: application/vnd.docker.multiplexed-stream\r\n")
	buf.WriteString("Connection: Upgrade\r\n")
	buf.WriteString("Upgrade: tcp\r\n")
	buf.WriteString("\r\n")
	buf.Flush()

	io.Copy(conn, resp.Reader)
}

func mapContainerFromDocker(info types.ContainerJSON) api.Container {
	c := api.Container{
		ID:      info.ID,
		Name:    info.Name,
		Created: info.Created,
		Path:    info.Path,
		Args:    info.Args,
		Image:   info.Image,
		Driver:  info.Driver,
	}

	if info.State != nil {
		c.State = api.ContainerState{
			Status:     info.State.Status,
			Running:    info.State.Running,
			Paused:     info.State.Paused,
			Restarting: info.State.Restarting,
			OOMKilled:  info.State.OOMKilled,
			Dead:       info.State.Dead,
			Pid:        info.State.Pid,
			ExitCode:   info.State.ExitCode,
			Error:      info.State.Error,
			StartedAt:  info.State.StartedAt,
			FinishedAt: info.State.FinishedAt,
		}
		if info.State.Health != nil {
			logs := make([]api.HealthLog, 0, len(info.State.Health.Log))
			for _, l := range info.State.Health.Log {
				logs = append(logs, api.HealthLog{
					Start:    l.Start.Format(time.RFC3339Nano),
					End:      l.End.Format(time.RFC3339Nano),
					ExitCode: l.ExitCode,
					Output:   l.Output,
				})
			}
			c.State.Health = &api.HealthState{
				Status:        string(info.State.Health.Status),
				FailingStreak: info.State.Health.FailingStreak,
				Log:           logs,
			}
		}
	}

	if info.Config != nil {
		c.Config = api.ContainerConfig{
			Hostname:     info.Config.Hostname,
			Domainname:   info.Config.Domainname,
			User:         info.Config.User,
			AttachStdin:  info.Config.AttachStdin,
			AttachStdout: info.Config.AttachStdout,
			AttachStderr: info.Config.AttachStderr,
			ExposedPorts: mapExposedPorts(info.Config.ExposedPorts),
			Tty:          info.Config.Tty,
			OpenStdin:    info.Config.OpenStdin,
			StdinOnce:    info.Config.StdinOnce,
			Env:          info.Config.Env,
			Cmd:          info.Config.Cmd,
			Image:        info.Config.Image,
			Volumes:      info.Config.Volumes,
			WorkingDir:   info.Config.WorkingDir,
			Entrypoint:   info.Config.Entrypoint,
			Labels:       info.Config.Labels,
			StopSignal:   info.Config.StopSignal,
			StopTimeout:  info.Config.StopTimeout,
			Shell:        info.Config.Shell,
		}
		if info.Config.Healthcheck != nil {
			c.Config.Healthcheck = &api.HealthcheckConfig{
				Test:        info.Config.Healthcheck.Test,
				Interval:    int64(info.Config.Healthcheck.Interval),
				Timeout:     int64(info.Config.Healthcheck.Timeout),
				StartPeriod: int64(info.Config.Healthcheck.StartPeriod),
				Retries:     info.Config.Healthcheck.Retries,
			}
		}
	}

	if info.HostConfig != nil {
		c.HostConfig = api.HostConfig{
			NetworkMode: string(info.HostConfig.NetworkMode),
			Binds:       info.HostConfig.Binds,
			AutoRemove:  info.HostConfig.AutoRemove,
			PortBindings: mapPortBindings(info.HostConfig.PortBindings),
			RestartPolicy: api.RestartPolicy{
				Name:              string(info.HostConfig.RestartPolicy.Name),
				MaximumRetryCount: info.HostConfig.RestartPolicy.MaximumRetryCount,
			},
			Privileged:  info.HostConfig.Privileged,
			CapAdd:      info.HostConfig.CapAdd,
			CapDrop:     info.HostConfig.CapDrop,
			Init:        info.HostConfig.Init,
			UsernsMode:  string(info.HostConfig.UsernsMode),
			ShmSize:     info.HostConfig.ShmSize,
			Tmpfs:       info.HostConfig.Tmpfs,
			SecurityOpt: info.HostConfig.SecurityOpt,
			ExtraHosts:  info.HostConfig.ExtraHosts,
			Isolation:   string(info.HostConfig.Isolation),
		}
		if info.HostConfig.LogConfig.Type != "" {
			c.HostConfig.LogConfig = &api.LogConfig{
				Type:   info.HostConfig.LogConfig.Type,
				Config: info.HostConfig.LogConfig.Config,
			}
		}
		for _, m := range info.HostConfig.Mounts {
			am := api.Mount{
				Type:     string(m.Type),
				Source:   m.Source,
				Target:   m.Target,
				ReadOnly: m.ReadOnly,
			}
			if m.BindOptions != nil {
				am.BindOptions = &api.BindOptions{
					Propagation: string(m.BindOptions.Propagation),
				}
			}
			c.HostConfig.Mounts = append(c.HostConfig.Mounts, am)
		}
	}

	c.NetworkSettings.Networks = make(map[string]*api.EndpointSettings)
	if info.NetworkSettings != nil {
		if info.NetworkSettings.Networks != nil {
			for name, ep := range info.NetworkSettings.Networks {
				c.NetworkSettings.Networks[name] = &api.EndpointSettings{
					NetworkID:   ep.NetworkID,
					EndpointID:  ep.EndpointID,
					Gateway:     ep.Gateway,
					IPAddress:   ep.IPAddress,
					IPPrefixLen: ep.IPPrefixLen,
					MacAddress:  ep.MacAddress,
					Aliases:     ep.Aliases,
				}
			}
		}
		if len(info.NetworkSettings.Ports) > 0 {
			c.NetworkSettings.Ports = mapPortBindings(info.NetworkSettings.Ports)
		}
	}

	c.Mounts = make([]api.MountPoint, 0, len(info.Mounts))
	for _, m := range info.Mounts {
		c.Mounts = append(c.Mounts, api.MountPoint{
			Type:        string(m.Type),
			Name:        m.Name,
			Source:      m.Source,
			Destination: m.Destination,
			Driver:      m.Driver,
			Mode:        m.Mode,
			RW:          m.RW,
			Propagation: string(m.Propagation),
		})
	}
	return c
}

func mapExposedPorts(ports nat.PortSet) map[string]struct{} {
	if len(ports) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(ports))
	for port := range ports {
		result[string(port)] = struct{}{}
	}
	return result
}

func mapPortBindings(pb nat.PortMap) map[string][]api.PortBinding {
	if len(pb) == 0 {
		return nil
	}
	result := make(map[string][]api.PortBinding, len(pb))
	for port, bindings := range pb {
		var mapped []api.PortBinding
		for _, b := range bindings {
			mapped = append(mapped, api.PortBinding{
				HostIP:   b.HostIP,
				HostPort: b.HostPort,
			})
		}
		result[string(port)] = mapped
	}
	return result
}

func mapMountsFromSummary(mounts []types.MountPoint) []api.MountPoint {
	result := make([]api.MountPoint, 0, len(mounts))
	for _, m := range mounts {
		result = append(result, api.MountPoint{
			Type:        string(m.Type),
			Name:        m.Name,
			Source:      m.Source,
			Destination: m.Destination,
			Driver:      m.Driver,
			Mode:        m.Mode,
			RW:          m.RW,
			Propagation: string(m.Propagation),
		})
	}
	return result
}

func mapDockerError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()

	if strings.Contains(msg, "No such") || strings.Contains(msg, "not found") {
		return &api.NotFoundError{Resource: "resource", ID: ""}
	}
	if strings.Contains(msg, "is already") || strings.Contains(msg, "Conflict") || strings.Contains(msg, "conflict") {
		return &api.ConflictError{Message: msg}
	}
	if strings.Contains(msg, "not modified") || strings.Contains(msg, "Not Modified") {
		return &api.NotModifiedError{}
	}

	// Try to extract JSON error from Docker API
	var dockerErr struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(msg), &dockerErr) == nil && dockerErr.Message != "" {
		msg = dockerErr.Message
	}

	return fmt.Errorf("%s", msg)
}

// Prevent unused import error
var _ = context.Background
