package core

import (
	"strings"
	"testing"

	"github.com/sockerless/api"
)

// fakeDriver is a tiny test-only Driver impl. The "fake" prefix is
// intentional and explicit — this is test scaffolding, not a
// production fallback (per the project's no-fakes-in-prod rule).
type fakeDriver struct{ desc string }

func (f *fakeDriver) Describe() string { return f.desc }

func TestRegisterDriverImpl_AndResolve_Happy(t *testing.T) {
	// Reset the registry for this test to avoid cross-test pollution.
	driverImplRegistry = map[driverImplKey]func() Driver{}

	RegisterDriverImpl("ecs", "BUILD", "Kaniko", func() Driver {
		return &fakeDriver{desc: "ecs Kaniko build"}
	})

	t.Setenv("SOCKERLESS_ECS_BUILD", "Kaniko")
	got, ok, err := ResolveDriverFor("ecs", "BUILD")
	if err != nil {
		t.Fatalf("ResolveDriverFor: unexpected error %v", err)
	}
	if !ok {
		t.Fatal("ResolveDriverFor: expected ok=true when env is set + impl is registered")
	}
	if got == nil || got.Describe() != "ecs Kaniko build" {
		t.Fatalf("ResolveDriverFor: got %v / Describe %q", got, describe(got))
	}
}

func TestRegisterDriverImpl_DimensionCaseInsensitive(t *testing.T) {
	driverImplRegistry = map[driverImplKey]func() Driver{}

	// Lowercase dimension at register-time, mixed-case at lookup-time.
	RegisterDriverImpl("lambda", "fsdiff", "OverlayUpper", func() Driver {
		return &fakeDriver{desc: "lambda overlay-upper diff"}
	})

	t.Setenv("SOCKERLESS_LAMBDA_FSDIFF", "OverlayUpper")
	got, ok, err := ResolveDriverFor("lambda", "FSDiff")
	if err != nil {
		t.Fatalf("ResolveDriverFor: unexpected error %v", err)
	}
	if !ok {
		t.Fatal("ResolveDriverFor: expected case-insensitive dimension match")
	}
	if got.Describe() != "lambda overlay-upper diff" {
		t.Fatalf("Describe: got %q", got.Describe())
	}
}

func TestResolveDriverFor_NoEnv_ReturnsNoOverride(t *testing.T) {
	driverImplRegistry = map[driverImplKey]func() Driver{}
	RegisterDriverImpl("ecs", "EXEC", "SSMExec", func() Driver { return &fakeDriver{} })

	// No env var set.
	got, ok, err := ResolveDriverFor("ecs", "EXEC")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when env var is not set")
	}
	if got != nil {
		t.Fatal("expected nil driver when no override")
	}
}

func TestResolveDriverFor_UnknownImpl_ReturnsError(t *testing.T) {
	driverImplRegistry = map[driverImplKey]func() Driver{}
	RegisterDriverImpl("ecs", "BUILD", "Kaniko", func() Driver { return &fakeDriver{} })

	t.Setenv("SOCKERLESS_ECS_BUILD", "DoesNotExist")
	_, _, err := ResolveDriverFor("ecs", "BUILD")
	if err == nil {
		t.Fatal("expected error for unknown impl")
	}
	var ipe *api.InvalidParameterError
	switch e := err.(type) {
	case *api.InvalidParameterError:
		ipe = e
	}
	if ipe == nil {
		t.Fatalf("expected InvalidParameterError, got %T: %v", err, err)
	}
	if !strings.Contains(ipe.Message, "SOCKERLESS_ECS_BUILD=DoesNotExist") {
		t.Errorf("error should name the env var + value, got %q", ipe.Message)
	}
	if !strings.Contains(ipe.Message, "unknown driver impl") {
		t.Errorf("error should say 'unknown driver impl', got %q", ipe.Message)
	}
}

func TestNotImplDriverError_Shape(t *testing.T) {
	err := NotImplDriverError("exec",
		"a reverse-agent bootstrap inside the Lambda container (SOCKERLESS_CALLBACK_URL); no session registered")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.HasPrefix(err.Message, "docker exec requires ") {
		t.Errorf("expected message to start with 'docker exec requires ', got %q", err.Message)
	}
	if !strings.Contains(err.Message, "SOCKERLESS_CALLBACK_URL") {
		t.Errorf("expected message to surface the missing prerequisite, got %q", err.Message)
	}
}

// describe is a nil-safe wrapper used in failure messages above.
func describe(d Driver) string {
	if d == nil {
		return "<nil>"
	}
	return d.Describe()
}
