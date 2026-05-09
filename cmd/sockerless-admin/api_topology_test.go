package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func setupTopologyServer(t *testing.T) (*TopologyManager, *http.ServeMux) {
	t.Helper()
	dir := t.TempDir()
	mgr := NewTopologyManager(filepath.Join(dir, "sockerless.yaml"), "")
	if err := mgr.LoadOrMigrate(); err != nil {
		t.Fatalf("load: %v", err)
	}
	mux := http.NewServeMux()
	registerTopologyAPI(mux, mgr)
	return mgr, mux
}

func TestAPITopologyGetEmpty(t *testing.T) {
	_, mux := setupTopologyServer(t)
	req := httptest.NewRequest("GET", "/api/v1/topology", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var got Topology
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Projects) != 0 {
		t.Errorf("empty topology should serialise with 0 projects, got %d", len(got.Projects))
	}
}

func TestAPITopologyPutThenGet(t *testing.T) {
	_, mux := setupTopologyServer(t)
	body, _ := json.Marshal(Topology{Projects: []ProjectConfig{
		{Name: "p", Instances: []Instance{
			{Name: "s", Kind: InstanceKindSim, Cloud: CloudAWS, Port: 4500},
		}},
	}})
	req := httptest.NewRequest("PUT", "/api/v1/topology", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	req2 := httptest.NewRequest("GET", "/api/v1/topology", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	var got Topology
	_ = json.Unmarshal(w2.Body.Bytes(), &got)
	if len(got.Projects) != 1 || got.Projects[0].Name != "p" {
		t.Errorf("GET after PUT: %+v", got)
	}
}

func TestAPITopologyPutInvalid(t *testing.T) {
	_, mux := setupTopologyServer(t)

	cases := []struct {
		name string
		body string
		want int
	}{
		{
			name: "invalid JSON",
			body: `{not-json`,
			want: http.StatusBadRequest,
		},
		{
			name: "validation failure (duplicate project)",
			body: `{"projects": [{"name": "p"}, {"name": "p"}]}`,
			want: http.StatusBadRequest,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("PUT", "/api/v1/topology", bytes.NewReader([]byte(tc.body)))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Errorf("status = %d, want %d; body=%s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestAPITopologyInstancesAndFind(t *testing.T) {
	mgr, mux := setupTopologyServer(t)
	_ = mgr.Replace(Topology{Projects: []ProjectConfig{
		{Name: "p", Instances: []Instance{
			{Name: "s", Kind: InstanceKindSim, Cloud: CloudAWS, Port: 4500},
		}},
	}})

	req := httptest.NewRequest("GET", "/api/v1/topology/instances", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var refs []InstanceRef
	_ = json.Unmarshal(w.Body.Bytes(), &refs)
	if len(refs) != 1 || refs[0].Project != "p" || refs[0].Instance.Name != "s" {
		t.Errorf("flat list: %+v", refs)
	}

	// Find by path.
	req2 := httptest.NewRequest("GET", "/api/v1/topology/projects/p/instances/s", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("find hit: %d %s", w2.Code, w2.Body.String())
	}

	// Find miss.
	req3 := httptest.NewRequest("GET", "/api/v1/topology/projects/p/instances/absent", nil)
	w3 := httptest.NewRecorder()
	mux.ServeHTTP(w3, req3)
	if w3.Code != http.StatusNotFound {
		t.Errorf("find miss: %d", w3.Code)
	}
}
