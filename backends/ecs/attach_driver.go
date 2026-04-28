package ecs

import (
	"errors"
	"io"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// ecsStdinAttachDriver is the typed AttachDriver for the ECS backend.
// Read side: streams the container's CloudWatch logs (mux-framed for
// non-TTY containers, raw for TTY) — same as the FaaS cloud-logs
// attach. Write side: routes stdin bytes into the per-container
// stdinPipe so the deferred-RunTask path in ContainerStart can bake
// them into the task definition's command at launch.
//
// The default `core.NewCloudLogsAttachDriver` discards stdin (cloud
// containers have no remote stdin channel for a running task). For
// containers created with `OpenStdin && AttachStdin` (the
// gitlab-runner / `docker run -i` pattern) we keep stdin instead so
// the script written by the caller across the hijacked connection
// becomes the container's actual command.
type ecsStdinAttachDriver struct {
	s *Server
}

func (d *ecsStdinAttachDriver) Describe() string {
	return "ecs (CloudWatch-logs read + stdin pipe to deferred-RunTask command override)"
}

func (d *ecsStdinAttachDriver) Attach(dctx core.DriverContext, tty bool, conn io.ReadWriter) error {
	id := dctx.Container.ID
	fetch := d.s.buildCloudWatchFetcher(id)

	rwc, err := core.AttachViaCloudLogs(d.s.BaseServer, id, api.ContainerAttachOptions{
		Stdout: true,
		Stderr: true,
		Stream: true,
		Logs:   true,
	}, fetch)
	if err != nil {
		return err
	}
	defer rwc.Close()

	// Wire stdin only when the container was created with the
	// `OpenStdin && AttachStdin` flags (gitlab-runner / `docker run -i`
	// pattern). The flag is persisted in ECSState (not in
	// Container.Config from CloudState, which doesn't synthesize stdin
	// flags from ECS task data). Get-or-create a pipe so per-cycle
	// restarts — gitlab-runner reuses the same container ID across
	// script steps; each cycle does attach → start → stream → close
	// stdin → wait → stop — each get a fresh buffer:
	// launchAfterStdin removes the pipe after consuming it, so the
	// subsequent attach lands on a freshly-created one.
	var pipe *stdinPipe
	ecsState, _ := d.s.ECS.Get(id)
	if ecsState.OpenStdin {
		p := newStdinPipe()
		actual, _ := d.s.stdinPipes.LoadOrStore(id, p)
		pipe = actual.(*stdinPipe)
		pipe.Open()
	}

	done := make(chan struct{})
	// Pump stdout/stderr (cloud-logs) → caller.
	go func() {
		_, _ = io.Copy(conn, rwc)
		close(done)
	}()
	// Pump caller → stdin pipe (or discard if no pipe).
	go func() {
		if pipe != nil {
			_, _ = io.Copy(stdinPipeWriter{p: pipe}, conn)
			_ = pipe.Close()
		} else {
			_, _ = io.Copy(io.Discard, conn)
		}
	}()
	<-done

	if err == nil {
		return nil
	}
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

type stdinPipeWriter struct{ p *stdinPipe }

func (w stdinPipeWriter) Write(b []byte) (int, error) { return w.p.Write(b) }
