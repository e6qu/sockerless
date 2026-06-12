package main

import (
	"testing"
	"time"

	"github.com/sockerless/github-runner-dispatcher-gcp/internal/spawner"
)

// TestShouldReap pins the cleanup-eligibility contract: terminal
// executions reap, running executions never reap, execution-less Jobs
// (RunJob failed) reap only past the orphan grace, and partial
// information (UNKNOWN state, missing creation time) never reaps.
func TestShouldReap(t *testing.T) {
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	old := now.Add(-time.Hour)
	fresh := now.Add(-time.Minute)

	cases := []struct {
		name string
		m    spawner.Managed
		want bool
	}{
		{"succeeded", spawner.Managed{State: spawner.StateExecutionSucceeded}, true},
		{"failed", spawner.Managed{State: spawner.StateExecutionFailed}, true},
		{"running", spawner.Managed{State: spawner.StateExecutionRunning, CreateTime: old}, false},
		{"no execution, fresh", spawner.Managed{State: spawner.StateNoExecution, CreateTime: fresh}, false},
		{"no execution, old", spawner.Managed{State: spawner.StateNoExecution, CreateTime: old}, true},
		{"no execution, unknown age", spawner.Managed{State: spawner.StateNoExecution}, false},
		{"unknown state", spawner.Managed{State: "UNKNOWN", CreateTime: old}, false},
	}
	for _, tc := range cases {
		if got := shouldReap(tc.m, now); got != tc.want {
			t.Errorf("%s: shouldReap = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsTerminalJobState(t *testing.T) {
	for state, want := range map[string]bool{
		spawner.StateExecutionSucceeded: true,
		spawner.StateExecutionFailed:    true,
		spawner.StateExecutionRunning:   false,
		spawner.StateNoExecution:        false,
		"UNKNOWN":                       false,
		// The legacy TerminalCondition vocabulary must never be
		// treated as terminal again (it deleted bootstrapping
		// runner-tasks).
		"CONDITION_SUCCEEDED": false,
		"CONDITION_FAILED":    false,
	} {
		if got := isTerminalJobState(state); got != want {
			t.Errorf("isTerminalJobState(%q) = %v, want %v", state, got, want)
		}
	}
}
