package tests

import (
	"testing"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
)

func TestNetworkCreate(t *testing.T) {
	id := createNetwork(t, "test-net")
	defer removeNetwork(t, id)

	if len(id) == 0 {
		t.Error("expected non-empty network ID")
	}
}

func TestNetworkInspect(t *testing.T) {
	id := createNetwork(t, "test-net-inspect")
	defer removeNetwork(t, id)

	net, err := dockerClient.NetworkInspect(ctx, id, network.InspectOptions{})
	if err != nil {
		t.Fatalf("network inspect failed: %v", err)
	}

	if net.Name != "test-net-inspect" {
		t.Errorf("expected name test-net-inspect, got %s", net.Name)
	}

	if net.Driver != "bridge" {
		t.Errorf("expected driver bridge, got %s", net.Driver)
	}
}

func TestNetworkList(t *testing.T) {
	id := createNetwork(t, "test-net-list")
	defer removeNetwork(t, id)

	networks, err := dockerClient.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		t.Fatalf("network list failed: %v", err)
	}

	found := false
	for _, n := range networks {
		if n.ID == id {
			found = true
			break
		}
	}
	if !found {
		t.Error("created network not found in list")
	}
}

func TestNetworkRemove(t *testing.T) {
	id := createNetwork(t, "test-net-remove")

	if err := dockerClient.NetworkRemove(ctx, id); err != nil {
		t.Fatalf("network remove failed: %v", err)
	}

	// Inspect should fail
	_, err := dockerClient.NetworkInspect(ctx, id, network.InspectOptions{})
	if err == nil {
		t.Error("expected error inspecting removed network")
	}
}

func TestNetworkPrune(t *testing.T) {
	id := createNetwork(t, "test-net-prune")
	_ = id

	report, err := dockerClient.NetworksPrune(ctx, filters.Args{})
	if err != nil {
		t.Fatalf("network prune failed: %v", err)
	}

	t.Logf("pruned networks: %v", report.NetworksDeleted)
}
