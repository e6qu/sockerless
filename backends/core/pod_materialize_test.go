package core

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sockerless/api"
)

func TestPodDeferredStartNotInPod(t *testing.T) {
	s := newPodTestServer()

	// Create a container not in any pod
	s.Store.Containers.Put("c1", api.Container{
		ID:    "c1",
		Name:  "/test",
		Image: "alpine",
	})

	shouldDefer, podContainers := s.PodDeferredStart("c1")
	if shouldDefer {
		t.Error("expected shouldDefer=false for container not in pod")
	}
	if podContainers != nil {
		t.Error("expected podContainers=nil for container not in pod")
	}
}

func TestPodDeferredStartSinglePodContainer(t *testing.T) {
	s := newPodTestServer()

	// Create a pod with only one container
	pod := s.Store.Pods.CreatePod("solo-pod", nil)
	s.Store.Pods.AddContainer(pod.ID, "c1")
	s.Store.Containers.Put("c1", api.Container{
		ID:    "c1",
		Name:  "/test",
		Image: "alpine",
	})

	shouldDefer, podContainers := s.PodDeferredStart("c1")
	if shouldDefer {
		t.Error("expected shouldDefer=false for single-container pod")
	}
	if podContainers != nil {
		t.Error("expected podContainers=nil for single-container pod")
	}
}

func TestPodDeferredStartTwoContainerFirst(t *testing.T) {
	s := newPodTestServer()

	// Create a pod with two containers
	pod := s.Store.Pods.CreatePod("duo-pod", nil)
	s.Store.Pods.AddContainer(pod.ID, "c1")
	s.Store.Pods.AddContainer(pod.ID, "c2")
	s.Store.Containers.Put("c1", api.Container{ID: "c1", Name: "/svc", Image: "postgres"})
	s.Store.Containers.Put("c2", api.Container{ID: "c2", Name: "/main", Image: "app"})

	// Start first container — should defer
	shouldDefer, podContainers := s.PodDeferredStart("c1")
	if !shouldDefer {
		t.Error("expected shouldDefer=true for first container in 2-container pod")
	}
	if podContainers != nil {
		t.Error("expected podContainers=nil when deferring")
	}
}

func TestPodDeferredStartTwoContainerSecond(t *testing.T) {
	s := newPodTestServer()

	pod := s.Store.Pods.CreatePod("duo-pod", nil)
	s.Store.Pods.AddContainer(pod.ID, "c1")
	s.Store.Pods.AddContainer(pod.ID, "c2")
	s.Store.Containers.Put("c1", api.Container{ID: "c1", Name: "/svc", Image: "postgres"})
	s.Store.Containers.Put("c2", api.Container{ID: "c2", Name: "/main", Image: "app"})

	// Start first container — defers
	shouldDefer, _ := s.PodDeferredStart("c1")
	if !shouldDefer {
		t.Fatal("expected shouldDefer=true for first container")
	}

	// Start second container — should materialize
	shouldDefer, podContainers := s.PodDeferredStart("c2")
	if shouldDefer {
		t.Error("expected shouldDefer=false for last container in pod")
	}
	if len(podContainers) != 2 {
		t.Fatalf("expected 2 pod containers, got %d", len(podContainers))
	}

	// Verify we got both containers
	ids := map[string]bool{}
	for _, c := range podContainers {
		ids[c.ID] = true
	}
	if !ids["c1"] || !ids["c2"] {
		t.Error("expected both c1 and c2 in pod containers")
	}
}

func TestPodDeferredStartThreeContainers(t *testing.T) {
	s := newPodTestServer()

	pod := s.Store.Pods.CreatePod("trio-pod", nil)
	s.Store.Pods.AddContainer(pod.ID, "c1")
	s.Store.Pods.AddContainer(pod.ID, "c2")
	s.Store.Pods.AddContainer(pod.ID, "c3")
	s.Store.Containers.Put("c1", api.Container{ID: "c1", Name: "/a", Image: "img1"})
	s.Store.Containers.Put("c2", api.Container{ID: "c2", Name: "/b", Image: "img2"})
	s.Store.Containers.Put("c3", api.Container{ID: "c3", Name: "/c", Image: "img3"})

	// Start c1 — defer
	shouldDefer, _ := s.PodDeferredStart("c1")
	if !shouldDefer {
		t.Error("expected defer after 1 of 3")
	}

	// Start c2 — still defer
	shouldDefer, _ = s.PodDeferredStart("c2")
	if !shouldDefer {
		t.Error("expected defer after 2 of 3")
	}

	// Start c3 — materialize
	shouldDefer, podContainers := s.PodDeferredStart("c3")
	if shouldDefer {
		t.Error("expected no defer after 3 of 3")
	}
	if len(podContainers) != 3 {
		t.Fatalf("expected 3 pod containers, got %d", len(podContainers))
	}
}

func TestMarkStartedIdempotent(t *testing.T) {
	pr := NewPodRegistry()
	pod := pr.CreatePod("idem-pod", nil)
	pr.AddContainer(pod.ID, "c1")
	pr.AddContainer(pod.ID, "c2")

	// Mark c1 started twice
	shouldDefer1, _ := pr.MarkStarted(pod.ID, "c1")
	shouldDefer2, _ := pr.MarkStarted(pod.ID, "c1")

	if !shouldDefer1 {
		t.Error("first MarkStarted should defer (1 of 2)")
	}
	if !shouldDefer2 {
		t.Error("idempotent MarkStarted should still defer (1 of 2)")
	}

	// c1 should only appear once in StartedIDs
	if len(pod.StartedIDs) != 1 {
		t.Errorf("expected 1 started ID after idempotent call, got %d", len(pod.StartedIDs))
	}

	// Now mark c2 — should not defer
	shouldDefer3, ids := pr.MarkStarted(pod.ID, "c2")
	if shouldDefer3 {
		t.Error("MarkStarted c2 should not defer (2 of 2)")
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 container IDs, got %d", len(ids))
	}
}

func TestMemoryBackendRejectsMultiContainerPod(t *testing.T) {
	s := newPodTestServer()

	// Create a pod with two containers
	pod := s.Store.Pods.CreatePod("reject-pod", nil)
	s.Store.Pods.AddContainer(pod.ID, "c1aaaaaaaaaaaa")
	s.Store.Pods.AddContainer(pod.ID, "c2aaaaaaaaaaaa")

	c := api.Container{
		ID:    "c1aaaaaaaaaaaa",
		Name:  "/svc",
		Image: "postgres",
		State: api.ContainerState{Status: "created"},
		Config: api.ContainerConfig{
			Image: "postgres",
		},
		HostConfig: api.HostConfig{NetworkMode: "bridge"},
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
		Mounts: make([]api.MountPoint, 0),
	}
	s.Store.Containers.Put("c1aaaaaaaaaaaa", c)
	s.Store.ContainerNames.Put("/svc", "c1aaaaaaaaaaaa")

	// Try to start it — should be rejected
	req := httptest.NewRequest("POST", "/internal/v1/containers/c1aaaaaaaaaaaa/start", nil)
	req.SetPathValue("id", "c1aaaaaaaaaaaa")
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var errResp map[string]string
	json.Unmarshal(w.Body.Bytes(), &errResp)
	if errResp["message"] == "" {
		t.Error("expected error message about multi-container pods")
	}
}

func TestSingleContainerPodStillWorks(t *testing.T) {
	s := newPodTestServer()

	// Create a pod with one container
	pod := s.Store.Pods.CreatePod("solo-pod", nil)

	// Create container via API
	createBody, _ := json.Marshal(api.ContainerCreateRequest{
		ContainerConfig: &api.ContainerConfig{
			Image: "alpine",
			Cmd:   []string{"echo", "hello"},
		},
	})
	req := httptest.NewRequest("POST", "/internal/v1/containers?pod=solo-pod", bytes.NewReader(createBody))
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var createResp api.ContainerCreateResponse
	json.Unmarshal(w.Body.Bytes(), &createResp)
	containerID := createResp.ID

	// Verify it's in the pod
	if len(pod.ContainerIDs) != 1 || pod.ContainerIDs[0] != containerID {
		t.Fatal("container should be in the pod")
	}

	// Start it — should succeed (single-container pod, no rejection)
	req = httptest.NewRequest("POST", "/internal/v1/containers/"+containerID+"/start", nil)
	req.SetPathValue("id", containerID)
	w = httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("start expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWaitForServiceHealthAllHealthy(t *testing.T) {
	s := newPodTestServer()

	pod := s.Store.Pods.CreatePod("health-pod", nil)
	s.Store.Pods.AddContainer(pod.ID, "svc1")
	s.Store.Pods.AddContainer(pod.ID, "main1")

	// svc1 has a healthcheck, main1 does not
	s.Store.Containers.Put("svc1", api.Container{
		ID:    "svc1",
		Name:  "/svc",
		Image: "redis",
		Config: api.ContainerConfig{
			Healthcheck: &api.HealthcheckConfig{
				Test: []string{"CMD", "redis-cli", "ping"},
			},
		},
		State: api.ContainerState{
			Health: &api.HealthState{Status: "healthy"},
		},
	})
	s.Store.Containers.Put("main1", api.Container{
		ID:    "main1",
		Name:  "/main",
		Image: "alpine",
	})

	err := s.WaitForServiceHealth(pod.ID, 2*time.Second)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestWaitForServiceHealthTimeout(t *testing.T) {
	s := newPodTestServer()

	pod := s.Store.Pods.CreatePod("timeout-pod", nil)
	s.Store.Pods.AddContainer(pod.ID, "svc1")

	// svc1 has a healthcheck but stays "starting" (never becomes healthy)
	s.Store.Containers.Put("svc1", api.Container{
		ID:    "svc1",
		Name:  "/svc",
		Image: "redis",
		Config: api.ContainerConfig{
			Healthcheck: &api.HealthcheckConfig{
				Test: []string{"CMD", "redis-cli", "ping"},
			},
		},
		State: api.ContainerState{
			Health: &api.HealthState{Status: "starting"},
		},
	})

	err := s.WaitForServiceHealth(pod.ID, 1*time.Second)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestWaitForServiceHealthNoHealthcheck(t *testing.T) {
	s := newPodTestServer()

	pod := s.Store.Pods.CreatePod("no-hc-pod", nil)
	s.Store.Pods.AddContainer(pod.ID, "svc1")
	s.Store.Pods.AddContainer(pod.ID, "main1")

	// Neither container has a healthcheck
	s.Store.Containers.Put("svc1", api.Container{
		ID:    "svc1",
		Name:  "/svc",
		Image: "redis",
	})
	s.Store.Containers.Put("main1", api.Container{
		ID:    "main1",
		Name:  "/main",
		Image: "alpine",
	})

	// Should return immediately (no containers to wait for)
	err := s.WaitForServiceHealth(pod.ID, 2*time.Second)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}
