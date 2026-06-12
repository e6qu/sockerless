package main

import (
	"testing"
	"time"

	"github.com/sockerless/github-runner-dispatcher-azure/internal/spawner"
)

// TestShouldReap pins the cleanup-eligibility contract: terminal
// executions reap, running executions NEVER reap (deleting the ACA Job
// kills the in-flight execution), execution-less Jobs reap only past
// the orphan grace, and partial information (UNKNOWN state, missing
// creation time) never reaps.
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
		{"running", spawner.Managed{State: spawner.StateExecutionRunning, CreatedAt: old}, false},
		{"no execution, fresh", spawner.Managed{State: spawner.StateNoExecution, CreatedAt: fresh}, false},
		{"no execution, old", spawner.Managed{State: spawner.StateNoExecution, CreatedAt: old}, true},
		{"no execution, unknown age", spawner.Managed{State: spawner.StateNoExecution}, false},
		{"unknown state", spawner.Managed{State: "UNKNOWN", CreatedAt: old}, false},
	}
	for _, tc := range cases {
		if got := shouldReap(tc.m, now); got != tc.want {
			t.Errorf("%s: shouldReap = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsTerminalState(t *testing.T) {
	for state, want := range map[string]bool{
		spawner.StateExecutionSucceeded: true,
		spawner.StateExecutionFailed:    true,
		spawner.StateExecutionRunning:   false,
		spawner.StateNoExecution:        false,
		"UNKNOWN":                       false,
		// The old (buggy) ProvisioningState vocabulary must never be
		// treated as terminal again.
		"Succeeded": false,
		"Failed":    false,
		"Canceled":  false,
	} {
		if got := isTerminalState(state); got != want {
			t.Errorf("isTerminalState(%q) = %v, want %v", state, got, want)
		}
	}
}
