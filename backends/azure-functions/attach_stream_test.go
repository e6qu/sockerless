package azf

import (
	"io"
	"testing"
	"time"
)

// TestAttachStreamReadDeadline verifies the buffered-attach reader returns EOF
// when the invoke never publishes within the deadline — bounding the strand
// that otherwise lasts up to the 600s invoke cap when a FaaS pod stalls or
// expires before an instant workload runs.
func TestAttachStreamReadDeadline(t *testing.T) {
	a := &attachStream{
		respReady: make(chan struct{}),
		deadline:  80 * time.Millisecond,
	}

	start := time.Now()
	n, err := a.Read(make([]byte, 64))
	elapsed := time.Since(start)

	if err != io.EOF {
		t.Fatalf("Read err = %v, want io.EOF on deadline", err)
	}
	if n != 0 {
		t.Fatalf("Read n = %d, want 0", n)
	}
	if elapsed < a.deadline {
		t.Fatalf("Read returned after %s, want >= deadline %s (returned too early)", elapsed, a.deadline)
	}
	if elapsed > a.deadline+2*time.Second {
		t.Fatalf("Read returned after %s, want ~deadline %s (did not bound)", elapsed, a.deadline)
	}
}

// TestAttachStreamReadPublishedBeforeDeadline verifies a normal publish before
// the deadline delivers the captured output (the deadline is a pure safety net
// that must not truncate a healthy invoke).
func TestAttachStreamReadPublishedBeforeDeadline(t *testing.T) {
	a := &attachStream{
		respReady: make(chan struct{}),
		deadline:  5 * time.Second,
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		a.publishAttachResponse([]byte("hello-attach"), nil)
	}()

	// First Read returns the muxed stdout frame; drain until EOF and confirm
	// the payload is present.
	var got []byte
	buf := make([]byte, 256)
	for {
		n, err := a.Read(buf)
		got = append(got, buf[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read err = %v", err)
		}
	}
	if len(got) == 0 {
		t.Fatal("published output was not delivered before the deadline")
	}
	// The stdout payload is muxed after an 8-byte frame header.
	if !containsBytes(got, []byte("hello-attach")) {
		t.Fatalf("delivered frame %q missing payload", got)
	}
}

func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if string(haystack[i:i+len(needle)]) == string(needle) {
			return true
		}
	}
	return false
}
