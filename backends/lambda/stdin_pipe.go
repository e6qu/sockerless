package lambda

import (
	"bytes"
	"errors"
	"sync"
)

// stdinPipe captures bytes written via docker attach stdin so they can
// be replayed as the Lambda Invoke Payload at deferred-Invoke time.
// Used for the docker-executor pattern (e.g. gitlab-runner): create
// the container with OpenStdin=true Cmd=[sh], hijack-attach, start,
// pipe the script through stdin, half-close — the bootstrap's
// runUserInvocation pipes Payload to sh's stdin, so sh runs the
// buffered script.
//
// Lambda has no remote stdin channel for a running invocation. The
// fix buffers stdin during the attach window and bakes it into
// InvokeInput.Payload at deferred Invoke time. Mirrors
// `backends/ecs/stdin_pipe.go`.
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

// Open marks the pipe as having an active attach reader. ContainerStart
// uses this to distinguish "attach is wired, wait for stdin EOF" from
// "no attach happened, run with original entrypoint". Idempotent.
func (p *stdinPipe) Open() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.opened = true
}

func (p *stdinPipe) IsOpen() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.opened
}

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

func (p *stdinPipe) Done() <-chan struct{} { return p.done }

func (p *stdinPipe) Bytes() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]byte, p.buf.Len())
	copy(out, p.buf.Bytes())
	return out
}
