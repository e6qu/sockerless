package agent

import (
	"os"
	"syscall"
	"testing"
)

func TestParseSignalValid(t *testing.T) {
	tests := []struct {
		input    string
		expected os.Signal
	}{
		{"TERM", syscall.SIGTERM},
		{"KILL", syscall.SIGKILL},
		{"INT", syscall.SIGINT},
		{"HUP", syscall.SIGHUP},
		{"QUIT", syscall.SIGQUIT},
		{"USR1", syscall.SIGUSR1},
		{"USR2", syscall.SIGUSR2},
	}
	for _, tt := range tests {
		sig := parseSignal(tt.input)
		if sig == nil {
			t.Errorf("parseSignal(%q) returned nil", tt.input)
			continue
		}
		if sig != tt.expected {
			t.Errorf("parseSignal(%q) = %v, want %v", tt.input, sig, tt.expected)
		}
	}
}

func TestParseSignalWithPrefix(t *testing.T) {
	sig := parseSignal("SIGTERM")
	if sig == nil {
		t.Fatal("parseSignal(SIGTERM) returned nil")
	}
	if sig != syscall.SIGTERM {
		t.Errorf("expected SIGTERM, got %v", sig)
	}
}

func TestParseSignalCaseInsensitive(t *testing.T) {
	for _, input := range []string{"sigterm", "Term", "TERM", "SigTerm"} {
		sig := parseSignal(input)
		if sig == nil {
			t.Errorf("parseSignal(%q) returned nil", input)
			continue
		}
		if sig != syscall.SIGTERM {
			t.Errorf("parseSignal(%q) = %v, want SIGTERM", input, sig)
		}
	}
}

func TestParseSignalUnknown(t *testing.T) {
	sig := parseSignal("BOGUS")
	if sig != nil {
		t.Errorf("expected nil for unknown signal, got %v", sig)
	}
}
