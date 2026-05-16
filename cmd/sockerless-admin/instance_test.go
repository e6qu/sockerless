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
