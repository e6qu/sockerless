package ecs

import (
	"testing"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// TestResolveTaskState_CacheHit returns the cached state without
// consulting cloud when TaskARN is populated.
func TestResolveTaskState_CacheHit(t *testing.T) {
	s := &Server{
		BaseServer: core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
			ID: "t", Name: "test",
		}, zerolog.Nop()),
		ECS:          core.NewStateStore[ECSState](),
		NetworkState: core.NewStateStore[NetworkState](),
	}
	s.SetSelf(s)
	// No CloudState → resolveTaskState must rely on the cache.
	s.ECS.Put("abc123", ECSState{
		TaskARN:    "arn:aws:ecs:eu-west-1:000:task/sim/taskid",
		ClusterARN: "arn:aws:ecs:eu-west-1:000:cluster/sim",
	})
	got, ok := s.resolveTaskState(s.ctx(), "abc123")
	if !ok || got.TaskARN == "" {
		t.Fatalf("expected cache hit, got ok=%v arn=%q", ok, got.TaskARN)
	}
	if got.ClusterARN == "" {
		t.Fatalf("expected ClusterARN from cache")
	}
}

// TestResolveTaskState_MissAndNoCloud returns (zero, false) when the
// cache is empty and no CloudState is wired — the helper must NOT
// panic or hang on a nil cloud client.
func TestResolveTaskState_MissAndNoCloud(t *testing.T) {
	s := &Server{
		BaseServer: core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
			ID: "t", Name: "test",
		}, zerolog.Nop()),
		ECS:          core.NewStateStore[ECSState](),
		NetworkState: core.NewStateStore[NetworkState](),
	}
	s.SetSelf(s)
	// CloudState not assigned → type assertion in resolveTaskState fails
	// and the helper short-circuits to (zero, false).
	got, ok := s.resolveTaskState(s.ctx(), "nonexistent")
	if ok {
		t.Fatalf("expected cache miss + no cloud = false, got ok=%v arn=%q", ok, got.TaskARN)
	}
}

// TestResolveNetworkState_CacheHit returns the cached NetworkState
// when SecurityGroupID is populated — no EC2 API calls issued.
func TestResolveNetworkState_CacheHit(t *testing.T) {
	s := &Server{
		BaseServer: core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
			ID: "t", Name: "test",
		}, zerolog.Nop()),
		ECS:          core.NewStateStore[ECSState](),
		NetworkState: core.NewStateStore[NetworkState](),
	}
	s.SetSelf(s)
	s.NetworkState.Put("netid-xyz", NetworkState{
		SecurityGroupID: "sg-abc",
		NamespaceID:     "ns-def",
	})
	got, ok := s.resolveNetworkState(s.ctx(), "netid-xyz")
	if !ok || got.SecurityGroupID != "sg-abc" || got.NamespaceID != "ns-def" {
		t.Fatalf("expected cache hit, got ok=%v sg=%q ns=%q", ok, got.SecurityGroupID, got.NamespaceID)
	}
}
