package realexec

import (
	"net"
	"sync"
)

var publicIPv4Pool = struct {
	sync.Mutex
	ipam *IPAM
}{
	ipam: mustPublicIPAM(),
}

func ReservePublicIPv4(owner string, requested net.IP) (net.IP, error) {
	publicIPv4Pool.Lock()
	defer publicIPv4Pool.Unlock()
	return publicIPv4Pool.ipam.Reserve(owner, requested)
}

func ReleasePublicIPv4(ip net.IP) {
	publicIPv4Pool.Lock()
	defer publicIPv4Pool.Unlock()
	publicIPv4Pool.ipam.Release(ip)
}

func mustPublicIPAM() *IPAM {
	ipam, err := NewIPAM("198.51.100.0/24", net.IPv4(198, 51, 100, 1))
	if err != nil {
		panic(err)
	}
	return ipam
}
