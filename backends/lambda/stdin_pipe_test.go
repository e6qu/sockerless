package lambda

import (
	"testing"
	"time"
)

func TestStdinPipeWriteThenClose(t *testing.T) {
	p := newStdinPipe()
	if p.IsOpen() {
		t.Fatalf("IsOpen should be false before Open()")
	}
	p.Open()
	if !p.IsOpen() {
		t.Fatalf("IsOpen should be true after Open()")
	}

	want := []byte(`{"script":"echo hello from sockerless lambda"}`)
	if n, err := p.Write(want); err != nil || n != len(want) {
		t.Fatalf("Write: n=%d err=%v", n, err)
	}

	select {
	case <-p.Done():
		t.Fatalf("Done should not fire before Close")
	default:
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case <-p.Done():
	case <-time.After(time.Second):
		t.Fatalf("Done should fire after Close")
	}

	if got := string(p.Bytes()); got != string(want) {
		t.Fatalf("Bytes mismatch: got %q want %q", got, want)
	}
}

func TestStdinPipeWriteAfterCloseFails(t *testing.T) {
	p := newStdinPipe()
	_ = p.Close()
	if _, err := p.Write([]byte("x")); err == nil {
		t.Fatalf("Write after Close must fail")
	}
}

func TestStdinPipeCloseIdempotent(t *testing.T) {
	p := newStdinPipe()
	if err := p.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
