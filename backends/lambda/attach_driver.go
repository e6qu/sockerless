package lambda

import (
	"errors"
	"io"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// lambdaStdinAttachDriver is the typed AttachDriver for the Lambda
// backend. Read side: streams the function's CloudWatch logs (mux-
// framed for non-TTY containers, raw for TTY). Write side: routes
// stdin bytes into the per-container stdinPipe so the deferred-Invoke
// path in ContainerStart can bake them into the Lambda Invoke
// Payload.
//
// The default `core.NewCloudLogsAttachDriver` discards stdin (Lambda
// has no remote stdin channel for a running invocation). For
// containers created with `OpenStdin && AttachStdin` (the
// gitlab-runner / `docker run -i` pattern) we keep stdin instead so
// the script written by the caller across the hijacked connection
// becomes the Invoke Payload — the bootstrap's runUserInvocation
// pipes Payload to the user entrypoint as stdin, so `Cmd=[sh]` +
// Payload=script naturally runs the buffered script.
//
// Mirrors `backends/ecs/attach_driver.go::ecsStdinAttachDriver`.
type lambdaStdinAttachDriver struct {
	s *Server
}

func (d *lambdaStdinAttachDriver) Describe() string {
	return "lambda (CloudWatch-logs read + stdin pipe to deferred-Invoke Payload)"
}

func (d *lambdaStdinAttachDriver) Attach(dctx core.DriverContext, tty bool, conn io.ReadWriter) error {
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
	// `OpenStdin && AttachStdin` flags. The flag is persisted in
	// LambdaState (CloudState's synthesised Container.Config doesn't
	// carry stdin flags). Get-or-create a pipe so per-cycle restarts
	// each get a fresh buffer.
	var pipe *stdinPipe
	lambdaState, _ := d.s.Lambda.Get(id)
	if lambdaState.OpenStdin {
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
