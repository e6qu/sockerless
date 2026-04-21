package aca

import (
	"testing"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// TestResolveACAState_CacheHit returns cached state when JobName is
// populated — no cloud call.
func TestResolveACAState_CacheHit(t *testing.T) {
	s := &Server{
		BaseServer: core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
			ID: "t", Name: "test",
		}, zerolog.Nop()),
		ACA:          core.NewStateStore[ACAState](),
		NetworkState: core.NewStateStore[NetworkState](),
	}
	s.SetSelf(s)
	s.ACA.Put("abc123", ACAState{
		JobName:       "skls-abc",
		ExecutionName: "skls-abc-exec-1",
	})
	got, ok := s.resolveACAState(s.ctx(), "abc123")
	if !ok || got.JobName != "skls-abc" || got.ExecutionName == "" {
		t.Fatalf("expected cache hit, got ok=%v job=%q exec=%q", ok, got.JobName, got.ExecutionName)
	}
}

// TestResolveACAState_MissAndNoCloud returns (zero, false).
func TestResolveACAState_MissAndNoCloud(t *testing.T) {
	s := &Server{
		BaseServer: core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
			ID: "t", Name: "test",
		}, zerolog.Nop()),
		ACA:          core.NewStateStore[ACAState](),
		NetworkState: core.NewStateStore[NetworkState](),
	}
	s.SetSelf(s)
	got, ok := s.resolveACAState(s.ctx(), "nonexistent")
	if ok {
		t.Fatalf("expected cache miss + no cloud = false, got ok=%v job=%q", ok, got.JobName)
	}
}

// TestResolveNetworkState_CacheHit returns cached NetworkState when
// DNSZoneName is populated.
func TestResolveNetworkState_CacheHit(t *testing.T) {
	s := &Server{
		BaseServer: core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
			ID: "t", Name: "test",
		}, zerolog.Nop()),
		ACA:          core.NewStateStore[ACAState](),
		NetworkState: core.NewStateStore[NetworkState](),
	}
	s.SetSelf(s)
	s.NetworkState.Put("netid-xyz", NetworkState{
		DNSZoneName: "skls-foo.local",
		NSGName:     "nsg-dev-foo",
	})
	got, ok := s.resolveNetworkState(s.ctx(), "netid-xyz")
	if !ok || got.DNSZoneName != "skls-foo.local" {
		t.Fatalf("expected cache hit, got ok=%v zone=%q", ok, got.DNSZoneName)
	}
}
