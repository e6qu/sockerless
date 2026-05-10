package main

import (
	"path/filepath"
	"testing"
)

func TestManagedEnvForSim(t *testing.T) {
	got := managedEnvFor("proj-a", Instance{
		Name: "sim-aws",
		Kind: InstanceKindSim,
	}, "/repo/.sockerless-state")

	want := filepath.Join("/repo/.sockerless-state", "proj-a", "sim-aws")
	if got["SIM_DATA_DIR"] != want {
		t.Errorf("SIM_DATA_DIR = %q, want %q", got["SIM_DATA_DIR"], want)
	}
}

func TestManagedEnvForBackend(t *testing.T) {
	// Only sim instances get admin-managed env entries today. Backends
	// rely entirely on operator config + the make targets; admin must
	// not synthesise a SIM_DATA_DIR for them (would leak the env name
	// into a context where it has no meaning).
	got := managedEnvFor("proj-a", Instance{
		Name: "be-ecs",
		Kind: InstanceKindBackend,
	}, "/repo/.sockerless-state")

	if got != nil {
		t.Errorf("backend should get no managed env, got %+v", got)
	}
}

func TestManagedEnvForBleephub(t *testing.T) {
	got := managedEnvFor("proj-a", Instance{
		Name: "bh",
		Kind: InstanceKindBleephub,
	}, "/repo/.sockerless-state")

	if got != nil {
		t.Errorf("bleephub should get no managed env, got %+v", got)
	}
}

func TestMergeConfigOperatorWins(t *testing.T) {
	managed := map[string]string{
		"SIM_DATA_DIR":  "/admin/path",
		"SIM_LOG_LEVEL": "info",
	}
	operator := map[string]string{
		// Operator wants state on a separate volume.
		"SIM_DATA_DIR": "/mnt/big-disk/sim-state",
		"EXTRA":        "operator-only",
	}
	got := mergeConfig(managed, operator)

	if got["SIM_DATA_DIR"] != "/mnt/big-disk/sim-state" {
		t.Errorf("operator override lost: %q", got["SIM_DATA_DIR"])
	}
	if got["SIM_LOG_LEVEL"] != "info" {
		t.Errorf("admin-managed entry dropped: %q", got["SIM_LOG_LEVEL"])
	}
	if got["EXTRA"] != "operator-only" {
		t.Errorf("operator-only entry dropped: %q", got["EXTRA"])
	}
}

func TestMergeConfigEmptyInputs(t *testing.T) {
	if got := mergeConfig(nil, nil); got != nil {
		t.Errorf("nil+nil should return nil, got %+v", got)
	}
	if got := mergeConfig(map[string]string{}, map[string]string{}); got != nil {
		t.Errorf("empty+empty should return nil, got %+v", got)
	}
	if got := mergeConfig(map[string]string{"a": "1"}, nil); got["a"] != "1" {
		t.Errorf("managed-only path: %+v", got)
	}
	if got := mergeConfig(nil, map[string]string{"a": "1"}); got["a"] != "1" {
		t.Errorf("operator-only path: %+v", got)
	}
}
