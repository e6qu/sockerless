package ecs

import (
	"errors"
	"strings"
	"testing"

	"github.com/sockerless/api"
)

func TestValidateSurfacesSharedVolumesParseError(t *testing.T) {
	cfg := Config{
		Cluster:          "c",
		Subnets:          []string{"subnet-1"},
		ExecutionRoleARN: "arn:aws:iam::000000000000:role/x",
		CpuArchitecture:  "ARM64",
		NetworkDiscovery: api.NetworkDiscoveryServiceMesh,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("baseline config should validate, got: %v", err)
	}
	cfg.sharedVolumesErr = errors.New("entry \"junk\" malformed")
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want shared-volumes parse error")
	}
	if !strings.Contains(err.Error(), "SOCKERLESS_ECS_SHARED_VOLUMES") {
		t.Errorf("error %q does not mention SOCKERLESS_ECS_SHARED_VOLUMES", err)
	}
}

// TestConfigFromEnvReadsSharedVolumes pins the variable the ECS backend
// reads and the tuple shape it accepts.
func TestConfigFromEnvReadsSharedVolumes(t *testing.T) {
	t.Setenv("SOCKERLESS_ECS_SHARED_VOLUMES", "ws=/home/runner/_work=fsap-1=fs-1")
	c := ConfigFromEnv()
	if c.sharedVolumesErr != nil {
		t.Fatal(c.sharedVolumesErr)
	}
	sv := c.SharedVolumes.ByName("ws")
	if sv == nil || sv.ContainerPath != "/home/runner/_work" || sv.EFSAccessPointID != "fsap-1" || sv.EFSFileSystemID != "fs-1" {
		t.Fatalf("SharedVolumes = %+v", c.SharedVolumes)
	}
	t.Setenv("SOCKERLESS_ECS_SHARED_VOLUMES", "junk")
	if c := ConfigFromEnv(); c.sharedVolumesErr == nil {
		t.Fatal("malformed variable must be carried to Validate")
	}
}
