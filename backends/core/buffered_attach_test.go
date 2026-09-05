package core

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// TestBufferedAttachReadDeadline: a reader whose invocation never
// publishes gets EOF at the deadline instead of hanging for the whole
// invoke budget.
func TestBufferedAttachReadDeadline(t *testing.T) {
	a := NewBufferedAttachStream(NewStdinPipe(), 80*time.Millisecond, nil)

	start := time.Now()
	n, err := a.Read(make([]byte, 64))
	elapsed := time.Since(start)

	if err != io.EOF {
		t.Fatalf("Read err = %v, want io.EOF on deadline", err)
	}
	if n != 0 {
		t.Fatalf("Read n = %d, want 0", n)
	}
	if elapsed < 80*time.Millisecond {
		t.Fatalf("Read returned after %s, before the deadline", elapsed)
	}
	if elapsed > 80*time.Millisecond+2*time.Second {
		t.Fatalf("Read returned after %s, the deadline did not bound it", elapsed)
	}
}

// TestBufferedAttachReadPublishedBeforeDeadline: a publish before the
// deadline delivers the output framed as stdcopy; the deadline is a
// safety net and must not truncate a healthy invoke.
func TestBufferedAttachReadPublishedBeforeDeadline(t *testing.T) {
	a := NewBufferedAttachStream(NewStdinPipe(), 5*time.Second, nil)
	go func() {
		time.Sleep(20 * time.Millisecond)
		a.PublishResponse([]byte("hello-attach"), []byte("warn"))
	}()

	got, err := io.ReadAll(a)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	var want bytes.Buffer
	WriteMuxFrame(&want, 0x01, []byte("hello-attach"))
	WriteMuxFrame(&want, 0x02, []byte("warn"))
	if !bytes.Equal(got, want.Bytes()) {
		t.Fatalf("delivered %q, want stdcopy frames %q", got, want.Bytes())
	}
}

func TestBufferedAttachPublishOnlyOnce(t *testing.T) {
	a := NewBufferedAttachStream(NewStdinPipe(), time.Second, nil)
	a.PublishResponse([]byte("first"), nil)
	a.PublishResponse([]byte("second"), nil)
	got, _ := io.ReadAll(a)
	if bytes.Contains(got, []byte("second")) {
		t.Fatalf("second publish must be ignored; got %q", got)
	}
}

func TestBufferedAttachCloseReleasesReaderAndStdin(t *testing.T) {
	pipe := NewStdinPipe()
	closed := false
	a := NewBufferedAttachStream(pipe, time.Minute, func() { closed = true })
	done := make(chan error, 1)
	go func() {
		_, err := a.Read(make([]byte, 8))
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != io.EOF {
			t.Fatalf("Read after Close = %v, want io.EOF", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not release the blocked reader")
	}
	select {
	case <-pipe.Done():
	default:
		t.Fatal("Close must close stdin so a deferred start is not held forever")
	}
	if !closed {
		t.Fatal("onClose hook did not run")
	}
}

func TestBufferedAttachWriteGoesToStdin(t *testing.T) {
	pipe := NewStdinPipe()
	a := NewBufferedAttachStream(pipe, time.Second, nil)
	if _, err := a.Write([]byte("script")); err != nil {
		t.Fatal(err)
	}
	if err := a.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if string(pipe.Bytes()) != "script" {
		t.Fatalf("stdin = %q", pipe.Bytes())
	}
}

func TestPublishBufferedAttachResponse(t *testing.T) {
	var streams sync.Map
	a := NewBufferedAttachStream(NewStdinPipe(), time.Second, nil)
	streams.Store("c1", a)
	PublishBufferedAttachResponse(&streams, "c1", []byte("out"), nil)
	if _, still := streams.Load("c1"); still {
		t.Fatal("publish must remove the stream from the registry")
	}
	got, _ := io.ReadAll(a)
	if !bytes.Contains(got, []byte("out")) {
		t.Fatalf("published output not delivered: %q", got)
	}
	// A container with no attach is a no-op.
	PublishBufferedAttachResponse(&streams, "absent", []byte("x"), nil)
}

func TestCaptureBufferedStdin(t *testing.T) {
	var pipes sync.Map
	logger := zerolog.Nop()

	if _, ok := CaptureBufferedStdin(context.Background(), &pipes, "none", time.Second, logger); ok {
		t.Fatal("no registered pipe must report false so the original command runs")
	}

	p := NewStdinPipe()
	_, _ = p.Write([]byte("echo hi"))
	_ = p.Close()
	pipes.Store("c1", p)
	got, ok := CaptureBufferedStdin(context.Background(), &pipes, "c1", time.Second, logger)
	if !ok || string(got) != "echo hi" {
		t.Fatalf("captured = %q ok=%v", got, ok)
	}
	if _, still := pipes.Load("c1"); still {
		t.Fatal("capture must remove the pipe from the registry")
	}

	// A caller that never half-closes is given the timeout, then the
	// bytes buffered so far are used.
	open := NewStdinPipe()
	_, _ = open.Write([]byte("partial"))
	pipes.Store("c2", open)
	got, ok = CaptureBufferedStdin(context.Background(), &pipes, "c2", 30*time.Millisecond, logger)
	if !ok || string(got) != "partial" {
		t.Fatalf("captured = %q ok=%v after timeout", got, ok)
	}

	// A cancelled context reports the shutdown rather than launching.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pipes.Store("c3", NewStdinPipe())
	got, ok = CaptureBufferedStdin(ctx, &pipes, "c3", time.Second, logger)
	if !ok || got != nil {
		t.Fatalf("cancelled capture = %q ok=%v, want nil,true", got, ok)
	}
}
