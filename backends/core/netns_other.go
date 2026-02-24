//go:build !linux

package core

// NetnsManager is a no-op on non-Linux platforms.
type NetnsManager struct{}

// NewNetnsManager creates a no-op namespace manager.
func NewNetnsManager() *NetnsManager { return &NetnsManager{} }

// Available always returns false on non-Linux platforms.
func (m *NetnsManager) Available() bool { return false }

// CreateNamespace is a no-op on non-Linux platforms.
func (m *NetnsManager) CreateNamespace(networkID, name, bridgeIP, subnet string) error { return nil }

// DeleteNamespace is a no-op on non-Linux platforms.
func (m *NetnsManager) DeleteNamespace(networkID string) error { return nil }

// CreateVethPair is a no-op on non-Linux platforms.
func (m *NetnsManager) CreateVethPair(networkID, containerID, containerIP string) error { return nil }

// RemoveVethPair is a no-op on non-Linux platforms.
func (m *NetnsManager) RemoveVethPair(networkID, containerID string) error { return nil }
