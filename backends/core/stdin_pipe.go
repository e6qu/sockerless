package core

import (
	"bytes"
	"errors"
	"sync"
)

// StdinPipe captures the bytes a docker client writes over a hijacked
// attach so a backend can replay them as the container's command when it
// launches the workload. This is how the docker-executor pattern
// (gitlab-runner, for one) works on a cloud that has no remote stdin
// channel into a running task: the client creates the container with
// OpenStdin=true and Cmd=[sh], attaches, starts, pipes the script through
// stdin and half-closes; the backend buffers the script during the attach
// window and hands it to the workload as `sh -c <script>`.
type StdinPipe struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	done   chan struct{}
	closed bool
	opened bool
}

// NewStdinPipe returns an empty, open-able pipe.
func NewStdinPipe() *StdinPipe {
	return &StdinPipe{done: make(chan struct{})}
}

// Open marks the pipe as having an active attach reader, so a deferred
// start can tell "attach is wired, wait for stdin EOF" from "no attach
// happened, run the original command". Idempotent.
func (p *StdinPipe) Open() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.opened = true
}

// IsOpen reports whether an attach has wired up the pipe.
func (p *StdinPipe) IsOpen() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.opened
}

// Write appends bytes to the buffered script. It fails once the pipe is
// closed.
func (p *StdinPipe) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return 0, errors.New("stdin pipe closed")
	}
	return p.buf.Write(b)
}

// Close signals stdin EOF. Idempotent.
func (p *StdinPipe) Close() error {
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
func (p *StdinPipe) Done() <-chan struct{} { return p.done }

// Bytes returns a copy of the buffered stdin bytes. Safe to call after
// Done fires.
func (p *StdinPipe) Bytes() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]byte, p.buf.Len())
	copy(out, p.buf.Bytes())
	return out
}
