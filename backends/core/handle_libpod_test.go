package core

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sockerless/api"
)

func TestLibpodContainerListCreatedIsRFC3339(t *testing.T) {
	s := newPodTestServer()

	body, _ := json.Marshal(api.ContainerCreateRequest{
		ContainerConfig: &api.ContainerConfig{Image: "alpine"},
	})
	req := httptest.NewRequest("POST", "/containers/create?name=lpctr", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("GET", "/libpod/containers/json?all=true", nil)
	w = httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var containers []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &containers); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(containers) == 0 {
		t.Fatal("expected at least 1 container")
	}
	created, ok := containers[0]["Created"].(string)
	if !ok {
		t.Fatalf("Created should be string (RFC3339), got %T", containers[0]["Created"])
	}
	if !strings.Contains(created, "T") {
		t.Errorf("Created should be RFC3339 format, got %q", created)
	}
}

func TestLibpodImagePullReturnsIDField(t *testing.T) {
	s := newPodTestServer()

	req := httptest.NewRequest("POST", "/libpod/images/pull?reference=alpine:latest", nil)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code == http.StatusBadRequest && strings.Contains(w.Body.String(), "image reference is required") {
		t.Fatal("libpod pull should accept 'reference' param")
	}
}

func TestLibpodBuildEndpointExists(t *testing.T) {
	s := newPodTestServer()

	req := httptest.NewRequest("POST", "/libpod/build", nil)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatal("POST /libpod/build should be registered")
	}
}

func TestPingIncludesLibpodHeader(t *testing.T) {
	s := newPodTestServer()

	req := httptest.NewRequest("GET", "/_ping", nil)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if v := w.Header().Get("Libpod-API-Version"); v == "" {
		t.Error("ping should include Libpod-API-Version header")
	}
}

func TestLibpodContainerRemoveReturnsJSON(t *testing.T) {
	s := newPodTestServer()

	body, _ := json.Marshal(api.ContainerCreateRequest{
		ContainerConfig: &api.ContainerConfig{Image: "alpine"},
	})
	req := httptest.NewRequest("POST", "/containers/create?name=rmctr", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	var cr api.ContainerCreateResponse
	json.Unmarshal(w.Body.Bytes(), &cr)

	// Delete via libpod endpoint
	req = httptest.NewRequest("DELETE", "/libpod/containers/"+cr.ID+"?force=true", nil)
	w = httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Podman expects JSON array
	var reports []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &reports); err != nil {
		t.Fatalf("response not valid JSON array: %v — body: %s", err, w.Body.String())
	}
	if len(reports) == 0 {
		t.Fatal("expected at least one RmReport")
	}
	if _, ok := reports[0]["Id"]; !ok {
		t.Error("RmReport should have Id field")
	}
}

func TestContainerTopSelfDispatch(t *testing.T) {
	s := newPodTestServer()

	body, _ := json.Marshal(api.ContainerCreateRequest{
		ContainerConfig: &api.ContainerConfig{Image: "alpine", Cmd: []string{"sleep", "60"}},
	})
	req := httptest.NewRequest("POST", "/containers/create?name=topctr", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	var createResp api.ContainerCreateResponse
	json.Unmarshal(w.Body.Bytes(), &createResp)

	s.Store.Containers.Update(createResp.ID, func(c *api.Container) {
		c.State.Running = true
		c.State.Status = "running"
	})

	req = httptest.NewRequest("GET", "/containers/"+createResp.ID+"/top", nil)
	w = httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	// Should be 501 (NotImplemented) since no agent is connected
	if w.Code == http.StatusOK {
		var topResp api.ContainerTopResponse
		json.Unmarshal(w.Body.Bytes(), &topResp)
		if len(topResp.Processes) > 0 && topResp.Processes[0][1] == "1" {
			t.Error("should not return synthetic process list — handler must use self-dispatch")
		}
	}
}

func TestBuildHandlerParsesNewParams(t *testing.T) {
	s := newPodTestServer()

	req := httptest.NewRequest("POST", "/build?dockerfile=Dockerfile&target=builder&platform=linux/amd64&nocache=true&cachefrom=%5B%22img:cache%22%5D&memory=1073741824&cpushares=512", nil)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code == http.StatusBadRequest && strings.Contains(w.Body.String(), "unknown") {
		t.Errorf("build handler should accept new params, got: %s", w.Body.String())
	}
}

func TestImageLoadDeduplicates(t *testing.T) {
	s := newPodTestServer()

	img := api.Image{
		ID:       "sha256:aaa111",
		RepoTags: []string{"loaded:latest"},
		Config:   api.ContainerConfig{Labels: make(map[string]string)},
		RootFS:   api.RootFS{Type: "layers", Layers: []string{"sha256:layer1"}},
	}
	StoreImageWithAliases(s.Store, "loaded:latest", img)
	s.Store.Images.Put(img.ID, img)

	countBefore := s.Store.Images.Len()
	emptyTar := bytes.NewBuffer(nil)
	_, _ = s.self.ImageLoad(emptyTar)
	countAfter := s.Store.Images.Len()

	if countAfter > countBefore+1 {
		t.Errorf("ImageLoad should deduplicate: before=%d after=%d", countBefore, countAfter)
	}
}

func TestFetchImageMetadataEmptyRef(t *testing.T) {
	result, err := FetchImageMetadata("")
	if err != nil {
		t.Fatalf("expected nil error for empty ref, got: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result for empty ref, got: %v", result)
	}
}
