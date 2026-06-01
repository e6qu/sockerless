package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"net"
	"net/http"
	"strings"
	"sync"

	sim "github.com/sockerless/simulator"
	realexec "github.com/sockerless/simulator-realexec"
)

var (
	gcpRealHost           = realexec.NewHost()
	gcpRealMu             sync.Mutex
	gcpRealNetworks       = map[string]*realexec.Network{}
	gcpRealSubnets        = map[string]*realexec.Subnet{}
	gcpRealSubnetNetworks = map[string]string{}
	gcpRealNICs           = map[string]*realexec.NamespaceNIC{}
)

func gcpRequireNetworkHost(w http.ResponseWriter) bool {
	if err := realexec.DetectNetworkCapabilities().Require(); err != nil {
		sim.GCPErrorf(w, http.StatusServiceUnavailable, "FAILED_PRECONDITION",
			"real Compute networking requires Linux network namespace, bridge, veth, route, and nftables host capabilities: %v", err)
		return false
	}
	return true
}

func gcpRealName(prefix, id string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	suffix := fmt.Sprintf("%08x", h.Sum32())
	if len(prefix) >= 15 {
		return prefix[:15]
	}
	return (prefix + suffix)[:min(15, len(prefix)+len(suffix))]
}

func gcpCreateRealNetwork(ctx context.Context, selfLink string) error {
	gcpRealMu.Lock()
	if _, ok := gcpRealNetworks[selfLink]; ok {
		gcpRealMu.Unlock()
		return nil
	}
	namespaceName := gcpRealName("gn", selfLink)
	network, err := gcpRealHost.CreateNetworkNamespace(ctx, namespaceName)
	if err != nil {
		gcpRealMu.Unlock()
		return err
	}
	gcpRealNetworks[selfLink] = network
	gcpRealMu.Unlock()
	return nil
}

func gcpDeleteRealNetwork(ctx context.Context, selfLink string) error {
	gcpRealMu.Lock()
	network := gcpRealNetworks[selfLink]
	delete(gcpRealNetworks, selfLink)
	for nicID, nic := range gcpRealNICs {
		if strings.Contains(nicID, selfLink) {
			_ = nic.Close(ctx)
			delete(gcpRealNICs, nicID)
		}
	}
	for subnetLink, subnet := range gcpRealSubnets {
		if gcpRealSubnetNetworks[subnetLink] == selfLink {
			_ = subnet.Close(ctx)
			delete(gcpRealSubnets, subnetLink)
			delete(gcpRealSubnetNetworks, subnetLink)
		}
	}
	gcpRealMu.Unlock()
	if network == nil {
		return nil
	}
	return network.Close(ctx)
}

func gcpCreateRealSubnetwork(ctx context.Context, subnet ComputeSubnetwork) error {
	gcpRealMu.Lock()
	if _, ok := gcpRealSubnets[subnet.SelfLink]; ok {
		gcpRealMu.Unlock()
		return nil
	}
	network := gcpRealNetworks[subnet.Network]
	gcpRealMu.Unlock()
	if network == nil {
		if err := gcpCreateRealNetwork(ctx, subnet.Network); err != nil {
			return err
		}
		gcpRealMu.Lock()
		if _, ok := gcpRealSubnets[subnet.SelfLink]; ok {
			gcpRealMu.Unlock()
			return nil
		}
		network = gcpRealNetworks[subnet.Network]
	} else {
		gcpRealMu.Lock()
	}
	realSubnet, err := network.CreateSubnet(ctx, realexec.SubnetSpec{
		Name:       subnet.SelfLink,
		BridgeName: gcpRealName("gs", subnet.SelfLink),
		CIDR:       subnet.IpCidrRange,
		Gateway:    net.ParseIP(subnet.GatewayAddress),
	})
	if err != nil {
		gcpRealMu.Unlock()
		return err
	}
	gcpRealSubnets[subnet.SelfLink] = realSubnet
	gcpRealSubnetNetworks[subnet.SelfLink] = subnet.Network
	gcpRealMu.Unlock()
	return nil
}

func gcpDeleteRealSubnetwork(ctx context.Context, selfLink string) error {
	gcpRealMu.Lock()
	subnet := gcpRealSubnets[selfLink]
	delete(gcpRealSubnets, selfLink)
	delete(gcpRealSubnetNetworks, selfLink)
	gcpRealMu.Unlock()
	if subnet == nil {
		return nil
	}
	return subnet.Close(ctx)
}

func gcpCreateRealNIC(ctx context.Context, nicID, subnetLink, requestedIP string) (string, error) {
	gcpRealMu.Lock()
	if existing, ok := gcpRealNICs[nicID]; ok {
		gcpRealMu.Unlock()
		return existing.PrivateIP.String(), nil
	}
	subnet := gcpRealSubnets[subnetLink]
	gcpRealMu.Unlock()
	if subnet == nil {
		sn, ok := gcpSubnetworks.Get(subnetLink)
		if !ok {
			return "", fmt.Errorf("subnetwork %s not found", subnetLink)
		}
		if err := gcpCreateRealSubnetwork(ctx, sn); err != nil {
			return "", err
		}
		gcpRealMu.Lock()
		if existing, ok := gcpRealNICs[nicID]; ok {
			gcpRealMu.Unlock()
			return existing.PrivateIP.String(), nil
		}
		subnet = gcpRealSubnets[subnetLink]
	} else {
		gcpRealMu.Lock()
	}
	privateIP := net.ParseIP(requestedIP)
	if requestedIP == "" {
		privateIP = nil
	}
	nic, err := subnet.AttachNamespaceNIC(ctx, realexec.NamespaceNICSpec{
		NamespaceName: gcpRealName("gi", nicID),
		HostVethName:  gcpRealName("gh", nicID),
		GuestVethName: gcpRealName("gg", nicID),
		PrivateIP:     privateIP,
		MAC:           gcpNICMAC(nicID),
	})
	if err != nil {
		gcpRealMu.Unlock()
		return "", err
	}
	gcpRealNICs[nicID] = nic
	gcpRealMu.Unlock()
	return nic.PrivateIP.String(), nil
}

func gcpDeleteRealNIC(ctx context.Context, nicID string) error {
	gcpRealMu.Lock()
	nic := gcpRealNICs[nicID]
	delete(gcpRealNICs, nicID)
	gcpRealMu.Unlock()
	if nic == nil {
		return nil
	}
	return nic.Close(ctx)
}

func gcpConfigureRealRouterNAT(ctx context.Context, router ComputeRouter) error {
	gcpRealMu.Lock()
	network := gcpRealNetworks[router.Network]
	gcpRealMu.Unlock()
	if network == nil {
		if err := gcpCreateRealNetwork(ctx, router.Network); err != nil {
			return err
		}
		gcpRealMu.Lock()
		network = gcpRealNetworks[router.Network]
		gcpRealMu.Unlock()
	}
	for _, nat := range router.Nats {
		publicIP := net.IP(nil)
		for _, ref := range nat.NatIps {
			if addr, ok := gcpComputeAddressByRef(ref); ok {
				publicIP = net.ParseIP(addr.Address)
				break
			}
		}
		if publicIP == nil {
			ip, err := realexec.ReserveGCPPublicIPv4(router.SelfLink+"/"+nat.Name, nil)
			if err != nil {
				return err
			}
			publicIP = ip
		}
		sources := gcpNATSourceCIDRs(router.Network, nat)
		for _, cidr := range sources {
			if err := network.ConfigureSNAT(ctx, cidr, publicIP, gcpRealName("gsn", router.SelfLink+nat.Name+cidr)); err != nil {
				return err
			}
		}
	}
	return nil
}

func gcpComputeAddressByRef(ref string) (ComputeAddress, bool) {
	for _, addr := range gcpAddresses.List() {
		if ref == addr.SelfLink || strings.HasSuffix(ref, "/"+addr.SelfLink) || ref == addr.Name {
			return addr, true
		}
	}
	return ComputeAddress{}, false
}

func gcpNATSourceCIDRs(networkLink string, nat ComputeRouterNAT) []string {
	if strings.EqualFold(nat.SourceSubnetworkIpRangesToNat, "ALL_SUBNETWORKS_ALL_IP_RANGES") {
		var cidrs []string
		for _, sn := range gcpSubnetworks.List() {
			if sn.Network == networkLink {
				cidrs = append(cidrs, sn.IpCidrRange)
			}
		}
		return cidrs
	}
	var cidrs []string
	for _, snRef := range nat.Subnetworks {
		link := snRef.Name
		if sn, ok := gcpSubnetworks.Get(link); ok {
			cidrs = append(cidrs, sn.IpCidrRange)
		}
	}
	return cidrs
}

func gcpSubnetGateway(cidr string) string {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil || ip.To4() == nil {
		return ""
	}
	out := append(net.IP(nil), ip.To4()...)
	out[3]++
	return out.String()
}

func gcpNICMAC(nicID string) string {
	id := strings.NewReplacer("/", "", "-", "", "_", "").Replace(nicID)
	var b [3]byte
	for i := range id {
		b[i%3] ^= id[i]
	}
	return fmt.Sprintf("02:42:ac:%02x:%02x:%02x", b[0], b[1], b[2])
}
