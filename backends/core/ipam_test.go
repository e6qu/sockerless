package core

import (
	"strings"
	"testing"

	"github.com/sockerless/api"
)

func TestIPAllocatorSubnetUniqueness(t *testing.T) {
	alloc := NewIPAllocator()
	seen := make(map[string]bool)
	for i := 0; i < 5; i++ {
		cfg := alloc.AllocateSubnet("net-"+string(rune('a'+i)), nil)
		if seen[cfg.Subnet] {
			t.Fatalf("duplicate subnet: %s", cfg.Subnet)
		}
		seen[cfg.Subnet] = true
		if cfg.Gateway == "" {
			t.Fatal("gateway should not be empty")
		}
	}
}

func TestIPAllocatorSequentialIPs(t *testing.T) {
	alloc := NewIPAllocator()
	alloc.AllocateSubnet("net1", nil)

	var ips []string
	for i := 0; i < 5; i++ {
		ip, prefixLen, gw, mac := alloc.AllocateIP("net1")
		if ip == "" || gw == "" || mac == "" {
			t.Fatalf("empty field: ip=%s gw=%s mac=%s", ip, gw, mac)
		}
		if prefixLen != 16 {
			t.Fatalf("expected prefix 16, got %d", prefixLen)
		}
		ips = append(ips, ip)
	}
	// IPs should be sequential from .2
	if !strings.HasSuffix(ips[0], ".2") {
		t.Fatalf("first IP should end in .2, got %s", ips[0])
	}
	if !strings.HasSuffix(ips[4], ".6") {
		t.Fatalf("fifth IP should end in .6, got %s", ips[4])
	}
}

func TestIPAllocatorRelease(t *testing.T) {
	alloc := NewIPAllocator()
	alloc.AllocateSubnet("net1", nil)

	ip1, _, _, _ := alloc.AllocateIP("net1")
	ip2, _, _, _ := alloc.AllocateIP("net1")
	_ = ip2

	// Release ip1, next allocation should reuse it
	alloc.ReleaseIP("net1", ip1)
	ip3, _, _, _ := alloc.AllocateIP("net1")
	if ip3 != ip1 {
		t.Fatalf("expected reuse of %s, got %s", ip1, ip3)
	}
}

func TestIPAllocatorCustomSubnet(t *testing.T) {
	alloc := NewIPAllocator()
	cfg := alloc.AllocateSubnet("custom", &api.IPAMConfig{
		Subnet:  "10.5.0.0/24",
		Gateway: "10.5.0.1",
	})
	if cfg.Subnet != "10.5.0.0/24" {
		t.Fatalf("expected custom subnet, got %s", cfg.Subnet)
	}
	if cfg.Gateway != "10.5.0.1" {
		t.Fatalf("expected custom gateway, got %s", cfg.Gateway)
	}
	ip, prefixLen, gw, _ := alloc.AllocateIP("custom")
	if !strings.HasPrefix(ip, "10.5.0.") {
		t.Fatalf("IP should be in 10.5.0.x, got %s", ip)
	}
	if prefixLen != 24 {
		t.Fatalf("expected prefix 24, got %d", prefixLen)
	}
	if gw != "10.5.0.1" {
		t.Fatalf("expected gateway 10.5.0.1, got %s", gw)
	}
}

func TestIPAllocatorMACGeneration(t *testing.T) {
	mac := macFromIP("172.18.0.2")
	// 18 = 0x12, 0 = 0x00, 2 = 0x02
	if mac != "02:42:ac:12:00:02" {
		t.Fatalf("expected 02:42:ac:12:00:02, got %s", mac)
	}

	mac2 := macFromIP("172.20.1.100")
	// 20 = 0x14, 1 = 0x01, 100 = 0x64
	if mac2 != "02:42:ac:14:01:64" {
		t.Fatalf("expected 02:42:ac:14:01:64, got %s", mac2)
	}
}

func TestIPAllocatorReleaseSubnet(t *testing.T) {
	alloc := NewIPAllocator()
	alloc.AllocateSubnet("net1", nil)
	alloc.AllocateIP("net1")

	alloc.ReleaseSubnet("net1")

	// After release, AllocateIP should return fallback
	ip, _, _, _ := alloc.AllocateIP("net1")
	if ip != "172.17.0.2" {
		t.Fatalf("expected fallback IP after subnet release, got %s", ip)
	}
}
