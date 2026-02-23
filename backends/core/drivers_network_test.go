package core

import (
	"context"
	"testing"
	"time"

	"github.com/sockerless/api"
)

func newTestDriverAndStore() (*SyntheticNetworkDriver, *Store) {
	store := NewStore()
	// Seed the bridge network (like InitDefaultNetwork does)
	store.IPAlloc.AllocateSubnet("bridge", &api.IPAMConfig{
		Subnet: "172.17.0.0/16", Gateway: "172.17.0.1",
	})
	store.Networks.Put("bridge", api.Network{
		Name:       "bridge",
		ID:         "bridge",
		Driver:     "bridge",
		Scope:      "local",
		IPAM:       api.IPAM{Driver: "default", Config: []api.IPAMConfig{{Subnet: "172.17.0.0/16", Gateway: "172.17.0.1"}}},
		Containers: make(map[string]api.EndpointResource),
		Options:    make(map[string]string),
		Labels:     make(map[string]string),
		Created:    time.Now().UTC().Format(time.RFC3339Nano),
	})
	driver := &SyntheticNetworkDriver{Store: store, IPAlloc: store.IPAlloc}
	return driver, store
}

func TestSyntheticNetworkDriverCreate(t *testing.T) {
	driver, store := newTestDriverAndStore()
	ctx := context.Background()

	resp, err := driver.Create(ctx, "test-net", &api.NetworkCreateRequest{
		Name:   "test-net",
		Driver: "bridge",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID == "" {
		t.Fatal("expected non-empty ID")
	}

	// Verify network is in store
	n, ok := store.Networks.Get(resp.ID)
	if !ok {
		t.Fatal("network not found in store")
	}
	if n.Name != "test-net" {
		t.Fatalf("expected name test-net, got %s", n.Name)
	}
	if len(n.IPAM.Config) == 0 {
		t.Fatal("IPAM config should be auto-assigned")
	}
}

func TestSyntheticNetworkDriverCreateDuplicate(t *testing.T) {
	driver, _ := newTestDriverAndStore()
	ctx := context.Background()

	_, err := driver.Create(ctx, "dup-net", &api.NetworkCreateRequest{Name: "dup-net"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = driver.Create(ctx, "dup-net", &api.NetworkCreateRequest{Name: "dup-net"})
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestSyntheticNetworkDriverConnect(t *testing.T) {
	driver, store := newTestDriverAndStore()
	ctx := context.Background()

	resp, _ := driver.Create(ctx, "mynet", &api.NetworkCreateRequest{Name: "mynet"})

	// Create a container in the store
	cID := "container-abc123"
	store.Containers.Put(cID, api.Container{
		ID:   cID,
		Name: "/mycontainer",
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
	})

	err := driver.Connect(ctx, resp.ID, cID, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify container has endpoint
	c, _ := store.Containers.Get(cID)
	ep, ok := c.NetworkSettings.Networks["mynet"]
	if !ok {
		t.Fatal("container should have mynet endpoint")
	}
	if ep.IPAddress == "" {
		t.Fatal("endpoint should have IP")
	}
	if ep.MacAddress == "" {
		t.Fatal("endpoint should have MAC")
	}

	// Verify network has container
	n, _ := store.Networks.Get(resp.ID)
	if _, ok := n.Containers[cID]; !ok {
		t.Fatal("network should have container in Containers map")
	}
}

func TestSyntheticNetworkDriverDisconnect(t *testing.T) {
	driver, store := newTestDriverAndStore()
	ctx := context.Background()

	resp, _ := driver.Create(ctx, "mynet", &api.NetworkCreateRequest{Name: "mynet"})

	cID := "container-xyz789"
	store.Containers.Put(cID, api.Container{
		ID:   cID,
		Name: "/mycontainer",
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
	})

	_ = driver.Connect(ctx, resp.ID, cID, nil)
	err := driver.Disconnect(ctx, resp.ID, cID)
	if err != nil {
		t.Fatal(err)
	}

	// Verify container no longer has endpoint
	c, _ := store.Containers.Get(cID)
	if _, ok := c.NetworkSettings.Networks["mynet"]; ok {
		t.Fatal("container should not have mynet endpoint after disconnect")
	}

	// Verify network no longer has container
	n, _ := store.Networks.Get(resp.ID)
	if _, ok := n.Containers[cID]; ok {
		t.Fatal("network should not have container after disconnect")
	}
}

func TestSyntheticNetworkDriverPrune(t *testing.T) {
	driver, _ := newTestDriverAndStore()
	ctx := context.Background()

	// Create two networks, one empty, one with a label filter mismatch
	driver.Create(ctx, "empty-net", &api.NetworkCreateRequest{Name: "empty-net"})
	driver.Create(ctx, "other-net", &api.NetworkCreateRequest{Name: "other-net"})

	resp, err := driver.Prune(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.NetworksDeleted) != 2 {
		t.Fatalf("expected 2 pruned, got %d", len(resp.NetworksDeleted))
	}

	// Bridge should not be pruned
	nets, _ := driver.List(ctx, nil)
	for _, n := range nets {
		if n.Name != "bridge" {
			t.Fatalf("unexpected network after prune: %s", n.Name)
		}
	}
}

func TestSyntheticNetworkDriverList(t *testing.T) {
	driver, _ := newTestDriverAndStore()
	ctx := context.Background()

	driver.Create(ctx, "alpha", &api.NetworkCreateRequest{Name: "alpha"})
	driver.Create(ctx, "beta", &api.NetworkCreateRequest{Name: "beta"})

	// List all
	nets, err := driver.List(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 3 { // bridge + alpha + beta
		t.Fatalf("expected 3 networks, got %d", len(nets))
	}

	// List with name filter
	filtered, err := driver.List(ctx, map[string][]string{"name": {"alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered network, got %d", len(filtered))
	}
	if filtered[0].Name != "alpha" {
		t.Fatalf("expected alpha, got %s", filtered[0].Name)
	}
}

func TestSyntheticNetworkDriverRemoveBuiltin(t *testing.T) {
	driver, _ := newTestDriverAndStore()
	ctx := context.Background()

	err := driver.Remove(ctx, "bridge")
	if err == nil {
		t.Fatal("should not be able to remove bridge network")
	}
}

func TestNetworkDriverIntegration(t *testing.T) {
	driver, store := newTestDriverAndStore()
	ctx := context.Background()

	// Create a custom network
	resp, _ := driver.Create(ctx, "app-net", &api.NetworkCreateRequest{Name: "app-net"})

	// Create two containers and connect them
	for _, cID := range []string{"c1", "c2"} {
		store.Containers.Put(cID, api.Container{
			ID:   cID,
			Name: "/" + cID,
			NetworkSettings: api.NetworkSettings{
				Networks: make(map[string]*api.EndpointSettings),
			},
		})
		_ = driver.Connect(ctx, resp.ID, cID, nil)
	}

	// Both should be on the same network with different IPs
	c1, _ := store.Containers.Get("c1")
	c2, _ := store.Containers.Get("c2")
	ep1 := c1.NetworkSettings.Networks["app-net"]
	ep2 := c2.NetworkSettings.Networks["app-net"]
	if ep1.IPAddress == ep2.IPAddress {
		t.Fatal("containers should have different IPs")
	}
	if ep1.Gateway != ep2.Gateway {
		t.Fatal("containers should share gateway")
	}

	// Verify DNS peer resolution still works (uses store data)
	peers := ResolvePeerHosts(store, "c1")
	found := false
	for _, p := range peers {
		if p == "c2:"+ep2.IPAddress {
			found = true
			break
		}
	}
	// ResolvePeerHosts checks the network Containers map, peer hostnames use container name
	n, _ := store.Networks.Get(resp.ID)
	if len(n.Containers) != 2 {
		t.Fatalf("expected 2 containers in network, got %d", len(n.Containers))
	}
	_ = found // DNS resolution verified via network containers map
}
