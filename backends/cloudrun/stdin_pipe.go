package cloudrun

import (
	"bytes"
	"errors"
	"sync"
)

// stdinPipe captures bytes written via docker attach stdin so they can
// be replayed as the bootstrap's `execEnvelope.Stdin` payload at
// Service-invoke time. Used for the docker-executor pattern (e.g.
// gitlab-runner): create the container with OpenStdin=true Cmd=[sh],
// hijack-attach, start, pipe the script through stdin, half-close —
// the bootstrap runs `sh` with the buffered stdin piped in, returns
// stdout/stderr in the response.
//
// Cloud Run Service has no remote stdin channel for a running container.
// The fix (mirror of backends/ecs/stdin_pipe.go + backends/lambda/
// stdin_pipe.go): buffer stdin during the attach window and bake it
// into the bootstrap POST envelope at deferred-invoke time.
type stdinPipe struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	done   chan struct{}
	closed bool
	opened bool
}

func newStdinPipe() *stdinPipe {
	return &stdinPipe{done: make(chan struct{})}
}

// Open marks the pipe as having an active attach reader. The deferred
// invoke path uses this to distinguish "attach is wired, wait for stdin
// EOF" from "no attach happened, run with original env-baked cmd".
// Idempotent.
func (p *stdinPipe) Open() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.opened = true
}

// IsOpen reports whether an attach has wired up the pipe.
func (p *stdinPipe) IsOpen() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.opened
}

// Write appends bytes to the buffered script.
func (p *stdinPipe) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, errors.New("stdin pipe closed")
	}
	return p.buf.Write(b)
}

// Close signals stdin EOF. Idempotent.
func (p *stdinPipe) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	close(p.done)
	return nil
}

// Done returns a channel closed when stdin reaches EOF.
func (p *stdinPipe) Done() <-chan struct{} { return p.done }

// Bytes returns a snapshot of the buffered stdin bytes. Safe to call
// after Done() fires.
func (p *stdinPipe) Bytes() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]byte, p.buf.Len())
	copy(out, p.buf.Bytes())
	return out
}
