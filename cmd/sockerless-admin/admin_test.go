package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegistryAddAndGet(t *testing.T) {
	reg := NewRegistry()
	reg.Add(Component{Name: "memory", Type: "backend", Addr: "http://localhost:3375"})

	c, ok := reg.Get("memory")
	if !ok {
		t.Fatal("expected component to exist")
	}
	if c.Name != "memory" {
		t.Errorf("expected name=memory, got %s", c.Name)
	}
	if c.Type != "backend" {
		t.Errorf("expected type=backend, got %s", c.Type)
	}
	if c.Health != "unknown" {
		t.Errorf("expected health=unknown, got %s", c.Health)
	}
}

func TestRegistryList(t *testing.T) {
	reg := NewRegistry()
	reg.Add(Component{Name: "memory", Type: "backend", Addr: "http://localhost:3375"})
	reg.Add(Component{Name: "sim-aws", Type: "simulator", Addr: "http://localhost:4566"})

	list := reg.List()
	if len(list) != 2 {
		t.Errorf("expected 2 components, got %d", len(list))
	}
}

func TestRegistryListByType(t *testing.T) {
	reg := NewRegistry()
	reg.Add(Component{Name: "memory", Type: "backend", Addr: "http://localhost:3375"})
	reg.Add(Component{Name: "ecs", Type: "backend", Addr: "http://localhost:9102"})
	reg.Add(Component{Name: "sim-aws", Type: "simulator", Addr: "http://localhost:4566"})

	backends := reg.ListByType("backend")
	if len(backends) != 2 {
		t.Errorf("expected 2 backends, got %d", len(backends))
	}

	sims := reg.ListByType("simulator")
	if len(sims) != 1 {
		t.Errorf("expected 1 simulator, got %d", len(sims))
	}
}

func TestRegistryOverwrite(t *testing.T) {
	reg := NewRegistry()
	reg.Add(Component{Name: "memory", Type: "backend", Addr: "http://localhost:3375"})
	reg.Add(Component{Name: "memory", Type: "backend", Addr: "http://localhost:9200"})

	if reg.Len() != 1 {
		t.Errorf("expected 1 component after overwrite, got %d", reg.Len())
	}
	c, _ := reg.Get("memory")
	if c.Addr != "http://localhost:9200" {
		t.Errorf("expected addr=http://localhost:9200, got %s", c.Addr)
	}
}

func TestHealthEndpoint(t *testing.T) {
	tests := []struct {
		typ  string
		want string
	}{
		{"backend", "/internal/v1/healthz"},
		{"simulator", "/health"},
		{"coordinator", "/health"},
	}
	for _, tt := range tests {
		if got := healthEndpoint(tt.typ); got != tt.want {
			t.Errorf("healthEndpoint(%s) = %s, want %s", tt.typ, got, tt.want)
		}
	}
}

func TestNormalizeAddr(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{":3375", "http://localhost:3375"},
		{"localhost:3375", "http://localhost:3375"},
		{"http://localhost:3375", "http://localhost:3375"},
		{"https://admin.example.com", "https://admin.example.com"},
	}
	for _, tt := range tests {
		if got := normalizeAddr(tt.input); got != tt.want {
			t.Errorf("normalizeAddr(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestHandleComponents(t *testing.T) {
	reg := NewRegistry()
	reg.Add(Component{Name: "memory", Type: "backend", Addr: "http://localhost:3375"})
	reg.Add(Component{Name: "sim-aws", Type: "simulator", Addr: "http://localhost:4566"})

	handler := handleComponents(reg)
	req := httptest.NewRequest("GET", "/api/v1/components", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var components []Component
	if err := json.Unmarshal(w.Body.Bytes(), &components); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(components) != 2 {
		t.Errorf("expected 2 components, got %d", len(components))
	}
}

func TestHandleComponentProxyNotFound(t *testing.T) {
	reg := NewRegistry()
	client := &http.Client{}

	handler := handleComponentProxy(reg, client, "health")
	req := httptest.NewRequest("GET", "/api/v1/components/nonexistent/health", nil)
	req.SetPathValue("name", "nonexistent")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleContexts(t *testing.T) {
	// Set SOCKERLESS_HOME to a temp dir with no contexts
	t.Setenv("SOCKERLESS_HOME", t.TempDir())

	handler := handleContexts()
	req := httptest.NewRequest("GET", "/api/v1/contexts", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var contexts []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &contexts); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(contexts) != 0 {
		t.Errorf("expected 0 contexts, got %d", len(contexts))
	}
}
