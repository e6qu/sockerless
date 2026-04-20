package ecs

import (
	"testing"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// serverWithNetworks returns a Server wired only with a NetworkState so
// searchDomainsForContainer can be tested without AWS clients.
func serverWithNetworks(entries map[string]NetworkState) *Server {
	s := &Server{
		NetworkState: core.NewStateStore[NetworkState](),
	}
	for id, ns := range entries {
		s.NetworkState.Put(id, ns)
	}
	return s
}

func TestSearchDomainsForContainer_Nil(t *testing.T) {
	s := serverWithNetworks(nil)
	if got := s.searchDomainsForContainer(nil); got != nil {
		t.Fatalf("expected nil for nil container, got %v", got)
	}
}

func TestSearchDomainsForContainer_SkipsPredefinedNetworks(t *testing.T) {
	s := serverWithNetworks(nil)
	c := &api.Container{
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"bridge": {NetworkID: "nid-bridge"},
				"host":   {NetworkID: "nid-host"},
				"none":   {NetworkID: "nid-none"},
			},
		},
	}
	if got := s.searchDomainsForContainer(c); len(got) != 0 {
		t.Fatalf("expected no domains for bridge/host/none, got %v", got)
	}
}

func TestSearchDomainsForContainer_SkipsNetworksWithoutNamespace(t *testing.T) {
	s := serverWithNetworks(map[string]NetworkState{
		"nid-foo": {NamespaceID: ""}, // namespace not registered
	})
	c := &api.Container{
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"foo": {NetworkID: "nid-foo"},
			},
		},
	}
	if got := s.searchDomainsForContainer(c); len(got) != 0 {
		t.Fatalf("expected no domains when namespace missing, got %v", got)
	}
}

func TestSearchDomainsForContainer_SkipsEmptyNetworkID(t *testing.T) {
	s := serverWithNetworks(nil)
	c := &api.Container{
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"foo": {NetworkID: ""},
				"bar": nil,
			},
		},
	}
	if got := s.searchDomainsForContainer(c); len(got) != 0 {
		t.Fatalf("expected no domains, got %v", got)
	}
}
