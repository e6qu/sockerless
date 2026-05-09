package core

import (
	"context"
	"strings"
	"testing"

	"github.com/sockerless/api"
)

func TestResolveNetworkDiscoveryDriver_NATDefault(t *testing.T) {
	d, err := ResolveNetworkDiscoveryDriver(api.NetworkDiscoveryNATGatewayOnly, nil)
	if err != nil {
		t.Fatalf("nat-gateway-only: unexpected err %v", err)
	}
	if d.Kind() != api.NetworkDiscoveryNATGatewayOnly {
		t.Errorf("Kind() = %q, want %q", d.Kind(), api.NetworkDiscoveryNATGatewayOnly)
	}

	// All methods on the no-op driver succeed without side effects.
	ctx := context.Background()
	if err := d.RegisterContainer(ctx, "net1", "peer", "cid-peer", nil); err != nil {
		t.Errorf("RegisterContainer: %v", err)
	}
	if err := d.DeregisterContainer(ctx, "net1", "peer", "cid-peer"); err != nil {
		t.Errorf("DeregisterContainer: %v", err)
	}
	if got, err := d.ResolveName(ctx, "net1", "peer"); err != nil || got != nil {
		t.Errorf("ResolveName: got %v, %v; want nil, nil", got, err)
	}
}

func TestResolveNetworkDiscoveryDriver_InvalidKind(t *testing.T) {
	_, err := ResolveNetworkDiscoveryDriver(api.NetworkDiscoveryKind(""), nil)
	if err == nil {
		t.Fatal("empty kind: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid kind") {
		t.Errorf("err = %v; want 'invalid kind' substring", err)
	}

	_, err = ResolveNetworkDiscoveryDriver(api.NetworkDiscoveryKind("garbage"), nil)
	if err == nil {
		t.Fatal("unknown kind: expected error, got nil")
	}
}

func TestResolveNetworkDiscoveryDriver_NotRegistered(t *testing.T) {
	// cloud-dns + service-mesh are valid but their constructors live
	// in per-cloud-common packages (gcp-common, aws-common, azure-common).
	// In this unit-test binary they aren't imported, so the registry
	// returns a clear "no constructor registered" error instead of
	// silently falling back.
	_, err := ResolveNetworkDiscoveryDriver(api.NetworkDiscoveryCloudDNS, nil)
	if err == nil {
		t.Fatal("unregistered constructor: expected error")
	}
	if !strings.Contains(err.Error(), "no constructor registered") {
		t.Errorf("err = %v; want 'no constructor registered'", err)
	}
}

func TestParseNetworkDiscoveryEnv(t *testing.T) {
	// Unset → backend default.
	got, err := ParseNetworkDiscoveryEnv("", api.NetworkDiscoveryServiceMesh)
	if err != nil {
		t.Fatalf("unset: unexpected err %v", err)
	}
	if got != api.NetworkDiscoveryServiceMesh {
		t.Errorf("unset: got %q, want %q", got, api.NetworkDiscoveryServiceMesh)
	}

	// Operator override valid.
	got, err = ParseNetworkDiscoveryEnv("cloud-dns", api.NetworkDiscoveryServiceMesh)
	if err != nil {
		t.Fatalf("override: unexpected err %v", err)
	}
	if got != api.NetworkDiscoveryCloudDNS {
		t.Errorf("override: got %q, want cloud-dns", got)
	}

	// Operator override invalid → error (no silent fallback).
	_, err = ParseNetworkDiscoveryEnv("garbage", api.NetworkDiscoveryServiceMesh)
	if err == nil {
		t.Fatal("invalid override: expected error")
	}

	// Whitespace trimmed.
	got, err = ParseNetworkDiscoveryEnv("  host-aliases  ", api.NetworkDiscoveryServiceMesh)
	if err != nil {
		t.Fatalf("trimmed: unexpected err %v", err)
	}
	if got != api.NetworkDiscoveryHostAliases {
		t.Errorf("trimmed: got %q, want host-aliases", got)
	}
}
