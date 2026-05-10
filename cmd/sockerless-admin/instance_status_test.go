package main

import (
	"os"
	"path/filepath"
	"testing"
)

// chdirToInstanceStatus pins a temp dir as cwd so the package's
// "<cwd>/.stack-pids/" lookups land in the test's sandbox.
func chdirToInstanceStatus(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	if err := os.MkdirAll(filepath.Join(tmp, ".stack-pids"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return tmp
}

func writeExitFile(t *testing.T, name, body string) {
	t.Helper()
	cwd, _ := os.Getwd()
	path := filepath.Join(cwd, ".stack-pids", name+".exit")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write exit: %v", err)
	}
}

func TestReadExitRecordMissing(t *testing.T) {
	chdirToInstanceStatus(t)
	if got := readExitRecord("ghost"); got != nil {
		t.Errorf("missing exit file should be nil, got %+v", got)
	}
}

func TestReadExitRecordValid(t *testing.T) {
	chdirToInstanceStatus(t)
	writeExitFile(t, "sim-aws", "0 2026-05-10T12:34:56Z\n")
	got := readExitRecord("sim-aws")
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.Code != 0 || got.At != "2026-05-10T12:34:56Z" {
		t.Errorf("got %+v", got)
	}
}

func TestReadExitRecordNonZero(t *testing.T) {
	chdirToInstanceStatus(t)
	writeExitFile(t, "sim-aws", "137 2026-05-10T12:34:56Z\n")
	got := readExitRecord("sim-aws")
	if got == nil || got.Code != 137 {
		t.Errorf("expected code 137, got %+v", got)
	}
}

func TestReadExitRecordCodeOnly(t *testing.T) {
	// Tolerate single-token form for forward-compat.
	chdirToInstanceStatus(t)
	writeExitFile(t, "sim-aws", "1\n")
	got := readExitRecord("sim-aws")
	if got == nil || got.Code != 1 || got.At != "" {
		t.Errorf("got %+v", got)
	}
}

func TestReadExitRecordGarbage(t *testing.T) {
	chdirToInstanceStatus(t)
	writeExitFile(t, "sim-aws", "not-an-int 2026-05-10T12:34:56Z\n")
	if got := readExitRecord("sim-aws"); got != nil {
		t.Errorf("garbage should yield nil, got %+v", got)
	}
}

func TestReadInstanceStatusCrashedSinceStart(t *testing.T) {
	chdirToInstanceStatus(t)
	cwd, _ := os.Getwd()

	// Pidfile points at a definitely-dead PID; exit record present.
	pidfile := filepath.Join(cwd, ".stack-pids", "sim-aws.pid")
	if err := os.WriteFile(pidfile, []byte("99999999\n"), 0o644); err != nil {
		t.Fatalf("write pidfile: %v", err)
	}
	writeExitFile(t, "sim-aws", "1 2026-05-10T12:34:56Z\n")

	status := readInstanceStatus(Instance{
		Name: "sim-aws",
		Kind: InstanceKindSim,
		Port: 1, // unused — Running=false short-circuits
	})

	if status.Running {
		t.Errorf("Running should be false (pid is dead)")
	}
	if status.PID != 99999999 {
		t.Errorf("PID = %d", status.PID)
	}
	if status.Exit == nil || status.Exit.Code != 1 {
		t.Errorf("Exit not surfaced: %+v", status.Exit)
	}
	if !status.CrashedSinceStart {
		t.Errorf("CrashedSinceStart should be true: pidfile present + dead + exit record")
	}
}

func TestReadInstanceStatusCleanStop(t *testing.T) {
	// stop-component removes the pidfile, so even if an exit record
	// lingers there's no Running=false-with-pidfile signal. The
	// status reports !Running with PID=0, no CrashedSinceStart.
	chdirToInstanceStatus(t)
	writeExitFile(t, "sim-aws", "0 2026-05-10T12:34:56Z\n")

	status := readInstanceStatus(Instance{
		Name: "sim-aws",
		Kind: InstanceKindSim,
		Port: 1,
	})
	if status.Running || status.PID != 0 {
		t.Errorf("clean-stop should report !Running PID=0, got %+v", status)
	}
	if status.CrashedSinceStart {
		t.Errorf("clean-stop should not be flagged CrashedSinceStart")
	}
	// Exit record still surfaces — useful for showing "last exit was
	// graceful at <ts>".
	if status.Exit == nil || status.Exit.Code != 0 {
		t.Errorf("Exit should surface even on clean stop, got %+v", status.Exit)
	}
}
