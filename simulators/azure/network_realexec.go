package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	sim "github.com/sockerless/simulator"
	realexec "github.com/sockerless/simulator-realexec"
)

var (
	azureRealHost    = realexec.NewHost()
	azureRealMu      sync.Mutex
	azureRealVnets   = map[string]*realexec.Network{}
	azureRealSubnets = map[string]*realexec.Subnet{}
	azureRealNICs    = map[string]*realexec.NamespaceNIC{}
	azureRealNatIPs  = map[string]net.IP{}
)

func azureRequireNetworkHost(w http.ResponseWriter) bool {
	if err := realexec.DetectNetworkCapabilities().Require(); err != nil {
		sim.AzureErrorf(w, "OperationNotAllowed", http.StatusServiceUnavailable,
			"real Azure networking requires Linux network namespace, bridge, veth, route, and nftables host capabilities: %v", err)
		return false
	}
	return true
}

func azureRealName(prefix, id string) string {
	id = strings.NewReplacer("/", "", "-", "", "_", "").Replace(id)
	if len(id) > 10 {
		id = id[len(id)-10:]
	}
	name := prefix + id
	if len(name) > 15 {
		return name[:15]
	}
	return name
}

func azureCreateRealVnet(ctx context.Context, vnet VirtualNetwork) error {
	azureRealMu.Lock()
	if _, ok := azureRealVnets[vnet.ID]; ok {
		azureRealMu.Unlock()
		return nil
	}
	azureRealMu.Unlock()
	network, err := azureRealHost.CreateNetworkNamespace(ctx, azureRealName("zn", vnet.ID))
	if err != nil {
		return err
	}
	azureRealMu.Lock()
	azureRealVnets[vnet.ID] = network
	azureRealMu.Unlock()
	return nil
}

func azureDeleteRealVnet(ctx context.Context, vnetID string) error {
	azureRealMu.Lock()
	network := azureRealVnets[vnetID]
	delete(azureRealVnets, vnetID)
	for subnetID, subnet := range azureRealSubnets {
		if strings.HasPrefix(subnetID, vnetID+"/subnets/") {
			_ = subnet.Close(ctx)
			delete(azureRealSubnets, subnetID)
		}
	}
	azureRealMu.Unlock()
	if network == nil {
		return nil
	}
	return network.Close(ctx)
}

func azureCreateRealSubnet(ctx context.Context, subnet Subnet) error {
	vnetID := strings.Split(subnet.ID, "/subnets/")[0]
	azureRealMu.Lock()
	if _, ok := azureRealSubnets[subnet.ID]; ok {
		azureRealMu.Unlock()
		return nil
	}
	network := azureRealVnets[vnetID]
	azureRealMu.Unlock()
	if network == nil {
		vnet, ok := azureVnets.Get(vnetID)
		if !ok {
			return fmt.Errorf("virtual network %s not found", vnetID)
		}
		if err := azureCreateRealVnet(ctx, vnet); err != nil {
			return err
		}
		azureRealMu.Lock()
		network = azureRealVnets[vnetID]
		azureRealMu.Unlock()
	}
	realSubnet, err := network.CreateSubnet(ctx, realexec.SubnetSpec{
		Name:       subnet.ID,
		BridgeName: azureRealName("zs", subnet.ID),
		CIDR:       subnet.Properties.AddressPrefix,
		Gateway:    azureSubnetGateway(subnet.Properties.AddressPrefix),
	})
	if err != nil {
		return err
	}
	azureRealMu.Lock()
	azureRealSubnets[subnet.ID] = realSubnet
	azureRealMu.Unlock()
	if subnet.Properties.NatGateway != nil {
		if err := azureConfigureRealNATGatewayForSubnet(ctx, subnet); err != nil {
			return err
		}
	}
	return nil
}

func azureDeleteRealSubnet(ctx context.Context, subnetID string) error {
	azureRealMu.Lock()
	subnet := azureRealSubnets[subnetID]
	delete(azureRealSubnets, subnetID)
	azureRealMu.Unlock()
	if subnet == nil {
		return nil
	}
	return subnet.Close(ctx)
}

func azureCreateRealNIC(ctx context.Context, nicID, subnetID, requestedIP, mac string) (string, string, error) {
	azureRealMu.Lock()
	if existing, ok := azureRealNICs[nicID]; ok {
		azureRealMu.Unlock()
		return existing.PrivateIP.String(), mac, nil
	}
	subnet := azureRealSubnets[subnetID]
	azureRealMu.Unlock()
	if subnet == nil {
		sn, ok := azureSubnets.Get(subnetID)
		if !ok {
			return "", "", fmt.Errorf("subnet %s not found", subnetID)
		}
		if err := azureCreateRealSubnet(ctx, sn); err != nil {
			return "", "", err
		}
		azureRealMu.Lock()
		subnet = azureRealSubnets[subnetID]
		azureRealMu.Unlock()
	}
	privateIP := net.ParseIP(requestedIP)
	if requestedIP == "" {
		privateIP = nil
	}
	nic, err := subnet.AttachNamespaceNIC(ctx, realexec.NamespaceNICSpec{
		NamespaceName: azureRealName("zi", nicID),
		HostVethName:  azureRealName("zh", nicID),
		GuestVethName: azureRealName("zg", nicID),
		MAC:           mac,
		PrivateIP:     privateIP,
	})
	if err != nil {
		return "", "", err
	}
	azureRealMu.Lock()
	azureRealNICs[nicID] = nic
	azureRealMu.Unlock()
	return nic.PrivateIP.String(), formatAzureMAC(mac), nil
}

func azureDeleteRealNIC(ctx context.Context, nicID string) error {
	azureRealMu.Lock()
	nic := azureRealNICs[nicID]
	delete(azureRealNICs, nicID)
	azureRealMu.Unlock()
	if nic == nil {
		return nil
	}
	return nic.Close(ctx)
}

func azureConfigureRealNATGatewayForSubnet(ctx context.Context, subnet Subnet) error {
	if subnet.Properties.NatGateway == nil {
		return nil
	}
	gw, ok := azureNatGateways.Get(subnet.Properties.NatGateway.ID)
	if !ok {
		return fmt.Errorf("NAT gateway %s not found", subnet.Properties.NatGateway.ID)
	}
	publicIP := net.IP(nil)
	if len(gw.Properties.PublicIPAddresses) > 0 {
		pip, ok := azurePublicIPs.Get(gw.Properties.PublicIPAddresses[0].ID)
		if !ok {
			return fmt.Errorf("public IP address %s not found", gw.Properties.PublicIPAddresses[0].ID)
		}
		publicIP = net.ParseIP(pip.Properties.PublicIPAddress)
		if publicIP == nil {
			return fmt.Errorf("public IP address %s has no IPv4 lease", pip.ID)
		}
	} else if len(gw.Properties.PublicIPPrefixes) > 0 {
		ip, err := realexec.ReservePublicIPv4(gw.ID, nil)
		if err != nil {
			return err
		}
		publicIP = ip
		azureRealMu.Lock()
		azureRealNatIPs[gw.ID] = ip
		azureRealMu.Unlock()
	} else {
		return fmt.Errorf("NAT gateway %s has no public IP address or prefix", gw.ID)
	}
	vnetID := strings.Split(subnet.ID, "/subnets/")[0]
	azureRealMu.Lock()
	network := azureRealVnets[vnetID]
	azureRealMu.Unlock()
	if network == nil {
		vnet, ok := azureVnets.Get(vnetID)
		if !ok {
			return fmt.Errorf("virtual network %s not found", vnetID)
		}
		if err := azureCreateRealVnet(ctx, vnet); err != nil {
			return err
		}
		azureRealMu.Lock()
		network = azureRealVnets[vnetID]
		azureRealMu.Unlock()
	}
	return network.ConfigureSNAT(ctx, subnet.Properties.AddressPrefix, publicIP, azureRealName("zsn", subnet.ID))
}

func azureDeleteRealNATGateway(natID string) {
	azureRealMu.Lock()
	ip := azureRealNatIPs[natID]
	delete(azureRealNatIPs, natID)
	azureRealMu.Unlock()
	realexec.ReleasePublicIPv4(ip)
}

func azureSubnetGateway(cidr string) net.IP {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil || ip.To4() == nil {
		return nil
	}
	out := append(net.IP(nil), ip.To4()...)
	out[3]++
	return out
}

func azureNICMAC(nicID string) string {
	id := strings.NewReplacer("/", "", "-", "", "_", "").Replace(nicID)
	var b [3]byte
	for i := range id {
		b[i%3] ^= id[i]
	}
	return fmt.Sprintf("02:15:5d:%02x:%02x:%02x", b[0], b[1], b[2])
}

func formatAzureMAC(mac string) string {
	return strings.ToUpper(strings.ReplaceAll(mac, ":", "-"))
}
