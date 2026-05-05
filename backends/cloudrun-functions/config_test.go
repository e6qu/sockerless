package gcf

import (
	"reflect"
	"testing"
)

// TestParsePrewarmOverlays_HappyPath — comma-separated image:size pairs
// with a registry-prefixed image (the LastIndex split handles the
// `host:port/repo:tag:size` shape correctly).
func TestParsePrewarmOverlays_HappyPath(t *testing.T) {
	got := parsePrewarmOverlays("registry.gitlab.com/foo/runner-helper:v17.5.0:3,alpine:latest:5")
	want := []PrewarmOverlay{
		{Image: "registry.gitlab.com/foo/runner-helper:v17.5.0", Size: 3},
		{Image: "alpine:latest", Size: 5},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePrewarmOverlays = %+v, want %+v", got, want)
	}
}

// TestParsePrewarmOverlays_EmptyInput — empty / whitespace-only env
// returns nil so the prewarm path stays a no-op.
func TestParsePrewarmOverlays_EmptyInput(t *testing.T) {
	for _, in := range []string{"", "   ", ",,,"} {
		if got := parsePrewarmOverlays(in); got != nil {
			t.Errorf("parsePrewarmOverlays(%q) = %+v, want nil", in, got)
		}
	}
}

// TestParsePrewarmOverlays_SkipsMalformed — entries without a numeric
// size or with size <= 0 are dropped (they'd zero-deploy or panic
// downstream). Empty-image entries are dropped too.
func TestParsePrewarmOverlays_SkipsMalformed(t *testing.T) {
	got := parsePrewarmOverlays("good:1,no-colon,bad-size:abc,zero-size:0,negative:-1,:5,alpine:2")
	want := []PrewarmOverlay{
		{Image: "good", Size: 1},
		{Image: "alpine", Size: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsePrewarmOverlays skipped wrong entries; got %+v, want %+v", got, want)
	}
}
