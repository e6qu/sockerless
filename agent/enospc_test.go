package agent

import (
	"strings"
	"testing"
)

func TestDetectENOSPC(t *testing.T) {
	cases := []struct {
		name   string
		stderr string
		want   bool
	}{
		{"empty", "", false},
		{"unrelated", "command not found", false},
		{"kernel lowercase", "write failed: no space left on device", true},
		{"kernel capitalised", "tar: foo.txt: Cannot write: No space left on device", true},
		{"errno marker", "ENOSPC creating /tmp/x", true},
		{"quota", "Disk quota exceeded", false}, // case-sensitive substring; lowercase test below
		{"quota lowercase", "disk quota exceeded", true},
	}
	for _, c := range cases {
		got := DetectENOSPC([]byte(c.stderr))
		if got != c.want {
			t.Errorf("%s: DetectENOSPC(%q) = %v, want %v", c.name, c.stderr, got, c.want)
		}
	}
}

func TestENOSPC_HelperPure(t *testing.T) {
	// Sanity check: DetectENOSPC + AnnotateENOSPC are pure functions.
	// The exit-code override is the BOOTSTRAP's responsibility (after
	// BUG-1062 — only override on non-zero exit). Verify that
	// DetectENOSPC alone doesn't imply exit-code change.
	stderr := []byte("write failed: no space left on device\n")
	if !DetectENOSPC(stderr) {
		t.Fatal("DetectENOSPC missed marker")
	}
	annotated := AnnotateENOSPC(stderr, "lambda")
	if len(annotated) <= len(stderr) {
		t.Fatalf("AnnotateENOSPC didn't grow stderr; got len=%d, original=%d", len(annotated), len(stderr))
	}
	// Caller decides whether to use ENOSPCExitCode; helper is pure.
	if ENOSPCExitCode != 28 {
		t.Errorf("ENOSPCExitCode = %d, want 28", ENOSPCExitCode)
	}
}

func TestAnnotateENOSPC(t *testing.T) {
	in := []byte("write failed: No space left on device\n")
	out := AnnotateENOSPC(in, "lambda")
	got := string(out)
	if !strings.HasPrefix(got, "sockerless: ENOSPC detected") {
		t.Errorf("annotation missing prefix: %q", got)
	}
	if !strings.Contains(got, "SOCKERLESS_LAMBDA_TMPFS_SIZE_MIB") {
		t.Errorf("annotation missing env-var reference: %q", got)
	}
	if !strings.HasSuffix(got, string(in)) {
		t.Errorf("annotation did not preserve original stderr at tail: %q", got)
	}
}
