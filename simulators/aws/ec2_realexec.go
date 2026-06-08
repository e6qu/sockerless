package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	realexec "github.com/sockerless/simulator-realexec"
)

var (
	ec2RealHost     = realexec.NewHost()
	ec2RealMu       sync.Mutex
	ec2RealVPCs     = map[string]*realexec.Network{}
	ec2RealSubnets  = map[string]*realexec.Subnet{}
	ec2RealNICs     = map[string]*realexec.NamespaceNIC{}
	ec2RealVMNICs   = map[string]*realexec.TapNIC{}
	ec2RealVMs      = map[string]*realexec.FirecrackerVM{}
	ec2RealEBSSlots = map[string]map[string]string{}
	ec2RealNATNICs  = map[string]*realexec.NamespaceNIC{}
	ec2RealECSNICs  = map[string]*realexec.NamespaceNIC{} // taskID -> veth into the container netns
)

// ec2ECSRealNetAvailable reports whether ECS tasks can be plumbed into real VPC
// network namespaces — Linux network capabilities plus nsenter (to configure the
// container's netns). When false the sim uses the cross-platform Docker-network
// tier instead.
func ec2ECSRealNetAvailable() bool {
	if realexec.DetectNetworkCapabilities().Require() != nil {
		return false
	}
	_, err := exec.LookPath("nsenter")
	return err == nil
}

// ec2AttachRealECSTaskNIC plumbs a veth from the task's VPC subnet bridge into
// the container's network namespace, giving it eth0 at the ENI IP. Because each
// VPC is its own netns, overlapping VPC CIDRs work natively — no remapping, the
// ENI IP is the container's real address.
func ec2AttachRealECSTaskNIC(ctx context.Context, taskID, subnetID string, pid int, eniIP string) error {
	sn, ok := ec2Subnets.Get(subnetID)
	if !ok {
		return fmt.Errorf("subnet %s not found", subnetID)
	}
	if err := ec2CreateRealSubnet(ctx, sn); err != nil {
		return err
	}
	ec2RealMu.Lock()
	subnet := ec2RealSubnets[subnetID]
	ec2RealMu.Unlock()
	if subnet == nil {
		return fmt.Errorf("real subnet %s not provisioned", subnetID)
	}
	nic, err := subnet.AttachExternalNamespaceNIC(ctx, realexec.ExternalNamespaceNICSpec{
		PID:           pid,
		HostVethName:  ec2RealName("eh", taskID),
		GuestVethName: ec2RealName("eg", taskID),
		GuestIfName:   "eth0",
		MAC:           ec2ENIMAC(taskID),
		PrivateIP:     net.ParseIP(eniIP),
	})
	if err != nil {
		return err
	}
	ec2RealMu.Lock()
	ec2RealECSNICs[taskID] = nic
	ec2RealMu.Unlock()
	return nil
}

// ec2DetachRealECSTaskNIC tears down a task's VPC veth when the task stops.
func ec2DetachRealECSTaskNIC(ctx context.Context, taskID string) {
	ec2RealMu.Lock()
	nic := ec2RealECSNICs[taskID]
	delete(ec2RealECSNICs, taskID)
	ec2RealMu.Unlock()
	if nic != nil {
		_ = nic.Close(ctx)
	}
}

const ec2RealEBSMaxSlots = 15

// ec2RealNetHostAvailable reports whether the host can build real EC2 network
// fabric (namespaces, bridges, veth, nftables). ec2RealVMHostAvailable reports
// whether it can run real Firecracker VMs. When false, the sim is in the
// API-only tier: the corresponding operations are modeled at the
// control plane without real execution, so IaC/control-plane testing works on
// hosts lacking CAP_NET_ADMIN/nft/KVM.
func ec2RealNetHostAvailable() bool {
	return realexec.DetectNetworkCapabilities().Require() == nil
}

func ec2RealVMHostAvailable() bool {
	return realexec.DetectFirecrackerCapabilities().Require() == nil
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
	defer ec2RealMu.Unlock()
	if _, ok := ec2RealVPCs[vpc.VpcId]; ok {
		return nil
	}

	network, err := ec2RealHost.CreateNetworkNamespace(ctx, ec2RealName("avn", vpc.VpcId))
	if err != nil {
		return err
	}
	ec2RealVPCs[vpc.VpcId] = network
	return nil
}

func ec2DeleteRealVPC(ctx context.Context, vpcID string) error {
	ec2RealMu.Lock()
	network := ec2RealVPCs[vpcID]
	delete(ec2RealVPCs, vpcID)
	for taskID, nic := range ec2RealECSNICs {
		if ec2ECSTaskVPCID(taskID) == vpcID {
			delete(ec2RealECSNICs, taskID)
			_ = nic.Close(ctx)
		}
	}
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
			delete(ec2RealEBSSlots, instanceID)
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

func ec2ECSTaskVPCID(taskID string) string {
	task, ok := ecsTasks.Get(taskID)
	if !ok {
		return ""
	}
	for _, att := range task.Attachments {
		if att.Type != "ElasticNetworkInterface" {
			continue
		}
		for _, d := range att.Details {
			if d.Name != "subnetId" {
				continue
			}
			if subnet, ok := ec2Subnets.Get(d.Value); ok {
				return subnet.VpcId
			}
		}
	}
	return ""
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
	}
	ec2RealMu.Lock()
	defer ec2RealMu.Unlock()
	if _, ok := ec2RealSubnets[subnet.SubnetId]; ok {
		return nil
	}
	network = ec2RealVPCs[subnet.VpcId]
	if network == nil {
		return fmt.Errorf("real VPC %s not provisioned", subnet.VpcId)
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
	ec2RealSubnets[subnet.SubnetId] = realSubnet
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
		delete(ec2RealEBSSlots, instanceIDForENI)
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
	blockDrives, slots, err := ec2RealEBSBlockDrives(inst)
	if err != nil {
		return err
	}
	vm, err := realexec.StartFirecrackerVM(ctx, realexec.FirecrackerVMConfig{
		ID:          "aws-" + inst.InstanceId,
		Tap:         tap,
		MAC:         ec2ENIMAC(inst.NetworkInterfaceId),
		VCPUCount:   1,
		MemoryMiB:   512,
		BlockDrives: blockDrives,
	})
	if err != nil {
		return err
	}
	ec2RealMu.Lock()
	if old := ec2RealVMs[inst.InstanceId]; old != nil {
		_ = old.Stop(context.Background())
	}
	ec2RealVMs[inst.InstanceId] = vm
	ec2RealEBSSlots[inst.InstanceId] = slots
	ec2RealMu.Unlock()
	return ec2ApplyRealNICSecurityGroups(ctx, inst.NetworkInterfaceId, inst.SecurityGroupIds)
}

func ec2StopRealVM(ctx context.Context, instanceID string) error {
	ec2RealMu.Lock()
	vm := ec2RealVMs[instanceID]
	delete(ec2RealVMs, instanceID)
	delete(ec2RealEBSSlots, instanceID)
	ec2RealMu.Unlock()
	if vm == nil {
		return nil
	}
	return vm.Stop(ctx)
}

func ec2RealEBSBlockDrives(inst EC2Instance) ([]realexec.FirecrackerBlockDrive, map[string]string, error) {
	attachments := ec2RealEBSAttachments(inst.InstanceId, inst.RootDeviceName)
	if len(attachments) > ec2RealEBSMaxSlots {
		return nil, nil, fmt.Errorf("instance %s has %d EBS data volumes attached, maximum supported by the Firecracker substrate is %d", inst.InstanceId, len(attachments), ec2RealEBSMaxSlots)
	}
	slots := map[string]string{}
	drives := make([]realexec.FirecrackerBlockDrive, 0, ec2RealEBSMaxSlots)
	for i := 1; i <= ec2RealEBSMaxSlots; i++ {
		slot := ec2RealEBSSlotID(i)
		path := ec2RealEBSSlotPlaceholderPath(inst.InstanceId, slot)
		if i <= len(attachments) {
			vol, ok := ec2Volumes.Get(attachments[i-1].VolumeId)
			if !ok {
				return nil, nil, fmt.Errorf("attached volume %s not found", attachments[i-1].VolumeId)
			}
			blockPath, err := ebsEnsureVolumeBlockImage(&vol)
			if err != nil {
				return nil, nil, fmt.Errorf("prepare block image for %s: %w", vol.VolumeId, err)
			}
			ec2Volumes.Put(vol.VolumeId, vol)
			path = blockPath
			slots[vol.VolumeId] = slot
		} else if err := ec2PrepareRealEBSSlotPlaceholder(path); err != nil {
			return nil, nil, err
		}
		drives = append(drives, realexec.FirecrackerBlockDrive{
			ID:   slot,
			Path: path,
		})
	}
	return drives, slots, nil
}

func ec2RealEBSAttachments(instanceID, rootDeviceName string) []EC2VolumeAttachment {
	var attachments []EC2VolumeAttachment
	for _, vol := range ec2Volumes.List() {
		if len(vol.Attachments) == 0 {
			continue
		}
		att := vol.Attachments[0]
		if att.InstanceId != instanceID || att.Device == rootDeviceName {
			continue
		}
		attachments = append(attachments, att)
	}
	sort.Slice(attachments, func(i, j int) bool {
		return attachments[i].Device < attachments[j].Device
	})
	return attachments
}

func ec2AttachRealVolume(ctx context.Context, instanceID string, vol *EC2Volume) error {
	inst, ok := ec2Instances.Get(instanceID)
	if !ok || inst.State != "running" {
		return nil
	}
	blockPath, err := ebsEnsureVolumeBlockImage(vol)
	if err != nil {
		return err
	}
	ec2RealMu.Lock()
	vm := ec2RealVMs[instanceID]
	slots := ec2RealEBSSlots[instanceID]
	if slots == nil {
		slots = map[string]string{}
		ec2RealEBSSlots[instanceID] = slots
	}
	slot := slots[vol.VolumeId]
	if slot == "" {
		slot = ec2FirstFreeRealEBSSlot(slots)
	}
	ec2RealMu.Unlock()
	if vm == nil || !vm.Alive() {
		return fmt.Errorf("instance %s is running without a live Firecracker VM", instanceID)
	}
	if slot == "" {
		return fmt.Errorf("AttachmentLimitExceeded: no Firecracker EBS drive slots are available for instance %s", instanceID)
	}
	if err := vm.PatchBlockDrivePath(ctx, slot, blockPath); err != nil {
		return err
	}
	ec2RealMu.Lock()
	if ec2RealEBSSlots[instanceID] == nil {
		ec2RealEBSSlots[instanceID] = map[string]string{}
	}
	ec2RealEBSSlots[instanceID][vol.VolumeId] = slot
	ec2RealMu.Unlock()
	return nil
}

func ec2DetachRealVolume(ctx context.Context, instanceID, volumeID string) error {
	inst, ok := ec2Instances.Get(instanceID)
	if !ok || inst.State != "running" {
		return nil
	}
	ec2RealMu.Lock()
	vm := ec2RealVMs[instanceID]
	slot := ""
	if slots := ec2RealEBSSlots[instanceID]; slots != nil {
		slot = slots[volumeID]
	}
	ec2RealMu.Unlock()
	if slot == "" {
		return nil
	}
	if vm == nil || !vm.Alive() {
		return fmt.Errorf("instance %s is running without a live Firecracker VM", instanceID)
	}
	placeholder := ec2RealEBSSlotPlaceholderPath(instanceID, slot)
	if err := ec2PrepareRealEBSSlotPlaceholder(placeholder); err != nil {
		return err
	}
	if err := vm.PatchBlockDrivePath(ctx, slot, placeholder); err != nil {
		return err
	}
	ec2RealMu.Lock()
	if slots := ec2RealEBSSlots[instanceID]; slots != nil {
		delete(slots, volumeID)
	}
	ec2RealMu.Unlock()
	return nil
}

func ec2RefreshRealVolume(ctx context.Context, vol EC2Volume) error {
	if len(vol.Attachments) == 0 {
		return nil
	}
	inst, ok := ec2Instances.Get(vol.Attachments[0].InstanceId)
	if !ok || inst.State != "running" {
		return nil
	}
	ec2RealMu.Lock()
	vm := ec2RealVMs[inst.InstanceId]
	slot := ""
	if slots := ec2RealEBSSlots[inst.InstanceId]; slots != nil {
		slot = slots[vol.VolumeId]
	}
	ec2RealMu.Unlock()
	if vm == nil || !vm.Alive() {
		return fmt.Errorf("instance %s is running without a live Firecracker VM", inst.InstanceId)
	}
	if slot == "" {
		return fmt.Errorf("volume %s is attached to %s without a Firecracker drive slot", vol.VolumeId, inst.InstanceId)
	}
	blockPath, err := ebsEnsureVolumeBlockImage(&vol)
	if err != nil {
		return err
	}
	return vm.PatchBlockDrivePath(ctx, slot, blockPath)
}

func ec2FirstFreeRealEBSSlot(slots map[string]string) string {
	used := map[string]bool{}
	for _, slot := range slots {
		used[slot] = true
	}
	for i := 1; i <= ec2RealEBSMaxSlots; i++ {
		slot := ec2RealEBSSlotID(i)
		if !used[slot] {
			return slot
		}
	}
	return ""
}

func ec2RealEBSSlotID(index int) string {
	return fmt.Sprintf("ebs%d", index)
}

func ec2RealEBSSlotPlaceholderPath(instanceID, slot string) string {
	return filepath.Join(ebsHostRoot(), "firecracker-slots", instanceID, slot+".raw")
}

func ec2PrepareRealEBSSlotPlaceholder(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o666)
	if err != nil {
		return err
	}
	return f.Close()
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
