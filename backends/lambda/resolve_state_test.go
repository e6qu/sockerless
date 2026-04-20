package lambda

import (
	"testing"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// TestResolveLambdaState_CacheHit returns the cached state when
// FunctionARN is populated.
func TestResolveLambdaState_CacheHit(t *testing.T) {
	s := &Server{
		BaseServer: core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
			ID: "t", Name: "test",
		}, zerolog.Nop()),
		Lambda: core.NewStateStore[LambdaState](),
	}
	s.SetSelf(s)
	s.Lambda.Put("abc123", LambdaState{
		FunctionARN:  "arn:aws:lambda:us-east-1:000:function:skls-abc",
		FunctionName: "skls-abc",
	})
	got, ok := s.resolveLambdaState(s.ctx(), "abc123")
	if !ok || got.FunctionARN == "" || got.FunctionName == "" {
		t.Fatalf("expected cache hit, got ok=%v arn=%q name=%q", ok, got.FunctionARN, got.FunctionName)
	}
}

// TestResolveLambdaState_MissAndNoCloud returns (zero, false) when
// the cache is empty and CloudState isn't a *lambdaCloudState.
func TestResolveLambdaState_MissAndNoCloud(t *testing.T) {
	s := &Server{
		BaseServer: core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
			ID: "t", Name: "test",
		}, zerolog.Nop()),
		Lambda: core.NewStateStore[LambdaState](),
	}
	s.SetSelf(s)
	got, ok := s.resolveLambdaState(s.ctx(), "nonexistent")
	if ok {
		t.Fatalf("expected cache miss + no cloud = false, got ok=%v arn=%q", ok, got.FunctionARN)
	}
}
