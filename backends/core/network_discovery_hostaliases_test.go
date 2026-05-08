package core

import (
	"context"
	"testing"

	"github.com/sockerless/api"
)

func TestHostAliasesDiscovery_RegisterResolveDeregister(t *testing.T) {
	d := NewHostAliasesDiscovery()
	ctx := context.Background()

	ep1 := &CloudEndpoint{IPAddress: "10.0.0.1", Metadata: map[string]string{"fqdn": "alpha.local"}}
	ep2 := &CloudEndpoint{IPAddress: "10.0.0.2", Metadata: map[string]string{"fqdn": "beta.local"}}

	if err := d.RegisterContainer(ctx, "net1", "alpha", "cid-alpha", ep1); err != nil {
		t.Fatalf("register alpha: %v", err)
	}
	if err := d.RegisterContainer(ctx, "net1", "beta", "cid-beta", ep2); err != nil {
		t.Fatalf("register beta: %v", err)
	}

	got, err := d.ResolveName(ctx, "net1", "alpha")
	if err != nil || got == nil || got.IPAddress != "10.0.0.1" {
		t.Errorf("resolve alpha: got %+v, err %v", got, err)
	}

	got, err = d.ResolveName(ctx, "net1", "beta")
	if err != nil || got == nil || got.IPAddress != "10.0.0.2" {
		t.Errorf("resolve beta: got %+v, err %v", got, err)
	}

	got, err = d.ResolveName(ctx, "net1", "unknown")
	if err != nil || got != nil {
		t.Errorf("resolve unknown: got %+v, err %v; want nil, nil", got, err)
	}

	got, err = d.ResolveName(ctx, "net2", "alpha")
	if err != nil || got != nil {
		t.Errorf("resolve cross-network: got %+v, err %v; want nil, nil", got, err)
	}

	if err := d.DeregisterContainer(ctx, "net1", "alpha", "cid-alpha"); err != nil {
		t.Fatalf("deregister: %v", err)
	}
	got, _ = d.ResolveName(ctx, "net1", "alpha")
	if got != nil {
		t.Errorf("after deregister: got %+v, want nil", got)
	}

	// Idempotent: deregister twice succeeds.
	if err := d.DeregisterContainer(ctx, "net1", "alpha", "cid-alpha"); err != nil {
		t.Errorf("deregister twice: %v", err)
	}
}

func TestHostAliasesDiscovery_PeersOnNetwork(t *testing.T) {
	d := NewHostAliasesDiscovery()
	ctx := context.Background()
	d.RegisterContainer(ctx, "net1", "alpha", "cid-1", &CloudEndpoint{IPAddress: "10.0.0.1"})
	d.RegisterContainer(ctx, "net1", "beta", "cid-2", &CloudEndpoint{IPAddress: "10.0.0.2"})
	d.RegisterContainer(ctx, "net2", "gamma", "cid-3", &CloudEndpoint{IPAddress: "10.0.0.3"})

	peers := d.PeersOnNetwork("net1")
	if len(peers) != 2 {
		t.Errorf("net1 peers: got %d, want 2", len(peers))
	}
	if _, ok := peers["alpha"]; !ok {
		t.Errorf("net1 missing alpha")
	}

	// Snapshot: caller mutation must not affect internal state.
	delete(peers, "alpha")
	again := d.PeersOnNetwork("net1")
	if _, ok := again["alpha"]; !ok {
		t.Errorf("snapshot leaked: alpha disappeared after caller mutated copy")
	}

	if got := d.PeersOnNetwork("nonexistent"); len(got) != 0 {
		t.Errorf("nonexistent network: got %d peers, want 0", len(got))
	}
}

func TestHostAliasesDiscovery_RegistryConstructor(t *testing.T) {
	d, err := ResolveNetworkDiscoveryDriver(api.NetworkDiscoveryHostAliases, nil)
	if err != nil {
		t.Fatalf("resolve from registry: %v", err)
	}
	if d.Kind() != api.NetworkDiscoveryHostAliases {
		t.Errorf("Kind() = %q, want host-aliases", d.Kind())
	}
}
