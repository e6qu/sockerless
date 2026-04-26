package core

import (
	"errors"
	"io"

	"github.com/sockerless/api"
)

// WrapLegacyExecStart adapts the legacy `BaseServer.ExecStart`
// (`func(id, opts) (io.ReadWriteCloser, error)`) into the typed
// ExecDriver shape. The adapter:
//
//  1. Calls the legacy ExecStart to materialise the per-exec virtual
//     ReadWriteCloser the legacy backends produce (docker SDK
//     hijackedRWC, or the in-memory pipeConn the BaseServer default
//     spins up around `Drivers.Exec`).
//  2. Bridges the rwc to the typed driver's caller-supplied conn via
//     two io.Copy goroutines.
//  3. Reads the exit code from `Store.Execs` after the streams close
//     — that's where every legacy ExecStart impl records it.
//
// Once cloud backends migrate to a typed `ExecDriver` impl that owns
// the conn directly, this adapter goes away.

// LegacyExecStartFn matches BaseServer.ExecStart.
type LegacyExecStartFn func(id string, opts api.ExecStartRequest) (io.ReadWriteCloser, error)

// WrapLegacyExecStart returns an ExecDriver that delegates to a legacy
// ExecStart-shaped function and reads the exit code back from Store
// after the bridge completes.
func WrapLegacyExecStart(fn LegacyExecStartFn, store *Store, backend, impl string) ExecDriver {
	return &legacyExecStartAdapter{fn: fn, store: store, backend: backend, impl: impl}
}

type legacyExecStartAdapter struct {
	fn      LegacyExecStartFn
	store   *Store
	backend string
	impl    string
}

func (a *legacyExecStartAdapter) Describe() string {
	if a.backend == "" && a.impl == "" {
		return "exec via legacy ExecStart function"
	}
	return a.backend + " " + a.impl + " (legacy ExecStart adapter)"
}

func (a *legacyExecStartAdapter) Exec(dctx DriverContext, opts ExecOptions, conn io.ReadWriter) (int, error) {
	if a.fn == nil {
		return -1, errors.New("legacy exec-start adapter: function is nil")
	}
	rwc, err := a.fn(opts.ExecID, api.ExecStartRequest{
		Detach: false,
		Tty:    opts.TTY,
	})
	if err != nil {
		return -1, err
	}
	defer rwc.Close()

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(conn, rwc)
		close(done)
	}()
	go func() {
		_, _ = io.Copy(rwc, conn)
	}()
	<-done

	exit := 0
	if a.store != nil {
		if exec, ok := a.store.Execs.Get(opts.ExecID); ok {
			exit = exec.ExitCode
		}
	}
	return exit, nil
}
