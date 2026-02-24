package core

import (
	"testing"
	"time"

	"github.com/sockerless/api"
)

func TestRestartDelayFirstRestart(t *testing.T) {
	d := RestartDelay(0)
	if d != 100*time.Millisecond {
		t.Errorf("expected 100ms, got %v", d)
	}
}

func TestRestartDelayExponentialBackoff(t *testing.T) {
	expected := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1600 * time.Millisecond,
	}
	for i, want := range expected {
		got := RestartDelay(i)
		if got != want {
			t.Errorf("RestartDelay(%d) = %v, want %v", i, got, want)
		}
	}
}

func TestRestartDelayCappedAt60s(t *testing.T) {
	// At count 10, 100ms * 2^10 = 102.4s, should be capped at 60s
	d := RestartDelay(10)
	if d != 60*time.Second {
		t.Errorf("expected 60s cap, got %v", d)
	}
	// Very large count should still be 60s
	d = RestartDelay(100)
	if d != 60*time.Second {
		t.Errorf("expected 60s cap for count=100, got %v", d)
	}
}

func TestShouldRestartAlways(t *testing.T) {
	if !ShouldRestart(api.RestartPolicy{Name: "always"}, 1, 0) {
		t.Error("expected 'always' to restart on exit code 1")
	}
	if !ShouldRestart(api.RestartPolicy{Name: "always"}, 0, 0) {
		t.Error("expected 'always' to restart on exit code 0")
	}
}

func TestShouldRestartUnlessStopped(t *testing.T) {
	if !ShouldRestart(api.RestartPolicy{Name: "unless-stopped"}, 1, 0) {
		t.Error("expected 'unless-stopped' to restart")
	}
}

func TestShouldRestartOnFailureExitZero(t *testing.T) {
	if ShouldRestart(api.RestartPolicy{Name: "on-failure"}, 0, 0) {
		t.Error("expected 'on-failure' not to restart on exit code 0")
	}
}

func TestShouldRestartOnFailureMaxCount(t *testing.T) {
	policy := api.RestartPolicy{Name: "on-failure", MaximumRetryCount: 3}
	// Under max
	if !ShouldRestart(policy, 1, 2) {
		t.Error("expected restart at count 2 (max 3)")
	}
	// At max
	if ShouldRestart(policy, 1, 3) {
		t.Error("expected no restart at count 3 (max 3)")
	}
	// Over max
	if ShouldRestart(policy, 1, 5) {
		t.Error("expected no restart at count 5 (max 3)")
	}
}
