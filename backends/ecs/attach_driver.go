package ecs

import (
	"errors"
	"fmt"
	"io"
	"time"

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

	// Create + open the stdin pipe FIRST, before the stage barrier below.
	// ContainerStart's deferred-stdin path checks for an open pipe to decide
	// whether to bake the streamed script into the task command, and
	// gitlab-runner's docker executor does create → /attach → /start (the
	// /start races right behind /attach). If the pipe were created only
	// after the barrier — which itself waits for /start to register a
	// WaitCh — /start would arrive first, find no pipe, fall through, and
	// launch the image-default command (the helper's `gitlab-runner-build`
	// then waits forever for stdin). Pipe-before-barrier breaks that
	// dependency inversion.
	//
	// Wire stdin only when the container was created with `OpenStdin &&
	// AttachStdin` (the flag is persisted in ECSState — Container.Config
	// from CloudState doesn't synthesize stdin flags from ECS task data).
	// Get-or-create so per-cycle restarts (gitlab-runner reuses the same
	// container ID across script steps; each cycle does attach → start →
	// stream → close stdin → wait → stop) each get a fresh buffer:
	// launchAfterStdin removes the pipe after consuming it, so the
	// subsequent attach lands on a freshly-created one.
	var pipe *stdinPipe
	ecsState, _ := d.s.ECS.Get(id)
	if ecsState.OpenStdin {
		p := newStdinPipe()
		actual, _ := d.s.stdinPipes.LoadOrStore(id, p)
		var ok bool
		pipe, ok = actual.(*stdinPipe)
		if !ok {
			return fmt.Errorf("ecs attach: stdin pipe for %s has unexpected type %T", id, actual)
		}
		pipe.Open()
	}

	// Stage-boundary barrier for the gitlab-runner predefined-helper
	// flow: gitlab-runner does /attach then /start per stage on the
	// same container ID, but the previous stage's Fargate task is
	// already STOPPED in CloudState. If the cloud-logs follower below
	// starts before /start fires `markRunning`, its first poll sees
	// Status="exited" from CloudState and EOFs immediately — the new
	// stage's task hasn't even been registered yet, so nothing
	// streams to the caller and gitlab-runner reports an empty
	// (failed) stage.
	//
	// Wait briefly (up to 5 s) for /start to register a fresh WaitCh
	// in the Store. Once it's there, the ContainerInspect override
	// returns Status="running" while the WaitCh is open, keeping
	// the cloud-logs follower alive across the new task's startup.
	// Containers that never see a /start (e.g. log-streaming attaches
	// the caller will close on its own) just hit the timeout and
	// fall through to the existing flow.
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(50 * time.Millisecond)
	for {
		if _, ok := d.s.Store.WaitChs.Load(id); ok {
			break
		}
		select {
		case <-deadline:
			tick.Stop()
			goto barrierDone
		case <-tick.C:
		}
	}
barrierDone:
	tick.Stop()

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
	// The stdout pump finished (attach over) — unblock the stdin pump parked on
	// conn.Read so it doesn't leak until the caller closes conn.
	switch cc := conn.(type) {
	case interface{ SetReadDeadline(time.Time) error }:
		_ = cc.SetReadDeadline(time.Now())
	case interface{ CloseRead() error }:
		_ = cc.CloseRead()
	}

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
