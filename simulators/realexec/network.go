package realexec

import (
	"context"
	"fmt"
	"net"
	"regexp"
)

var linuxNameRE = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,15}$`)

type Host struct {
	runner Runner
}

func NewHost() *Host {
	return &Host{runner: Runner{}}
}

type NetworkSpec struct {
	BridgeName string
	CIDR       string
}

type Network struct {
	BridgeName string
	Gateway    net.IP
	ipam       *IPAM
	cleanup    *CleanupStack
	runner     Runner
}

func (h *Host) CreateNetwork(ctx context.Context, spec NetworkSpec) (*Network, error) {
	if err := validateLinuxName("bridge", spec.BridgeName); err != nil {
		return nil, err
	}
	ip, network, err := net.ParseCIDR(spec.CIDR)
	if err != nil {
		return nil, err
	}
	if ip.To4() == nil {
		return nil, fmt.Errorf("only IPv4 networks are supported in this substrate phase: %s", spec.CIDR)
	}
	gateway := nextIPv4(network.IP.To4())
	ipam, err := NewIPAM(network.String(), gateway)
	if err != nil {
		return nil, err
	}

	rollback := &CleanupStack{}
	if err := h.runner.Run(ctx, "ip", "link", "add", "name", spec.BridgeName, "type", "bridge"); err != nil {
		return nil, err
	}
	rollback.Add(func(cleanupCtx context.Context) error {
		return h.runner.Run(cleanupCtx, "ip", "link", "del", spec.BridgeName)
	})
	if err := h.runner.Run(ctx, "ip", "addr", "add", fmt.Sprintf("%s/%d", gateway, ipam.PrefixBits()), "dev", spec.BridgeName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := h.runner.Run(ctx, "ip", "link", "set", "dev", spec.BridgeName, "up"); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}

	n := &Network{
		BridgeName: spec.BridgeName,
		Gateway:    append(net.IP(nil), gateway...),
		ipam:       ipam,
		runner:     h.runner,
	}
	n.cleanup = rollback
	return n, nil
}

func (n *Network) Close(ctx context.Context) error {
	if n.cleanup == nil {
		return nil
	}
	return n.cleanup.Close(ctx)
}

type NamespaceNICSpec struct {
	NamespaceName string
	HostVethName  string
	GuestVethName string
	MAC           string
	PrivateIP     net.IP
}

type NamespaceNIC struct {
	NamespaceName string
	HostVethName  string
	GuestVethName string
	PrivateIP     net.IP
	Gateway       net.IP
	cleanup       *CleanupStack
	network       *Network
}

func (n *Network) AttachNamespaceNIC(ctx context.Context, spec NamespaceNICSpec) (*NamespaceNIC, error) {
	if err := validateLinuxName("namespace", spec.NamespaceName); err != nil {
		return nil, err
	}
	if err := validateLinuxName("host veth", spec.HostVethName); err != nil {
		return nil, err
	}
	if err := validateLinuxName("guest veth", spec.GuestVethName); err != nil {
		return nil, err
	}
	if spec.MAC != "" {
		if _, err := net.ParseMAC(spec.MAC); err != nil {
			return nil, fmt.Errorf("invalid MAC %q: %w", spec.MAC, err)
		}
	}

	ip, err := n.ipam.Reserve(spec.NamespaceName, spec.PrivateIP)
	if err != nil {
		return nil, err
	}
	rollback := &CleanupStack{}
	rollback.Add(func(context.Context) error {
		n.ipam.Release(ip)
		return nil
	})

	if err := n.runner.Run(ctx, "ip", "netns", "add", spec.NamespaceName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	rollback.Add(func(cleanupCtx context.Context) error {
		return n.runner.Run(cleanupCtx, "ip", "netns", "del", spec.NamespaceName)
	})

	if err := n.runner.Run(ctx, "ip", "link", "add", spec.HostVethName, "type", "veth", "peer", "name", spec.GuestVethName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	rollback.Add(func(cleanupCtx context.Context) error {
		return n.runner.Run(cleanupCtx, "ip", "link", "del", spec.HostVethName)
	})

	if err := n.runner.Run(ctx, "ip", "link", "set", spec.HostVethName, "master", n.BridgeName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := n.runner.Run(ctx, "ip", "link", "set", spec.HostVethName, "up"); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := n.runner.Run(ctx, "ip", "link", "set", spec.GuestVethName, "netns", spec.NamespaceName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}

	if spec.MAC != "" {
		if err := n.runner.Run(ctx, "ip", "netns", "exec", spec.NamespaceName, "ip", "link", "set", "dev", spec.GuestVethName, "address", spec.MAC); err != nil {
			_ = rollback.Close(context.Background())
			return nil, err
		}
	}
	if err := n.runner.Run(ctx, "ip", "netns", "exec", spec.NamespaceName, "ip", "addr", "add", fmt.Sprintf("%s/%d", ip, n.ipam.PrefixBits()), "dev", spec.GuestVethName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := n.runner.Run(ctx, "ip", "netns", "exec", spec.NamespaceName, "ip", "link", "set", "dev", "lo", "up"); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := n.runner.Run(ctx, "ip", "netns", "exec", spec.NamespaceName, "ip", "link", "set", "dev", spec.GuestVethName, "up"); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := n.runner.Run(ctx, "ip", "netns", "exec", spec.NamespaceName, "ip", "route", "replace", "default", "via", n.Gateway.String(), "dev", spec.GuestVethName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}

	nic := &NamespaceNIC{
		NamespaceName: spec.NamespaceName,
		HostVethName:  spec.HostVethName,
		GuestVethName: spec.GuestVethName,
		PrivateIP:     append(net.IP(nil), ip...),
		Gateway:       append(net.IP(nil), n.Gateway...),
		cleanup:       rollback,
		network:       n,
	}
	return nic, nil
}

func (n *NamespaceNIC) Close(ctx context.Context) error {
	if n.cleanup == nil {
		return nil
	}
	return n.cleanup.Close(ctx)
}

func validateLinuxName(label, name string) error {
	if !linuxNameRE.MatchString(name) {
		return fmt.Errorf("invalid %s name %q: must match %s", label, name, linuxNameRE.String())
	}
	return nil
}

func nextIPv4(ip net.IP) net.IP {
	out := append(net.IP(nil), ip.To4()...)
	out[3]++
	return out
}
