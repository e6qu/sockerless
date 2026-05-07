package spawner

import "testing"

func TestJobIDFromName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"projects/p/locations/us-central1/jobs/gh-abc1234-123456", "gh-abc1234-123456"},
		{"projects/p/locations/r/jobs/foo", "foo"},
		// Defensive — caller passes a bare jobID instead of a full name.
		{"gh-abc1234-123456", "gh-abc1234-123456"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := JobIDFromName(tt.in); got != tt.want {
			t.Errorf("JobIDFromName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestJobIDFromRunnerName_deterministic(t *testing.T) {
	// Same runner name + GitHub jobID must always produce the same Cloud
	// Run Job ID — that determinism is what the orphan-Service sweep
	// relies on (the Service's owner label has to match a value the
	// dispatcher can reproduce from the runner-task's state).
	a := jobIDFromRunnerName("dispatcher-gcp-9001-1700000000", 9001)
	b := jobIDFromRunnerName("dispatcher-gcp-9001-1700000000", 9001)
	if a != b {
		t.Errorf("not deterministic: %s vs %s", a, b)
	}
}
