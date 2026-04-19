package ecs

import (
	"strings"
	"testing"
)

// TestJoinWarnings covers the warning-string helper used by NetworkCreate
// to combine the BaseServer's warning with cloud-side failure messages.
// Behaviour asserted by BUG-700's fix: multiple errors get merged into
// a single semicolon-separated string matching Docker's Warning field
// convention.
func TestJoinWarnings(t *testing.T) {
	cases := []struct {
		name     string
		existing string
		extras   []string
		want     string
	}{
		{"no existing, one extra", "", []string{"cloud map failed"}, "cloud map failed"},
		{"no existing, two extras", "", []string{"sg", "dns"}, "sg; dns"},
		{"existing + two extras", "bridge already exists", []string{"sg", "dns"}, "bridge already exists; sg; dns"},
		{"existing only", "bridge already exists", nil, "bridge already exists"},
		{"neither", "", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := joinWarnings(tc.existing, tc.extras...)
			if got != tc.want {
				t.Fatalf("joinWarnings(%q, %v) = %q, want %q", tc.existing, tc.extras, got, tc.want)
			}
		})
	}
}

// TestJoinWarnings_SurfacesCloudMapFailure documents the exact
// phrasing clients will see when the Cloud Map namespace can't be
// created — the message mentions "Cloud Map" so a user reading the
// warning knows cross-container DNS won't work for this network.
func TestJoinWarnings_SurfacesCloudMapFailure(t *testing.T) {
	got := joinWarnings("", "Cloud Map namespace (cross-container DNS): DescribeSubnets returned empty")
	if !strings.Contains(got, "Cloud Map") || !strings.Contains(got, "cross-container DNS") {
		t.Fatalf("warning missing expected phrasing: %q", got)
	}
}
