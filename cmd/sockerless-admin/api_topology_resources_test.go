package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
)

func upstreamBackendOnPort(t *testing.T, status int, body string) (port int, stop func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/resources" {
			t.Errorf("upstream path = %q, want /internal/v1/resources", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	u, _ := url.Parse(srv.URL)
	p, _ := strconv.Atoi(u.Port())
	return p, srv.Close
}

func setupRollupServer(t *testing.T, instances []Instance) *http.ServeMux {
	t.Helper()
	tmp := t.TempDir()
	mgr := NewTopologyManager(filepath.Join(tmp, "sockerless.yaml"))
	if err := mgr.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := mgr.Replace(Topology{Projects: []ProjectConfig{{
		Name:      "p",
		Instances: instances,
	}}}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	mux := http.NewServeMux()
	registerTopologyAPI(mux, mgr, nil)
	return mux
}

func TestRollupAggregatesAcrossBackends(t *testing.T) {
	port1, stop1 := upstreamBackendOnPort(t, http.StatusOK,
		`[{"containerId":"c1","resourceType":"task","resourceId":"r1","status":"RUNNING","cleanedUp":false}]`)
	defer stop1()
	port2, stop2 := upstreamBackendOnPort(t, http.StatusOK,
		`[{"containerId":"c2","resourceType":"function","resourceId":"r2","status":"ACTIVE","cleanedUp":false},
		  {"containerId":"c3","resourceType":"function","resourceId":"r3","status":"ACTIVE","cleanedUp":true}]`)
	defer stop2()

	mux := setupRollupServer(t, []Instance{
		{Name: "be-ecs", Kind: InstanceKindBackend, Cloud: CloudAWS, Backend: BackendECS, Port: port1},
		{Name: "be-lambda", Kind: InstanceKindBackend, Cloud: CloudAWS, Backend: BackendLambda, Port: port2},
		{Name: "sim-aws", Kind: InstanceKindSim, Cloud: CloudAWS, Port: 9999}, // sims are skipped
	})

	req := httptest.NewRequest("GET", "/api/v1/topology/resources", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	var got rollupResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Sources) != 2 {
		t.Errorf("sources = %d, want 2 (sim instance must be excluded)", len(got.Sources))
	}
	if len(got.Resources) != 3 {
		t.Errorf("resources = %d, want 3", len(got.Resources))
	}
	// Each resource carries project + instance + cloud + backend.
	for _, r := range got.Resources {
		if r.Project != "p" {
			t.Errorf("project = %q", r.Project)
		}
		if r.Cloud != "aws" {
			t.Errorf("cloud = %q", r.Cloud)
		}
		if r.Instance == "" || r.Backend == "" {
			t.Errorf("missing identity: %+v", r)
		}
	}

	// Sources are tagged ok and have right counts.
	sort.Slice(got.Sources, func(i, j int) bool {
		return got.Sources[i].Instance < got.Sources[j].Instance
	})
	if !got.Sources[0].OK || got.Sources[0].ResourceCount != 1 {
		t.Errorf("be-ecs source: %+v", got.Sources[0])
	}
	if !got.Sources[1].OK || got.Sources[1].ResourceCount != 2 {
		t.Errorf("be-lambda source: %+v", got.Sources[1])
	}
}

func TestRollupSurfacesUpstreamErrors(t *testing.T) {
	// Upstream returns 500 → source should be marked not-ok with error.
	port, stop := upstreamBackendOnPort(t, http.StatusInternalServerError, `boom`)
	defer stop()

	mux := setupRollupServer(t, []Instance{
		{Name: "be-ecs", Kind: InstanceKindBackend, Cloud: CloudAWS, Backend: BackendECS, Port: port},
	})

	req := httptest.NewRequest("GET", "/api/v1/topology/resources", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got rollupResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.Sources) != 1 || got.Sources[0].OK {
		t.Errorf("expected one not-ok source, got %+v", got.Sources)
	}
	if got.Sources[0].Error == "" {
		t.Errorf("expected error message")
	}
	if len(got.Resources) != 0 {
		t.Errorf("expected no resources, got %d", len(got.Resources))
	}
}

func TestRollupUnreachableBackend(t *testing.T) {
	mux := setupRollupServer(t, []Instance{
		// Port 1 is privileged → connect refused on the test runner.
		{Name: "be-ecs", Kind: InstanceKindBackend, Cloud: CloudAWS, Backend: BackendECS, Port: 1},
	})
	req := httptest.NewRequest("GET", "/api/v1/topology/resources", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got rollupResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.Sources) != 1 || got.Sources[0].OK {
		t.Errorf("expected one not-ok source, got %+v", got.Sources)
	}
}

func TestRollupRespectsActiveQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "active=true" {
			t.Errorf("rawquery = %q, want active=true", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())

	mux := setupRollupServer(t, []Instance{
		{Name: "be-ecs", Kind: InstanceKindBackend, Cloud: CloudAWS, Backend: BackendECS, Port: port},
	})
	req := httptest.NewRequest("GET", "/api/v1/topology/resources?active=true", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestRollupEmpty(t *testing.T) {
	mux := setupRollupServer(t, []Instance{
		{Name: "sim", Kind: InstanceKindSim, Cloud: CloudAWS, Port: 4500},
	})
	req := httptest.NewRequest("GET", "/api/v1/topology/resources", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got rollupResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.Sources) != 0 {
		t.Errorf("sources = %d, want 0 (no backend instances)", len(got.Sources))
	}
	if got.Resources == nil {
		t.Errorf("resources should serialise as []")
	}
}

// Sanity: keep test runtime bounded under load.
func TestRollupConcurrent(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	// 5 backends each returning 2 resources → 10 rows.
	var ports []int
	var stops []func()
	for i := 0; i < 5; i++ {
		port, stop := upstreamBackendOnPort(t, http.StatusOK,
			fmt.Sprintf(`[{"resourceType":"x","resourceId":"r-%d-1"},{"resourceType":"x","resourceId":"r-%d-2"}]`, i, i))
		ports = append(ports, port)
		stops = append(stops, stop)
	}
	defer func() {
		for _, s := range stops {
			s()
		}
	}()

	insts := make([]Instance, 0, len(ports))
	for i, p := range ports {
		insts = append(insts, Instance{
			Name:    fmt.Sprintf("be-%d", i),
			Kind:    InstanceKindBackend,
			Cloud:   CloudAWS,
			Backend: BackendECS,
			Port:    p,
		})
	}
	mux := setupRollupServer(t, insts)
	req := httptest.NewRequest("GET", "/api/v1/topology/resources", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got rollupResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.Sources) != 5 {
		t.Errorf("sources = %d, want 5", len(got.Sources))
	}
	if len(got.Resources) != 10 {
		t.Errorf("resources = %d, want 10", len(got.Resources))
	}
}
