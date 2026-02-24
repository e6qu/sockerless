package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

func newVolumeTestServer() *BaseServer {
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

func TestVolumeAutoCreateFromBind(t *testing.T) {
	s := newVolumeTestServer()
	binds := []string{"mydata:/data"}
	result := s.resolveBindMounts(binds, nil)

	if result == nil || result["/data"] == "" {
		t.Fatalf("expected /data to be resolved, got %v", result)
	}
	// Volume should be registered in store
	if _, ok := s.Store.Volumes.Get("mydata"); !ok {
		t.Error("expected volume 'mydata' to be registered in store")
	}
	// VolumeDirs should have the entry
	if _, ok := s.Store.VolumeDirs.Load("mydata"); !ok {
		t.Error("expected 'mydata' in VolumeDirs")
	}
	// Clean up
	os.RemoveAll(result["/data"])
}

func TestVolumeAutoCreateIdempotent(t *testing.T) {
	s := newVolumeTestServer()
	// First resolve
	result1 := s.resolveBindMounts([]string{"idempvol:/data"}, nil)
	dir1 := result1["/data"]
	// Second resolve should reuse existing
	result2 := s.resolveBindMounts([]string{"idempvol:/data"}, nil)
	dir2 := result2["/data"]

	if dir1 != dir2 {
		t.Errorf("expected same dir, got %q and %q", dir1, dir2)
	}
	os.RemoveAll(dir1)
}

func TestVolumeAutoCreateAbsolutePathSkipped(t *testing.T) {
	s := newVolumeTestServer()
	// Absolute paths should not auto-create volumes
	result := s.resolveBindMounts([]string{"/tmp/hostdir:/data"}, nil)
	if _, ok := s.Store.Volumes.Get("/tmp/hostdir"); ok {
		t.Error("absolute path should not create a volume")
	}
	// If /tmp/hostdir doesn't exist, no bind mount either
	_ = result
}

func TestVolumeAutoCreateFromMounts(t *testing.T) {
	s := newVolumeTestServer()
	mounts := []api.Mount{
		{Type: "volume", Source: "mountvol", Target: "/app"},
	}
	result := s.resolveBindMounts(nil, mounts)
	if result == nil || result["/app"] == "" {
		t.Fatalf("expected /app to be resolved, got %v", result)
	}
	if _, ok := s.Store.Volumes.Get("mountvol"); !ok {
		t.Error("expected volume 'mountvol' to be registered in store")
	}
	os.RemoveAll(result["/app"])
}

func TestVolumeAutoCreateAppearsInList(t *testing.T) {
	s := newVolumeTestServer()
	s.resolveBindMounts([]string{"listvol:/data"}, nil)

	req := httptest.NewRequest("GET", "/internal/v1/volumes", nil)
	w := httptest.NewRecorder()
	s.handleVolumeList(w, req)

	var resp api.VolumeListResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	found := false
	for _, v := range resp.Volumes {
		if v.Name == "listvol" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'listvol' in volume list")
	}
	// Clean up
	if dir, ok := s.Store.VolumeDirs.Load("listvol"); ok {
		os.RemoveAll(dir.(string))
	}
}

func TestVolumeRemoveRejectsInUse(t *testing.T) {
	s := newVolumeTestServer()

	// Create a volume
	s.Store.Volumes.Put("busyvol", api.Volume{Name: "busyvol", Driver: "local"})

	// Create a container using this volume via Mounts
	c := api.Container{
		ID:   "c1",
		Name: "/busy",
		Mounts: []api.MountPoint{
			{Type: "volume", Name: "busyvol", Destination: "/data", RW: true},
		},
		Config:          api.ContainerConfig{Labels: make(map[string]string)},
		NetworkSettings: api.NetworkSettings{Networks: make(map[string]*api.EndpointSettings)},
	}
	s.Store.Containers.Put("c1", c)

	req := httptest.NewRequest("DELETE", "/internal/v1/volumes/busyvol", nil)
	req.SetPathValue("name", "busyvol")
	w := httptest.NewRecorder()
	s.handleVolumeRemove(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "in use") {
		t.Errorf("expected 'in use' in response, got %s", w.Body.String())
	}
}

func TestVolumeRemoveForce(t *testing.T) {
	s := newVolumeTestServer()

	dir, _ := os.MkdirTemp("", "vol-forcevol-")
	s.Store.Volumes.Put("forcevol", api.Volume{Name: "forcevol", Driver: "local"})
	s.Store.VolumeDirs.Store("forcevol", dir)

	// Create a container referencing the volume
	c := api.Container{
		ID:   "c1",
		Name: "/force",
		Mounts: []api.MountPoint{
			{Type: "volume", Name: "forcevol", Destination: "/data", RW: true},
		},
		Config:          api.ContainerConfig{Labels: make(map[string]string)},
		NetworkSettings: api.NetworkSettings{Networks: make(map[string]*api.EndpointSettings)},
	}
	s.Store.Containers.Put("c1", c)

	req := httptest.NewRequest("DELETE", "/internal/v1/volumes/forcevol?force=true", nil)
	req.SetPathValue("name", "forcevol")
	w := httptest.NewRecorder()
	s.handleVolumeRemove(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	// Verify directory cleaned up
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("expected volume dir to be removed")
	}
	if _, ok := s.Store.Volumes.Get("forcevol"); ok {
		t.Error("expected volume to be deleted from store")
	}
	_ = filepath.Join("") // keep filepath import alive
}
