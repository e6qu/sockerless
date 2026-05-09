package core

import (
	"context"
	"strings"
	"testing"

	"github.com/sockerless/api"
)

func TestResolveDNSDriver_NoneDefault(t *testing.T) {
	d, err := ResolveDNSDriver(api.DNSMechanismNone, nil)
	if err != nil {
		t.Fatalf("none: unexpected err %v", err)
	}
	if d.Mechanism() != api.DNSMechanismNone {
		t.Errorf("Mechanism() = %q, want %q", d.Mechanism(), api.DNSMechanismNone)
	}
	got, err := d.SearchDomain(context.Background(), "net1")
	if err != nil || got != "" {
		t.Errorf("SearchDomain: got %q, %v; want empty, nil", got, err)
	}
}

func TestResolveDNSDriver_InvalidMechanism(t *testing.T) {
	_, err := ResolveDNSDriver(api.DNSMechanism(""), nil)
	if err == nil {
		t.Fatal("empty: expected error")
	}
	if !strings.Contains(err.Error(), "invalid mechanism") {
		t.Errorf("err = %v; want 'invalid mechanism'", err)
	}
	_, err = ResolveDNSDriver(api.DNSMechanism("garbage"), nil)
	if err == nil {
		t.Fatal("unknown: expected error")
	}
}

func TestResolveDNSDriver_NotRegistered(t *testing.T) {
	_, err := ResolveDNSDriver(api.DNSMechanismCloudMap, nil)
	if err == nil {
		t.Fatal("unregistered: expected error")
	}
	if !strings.Contains(err.Error(), "no constructor registered") {
		t.Errorf("err = %v; want 'no constructor registered'", err)
	}
}

func TestParseDNSMechanismEnv(t *testing.T) {
	got, err := ParseDNSMechanismEnv("", api.DNSMechanismCloudMap)
	if err != nil || got != api.DNSMechanismCloudMap {
		t.Errorf("unset: got %q, err %v; want cloud-map", got, err)
	}
	got, err = ParseDNSMechanismEnv("cloud-dns-zone", api.DNSMechanismCloudMap)
	if err != nil || got != api.DNSMechanismCloudDNSZone {
		t.Errorf("override: got %q, err %v", got, err)
	}
	_, err = ParseDNSMechanismEnv("garbage", api.DNSMechanismCloudMap)
	if err == nil {
		t.Fatal("invalid override: expected error")
	}
	got, err = ParseDNSMechanismEnv("  private-dns-zone  ", api.DNSMechanismNone)
	if err != nil || got != api.DNSMechanismPrivateDNSZone {
		t.Errorf("trimmed: got %q, err %v", got, err)
	}
}

func TestDNSSearchDomainEnvIfSet(t *testing.T) {
	if got := DNSSearchDomainEnvIfSet(nil, ""); got != "" {
		t.Errorf("empty suffix → got %q, want empty", got)
	}
	if got := DNSSearchDomainEnvIfSet(nil, "tf-net.local"); got != "SOCKERLESS_DNS_SEARCH_DOMAIN=tf-net.local" {
		t.Errorf("missing user env → got %q", got)
	}
	if got := DNSSearchDomainEnvIfSet([]string{"SOCKERLESS_DNS_SEARCH_DOMAIN=user.local"}, "tf-net.local"); got != "" {
		t.Errorf("user override → got %q, want empty", got)
	}
}
