package core

import (
	"testing"

	"github.com/sockerless/api"
)

func mkC(id, status string, running bool, code int) api.Container {
	return api.Container{ID: id, State: api.ContainerState{Status: status, Running: running, ExitCode: code}}
}

func TestScanContainersForExit(t *testing.T) {
	list := []api.Container{
		mkC("a", "running", true, 0),
		mkC("b", "exited", false, 7),
	}
	// Running target → found, not exited.
	if exit, found, exited := ScanContainersForExit(list, "a"); !found || exited || exit != 0 {
		t.Fatalf("running: got exit=%d found=%v exited=%v", exit, found, exited)
	}
	// Exited target → found + exited + the exit code.
	if exit, found, exited := ScanContainersForExit(list, "b"); !found || !exited || exit != 7 {
		t.Fatalf("exited: got exit=%d found=%v exited=%v", exit, found, exited)
	}
	// Absent target → not found (caller counts this toward WaitGoneThreshold).
	if _, found, exited := ScanContainersForExit(list, "zzz"); found || exited {
		t.Fatalf("absent: found=%v exited=%v, want both false", found, exited)
	}
	if WaitGoneThreshold < 1 {
		t.Fatalf("WaitGoneThreshold must be positive, got %d", WaitGoneThreshold)
	}
}
