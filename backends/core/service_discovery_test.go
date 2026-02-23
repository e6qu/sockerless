package core

import (
	"strings"
	"testing"

	"github.com/sockerless/api"
)

func TestServiceDiscoverySamePodPeerResolution(t *testing.T) {
	store := NewStore()

	// Create three containers in the same pod on a shared network
	containers := []struct {
		id       string
		name     string
		hostname string
		ip       string
		aliases  []string
	}{
		{"svc-web", "/web", "web-host", "10.0.1.2", []string{"frontend"}},
		{"svc-api", "/api-server", "api-host", "10.0.1.3", []string{"backend"}},
		{"svc-db", "/postgres", "db-host", "10.0.1.4", []string{"database", "pg"}},
	}

	for _, ct := range containers {
		c := api.Container{
			ID:     ct.id,
			Name:   ct.name,
			Config: api.ContainerConfig{Hostname: ct.hostname},
			NetworkSettings: api.NetworkSettings{
				Networks: map[string]*api.EndpointSettings{
					"app-network": {IPAddress: ct.ip, Aliases: ct.aliases},
				},
			},
		}
		store.Containers.Put(ct.id, c)
	}

	pod := store.Pods.CreatePod("app-pod", nil)
	store.Pods.AddContainer(pod.ID, "svc-web")
	store.Pods.AddContainer(pod.ID, "svc-api")
	store.Pods.AddContainer(pod.ID, "svc-db")

	// From web, resolve peers
	hosts := ResolvePeerHosts(store, "svc-web")
	joined := strings.Join(hosts, "|")

	// Should see api-server and postgres by name
	if !strings.Contains(joined, "api-server:10.0.1.3") {
		t.Errorf("expected api-server:10.0.1.3, got: %v", hosts)
	}
	if !strings.Contains(joined, "postgres:10.0.1.4") {
		t.Errorf("expected postgres:10.0.1.4, got: %v", hosts)
	}

	// Should see aliases
	if !strings.Contains(joined, "backend:10.0.1.3") {
		t.Errorf("expected backend alias, got: %v", hosts)
	}
	if !strings.Contains(joined, "database:10.0.1.4") {
		t.Errorf("expected database alias, got: %v", hosts)
	}
	if !strings.Contains(joined, "pg:10.0.1.4") {
		t.Errorf("expected pg alias, got: %v", hosts)
	}

	// Should NOT see web itself
	if strings.Contains(joined, "web:") || strings.Contains(joined, "web-host:10.0.1.2") {
		t.Errorf("should not see own container in peer hosts, got: %v", hosts)
	}
}

func TestServiceDiscoveryHostnameOverride(t *testing.T) {
	store := NewStore()

	c1 := api.Container{
		ID:     "hname1",
		Name:   "/svc-a",
		Config: api.ContainerConfig{Hostname: "custom-hostname"},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"testnet": {IPAddress: "10.0.2.2"},
			},
		},
	}
	c2 := api.Container{
		ID:   "hname2",
		Name: "/svc-b",
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"testnet": {IPAddress: "10.0.2.3"},
			},
		},
	}
	store.Containers.Put("hname1", c1)
	store.Containers.Put("hname2", c2)

	pod := store.Pods.CreatePod("hname-pod", nil)
	store.Pods.AddContainer(pod.ID, "hname1")
	store.Pods.AddContainer(pod.ID, "hname2")

	// From svc-b, resolve svc-a
	hosts := ResolvePeerHosts(store, "hname2")
	joined := strings.Join(hosts, "|")

	// Should see both name and hostname
	if !strings.Contains(joined, "svc-a:10.0.2.2") {
		t.Errorf("expected svc-a:10.0.2.2, got: %v", hosts)
	}
	if !strings.Contains(joined, "custom-hostname:10.0.2.2") {
		t.Errorf("expected custom-hostname:10.0.2.2, got: %v", hosts)
	}
}

func TestServiceDiscoveryAliasResolution(t *testing.T) {
	store := NewStore()

	c1 := api.Container{
		ID:     "alias1",
		Name:   "/redis",
		Config: api.ContainerConfig{Hostname: "redis"},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"backend-net": {IPAddress: "10.0.3.2", Aliases: []string{"cache", "kv-store"}},
			},
		},
	}
	c2 := api.Container{
		ID:   "alias2",
		Name: "/app",
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"backend-net": {IPAddress: "10.0.3.3"},
			},
		},
	}
	store.Containers.Put("alias1", c1)
	store.Containers.Put("alias2", c2)

	pod := store.Pods.CreatePod("alias-pod", nil)
	store.Pods.AddContainer(pod.ID, "alias1")
	store.Pods.AddContainer(pod.ID, "alias2")

	hosts := ResolvePeerHosts(store, "alias2")
	joined := strings.Join(hosts, "|")

	if !strings.Contains(joined, "redis:10.0.3.2") {
		t.Errorf("expected redis:10.0.3.2, got: %v", hosts)
	}
	if !strings.Contains(joined, "cache:10.0.3.2") {
		t.Errorf("expected cache alias, got: %v", hosts)
	}
	if !strings.Contains(joined, "kv-store:10.0.3.2") {
		t.Errorf("expected kv-store alias, got: %v", hosts)
	}
}
