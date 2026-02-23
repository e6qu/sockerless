package core

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

func newNetTestServer() *BaseServer {
	store := NewStore()
	s := &BaseServer{
		Store:  store,
		Logger: zerolog.Nop(),
		Mux:    http.NewServeMux(),
	}
	s.InitDrivers()
	return s
}

func TestNetworkDisconnect_Basic(t *testing.T) {
	s := newNetTestServer()

	netID := "net1"
	s.Store.Networks.Put(netID, api.Network{
		Name:       "testnet",
		ID:         netID,
		Driver:     "bridge",
		Containers: map[string]api.EndpointResource{},
		IPAM:       api.IPAM{Config: []api.IPAMConfig{{Subnet: "172.20.0.0/16", Gateway: "172.20.0.1"}}},
	})

	cID := "c1"
	s.Store.Containers.Put(cID, api.Container{
		ID:   cID,
		Name: "/test",
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"testnet": {NetworkID: netID, IPAddress: "172.20.0.2"},
			},
		},
	})
	s.Store.ContainerNames.Put("/test", cID)

	s.Store.Networks.Update(netID, func(n *api.Network) {
		n.Containers[cID] = api.EndpointResource{Name: "test", IPv4Address: "172.20.0.2/16"}
	})

	body := `{"Container":"c1","Force":false}`
	req := httptest.NewRequest("POST", "/internal/v1/networks/net1/disconnect", strings.NewReader(body))
	req.SetPathValue("id", "net1")
	w := httptest.NewRecorder()
	s.handleNetworkDisconnect(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	net, _ := s.Store.Networks.Get(netID)
	if _, exists := net.Containers[cID]; exists {
		t.Fatal("container should be removed from network")
	}

	c, _ := s.Store.Containers.Get(cID)
	if _, exists := c.NetworkSettings.Networks["testnet"]; exists {
		t.Fatal("network should be removed from container")
	}
}

func TestNetworkDisconnect_NotFoundNetwork(t *testing.T) {
	s := newNetTestServer()

	body := `{"Container":"c1","Force":false}`
	req := httptest.NewRequest("POST", "/internal/v1/networks/missing/disconnect", strings.NewReader(body))
	req.SetPathValue("id", "missing")
	w := httptest.NewRecorder()
	s.handleNetworkDisconnect(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestNetworkDisconnect_NotFoundContainer(t *testing.T) {
	s := newNetTestServer()

	s.Store.Networks.Put("net1", api.Network{
		Name:       "testnet",
		ID:         "net1",
		Driver:     "bridge",
		Containers: map[string]api.EndpointResource{},
	})

	body := `{"Container":"missing","Force":false}`
	req := httptest.NewRequest("POST", "/internal/v1/networks/net1/disconnect", strings.NewReader(body))
	req.SetPathValue("id", "net1")
	w := httptest.NewRecorder()
	s.handleNetworkDisconnect(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestNetworkDisconnect_Force(t *testing.T) {
	s := newNetTestServer()

	s.Store.Networks.Put("net1", api.Network{
		Name:       "testnet",
		ID:         "net1",
		Driver:     "bridge",
		Containers: map[string]api.EndpointResource{},
	})

	body := `{"Container":"missing","Force":true}`
	req := httptest.NewRequest("POST", "/internal/v1/networks/net1/disconnect", strings.NewReader(body))
	req.SetPathValue("id", "net1")
	w := httptest.NewRecorder()
	s.handleNetworkDisconnect(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
