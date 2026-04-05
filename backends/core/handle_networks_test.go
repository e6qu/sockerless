package core

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sockerless/api"
)

func TestNetworkCreateSelfDispatch(t *testing.T) {
	s := newPodTestServer()

	body, _ := json.Marshal(api.NetworkCreateRequest{Name: "testnet", Driver: "bridge"})
	req := httptest.NewRequest("POST", "/networks/create", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp api.NetworkCreateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ID == "" {
		t.Fatal("expected non-empty network ID")
	}

	req = httptest.NewRequest("GET", "/networks/"+resp.ID, nil)
	w = httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("inspect after create: expected 200, got %d", w.Code)
	}
}

func TestNetworkRemoveSelfDispatch(t *testing.T) {
	s := newPodTestServer()

	body, _ := json.Marshal(api.NetworkCreateRequest{Name: "rmnet"})
	req := httptest.NewRequest("POST", "/networks/create", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	var resp api.NetworkCreateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	req = httptest.NewRequest("DELETE", "/networks/"+resp.ID, nil)
	w = httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("GET", "/networks/"+resp.ID, nil)
	w = httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after remove, got %d", w.Code)
	}
}

func TestNetworkInspectShowsContainers(t *testing.T) {
	s := newPodTestServer()

	body, _ := json.Marshal(api.NetworkCreateRequest{Name: "inspnet"})
	req := httptest.NewRequest("POST", "/networks/create", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	var netResp api.NetworkCreateResponse
	json.Unmarshal(w.Body.Bytes(), &netResp)

	createReq := api.ContainerCreateRequest{
		ContainerConfig: &api.ContainerConfig{Image: "alpine"},
		HostConfig:      &api.HostConfig{NetworkMode: "inspnet"},
	}
	body, _ = json.Marshal(createReq)
	req = httptest.NewRequest("POST", "/containers/create?name=netctr", bytes.NewReader(body))
	w = httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("container create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("GET", "/networks/"+netResp.ID, nil)
	w = httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("network inspect: expected 200, got %d", w.Code)
	}

	var net api.Network
	json.Unmarshal(w.Body.Bytes(), &net)
	if len(net.Containers) == 0 {
		t.Fatal("network inspect should show connected containers")
	}
}

func TestDefaultNetworkIDsAreFullLength(t *testing.T) {
	s := newPodTestServer()

	for _, name := range []string{"bridge", "host", "none"} {
		net, ok := s.Store.ResolveNetwork(name)
		if !ok {
			t.Fatalf("default network %q not found", name)
		}
		if len(net.ID) < 60 {
			t.Errorf("network %q ID should be full-length hex (%d chars): %q", name, len(net.ID), net.ID)
		}
	}
}
