package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestTopologyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sockerless.yaml")

	want := &Topology{
		Projects: []ProjectConfig{
			{
				Name: "proj-a",
				Instances: []Instance{
					{Name: "sim-a", Kind: InstanceKindSim, Cloud: CloudAWS, Port: 4566},
					{Name: "ecs-a", Kind: InstanceKindBackend, Cloud: CloudAWS, Backend: BackendECS, Port: 3375, Sim: "sim-a",
						Config: map[string]string{"AWS_REGION": "us-east-1"}},
				},
			},
			{
				Name: "proj-b",
				Instances: []Instance{
					{Name: "bleep-b", Kind: InstanceKindBleephub, Port: 5500},
				},
			},
		},
		Ports: PortConfig{Ranges: DefaultPortRanges()},
	}
	if err := SaveTopology(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadTopology(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got.Projects) != 2 {
		t.Fatalf("projects: want 2, got %d", len(got.Projects))
	}
	if got.Projects[0].Instances[0].Cloud != CloudAWS {
		t.Errorf("project 0 sim cloud = %q, want aws", got.Projects[0].Instances[0].Cloud)
	}
	if got.Projects[0].Instances[1].Sim != "sim-a" {
		t.Errorf("project 0 backend sim ref = %q, want sim-a", got.Projects[0].Instances[1].Sim)
	}
	if got.Projects[0].Instances[1].Config["AWS_REGION"] != "us-east-1" {
		t.Errorf("project 0 backend config didn't round-trip: %+v", got.Projects[0].Instances[1].Config)
	}
}

func TestLoadTopologyMissing(t *testing.T) {
	_, err := LoadTopology(filepath.Join(t.TempDir(), "absent.yaml"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("want ErrNotExist, got %v", err)
	}
}

func TestTopologyValidate(t *testing.T) {
	cases := []struct {
		name    string
		t       Topology
		wantErr bool
	}{
		{
			name: "ok minimal",
			t: Topology{Projects: []ProjectConfig{
				{Name: "p", Instances: []Instance{
					{Name: "s", Kind: InstanceKindSim, Cloud: CloudAWS, Port: 4500},
				}},
			}},
		},
		{
			name: "duplicate project name",
			t: Topology{Projects: []ProjectConfig{
				{Name: "p", Instances: []Instance{{Name: "s", Kind: InstanceKindBleephub, Port: 5500}}},
				{Name: "p", Instances: []Instance{{Name: "s2", Kind: InstanceKindBleephub, Port: 5501}}},
			}},
			wantErr: true,
		},
		{
			name: "duplicate instance name within project",
			t: Topology{Projects: []ProjectConfig{
				{Name: "p", Instances: []Instance{
					{Name: "x", Kind: InstanceKindSim, Cloud: CloudAWS, Port: 4500},
					{Name: "x", Kind: InstanceKindBleephub, Port: 5500},
				}},
			}},
			wantErr: true,
		},
		{
			name: "duplicate port across projects",
			t: Topology{Projects: []ProjectConfig{
				{Name: "p1", Instances: []Instance{{Name: "a", Kind: InstanceKindBleephub, Port: 5500}}},
				{Name: "p2", Instances: []Instance{{Name: "b", Kind: InstanceKindBleephub, Port: 5500}}},
			}},
			wantErr: true,
		},
		{
			name: "backend sim ref unknown",
			t: Topology{Projects: []ProjectConfig{
				{Name: "p", Instances: []Instance{
					{Name: "be", Kind: InstanceKindBackend, Cloud: CloudAWS, Backend: BackendECS, Port: 3375, Sim: "nope"},
				}},
			}},
			wantErr: true,
		},
		{
			name: "backend sim ref ok",
			t: Topology{Projects: []ProjectConfig{
				{Name: "p", Instances: []Instance{
					{Name: "s", Kind: InstanceKindSim, Cloud: CloudAWS, Port: 4500},
					{Name: "be", Kind: InstanceKindBackend, Cloud: CloudAWS, Backend: BackendECS, Port: 3375, Sim: "s"},
				}},
			}},
		},
		{
			name: "bad port range",
			t: Topology{
				Projects: []ProjectConfig{{Name: "p"}},
				Ports:    PortConfig{Ranges: map[InstanceKind]PortRange{InstanceKindSim: {From: 100, To: 50}}},
			},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.t.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("want error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("want nil, got %v", err)
			}
		})
	}
}

func TestMigrateLegacyProjects(t *testing.T) {
	dir := t.TempDir()
	if err := SaveProject(dir, &ProjectConfig{
		Name: "old1", Cloud: CloudAWS, Backend: BackendECS, SimPort: 4566, BackendPort: 3375,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := SaveProject(dir, &ProjectConfig{
		Name: "old2", Cloud: CloudGCP, Backend: BackendCloudRun, SimPort: 4567, BackendPort: 3376,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := MigrateLegacyProjects(dir)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(got.Projects) != 2 {
		t.Fatalf("projects: want 2, got %d", len(got.Projects))
	}
	// Stable order — old1 before old2.
	if got.Projects[0].Name != "old1" || got.Projects[1].Name != "old2" {
		t.Errorf("project order wrong: %v %v", got.Projects[0].Name, got.Projects[1].Name)
	}
	if len(got.Projects[0].Instances) != 2 {
		t.Errorf("old1 instances: want 2, got %d", len(got.Projects[0].Instances))
	}
	if got.Projects[0].Instances[0].Kind != InstanceKindSim {
		t.Errorf("old1[0] kind = %q, want sim", got.Projects[0].Instances[0].Kind)
	}
	if got.Projects[0].Instances[1].Sim != "old1-sim" {
		t.Errorf("old1[1] sim ref = %q, want old1-sim", got.Projects[0].Instances[1].Sim)
	}
	if len(got.Ports.Ranges) == 0 {
		t.Errorf("default port ranges not seeded")
	}
}

func TestMigrateLegacyProjects_Missing(t *testing.T) {
	_, err := MigrateLegacyProjects(filepath.Join(t.TempDir(), "absent"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("want ErrNotExist, got %v", err)
	}
}
