package core

import (
	"net/http"
	"strings"

	"github.com/sockerless/api"
)

// --- Common exec handlers ---

func (s *BaseServer) handleExecCreate(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	c, ok := s.ResolveContainerAuto(r.Context(), ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}
	id := c.ID
	if !c.State.Running {
		WriteError(w, &api.ConflictError{
			Message: "Container " + ref + " is not running",
		})
		return
	}

	var req api.ExecCreateRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	// Validate that Cmd is not empty
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

// --- Exec start dispatch — hijack first, then route through Typed.Exec ---

func (s *BaseServer) handleExecStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req api.ExecStartRequest
	_ = ReadJSON(r, &req)

	// Resolve exec metadata up front. The handler must surface a 404
	// before hijacking, because once the conn is hijacked the client
	// would parse error-message bytes as a multiplexed stdcopy stream
	// and emit `unrecognized stream: <byte>`.
	exec, ok := s.Store.Execs.Get(id)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "exec instance", ID: id})
		return
	}
	tty := exec.ProcessConfig.Tty || req.Tty
	c, _ := s.ResolveContainerAuto(r.Context(), exec.ContainerID)
	if c.ID == "" {
		c.ID = exec.ContainerID
	}

	// Hijack the connection.
	hj, ok := w.(http.Hijacker)
	if !ok {
		WriteError(w, &api.ServerError{Message: "hijacking not supported"})
		return
	}
	conn, buf, hjErr := hj.Hijack()
	if hjErr != nil {
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

	dctx := DriverContext{
		Ctx:       r.Context(),
		Container: c,
		Backend:   s.Desc.Driver,
		Logger:    s.Logger,
	}
	cmd := append([]string{exec.ProcessConfig.Entrypoint}, exec.ProcessConfig.Arguments...)
	env := mergeEnv(c.Config.Env, exec.ProcessConfig.Env)
	workDir := exec.ProcessConfig.WorkingDir
	if workDir == "" {
		workDir = c.Config.WorkingDir
	}
	opts := ExecOptions{
		ExecID:  id,
		Cmd:     cmd,
		Env:     env,
		WorkDir: workDir,
		TTY:     tty,
		User:    exec.ProcessConfig.User,
	}

	// Bookkeeping previously lived in BaseServer.ExecStart. Centralised
	// here so cloud-native typed drivers don't each have to track
	// Running/PID/ExitCode for the exec instance.
	execPid := s.Store.NextPID()
	s.Store.Execs.Update(id, func(e *api.ExecInstance) {
		e.Running = true
		e.Pid = execPid
	})
	exitCode, err := s.Typed.Exec.Exec(dctx, opts, conn)
	if err != nil {
		s.Logger.Debug().Err(err).Str("exec", id).Msg("typed exec dispatch error after hijack")
	}
	s.Store.Execs.Update(id, func(e *api.ExecInstance) {
		e.Running = false
		e.Pid = 0
		e.ExitCode = exitCode
		e.CanRemove = true
	})
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
