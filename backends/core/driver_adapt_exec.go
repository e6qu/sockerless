package core

import (
	"errors"
	"io"
	"net"
)

// Phase 104 — ExecDriver narrow→typed adapter.
//
// `legacyExecAdapter` wraps the existing narrow `ExecDriver` interface
// (`Exec(ctx, containerID, execID, cmd, env, workDir, tty, conn) int`)
// into the new typed `ExecDriver104` shape
// (`Exec(dctx DriverContext, opts ExecOptions, conn io.ReadWriter) (int, error)`).
//
// This is the first dimension-lift of Phase 104: it lets backends opt
// into the typed framework without rewriting their existing exec
// implementation. Backends keep their `core.ExecDriver` impl in the
// narrow `Drivers.Exec` slot during the transition; setting
// `DriverSet104.Exec = WrapLegacyExec(narrow, "<backend>", "<impl>")`
// surfaces it as a typed `ExecDriver104` for the new dispatch sites.
//
// Once every backend's exec call site is migrated to dispatch through
// `DriverSet104.Exec`, the narrow `core.ExecDriver` interface is
// removed and the adapter goes with it.

// WrapLegacyExec returns an ExecDriver104 that delegates to the
// supplied narrow ExecDriver. `backend` and `impl` populate the
// `Describe()` string so NotImpl messages name the backend +
// implementation when the wrapped driver is missing or returns -1.
func WrapLegacyExec(narrow ExecDriver, backend, impl string) ExecDriver104 {
	return &legacyExecAdapter{narrow: narrow, backend: backend, impl: impl}
}

type legacyExecAdapter struct {
	narrow  ExecDriver
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
		return -1, errors.New("legacy exec adapter: narrow driver is nil — register one via DriverSet.Exec or replace this adapter with a typed ExecDriver104")
	}
	netConn, ok := conn.(net.Conn)
	if !ok {
		// The narrow ExecDriver requires a net.Conn (it owns the
		// hijacked TCP connection so it can write Docker mux headers
		// directly). Refuse to proceed if the caller passes a plain
		// io.ReadWriter — silently shimming would mask exec errors
		// at a layer the operator can't see, which violates the no-
		// fallbacks rule.
		return -1, errors.New("legacy exec adapter: requires net.Conn for hijacked-stream exec; caller passed a non-Conn ReadWriter")
	}
	exit := a.narrow.Exec(dctx.Ctx, dctx.Container.ID, opts.ExecID, opts.Cmd, opts.Env, opts.WorkDir, opts.TTY, netConn)
	return exit, nil
}
