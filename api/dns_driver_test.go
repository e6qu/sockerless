package api

import "testing"

func TestDNSMechanism_IsValid(t *testing.T) {
	for _, m := range AllDNSMechanisms {
		if !m.IsValid() {
			t.Errorf("AllDNSMechanisms entry %q reports !IsValid", m)
		}
	}
	if DNSMechanism("").IsValid() {
		t.Errorf("empty string should not be valid")
	}
	if DNSMechanism("garbage").IsValid() {
		t.Errorf("unknown name should not be valid")
	}
}

func TestDNSMechanism_String(t *testing.T) {
	if got := DNSMechanismCloudMap.String(); got != "cloud-map" {
		t.Errorf("String() = %q, want cloud-map", got)
	}
}
