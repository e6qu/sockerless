package docker

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/docker/docker/api/types/container"
	"github.com/sockerless/api"
)

func (s *Server) handleExecCreate(w http.ResponseWriter, r *http.Request) {
	containerID := r.PathValue("id")
	var req api.ExecCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	resp, err := s.docker.ContainerExecCreate(r.Context(), containerID, container.ExecOptions{
		AttachStdin:  req.AttachStdin,
		AttachStdout: req.AttachStdout,
		AttachStderr: req.AttachStderr,
		Tty:          req.Tty,
		Cmd:          req.Cmd,
		Env:          req.Env,
		WorkingDir:   req.WorkingDir,
		User:         req.User,
		Privileged:   req.Privileged,
		DetachKeys:   req.DetachKeys,
	})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	writeJSON(w, http.StatusCreated, api.ExecCreateResponse{ID: resp.ID})
}

func (s *Server) handleExecInspect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Use raw HTTP to get ProcessConfig fields not exposed by the SDK
	resp, err := s.docker.ContainerExecInspect(r.Context(), id)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	exec := api.ExecInstance{
		ID:          resp.ExecID,
		ContainerID: resp.ContainerID,
		Running:     resp.Running,
		ExitCode:    resp.ExitCode,
		Pid:         resp.Pid,
		CanRemove:   !resp.Running,
	}

	// The Docker SDK's ExecInspect struct omits ProcessConfig.
	// Fetch raw JSON via the HTTP API to extract it.
	rawResp, rawErr := s.httpGet(r.Context(), "/exec/"+id+"/json")
	if rawErr == nil {
		defer rawResp.Body.Close()
		var raw struct {
			ProcessConfig *struct {
				Entrypoint string   `json:"entrypoint"`
				Arguments  []string `json:"arguments"`
				Tty        bool     `json:"tty"`
				User       string   `json:"user"`
				Privileged *bool    `json:"privileged,omitempty"`
			} `json:"ProcessConfig"`
		}
		if json.NewDecoder(rawResp.Body).Decode(&raw) == nil && raw.ProcessConfig != nil {
			exec.ProcessConfig = api.ExecProcessConfig{
				Entrypoint: raw.ProcessConfig.Entrypoint,
				Arguments:  raw.ProcessConfig.Arguments,
				Tty:        raw.ProcessConfig.Tty,
				User:       raw.ProcessConfig.User,
				Privileged: raw.ProcessConfig.Privileged,
			}
		}
	}

	writeJSON(w, http.StatusOK, exec)
}

func (s *Server) handleExecStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req api.ExecStartRequest
	_ = readJSON(r, &req)

	if req.Detach {
		err := s.docker.ContainerExecStart(r.Context(), id, container.ExecStartOptions{
			Detach: true,
			Tty:    req.Tty,
		})
		if err != nil {
			writeError(w, mapDockerError(err))
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	resp, err := s.docker.ContainerExecAttach(r.Context(), id, container.ExecAttachOptions{
		Detach: req.Detach,
		Tty:    req.Tty,
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

	contentType := "application/vnd.docker.multiplexed-stream"
	if req.Tty {
		contentType = "application/vnd.docker.raw-stream"
	}

	buf.WriteString("HTTP/1.1 101 UPGRADED\r\n")
	buf.WriteString("Content-Type: " + contentType + "\r\n")
	buf.WriteString("Connection: Upgrade\r\n")
	buf.WriteString("Upgrade: tcp\r\n")
	buf.WriteString("\r\n")
	buf.Flush()

	io.Copy(conn, resp.Reader)
}
