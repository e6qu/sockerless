package main

import (
	"strings"
	"testing"
)

// A workload namespace holds only its own interface, so the resolver its image
// was configured with — Docker's embedded 127.0.0.11, written before the pause
// container was detached from Docker's networks — answers nothing. Lookups then
// block until they time out rather than failing, which is why a workload can
// bind its ports minutes after its container started with nothing logged in
// between (GitHub issue #905). A VPC serves DNS at its own base address plus
// two, which is what AmazonProvidedDNS means.
func TestVPCResolverIsTheCIDRBasePlusTwo(t *testing.T) {
	for _, tc := range []struct{ cidr, want string }{
		{"10.0.0.0/16", "10.0.0.2"},
		{"172.31.0.0/16", "172.31.0.2"},
		{"192.168.100.0/24", "192.168.100.2"},
	} {
		if got := ec2VPCResolverIPv4(tc.cidr); got == nil || got.String() != tc.want {
			t.Errorf("resolver for %s = %v, want %s", tc.cidr, got, tc.want)
		}
	}
	// A gateway is base+1 and a resolver base+2; conflating them would send
	// every lookup to the router.
	if gw := ec2AWSSubnetGateway("10.0.0.0/16"); gw == nil || gw.String() == "10.0.0.2" {
		t.Errorf("the subnet gateway and the resolver are the same address: %v", gw)
	}
	if got := ec2VPCResolverIPv4("not-a-cidr"); got != nil {
		t.Errorf("an unparseable CIDR produced a resolver address: %v", got)
	}
}

// The resolver port is read from the address actually bound, because the
// simulator may take an ephemeral one.
func TestRoute53DNSPortReadsTheBoundAddress(t *testing.T) {
	previous := r53DNSAddr
	t.Cleanup(func() { r53DNSAddr = previous })

	r53DNSAddr = ""
	if _, err := route53DNSPort(); err == nil {
		t.Error("a resolver that is not listening reported a port")
	} else if !strings.Contains(err.Error(), "not listening") {
		t.Errorf("the error does not say the resolver is not listening: %v", err)
	}

	r53DNSAddr = "127.0.0.1:5353"
	port, err := route53DNSPort()
	if err != nil {
		t.Fatalf("read the bound port: %v", err)
	}
	if port != 5353 {
		t.Errorf("port = %d, want 5353", port)
	}

	r53DNSAddr = "[::]:41234"
	if port, err = route53DNSPort(); err != nil || port != 41234 {
		t.Errorf("an ephemeral port on the wildcard address read as (%d, %v)", port, err)
	}
}
