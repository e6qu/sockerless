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
