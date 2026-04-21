package aca

import "testing"

// TestConfig_Validate_UseAppRequiresEnvironment —.
// UseApp=true needs a managed environment: the whole point of the
// Apps path is peer-reachable internal FQDNs inside an environment
// with VNet integration. Without one we have nothing to bind to.
func TestConfig_Validate_UseAppRequiresEnvironment(t *testing.T) {
	c := Config{SubscriptionID: "s", ResourceGroup: "rg", UseApp: true}
	if err := c.Validate(); err == nil {
		t.Fatal("expected Validate to reject UseApp=true without Environment")
	}
	c.Environment = "my-env"
	if err := c.Validate(); err != nil {
		t.Fatalf("UseApp + Environment should validate, got: %v", err)
	}
	// Default (Jobs path) should continue to validate without an env.
	c.UseApp = false
	c.Environment = ""
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
