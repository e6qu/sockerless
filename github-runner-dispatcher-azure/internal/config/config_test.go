package config

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const validLabel = `
[[label]]
name             = "sockerless-aca"
subscription_id  = "00000000-0000-0000-0000-000000000000"
resource_group   = "runners-rg"
environment      = "/subscriptions/x/resourceGroups/runners-rg/providers/Microsoft.App/managedEnvironments/env"
location         = "eastus2"
image            = "acr.azurecr.io/runner:latest"
`

func TestLoadMissingFileIsEmpty(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("missing file: %v", err)
	}
	if len(cfg.Labels) != 0 {
		t.Fatalf("expected empty config, got %d labels", len(cfg.Labels))
	}
}

func TestLoadAppliesTimeoutDefault(t *testing.T) {
	cfg, err := Load(write(t, validLabel))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Labels[0].RunnerJobTimeout; got != DefaultRunnerJobTimeout {
		t.Fatalf("RunnerJobTimeout = %d, want default %d", got, DefaultRunnerJobTimeout)
	}
	if got := cfg.Labels[0].MaxConcurrent; got != 0 {
		t.Fatalf("MaxConcurrent = %d, want 0 (unbounded)", got)
	}
}

func TestLoadExplicitKnobs(t *testing.T) {
	cfg, err := Load(write(t, validLabel+`
runner_job_timeout = 7200
max_concurrent     = 5
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Labels[0].RunnerJobTimeout; got != 7200 {
		t.Fatalf("RunnerJobTimeout = %d, want 7200", got)
	}
	if got := cfg.Labels[0].MaxConcurrent; got != 5 {
		t.Fatalf("MaxConcurrent = %d, want 5", got)
	}
}

func TestLoadRejectsNegativeKnobs(t *testing.T) {
	if _, err := Load(write(t, validLabel+"runner_job_timeout = -1\n")); err == nil {
		t.Fatal("negative runner_job_timeout accepted")
	}
	if _, err := Load(write(t, validLabel+"max_concurrent = -1\n")); err == nil {
		t.Fatal("negative max_concurrent accepted")
	}
}

func TestLoadRejectsMissingRequired(t *testing.T) {
	for _, missing := range []string{"subscription_id", "resource_group", "environment", "location", "image"} {
		body := ""
		for _, line := range []string{
			`name             = "l"`,
			`subscription_id  = "s"`,
			`resource_group   = "rg"`,
			`environment      = "env"`,
			`location         = "eastus2"`,
			`image            = "img"`,
		} {
			if !startsWithKey(line, missing) {
				body += line + "\n"
			}
		}
		if _, err := Load(write(t, "[[label]]\n"+body)); err == nil {
			t.Errorf("config without %s accepted", missing)
		}
	}
}

func startsWithKey(line, key string) bool {
	return len(line) >= len(key) && line[:len(key)] == key
}

func TestLookupLabel(t *testing.T) {
	cfg, err := Load(write(t, validLabel))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LookupLabel("sockerless-aca") == nil {
		t.Fatal("LookupLabel missed existing label")
	}
	if cfg.LookupLabel("nope") != nil {
		t.Fatal("LookupLabel matched nonexistent label")
	}
}
