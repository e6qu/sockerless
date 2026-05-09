package main

import (
	"testing"
)

func TestIsValidInstanceKind(t *testing.T) {
	for _, k := range AllInstanceKinds {
		if !IsValidInstanceKind(k) {
			t.Errorf("AllInstanceKinds entry %q reports !IsValid", k)
		}
	}
	if IsValidInstanceKind("") {
		t.Error(`IsValidInstanceKind("") should be false`)
	}
	if IsValidInstanceKind("garbage") {
		t.Error(`IsValidInstanceKind("garbage") should be false`)
	}
}

func TestIsValidInstanceName(t *testing.T) {
	cases := map[string]bool{
		"":           false,
		"a":          true,
		"my-sim":     true,
		"sim_1":      true,
		"-leading":   false,
		"UPPER":      false,
		"with space": false,
		"with.dot":   false,
	}
	for in, want := range cases {
		if got := IsValidInstanceName(in); got != want {
			t.Errorf("IsValidInstanceName(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestInstanceValidate(t *testing.T) {
	cases := []struct {
		name    string
		inst    Instance
		wantErr bool
	}{
		{
			name: "sim ok",
			inst: Instance{Name: "my-sim", Kind: InstanceKindSim, Cloud: CloudAWS, Port: 4566},
		},
		{
			name:    "sim missing cloud",
			inst:    Instance{Name: "my-sim", Kind: InstanceKindSim, Port: 4566},
			wantErr: true,
		},
		{
			name: "backend ok",
			inst: Instance{Name: "my-be", Kind: InstanceKindBackend, Cloud: CloudAWS, Backend: BackendECS, Port: 3375},
		},
		{
			name:    "backend wrong cloud-backend pair",
			inst:    Instance{Name: "my-be", Kind: InstanceKindBackend, Cloud: CloudAWS, Backend: BackendCloudRun, Port: 3375},
			wantErr: true,
		},
		{
			name: "bleephub ok",
			inst: Instance{Name: "my-bleep", Kind: InstanceKindBleephub, Port: 5555},
		},
		{
			name:    "bad kind",
			inst:    Instance{Name: "x", Kind: InstanceKind("garbage"), Port: 1},
			wantErr: true,
		},
		{
			name:    "bad name",
			inst:    Instance{Name: "Up", Kind: InstanceKindBleephub, Port: 1},
			wantErr: true,
		},
		{
			name:    "zero port",
			inst:    Instance{Name: "x", Kind: InstanceKindBleephub, Port: 0},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.inst.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("Validate(%+v): want error, got nil", tc.inst)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Validate(%+v): want nil, got %v", tc.inst, err)
			}
		})
	}
}

func TestDeriveLegacyInstances(t *testing.T) {
	// Old-shape project with sim + backend → 2 instances, sim linked.
	p := ProjectConfig{
		Name:        "myproj",
		Cloud:       CloudGCP,
		Backend:     BackendCloudRun,
		SimPort:     4567,
		BackendPort: 3375,
	}
	got := DeriveLegacyInstances(p)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2; got=%+v", len(got), got)
	}
	if got[0].Kind != InstanceKindSim || got[0].Name != "myproj-sim" || got[0].Port != 4567 {
		t.Errorf("sim derivation wrong: %+v", got[0])
	}
	if got[1].Kind != InstanceKindBackend || got[1].Name != "myproj-backend" ||
		got[1].Port != 3375 || got[1].Sim != "myproj-sim" {
		t.Errorf("backend derivation wrong: %+v", got[1])
	}

	// Backend-only (no sim) — Sim field is empty.
	p2 := ProjectConfig{Name: "p2", Cloud: CloudAWS, Backend: BackendLambda, BackendPort: 3375}
	got2 := DeriveLegacyInstances(p2)
	if len(got2) != 1 || got2[0].Kind != InstanceKindBackend || got2[0].Sim != "" {
		t.Errorf("backend-only derivation wrong: %+v", got2)
	}

	// Empty config → empty list (no panic).
	got3 := DeriveLegacyInstances(ProjectConfig{Name: "p3"})
	if len(got3) != 0 {
		t.Errorf("empty derivation: got %+v", got3)
	}
}
