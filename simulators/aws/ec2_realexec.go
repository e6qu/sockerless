package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	realexec "github.com/sockerless/simulator-realexec"
)

var (
	ec2RealHost    = realexec.NewHost()
	ec2RealMu      sync.Mutex
	ec2RealVPCs    = map[string]*realexec.Network{}
	ec2RealSubnets = map[string]*realexec.Subnet{}
	ec2RealNICs    = map[string]*realexec.NamespaceNIC{}
	ec2RealVMNICs  = map[string]*realexec.TapNIC{}
	ec2RealVMs     = map[string]*realexec.FirecrackerVM{}
	ec2RealNATNICs = map[string]*realexec.NamespaceNIC{}
)

func ec2RequireNetworkHost(w http.ResponseWriter) bool {
	if err := realexec.DetectNetworkCapabilities().Require(); err != nil {
		ec2ErrorXML(w, "UnsupportedOperation",
			fmt.Sprintf("real EC2 networking requires Linux network namespace, bridge, veth, route, and nftables host capabilities: %v", err),
			http.StatusServiceUnavailable)
		return false
	}
	return true
}

func ec2RequireVMHost(w http.ResponseWriter) bool {
	if err := realexec.DetectFirecrackerCapabilities().Require(); err != nil {
		ec2ErrorXML(w, "UnsupportedOperation",
			fmt.Sprintf("real EC2 instances require Linux Firecracker, KVM, TAP, network namespace, bridge, route, and nftables host capabilities: %v", err),
			http.StatusServiceUnavailable)
		return false
	}
	return true
}

func ec2RealName(prefix, id string) string {
	id = strings.NewReplacer("/", "", "-", "", "_", "", ".", "").Replace(id)
	if len(id) > 10 {
		id = id[len(id)-10:]
	}
	name := prefix + id
	if len(name) > 15 {
		return name[:15]
	}
	return name
}

func ec2CreateRealVPC(ctx context.Context, vpc EC2Vpc) error {
	ec2RealMu.Lock()
	if _, ok := ec2RealVPCs[vpc.VpcId]; ok {
		ec2RealMu.Unlock()
		return nil
	}
	ec2RealMu.Unlock()

	network, err := ec2RealHost.CreateNetworkNamespace(ctx, ec2RealName("avn", vpc.VpcId))
	if err != nil {
		return err
	}
	ec2RealMu.Lock()
	ec2RealVPCs[vpc.VpcId] = network
	ec2RealMu.Unlock()
	return nil
}

func ec2DeleteRealVPC(ctx context.Context, vpcID string) error {
	ec2RealMu.Lock()
	network := ec2RealVPCs[vpcID]
	delete(ec2RealVPCs, vpcID)
	for natID, nic := range ec2RealNATNICs {
		if nat, ok := ec2NatGateways.Get(natID); ok && nat.VpcId == vpcID {
			delete(ec2RealNATNICs, natID)
			_ = nic.Close(ctx)
		}
	}
	for eniID, nic := range ec2RealNICs {
		if eni, ok := ec2NetworkInterfaces.Get(eniID); ok && eni.VpcId == vpcID {
			delete(ec2RealNICs, eniID)
			_ = nic.Close(ctx)
		}
	}
	for eniID, nic := range ec2RealVMNICs {
		if eni, ok := ec2NetworkInterfaces.Get(eniID); ok && eni.VpcId == vpcID {
			delete(ec2RealVMNICs, eniID)
			imdsInstancesByIP.Delete(nic.PrivateIP.String())
			_ = nic.Close(ctx)
		}
	}
	for instanceID, vm := range ec2RealVMs {
		if inst, ok := ec2Instances.Get(instanceID); ok && inst.VpcId == vpcID {
			delete(ec2RealVMs, instanceID)
			_ = vm.Stop(ctx)
		}
	}
	for subnetID, subnet := range ec2RealSubnets {
		if subnetForID, ok := ec2Subnets.Get(subnetID); ok && subnetForID.VpcId == vpcID {
			delete(ec2RealSubnets, subnetID)
			_ = subnet.Close(ctx)
		}
	}
	ec2RealMu.Unlock()
	if network == nil {
		return nil
	}
	return network.Close(ctx)
}

func ec2CreateRealSubnet(ctx context.Context, subnet EC2Subnet) error {
	ec2RealMu.Lock()
	if _, ok := ec2RealSubnets[subnet.SubnetId]; ok {
		ec2RealMu.Unlock()
		return nil
	}
	network := ec2RealVPCs[subnet.VpcId]
	ec2RealMu.Unlock()
	if network == nil {
		vpc, ok := ec2Vpcs.Get(subnet.VpcId)
		if !ok {
			return fmt.Errorf("VPC %s not found", subnet.VpcId)
		}
		if err := ec2CreateRealVPC(ctx, vpc); err != nil {
			return err
		}
		ec2RealMu.Lock()
		network = ec2RealVPCs[subnet.VpcId]
		ec2RealMu.Unlock()
	}
	realSubnet, err := network.CreateSubnet(ctx, realexec.SubnetSpec{
		Name:       subnet.SubnetId,
		BridgeName: ec2RealName("asb", subnet.SubnetId),
		CIDR:       subnet.CidrBlock,
		Gateway:    ec2AWSSubnetGateway(subnet.CidrBlock),
	})
	if err != nil {
		return err
	}
	ec2RealMu.Lock()
	ec2RealSubnets[subnet.SubnetId] = realSubnet
	ec2RealMu.Unlock()
	return nil
}

func ec2DeleteRealSubnet(ctx context.Context, subnetID string) error {
	ec2RealMu.Lock()
	subnet := ec2RealSubnets[subnetID]
	delete(ec2RealSubnets, subnetID)
	ec2RealMu.Unlock()
	if subnet == nil {
		return nil
	}
	return subnet.Close(ctx)
}

func ec2DeleteRealNIC(ctx context.Context, eniID string) error {
	instanceIDForENI := ""
	for _, inst := range ec2Instances.List() {
		if inst.NetworkInterfaceId == eniID {
			instanceIDForENI = inst.InstanceId
			break
		}
	}
	ec2RealMu.Lock()
	nic := ec2RealNICs[eniID]
	delete(ec2RealNICs, eniID)
	tap := ec2RealVMNICs[eniID]
	delete(ec2RealVMNICs, eniID)
	var vm *realexec.FirecrackerVM
	if instanceIDForENI != "" {
		vm = ec2RealVMs[instanceIDForENI]
		delete(ec2RealVMs, instanceIDForENI)
	}
	ec2RealMu.Unlock()
	var errs []error
	if vm != nil {
		errs = append(errs, vm.Stop(ctx))
	}
	if nic == nil {
		if tap != nil {
			imdsInstancesByIP.Delete(tap.PrivateIP.String())
			errs = append(errs, tap.Close(ctx))
		}
		return errors.Join(errs...)
	}
	errs = append(errs, nic.Close(ctx))
	if tap != nil {
		imdsInstancesByIP.Delete(tap.PrivateIP.String())
		errs = append(errs, tap.Close(ctx))
	}
	return errors.Join(errs...)
}

func ec2ApplyRealNICSecurityGroups(ctx context.Context, eniID string, securityGroupIDs []string) error {
	ec2RealMu.Lock()
	nic := ec2RealNICs[eniID]
	tap := ec2RealVMNICs[eniID]
	ec2RealMu.Unlock()
	if nic == nil && tap == nil {
		return nil
	}
	var rules []realexec.PacketRule
	for _, groupID := range securityGroupIDs {
		sg, ok := ec2SecurityGroups.Get(groupID)
		if !ok {
			continue
		}
		for _, perm := range sg.IpPermissions {
			if len(perm.IpRanges) == 0 {
				rules = append(rules, realexec.PacketRule{
					Protocol:   perm.IpProtocol,
					SourceCIDR: "0.0.0.0/0",
					FromPort:   perm.FromPort,
					ToPort:     perm.ToPort,
				})
				continue
			}
			for _, ipRange := range perm.IpRanges {
				rules = append(rules, realexec.PacketRule{
					Protocol:   perm.IpProtocol,
					SourceCIDR: ipRange.CidrIp,
					FromPort:   perm.FromPort,
					ToPort:     perm.ToPort,
				})
			}
		}
	}
	if nic != nil {
		if err := nic.ConfigureIngressFilter(ctx, rules); err != nil {
			return err
		}
	}
	if tap != nil {
		if err := tap.ConfigureIngressFilter(ctx, rules); err != nil {
			return err
		}
	}
	return nil
}

func ec2StartRealVM(ctx context.Context, inst EC2Instance) error {
	if inst.NetworkInterfaceId == "" {
		return fmt.Errorf("instance %s has no network interface", inst.InstanceId)
	}
	ec2RealMu.Lock()
	if vm := ec2RealVMs[inst.InstanceId]; vm != nil && vm.Alive() {
		ec2RealMu.Unlock()
		return nil
	}
	tap := ec2RealVMNICs[inst.NetworkInterfaceId]
	subnet := ec2RealSubnets[inst.SubnetId]
	ec2RealMu.Unlock()
	if subnet == nil {
		sn, ok := ec2Subnets.Get(inst.SubnetId)
		if !ok {
			return fmt.Errorf("subnet %s not found", inst.SubnetId)
		}
		if err := ec2CreateRealSubnet(ctx, sn); err != nil {
			return err
		}
		ec2RealMu.Lock()
		subnet = ec2RealSubnets[inst.SubnetId]
		ec2RealMu.Unlock()
	}
	if tap == nil {
		created, err := subnet.AttachTapNIC(ctx, realexec.TapNICSpec{
			TapName:   ec2RealName("at", inst.NetworkInterfaceId),
			PrivateIP: net.ParseIP(inst.PrivateIpAddress),
			MAC:       ec2ENIMAC(inst.NetworkInterfaceId),
		})
		if err != nil {
			return err
		}
		tap = created
		ec2RealMu.Lock()
		ec2RealVMNICs[inst.NetworkInterfaceId] = tap
		ec2RealMu.Unlock()
	}
	imdsInstancesByIP.Store(tap.PrivateIP.String(), inst)
	metadataPort, err := simHostMetadataPort()
	if err != nil {
		return err
	}
	if err := subnet.ConfigureMetadataDNAT(ctx, metadataPort, ec2RealName("amd", inst.VpcId)); err != nil {
		return fmt.Errorf("configure EC2 IMDS routing for %s: %w", inst.InstanceId, err)
	}
	vm, err := realexec.StartFirecrackerVM(ctx, realexec.FirecrackerVMConfig{
		ID:        "aws-" + inst.InstanceId,
		Tap:       tap,
		MAC:       ec2ENIMAC(inst.NetworkInterfaceId),
		VCPUCount: 1,
		MemoryMiB: 512,
	})
	if err != nil {
		return err
	}
	ec2RealMu.Lock()
	if old := ec2RealVMs[inst.InstanceId]; old != nil {
		_ = old.Stop(context.Background())
	}
	ec2RealVMs[inst.InstanceId] = vm
	ec2RealMu.Unlock()
	return ec2ApplyRealNICSecurityGroups(ctx, inst.NetworkInterfaceId, inst.SecurityGroupIds)
}

func ec2StopRealVM(ctx context.Context, instanceID string) error {
	ec2RealMu.Lock()
	vm := ec2RealVMs[instanceID]
	delete(ec2RealVMs, instanceID)
	ec2RealMu.Unlock()
	if vm == nil {
		return nil
	}
	return vm.Stop(ctx)
}

func ec2RealVMAlive(instanceID string) bool {
	ec2RealMu.Lock()
	vm := ec2RealVMs[instanceID]
	ec2RealMu.Unlock()
	return vm != nil && vm.Alive()
}

func ec2ReapplyRealSecurityGroup(ctx context.Context, groupID string) error {
	for _, eni := range ec2NetworkInterfaces.List() {
		for _, attachedGroupID := range eni.SecurityGroupIds {
			if attachedGroupID != groupID {
				continue
			}
			if err := ec2ApplyRealNICSecurityGroups(ctx, eni.NetworkInterfaceId, eni.SecurityGroupIds); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func ec2CreateRealNATGateway(ctx context.Context, nat EC2NatGateway) error {
	ec2RealMu.Lock()
	if _, ok := ec2RealNATNICs[nat.NatGatewayId]; ok {
		ec2RealMu.Unlock()
		return nil
	}
	subnet := ec2RealSubnets[nat.SubnetId]
	ec2RealMu.Unlock()
	if subnet == nil {
		sn, ok := ec2Subnets.Get(nat.SubnetId)
		if !ok {
			return fmt.Errorf("subnet %s not found", nat.SubnetId)
		}
		if err := ec2CreateRealSubnet(ctx, sn); err != nil {
			return err
		}
		ec2RealMu.Lock()
		subnet = ec2RealSubnets[nat.SubnetId]
		ec2RealMu.Unlock()
	}
	if len(nat.NatGatewayAddresses) == 0 {
		return fmt.Errorf("NAT gateway %s has no address attachment", nat.NatGatewayId)
	}
	addr := nat.NatGatewayAddresses[0]
	nic, err := subnet.AttachNamespaceNIC(ctx, realexec.NamespaceNICSpec{
		NamespaceName: ec2RealName("an", nat.NatGatewayId),
		HostVethName:  ec2RealName("nh", nat.NatGatewayId),
		GuestVethName: ec2RealName("ng", nat.NatGatewayId),
		PrivateIP:     net.ParseIP(addr.PrivateIp),
		MAC:           ec2ENIMAC(addr.NetworkInterfaceId),
	})
	if err != nil {
		return err
	}
	ec2RealMu.Lock()
	ec2RealNATNICs[nat.NatGatewayId] = nic
	ec2RealMu.Unlock()
	return nil
}

func ec2DeleteRealNATGateway(ctx context.Context, natID string) error {
	ec2RealMu.Lock()
	nic := ec2RealNATNICs[natID]
	delete(ec2RealNATNICs, natID)
	ec2RealMu.Unlock()
	if nic == nil {
		return nil
	}
	return nic.Close(ctx)
}

func ec2ConfigureRealNATRoute(ctx context.Context, routeTableID, destinationCIDR, natID string) error {
	nat, ok := ec2NatGateways.Get(natID)
	if !ok {
		return fmt.Errorf("NAT gateway %s not found", natID)
	}
	if len(nat.NatGatewayAddresses) == 0 || nat.NatGatewayAddresses[0].PublicIp == "" {
		return fmt.Errorf("NAT gateway %s has no public IPv4 address", natID)
	}
	rt, ok := ec2RouteTables.Get(routeTableID)
	if !ok {
		return fmt.Errorf("route table %s not found", routeTableID)
	}
	network := (*realexec.Network)(nil)
	ec2RealMu.Lock()
	network = ec2RealVPCs[rt.VpcId]
	ec2RealMu.Unlock()
	if network == nil {
		vpc, ok := ec2Vpcs.Get(rt.VpcId)
		if !ok {
			return fmt.Errorf("VPC %s not found", rt.VpcId)
		}
		if err := ec2CreateRealVPC(ctx, vpc); err != nil {
			return err
		}
		ec2RealMu.Lock()
		network = ec2RealVPCs[rt.VpcId]
		ec2RealMu.Unlock()
	}
	sourceCIDR := ""
	for _, assoc := range rt.Associations {
		if subnet, ok := ec2Subnets.Get(assoc.SubnetId); ok {
			sourceCIDR = subnet.CidrBlock
			break
		}
	}
	if sourceCIDR == "" {
		if subnet, ok := ec2Subnets.Get(nat.SubnetId); ok {
			sourceCIDR = subnet.CidrBlock
		}
	}
	if sourceCIDR == "" {
		return fmt.Errorf("route table %s has no subnet CIDR for NAT source", routeTableID)
	}
	return network.ConfigureSNAT(ctx, sourceCIDR, net.ParseIP(nat.NatGatewayAddresses[0].PublicIp), ec2RealName("sn", routeTableID+destinationCIDR))
}

func ec2AWSSubnetGateway(cidr string) net.IP {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil || ip.To4() == nil {
		return nil
	}
	out := append(net.IP(nil), ip.To4()...)
	out[3]++
	return out
}

func ec2ENIMAC(id string) string {
	id = strings.NewReplacer("-", "", "_", "").Replace(id)
	var b [3]byte
	for i := range id {
		b[i%3] ^= id[i]
	}
	return fmt.Sprintf("02:0a:ec:%02x:%02x:%02x", b[0], b[1], b[2])
}
