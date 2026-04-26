package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/sockerless/api"
)

type stubLegacyKill struct {
	gotRef    string
	gotSignal string
	returnErr error
}

func (s *stubLegacyKill) fn(ref, signal string) error {
	s.gotRef = ref
	s.gotSignal = signal
	return s.returnErr
}

func TestWrapLegacyKill_DelegatesAndPropagatesError(t *testing.T) {
	stub := &stubLegacyKill{}
	wrapped := WrapLegacyKill(stub.fn, "ecs", "SSMKill")

	dctx := DriverContext{Container: api.Container{ID: "abc"}}
	if err := wrapped.Kill(dctx, "SIGTERM"); err != nil {
		t.Fatalf("Kill: unexpected error %v", err)
	}
	if stub.gotRef != "abc" {
		t.Errorf("ref: got %q, want abc", stub.gotRef)
	}
	if stub.gotSignal != "SIGTERM" {
		t.Errorf("signal: got %q, want SIGTERM", stub.gotSignal)
	}

	stub.returnErr = errors.New("kill failed")
	if err := wrapped.Kill(dctx, "SIGKILL"); err == nil {
		t.Fatal("expected error to propagate from legacy fn")
	}
}

func TestWrapLegacyKill_PauseAsSIGSTOP(t *testing.T) {
	// Pause/unpause flow through Kill("SIGSTOP") / Kill("SIGCONT") per
	// the SignalDriver contract. Verify the adapter forwards both.
	stub := &stubLegacyKill{}
	wrapped := WrapLegacyKill(stub.fn, "lambda", "ReverseAgentKill")

	dctx := DriverContext{Container: api.Container{ID: "fn-1"}}
	for _, sig := range []string{"SIGSTOP", "SIGCONT"} {
		if err := wrapped.Kill(dctx, sig); err != nil {
			t.Fatalf("Kill(%q): %v", sig, err)
		}
		if stub.gotSignal != sig {
			t.Errorf("Kill(%q): adapter forwarded %q", sig, stub.gotSignal)
		}
	}
}

func TestWrapLegacyKill_Describe(t *testing.T) {
	stub := &stubLegacyKill{}
	got := WrapLegacyKill(stub.fn, "ecs", "SSMKill").Describe()
	if !strings.Contains(got, "ecs") || !strings.Contains(got, "SSMKill") {
		t.Errorf("Describe should name backend + impl, got %q", got)
	}
	if WrapLegacyKill(stub.fn, "", "").Describe() == "" {
		t.Errorf("Describe should never return empty")
	}
}

func TestWrapLegacyKill_NilFn_ReturnsError(t *testing.T) {
	wrapped := WrapLegacyKill(nil, "ecs", "")
	err := wrapped.Kill(DriverContext{}, "SIGTERM")
	if err == nil || !strings.Contains(err.Error(), "function is nil") {
		t.Errorf("expected nil-fn error, got %v", err)
	}
}
