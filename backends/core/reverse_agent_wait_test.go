package core

import (
	"context"
	"errors"
	"testing"
	"time"
)

// nilConn is sufficient as a stand-in for *agent.ReverseAgentConn for
// registry book-keeping tests — the registry stores the pointer
// without dereferencing it.
//
// Register/Resolve/Drop/WaitForAgent never inspect conn contents.
func registerNil(t *testing.T, r *ReverseAgentRegistry, id string) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[id] = nil
	if ch, ok := r.waiters[id]; ok {
		close(ch)
		delete(r.waiters, id)
	}
}

func TestWaitForAgent_FastPathAlreadyRegistered(t *testing.T) {
	r := NewReverseAgentRegistry()
	registerNil(t, r, "c1")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := r.WaitForAgent(ctx, "c1"); err != nil {
		t.Fatalf("fast-path WaitForAgent: %v", err)
	}
}

func TestWaitForAgent_WakeOnLateRegister(t *testing.T) {
	r := NewReverseAgentRegistry()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- r.WaitForAgent(ctx, "c2") }()

	// Give the waiter time to subscribe before we register.
	time.Sleep(20 * time.Millisecond)
	registerNil(t, r, "c2")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("late-register WaitForAgent: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitForAgent did not return after Register")
	}
}

func TestWaitForAgent_TimeoutReturnsContextError(t *testing.T) {
	r := NewReverseAgentRegistry()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := r.WaitForAgent(ctx, "never")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}

	// Waiter map must not leak the timed-out subscriber.
	r.mu.Lock()
	_, leaked := r.waiters["never"]
	r.mu.Unlock()
	if leaked {
		t.Fatalf("waiters map leaked entry after timeout")
	}
}

func TestWaitForAgent_MultipleConcurrentWaiters(t *testing.T) {
	r := NewReverseAgentRegistry()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	const n = 5
	done := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() { done <- r.WaitForAgent(ctx, "shared") }()
	}
	time.Sleep(20 * time.Millisecond)
	registerNil(t, r, "shared")

	for i := 0; i < n; i++ {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("waiter %d: %v", i, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("waiter %d never woke", i)
		}
	}
}
