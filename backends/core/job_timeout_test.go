package core

import "testing"

func TestJobTimeoutDefault(t *testing.T) {
	t.Setenv(JobTimeoutEnvName, "")
	if got := JobTimeoutDefault(); got != DefaultJobTimeoutSeconds {
		t.Errorf("empty → %d, want %d", got, DefaultJobTimeoutSeconds)
	}
	t.Setenv(JobTimeoutEnvName, "120")
	if got := JobTimeoutDefault(); got != 120 {
		t.Errorf("120 → %d, want 120", got)
	}
	t.Setenv(JobTimeoutEnvName, "garbage")
	if got := JobTimeoutDefault(); got != DefaultJobTimeoutSeconds {
		t.Errorf("garbage → %d, want default", got)
	}
	t.Setenv(JobTimeoutEnvName, "-5")
	if got := JobTimeoutDefault(); got != 0 {
		t.Errorf("-5 → %d, want 0 (clamped)", got)
	}
}

func TestJobTimeoutEnvIfUnset(t *testing.T) {
	t.Setenv(JobTimeoutEnvName, "60")

	// User didn't set → backend default appended.
	got := JobTimeoutEnvIfUnset([]string{"FOO=bar"})
	want := "SOCKERLESS_JOB_TIMEOUT_SECONDS=60"
	if got != want {
		t.Errorf("missing user env: got %q, want %q", got, want)
	}

	// User set → empty (user's wins).
	got = JobTimeoutEnvIfUnset([]string{"FOO=bar", "SOCKERLESS_JOB_TIMEOUT_SECONDS=999"})
	if got != "" {
		t.Errorf("user-set env: got %q, want empty", got)
	}
}
