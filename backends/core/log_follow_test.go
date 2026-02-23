package core

import (
	"testing"
	"time"
)

func TestSyntheticLogSubscribe_BufferedThenClose(t *testing.T) {
	store := NewStore()
	driver := &SyntheticStreamDriver{Store: store}

	// Create a container with a wait channel
	exitCh := make(chan struct{})
	store.WaitChs.Store("c1", exitCh)
	store.LogBuffers.Store("c1", []byte("hello\n"))

	ch := driver.LogSubscribe("c1", "sub1")
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}

	// Close the exit channel to simulate container exit
	close(exitCh)

	// Channel should close
	select {
	case _, ok := <-ch:
		if ok {
			// Might get a value; drain and wait for close
			select {
			case _, ok := <-ch:
				if ok {
					t.Fatal("expected channel to close")
				}
			case <-time.After(time.Second):
				t.Fatal("channel did not close in time")
			}
		}
	case <-time.After(time.Second):
		t.Fatal("channel did not close in time")
	}
}

func TestSyntheticLogSubscribe_EmptyLogs(t *testing.T) {
	store := NewStore()
	driver := &SyntheticStreamDriver{Store: store}

	exitCh := make(chan struct{})
	store.WaitChs.Store("c2", exitCh)

	ch := driver.LogSubscribe("c2", "sub1")
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}

	close(exitCh)

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to close immediately for empty logs")
		}
	case <-time.After(time.Second):
		t.Fatal("channel did not close in time")
	}
}

func TestLogFollow_TailAndFollow(t *testing.T) {
	// Test that FilterLogTail works before follow subscription
	lines := []string{
		"2024-01-01T12:00:00Z line1",
		"2024-01-01T12:01:00Z line2",
		"2024-01-01T12:02:00Z line3",
		"2024-01-01T12:03:00Z line4",
	}
	tailed := FilterLogTail(lines, 2)
	if len(tailed) != 2 {
		t.Fatalf("expected 2 tailed lines, got %d", len(tailed))
	}
	if tailed[0] != "2024-01-01T12:02:00Z line3" {
		t.Fatalf("expected line3, got %s", tailed[0])
	}
	if tailed[1] != "2024-01-01T12:03:00Z line4" {
		t.Fatalf("expected line4, got %s", tailed[1])
	}
}
