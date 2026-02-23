package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

func newMountsTestServer() *BaseServer {
	store := NewStore()
	s := &BaseServer{
		Store:    store,
		Logger:   zerolog.Nop(),
		Mux:      http.NewServeMux(),
		EventBus: NewEventBus(),
	}
	s.InitDrivers()
	return s
}

func TestBuildMountsFromBinds(t *testing.T) {
	hc := api.HostConfig{
		Binds: []string{"/host/path:/container/path", "myvolume:/data:ro"},
	}
	mounts := buildMounts(hc)
	if len(mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(mounts))
	}
	// First: absolute bind
	if mounts[0].Type != "bind" || mounts[0].Source != "/host/path" || mounts[0].Destination != "/container/path" {
		t.Errorf("mount[0] = %+v, want bind /host/path -> /container/path", mounts[0])
	}
	if !mounts[0].RW {
		t.Error("mount[0] should be RW")
	}
	// Second: named volume, read-only
	if mounts[1].Type != "volume" || mounts[1].Name != "myvolume" || mounts[1].Destination != "/data" {
		t.Errorf("mount[1] = %+v, want volume myvolume -> /data", mounts[1])
	}
	if mounts[1].RW {
		t.Error("mount[1] should be RO")
	}
}

func TestBuildMountsFromNamedVolume(t *testing.T) {
	hc := api.HostConfig{
		Binds: []string{"dbdata:/var/lib/db"},
	}
	mounts := buildMounts(hc)
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].Type != "volume" || mounts[0].Name != "dbdata" {
		t.Errorf("expected volume mount, got %+v", mounts[0])
	}
}

func TestBuildMountsFromHostConfigMounts(t *testing.T) {
	hc := api.HostConfig{
		Mounts: []api.Mount{
			{Type: "volume", Source: "appdata", Target: "/app"},
		},
	}
	mounts := buildMounts(hc)
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].Type != "volume" || mounts[0].Source != "appdata" || mounts[0].Destination != "/app" {
		t.Errorf("mount = %+v, want volume appdata -> /app", mounts[0])
	}
}

func TestBuildMountsFromTmpfs(t *testing.T) {
	hc := api.HostConfig{
		Tmpfs: map[string]string{"/tmp": "rw,noexec"},
	}
	mounts := buildMounts(hc)
	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].Type != "tmpfs" || mounts[0].Destination != "/tmp" {
		t.Errorf("mount = %+v, want tmpfs -> /tmp", mounts[0])
	}
}

func TestBuildMountsEmpty(t *testing.T) {
	mounts := buildMounts(api.HostConfig{})
	if len(mounts) != 0 {
		t.Errorf("expected empty mounts, got %d", len(mounts))
	}
}

func TestContainerRemoveCleansNetworks(t *testing.T) {
	s := newMountsTestServer()

	// Create a network
	netID := "net123"
	s.Store.Networks.Put(netID, api.Network{
		ID:   netID,
		Name: "testnet",
		Containers: map[string]api.EndpointResource{
			"c1": {Name: "test", EndpointID: "ep1"},
		},
	})

	// Create a container on the network
	c := api.Container{
		ID:      "c1",
		Name:    "/test",
		Created: time.Now().UTC().Format(time.RFC3339Nano),
		Config:  api.ContainerConfig{Labels: make(map[string]string)},
		State:   api.ContainerState{Status: "exited"},
		HostConfig: api.HostConfig{},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"testnet": {NetworkID: netID, EndpointID: "ep1"},
			},
		},
		Mounts: []api.MountPoint{},
	}
	s.Store.Containers.Put("c1", c)
	s.Store.ContainerNames.Put("/test", "c1")

	req := httptest.NewRequest("DELETE", "/internal/v1/containers/c1", nil)
	req.SetPathValue("id", "c1")
	w := httptest.NewRecorder()
	s.handleContainerRemove(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	// Network should no longer have the container
	net, _ := s.Store.Networks.Get(netID)
	if _, found := net.Containers["c1"]; found {
		t.Error("expected container to be removed from network")
	}
}

func TestContainerRemoveCleansMultipleNetworks(t *testing.T) {
	s := newMountsTestServer()

	// Create two networks
	s.Store.Networks.Put("n1", api.Network{
		ID: "n1", Name: "net1",
		Containers: map[string]api.EndpointResource{"c1": {Name: "test"}},
	})
	s.Store.Networks.Put("n2", api.Network{
		ID: "n2", Name: "net2",
		Containers: map[string]api.EndpointResource{"c1": {Name: "test"}},
	})

	c := api.Container{
		ID: "c1", Name: "/test",
		Created: time.Now().UTC().Format(time.RFC3339Nano),
		Config:  api.ContainerConfig{Labels: make(map[string]string)},
		State:   api.ContainerState{Status: "exited"},
		HostConfig: api.HostConfig{},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"net1": {NetworkID: "n1"},
				"net2": {NetworkID: "n2"},
			},
		},
		Mounts: []api.MountPoint{},
	}
	s.Store.Containers.Put("c1", c)
	s.Store.ContainerNames.Put("/test", "c1")

	req := httptest.NewRequest("DELETE", "/internal/v1/containers/c1", nil)
	req.SetPathValue("id", "c1")
	w := httptest.NewRecorder()
	s.handleContainerRemove(w, req)

	n1, _ := s.Store.Networks.Get("n1")
	n2, _ := s.Store.Networks.Get("n2")
	if _, found := n1.Containers["c1"]; found {
		t.Error("expected container removed from net1")
	}
	if _, found := n2.Containers["c1"]; found {
		t.Error("expected container removed from net2")
	}
}

func TestContainerRemoveFreesName(t *testing.T) {
	s := newMountsTestServer()

	c := api.Container{
		ID: "c1", Name: "/uniquename",
		Created:         time.Now().UTC().Format(time.RFC3339Nano),
		Config:          api.ContainerConfig{Labels: make(map[string]string)},
		State:           api.ContainerState{Status: "exited"},
		HostConfig:      api.HostConfig{},
		NetworkSettings: api.NetworkSettings{Networks: make(map[string]*api.EndpointSettings)},
		Mounts:          []api.MountPoint{},
	}
	s.Store.Containers.Put("c1", c)
	s.Store.ContainerNames.Put("/uniquename", "c1")

	req := httptest.NewRequest("DELETE", "/internal/v1/containers/c1", nil)
	req.SetPathValue("id", "c1")
	w := httptest.NewRecorder()
	s.handleContainerRemove(w, req)

	// Name should be freed
	if _, ok := s.Store.ContainerNames.Get("/uniquename"); ok {
		t.Error("expected container name to be freed after remove")
	}

	// Verify inspect also confirms by checking body from list
	req2 := httptest.NewRequest("GET", "/internal/v1/containers?all=1", nil)
	w2 := httptest.NewRecorder()
	s.handleContainerList(w2, req2)

	var list []*api.ContainerSummary
	json.Unmarshal(w2.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Errorf("expected empty container list, got %d", len(list))
	}
}
