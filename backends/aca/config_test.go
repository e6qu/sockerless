package aca

import "testing"

// TestConfig_Validate_UseAppGate — Phase 88 foundation.
// `UseApp=true` is rejected until the Apps code path lands so the
// flag can't silently fall back to Jobs.
func TestConfig_Validate_UseAppGate(t *testing.T) {
	c := Config{SubscriptionID: "s", ResourceGroup: "rg", UseApp: true}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected Validate to reject UseApp=true until Phase 88")
	}
	c.UseApp = false
	if err := c.Validate(); err != nil {
		t.Fatalf("default Jobs path should validate, got: %v", err)
	}
}

// TestConfigFromEnv_UseAppFlag — env var maps to the config field.
func TestConfigFromEnv_UseAppFlag(t *testing.T) {
	t.Setenv("SOCKERLESS_ACA_SUBSCRIPTION_ID", "sub")
	t.Setenv("SOCKERLESS_ACA_RESOURCE_GROUP", "rg")
	t.Setenv("SOCKERLESS_ACA_USE_APP", "1")
	c := ConfigFromEnv()
	if !c.UseApp {
		t.Fatal("expected UseApp=true from env var")
	}
	t.Setenv("SOCKERLESS_ACA_USE_APP", "")
	c = ConfigFromEnv()
	if c.UseApp {
		t.Fatal("expected UseApp=false when env var unset")
	}
}
