package main

import (
	"fmt"
	"regexp"
)

// InstanceKind names the kind of long-running component a project
// instance represents. A project can declare 0..N of each kind.
type InstanceKind string

const (
	// InstanceKindSim is a per-cloud simulator process (simulator-aws,
	// simulator-gcp, simulator-azure). Carries Cloud + Port.
	InstanceKindSim InstanceKind = "sim"
	// InstanceKindBackend is a sockerless backend process (one per
	// cloud product: ecs, lambda, cloudrun, gcf, aca, azf). Carries
	// Cloud + Backend + Port + optional Sim link (an instance Name
	// elsewhere in the same project).
	InstanceKindBackend InstanceKind = "backend"
	// InstanceKindBleephub is a bleephub (GitHub simulator) process.
	// Multiple may run; each gets its own port + state. Doesn't carry
	// Cloud / Backend.
	InstanceKindBleephub InstanceKind = "bleephub"
	// InstanceKindFrontendDocker is the docker-frontend admin UI.
	// Mostly orthogonal to a project but listed for completeness when
	// operators want a per-project frontend.
	InstanceKindFrontendDocker InstanceKind = "frontend-docker"
)

// IsValidInstanceKind reports whether k is one of the supported kinds.
func IsValidInstanceKind(k InstanceKind) bool {
	switch k {
	case InstanceKindSim, InstanceKindBackend, InstanceKindBleephub, InstanceKindFrontendDocker:
		return true
	}
	return false
}

// AllInstanceKinds is the closed enumeration.
var AllInstanceKinds = []InstanceKind{
	InstanceKindSim,
	InstanceKindBackend,
	InstanceKindBleephub,
	InstanceKindFrontendDocker,
}

// validInstanceNameRE constrains instance names to the same shape as
// project names so they can be used as filename + URL segments.
var validInstanceNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// IsValidInstanceName reports whether name is a legal instance label.
func IsValidInstanceName(name string) bool {
	return validInstanceNameRE.MatchString(name)
}

// Instance is one long-running component declared by a project's
// topology. Each instance is independently start/stop/rebuild-able
// from the admin UI; collectively they make up the project.
//
// The Cloud + Backend fields are kind-specific:
//   - InstanceKindSim:             Cloud required;     Backend ignored.
//   - InstanceKindBackend:         Cloud + Backend required; Sim optional.
//   - InstanceKindBleephub:        Cloud + Backend ignored.
//   - InstanceKindFrontendDocker:  Cloud + Backend ignored.
//
// Config holds the per-instance environment variables passed to the
// process at start time (e.g. SOCKERLESS_GCR_NETWORK_DISCOVERY).
// Edits made via admin write back here; the topology-file marshaller
// preserves arbitrary keys so backend-specific overrides round-trip.
type Instance struct {
	Name    string       `json:"name" yaml:"name"`
	Kind    InstanceKind `json:"kind" yaml:"kind"`
	Cloud   CloudType    `json:"cloud,omitempty" yaml:"cloud,omitempty"`
	Backend BackendType  `json:"backend,omitempty" yaml:"backend,omitempty"`
	Port    int          `json:"port" yaml:"port"`
	// Sim is the Name of another instance in the same project of kind
	// `sim` that this backend should talk to (translated to
	// SOCKERLESS_ENDPOINT_URL=http://localhost:<that-sim-port> at start
	// time). Only meaningful for InstanceKindBackend; empty = no sim.
	Sim    string            `json:"sim,omitempty" yaml:"sim,omitempty"`
	Config map[string]string `json:"config,omitempty" yaml:"config,omitempty"`
}

// Validate checks the per-kind required-field rules.
func (i Instance) Validate() error {
	if !IsValidInstanceName(i.Name) {
		return fmt.Errorf("instance name %q must match %s", i.Name, validInstanceNameRE)
	}
	if !IsValidInstanceKind(i.Kind) {
		return fmt.Errorf("instance %q: unknown kind %q (one of %v required)", i.Name, i.Kind, AllInstanceKinds)
	}
	if i.Port <= 0 {
		return fmt.Errorf("instance %q: port must be > 0 (got %d)", i.Name, i.Port)
	}
	switch i.Kind {
	case InstanceKindSim:
		if !IsValidCloud(i.Cloud) {
			return fmt.Errorf("instance %q (sim): cloud must be one of %v (got %q)", i.Name, ValidClouds(), i.Cloud)
		}
	case InstanceKindBackend:
		if !IsValidCloud(i.Cloud) {
			return fmt.Errorf("instance %q (backend): cloud must be one of %v (got %q)", i.Name, ValidClouds(), i.Cloud)
		}
		if !IsValidBackend(i.Cloud, i.Backend) {
			return fmt.Errorf("instance %q (backend): backend %q is not valid for cloud %q (one of %v required)",
				i.Name, i.Backend, i.Cloud, ValidBackends(i.Cloud))
		}
	}
	return nil
}

// DeriveLegacyInstances returns the implicit instance list for an old-
// shape ProjectConfig that pre-dates the Instances slice. Used by the
// loader so existing on-disk JSONs continue to enumerate as a project
// with one sim + one backend without a manual migration step.
//
// The names are derived from the project name so they're predictable
// across reloads.
func DeriveLegacyInstances(p ProjectConfig) []Instance {
	out := []Instance{}
	if p.SimPort > 0 {
		out = append(out, Instance{
			Name:  p.Name + "-sim",
			Kind:  InstanceKindSim,
			Cloud: p.Cloud,
			Port:  p.SimPort,
		})
	}
	if p.BackendPort > 0 {
		simName := ""
		if p.SimPort > 0 {
			simName = p.Name + "-sim"
		}
		out = append(out, Instance{
			Name:    p.Name + "-backend",
			Kind:    InstanceKindBackend,
			Cloud:   p.Cloud,
			Backend: p.Backend,
			Port:    p.BackendPort,
			Sim:     simName,
		})
	}
	return out
}
