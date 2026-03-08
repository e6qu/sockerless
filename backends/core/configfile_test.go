package core

import (
	"os"
	"path/filepath"
	"testing"
)

const testConfig = `
simulators:
  sim-aws:
    cloud: aws
    port: 4566
    log_level: debug
  sim-gcp:
    cloud: gcp
    port: 4567
    grpc_port: 4568

environments:
  dev-ecs:
    backend: ecs
    addr: ":2375"
    log_level: info
    simulator: sim-aws
    aws:
      region: us-east-1
      ecs:
        cluster: sockerless
        subnets: [subnet-abc123]
        execution_role_arn: arn:aws:iam::123456:role/ecsExec
        log_group: /sockerless
        assign_public_ip: false
    common:
      agent_image: sockerless/agent:latest
      poll_interval: 2s
      agent_timeout: 30s

  dev-lambda:
    backend: lambda
    addr: ":2378"
    simulator: sim-aws
    aws:
      region: us-east-1
      lambda:
        role_arn: arn:aws:iam::123456:role/lambdaExec
        memory_size: 1024
        timeout: 900

  prod-ecs:
    backend: ecs
    addr: ":2375"
    aws:
      region: us-west-2
      ecs:
        cluster: prod-cluster
        subnets: [subnet-prod1, subnet-prod2]
        execution_role_arn: arn:aws:iam::123456:role/prodExec
    common:
      agent_image: sockerless/agent:v1.2.3
      callback_url: https://backend.prod.example.com

  dev-cloudrun:
    backend: cloudrun
    addr: ":2376"
    simulator: sim-gcp
    gcp:
      project: my-gcp-project
      cloudrun:
        region: us-central1
`

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigFile(t *testing.T) {
	path := writeTestConfig(t, testConfig)
	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(cfg.Simulators) != 2 {
		t.Fatalf("expected 2 simulators, got %d", len(cfg.Simulators))
	}
	if len(cfg.Environments) != 4 {
		t.Fatalf("expected 4 environments, got %d", len(cfg.Environments))
	}

	sim := cfg.Simulators["sim-aws"]
	if sim.Cloud != "aws" || sim.Port != 4566 || sim.LogLevel != "debug" {
		t.Errorf("sim-aws: got cloud=%q port=%d log_level=%q", sim.Cloud, sim.Port, sim.LogLevel)
	}

	simGCP := cfg.Simulators["sim-gcp"]
	if simGCP.GRPCPort != 4568 {
		t.Errorf("sim-gcp: expected grpc_port=4568, got %d", simGCP.GRPCPort)
	}

	env := cfg.Environments["dev-ecs"]
	if env.Backend != "ecs" {
		t.Errorf("dev-ecs: expected backend=ecs, got %q", env.Backend)
	}
	if env.Simulator != "sim-aws" {
		t.Errorf("dev-ecs: expected simulator=sim-aws, got %q", env.Simulator)
	}
	if env.AWS == nil || env.AWS.ECS == nil {
		t.Fatal("dev-ecs: missing AWS/ECS config")
	}
	if env.AWS.ECS.Cluster != "sockerless" {
		t.Errorf("dev-ecs: expected cluster=sockerless, got %q", env.AWS.ECS.Cluster)
	}
	if len(env.AWS.ECS.Subnets) != 1 || env.AWS.ECS.Subnets[0] != "subnet-abc123" {
		t.Errorf("dev-ecs: unexpected subnets: %v", env.AWS.ECS.Subnets)
	}
	if env.Common.PollInterval != "2s" {
		t.Errorf("dev-ecs: expected poll_interval=2s, got %q", env.Common.PollInterval)
	}

	prodECS := cfg.Environments["prod-ecs"]
	if prodECS.Simulator != "" {
		t.Errorf("prod-ecs: expected no simulator, got %q", prodECS.Simulator)
	}
	if len(prodECS.AWS.ECS.Subnets) != 2 {
		t.Errorf("prod-ecs: expected 2 subnets, got %d", len(prodECS.AWS.ECS.Subnets))
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := LoadConfigFile("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfigFileInvalidYAML(t *testing.T) {
	path := writeTestConfig(t, "{{invalid yaml")
	_, err := LoadConfigFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadConfigFileEmpty(t *testing.T) {
	path := writeTestConfig(t, "")
	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Environments) != 0 {
		t.Errorf("expected 0 environments, got %d", len(cfg.Environments))
	}
	if len(cfg.Simulators) != 0 {
		t.Errorf("expected 0 simulators, got %d", len(cfg.Simulators))
	}
}

func TestResolveSimulator(t *testing.T) {
	path := writeTestConfig(t, testConfig)
	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Environment with simulator reference
	env := cfg.Environments["dev-ecs"]
	sim, err := cfg.ResolveSimulator(env)
	if err != nil {
		t.Fatal(err)
	}
	if sim == nil {
		t.Fatal("expected non-nil simulator")
	}
	if sim.Cloud != "aws" || sim.Port != 4566 {
		t.Errorf("expected aws:4566, got %s:%d", sim.Cloud, sim.Port)
	}

	// Environment without simulator
	prodEnv := cfg.Environments["prod-ecs"]
	sim, err = cfg.ResolveSimulator(prodEnv)
	if err != nil {
		t.Fatal(err)
	}
	if sim != nil {
		t.Error("expected nil simulator for prod-ecs")
	}

	// Invalid reference
	badEnv := &Environment{Simulator: "nonexistent"}
	_, err = cfg.ResolveSimulator(badEnv)
	if err == nil {
		t.Error("expected error for missing simulator reference")
	}
}

func TestValidate(t *testing.T) {
	path := writeTestConfig(t, testConfig)
	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config, got: %v", err)
	}
}

func TestValidateBadSimulatorRef(t *testing.T) {
	path := writeTestConfig(t, `
environments:
  broken:
    backend: ecs
    simulator: nonexistent
`)
	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for bad simulator ref")
	}
}

func TestValidateBadCloud(t *testing.T) {
	path := writeTestConfig(t, `
simulators:
  bad:
    cloud: oracle
environments: {}
`)
	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for bad cloud")
	}
}

func TestValidateMissingBackend(t *testing.T) {
	path := writeTestConfig(t, `
environments:
  nobackend:
    addr: ":2375"
`)
	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for missing backend")
	}
}

func TestSaveAndRoundTrip(t *testing.T) {
	path := writeTestConfig(t, testConfig)
	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Save to a new path
	newPath := filepath.Join(t.TempDir(), "out.yaml")
	if err := cfg.Save(newPath); err != nil {
		t.Fatal(err)
	}

	// Reload and verify
	cfg2, err := LoadConfigFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg2.Environments) != len(cfg.Environments) {
		t.Errorf("round-trip: expected %d environments, got %d", len(cfg.Environments), len(cfg2.Environments))
	}
	if len(cfg2.Simulators) != len(cfg.Simulators) {
		t.Errorf("round-trip: expected %d simulators, got %d", len(cfg.Simulators), len(cfg2.Simulators))
	}

	env := cfg2.Environments["dev-ecs"]
	if env.AWS == nil || env.AWS.ECS == nil || env.AWS.ECS.Cluster != "sockerless" {
		t.Error("round-trip: dev-ecs ECS cluster not preserved")
	}
}

func TestAddRemoveEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &UnifiedConfig{
		Environments: make(map[string]*Environment),
		Simulators:   make(map[string]*SimulatorConfig),
	}

	env := &Environment{Backend: "ecs", Addr: ":2375"}
	if err := cfg.AddEnvironment("test", env, path); err != nil {
		t.Fatal(err)
	}

	cfg2, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg2.Environments["test"]; !ok {
		t.Error("environment 'test' not found after add")
	}

	if err := cfg2.RemoveEnvironment("test", path); err != nil {
		t.Fatal(err)
	}

	cfg3, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg3.Environments["test"]; ok {
		t.Error("environment 'test' still exists after remove")
	}
}

func TestRemoveEnvironmentNotFound(t *testing.T) {
	cfg := &UnifiedConfig{Environments: make(map[string]*Environment)}
	err := cfg.RemoveEnvironment("nope", "/dev/null")
	if err == nil {
		t.Error("expected error removing nonexistent environment")
	}
}

func TestAddRemoveSimulator(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &UnifiedConfig{
		Environments: make(map[string]*Environment),
		Simulators:   make(map[string]*SimulatorConfig),
	}

	sim := &SimulatorConfig{Cloud: "aws", Port: 4566}
	if err := cfg.AddSimulator("sim-aws", sim, path); err != nil {
		t.Fatal(err)
	}

	cfg2, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg2.Simulators["sim-aws"]; !ok {
		t.Error("simulator 'sim-aws' not found after add")
	}

	if err := cfg2.RemoveSimulator("sim-aws", path); err != nil {
		t.Fatal(err)
	}

	cfg3, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg3.Simulators["sim-aws"]; ok {
		t.Error("simulator 'sim-aws' still exists after remove")
	}
}

func TestRemoveSimulatorInUse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &UnifiedConfig{
		Simulators: map[string]*SimulatorConfig{
			"sim-aws": {Cloud: "aws", Port: 4566},
		},
		Environments: map[string]*Environment{
			"dev-ecs": {Backend: "ecs", Simulator: "sim-aws"},
		},
	}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	err := cfg.RemoveSimulator("sim-aws", path)
	if err == nil {
		t.Error("expected error removing simulator in use")
	}
}

func TestActiveEnvironment(t *testing.T) {
	dir := t.TempDir()

	// Write config.yaml
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(testConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write active file
	if err := os.WriteFile(filepath.Join(dir, "active"), []byte("dev-ecs\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Override paths
	t.Setenv("SOCKERLESS_HOME", dir)
	t.Setenv("SOCKERLESS_CONFIG", configPath)
	t.Setenv("SOCKERLESS_CONTEXT", "")

	env, name, err := ActiveEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if name != "dev-ecs" {
		t.Errorf("expected name=dev-ecs, got %q", name)
	}
	if env.Backend != "ecs" {
		t.Errorf("expected backend=ecs, got %q", env.Backend)
	}
}

func TestActiveEnvironmentWithContextEnv(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(testConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SOCKERLESS_HOME", dir)
	t.Setenv("SOCKERLESS_CONFIG", configPath)
	t.Setenv("SOCKERLESS_CONTEXT", "dev-lambda")

	env, name, err := ActiveEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if name != "dev-lambda" {
		t.Errorf("expected name=dev-lambda, got %q", name)
	}
	if env.Backend != "lambda" {
		t.Errorf("expected backend=lambda, got %q", env.Backend)
	}
}

func TestActiveEnvironmentNoActive(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SOCKERLESS_HOME", dir)
	t.Setenv("SOCKERLESS_CONTEXT", "")

	_, _, err := ActiveEnvironment()
	if err == nil {
		t.Error("expected error when no active environment")
	}
}

func TestActiveEnvironmentNotInConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("environments: {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "active"), []byte("nonexistent\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SOCKERLESS_HOME", dir)
	t.Setenv("SOCKERLESS_CONFIG", configPath)
	t.Setenv("SOCKERLESS_CONTEXT", "")

	_, _, err := ActiveEnvironment()
	if err == nil {
		t.Error("expected error for nonexistent active environment")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	t.Setenv("SOCKERLESS_CONFIG", "/custom/path.yaml")
	if got := DefaultConfigPath(); got != "/custom/path.yaml" {
		t.Errorf("expected /custom/path.yaml, got %q", got)
	}

	t.Setenv("SOCKERLESS_CONFIG", "")
	t.Setenv("SOCKERLESS_HOME", "/home/test/.sockerless")
	got := DefaultConfigPath()
	if got != "/home/test/.sockerless/config.yaml" {
		t.Errorf("expected /home/test/.sockerless/config.yaml, got %q", got)
	}
}
