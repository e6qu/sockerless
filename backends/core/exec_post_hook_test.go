package core

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type closeCounter struct {
	io.ReadWriter
	closes int
	err    error
}

func (c *closeCounter) Close() error { c.closes++; return c.err }

func TestExecPostHookRunsOnceAfterClose(t *testing.T) {
	inner := &closeCounter{ReadWriter: &bytes.Buffer{}, err: errors.New("close failed")}
	runs := 0
	h := NewExecPostHook(inner, func() { runs++ })
	if err := h.Close(); err == nil || err.Error() != "close failed" {
		t.Fatalf("Close must return the stream's own error, got %v", err)
	}
	_ = h.Close()
	if inner.closes != 2 {
		t.Fatalf("underlying Close called %d times, want 2", inner.closes)
	}
	if runs != 1 {
		t.Fatalf("hook ran %d times, want exactly once", runs)
	}
}
