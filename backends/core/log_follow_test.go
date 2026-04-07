package core

import (
	"testing"
	"time"
)

func TestLogSubscribe_ClosesImmediately(t *testing.T) {
	store := NewStore()
	driver := &LocalStreamDriver{Store: store}

	ch := driver.LogSubscribe("c1", "sub1")
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}

	// Channel should close immediately (no agent, no log subscription support)
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to close immediately")
		}
	case <-time.After(time.Second):
		t.Fatal("channel did not close in time")
	}
}

func TestLogBytes_ReturnsNilWhenEmpty(t *testing.T) {
	store := NewStore()
	driver := &LocalStreamDriver{Store: store}

	// No data in LogBuffers — returns nil
	data := driver.LogBytes("c1")
	if data != nil {
		t.Fatalf("expected nil, got %v", data)
	}
}

func TestLogBytes_ReturnsStoredData(t *testing.T) {
	store := NewStore()
	driver := &LocalStreamDriver{Store: store}

	// Data in LogBuffers — returns it
	store.LogBuffers.Store("c1", []byte("hello\n"))
	data := driver.LogBytes("c1")
	if string(data) != "hello\n" {
		t.Fatalf("expected %q, got %q", "hello\n", string(data))
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
