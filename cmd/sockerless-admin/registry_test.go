package main

import (
	"testing"
	"time"
)

func TestPollLoopCancellation(t *testing.T) {
	reg := NewRegistry()
	done := make(chan struct{})

	exited := make(chan struct{})
	go func() {
		reg.PollLoop(50*time.Millisecond, done)
		close(exited)
	}()

	// Let it run a couple of ticks
	time.Sleep(120 * time.Millisecond)

	// Signal stop
	close(done)

	select {
	case <-exited:
		// OK — PollLoop exited
	case <-time.After(2 * time.Second):
		t.Fatal("PollLoop did not exit after done channel closed")
	}
}
