package core

import "testing"

// TestParseTopOutput verifies the `ps` output parser handles the usual
// `ps -ef` layout: variable whitespace, trailing COMMAND column with
// internal spaces, optional blank leading/trailing lines.
func TestParseTopOutput(t *testing.T) {
	raw := `UID        PID  PPID  C STIME TTY          TIME CMD
root         1     0  0 10:00 ?        00:00:01 /usr/bin/app --flag=value
root       123     1  0 10:01 ?        00:00:00 sh -c 'echo hello world'
`
	got := ParseTopOutput(raw)
	wantTitles := []string{"UID", "PID", "PPID", "C", "STIME", "TTY", "TIME", "CMD"}
	if len(got.Titles) != len(wantTitles) {
		t.Fatalf("titles len=%d want %d: %+v", len(got.Titles), len(wantTitles), got.Titles)
	}
	for i, w := range wantTitles {
		if got.Titles[i] != w {
			t.Errorf("title[%d]=%q want %q", i, got.Titles[i], w)
		}
	}
	if len(got.Processes) != 2 {
		t.Fatalf("processes len=%d want 2", len(got.Processes))
	}
	// First row — last col is `/usr/bin/app --flag=value`.
	if got.Processes[0][len(wantTitles)-1] != "/usr/bin/app --flag=value" {
		t.Errorf("row0 CMD = %q", got.Processes[0][len(wantTitles)-1])
	}
	// Second row — last col absorbs spaces through the quoted shell.
	if got.Processes[1][len(wantTitles)-1] != "sh -c 'echo hello world'" {
		t.Errorf("row1 CMD = %q", got.Processes[1][len(wantTitles)-1])
	}
}

func TestParseTopOutput_Empty(t *testing.T) {
	got := ParseTopOutput("")
	if len(got.Titles) != 0 {
		t.Errorf("empty input should have 0 titles, got %+v", got.Titles)
	}
	if len(got.Processes) != 0 {
		t.Errorf("empty input should have 0 processes")
	}
}

func TestParseTopOutput_HeaderOnly(t *testing.T) {
	got := ParseTopOutput("UID PID CMD\n")
	if len(got.Titles) != 3 {
		t.Fatalf("titles=%+v", got.Titles)
	}
	if len(got.Processes) != 0 {
		t.Errorf("header-only should yield 0 processes")
	}
}
