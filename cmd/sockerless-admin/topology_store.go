package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// TopologyFileName is the canonical filename admin reads + writes at
// the repo root. Locked to one path so editor + tooling + version-
// control all see the same source of truth.
const TopologyFileName = "sockerless.yaml"

// Topology is the on-disk representation of every admin-managed
// project + instance. The YAML form is operator-friendly (read by
// hand, version-controlled); admin never invents fields not declared
// here, so editing in either place is safe.
type Topology struct {
	Projects []ProjectConfig `yaml:"projects,omitempty" json:"projects,omitempty"`
	Ports    PortConfig      `yaml:"ports,omitempty" json:"ports,omitempty"`
}

// PortConfig describes the per-kind port allocation pool admin draws
// from when the operator picks "auto" rather than a literal port. The
// pool is global across projects so two projects can't accidentally
// pick the same port.
type PortConfig struct {
	Ranges map[InstanceKind]PortRange `yaml:"ranges,omitempty" json:"ranges,omitempty"`
}

// PortRange is a closed interval. Allocation walks From..To inclusive
// and skips ports already claimed by another instance in the topology.
type PortRange struct {
	From int `yaml:"from" json:"from"`
	To   int `yaml:"to" json:"to"`
}

// DefaultPortRanges is the seed used when sockerless.yaml doesn't
// declare a Ports.Ranges block. Mirrors the historical defaults in
// make/stack.mk so day-1 behaviour is unchanged.
func DefaultPortRanges() map[InstanceKind]PortRange {
	return map[InstanceKind]PortRange{
		InstanceKindSim:      {From: 4500, To: 4999},
		InstanceKindBackend:  {From: 3300, To: 3399},
		InstanceKindBleephub: {From: 5500, To: 5599},
	}
}

// LoadTopology reads + parses the topology file at path. Returns
// (nil, os.ErrNotExist) when the file is missing so callers can
// distinguish "no file yet" from "file is broken".
func LoadTopology(path string) (*Topology, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Topology
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &t, nil
}

// SaveTopology writes t to path atomically (write tmp, rename) so a
// crashed admin can't leave a half-written YAML behind.
func SaveTopology(path string, t *Topology) error {
	if t == nil {
		return errors.New("nil topology")
	}
	if err := t.Validate(); err != nil {
		return fmt.Errorf("validate before save: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(t)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Validate runs cross-cutting checks on the topology:
//   - project names unique
//   - instance names unique within a project
//   - per-instance Validate
//   - sim references on backend instances point to a real sim in the
//     same project
//   - port ranges are well-formed
//   - no two instances claim the same port (global)
//
// Fail-loud — any issue returns an error with enough context for the
// operator to find + fix it.
func (t *Topology) Validate() error {
	if t == nil {
		return errors.New("nil topology")
	}
	for kind, r := range t.Ports.Ranges {
		if !IsValidInstanceKind(kind) {
			return fmt.Errorf("ports.ranges: unknown kind %q", kind)
		}
		if r.From <= 0 || r.To <= 0 || r.From > r.To {
			return fmt.Errorf("ports.ranges[%q]: invalid range [%d, %d]", kind, r.From, r.To)
		}
	}

	seenProjects := map[string]bool{}
	seenPorts := map[int]string{} // port -> "project/instance"

	for pi, p := range t.Projects {
		if !isValidProjectName(p.Name) {
			return fmt.Errorf("projects[%d]: invalid name %q", pi, p.Name)
		}
		if seenProjects[p.Name] {
			return fmt.Errorf("project %q: duplicate", p.Name)
		}
		seenProjects[p.Name] = true

		seenInstances := map[string]bool{}
		simNames := map[string]bool{}
		for ii, inst := range p.Instances {
			if seenInstances[inst.Name] {
				return fmt.Errorf("project %q: duplicate instance name %q", p.Name, inst.Name)
			}
			seenInstances[inst.Name] = true
			if err := inst.Validate(); err != nil {
				return fmt.Errorf("project %q instance[%d]: %w", p.Name, ii, err)
			}
			if inst.Kind == InstanceKindSim {
				simNames[inst.Name] = true
			}
			if owner, taken := seenPorts[inst.Port]; taken {
				return fmt.Errorf("project %q instance %q: port %d already claimed by %s",
					p.Name, inst.Name, inst.Port, owner)
			}
			seenPorts[inst.Port] = p.Name + "/" + inst.Name
		}
		// Backend → Sim references must point to a sim in the same project.
		for _, inst := range p.Instances {
			if inst.Kind == InstanceKindBackend && inst.Sim != "" && !simNames[inst.Sim] {
				return fmt.Errorf("project %q instance %q: sim ref %q not found in project (sims here: %v)",
					p.Name, inst.Name, inst.Sim, sortedKeys(simNames))
			}
		}
	}
	return nil
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// DefaultTopologyPath is the conventional location admin reads from
// + writes to: `<repo-root>/sockerless.yaml`. Resolved relative to
// the current working directory so admin invocations from anywhere
// in the repo find the same file.
func DefaultTopologyPath() string {
	return TopologyFileName
}
