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
name            = "sockerless-cloudrun"
gcp_project     = "my-project"
gcp_region      = "us-central1"
image           = "us-central1-docker.pkg.dev/my-project/runners/runner:latest"
service_account = "github-runners@my-project.iam.gserviceaccount.com"
`

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
max_concurrent     = 8
`))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Labels[0].RunnerJobTimeout; got != 7200 {
		t.Fatalf("RunnerJobTimeout = %d, want 7200", got)
	}
	if got := cfg.Labels[0].MaxConcurrent; got != 8 {
		t.Fatalf("MaxConcurrent = %d, want 8", got)
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

func TestLoadWorkspaceBackingRequiredWithBucket(t *testing.T) {
	if _, err := Load(write(t, validLabel+`runner_workspace_bucket = "b"`+"\n")); err == nil {
		t.Fatal("bucket without backing accepted")
	}
	cfg, err := Load(write(t, validLabel+`
runner_workspace_bucket  = "b"
runner_workspace_backing = "gcs-sync"
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Labels[0].RunnerWorkspaceBacking != "gcs-sync" {
		t.Fatalf("backing = %q", cfg.Labels[0].RunnerWorkspaceBacking)
	}
}
