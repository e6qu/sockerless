package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockerimage "github.com/docker/docker/api/types/image"
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
		config.AttachStdin = cc.AttachStdin
		config.AttachStdout = cc.AttachStdout
		config.AttachStderr = cc.AttachStderr
		config.WorkingDir = cc.WorkingDir
		config.Entrypoint = cc.Entrypoint
		config.User = cc.User
		config.Hostname = cc.Hostname
		config.StopSignal = cc.StopSignal
	}

	var hostConfig *container.HostConfig
	if req.HostConfig != nil {
		hostConfig = &container.HostConfig{
			NetworkMode: container.NetworkMode(req.HostConfig.NetworkMode),
			Binds:       req.HostConfig.Binds,
			AutoRemove:  req.HostConfig.AutoRemove,
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
	}

	if info.Config != nil {
		c.Config = api.ContainerConfig{
			Hostname:     info.Config.Hostname,
			Domainname:   info.Config.Domainname,
			User:         info.Config.User,
			AttachStdin:  info.Config.AttachStdin,
			AttachStdout: info.Config.AttachStdout,
			AttachStderr: info.Config.AttachStderr,
			Tty:          info.Config.Tty,
			OpenStdin:    info.Config.OpenStdin,
			StdinOnce:    info.Config.StdinOnce,
			Env:          info.Config.Env,
			Cmd:          info.Config.Cmd,
			Image:        info.Config.Image,
			WorkingDir:   info.Config.WorkingDir,
			Entrypoint:   info.Config.Entrypoint,
			Labels:       info.Config.Labels,
			StopSignal:   info.Config.StopSignal,
		}
	}

	if info.HostConfig != nil {
		c.HostConfig = api.HostConfig{
			NetworkMode: string(info.HostConfig.NetworkMode),
			Binds:       info.HostConfig.Binds,
			AutoRemove:  info.HostConfig.AutoRemove,
		}
	}

	c.NetworkSettings.Networks = make(map[string]*api.EndpointSettings)
	if info.NetworkSettings != nil && info.NetworkSettings.Networks != nil {
		for name, ep := range info.NetworkSettings.Networks {
			c.NetworkSettings.Networks[name] = &api.EndpointSettings{
				NetworkID:   ep.NetworkID,
				EndpointID:  ep.EndpointID,
				Gateway:     ep.Gateway,
				IPAddress:   ep.IPAddress,
				IPPrefixLen: ep.IPPrefixLen,
				MacAddress:  ep.MacAddress,
			}
		}
	}

	c.Mounts = make([]api.MountPoint, 0)
	return c
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
