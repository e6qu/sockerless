package ecs

import (
	"io"
	"strings"
	"testing"
)

// TestAttachStream_DiscardsWrites ensures Write returns len(p), nil so
// callers that pipe stdin into the attach conn don't error out.
func TestAttachStream_DiscardsWrites(t *testing.T) {
	inner := io.NopCloser(strings.NewReader("hello"))
	rwc := &attachStream{reader: inner}

	n, err := rwc.Write([]byte("user-stdin"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != len("user-stdin") {
		t.Fatalf("Write returned %d, want %d", n, len("user-stdin"))
	}
}

// TestAttachStream_PassThroughReads confirms the reader's contents are
// returned verbatim — the discard-only wrapper must not interfere with
// stdout bytes flowing out of CloudWatch.
func TestAttachStream_PassThroughReads(t *testing.T) {
	inner := io.NopCloser(strings.NewReader("log-line-1\nlog-line-2\n"))
	rwc := &attachStream{reader: inner}

	buf, err := io.ReadAll(rwc)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if string(buf) != "log-line-1\nlog-line-2\n" {
		t.Fatalf("reader content mismatch: got %q", buf)
	}
}

// TestAttachStream_CloseClosesInner ensures Close propagates to the
// underlying CloudWatch log reader so the follow-mode goroutine exits.
func TestAttachStream_CloseClosesInner(t *testing.T) {
	closed := false
	inner := &trackingCloser{
		Reader:    strings.NewReader(""),
		closeFunc: func() error { closed = true; return nil },
	}
	rwc := &attachStream{reader: inner}

	if err := rwc.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if !closed {
		t.Fatal("expected inner reader to be closed")
	}
}

type trackingCloser struct {
	io.Reader
	closeFunc func() error
}

func (t *trackingCloser) Close() error { return t.closeFunc() }
