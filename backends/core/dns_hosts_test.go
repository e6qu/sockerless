package core

import (
	"strings"
	"testing"

	"github.com/sockerless/api"
)

func TestResolvePeerHosts_SamePod(t *testing.T) {
	store := NewStore()

	// Create two containers on the same user-defined network
	c1 := api.Container{
		ID:   "aaa111",
		Name: "/web",
		Config: api.ContainerConfig{
			Hostname: "web",
		},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"mynet": {IPAddress: "10.0.0.2", Aliases: []string{"web-alias"}},
			},
		},
	}
	c2 := api.Container{
		ID:   "bbb222",
		Name: "/db",
		Config: api.ContainerConfig{
			Hostname: "db",
		},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"mynet": {IPAddress: "10.0.0.3", Aliases: []string{"database"}},
			},
		},
	}
	store.Containers.Put("aaa111", c1)
	store.Containers.Put("bbb222", c2)

	// Create pod with both containers
	pod := store.Pods.CreatePod("testpod", nil)
	store.Pods.AddContainer(pod.ID, "aaa111")
	store.Pods.AddContainer(pod.ID, "bbb222")

	// From container A, resolve B
	hosts := ResolvePeerHosts(store, "aaa111")
	joined := strings.Join(hosts, "|")

	if !strings.Contains(joined, "db:10.0.0.3") {
		t.Fatalf("expected db:10.0.0.3 in hosts, got: %v", hosts)
	}
	if !strings.Contains(joined, "database:10.0.0.3") {
		t.Fatalf("expected database:10.0.0.3 alias in hosts, got: %v", hosts)
	}
}

func TestResolvePeerHosts_NoPod(t *testing.T) {
	store := NewStore()

	c1 := api.Container{
		ID:   "aaa111",
		Name: "/solo",
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"bridge": {IPAddress: "172.17.0.2"},
			},
		},
	}
	store.Containers.Put("aaa111", c1)

	hosts := ResolvePeerHosts(store, "aaa111")
	if len(hosts) != 0 {
		t.Fatalf("expected empty hosts for solo bridge container, got: %v", hosts)
	}
}

func TestResolvePeerHosts_IncludesHostname(t *testing.T) {
	store := NewStore()

	c1 := api.Container{
		ID:   "aaa111",
		Name: "/app",
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"mynet": {IPAddress: "10.0.0.2"},
			},
		},
	}
	c2 := api.Container{
		ID:   "bbb222",
		Name: "/redis",
		Config: api.ContainerConfig{
			Hostname: "cache-host",
		},
		NetworkSettings: api.NetworkSettings{
			Networks: map[string]*api.EndpointSettings{
				"mynet": {IPAddress: "10.0.0.5"},
			},
		},
	}
	store.Containers.Put("aaa111", c1)
	store.Containers.Put("bbb222", c2)

	pod := store.Pods.CreatePod("testpod2", nil)
	store.Pods.AddContainer(pod.ID, "aaa111")
	store.Pods.AddContainer(pod.ID, "bbb222")

	hosts := ResolvePeerHosts(store, "aaa111")
	joined := strings.Join(hosts, "|")

	if !strings.Contains(joined, "redis:10.0.0.5") {
		t.Fatalf("expected redis:10.0.0.5 in hosts, got: %v", hosts)
	}
	if !strings.Contains(joined, "cache-host:10.0.0.5") {
		t.Fatalf("expected cache-host:10.0.0.5 in hosts, got: %v", hosts)
	}
}
