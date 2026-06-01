package realexec

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCapabilityErrorIsTyped(t *testing.T) {
	report := CapabilityReport{Missing: []string{"linux host", "command:ip"}}
	err := report.Require()
	var missing *ErrMissingCapability
	if !errors.As(err, &missing) {
		t.Fatalf("Require returned %T, want ErrMissingCapability", err)
	}
	if len(missing.Missing) != 2 {
		t.Fatalf("missing count = %d, want 2", len(missing.Missing))
	}
}

func TestLinuxEffectiveCapabilities(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status")
	if err := os.WriteFile(path, []byte("Name:\ttest\nCapEff:\t0000000000201000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mask, err := linuxEffectiveCapabilities(path)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCapability(mask, capNetAdmin) {
		t.Fatalf("expected CAP_NET_ADMIN in mask")
	}
	if !hasCapability(mask, capSysAdmin) {
		t.Fatalf("expected CAP_SYS_ADMIN in mask")
	}
}
