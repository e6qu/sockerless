package main

import (
	"errors"
	"fmt"
	"os"
	"sync"
)

// TopologyManager owns the in-memory copy of sockerless.yaml. It's
// the single point that reads + writes the file so concurrent admin
// HTTP handlers can't race each other to a half-written state.
//
// Lifecycle:
//   - NewTopologyManager loads the file from `path`. Missing file is
//     not an error (returns an empty Topology). Operators routinely
//     start admin in a fresh repo with no sockerless.yaml.
//   - LoadOrMigrate (used at startup) prefers the YAML; if absent,
//     migrates from the legacy per-project JSONs at `legacyDir` and
//     writes the YAML so subsequent reads use the new path.
//   - Get returns a defensive copy so callers can't mutate internal state.
//   - Replace validates + writes + replaces atomically.
type TopologyManager struct {
	mu        sync.RWMutex
	path      string
	legacyDir string
	current   *Topology
}

// NewTopologyManager constructs a manager bound to a YAML path + a
// legacy-projects-dir for one-shot migration. Either may be empty
// (legacyDir empty disables migration).
func NewTopologyManager(path, legacyDir string) *TopologyManager {
	return &TopologyManager{
		path:      path,
		legacyDir: legacyDir,
		current:   &Topology{Ports: PortConfig{Ranges: DefaultPortRanges()}},
	}
}

// LoadOrMigrate is the startup hook. Returns nil on success — admin
// is allowed to come up with an empty topology when neither the YAML
// nor any legacy JSONs exist.
func (m *TopologyManager) LoadOrMigrate() error {
	t, err := LoadTopology(m.path)
	if err == nil {
		if vErr := t.Validate(); vErr != nil {
			return fmt.Errorf("loaded %s but it failed validation: %w", m.path, vErr)
		}
		m.mu.Lock()
		m.current = t
		m.mu.Unlock()
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// YAML missing — try legacy migration if a dir was supplied.
	if m.legacyDir == "" {
		return nil
	}
	migrated, mErr := MigrateLegacyProjects(m.legacyDir)
	if mErr != nil {
		if errors.Is(mErr, os.ErrNotExist) {
			return nil
		}
		return mErr
	}
	if vErr := migrated.Validate(); vErr != nil {
		return fmt.Errorf("legacy migration produced invalid topology: %w", vErr)
	}
	if sErr := SaveTopology(m.path, migrated); sErr != nil {
		return fmt.Errorf("write migrated topology to %s: %w", m.path, sErr)
	}
	m.mu.Lock()
	m.current = migrated
	m.mu.Unlock()
	return nil
}

// Get returns a defensive copy of the current topology.
func (m *TopologyManager) Get() Topology {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return deepCopyTopology(m.current)
}

// Replace validates `next`, writes it to disk, and adopts it as the
// current topology. Returns an error without mutating state if validation
// or the write fails.
func (m *TopologyManager) Replace(next Topology) error {
	if err := next.Validate(); err != nil {
		return fmt.Errorf("validate: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := SaveTopology(m.path, &next); err != nil {
		return fmt.Errorf("save: %w", err)
	}
	cp := deepCopyTopology(&next)
	m.current = &cp
	return nil
}

// Path returns the file path the manager reads/writes (for diagnostics).
func (m *TopologyManager) Path() string { return m.path }

// InstanceRef pairs a project + instance name so a lookup by full key
// fits in one struct.
type InstanceRef struct {
	Project  string   `json:"project"`
	Instance Instance `json:"instance"`
}

// Instances returns a flat list of every instance across every project,
// each annotated with its project name. Useful for the admin UI's
// instance-tree view + the per-instance lifecycle endpoints.
func (m *TopologyManager) Instances() []InstanceRef {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []InstanceRef{}
	for _, p := range m.current.Projects {
		for _, inst := range p.Instances {
			out = append(out, InstanceRef{Project: p.Name, Instance: inst})
		}
	}
	return out
}

// FindInstance looks up a single instance by (project, name). Returns
// the InstanceRef + true on hit, zero-value + false otherwise.
func (m *TopologyManager) FindInstance(project, instance string) (InstanceRef, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.current.Projects {
		if p.Name != project {
			continue
		}
		for _, inst := range p.Instances {
			if inst.Name == instance {
				return InstanceRef{Project: p.Name, Instance: inst}, true
			}
		}
	}
	return InstanceRef{}, false
}

func deepCopyTopology(t *Topology) Topology {
	if t == nil {
		return Topology{}
	}
	out := Topology{
		Projects: make([]ProjectConfig, len(t.Projects)),
		Ports:    PortConfig{Ranges: make(map[InstanceKind]PortRange, len(t.Ports.Ranges))},
	}
	for k, v := range t.Ports.Ranges {
		out.Ports.Ranges[k] = v
	}
	for i, p := range t.Projects {
		cp := p
		if len(p.Instances) > 0 {
			cp.Instances = make([]Instance, len(p.Instances))
			for j, inst := range p.Instances {
				ic := inst
				if len(inst.Config) > 0 {
					ic.Config = make(map[string]string, len(inst.Config))
					for ck, cv := range inst.Config {
						ic.Config[ck] = cv
					}
				}
				cp.Instances[j] = ic
			}
		}
		out.Projects[i] = cp
	}
	return out
}
