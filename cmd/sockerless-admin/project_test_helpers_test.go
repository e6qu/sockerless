package main

// testProject builds the canonical "1 sim + 1 backend" project shape
// admin tests commonly want. Used in place of the previous
// ProjectConfig{Name, Cloud, Backend, SimPort, BackendPort} literal —
// the production code now requires explicit Instances declarations.
// Auto-allocate by passing 0 for either port.
func testProject(name string, cloud CloudType, backend BackendType, simPort, backendPort int) ProjectConfig {
	return ProjectConfig{
		Name: name,
		Instances: []Instance{
			{Name: "sim", Kind: InstanceKindSim, Cloud: cloud, Port: simPort},
			{Name: "backend", Kind: InstanceKindBackend, Cloud: cloud, Backend: backend, Port: backendPort, Sim: "sim"},
		},
	}
}

// projectInstance returns the named instance from a ProjectStatus.
// Returns the zero Instance value when not found so callers using it in
// a comparison get a deterministic empty struct.
func projectInstance(s ProjectStatus, name string) Instance {
	for _, inst := range s.Instances {
		if inst.Name == name {
			return inst.Instance
		}
	}
	return Instance{}
}
