package realexec

import (
	"net"
	"sync"
)

var publicIPv4Pool = struct {
	sync.Mutex
	ipams map[string]*IPAM
}{
	ipams: mustPublicIPAMs(),
}

func ReservePublicIPv4(owner string, requested net.IP) (net.IP, error) {
	return reservePublicIPv4("default", owner, requested)
}

func ReserveAWSPublicIPv4(owner string, requested net.IP) (net.IP, error) {
	return reservePublicIPv4("aws", owner, requested)
}

func ReserveGCPPublicIPv4(owner string, requested net.IP) (net.IP, error) {
	return reservePublicIPv4("gcp", owner, requested)
}

func ReserveAzurePublicIPv4(owner string, requested net.IP) (net.IP, error) {
	return reservePublicIPv4("azure", owner, requested)
}

func reservePublicIPv4(pool, owner string, requested net.IP) (net.IP, error) {
	publicIPv4Pool.Lock()
	defer publicIPv4Pool.Unlock()
	return publicIPv4Pool.ipams[pool].Reserve(owner, requested)
}

func ReleasePublicIPv4(ip net.IP) {
	publicIPv4Pool.Lock()
	defer publicIPv4Pool.Unlock()
	for _, ipam := range publicIPv4Pool.ipams {
		ipam.Release(ip)
	}
}

func mustPublicIPAMs() map[string]*IPAM {
	specs := map[string]struct {
		cidr    string
		gateway net.IP
	}{
		"default": {"8.34.210.0/24", net.IPv4(8, 34, 210, 1)},
		"aws":     {"3.2.0.0/24", net.IPv4(3, 2, 0, 1)},
		"gcp":     {"8.34.210.0/24", net.IPv4(8, 34, 210, 1)},
		"azure":   {"13.82.0.0/24", net.IPv4(13, 82, 0, 1)},
	}
	ipams := make(map[string]*IPAM, len(specs))
	for name, spec := range specs {
		ipam, err := NewIPAM(spec.cidr, spec.gateway)
		if err != nil {
			panic(err)
		}
		ipams[name] = ipam
	}
	return ipams
}
