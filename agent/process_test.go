package agent

import (
	"bytes"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestRingBufferBasic(t *testing.T) {
	rb := NewRingBuffer(64)
	data := []byte("hello world")
	rb.Write(data)
	got := rb.Bytes()
	if !bytes.Equal(got, data) {
		t.Fatalf("expected %q, got %q", data, got)
	}
}

func TestRingBufferWrap(t *testing.T) {
	rb := NewRingBuffer(8)
	// Write 12 bytes into an 8-byte buffer
	rb.Write([]byte("abcdefghijkl"))
	got := rb.Bytes()
	// Should contain the last 8 bytes in order
	if !bytes.Equal(got, []byte("efghijkl")) {
		t.Fatalf("expected %q, got %q", "efghijkl", got)
	}
}

func TestRingBufferExact(t *testing.T) {
	rb := NewRingBuffer(8)
	rb.Write([]byte("abcdefgh"))
	if !rb.full {
		t.Fatal("expected full=true after writing exactly capacity")
	}
	got := rb.Bytes()
	if !bytes.Equal(got, []byte("abcdefgh")) {
		t.Fatalf("expected %q, got %q", "abcdefgh", got)
	}
}

func TestMainProcessExitCode(t *testing.T) {
	mp, err := NewMainProcess(testLogger(), []string{"true"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-mp.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit")
	}
	code := mp.ExitCode()
	if code == nil || *code != 0 {
		t.Fatalf("expected exit code 0, got %v", code)
	}
}

func TestMainProcessNonZeroExit(t *testing.T) {
	mp, err := NewMainProcess(testLogger(), []string{"false"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-mp.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit")
	}
	code := mp.ExitCode()
	if code == nil || *code == 0 {
		t.Fatalf("expected non-zero exit code, got %v", code)
	}
}

func TestMainProcessSubscribeAfterExit(t *testing.T) {
	mp, err := NewMainProcess(testLogger(), []string{"/bin/sh", "-c", "echo hello"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-mp.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit")
	}

	// Subscribe after exit — channel should be closed immediately
	_, _, ch := mp.Subscribe("late-sub")
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel was not closed")
	}

	code := mp.ExitCode()
	if code == nil || *code != 0 {
		t.Fatalf("expected exit code 0, got %v", code)
	}
}

// TestMainProcessUnsubscribeClosesChannel verifies Unsubscribe closes the
// listener channel so a consumer ranging over it (AttachSession.stream) exits,
// even while the main process is still running. Without the close the channel
// and its draining goroutine leak for the rest of a keep-alive container's life
// — one leak per attach/detach cycle.
func TestMainProcessUnsubscribeClosesChannel(t *testing.T) {
	// Long-running process so the main process stays alive across the detach.
	mp, err := NewMainProcess(testLogger(), []string{"/bin/sh", "-c", "sleep 5"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mp.Signal(syscall.SIGKILL) }()

	_, _, ch := mp.Subscribe("attach-1")

	closed := make(chan struct{})
	go func() {
		for range ch { // exits only when ch is closed
		}
		close(closed)
	}()

	// Detach while the process is still running.
	mp.Unsubscribe("attach-1")

	select {
	case <-closed:
		// good — the range loop saw the channel close and returned.
	case <-time.After(2 * time.Second):
		t.Fatal("Unsubscribe did not close the listener channel; stream goroutine leaked")
	}
}

func TestMainProcessConcurrentSubscribers(t *testing.T) {
	mp, err := NewMainProcess(testLogger(), []string{"/bin/sh", "-c", "sleep 0.1 && echo done"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "sub-" + string(rune('A'+n%26)) + string(rune('0'+n/26))
			_, _, ch := mp.Subscribe(id)

			// Drain channel
			for range ch {
			}

			mp.Unsubscribe(id)
		}(i)
	}

	select {
	case <-mp.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit")
	}

	wg.Wait()

	code := mp.ExitCode()
	if code == nil || *code != 0 {
		t.Fatalf("expected exit code 0, got %v", code)
	}
}
