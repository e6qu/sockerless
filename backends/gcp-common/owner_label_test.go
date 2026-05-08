package gcpcommon

import (
	"strings"
	"testing"
)

func TestSanitizeOwnerLabel(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"gh-abc1234-123456", "gh-abc1234-123456"},
		{"GH-ABC1234-123456", "gh-abc1234-123456"},
		{"foo.bar/baz", "foo-bar-baz"},
		{strings.Repeat("a", 100), strings.Repeat("a", 63)},
	}
	for _, tt := range tests {
		got := sanitizeOwnerLabel(tt.in)
		if got != tt.want {
			t.Errorf("sanitizeOwnerLabel(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestOwnerRunnerTaskLabelValue_unset(t *testing.T) {
	// CLOUD_RUN_JOB unset = sockerless not running inside a Cloud Run
	// Job — no owner label written.
	t.Setenv(OwnerRunnerTaskEnv, "")
	if got := OwnerRunnerTaskLabelValue(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestOwnerRunnerTaskLabelValue_set(t *testing.T) {
	// Cloud Run sets CLOUD_RUN_JOB to the Job's ID (the last segment of
	// the Job resource name). Sockerless reads it directly without
	// dispatcher cooperation, keeping the dispatcher generic.
	t.Setenv(OwnerRunnerTaskEnv, "gh-abc1234-987654")
	if got := OwnerRunnerTaskLabelValue(); got != "gh-abc1234-987654" {
		t.Errorf("got %q", got)
	}
}
