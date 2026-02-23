//go:build linux

package core

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
)

// NetnsManager manages Linux network namespaces for container networking.
type NetnsManager struct {
	mu         sync.Mutex
	namespaces map[string]*netnsInfo
	basePath   string
}

type netnsInfo struct {
	Path       string // /var/run/netns/sockerless-<shortID>
	BridgeName string // br-<shortID>
	BridgeAddr string // gateway IP
	Subnet     string
}

// NewNetnsManager creates a new namespace manager.
func NewNetnsManager() *NetnsManager {
	return &NetnsManager{
		namespaces: make(map[string]*netnsInfo),
		basePath:   "/var/run/netns",
	}
}

// Available checks if we have sufficient privileges for network namespace operations.
func (m *NetnsManager) Available() bool {
	return os.Getuid() == 0
}

// CreateNamespace creates a network namespace with a bridge interface.
func (m *NetnsManager) CreateNamespace(networkID, name, bridgeIP, subnet string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	shortID := networkID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}

	nsName := "sockerless-" + shortID
	bridgeName := "br-" + shortID

	// Create network namespace
	if err := exec.Command("ip", "netns", "add", nsName).Run(); err != nil {
		return fmt.Errorf("failed to create netns %s: %w", nsName, err)
	}

	// Create bridge
	if err := exec.Command("ip", "link", "add", bridgeName, "type", "bridge").Run(); err != nil {
		_ = exec.Command("ip", "netns", "del", nsName).Run()
		return fmt.Errorf("failed to create bridge %s: %w", bridgeName, err)
	}

	// Assign IP to bridge
	cidr := bridgeIP + "/16"
	if subnet != "" {
		// Extract mask from subnet
		if idx := len(subnet) - 1; idx > 0 {
			for i := len(subnet) - 1; i >= 0; i-- {
				if subnet[i] == '/' {
					cidr = bridgeIP + subnet[i:]
					break
				}
			}
		}
	}
	if err := exec.Command("ip", "addr", "add", cidr, "dev", bridgeName).Run(); err != nil {
		_ = exec.Command("ip", "link", "del", bridgeName).Run()
		_ = exec.Command("ip", "netns", "del", nsName).Run()
		return fmt.Errorf("failed to assign IP to bridge: %w", err)
	}

	// Bring bridge up
	if err := exec.Command("ip", "link", "set", bridgeName, "up").Run(); err != nil {
		_ = exec.Command("ip", "link", "del", bridgeName).Run()
		_ = exec.Command("ip", "netns", "del", nsName).Run()
		return fmt.Errorf("failed to bring bridge up: %w", err)
	}

	m.namespaces[networkID] = &netnsInfo{
		Path:       m.basePath + "/" + nsName,
		BridgeName: bridgeName,
		BridgeAddr: bridgeIP,
		Subnet:     subnet,
	}
	return nil
}

// DeleteNamespace removes a network namespace and its bridge.
func (m *NetnsManager) DeleteNamespace(networkID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, ok := m.namespaces[networkID]
	if !ok {
		return nil
	}

	_ = exec.Command("ip", "link", "del", info.BridgeName).Run()

	shortID := networkID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	_ = exec.Command("ip", "netns", "del", "sockerless-"+shortID).Run()

	delete(m.namespaces, networkID)
	return nil
}

// CreateVethPair creates a veth pair connecting a container to the bridge.
func (m *NetnsManager) CreateVethPair(networkID, containerID, containerIP string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, ok := m.namespaces[networkID]
	if !ok {
		return fmt.Errorf("namespace for network %s not found", networkID)
	}

	shortCID := containerID
	if len(shortCID) > 8 {
		shortCID = shortCID[:8]
	}

	vethHost := "veth-" + shortCID
	vethContainer := "eth0-" + shortCID
	nsName := "sockerless-" + networkID[:12]

	// Create veth pair
	if err := exec.Command("ip", "link", "add", vethHost, "type", "veth", "peer", "name", vethContainer).Run(); err != nil {
		return fmt.Errorf("failed to create veth pair: %w", err)
	}

	// Attach host end to bridge
	if err := exec.Command("ip", "link", "set", vethHost, "master", info.BridgeName).Run(); err != nil {
		_ = exec.Command("ip", "link", "del", vethHost).Run()
		return fmt.Errorf("failed to attach veth to bridge: %w", err)
	}

	// Move container end to namespace
	if err := exec.Command("ip", "link", "set", vethContainer, "netns", nsName).Run(); err != nil {
		_ = exec.Command("ip", "link", "del", vethHost).Run()
		return fmt.Errorf("failed to move veth to netns: %w", err)
	}

	// Configure IP on container end
	cidr := containerIP + "/16"
	if info.Subnet != "" {
		for i := len(info.Subnet) - 1; i >= 0; i-- {
			if info.Subnet[i] == '/' {
				cidr = containerIP + info.Subnet[i:]
				break
			}
		}
	}
	_ = exec.Command("ip", "netns", "exec", nsName, "ip", "addr", "add", cidr, "dev", vethContainer).Run()
	_ = exec.Command("ip", "netns", "exec", nsName, "ip", "link", "set", vethContainer, "up").Run()
	_ = exec.Command("ip", "link", "set", vethHost, "up").Run()

	return nil
}

// RemoveVethPair removes a container's veth pair.
func (m *NetnsManager) RemoveVethPair(networkID, containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	shortCID := containerID
	if len(shortCID) > 8 {
		shortCID = shortCID[:8]
	}
	vethHost := "veth-" + shortCID

	// Deleting one end removes both
	_ = exec.Command("ip", "link", "del", vethHost).Run()
	return nil
}
