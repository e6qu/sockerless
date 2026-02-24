package core

import (
	"fmt"
	"sync"
	"testing"

	"github.com/sockerless/api"
)

func TestPruneIfBasic(t *testing.T) {
	s := NewStateStore[string]()
	s.Put("a", "keep")
	s.Put("b", "remove")
	s.Put("c", "remove")

	pruned := s.PruneIf(func(_ string, v string) bool {
		return v == "remove"
	})
	if len(pruned) != 2 {
		t.Errorf("expected 2 pruned, got %d", len(pruned))
	}
	if s.Len() != 1 {
		t.Errorf("expected 1 remaining, got %d", s.Len())
	}
	if v, ok := s.Get("a"); !ok || v != "keep" {
		t.Errorf("expected 'keep' for key 'a', got %q", v)
	}
}

func TestPruneIfEmpty(t *testing.T) {
	s := NewStateStore[string]()
	pruned := s.PruneIf(func(_ string, _ string) bool { return true })
	if len(pruned) != 0 {
		t.Errorf("expected 0 pruned from empty store, got %d", len(pruned))
	}
}

func TestPruneIfNoMatch(t *testing.T) {
	s := NewStateStore[string]()
	s.Put("a", "keep")
	s.Put("b", "keep")

	pruned := s.PruneIf(func(_ string, v string) bool { return v == "nope" })
	if len(pruned) != 0 {
		t.Errorf("expected 0 pruned, got %d", len(pruned))
	}
	if s.Len() != 2 {
		t.Errorf("expected 2 remaining, got %d", s.Len())
	}
}

func TestConcurrentContainerCreateAndPrune(t *testing.T) {
	store := NewStore()
	var wg sync.WaitGroup

	// Prune goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			store.Containers.PruneIf(func(_ string, c api.Container) bool {
				return c.State.Status == "exited"
			})
		}
	}()

	// Create goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			id := fmt.Sprintf("c%d", i)
			store.Containers.Put(id, api.Container{
				ID:    id,
				State: api.ContainerState{Status: "running"},
			})
		}
	}()

	wg.Wait()
	// All running containers should survive prune
	for _, c := range store.Containers.List() {
		if c.State.Status != "running" {
			t.Errorf("container %s has unexpected status %q", c.ID, c.State.Status)
		}
	}
}

func TestConcurrentVolumePrune(t *testing.T) {
	store := NewStore()
	var wg sync.WaitGroup

	// Pre-populate
	for i := 0; i < 50; i++ {
		store.Volumes.Put(fmt.Sprintf("v%d", i), api.Volume{Name: fmt.Sprintf("v%d", i)})
	}

	// Concurrent prune + create
	wg.Add(2)
	go func() {
		defer wg.Done()
		store.Volumes.PruneIf(func(k string, _ api.Volume) bool {
			return k >= "v2" // prune v2-v9, v20-v29
		})
	}()
	go func() {
		defer wg.Done()
		for i := 50; i < 100; i++ {
			store.Volumes.Put(fmt.Sprintf("v%d", i), api.Volume{Name: fmt.Sprintf("v%d", i)})
		}
	}()

	wg.Wait()
	// No panic = success
}

func TestConcurrentNetworkPrune(t *testing.T) {
	store := NewStore()
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		store.Networks.Put(fmt.Sprintf("n%d", i), api.Network{
			ID:         fmt.Sprintf("n%d", i),
			Name:       fmt.Sprintf("net%d", i),
			Containers: make(map[string]api.EndpointResource),
		})
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		store.Networks.PruneIf(func(_ string, n api.Network) bool {
			return len(n.Containers) == 0
		})
	}()
	go func() {
		defer wg.Done()
		for i := 20; i < 40; i++ {
			store.Networks.Put(fmt.Sprintf("n%d", i), api.Network{
				ID:         fmt.Sprintf("n%d", i),
				Name:       fmt.Sprintf("net%d", i),
				Containers: make(map[string]api.EndpointResource),
			})
		}
	}()

	wg.Wait()
}

func TestConcurrentContainerCreate(t *testing.T) {
	store := NewStore()
	var wg sync.WaitGroup

	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(group int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				id := fmt.Sprintf("c%d_%d", group, i)
				store.Containers.Put(id, api.Container{
					ID:    id,
					State: api.ContainerState{Status: "running"},
				})
			}
		}(g)
	}

	wg.Wait()
	if store.Containers.Len() != 200 {
		t.Errorf("expected 200 containers, got %d", store.Containers.Len())
	}
}

func TestConcurrentStateUpdate(t *testing.T) {
	store := NewStore()
	store.Containers.Put("c1", api.Container{
		ID:    "c1",
		State: api.ContainerState{Status: "running"},
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store.Containers.Update("c1", func(c *api.Container) {
				c.RestartCount = i
			})
		}(i)
	}

	wg.Wait()
	c, _ := store.Containers.Get("c1")
	// Final value is nondeterministic, but should be one of 0-99
	if c.RestartCount < 0 || c.RestartCount > 99 {
		t.Errorf("unexpected restart count: %d", c.RestartCount)
	}
}
