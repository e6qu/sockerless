package cloudrun

import (
	"errors"
	"strings"
	"testing"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

func TestValidateSurfacesSharedVolumesParseError(t *testing.T) {
	cfg := Config{
		Project:          "p",
		NetworkDiscovery: api.NetworkDiscoveryCloudDNS,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("baseline config should validate, got: %v", err)
	}
	cfg.sharedVolumesErr = errors.New("entry \"junk\" malformed")
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want shared-volumes parse error")
	}
	if !strings.Contains(err.Error(), "SOCKERLESS_GCP_SHARED_VOLUMES") {
		t.Errorf("error %q does not mention SOCKERLESS_GCP_SHARED_VOLUMES", err)
	}
}

// TestConfigFromEnvReadsSharedVolumes pins the variable the Cloud Run
// backend reads and the tuple shape it accepts.
func TestConfigFromEnvReadsSharedVolumes(t *testing.T) {
	t.Setenv("SOCKERLESS_GCR_PROJECT", "p")
	t.Setenv("SOCKERLESS_GCP_SHARED_VOLUMES", "ws=/home/runner/_work=ws-bucket=gcs-sync")
	c := ConfigFromEnv()
	if c.sharedVolumesErr != nil {
		t.Fatal(c.sharedVolumesErr)
	}
	sv := c.SharedVolumes.ByName("ws")
	if sv == nil || sv.GCSBucket != "ws-bucket" || sv.Backing != core.BackingGCSSync {
		t.Fatalf("SharedVolumes = %+v", c.SharedVolumes)
	}
	t.Setenv("SOCKERLESS_GCP_SHARED_VOLUMES", "ws=/home/runner/_work=bucket")
	if c := ConfigFromEnv(); c.sharedVolumesErr == nil {
		t.Fatal("a declaration without a backing must be carried to Validate as an error")
	}
}
