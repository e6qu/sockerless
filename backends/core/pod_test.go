package core

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

// --- PodRegistry unit tests ---

func TestPodRegistryCreateAndLookup(t *testing.T) {
	pr := NewPodRegistry()
	pod := pr.CreatePod("test-pod", map[string]string{"env": "test"})

	if pod.Name != "test-pod" {
		t.Errorf("expected name 'test-pod', got %q", pod.Name)
	}
	if pod.Status != "created" {
		t.Errorf("expected status 'created', got %q", pod.Status)
	}
	if len(pod.SharedNS) != 3 {
		t.Errorf("expected 3 default shared namespaces, got %d", len(pod.SharedNS))
	}

	// Lookup by ID
	found, ok := pr.GetPod(pod.ID)
	if !ok || found.Name != "test-pod" {
		t.Error("lookup by ID failed")
	}

	// Lookup by name
	found, ok = pr.GetPod("test-pod")
	if !ok || found.ID != pod.ID {
		t.Error("lookup by name failed")
	}

	// Lookup by ID prefix
	found, ok = pr.GetPod(pod.ID[:12])
	if !ok || found.ID != pod.ID {
		t.Error("lookup by ID prefix failed")
	}
}

func TestPodRegistryAddContainer(t *testing.T) {
	pr := NewPodRegistry()
	pod := pr.CreatePod("multi", nil)

	pr.AddContainer(pod.ID, "c1")
	pr.AddContainer(pod.ID, "c2")
	pr.AddContainer(pod.ID, "c3")

	if len(pod.ContainerIDs) != 3 {
		t.Fatalf("expected 3 containers, got %d", len(pod.ContainerIDs))
	}

	for _, cid := range []string{"c1", "c2", "c3"} {
		found, ok := pr.GetPodForContainer(cid)
		if !ok || found.ID != pod.ID {
			t.Errorf("GetPodForContainer(%s) failed", cid)
		}
	}
}

func TestPodRegistryDuplicateContainer(t *testing.T) {
	pr := NewPodRegistry()
	pod := pr.CreatePod("dup", nil)

	pr.AddContainer(pod.ID, "c1")
	pr.AddContainer(pod.ID, "c1") // idempotent

	if len(pod.ContainerIDs) != 1 {
		t.Errorf("expected 1 container after duplicate add, got %d", len(pod.ContainerIDs))
	}
}

func TestPodRegistryNetworkIndex(t *testing.T) {
	pr := NewPodRegistry()
	pod := pr.CreatePod("net-pod", nil)
	pr.SetNetwork(pod.ID, "my-network")

	found, ok := pr.GetPodForNetwork("my-network")
	if !ok || found.ID != pod.ID {
		t.Error("GetPodForNetwork failed")
	}

	_, ok = pr.GetPodForNetwork("other-network")
	if ok {
		t.Error("expected no pod for other-network")
	}
}

func TestPodRegistryDelete(t *testing.T) {
	pr := NewPodRegistry()
	pod := pr.CreatePod("del-pod", nil)
	pr.AddContainer(pod.ID, "c1")
	pr.SetNetwork(pod.ID, "del-net")

	ok := pr.DeletePod(pod.ID)
	if !ok {
		t.Fatal("DeletePod returned false")
	}

	// All indexes should be cleared
	if _, ok := pr.GetPod("del-pod"); ok {
		t.Error("pod still findable by name after delete")
	}
	if _, ok := pr.GetPodForContainer("c1"); ok {
		t.Error("container still indexed after pod delete")
	}
	if _, ok := pr.GetPodForNetwork("del-net"); ok {
		t.Error("network still indexed after pod delete")
	}
}

func TestPodRegistryExists(t *testing.T) {
	pr := NewPodRegistry()
	pod := pr.CreatePod("exists-pod", nil)

	if !pr.Exists("exists-pod") {
		t.Error("Exists returned false for existing pod name")
	}
	if !pr.Exists(pod.ID) {
		t.Error("Exists returned false for existing pod ID")
	}
	if pr.Exists("no-such-pod") {
		t.Error("Exists returned true for non-existent pod")
	}
}

func TestPodRegistryListPods(t *testing.T) {
	pr := NewPodRegistry()
	pr.CreatePod("pod-a", nil)
	pr.CreatePod("pod-b", nil)

	pods := pr.ListPods()
	if len(pods) != 2 {
		t.Fatalf("expected 2 pods, got %d", len(pods))
	}
}

func TestPodRegistrySetStatus(t *testing.T) {
	pr := NewPodRegistry()
	pod := pr.CreatePod("status-pod", nil)

	pr.SetStatus(pod.ID, "running")
	found, _ := pr.GetPod(pod.ID)
	if found.Status != "running" {
		t.Errorf("expected status 'running', got %q", found.Status)
	}
}

// --- Pod API handler tests ---

func newPodTestServer() *BaseServer {
	store := NewStore()
	logger := zerolog.Nop()
	s := &BaseServer{
		Store:         store,
		Logger:        logger,
		Mux:           http.NewServeMux(),
		AgentRegistry: NewAgentRegistry(),
		Desc:          BackendDescriptor{Driver: "test"},
		Registry:      NewResourceRegistry(""),
	}
	s.InitDrivers()
	s.registerRoutes(RouteOverrides{})
	s.InitDefaultNetwork()
	return s
}

func TestHandlePodCreate(t *testing.T) {
	s := newPodTestServer()

	body, _ := json.Marshal(PodCreateRequest{Name: "my-pod", Labels: map[string]string{"app": "test"}})
	req := httptest.NewRequest("POST", "/internal/v1/libpod/pods/create", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp PodCreateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ID == "" {
		t.Fatal("expected non-empty pod ID")
	}
}

func TestHandlePodCreateDuplicate(t *testing.T) {
	s := newPodTestServer()

	body, _ := json.Marshal(PodCreateRequest{Name: "dup-pod"})
	req := httptest.NewRequest("POST", "/internal/v1/libpod/pods/create", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("first create expected 201, got %d", w.Code)
	}

	// Second create with same name
	req = httptest.NewRequest("POST", "/internal/v1/libpod/pods/create", bytes.NewReader(body))
	w = httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate create expected 409, got %d", w.Code)
	}
}

func TestHandlePodList(t *testing.T) {
	s := newPodTestServer()
	s.Store.Pods.CreatePod("pod-1", nil)
	s.Store.Pods.CreatePod("pod-2", nil)

	req := httptest.NewRequest("GET", "/internal/v1/libpod/pods/json", nil)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var pods []PodListEntry
	json.Unmarshal(w.Body.Bytes(), &pods)
	if len(pods) != 2 {
		t.Fatalf("expected 2 pods, got %d", len(pods))
	}
}

func TestHandlePodInspect(t *testing.T) {
	s := newPodTestServer()
	pod := s.Store.Pods.CreatePod("inspect-pod", map[string]string{"env": "dev"})

	req := httptest.NewRequest("GET", "/internal/v1/libpod/pods/inspect-pod/json", nil)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp PodInspectResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ID != pod.ID {
		t.Errorf("expected pod ID %s, got %s", pod.ID, resp.ID)
	}
	if resp.Name != "inspect-pod" {
		t.Errorf("expected name 'inspect-pod', got %q", resp.Name)
	}
}

func TestHandlePodExists(t *testing.T) {
	s := newPodTestServer()
	s.Store.Pods.CreatePod("exists-test", nil)

	// Existing pod
	req := httptest.NewRequest("GET", "/internal/v1/libpod/pods/exists-test/exists", nil)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for existing pod, got %d", w.Code)
	}

	// Non-existent pod
	req = httptest.NewRequest("GET", "/internal/v1/libpod/pods/nope/exists", nil)
	w = httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent pod, got %d", w.Code)
	}
}

func TestHandlePodStartStop(t *testing.T) {
	s := newPodTestServer()
	s.Store.Pods.CreatePod("lifecycle-pod", nil)

	// Start
	req := httptest.NewRequest("POST", "/internal/v1/libpod/pods/lifecycle-pod/start", nil)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("start expected 200, got %d", w.Code)
	}

	pod, _ := s.Store.Pods.GetPod("lifecycle-pod")
	if pod.Status != "running" {
		t.Errorf("expected status 'running', got %q", pod.Status)
	}

	// Stop
	req = httptest.NewRequest("POST", "/internal/v1/libpod/pods/lifecycle-pod/stop", nil)
	w = httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("stop expected 200, got %d", w.Code)
	}

	pod, _ = s.Store.Pods.GetPod("lifecycle-pod")
	if pod.Status != "stopped" {
		t.Errorf("expected status 'stopped', got %q", pod.Status)
	}
}

func TestHandlePodRemove(t *testing.T) {
	s := newPodTestServer()
	s.Store.Pods.CreatePod("rm-pod", nil)

	req := httptest.NewRequest("DELETE", "/internal/v1/libpod/pods/rm-pod", nil)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	// Verify gone
	req = httptest.NewRequest("GET", "/internal/v1/libpod/pods/rm-pod/exists", nil)
	w = httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after remove, got %d", w.Code)
	}
}

// --- Container-pod association tests ---

func TestContainerCreateWithPodParam(t *testing.T) {
	s := newPodTestServer()
	pod := s.Store.Pods.CreatePod("param-pod", nil)

	// Create a container with ?pod=param-pod
	body, _ := json.Marshal(api.ContainerCreateRequest{
		ContainerConfig: &api.ContainerConfig{Image: "alpine"},
	})
	req := httptest.NewRequest("POST", "/internal/v1/containers?pod=param-pod", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp api.ContainerCreateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	// Verify container is in the pod
	foundPod, ok := s.Store.Pods.GetPodForContainer(resp.ID)
	if !ok {
		t.Fatal("container not found in any pod")
	}
	if foundPod.ID != pod.ID {
		t.Errorf("container in wrong pod: expected %s, got %s", pod.ID, foundPod.ID)
	}
}

func TestContainerCreateWithInvalidPod(t *testing.T) {
	s := newPodTestServer()

	body, _ := json.Marshal(api.ContainerCreateRequest{
		ContainerConfig: &api.ContainerConfig{Image: "alpine"},
	})
	req := httptest.NewRequest("POST", "/internal/v1/containers?pod=no-such-pod", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for invalid pod, got %d: %s", w.Code, w.Body.String())
	}
}

func TestImplicitPodViaContainerNetworkMode(t *testing.T) {
	s := newPodTestServer()

	// Create first container
	c1 := api.Container{
		ID:    "container1111111",
		Name:  "/first",
		Image: "alpine",
		State: api.ContainerState{Status: "running", Running: true},
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
		Mounts: make([]api.MountPoint, 0),
	}
	s.Store.Containers.Put(c1.ID, c1)
	s.Store.ContainerNames.Put(c1.Name, c1.ID)

	// Create second container with NetworkMode: "container:container1111111"
	body, _ := json.Marshal(api.ContainerCreateRequest{
		ContainerConfig: &api.ContainerConfig{Image: "nginx"},
		HostConfig:      &api.HostConfig{NetworkMode: "container:container1111111"},
	})
	req := httptest.NewRequest("POST", "/internal/v1/containers", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp api.ContainerCreateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	// Both containers should be in the same pod
	pod1, ok1 := s.Store.Pods.GetPodForContainer("container1111111")
	pod2, ok2 := s.Store.Pods.GetPodForContainer(resp.ID)
	if !ok1 || !ok2 {
		t.Fatal("both containers should be in a pod")
	}
	if pod1.ID != pod2.ID {
		t.Error("containers should be in the same pod")
	}
}

func TestImplicitPodViaSharedNetwork(t *testing.T) {
	s := newPodTestServer()

	// Create a user-defined network
	netID := GenerateID()
	s.Store.Networks.Put(netID, api.Network{
		Name:       "my-net",
		ID:         netID,
		Driver:     "bridge",
		Containers: make(map[string]api.EndpointResource),
		IPAM: api.IPAM{
			Driver: "default",
			Config: []api.IPAMConfig{{Subnet: "172.20.0.0/16", Gateway: "172.20.0.1"}},
		},
		Options: make(map[string]string),
		Labels:  make(map[string]string),
	})

	// Create first container on my-net
	body1, _ := json.Marshal(api.ContainerCreateRequest{
		ContainerConfig: &api.ContainerConfig{Image: "postgres"},
		HostConfig:      &api.HostConfig{NetworkMode: "my-net"},
	})
	req := httptest.NewRequest("POST", "/internal/v1/containers", bytes.NewReader(body1))
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("first container: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp1 api.ContainerCreateResponse
	json.Unmarshal(w.Body.Bytes(), &resp1)

	// Create second container on my-net — should auto-pod
	body2, _ := json.Marshal(api.ContainerCreateRequest{
		ContainerConfig: &api.ContainerConfig{Image: "alpine"},
		HostConfig:      &api.HostConfig{NetworkMode: "my-net"},
	})
	req = httptest.NewRequest("POST", "/internal/v1/containers", bytes.NewReader(body2))
	w = httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("second container: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp2 api.ContainerCreateResponse
	json.Unmarshal(w.Body.Bytes(), &resp2)

	// Both should be in same pod
	pod1, ok1 := s.Store.Pods.GetPodForContainer(resp1.ID)
	pod2, ok2 := s.Store.Pods.GetPodForContainer(resp2.ID)
	if !ok1 || !ok2 {
		t.Fatal("both containers should be in a pod")
	}
	if pod1.ID != pod2.ID {
		t.Error("containers should be in the same pod")
	}
	if pod1.NetworkName != "my-net" {
		t.Errorf("expected pod network 'my-net', got %q", pod1.NetworkName)
	}
}

func TestBuiltinNetworkSkipsImplicitPod(t *testing.T) {
	s := newPodTestServer()

	// Create container on default bridge — should NOT create a pod
	body, _ := json.Marshal(api.ContainerCreateRequest{
		ContainerConfig: &api.ContainerConfig{Image: "alpine"},
		HostConfig:      &api.HostConfig{NetworkMode: "bridge"},
	})
	req := httptest.NewRequest("POST", "/internal/v1/containers", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	var resp api.ContainerCreateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	_, inPod := s.Store.Pods.GetPodForContainer(resp.ID)
	if inPod {
		t.Error("container on bridge network should not be in a pod")
	}
}

func TestNetworkConnectJoinsPod(t *testing.T) {
	s := newPodTestServer()

	// Create a user-defined network
	netID := GenerateID()
	s.Store.Networks.Put(netID, api.Network{
		Name:       "join-net",
		ID:         netID,
		Driver:     "bridge",
		Containers: make(map[string]api.EndpointResource),
		IPAM: api.IPAM{
			Driver: "default",
			Config: []api.IPAMConfig{{Subnet: "172.21.0.0/16", Gateway: "172.21.0.1"}},
		},
		Options: make(map[string]string),
		Labels:  make(map[string]string),
	})

	// Create a pod for this network
	pod := s.Store.Pods.CreatePod("join-pod", nil)
	s.Store.Pods.SetNetwork(pod.ID, "join-net")

	// Create a container on bridge (not in any pod)
	c := api.Container{
		ID:    "joincontainer1",
		Name:  "/joiner",
		Image: "alpine",
		State: api.ContainerState{Status: "created"},
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
		Mounts: make([]api.MountPoint, 0),
	}
	s.Store.Containers.Put(c.ID, c)
	s.Store.ContainerNames.Put(c.Name, c.ID)

	// Connect the container to the network via API
	connectBody, _ := json.Marshal(api.NetworkConnectRequest{Container: "joincontainer1"})
	req := httptest.NewRequest("POST", "/internal/v1/networks/"+netID+"/connect", bytes.NewReader(connectBody))
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Container should now be in the pod
	foundPod, ok := s.Store.Pods.GetPodForContainer("joincontainer1")
	if !ok {
		t.Fatal("container should be in pod after network connect")
	}
	if foundPod.ID != pod.ID {
		t.Error("container in wrong pod after network connect")
	}
}
