package main

import (
	"path/filepath"
	"testing"
)

func TestTopologyManagerEmptyOnFirstLoad(t *testing.T) {
	dir := t.TempDir()
	mgr := NewTopologyManager(filepath.Join(dir, "sockerless.yaml"))
	if err := mgr.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	got := mgr.Get()
	if len(got.Projects) != 0 {
		t.Errorf("fresh manager: want 0 projects, got %d", len(got.Projects))
	}
	if len(got.Ports.Ranges) == 0 {
		t.Errorf("fresh manager should default port ranges")
	}
}

func TestTopologyManagerReplace(t *testing.T) {
	dir := t.TempDir()
	mgr := NewTopologyManager(filepath.Join(dir, "sockerless.yaml"))
	if err := mgr.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	next := Topology{Projects: []ProjectConfig{
		{Name: "p", Instances: []Instance{
			{Name: "s", Kind: InstanceKindSim, Cloud: CloudGCP, Port: 4567},
		}},
	}}
	if err := mgr.Replace(next); err != nil {
		t.Fatalf("replace: %v", err)
	}
	got := mgr.Get()
	if len(got.Projects) != 1 || got.Projects[0].Instances[0].Cloud != CloudGCP {
		t.Errorf("replace didn't update state: %+v", got)
	}

	// Invalid replace should be rejected without state change.
	bad := Topology{Projects: []ProjectConfig{
		{Name: "BAD!", Instances: []Instance{}},
	}}
	if err := mgr.Replace(bad); err == nil {
		t.Errorf("invalid topology should fail Replace")
	}
	got2 := mgr.Get()
	if len(got2.Projects) != 1 || got2.Projects[0].Name != "p" {
		t.Errorf("rejected replace mutated state: %+v", got2)
	}
}

func TestTopologyManagerInstancesFlat(t *testing.T) {
	dir := t.TempDir()
	mgr := NewTopologyManager(filepath.Join(dir, "sockerless.yaml"))
	_ = mgr.Load()
	if err := mgr.Replace(Topology{Projects: []ProjectConfig{
		{Name: "p1", Instances: []Instance{
			{Name: "s", Kind: InstanceKindSim, Cloud: CloudAWS, Port: 4500},
			{Name: "be", Kind: InstanceKindBackend, Cloud: CloudAWS, Backend: BackendECS, Port: 3300, Sim: "s"},
		}},
		{Name: "p2", Instances: []Instance{
			{Name: "bleep", Kind: InstanceKindBleephub, Port: 5500},
		}},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	flat := mgr.Instances()
	if len(flat) != 3 {
		t.Errorf("flat: want 3, got %d (%+v)", len(flat), flat)
	}
	if flat[0].Project != "p1" {
		t.Errorf("flat[0].Project = %q, want p1", flat[0].Project)
	}

	// Find: hit + miss.
	if got, ok := mgr.FindInstance("p1", "be"); !ok || got.Instance.Backend != BackendECS {
		t.Errorf("FindInstance miss or wrong: %+v ok=%v", got, ok)
	}
	if _, ok := mgr.FindInstance("p1", "absent"); ok {
		t.Errorf("FindInstance should miss")
	}
	if _, ok := mgr.FindInstance("absent", "s"); ok {
		t.Errorf("FindInstance should miss on absent project")
	}
}

func TestTopologyManagerGetIsCopy(t *testing.T) {
	dir := t.TempDir()
	mgr := NewTopologyManager(filepath.Join(dir, "sockerless.yaml"))
	_ = mgr.Load()
	_ = mgr.Replace(Topology{Projects: []ProjectConfig{
		{Name: "p", Instances: []Instance{
			{Name: "s", Kind: InstanceKindSim, Cloud: CloudAWS, Port: 4500,
				Config: map[string]string{"k": "v"}},
		}},
	}})
	got := mgr.Get()
	got.Projects[0].Name = "MUTATED"
	got.Projects[0].Instances[0].Config["k"] = "MUTATED"
	again := mgr.Get()
	if again.Projects[0].Name != "p" {
		t.Errorf("Get should return defensive copy; project name leaked: %q", again.Projects[0].Name)
	}
	if again.Projects[0].Instances[0].Config["k"] != "v" {
		t.Errorf("Get should deep-copy Config; mutation leaked")
	}
}
