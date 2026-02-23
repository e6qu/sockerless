package core

import (
	"testing"

	"github.com/sockerless/api"
)

func TestShouldRestart_OnFailure(t *testing.T) {
	policy := api.RestartPolicy{Name: "on-failure"}
	if !ShouldRestart(policy, 1, 0) {
		t.Fatal("expected restart on exit 1")
	}
	if ShouldRestart(policy, 0, 0) {
		t.Fatal("should not restart on exit 0")
	}
}

func TestShouldRestart_MaxRetry(t *testing.T) {
	policy := api.RestartPolicy{Name: "on-failure", MaximumRetryCount: 3}
	if !ShouldRestart(policy, 1, 2) {
		t.Fatal("expected restart when count < max")
	}
	if ShouldRestart(policy, 1, 3) {
		t.Fatal("should not restart when count >= max")
	}
}

func TestShouldRestart_NoPolicy(t *testing.T) {
	if ShouldRestart(api.RestartPolicy{}, 1, 0) {
		t.Fatal("empty policy should not restart")
	}
	if ShouldRestart(api.RestartPolicy{Name: "no"}, 1, 0) {
		t.Fatal("'no' policy should not restart")
	}
}
