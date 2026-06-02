package realexec

import (
	"context"
	"fmt"
	"hash/fnv"
	"net"
	"regexp"
	"strings"
	"sync"
)

var linuxNameRE = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,15}$`)

type Host struct {
	runner Runner
}

func NewHost() *Host {
	return &Host{runner: Runner{}}
}

type NetworkSpec struct {
	NamespaceName string
	BridgeName    string
	CIDR          string
}

type Network struct {
	NamespaceName string
	BridgeName    string
	Gateway       net.IP
	egress        *EgressLink
	defaultSubnet *Subnet
	subnets       map[string]*Subnet
	mu            sync.Mutex
	cleanup       *CleanupStack
	runner        Runner
}

type EgressLink struct {
	HostVethName string
	NetVethName  string
	HostIP       net.IP
	NetIP        net.IP
	PrefixBits   int
}

func (h *Host) CreateNetwork(ctx context.Context, spec NetworkSpec) (*Network, error) {
	if spec.NamespaceName == "" {
		spec.NamespaceName = deriveLinuxName(spec.BridgeName, "ns")
	}
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
	n, err := h.CreateNetworkNamespace(ctx, spec.NamespaceName)
	if err != nil {
		return nil, err
	}
	n.BridgeName = spec.BridgeName
	subnet, err := n.CreateSubnet(ctx, SubnetSpec{
		Name:       "default",
		BridgeName: spec.BridgeName,
		CIDR:       network.String(),
	})
	if err != nil {
		_ = n.Close(context.Background())
		return nil, err
	}
	n.defaultSubnet = subnet
	n.Gateway = append(net.IP(nil), subnet.Gateway...)
	return n, nil
}

func (h *Host) CreateNetworkNamespace(ctx context.Context, namespaceName string) (*Network, error) {
	if err := validateLinuxName("network namespace", namespaceName); err != nil {
		return nil, err
	}

	rollback := &CleanupStack{}
	if err := h.runner.Run(ctx, "ip", "netns", "add", namespaceName); err != nil {
		return nil, err
	}
	rollback.Add(func(cleanupCtx context.Context) error {
		return h.runner.Run(cleanupCtx, "ip", "netns", "del", namespaceName)
	})
	if err := h.runner.Run(ctx, "ip", "netns", "exec", namespaceName, "ip", "link", "set", "dev", "lo", "up"); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}

	return &Network{
		NamespaceName: namespaceName,
		subnets:       map[string]*Subnet{},
		cleanup:       rollback,
		runner:        h.runner,
	}, nil
}

func (n *Network) Close(ctx context.Context) error {
	if n.cleanup == nil {
		return nil
	}
	return n.cleanup.Close(ctx)
}

func (n *Network) EnsureEgress(ctx context.Context) (*EgressLink, error) {
	n.mu.Lock()
	if n.egress != nil {
		link := *n.egress
		n.mu.Unlock()
		return &link, nil
	}
	n.mu.Unlock()

	hostName := deriveLinuxName("eh"+n.NamespaceName, "eh")
	netName := deriveLinuxName("en"+n.NamespaceName, "en")
	if err := validateLinuxName("egress host veth", hostName); err != nil {
		return nil, err
	}
	if err := validateLinuxName("egress network veth", netName); err != nil {
		return nil, err
	}
	hostIP, netIP, prefixBits := egressIPs(n.NamespaceName)

	rollback := &CleanupStack{}
	if err := n.runner.Run(ctx, "ip", "link", "add", hostName, "type", "veth", "peer", "name", netName); err != nil {
		return nil, err
	}
	rollback.Add(func(cleanupCtx context.Context) error {
		if err := n.runner.Run(cleanupCtx, "ip", "link", "del", hostName); err == nil {
			return nil
		}
		return n.runner.Run(cleanupCtx, "ip", "netns", "exec", n.NamespaceName, "ip", "link", "del", netName)
	})
	if err := n.runner.Run(ctx, "ip", "link", "set", netName, "netns", n.NamespaceName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := n.runner.Run(ctx, "ip", "addr", "replace", fmt.Sprintf("%s/%d", hostIP, prefixBits), "dev", hostName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := n.runner.Run(ctx, "ip", "link", "set", "dev", hostName, "up"); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := n.runner.Run(ctx, "ip", "netns", "exec", n.NamespaceName, "ip", "addr", "replace", fmt.Sprintf("%s/%d", netIP, prefixBits), "dev", netName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := n.runner.Run(ctx, "ip", "netns", "exec", n.NamespaceName, "ip", "link", "set", "dev", netName, "up"); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := n.runner.Run(ctx, "ip", "netns", "exec", n.NamespaceName, "sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := n.runner.Run(ctx, "ip", "netns", "exec", n.NamespaceName, "ip", "route", "replace", "default", "via", hostIP.String(), "dev", netName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}

	link := &EgressLink{
		HostVethName: hostName,
		NetVethName:  netName,
		HostIP:       hostIP,
		NetIP:        netIP,
		PrefixBits:   prefixBits,
	}
	installed := false
	n.mu.Lock()
	if n.egress == nil {
		n.egress = link
		n.cleanup.Add(func(cleanupCtx context.Context) error { return rollback.Close(cleanupCtx) })
		installed = true
	}
	out := *n.egress
	n.mu.Unlock()
	if !installed {
		_ = rollback.Close(context.Background())
	}
	return &out, nil
}

func (n *Network) ConfigureSNAT(ctx context.Context, sourceCIDR string, publicIP net.IP, tableName string) error {
	if publicIP == nil || publicIP.To4() == nil {
		return fmt.Errorf("SNAT public IPv4 address is required")
	}
	if tableName == "" {
		tableName = deriveLinuxName("sn"+n.NamespaceName, "sn")
	}
	link, err := n.EnsureEgress(ctx)
	if err != nil {
		return err
	}
	publicIP = publicIP.To4()
	if err := n.runner.Run(ctx, "ip", "netns", "exec", n.NamespaceName, "ip", "addr", "replace", publicIP.String()+"/32", "dev", link.NetVethName); err != nil {
		return err
	}
	if err := n.runner.Run(ctx, "ip", "route", "replace", publicIP.String()+"/32", "via", link.NetIP.String(), "dev", link.HostVethName); err != nil {
		return err
	}
	_ = n.runner.Run(ctx, "ip", "netns", "exec", n.NamespaceName, "nft", "delete", "table", "inet", tableName)
	if err := n.runner.Run(ctx, "ip", "netns", "exec", n.NamespaceName, "nft", "add", "table", "inet", tableName); err != nil {
		return err
	}
	if err := n.runner.Run(ctx, "ip", "netns", "exec", n.NamespaceName, "nft", "add", "chain", "inet", tableName, "postrouting", "{", "type", "nat", "hook", "postrouting", "priority", "srcnat", ";", "}"); err != nil {
		return err
	}
	return n.runner.Run(ctx, "ip", "netns", "exec", n.NamespaceName, "nft", "add", "rule", "inet", tableName, "postrouting", "ip", "saddr", sourceCIDR, "oifname", link.NetVethName, "snat", "to", publicIP.String())
}

func egressIPs(namespaceName string) (hostIP, netIP net.IP, prefixBits int) {
	h := fnv.New32a()
	_, _ = h.Write([]byte(namespaceName))
	index := h.Sum32() % 32768
	second := byte(18 + index/16384)
	third := byte((index / 64) % 256)
	fourth := byte((index % 64) * 4)
	return net.IPv4(198, second, third, fourth+1), net.IPv4(198, second, third, fourth+2), 30
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

type TapNICSpec struct {
	TapName   string
	PrivateIP net.IP
	MAC       string
}

type TapNIC struct {
	TapName    string
	PrivateIP  net.IP
	Gateway    net.IP
	PrefixBits int
	cleanup    *CleanupStack
	subnet     *Subnet
	network    *Network
}

type PacketRule struct {
	Protocol   string
	SourceCIDR string
	FromPort   int
	ToPort     int
	Action     string
}

type SubnetSpec struct {
	Name       string
	BridgeName string
	CIDR       string
	Gateway    net.IP
}

type Subnet struct {
	Name       string
	BridgeName string
	CIDR       string
	Gateway    net.IP
	ipam       *IPAM
	cleanup    *CleanupStack
	network    *Network
}

func (n *Network) CreateSubnet(ctx context.Context, spec SubnetSpec) (*Subnet, error) {
	if spec.Name == "" {
		return nil, fmt.Errorf("subnet name is required")
	}
	if err := validateLinuxName("subnet bridge", spec.BridgeName); err != nil {
		return nil, err
	}
	ip, network, err := net.ParseCIDR(spec.CIDR)
	if err != nil {
		return nil, err
	}
	if ip.To4() == nil {
		return nil, fmt.Errorf("only IPv4 subnets are supported in this substrate phase: %s", spec.CIDR)
	}
	network.IP = append(net.IP(nil), ip.To4()...)
	gateway := spec.Gateway
	if gateway == nil {
		gateway = nextIPv4(network.IP.To4())
	}
	ipam, err := NewIPAM(network.String(), gateway)
	if err != nil {
		return nil, err
	}

	n.mu.Lock()
	if _, ok := n.subnets[spec.Name]; ok {
		n.mu.Unlock()
		return nil, fmt.Errorf("subnet %q already exists", spec.Name)
	}
	n.mu.Unlock()

	rollback := &CleanupStack{}
	if err := n.runner.Run(ctx, "ip", "netns", "exec", n.NamespaceName, "ip", "link", "add", "name", spec.BridgeName, "type", "bridge"); err != nil {
		return nil, err
	}
	rollback.Add(func(cleanupCtx context.Context) error {
		return n.runner.Run(cleanupCtx, "ip", "netns", "exec", n.NamespaceName, "ip", "link", "del", spec.BridgeName)
	})
	if err := n.runner.Run(ctx, "ip", "netns", "exec", n.NamespaceName, "ip", "addr", "add", fmt.Sprintf("%s/%d", gateway, ipam.PrefixBits()), "dev", spec.BridgeName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := n.runner.Run(ctx, "ip", "netns", "exec", n.NamespaceName, "ip", "link", "set", "dev", spec.BridgeName, "up"); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	egress, err := n.EnsureEgress(ctx)
	if err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := n.runner.Run(ctx, "ip", "route", "replace", network.String(), "via", egress.NetIP.String(), "dev", egress.HostVethName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	rollback.Add(func(cleanupCtx context.Context) error {
		return n.runner.Run(cleanupCtx, "ip", "route", "del", network.String(), "via", egress.NetIP.String(), "dev", egress.HostVethName)
	})

	subnet := &Subnet{
		Name:       spec.Name,
		BridgeName: spec.BridgeName,
		CIDR:       network.String(),
		Gateway:    append(net.IP(nil), gateway.To4()...),
		ipam:       ipam,
		cleanup:    rollback,
		network:    n,
	}
	n.mu.Lock()
	n.subnets[spec.Name] = subnet
	n.mu.Unlock()
	return subnet, nil
}

func (s *Subnet) Close(ctx context.Context) error {
	if s.cleanup == nil {
		return nil
	}
	if err := s.cleanup.Close(ctx); err != nil {
		return err
	}
	s.network.mu.Lock()
	delete(s.network.subnets, s.Name)
	s.network.mu.Unlock()
	return nil
}

func (n *Network) AttachNamespaceNIC(ctx context.Context, spec NamespaceNICSpec) (*NamespaceNIC, error) {
	if n.defaultSubnet == nil {
		return nil, fmt.Errorf("network %s has no default subnet", n.NamespaceName)
	}
	return n.defaultSubnet.AttachNamespaceNIC(ctx, spec)
}

func (s *Subnet) AttachNamespaceNIC(ctx context.Context, spec NamespaceNICSpec) (*NamespaceNIC, error) {
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

	ip, err := s.ipam.Reserve(spec.NamespaceName, spec.PrivateIP)
	if err != nil {
		return nil, err
	}
	rollback := &CleanupStack{}
	rollback.Add(func(context.Context) error {
		s.ipam.Release(ip)
		return nil
	})

	if err := s.network.runner.Run(ctx, "ip", "netns", "add", spec.NamespaceName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	rollback.Add(func(cleanupCtx context.Context) error {
		return s.network.runner.Run(cleanupCtx, "ip", "netns", "del", spec.NamespaceName)
	})

	if err := s.network.runner.Run(ctx, "ip", "link", "add", spec.HostVethName, "type", "veth", "peer", "name", spec.GuestVethName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	rollback.Add(func(cleanupCtx context.Context) error {
		if err := s.network.runner.Run(cleanupCtx, "ip", "netns", "exec", s.network.NamespaceName, "ip", "link", "del", spec.HostVethName); err == nil {
			return nil
		}
		return s.network.runner.Run(cleanupCtx, "ip", "link", "del", spec.HostVethName)
	})

	if err := s.network.runner.Run(ctx, "ip", "link", "set", spec.HostVethName, "netns", s.network.NamespaceName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := s.network.runner.Run(ctx, "ip", "netns", "exec", s.network.NamespaceName, "ip", "link", "set", spec.HostVethName, "master", s.BridgeName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := s.network.runner.Run(ctx, "ip", "netns", "exec", s.network.NamespaceName, "ip", "link", "set", spec.HostVethName, "up"); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := s.network.runner.Run(ctx, "ip", "link", "set", spec.GuestVethName, "netns", spec.NamespaceName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}

	if spec.MAC != "" {
		if err := s.network.runner.Run(ctx, "ip", "netns", "exec", spec.NamespaceName, "ip", "link", "set", "dev", spec.GuestVethName, "address", spec.MAC); err != nil {
			_ = rollback.Close(context.Background())
			return nil, err
		}
	}
	if err := s.network.runner.Run(ctx, "ip", "netns", "exec", spec.NamespaceName, "ip", "addr", "add", fmt.Sprintf("%s/%d", ip, s.ipam.PrefixBits()), "dev", spec.GuestVethName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := s.network.runner.Run(ctx, "ip", "netns", "exec", spec.NamespaceName, "ip", "link", "set", "dev", "lo", "up"); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := s.network.runner.Run(ctx, "ip", "netns", "exec", spec.NamespaceName, "ip", "link", "set", "dev", spec.GuestVethName, "up"); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := s.network.runner.Run(ctx, "ip", "netns", "exec", spec.NamespaceName, "ip", "route", "replace", "default", "via", s.Gateway.String(), "dev", spec.GuestVethName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}

	nic := &NamespaceNIC{
		NamespaceName: spec.NamespaceName,
		HostVethName:  spec.HostVethName,
		GuestVethName: spec.GuestVethName,
		PrivateIP:     append(net.IP(nil), ip...),
		Gateway:       append(net.IP(nil), s.Gateway...),
		cleanup:       rollback,
		network:       s.network,
	}
	return nic, nil
}

func (s *Subnet) AttachTapNIC(ctx context.Context, spec TapNICSpec) (*TapNIC, error) {
	if err := validateLinuxName("tap", spec.TapName); err != nil {
		return nil, err
	}
	if spec.MAC != "" {
		if _, err := net.ParseMAC(spec.MAC); err != nil {
			return nil, fmt.Errorf("invalid MAC %q: %w", spec.MAC, err)
		}
	}

	ip, err := s.ipam.Reserve(spec.TapName, spec.PrivateIP)
	if err != nil {
		return nil, err
	}
	rollback := &CleanupStack{}
	rollback.Add(func(context.Context) error {
		s.ipam.Release(ip)
		return nil
	})

	if err := s.network.runner.Run(ctx, "ip", "netns", "exec", s.network.NamespaceName, "ip", "tuntap", "add", "dev", spec.TapName, "mode", "tap"); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	rollback.Add(func(cleanupCtx context.Context) error {
		return s.network.runner.Run(cleanupCtx, "ip", "netns", "exec", s.network.NamespaceName, "ip", "link", "del", spec.TapName)
	})
	if spec.MAC != "" {
		if err := s.network.runner.Run(ctx, "ip", "netns", "exec", s.network.NamespaceName, "ip", "link", "set", "dev", spec.TapName, "address", spec.MAC); err != nil {
			_ = rollback.Close(context.Background())
			return nil, err
		}
	}
	if err := s.network.runner.Run(ctx, "ip", "netns", "exec", s.network.NamespaceName, "ip", "link", "set", spec.TapName, "master", s.BridgeName); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}
	if err := s.network.runner.Run(ctx, "ip", "netns", "exec", s.network.NamespaceName, "ip", "link", "set", "dev", spec.TapName, "up"); err != nil {
		_ = rollback.Close(context.Background())
		return nil, err
	}

	nic := &TapNIC{
		TapName:    spec.TapName,
		PrivateIP:  append(net.IP(nil), ip...),
		Gateway:    append(net.IP(nil), s.Gateway...),
		PrefixBits: s.ipam.PrefixBits(),
		cleanup:    rollback,
		subnet:     s,
		network:    s.network,
	}
	return nic, nil
}

func (n *NamespaceNIC) Close(ctx context.Context) error {
	if n.cleanup == nil {
		return nil
	}
	return n.cleanup.Close(ctx)
}

func (n *TapNIC) Close(ctx context.Context) error {
	if n.cleanup == nil {
		return nil
	}
	return n.cleanup.Close(ctx)
}

func (n *NamespaceNIC) ConfigureIngressFilter(ctx context.Context, rules []PacketRule) error {
	if n.network == nil {
		return fmt.Errorf("NIC %s is not attached to a network", n.HostVethName)
	}
	return n.configureIngressFilter(ctx, n.HostVethName, rules)
}

func (n *TapNIC) ConfigureIngressFilter(ctx context.Context, rules []PacketRule) error {
	if n.network == nil {
		return fmt.Errorf("tap NIC %s is not attached to a network", n.TapName)
	}
	return configureBridgeIngressFilter(ctx, n.network, n.cleanup, n.TapName, rules)
}

func (n *NamespaceNIC) configureIngressFilter(ctx context.Context, devName string, rules []PacketRule) error {
	return configureBridgeIngressFilter(ctx, n.network, n.cleanup, devName, rules)
}

func configureBridgeIngressFilter(ctx context.Context, network *Network, cleanup *CleanupStack, devName string, rules []PacketRule) error {
	table := deriveLinuxName("fw"+devName, "fw")
	_ = network.runner.Run(ctx, "ip", "netns", "exec", network.NamespaceName, "nft", "delete", "table", "bridge", table)
	if err := network.runner.Run(ctx, "ip", "netns", "exec", network.NamespaceName, "nft", "add", "table", "bridge", table); err != nil {
		return err
	}
	cleanup.Add(func(cleanupCtx context.Context) error {
		_ = network.runner.Run(cleanupCtx, "ip", "netns", "exec", network.NamespaceName, "nft", "delete", "table", "bridge", table)
		return nil
	})
	if err := network.runner.Run(ctx, "ip", "netns", "exec", network.NamespaceName, "nft", "add", "chain", "bridge", table, "forward", "{", "type", "filter", "hook", "forward", "priority", "filter", ";", "policy", "accept", ";", "}"); err != nil {
		return err
	}
	if err := network.runner.Run(ctx, "ip", "netns", "exec", network.NamespaceName, "nft", "add", "rule", "bridge", table, "forward", "ct", "state", "established,related", "accept"); err != nil {
		return err
	}
	for _, rule := range rules {
		args := []string{"ip", "netns", "exec", network.NamespaceName, "nft", "add", "rule", "bridge", table, "forward", "oifname", devName}
		if rule.SourceCIDR != "" {
			args = append(args, "ip", "saddr", rule.SourceCIDR)
		}
		switch strings.ToLower(rule.Protocol) {
		case "tcp", "udp":
			proto := strings.ToLower(rule.Protocol)
			args = append(args, "ip", "protocol", proto, proto)
			if rule.FromPort > 0 || rule.ToPort > 0 {
				from, to := rule.FromPort, rule.ToPort
				if from == 0 {
					from = to
				}
				if to == 0 {
					to = from
				}
				if from == to {
					args = append(args, "dport", fmt.Sprintf("%d", from))
				} else {
					args = append(args, "dport", fmt.Sprintf("%d-%d", from, to))
				}
			}
		case "icmp":
			args = append(args, "ip", "protocol", "icmp")
		case "-1", "all", "":
		default:
			return fmt.Errorf("unsupported packet filter protocol %q", rule.Protocol)
		}
		action := strings.ToLower(rule.Action)
		if action == "" {
			action = "accept"
		}
		switch action {
		case "accept", "drop":
		default:
			return fmt.Errorf("unsupported packet filter action %q", rule.Action)
		}
		args = append(args, action)
		if err := network.runner.Run(ctx, args[0], args[1:]...); err != nil {
			return err
		}
	}
	return network.runner.Run(ctx, "ip", "netns", "exec", network.NamespaceName, "nft", "add", "rule", "bridge", table, "forward", "oifname", devName, "drop")
}

func (n *NamespaceNIC) ClearIngressFilter(ctx context.Context) error {
	if n.network == nil {
		return fmt.Errorf("NIC %s is not attached to a network", n.HostVethName)
	}
	table := deriveLinuxName("fw"+n.HostVethName, "fw")
	_ = n.network.runner.Run(ctx, "ip", "netns", "exec", n.network.NamespaceName, "nft", "delete", "table", "bridge", table)
	return nil
}

func (n *TapNIC) ClearIngressFilter(ctx context.Context) error {
	if n.network == nil {
		return fmt.Errorf("tap NIC %s is not attached to a network", n.TapName)
	}
	table := deriveLinuxName("fw"+n.TapName, "fw")
	_ = n.network.runner.Run(ctx, "ip", "netns", "exec", n.network.NamespaceName, "nft", "delete", "table", "bridge", table)
	return nil
}

func (n *TapNIC) NetworkNamespace() string {
	if n.network == nil {
		return ""
	}
	return n.network.NamespaceName
}

func validateLinuxName(label, name string) error {
	if !linuxNameRE.MatchString(name) {
		return fmt.Errorf("invalid %s name %q: must match %s", label, name, linuxNameRE.String())
	}
	return nil
}

func deriveLinuxName(base, suffix string) string {
	if len(base)+len(suffix) <= 15 {
		return base + suffix
	}
	if len(suffix) >= 15 {
		return suffix[:15]
	}
	return base[:15-len(suffix)] + suffix
}

func nextIPv4(ip net.IP) net.IP {
	out := append(net.IP(nil), ip.To4()...)
	out[3]++
	return out
}
