package cloudrun

import "testing"

// TestConfig_Validate_UseServiceRequiresVPCConnector —.
// UseService=true needs a VPC connector: the whole point of the
// Services path is peer-reachable internal DNS over the connector.
// Without one, the CNAME records we write would target a URL that
// isn't reachable from sibling Services.
func TestConfig_Validate_UseServiceRequiresVPCConnector(t *testing.T) {
	c := Config{Project: "p", UseService: true}
	if err := c.Validate(); err == nil {
		t.Fatal("expected Validate to reject UseService=true without VPCConnector")
	}
	c.VPCConnector = "projects/p/locations/us-central1/connectors/c1"
	if err := c.Validate(); err != nil {
		t.Fatalf("UseService + VPCConnector should validate, got: %v", err)
	}
	// Default (Jobs path) should continue to validate without a connector.
	c.UseService = false
	c.VPCConnector = ""
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
