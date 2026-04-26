package core

import (
	"errors"
	"io"
	"net"
)

// legacyExecAdapter wraps the narrow LegacyExecDriver
// (`Exec(ctx, containerID, execID, cmd, env, workDir, tty, conn) int`)
// into the typed ExecDriver shape
// (`Exec(dctx DriverContext, opts ExecOptions, conn io.ReadWriter) (int, error)`).
// Backends with the legacy implementation plug it into TypedDriverSet.Exec
// via WrapLegacyExec.

// WrapLegacyExec returns a typed ExecDriver that delegates to the
// supplied LegacyExecDriver. `backend` and `impl` populate the
// `Describe()` string so NotImpl messages name the backend +
// implementation when the wrapped driver is missing or returns -1.
func WrapLegacyExec(narrow LegacyExecDriver, backend, impl string) ExecDriver {
	return &legacyExecAdapter{narrow: narrow, backend: backend, impl: impl}
}

type legacyExecAdapter struct {
	narrow  LegacyExecDriver
	backend string
	impl    string
}

func (a *legacyExecAdapter) Describe() string {
	if a.backend == "" && a.impl == "" {
		return "exec via legacy narrow driver"
	}
	return a.backend + " " + a.impl + " (legacy-narrow exec adapter)"
}

func (a *legacyExecAdapter) Exec(dctx DriverContext, opts ExecOptions, conn io.ReadWriter) (int, error) {
	if a.narrow == nil {
		return -1, errors.New("legacy exec adapter: narrow driver is nil — register one via DriverSet.Exec or replace this adapter with a typed ExecDriver")
	}
	netConn, ok := conn.(net.Conn)
	if !ok {
		// The narrow LegacyExecDriver requires a net.Conn (it owns the
		// hijacked TCP connection so it can write Docker mux headers
		// directly). Refuse to proceed if the caller passes a plain
		// io.ReadWriter — silently shimming would mask exec errors at a
		// layer the operator can't see, which violates the no-fallbacks
		// rule.
		return -1, errors.New("legacy exec adapter: requires net.Conn for hijacked-stream exec; caller passed a non-Conn ReadWriter")
	}
	exit := a.narrow.Exec(dctx.Ctx, dctx.Container.ID, opts.ExecID, opts.Cmd, opts.Env, opts.WorkDir, opts.TTY, netConn)
	return exit, nil
}
