package main

import (
	"strings"
	"testing"
)

func TestSimulatorEnv(t *testing.T) {
	env := SimulatorEnv(CloudAWS, 4566, "debug")
	if len(env) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(env))
	}
	if env[0] != "SIM_LISTEN_ADDR=:4566" {
		t.Errorf("env[0] = %s, want SIM_LISTEN_ADDR=:4566", env[0])
	}
	if env[1] != "SIM_LOG_LEVEL=debug" {
		t.Errorf("env[1] = %s, want SIM_LOG_LEVEL=debug", env[1])
	}
}

func TestSimulatorEnvNoLogLevel(t *testing.T) {
	env := SimulatorEnv(CloudGCP, 5000, "")
	if len(env) != 1 {
		t.Fatalf("expected 1 env var, got %d", len(env))
	}
}

func TestBackendEnvECS(t *testing.T) {
	env := BackendEnv(CloudAWS, BackendECS, 4566, "test-project")
	hasEndpoint := false
	hasCluster := false
	hasRegion := false
	for _, e := range env {
		if e == "SOCKERLESS_ENDPOINT_URL=http://localhost:4566" {
			hasEndpoint = true
		}
		if e == "SOCKERLESS_ECS_CLUSTER=test-project-cluster" {
			hasCluster = true
		}
		if e == "AWS_REGION=us-east-1" {
			hasRegion = true
		}
	}
	if !hasEndpoint {
		t.Error("missing SOCKERLESS_ENDPOINT_URL")
	}
	if !hasCluster {
		t.Error("missing SOCKERLESS_ECS_CLUSTER")
	}
	if !hasRegion {
		t.Error("missing AWS_REGION")
	}
}

func TestBackendEnvLambda(t *testing.T) {
	env := BackendEnv(CloudAWS, BackendLambda, 4566, "test")
	hasRole := false
	for _, e := range env {
		if strings.HasPrefix(e, "SOCKERLESS_LAMBDA_ROLE_ARN=") {
			hasRole = true
		}
	}
	if !hasRole {
		t.Error("missing SOCKERLESS_LAMBDA_ROLE_ARN")
	}
}

func TestBackendEnvCloudRun(t *testing.T) {
	env := BackendEnv(CloudGCP, BackendCloudRun, 5000, "test")
	hasProject := false
	for _, e := range env {
		if e == "SOCKERLESS_GCR_PROJECT=sim-project" {
			hasProject = true
		}
	}
	if !hasProject {
		t.Error("missing SOCKERLESS_GCR_PROJECT")
	}
}

func TestBackendEnvGCF(t *testing.T) {
	env := BackendEnv(CloudGCP, BackendGCF, 5000, "test")
	hasProject := false
	for _, e := range env {
		if e == "SOCKERLESS_GCF_PROJECT=sim-project" {
			hasProject = true
		}
	}
	if !hasProject {
		t.Error("missing SOCKERLESS_GCF_PROJECT")
	}
}

func TestBackendEnvACA(t *testing.T) {
	env := BackendEnv(CloudAzure, BackendACA, 6000, "test")
	hasSub := false
	hasRG := false
	for _, e := range env {
		if strings.HasPrefix(e, "SOCKERLESS_ACA_SUBSCRIPTION_ID=") {
			hasSub = true
		}
		if e == "SOCKERLESS_ACA_RESOURCE_GROUP=sim-rg" {
			hasRG = true
		}
	}
	if !hasSub {
		t.Error("missing SOCKERLESS_ACA_SUBSCRIPTION_ID")
	}
	if !hasRG {
		t.Error("missing SOCKERLESS_ACA_RESOURCE_GROUP")
	}
}

func TestBackendEnvAZF(t *testing.T) {
	env := BackendEnv(CloudAzure, BackendAZF, 6000, "test")
	hasStorage := false
	for _, e := range env {
		if e == "SOCKERLESS_AZF_STORAGE_ACCOUNT=simstorage" {
			hasStorage = true
		}
	}
	if !hasStorage {
		t.Error("missing SOCKERLESS_AZF_STORAGE_ACCOUNT")
	}
}

func TestBackendArgs(t *testing.T) {
	args := BackendArgs(9100, "info")
	if len(args) != 4 {
		t.Fatalf("expected 4 args, got %d", len(args))
	}
	if args[0] != "-addr" || args[1] != ":9100" {
		t.Errorf("unexpected addr args: %v", args[:2])
	}
	if args[2] != "-log-level" || args[3] != "info" {
		t.Errorf("unexpected log-level args: %v", args[2:])
	}
}

func TestFrontendArgs(t *testing.T) {
	args := FrontendArgs(2375, 9100, 9200, "debug")
	if len(args) != 8 {
		t.Fatalf("expected 8 args, got %d: %v", len(args), args)
	}
	if args[1] != ":2375" {
		t.Errorf("frontend port = %s, want :2375", args[1])
	}
	if args[3] != "http://localhost:9100" {
		t.Errorf("backend = %s, want http://localhost:9100", args[3])
	}
	if args[5] != ":9200" {
		t.Errorf("mgmt port = %s, want :9200", args[5])
	}
}

func TestBootstrapSimulatorNoop(t *testing.T) {
	// Non-ECS backends should be a no-op
	err := BootstrapSimulator(CloudGCP, BackendCloudRun, "http://localhost:5000", "test", nil)
	if err != nil {
		t.Errorf("expected no-op, got error: %v", err)
	}

	err = BootstrapSimulator(CloudAzure, BackendACA, "http://localhost:6000", "test", nil)
	if err != nil {
		t.Errorf("expected no-op, got error: %v", err)
	}
}
