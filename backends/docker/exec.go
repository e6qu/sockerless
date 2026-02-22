package docker

import (
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
	info, err := s.docker.ContainerExecInspect(r.Context(), id)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	exec := api.ExecInstance{
		ID:          info.ExecID,
		ContainerID: info.ContainerID,
		Running:     info.Running,
		ExitCode:    info.ExitCode,
		Pid:         info.Pid,
	}
	writeJSON(w, http.StatusOK, exec)
}

func (s *Server) handleExecStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req api.ExecStartRequest
	_ = readJSON(r, &req)

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

	buf.WriteString("HTTP/1.1 101 UPGRADED\r\n")
	buf.WriteString("Content-Type: application/vnd.docker.multiplexed-stream\r\n")
	buf.WriteString("Connection: Upgrade\r\n")
	buf.WriteString("Upgrade: tcp\r\n")
	buf.WriteString("\r\n")
	buf.Flush()

	io.Copy(conn, resp.Reader)
}
