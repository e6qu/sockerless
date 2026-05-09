//go:build linux

package core

import (
	"os"
	"testing"
)

func TestNetnsManagerAvailableWithoutRoot(t *testing.T) {
	if os.Getuid() == 0 {
		// Mutually-exclusive perm gate with TestNetnsManagerCreateDelete:
		// this asserts the non-root path, that one asserts the root
		// path. CI must run one or the other; t.Skip documents which
		// half this run exercised. Not a config-fallback skip.
		t.Skip("non-root assertion: this run is root → exercised by TestNetnsManagerCreateDelete")
	}
	m := NewNetnsManager()
	if m.Available() {
		t.Error("should report unavailable without root")
	}
}

func TestNetnsManagerCreateDelete(t *testing.T) {
	if os.Getuid() != 0 {
		// Mutually-exclusive perm gate with TestNetnsManagerAvailableWithoutRoot.
		// Network-namespace creation needs CAP_NET_ADMIN; non-root runs
		// can't exercise it. Not a config-fallback skip.
		t.Skip("root/CAP_NET_ADMIN assertion: this run is non-root → exercised by TestNetnsManagerAvailableWithoutRoot")
	}
	m := NewNetnsManager()

	err := m.CreateNamespace("test-net-001", "test", "10.0.0.1", "10.0.0.0/24")
	if err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}

	// Verify namespace file exists
	if _, err := os.Stat("/var/run/netns/sockerless-test-net-001"); os.IsNotExist(err) {
		t.Error("namespace file should exist")
	}

	err = m.DeleteNamespace("test-net-001")
	if err != nil {
		t.Fatalf("DeleteNamespace failed: %v", err)
	}

	// Verify cleanup
	if _, err := os.Stat("/var/run/netns/sockerless-test-net-001"); !os.IsNotExist(err) {
		t.Error("namespace file should be cleaned up")
	}
}

func TestNetnsManagerVethPair(t *testing.T) {
	if os.Getuid() != 0 {
		// CAP_NET_ADMIN required for veth pair creation. Non-root runs
		// can't exercise this path. Mutually-exclusive with the
		// non-root-only assertion above.
		t.Skip("root/CAP_NET_ADMIN assertion: non-root run cannot exercise veth pair")
	}
	m := NewNetnsManager()

	err := m.CreateNamespace("veth-test-net", "vethtest", "10.1.0.1", "10.1.0.0/24")
	if err != nil {
		t.Fatalf("CreateNamespace failed: %v", err)
	}
	defer m.DeleteNamespace("veth-test-net")

	err = m.CreateVethPair("veth-test-net", "container-01", "10.1.0.2")
	if err != nil {
		t.Fatalf("CreateVethPair failed: %v", err)
	}

	err = m.RemoveVethPair("veth-test-net", "container-01")
	if err != nil {
		t.Fatalf("RemoveVethPair failed: %v", err)
	}
}
