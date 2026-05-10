package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func setupConfigServer(t *testing.T) (*TopologyManager, *http.ServeMux) {
	t.Helper()
	tmp := t.TempDir()
	mgr := NewTopologyManager(filepath.Join(tmp, "sockerless.yaml"), "")
	if err := mgr.LoadOrMigrate(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := mgr.Replace(Topology{Projects: []ProjectConfig{{
		Name: "p",
		Instances: []Instance{{
			Name:  "sim-aws",
			Kind:  InstanceKindSim,
			Cloud: CloudAWS,
			Port:  4500,
			Config: map[string]string{
				"SIM_LOG_LEVEL": "info",
				"SIM_DATA_DIR":  "/tmp/old",
			},
		}},
	}}}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	mux := http.NewServeMux()
	registerTopologyAPI(mux, mgr, nil) // lifecycle nil — reload tests handle 503
	return mgr, mux
}

func TestConfigMetadataEndpoint(t *testing.T) {
	_, mux := setupConfigServer(t)
	req := httptest.NewRequest("GET", "/api/v1/topology/config-metadata", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got struct {
		Keys []ConfigKeyMeta `json:"keys"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.Keys) == 0 {
		t.Fatalf("expected curated keys, got empty list")
	}
	// At least one hot-reloadable key in the curated set.
	hot := 0
	for _, k := range got.Keys {
		if k.HotReloadable {
			hot++
		}
	}
	if hot == 0 {
		t.Errorf("curated set should include at least one hot-reloadable key")
	}
}

func TestConfigUpdateClassifies(t *testing.T) {
	mgr, mux := setupConfigServer(t)

	// Change a hot-reloadable key + a restart-required key.
	body, _ := json.Marshal(map[string]string{
		"SIM_LOG_LEVEL": "debug",
		"SIM_DATA_DIR":  "/tmp/new",
	})
	req := httptest.NewRequest("PUT",
		"/api/v1/topology/projects/p/instances/sim-aws/config",
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}

	var got configUpdateResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.HotReloadableChanges) != 1 || got.HotReloadableChanges[0] != "SIM_LOG_LEVEL" {
		t.Errorf("hot = %v, want [SIM_LOG_LEVEL]", got.HotReloadableChanges)
	}
	if len(got.RestartRequiredChanges) != 1 || got.RestartRequiredChanges[0] != "SIM_DATA_DIR" {
		t.Errorf("restart = %v, want [SIM_DATA_DIR]", got.RestartRequiredChanges)
	}

	// Topology should now reflect the new config.
	ref, _ := mgr.FindInstance("p", "sim-aws")
	if ref.Instance.Config["SIM_LOG_LEVEL"] != "debug" {
		t.Errorf("config not persisted")
	}
	if ref.Instance.Config["SIM_DATA_DIR"] != "/tmp/new" {
		t.Errorf("config not persisted")
	}
}

func TestConfigUpdateNoChange(t *testing.T) {
	_, mux := setupConfigServer(t)
	body, _ := json.Marshal(map[string]string{
		"SIM_LOG_LEVEL": "info",     // unchanged
		"SIM_DATA_DIR":  "/tmp/old", // unchanged
	})
	req := httptest.NewRequest("PUT",
		"/api/v1/topology/projects/p/instances/sim-aws/config",
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got configUpdateResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.HotReloadableChanges) != 0 || len(got.RestartRequiredChanges) != 0 {
		t.Errorf("identity update should be a no-op, got hot=%v restart=%v",
			got.HotReloadableChanges, got.RestartRequiredChanges)
	}
}

func TestConfigUpdateInstanceNotFound(t *testing.T) {
	_, mux := setupConfigServer(t)
	body, _ := json.Marshal(map[string]string{"K": "v"})
	req := httptest.NewRequest("PUT",
		"/api/v1/topology/projects/p/instances/missing/config",
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestConfigUpdateBadJSON(t *testing.T) {
	_, mux := setupConfigServer(t)
	req := httptest.NewRequest("PUT",
		"/api/v1/topology/projects/p/instances/sim-aws/config",
		bytes.NewReader([]byte("{not-json")))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestReloadLifecycleNil(t *testing.T) {
	_, mux := setupConfigServer(t)
	req := httptest.NewRequest("POST",
		"/api/v1/topology/projects/p/instances/sim-aws/reload", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestReloadInstanceNotFound(t *testing.T) {
	tmp := t.TempDir()
	mgr := NewTopologyManager(filepath.Join(tmp, "sockerless.yaml"), "")
	_ = mgr.LoadOrMigrate()
	_ = mgr.Replace(Topology{Projects: []ProjectConfig{{Name: "p"}}})
	mux := http.NewServeMux()
	registerTopologyAPI(mux, mgr, NewInstanceLifecycle(tmp, 0))

	req := httptest.NewRequest("POST",
		"/api/v1/topology/projects/p/instances/missing/reload", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
