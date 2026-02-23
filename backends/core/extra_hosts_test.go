package core

import (
	"strings"
	"testing"
)

func TestFormatExtraHostsEnv(t *testing.T) {
	got := FormatExtraHostsEnv([]string{"db:1.2.3.4", "cache:5.6.7.8"})
	if got != "db:1.2.3.4,cache:5.6.7.8" {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestFormatExtraHostsEnv_Empty(t *testing.T) {
	got := FormatExtraHostsEnv(nil)
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestBuildHostsFile(t *testing.T) {
	content := string(BuildHostsFile("myhost", []string{"db:10.0.0.2", "cache:10.0.0.3"}))

	// Check localhost entries
	if !strings.Contains(content, "127.0.0.1\tlocalhost") {
		t.Fatal("missing IPv4 localhost")
	}
	if !strings.Contains(content, "::1\tlocalhost") {
		t.Fatal("missing IPv6 localhost")
	}
	// Check hostname entry
	if !strings.Contains(content, "127.0.0.1\tmyhost") {
		t.Fatal("missing hostname entry")
	}
	// Check extra hosts (IP\thost format)
	if !strings.Contains(content, "10.0.0.2\tdb") {
		t.Fatal("missing db extra host")
	}
	if !strings.Contains(content, "10.0.0.3\tcache") {
		t.Fatal("missing cache extra host")
	}
}
