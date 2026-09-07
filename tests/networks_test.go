package tests

import (
	"testing"

	"github.com/moby/moby/client"
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

	netInspected, err := dockerClient.NetworkInspect(ctx, id, client.NetworkInspectOptions{})
	net := netInspected.Network
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

	netListed, err := dockerClient.NetworkList(ctx, client.NetworkListOptions{})
	networks := netListed.Items
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

	if _, err := dockerClient.NetworkRemove(ctx, id, client.NetworkRemoveOptions{}); err != nil {
		t.Fatalf("network remove failed: %v", err)
	}

	// Inspect should fail
	_, err := dockerClient.NetworkInspect(ctx, id, client.NetworkInspectOptions{})
	if err == nil {
		t.Error("expected error inspecting removed network")
	}
}

func TestNetworkPrune(t *testing.T) {
	id := createNetwork(t, "test-net-prune")
	_ = id

	pruned, err := dockerClient.NetworkPrune(ctx, client.NetworkPruneOptions{Filters: client.Filters{}})
	report := pruned.Report
	if err != nil {
		t.Fatalf("network prune failed: %v", err)
	}

	t.Logf("pruned networks: %v", report.NetworksDeleted)
}
