package aca

import (
	"bytes"
	"errors"
	"sync"
)

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
