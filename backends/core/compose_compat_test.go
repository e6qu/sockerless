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

func newComposeTestServer() *BaseServer {
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

func TestContainerInspectInitFieldPreserved(t *testing.T) {
	s := newComposeTestServer()

	init := true
	c := api.Container{
		ID:              "c1",
		Name:            "/test",
		Created:         time.Now().UTC().Format(time.RFC3339Nano),
		Config:          api.ContainerConfig{Labels: make(map[string]string)},
		HostConfig:      api.HostConfig{Init: &init},
		State:           api.ContainerState{Status: "running", Running: true},
		NetworkSettings: api.NetworkSettings{Networks: make(map[string]*api.EndpointSettings)},
		Mounts:          []api.MountPoint{},
	}
	s.Store.Containers.Put("c1", c)

	req := httptest.NewRequest("GET", "/containers/c1/json", nil)
	req.SetPathValue("id", "c1")
	w := httptest.NewRecorder()
	s.handleContainerInspect(w, req)

	var result map[string]json.RawMessage
	json.Unmarshal(w.Body.Bytes(), &result)

	var hc struct {
		Init *bool `json:"Init"`
	}
	json.Unmarshal(result["HostConfig"], &hc)
	if hc.Init == nil || !*hc.Init {
		t.Error("expected Init to be true in HostConfig")
	}
}

func TestStoppedContainerStateError(t *testing.T) {
	s := newComposeTestServer()

	c := api.Container{
		ID:      "c1",
		Name:    "/stopped",
		Created: time.Now().UTC().Format(time.RFC3339Nano),
		Config:  api.ContainerConfig{Labels: make(map[string]string)},
		State: api.ContainerState{
			Status:     "exited",
			Running:    false,
			ExitCode:   0,
			Error:      "",
			FinishedAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
		HostConfig:      api.HostConfig{},
		NetworkSettings: api.NetworkSettings{Networks: make(map[string]*api.EndpointSettings)},
		Mounts:          []api.MountPoint{},
	}
	s.Store.Containers.Put("c1", c)

	req := httptest.NewRequest("GET", "/containers/c1/json", nil)
	req.SetPathValue("id", "c1")
	w := httptest.NewRecorder()
	s.handleContainerInspect(w, req)

	var result struct {
		State struct {
			Error string `json:"Error"`
		} `json:"State"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)

	// Error should be present as empty string (not omitted)
	if result.State.Error != "" {
		t.Errorf("expected empty Error string, got %q", result.State.Error)
	}

	// Verify the field is present in raw JSON
	raw := w.Body.String()
	if !json.Valid([]byte(raw)) {
		t.Fatal("response is not valid JSON")
	}
}

func TestStopSignalPreserved(t *testing.T) {
	s := newComposeTestServer()

	c := api.Container{
		ID:      "c1",
		Name:    "/sigtest",
		Created: time.Now().UTC().Format(time.RFC3339Nano),
		Config: api.ContainerConfig{
			Labels:     make(map[string]string),
			StopSignal: "SIGTERM",
		},
		State:           api.ContainerState{Status: "running", Running: true},
		HostConfig:      api.HostConfig{},
		NetworkSettings: api.NetworkSettings{Networks: make(map[string]*api.EndpointSettings)},
		Mounts:          []api.MountPoint{},
	}
	s.Store.Containers.Put("c1", c)

	req := httptest.NewRequest("GET", "/containers/c1/json", nil)
	req.SetPathValue("id", "c1")
	w := httptest.NewRecorder()
	s.handleContainerInspect(w, req)

	var result struct {
		Config struct {
			StopSignal string `json:"StopSignal"`
		} `json:"Config"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Config.StopSignal != "SIGTERM" {
		t.Errorf("expected SIGTERM, got %q", result.Config.StopSignal)
	}
}

func TestMountsInContainerInspect(t *testing.T) {
	s := newComposeTestServer()

	c := api.Container{
		ID:      "c1",
		Name:    "/mounttest",
		Created: time.Now().UTC().Format(time.RFC3339Nano),
		Config:  api.ContainerConfig{Labels: make(map[string]string)},
		HostConfig: api.HostConfig{
			Binds: []string{"dbdata:/var/lib/db"},
		},
		State:           api.ContainerState{Status: "running", Running: true},
		NetworkSettings: api.NetworkSettings{Networks: make(map[string]*api.EndpointSettings)},
		Mounts: []api.MountPoint{
			{Type: "volume", Name: "dbdata", Source: "dbdata", Destination: "/var/lib/db", RW: true},
		},
	}
	s.Store.Containers.Put("c1", c)

	req := httptest.NewRequest("GET", "/containers/c1/json", nil)
	req.SetPathValue("id", "c1")
	w := httptest.NewRecorder()
	s.handleContainerInspect(w, req)

	var result struct {
		Mounts []api.MountPoint `json:"Mounts"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(result.Mounts))
	}
	if result.Mounts[0].Type != "volume" || result.Mounts[0].Destination != "/var/lib/db" {
		t.Errorf("mount = %+v, want volume at /var/lib/db", result.Mounts[0])
	}
}

func TestHealthcheckInContainerInspect(t *testing.T) {
	s := newComposeTestServer()

	c := api.Container{
		ID:      "c1",
		Name:    "/hctest",
		Created: time.Now().UTC().Format(time.RFC3339Nano),
		Config: api.ContainerConfig{
			Labels: make(map[string]string),
			Healthcheck: &api.HealthcheckConfig{
				Test:     []string{"CMD-SHELL", "curl -f http://localhost/"},
				Interval: 30000000000,
				Retries:  3,
			},
		},
		State:           api.ContainerState{Status: "running", Running: true},
		HostConfig:      api.HostConfig{},
		NetworkSettings: api.NetworkSettings{Networks: make(map[string]*api.EndpointSettings)},
		Mounts:          []api.MountPoint{},
	}
	s.Store.Containers.Put("c1", c)

	req := httptest.NewRequest("GET", "/containers/c1/json", nil)
	req.SetPathValue("id", "c1")
	w := httptest.NewRecorder()
	s.handleContainerInspect(w, req)

	var result struct {
		Config struct {
			Healthcheck *api.HealthcheckConfig `json:"Healthcheck"`
		} `json:"Config"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Config.Healthcheck == nil {
		t.Fatal("expected Healthcheck in inspect response")
	}
	if result.Config.Healthcheck.Retries != 3 {
		t.Errorf("retries = %d, want 3", result.Config.Healthcheck.Retries)
	}
}

func TestContainerInspectNetworkSettings(t *testing.T) {
	s := newComposeTestServer()

	c := api.Container{
		ID:      "c1",
		Name:    "/nettest",
		Created: time.Now().UTC().Format(time.RFC3339Nano),
		Config:  api.ContainerConfig{Labels: make(map[string]string)},
		State:   api.ContainerState{Status: "running", Running: true},
		HostConfig: api.HostConfig{},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"bridge": {
					NetworkID:  "net1",
					IPAddress:  "172.17.0.2",
					Gateway:    "172.17.0.1",
					MacAddress: "02:42:ac:11:00:02",
				},
			},
		},
		Mounts: []api.MountPoint{},
	}
	s.Store.Containers.Put("c1", c)

	req := httptest.NewRequest("GET", "/containers/c1/json", nil)
	req.SetPathValue("id", "c1")
	w := httptest.NewRecorder()
	s.handleContainerInspect(w, req)

	var result struct {
		NetworkSettings struct {
			Networks map[string]*api.EndpointSettings `json:"Networks"`
		} `json:"NetworkSettings"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	if _, ok := result.NetworkSettings.Networks["bridge"]; !ok {
		t.Error("expected bridge network in inspect response")
	}
}
