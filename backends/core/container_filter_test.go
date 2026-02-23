package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sockerless/api"
)

func TestContainerFilter_Ancestor(t *testing.T) {
	s := newListTestServer()
	addContainer(s, "c1", "alpine-ctr", "alpine:3.18", true, 0)
	addContainer(s, "c2", "nginx-ctr", "nginx:latest", true, time.Hour)

	filters := `{"ancestor":{"alpine:3.18":true}}`
	req := httptest.NewRequest("GET", "/internal/v1/containers?all=1&filters="+filters, nil)
	w := httptest.NewRecorder()
	s.handleContainerList(w, req)

	var result []*api.ContainerSummary
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Image != "alpine:3.18" {
		t.Fatalf("expected alpine:3.18, got %q", result[0].Image)
	}
}

func TestContainerFilter_AncestorNoMatch(t *testing.T) {
	s := newListTestServer()
	addContainer(s, "c1", "alpine-ctr", "alpine:3.18", true, 0)

	filters := `{"ancestor":{"ubuntu:22.04":true}}`
	req := httptest.NewRequest("GET", "/internal/v1/containers?all=1&filters="+filters, nil)
	w := httptest.NewRecorder()
	s.handleContainerList(w, req)

	var result []*api.ContainerSummary
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 0 {
		t.Fatalf("expected 0 results, got %d", len(result))
	}
}

func TestContainerFilter_Network(t *testing.T) {
	s := newListTestServer()

	cID := "c1"
	s.Store.Containers.Put(cID, api.Container{
		ID:      cID,
		Name:    "/net-ctr",
		Created: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		Config: api.ContainerConfig{
			Image:  "alpine:3.18",
			Labels: make(map[string]string),
		},
		State: api.ContainerState{Status: "running", Running: true},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"mynet": {NetworkID: "net1", IPAddress: "172.18.0.2"},
			},
		},
		Mounts: make([]api.MountPoint, 0),
	})
	s.Store.ContainerNames.Put("/net-ctr", cID)

	addContainer(s, "c2", "other-ctr", "alpine:3.18", true, time.Hour)

	filters := `{"network":{"mynet":true}}`
	req := httptest.NewRequest("GET", "/internal/v1/containers?all=1&filters="+filters, nil)
	w := httptest.NewRecorder()
	s.handleContainerList(w, req)

	var result []*api.ContainerSummary
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
}

func TestContainerFilter_Health(t *testing.T) {
	s := newListTestServer()

	// Container with health status "healthy"
	s.Store.Containers.Put("c1", api.Container{
		ID:      "c1",
		Name:    "/healthy-ctr",
		Created: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		Config: api.ContainerConfig{
			Image:  "alpine:3.18",
			Labels: make(map[string]string),
		},
		State: api.ContainerState{
			Status:  "running",
			Running: true,
			Health:  &api.HealthState{Status: "healthy"},
		},
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
		Mounts: make([]api.MountPoint, 0),
	})
	s.Store.ContainerNames.Put("/healthy-ctr", "c1")

	// Container with no health check (defaults to "none")
	addContainer(s, "c2", "no-health", "alpine:3.18", true, time.Hour)

	// Filter for healthy
	filters := `{"health":{"healthy":true}}`
	req := httptest.NewRequest("GET", "/internal/v1/containers?all=1&filters="+filters, nil)
	w := httptest.NewRecorder()
	s.handleContainerList(w, req)

	var result []*api.ContainerSummary
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 1 {
		t.Fatalf("expected 1 healthy result, got %d", len(result))
	}

	// Filter for "none" should match the container without health check
	filters = `{"health":{"none":true}}`
	req = httptest.NewRequest("GET", "/internal/v1/containers?all=1&filters="+filters, nil)
	w = httptest.NewRecorder()
	s.handleContainerList(w, req)

	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 1 {
		t.Fatalf("expected 1 none-health result, got %d", len(result))
	}
}

func TestContainerFilter_BeforeSince(t *testing.T) {
	s := newListTestServer()
	// A (oldest), B (middle), C (newest)
	for i, name := range []string{"cA", "cB", "cC"} {
		addContainer(s, fmt.Sprintf("c%d", i), name, "alpine:3.18", true, time.Duration(i)*time.Hour)
	}

	// before=cB should return only A (created before B)
	filters := `{"before":{"cB":true}}`
	req := httptest.NewRequest("GET", "/internal/v1/containers?all=1&filters="+filters, nil)
	w := httptest.NewRecorder()
	s.handleContainerList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result []*api.ContainerSummary
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 1 {
		t.Fatalf("before: expected 1 result, got %d", len(result))
	}

	// since=cA should return B and C (created after A)
	filters = `{"since":{"cA":true}}`
	req = httptest.NewRequest("GET", "/internal/v1/containers?all=1&filters="+filters, nil)
	w = httptest.NewRecorder()
	s.handleContainerList(w, req)

	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 2 {
		t.Fatalf("since: expected 2 results, got %d", len(result))
	}
}
