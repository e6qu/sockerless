package cloudrun

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

func TestMapCPUMemory_HonorsRequest(t *testing.T) {
	const gib = int64(1024 * 1024 * 1024)
	cases := []struct {
		name    string
		hc      api.HostConfig
		wantCPU string
		wantMem string
	}{
		{
			name:    "zero request → default floor",
			hc:      api.HostConfig{},
			wantCPU: "1",
			wantMem: "4Gi",
		},
		{
			name:    "small memory snaps to 1 vCPU tier minimum",
			hc:      api.HostConfig{Memory: 256 * 1024 * 1024},
			wantCPU: "1",
			wantMem: "512Mi",
		},
		{
			name:    "2Gi memory fits 1 vCPU",
			hc:      api.HostConfig{Memory: 2 * gib},
			wantCPU: "1",
			wantMem: "2048Mi",
		},
		{
			name:    "6Gi memory forces 2 vCPU tier",
			hc:      api.HostConfig{Memory: 6 * gib},
			wantCPU: "2",
			wantMem: "6144Mi",
		},
		{
			name:    "explicit 2 vCPU request honored",
			hc:      api.HostConfig{NanoCPUs: 2e9},
			wantCPU: "2",
			wantMem: "512Mi",
		},
		{
			name:    "4 vCPU + 12Gi",
			hc:      api.HostConfig{NanoCPUs: 4e9, Memory: 12 * gib},
			wantCPU: "4",
			wantMem: "12288Mi",
		},
		{
			name:    "MemoryReservation used when Memory unset",
			hc:      api.HostConfig{MemoryReservation: 3 * gib},
			wantCPU: "1",
			wantMem: "3072Mi",
		},
		{
			name:    "request beyond largest tier clamps to max",
			hc:      api.HostConfig{NanoCPUs: 16e9, Memory: 64 * gib},
			wantCPU: "8",
			wantMem: "32768Mi",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cpu, mem := mapCPUMemory(ciWithHostConfig(tc.hc))
			if cpu != tc.wantCPU || mem != tc.wantMem {
				t.Fatalf("mapCPUMemory = (%q,%q), want (%q,%q)", cpu, mem, tc.wantCPU, tc.wantMem)
			}
		})
	}
}
