package core

import (
	"fmt"
	"strings"
	"sync"

	"github.com/sockerless/api"
)

// IPAllocator manages IP address allocation for networks.
// It replaces the fragile inline IP math that was spread across handlers.
type IPAllocator struct {
	mu       sync.Mutex
	subnets  map[string]*subnetState // networkID → state
	nextByte int                     // next third-octet for auto-subnet (starts at 18)
}

type subnetState struct {
	base     string // e.g. "172.20.0."
	gateway  string // e.g. "172.20.0.1"
	mask     int    // e.g. 16
	nextHost int    // next host to allocate (starts at 2)
	released []int  // released host numbers for reuse
}

// NewIPAllocator creates a new IP allocator.
func NewIPAllocator() *IPAllocator {
	return &IPAllocator{
		subnets:  make(map[string]*subnetState),
		nextByte: 18,
	}
}

// AllocateSubnet assigns a subnet to a network. If requested config is provided,
// it uses that; otherwise auto-assigns from the pool.
func (a *IPAllocator) AllocateSubnet(networkID string, requested *api.IPAMConfig) api.IPAMConfig {
	a.mu.Lock()
	defer a.mu.Unlock()

	if requested != nil && requested.Subnet != "" {
		// Parse user-provided subnet
		gateway := requested.Gateway
		base := gateway[:strings.LastIndex(gateway, ".")+1]
		mask := 16
		if idx := strings.Index(requested.Subnet, "/"); idx >= 0 {
			fmt.Sscanf(requested.Subnet[idx+1:], "%d", &mask)
		}
		a.subnets[networkID] = &subnetState{
			base:     base,
			gateway:  gateway,
			mask:     mask,
			nextHost: 2,
		}
		return *requested
	}

	// Auto-assign subnet
	octet := a.nextByte
	a.nextByte++
	subnet := fmt.Sprintf("172.%d.0.0/16", octet)
	gateway := fmt.Sprintf("172.%d.0.1", octet)
	base := fmt.Sprintf("172.%d.0.", octet)

	a.subnets[networkID] = &subnetState{
		base:     base,
		gateway:  gateway,
		mask:     16,
		nextHost: 2,
	}

	return api.IPAMConfig{Subnet: subnet, Gateway: gateway}
}

// AllocateIP allocates an IP address within a network's subnet.
// Returns the IP, prefix length, gateway, and a MAC address derived from the IP.
func (a *IPAllocator) AllocateIP(networkID string) (ip string, prefixLen int, gateway string, mac string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	state, ok := a.subnets[networkID]
	if !ok {
		// Fallback for unknown networks (e.g. bridge initialized before allocator)
		return "172.17.0.2", 16, "172.17.0.1", "02:42:ac:11:00:02"
	}

	var host int
	if len(state.released) > 0 {
		host = state.released[len(state.released)-1]
		state.released = state.released[:len(state.released)-1]
	} else {
		host = state.nextHost
		state.nextHost++
	}

	ip = fmt.Sprintf("%s%d", state.base, host)
	mac = macFromIP(ip)
	return ip, state.mask, state.gateway, mac
}

// ReleaseIP returns an IP address to the pool for reuse.
func (a *IPAllocator) ReleaseIP(networkID, ip string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	state, ok := a.subnets[networkID]
	if !ok {
		return
	}

	// Extract host number from IP
	base := state.base
	if strings.HasPrefix(ip, base) {
		var host int
		fmt.Sscanf(ip[len(base):], "%d", &host)
		if host >= 2 {
			state.released = append(state.released, host)
		}
	}
}

// ReleaseSubnet removes a network's subnet allocation entirely.
func (a *IPAllocator) ReleaseSubnet(networkID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.subnets, networkID)
}

// macFromIP generates a MAC address from IP octets, matching Docker's convention.
// Format: 02:42:ac:XX:YY:ZZ where XX:YY:ZZ are the last 3 IP octets in hex.
func macFromIP(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return "02:42:ac:11:00:02"
	}
	var b, c, d int
	fmt.Sscanf(parts[1], "%d", &b)
	fmt.Sscanf(parts[2], "%d", &c)
	fmt.Sscanf(parts[3], "%d", &d)
	return fmt.Sprintf("02:42:ac:%02x:%02x:%02x", b, c, d)
}
