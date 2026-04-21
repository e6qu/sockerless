package cloudrun

import (
	"testing"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// TestResolveCloudRunState_CacheHit returns the cached state when
// JobName is populated — no cloud call.
func TestResolveCloudRunState_CacheHit(t *testing.T) {
	s := &Server{
		BaseServer: core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
			ID: "t", Name: "test",
		}, zerolog.Nop()),
		CloudRun:     core.NewStateStore[CloudRunState](),
		NetworkState: core.NewStateStore[NetworkState](),
	}
	s.SetSelf(s)
	s.CloudRun.Put("abc123", CloudRunState{
		JobName:       "projects/p/locations/us/jobs/skls-abc",
		ExecutionName: "projects/p/locations/us/jobs/skls-abc/executions/e1",
	})
	got, ok := s.resolveCloudRunState(s.ctx(), "abc123")
	if !ok || got.JobName == "" || got.ExecutionName == "" {
		t.Fatalf("expected cache hit, got ok=%v job=%q exec=%q", ok, got.JobName, got.ExecutionName)
	}
}

// TestResolveCloudRunState_MissAndNoCloud returns (zero, false).
func TestResolveCloudRunState_MissAndNoCloud(t *testing.T) {
	s := &Server{
		BaseServer: core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
			ID: "t", Name: "test",
		}, zerolog.Nop()),
		CloudRun:     core.NewStateStore[CloudRunState](),
		NetworkState: core.NewStateStore[NetworkState](),
	}
	s.SetSelf(s)
	got, ok := s.resolveCloudRunState(s.ctx(), "nonexistent")
	if ok {
		t.Fatalf("expected cache miss + no cloud = false, got ok=%v job=%q", ok, got.JobName)
	}
}

// TestResolveNetworkState_CacheHit returns the cached NetworkState
// when ManagedZoneName is populated.
func TestResolveNetworkState_CacheHit(t *testing.T) {
	s := &Server{
		BaseServer: core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
			ID: "t", Name: "test",
		}, zerolog.Nop()),
		CloudRun:     core.NewStateStore[CloudRunState](),
		NetworkState: core.NewStateStore[NetworkState](),
	}
	s.SetSelf(s)
	s.NetworkState.Put("netid-xyz", NetworkState{
		ManagedZoneName: "skls-foo",
		DNSName:         "skls-foo.internal.",
	})
	got, ok := s.resolveNetworkState(s.ctx(), "netid-xyz")
	if !ok || got.ManagedZoneName != "skls-foo" {
		t.Fatalf("expected cache hit, got ok=%v zone=%q", ok, got.ManagedZoneName)
	}
}
