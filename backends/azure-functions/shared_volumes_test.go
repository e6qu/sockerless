package azf

import (
	"errors"
	"strings"
	"testing"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

func TestValidateSurfacesSharedVolumesParseError(t *testing.T) {
	cfg := Config{
		SubscriptionID:   "sub",
		ResourceGroup:    "rg",
		StorageAccount:   "acct",
		NetworkDiscovery: api.NetworkDiscoveryNATGatewayOnly,
		Access:           api.AccessMechanismNoneInternal,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("baseline config should validate, got: %v", err)
	}
	cfg.sharedVolumesErr = errors.New("entry \"junk\" malformed")
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want shared-volumes parse error")
	}
	if !strings.Contains(err.Error(), "SOCKERLESS_AZF_SHARED_VOLUMES") {
		t.Errorf("error %q does not mention SOCKERLESS_AZF_SHARED_VOLUMES", err)
	}
}

// TestConfigFromEnvReadsSharedVolumes pins the variable the Azure Functions
// backend reads and the tuple shape it accepts.
func TestConfigFromEnvReadsSharedVolumes(t *testing.T) {
	t.Setenv("SOCKERLESS_AZF_SUBSCRIPTION_ID", "sub")
	t.Setenv("SOCKERLESS_AZF_RESOURCE_GROUP", "rg")
	t.Setenv("SOCKERLESS_AZF_STORAGE_ACCOUNT", "acct")
	t.Setenv("SOCKERLESS_AZF_SHARED_VOLUMES", "ws=/home/runner/_work=ws-share=azure-files-ephemeral")
	c := ConfigFromEnv()
	if c.sharedVolumesErr != nil {
		t.Fatal(c.sharedVolumesErr)
	}
	sv := c.SharedVolumes.ByName("ws")
	if sv == nil || sv.AzureShareName != "ws-share" || sv.Backing != core.BackingAzureFilesEphemeral || sv.AzureAccountOrDefault(c.StorageAccount) != "acct" {
		t.Fatalf("SharedVolumes = %+v", c.SharedVolumes)
	}
	t.Setenv("SOCKERLESS_AZF_SHARED_VOLUMES", "ws=/home/runner/_work=share")
	if c := ConfigFromEnv(); c.sharedVolumesErr == nil {
		t.Fatal("a declaration without a backing must be carried to Validate as an error")
	}
}
