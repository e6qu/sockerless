package gcf

import (
	"bytes"
	"errors"
	"sync"
)

// stdinPipe captures bytes written via docker attach stdin so they can
// be replayed as the bootstrap's `execEnvelope.Stdin` payload at
// Service-invoke time. Mirrors backends/cloudrun/stdin_pipe.go: same
// shape, same semantics. The gitlab-runner attach pattern (create
// container with OpenStdin=true Cmd=[sh] + hijack-attach + start +
// pipe-script-and-half-close) needs this because Cloud Run Service
// has no remote stdin channel for a running container.
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

// Open marks the pipe as having an active attach reader.
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

// Bytes returns a snapshot of the buffered stdin bytes.
func (p *stdinPipe) Bytes() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]byte, p.buf.Len())
	copy(out, p.buf.Bytes())
	return out
}
