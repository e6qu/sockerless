package api

import "testing"

func TestNetworkDiscoveryKind_IsValid(t *testing.T) {
	for _, k := range AllNetworkDiscoveryKinds {
		if !k.IsValid() {
			t.Errorf("AllNetworkDiscoveryKinds entry %q reports !IsValid", k)
		}
	}
	if NetworkDiscoveryKind("").IsValid() {
		t.Errorf("empty string should not be valid")
	}
	if NetworkDiscoveryKind("garbage").IsValid() {
		t.Errorf("unknown name should not be valid")
	}
}

func TestNetworkDiscoveryKind_String(t *testing.T) {
	if got := NetworkDiscoveryHostAliases.String(); got != "host-aliases" {
		t.Errorf("String() = %q, want host-aliases", got)
	}
}
