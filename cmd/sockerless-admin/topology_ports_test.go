package main

import (
	"path/filepath"
	"testing"
)

func TestAllocatePortHappy(t *testing.T) {
	dir := t.TempDir()
	mgr := NewTopologyManager(filepath.Join(dir, "sockerless.yaml"))
	if err := mgr.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	port, err := mgr.AllocatePort(InstanceKindSim)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	r := DefaultPortRanges()[InstanceKindSim]
	if port < r.From || port > r.To {
		t.Errorf("port %d outside default simulator range [%d, %d]", port, r.From, r.To)
	}
}

func TestAllocatePortSkipsClaimed(t *testing.T) {
	dir := t.TempDir()
	mgr := NewTopologyManager(filepath.Join(dir, "sockerless.yaml"))
	_ = mgr.Load()
	// Claim the bottom of the simulator range explicitly so allocation
	// has to skip past it. Replace fully overrides Ports too, so seed
	// the ranges back so AllocatePort can find them.
	r := DefaultPortRanges()[InstanceKindSim]
	if err := mgr.Replace(Topology{
		Projects: []ProjectConfig{
			{Name: "p", Instances: []Instance{
				{Name: "claimed", Kind: InstanceKindSim, Cloud: CloudAWS, Port: r.From},
			}},
		},
		Ports: PortConfig{Ranges: DefaultPortRanges()},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	port, err := mgr.AllocatePort(InstanceKindSim)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if port == r.From {
		t.Errorf("allocator returned %d which is already claimed", port)
	}
}

func TestAllocatePortNoRange(t *testing.T) {
	dir := t.TempDir()
	mgr := NewTopologyManager(filepath.Join(dir, "sockerless.yaml"))
	_ = mgr.Load()
	// Replace with a topology that explicitly drops the backend range.
	if err := mgr.Replace(Topology{
		Ports: PortConfig{Ranges: map[InstanceKind]PortRange{
			InstanceKindSim: {From: 4500, To: 4999},
		}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := mgr.AllocatePort(InstanceKindBackend); err == nil {
		t.Errorf("want error when kind has no range, got nil")
	}
}

func TestAllocatePortExhausted(t *testing.T) {
	dir := t.TempDir()
	mgr := NewTopologyManager(filepath.Join(dir, "sockerless.yaml"))
	_ = mgr.Load()
	// Tiny range — claim every port.
	if err := mgr.Replace(Topology{
		Projects: []ProjectConfig{{Name: "p", Instances: []Instance{
			{Name: "a", Kind: InstanceKindSim, Cloud: CloudAWS, Port: 56000},
			{Name: "b", Kind: InstanceKindSim, Cloud: CloudGCP, Port: 56001},
		}}},
		Ports: PortConfig{Ranges: map[InstanceKind]PortRange{
			InstanceKindSim: {From: 56000, To: 56001},
		}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := mgr.AllocatePort(InstanceKindSim); err == nil {
		t.Errorf("want exhaustion error, got nil")
	}
}
