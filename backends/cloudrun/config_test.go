package cloudrun

import "testing"

// TestConfig_Validate_UseServiceGate — Phase 87 foundation.
// `UseService=true` is rejected until the Services code path lands,
// so the flag can't silently fall back to Jobs.
func TestConfig_Validate_UseServiceGate(t *testing.T) {
	c := Config{Project: "p", UseService: true}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected Validate to reject UseService=true until Phase 87")
	}
	// Default (Jobs path) should continue to validate.
	c.UseService = false
	if err := c.Validate(); err != nil {
		t.Fatalf("default Jobs path should validate, got: %v", err)
	}
}

// TestConfigFromEnv_UseServiceFlag — env var maps to the config field.
func TestConfigFromEnv_UseServiceFlag(t *testing.T) {
	t.Setenv("SOCKERLESS_GCR_PROJECT", "test-proj")
	t.Setenv("SOCKERLESS_GCR_USE_SERVICE", "1")
	c := ConfigFromEnv()
	if !c.UseService {
		t.Fatal("expected UseService=true from env var")
	}
	t.Setenv("SOCKERLESS_GCR_USE_SERVICE", "")
	c = ConfigFromEnv()
	if c.UseService {
		t.Fatal("expected UseService=false when env var unset")
	}
}
