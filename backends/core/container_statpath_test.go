package core

import (
	"os"
	"testing"
	"time"
)

// TestParseStatOutput covers regular-file, directory, and symlink
// cases of `stat -c '%n\t%s\t%f\t%Y\t%N'`.
func TestParseStatOutput_File(t *testing.T) {
	// `/etc/hostname`, 14 bytes, mode 0100644 (regular file, 0644),
	// mtime 1735689600 (2025-01-01 UTC), %N prints `'path'`.
	raw := "/etc/hostname\t14\t81a4\t1735689600\t'/etc/hostname'"
	got, err := ParseStatOutput(raw, "/etc/hostname")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Name != "hostname" {
		t.Errorf("Name=%q want hostname", got.Name)
	}
	if got.Size != 14 {
		t.Errorf("Size=%d want 14", got.Size)
	}
	if got.Mode.Perm() != 0o644 {
		t.Errorf("Mode.Perm()=%o want 0644", got.Mode.Perm())
	}
	if got.Mode&os.ModeType != 0 {
		t.Errorf("expected regular file, got mode type bits=%v", got.Mode&os.ModeType)
	}
	if got.Mtime.Unix() != 1735689600 {
		t.Errorf("Mtime=%v want 2025-01-01 UTC", got.Mtime)
	}
	_ = time.Time{}
	if got.LinkTarget != "" {
		t.Errorf("LinkTarget=%q want empty", got.LinkTarget)
	}
}

func TestParseStatOutput_Symlink(t *testing.T) {
	// symlink at /etc/os-release → ../usr/lib/os-release, mode 0120777
	raw := "/etc/os-release\t21\t"
	raw += "a1ff" + "\t1700000000\t'/etc/os-release' -> '../usr/lib/os-release'"
	got, err := ParseStatOutput(raw, "/etc/os-release")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Mode&os.ModeSymlink == 0 {
		t.Errorf("expected symlink bit, got mode=%v", got.Mode)
	}
	if got.LinkTarget != "../usr/lib/os-release" {
		t.Errorf("LinkTarget=%q", got.LinkTarget)
	}
}

func TestParseStatOutput_Directory(t *testing.T) {
	// mode 041755 = dir + 0755
	raw := "/var\t4096\t41ed\t1700000000\t'/var'"
	got, err := ParseStatOutput(raw, "/var")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !got.Mode.IsDir() {
		t.Errorf("expected dir bit, got mode=%v", got.Mode)
	}
	if got.Mode.Perm() != 0o755 {
		t.Errorf("Perm()=%o want 0755", got.Mode.Perm())
	}
}

func TestParseStatOutput_Malformed(t *testing.T) {
	if _, err := ParseStatOutput("bogus", "/x"); err == nil {
		t.Errorf("expected error on malformed input")
	}
}
