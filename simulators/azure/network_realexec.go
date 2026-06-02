package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
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

func azureReapplyRealNSGs(ctx context.Context) error {
	if azureNICs == nil {
		return nil
	}
	for _, nic := range azureNICs.List() {
		if err := azureApplyRealNSGsToNIC(ctx, nic); err != nil {
			return err
		}
	}
	return nil
}

func azureApplyRealNSGsToNIC(ctx context.Context, armNIC NetworkInterface) error {
	azureRealMu.Lock()
	nic := azureRealNICs[armNIC.ID]
	azureRealMu.Unlock()
	if nic == nil {
		return nil
	}
	rules, filtered := azureIngressPacketRules(armNIC)
	if !filtered {
		return nic.ClearIngressFilter(ctx)
	}
	if err := nic.ConfigureIngressFilter(ctx, rules); err != nil {
		return fmt.Errorf("configure NSG on %s: %w", armNIC.ID, err)
	}
	return nil
}

func azureIngressPacketRules(nic NetworkInterface) ([]realexec.PacketRule, bool) {
	nsgs := azureAttachedNSGs(nic)
	if len(nsgs) == 0 {
		return nil, false
	}
	var rules []realexec.PacketRule
	for _, nsg := range nsgs {
		securityRules := append([]SecurityRule(nil), nsg.Properties.SecurityRules...)
		sort.SliceStable(securityRules, func(i, j int) bool {
			if securityRules[i].Properties.Priority == securityRules[j].Properties.Priority {
				return securityRules[i].Name < securityRules[j].Name
			}
			return securityRules[i].Properties.Priority < securityRules[j].Properties.Priority
		})
		for _, rule := range securityRules {
			props := rule.Properties
			if !strings.EqualFold(defaultString(props.Direction, "Inbound"), "Inbound") {
				continue
			}
			verdict := "drop"
			if strings.EqualFold(props.Access, "Allow") {
				verdict = "accept"
			}
			rules = append(rules, azurePacketRulesForSecurityRule(props, verdict)...)
		}
		for _, cidr := range azureNICVNetCIDRs(nic) {
			rules = append(rules, realexec.PacketRule{Protocol: "*", SourceCIDR: cidr, Action: "accept"})
		}
	}
	return rules, true
}

func azureAttachedNSGs(nic NetworkInterface) []NetworkSecurityGroup {
	seen := map[string]bool{}
	var out []NetworkSecurityGroup
	add := func(id string) {
		if id == "" || seen[id] || azureNSGs == nil {
			return
		}
		seen[id] = true
		if nsg, ok := azureNSGs.Get(id); ok {
			out = append(out, nsg)
		}
	}
	if nic.Properties.NetworkSecurityGroup != nil {
		add(nic.Properties.NetworkSecurityGroup.ID)
	}
	for _, ipcfg := range nic.Properties.IPConfigurations {
		if ipcfg.Properties.Subnet == nil || azureSubnets == nil {
			continue
		}
		if subnet, ok := azureSubnets.Get(ipcfg.Properties.Subnet.ID); ok && subnet.Properties.NetworkSecurityGroup != nil {
			add(subnet.Properties.NetworkSecurityGroup.ID)
		}
	}
	return out
}

func azurePacketRulesForSecurityRule(props SecurityRuleProperties, verdict string) []realexec.PacketRule {
	sources := azureAddressPrefixes(props.SourceAddressPrefix, props.SourceAddressPrefixes)
	ports := azurePortRanges(props.DestinationPortRange, props.DestinationPortRanges)
	var rules []realexec.PacketRule
	for _, source := range sources {
		for _, port := range ports {
			from, to := azureParsePortRange(port)
			rules = append(rules, realexec.PacketRule{
				Protocol:   azurePacketProtocol(props.Protocol),
				SourceCIDR: source,
				FromPort:   from,
				ToPort:     to,
				Action:     verdict,
			})
		}
	}
	return rules
}

func azureAddressPrefixes(single string, many []string) []string {
	var values []string
	if single != "" {
		values = append(values, single)
	}
	values = append(values, many...)
	if len(values) == 0 {
		return []string{"0.0.0.0/0"}
	}
	var out []string
	for _, value := range values {
		switch {
		case value == "" || value == "*" || strings.EqualFold(value, "Internet"):
			out = append(out, "0.0.0.0/0")
		case strings.EqualFold(value, "VirtualNetwork"):
			out = append(out, azureAllVNetCIDRs()...)
		case strings.EqualFold(value, "AzureLoadBalancer"):
			out = append(out, "168.63.129.16/32")
		default:
			out = append(out, value)
		}
	}
	return out
}

func azurePortRanges(single string, many []string) []string {
	var values []string
	if single != "" {
		values = append(values, single)
	}
	values = append(values, many...)
	if len(values) == 0 {
		return []string{""}
	}
	return values
}

func azureNICVNetCIDRs(nic NetworkInterface) []string {
	seen := map[string]bool{}
	var out []string
	for _, ipcfg := range nic.Properties.IPConfigurations {
		if ipcfg.Properties.Subnet == nil || azureSubnets == nil {
			continue
		}
		subnet, ok := azureSubnets.Get(ipcfg.Properties.Subnet.ID)
		if !ok {
			continue
		}
		vnetID := strings.Split(subnet.ID, "/subnets/")[0]
		if azureVnets == nil {
			continue
		}
		vnet, ok := azureVnets.Get(vnetID)
		if !ok {
			continue
		}
		for _, cidr := range vnet.Properties.AddressSpace.AddressPrefixes {
			if !seen[cidr] {
				seen[cidr] = true
				out = append(out, cidr)
			}
		}
	}
	return out
}

func azureAllVNetCIDRs() []string {
	if azureVnets == nil {
		return []string{"0.0.0.0/0"}
	}
	var out []string
	for _, vnet := range azureVnets.List() {
		out = append(out, vnet.Properties.AddressSpace.AddressPrefixes...)
	}
	if len(out) == 0 {
		return []string{"0.0.0.0/0"}
	}
	return out
}

func azurePacketProtocol(protocol string) string {
	switch strings.ToLower(protocol) {
	case "", "*":
		return "all"
	default:
		return strings.ToLower(protocol)
	}
}

func azureParsePortRange(port string) (int, int) {
	if port == "" || port == "*" {
		return 0, 0
	}
	if from, to, ok := strings.Cut(port, "-"); ok {
		start, _ := strconv.Atoi(from)
		end, _ := strconv.Atoi(to)
		return start, end
	}
	value, _ := strconv.Atoi(port)
	return value, value
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
		ip, err := realexec.ReserveAzurePublicIPv4(gw.ID, nil)
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
