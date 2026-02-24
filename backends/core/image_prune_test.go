package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sockerless/api"
)

func TestImagePruneUnreferenced(t *testing.T) {
	s := newTestServer(&testExecDriver{exitCode: 0})

	// Add an image that no container references
	img := api.Image{
		ID:       "sha256:deadbeef",
		RepoTags: []string{"unused:latest"},
		Created:  time.Now().UTC().Format(time.RFC3339Nano),
		Size:     1024,
	}
	s.Store.Images.Put(img.ID, img)
	s.Store.Images.Put("unused:latest", img)

	req := httptest.NewRequest("POST", "/images/prune", nil)
	w := httptest.NewRecorder()
	s.handleImagePrune(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp api.ImagePruneResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.ImagesDeleted) == 0 {
		t.Fatal("expected at least one deleted image")
	}
	if resp.SpaceReclaimed != 1024 {
		t.Errorf("space reclaimed = %d, want 1024", resp.SpaceReclaimed)
	}

	// Verify image is gone
	if _, ok := s.Store.Images.Get("sha256:deadbeef"); ok {
		t.Error("expected image to be deleted from store")
	}
}

func TestImagePruneSkipsInUse(t *testing.T) {
	s := newTestServer(&testExecDriver{exitCode: 0})

	// Add image
	img := api.Image{
		ID:       "sha256:inuse123",
		RepoTags: []string{"myapp:latest"},
		Created:  time.Now().UTC().Format(time.RFC3339Nano),
		Size:     2048,
	}
	s.Store.Images.Put(img.ID, img)
	s.Store.Images.Put("myapp:latest", img)

	// Create a container referencing this image
	c := api.Container{
		ID:   "container1",
		Name: "/test-container",
		Config: api.ContainerConfig{
			Image:  "myapp:latest",
			Labels: make(map[string]string),
		},
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
	}
	s.Store.Containers.Put(c.ID, c)

	req := httptest.NewRequest("POST", "/images/prune", nil)
	w := httptest.NewRecorder()
	s.handleImagePrune(w, req)

	var resp api.ImagePruneResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.ImagesDeleted) != 0 {
		t.Errorf("expected 0 deleted images (in use), got %d", len(resp.ImagesDeleted))
	}

	// Verify image still exists
	if _, ok := s.Store.Images.Get("sha256:inuse123"); !ok {
		t.Error("expected in-use image to remain in store")
	}
}

func TestImagePruneEmpty(t *testing.T) {
	s := newTestServer(&testExecDriver{exitCode: 0})

	req := httptest.NewRequest("POST", "/images/prune", nil)
	w := httptest.NewRecorder()
	s.handleImagePrune(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp api.ImagePruneResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.ImagesDeleted) != 0 {
		t.Errorf("expected 0 deleted images, got %d", len(resp.ImagesDeleted))
	}
	if resp.SpaceReclaimed != 0 {
		t.Errorf("space reclaimed = %d, want 0", resp.SpaceReclaimed)
	}
}

func TestImagePruneDanglingFilter(t *testing.T) {
	s := newTestServer(&testExecDriver{exitCode: 0})

	// Add a tagged image (should NOT be pruned with dangling=true)
	tagged := api.Image{
		ID:       "sha256:tagged123",
		RepoTags: []string{"myapp:v1"},
		Created:  time.Now().UTC().Format(time.RFC3339Nano),
		Size:     512,
	}
	s.Store.Images.Put(tagged.ID, tagged)
	s.Store.Images.Put("myapp:v1", tagged)

	// Add a dangling image (no real tags)
	dangling := api.Image{
		ID:       "sha256:dangling456",
		RepoTags: []string{"<none>:<none>"},
		Created:  time.Now().UTC().Format(time.RFC3339Nano),
		Size:     256,
	}
	s.Store.Images.Put(dangling.ID, dangling)

	req := httptest.NewRequest("POST", "/images/prune?filters=%7B%22dangling%22%3A%5B%22true%22%5D%7D", nil)
	w := httptest.NewRecorder()
	s.handleImagePrune(w, req)

	var resp api.ImagePruneResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Only the dangling image should be pruned
	if resp.SpaceReclaimed != 256 {
		t.Errorf("space reclaimed = %d, want 256 (only dangling)", resp.SpaceReclaimed)
	}

	// Tagged image should still exist
	if _, ok := s.Store.Images.Get("sha256:tagged123"); !ok {
		t.Error("expected tagged image to remain in store")
	}

	// Dangling image should be gone
	if _, ok := s.Store.Images.Get("sha256:dangling456"); ok {
		t.Error("expected dangling image to be deleted")
	}
}
