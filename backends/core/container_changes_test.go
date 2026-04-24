package core

import "testing"

func TestParseChangesOutput(t *testing.T) {
	raw := "d\t/tmp\nf\t/tmp/foo.log\nl\t/tmp/link\n"
	got := ParseChangesOutput(raw)
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3", len(got))
	}
	for i, want := range []string{"/tmp", "/tmp/foo.log", "/tmp/link"} {
		if got[i].Path != want {
			t.Errorf("row[%d].Path = %q want %q", i, got[i].Path, want)
		}
		if got[i].Kind != ChangeKindAdded {
			t.Errorf("row[%d].Kind = %d want ChangeKindAdded", i, got[i].Kind)
		}
	}
}

func TestParseChangesOutput_FiltersProcSys(t *testing.T) {
	raw := "d\t/proc/1\nf\t/sys/kernel/foo\nf\t/tmp/real.log\n"
	got := ParseChangesOutput(raw)
	if len(got) != 1 {
		t.Fatalf("got %d, want 1 (proc/sys filtered)", len(got))
	}
	if got[0].Path != "/tmp/real.log" {
		t.Errorf("path = %q", got[0].Path)
	}
}

func TestParseChangesOutput_Empty(t *testing.T) {
	if len(ParseChangesOutput("")) != 0 {
		t.Errorf("empty input should yield 0 changes")
	}
}
