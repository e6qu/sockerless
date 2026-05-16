package main

import (
	"fmt"
	"net"
)

// AllocatePort returns the first port in the configured range for
// `kind` that is (a) not already claimed by any instance in the
// topology and (b) currently free on the host (TCP listen check).
//
// Used by admin lifecycle endpoints when the
// operator picks "auto" rather than a literal port for a new instance.
//
// Fail-loud: returns an error when the kind has no configured range
// or every candidate port is taken — admin surfaces the error rather
// than silently picking an out-of-pool port.
func (m *TopologyManager) AllocatePort(kind InstanceKind) (int, error) {
	m.mu.RLock()
	rangesCopy := m.current.Ports.Ranges
	r, ok := rangesCopy[kind]
	if !ok {
		// Defaults are seeded by LoadOrMigrate, so this only fires when
		// the operator wrote a sockerless.yaml that explicitly drops the
		// pool for this kind.
		m.mu.RUnlock()
		return 0, fmt.Errorf("port allocator: no range configured for kind %q", kind)
	}
	taken := make(map[int]bool, len(m.current.Projects)*4)
	for _, p := range m.current.Projects {
		for _, inst := range p.Instances {
			taken[inst.Port] = true
		}
	}
	m.mu.RUnlock()

	for port := r.From; port <= r.To; port++ {
		if taken[port] {
			continue
		}
		if !portFree(port) {
			continue
		}
		return port, nil
	}
	return 0, fmt.Errorf("port allocator: range [%d, %d] for kind %q exhausted (%d ports already claimed)",
		r.From, r.To, kind, len(taken))
}

// portFree returns true if the host has no listener on `port` right
// now (TCP4 + TCP6 both checked via 0.0.0.0). Best-effort — a TOCTOU
// gap exists between this check and the eventual bind, but on a
// single-host dev machine that's vanishingly rare and the bind would
// just fail loudly.
func portFree(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
