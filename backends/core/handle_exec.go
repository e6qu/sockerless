package core

import (
	"encoding/binary"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// muxWriter wraps an io.Writer to add Docker multiplexed stream headers.
// When TTY is false, Docker clients expect each chunk to be prefixed with
// an 8-byte header: [stream_type, 0, 0, 0, size_big_endian(4)].
type muxWriter struct {
	w          io.Writer
	streamType byte // 1 = stdout, 2 = stderr
}

func (m *muxWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	var header [8]byte
	header[0] = m.streamType
	binary.BigEndian.PutUint32(header[4:], uint32(len(p)))
	if _, err := m.w.Write(header[:]); err != nil {
		return 0, err
	}
	return m.w.Write(p)
}

// --- Common exec handlers ---

func (s *BaseServer) handleExecCreate(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		// Allow exec on exited containers that use synthetic exec (no agent).
		// FaaS backends stop containers immediately after invoke, but exec
		// can still work via the synthetic fallback.
		if c.AgentAddress != "" || c.State.Status == "" {
			WriteError(w, &api.ConflictError{
				Message: "Container " + ref + " is not running",
			})
			return
		}
	}

	var req api.ExecCreateRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	// BUG-503: Validate that Cmd is not empty
	if len(req.Cmd) == 0 {
		WriteError(w, &api.InvalidParameterError{
			Message: "No exec command specified",
		})
		return
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

	// Add exec ID to container
	s.Store.Containers.Update(id, func(c *api.Container) {
		c.ExecIDs = append(c.ExecIDs, execID)
	})

	WriteJSON(w, http.StatusCreated, api.ExecCreateResponse{ID: execID})
}

func (s *BaseServer) handleExecInspect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	exec, ok := s.Store.Execs.Get(id)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "exec instance", ID: id})
		return
	}
	WriteJSON(w, http.StatusOK, exec)
}

// --- Default exec start (delegates to s.self for virtual dispatch) ---

func (s *BaseServer) handleExecStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req api.ExecStartRequest
	_ = ReadJSON(r, &req)

	// Determine TTY before starting (for HTTP upgrade framing)
	exec, ok := s.Store.Execs.Get(id)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "exec instance", ID: id})
		return
	}
	tty := exec.ProcessConfig.Tty || req.Tty

	// Hijack the connection
	hj, ok := w.(http.Hijacker)
	if !ok {
		WriteError(w, &api.ServerError{Message: "hijacking not supported"})
		return
	}

	conn, buf, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()

	contentType := "application/vnd.docker.multiplexed-stream"
	if tty {
		contentType = "application/vnd.docker.raw-stream"
	}

	buf.WriteString("HTTP/1.1 101 UPGRADED\r\n")
	buf.WriteString("Content-Type: " + contentType + "\r\n")
	buf.WriteString("Connection: Upgrade\r\n")
	buf.WriteString("Upgrade: tcp\r\n")
	buf.WriteString("\r\n")
	buf.Flush()

	rwc, err := s.self.ExecStart(id, req)
	if err != nil {
		// Already hijacked — write error inline
		_, _ = conn.Write([]byte(err.Error()))
		return
	}
	defer rwc.Close()

	// Copy data between the exec stream and the hijacked connection
	done := make(chan struct{})
	go func() {
		io.Copy(conn, rwc)
		close(done)
	}()
	go func() {
		io.Copy(rwc, conn)
	}()
	<-done
}

// scheduleExecAutoStop stops a synthetic container after a grace period,
// provided all its exec instances have completed.
func (s *BaseServer) scheduleExecAutoStop(containerID string) {
	time.Sleep(500 * time.Millisecond)
	c, ok := s.Store.Containers.Get(containerID)
	if !ok || !c.State.Running {
		return
	}
	for _, eid := range c.ExecIDs {
		e, ok := s.Store.Execs.Get(eid)
		if ok && e.Running {
			return
		}
	}
	s.emitEvent("container", "die", containerID, map[string]string{
		"exitCode": "0",
		"name":     strings.TrimPrefix(c.Name, "/"),
	})
	s.Store.StopContainer(containerID, 0)
}

// mergeEnv merges base env vars with override env vars. Override values
// replace base values with the same key.
func mergeEnv(base, override []string) []string {
	if len(override) == 0 {
		return base
	}
	env := make(map[string]string)
	for _, e := range base {
		if i := strings.IndexByte(e, '='); i >= 0 {
			env[e[:i]] = e[i+1:]
		}
	}
	for _, e := range override {
		if i := strings.IndexByte(e, '='); i >= 0 {
			env[e[:i]] = e[i+1:]
		}
	}
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}
