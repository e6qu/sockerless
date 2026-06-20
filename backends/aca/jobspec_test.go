package aca

import (
	"testing"

	"github.com/sockerless/api"
)

func ciWithHostConfig(hc api.HostConfig) containerInput {
	return containerInput{
		ID:        "c0123456789abc",
		IsMain:    true,
		Container: &api.Container{HostConfig: hc},
	}
}

func TestMapCPUTier_HonorsRequest(t *testing.T) {
	const gib = int64(1024 * 1024 * 1024)
	cases := []struct {
		name    string
		hc      api.HostConfig
		wantCPU float64
		wantMem string
	}{
		{
			name:    "zero request → default floor",
			hc:      api.HostConfig{},
			wantCPU: 2.0,
			wantMem: "4Gi",
		},
		{
			name:    "256Mi snaps to smallest 0.25/0.5Gi tier",
			hc:      api.HostConfig{Memory: 256 * 1024 * 1024},
			wantCPU: 0.25,
			wantMem: "0.5Gi",
		},
		{
			name:    "1Gi memory → 0.5 vCPU (2:1 ratio)",
			hc:      api.HostConfig{Memory: 1 * gib},
			wantCPU: 0.5,
			wantMem: "1Gi",
		},
		{
			name:    "3Gi memory pulls CPU to 1.5",
			hc:      api.HostConfig{Memory: 3 * gib},
			wantCPU: 1.5,
			wantMem: "3Gi",
		},
		{
			name:    "explicit 1 vCPU request → 1.0/2Gi",
			hc:      api.HostConfig{NanoCPUs: 1e9},
			wantCPU: 1.0,
			wantMem: "2Gi",
		},
		{
			name:    "memory-heavy request forces the 4.0/8Gi tier",
			hc:      api.HostConfig{Memory: 6 * gib},
			wantCPU: 4.0,
			wantMem: "8Gi",
		},
		{
			name:    "MemoryReservation used when Memory unset",
			hc:      api.HostConfig{MemoryReservation: 2 * gib},
			wantCPU: 1.0,
			wantMem: "2Gi",
		},
		{
			name:    "request beyond largest tier clamps to max",
			hc:      api.HostConfig{NanoCPUs: 8e9, Memory: 32 * gib},
			wantCPU: 4.0,
			wantMem: "8Gi",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cpu, mem := mapCPUTier(ciWithHostConfig(tc.hc))
			if cpu != tc.wantCPU || mem != tc.wantMem {
				t.Fatalf("mapCPUTier = (%v,%q), want (%v,%q)", cpu, mem, tc.wantCPU, tc.wantMem)
			}
		})
	}
}
