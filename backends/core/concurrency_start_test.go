package core

import (
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

func newStartTestServer() *BaseServer {
	s := &BaseServer{
		Store:          NewStore(),
		Logger:         zerolog.Nop(),
		Mux:            http.NewServeMux(),
		EventBus:       NewEventBus(),
		PendingCreates: NewStateStore[api.Container](),
	}
	s.InitDrivers()
	s.self = s
	return s
}

// TestContainerStart_ConcurrentStartsSerialize verifies the per-container start
// lock: N concurrent ContainerStart(id) install exactly one wait channel (no
// orphaned waiter) and leave the container running, with the losers returning
// NotModified. Run with -race to catch a regression.
func TestContainerStart_ConcurrentStartsSerialize(t *testing.T) {
	s := newStartTestServer()
	c := api.Container{
		ID:              "race1",
		Name:            "/race1",
		Config:          api.ContainerConfig{Labels: map[string]string{}},
		State:           api.ContainerState{Status: "created", Running: false},
		NetworkSettings: api.NetworkSettings{Networks: map[string]*api.EndpointSettings{}},
		Mounts:          []api.MountPoint{},
	}
	s.Store.Containers.Put("race1", c)

	const N = 24
	var wg sync.WaitGroup
	errs := make([]error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); errs[i] = s.ContainerStart("race1") }(i)
	}
	wg.Wait()

	waitChs := 0
	s.Store.WaitChs.Range(func(k, _ any) bool {
		if k == "race1" {
			waitChs++
		}
		return true
	})
	if waitChs != 1 {
		t.Fatalf("want exactly 1 wait channel for race1, got %d", waitChs)
	}
	got, _ := s.Store.Containers.Get("race1")
	if !got.State.Running {
		t.Fatalf("container should be running after concurrent starts")
	}
	notMod := 0
	for _, e := range errs {
		if e == nil {
			continue
		}
		var nm *api.NotModifiedError
		if errors.As(e, &nm) {
			notMod++
		} else {
			t.Fatalf("unexpected ContainerStart error: %v", e)
		}
	}
	if notMod != N-1 {
		t.Fatalf("want %d NotModified (one winner), got %d", N-1, notMod)
	}
}
