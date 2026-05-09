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
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.replaceLocked(next)
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

// AddProject appends p to the topology and persists. Project name
// must not already exist.
func (m *TopologyManager) AddProject(p ProjectConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.current.Projects {
		if existing.Name == p.Name {
			return fmt.Errorf("project %q already exists", p.Name)
		}
	}
	next := deepCopyTopology(m.current)
	next.Projects = append(next.Projects, p)
	return m.replaceLocked(next)
}

// RemoveProject deletes a project + all its instances. Returns an
// error if the project is unknown.
func (m *TopologyManager) RemoveProject(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := -1
	for i, p := range m.current.Projects {
		if p.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("project %q not found", name)
	}
	next := deepCopyTopology(m.current)
	next.Projects = append(next.Projects[:idx], next.Projects[idx+1:]...)
	return m.replaceLocked(next)
}

// AddInstance appends inst to project's instance list. Project must
// exist; instance name must be unique within the project.
func (m *TopologyManager) AddInstance(project string, inst Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pi := -1
	for i, p := range m.current.Projects {
		if p.Name == project {
			pi = i
			break
		}
	}
	if pi < 0 {
		return fmt.Errorf("project %q not found", project)
	}
	for _, existing := range m.current.Projects[pi].Instances {
		if existing.Name == inst.Name {
			return fmt.Errorf("instance %q already exists in project %q", inst.Name, project)
		}
	}
	next := deepCopyTopology(m.current)
	next.Projects[pi].Instances = append(next.Projects[pi].Instances, inst)
	return m.replaceLocked(next)
}

// RemoveInstance deletes an instance from a project.
func (m *TopologyManager) RemoveInstance(project, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pi := -1
	for i, p := range m.current.Projects {
		if p.Name == project {
			pi = i
			break
		}
	}
	if pi < 0 {
		return fmt.Errorf("project %q not found", project)
	}
	ii := -1
	for j, inst := range m.current.Projects[pi].Instances {
		if inst.Name == name {
			ii = j
			break
		}
	}
	if ii < 0 {
		return fmt.Errorf("instance %q not found in project %q", name, project)
	}
	next := deepCopyTopology(m.current)
	next.Projects[pi].Instances = append(next.Projects[pi].Instances[:ii], next.Projects[pi].Instances[ii+1:]...)
	return m.replaceLocked(next)
}

// UpdateInstance replaces an existing instance in-place. The new
// Instance.Name must equal the current name (renames go through
// remove + add to keep the UI's URL stable).
func (m *TopologyManager) UpdateInstance(project string, inst Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pi := -1
	for i, p := range m.current.Projects {
		if p.Name == project {
			pi = i
			break
		}
	}
	if pi < 0 {
		return fmt.Errorf("project %q not found", project)
	}
	ii := -1
	for j, existing := range m.current.Projects[pi].Instances {
		if existing.Name == inst.Name {
			ii = j
			break
		}
	}
	if ii < 0 {
		return fmt.Errorf("instance %q not found in project %q", inst.Name, project)
	}
	next := deepCopyTopology(m.current)
	next.Projects[pi].Instances[ii] = inst
	return m.replaceLocked(next)
}

// replaceLocked validates next, persists it, and adopts it. Caller
// must already hold m.mu (write lock).
func (m *TopologyManager) replaceLocked(next Topology) error {
	if err := next.Validate(); err != nil {
		return fmt.Errorf("validate: %w", err)
	}
	if err := SaveTopology(m.path, &next); err != nil {
		return fmt.Errorf("save: %w", err)
	}
	cp := deepCopyTopology(&next)
	m.current = &cp
	return nil
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
