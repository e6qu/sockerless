package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ProjectManager owns the lifecycle of every admin-managed project.
// Each project is a name + a list of Instances. ProjectManager
// registers one ProcessManager process per instance, allocates ports
// for any instance with Port==0, and orchestrates ordered start/stop
// so backends wait on their referenced simulators.
type ProjectManager struct {
	mu       sync.Mutex
	projects map[string]*ProjectConfig
	opLock   map[string]string
	pm       *ProcessManager
	reg      *Registry
	ports    *PortAllocator
	storeDir string
	client   *http.Client
}

// NewProjectManager creates a new ProjectManager.
func NewProjectManager(pm *ProcessManager, reg *Registry, storeDir string) *ProjectManager {
	return &ProjectManager{
		projects: make(map[string]*ProjectConfig),
		opLock:   make(map[string]string),
		pm:       pm,
		reg:      reg,
		ports:    NewPortAllocator(),
		storeDir: storeDir,
		client:   tracedHTTPClient(10 * time.Second),
	}
}

// Create registers a new project and its instances' processes. Each
// instance with Port==0 gets an ephemeral port allocated.
func (m *ProjectManager) Create(cfg ProjectConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cfg.Name == "" {
		return fmt.Errorf("project name is required")
	}
	if !isValidProjectName(cfg.Name) {
		return fmt.Errorf("invalid project name %q: must match [a-z0-9][a-z0-9_-]*", cfg.Name)
	}
	if _, ok := m.projects[cfg.Name]; ok {
		return fmt.Errorf("project %q already exists", cfg.Name)
	}
	if len(cfg.Instances) == 0 {
		return fmt.Errorf("project %q must declare at least one instance", cfg.Name)
	}

	// Allocate ephemeral ports for any instance with Port==0.
	autoIdx := []int{}
	for i, inst := range cfg.Instances {
		if inst.Port == 0 {
			autoIdx = append(autoIdx, i)
		}
	}
	if len(autoIdx) > 0 {
		ports, err := m.ports.Allocate(cfg.Name, len(autoIdx))
		if err != nil {
			return fmt.Errorf("port allocation: %w", err)
		}
		for j, i := range autoIdx {
			cfg.Instances[i].Port = ports[j]
		}
	}

	// Reserve any explicitly-set ports.
	var explicit []int
	for _, inst := range cfg.Instances {
		explicit = append(explicit, inst.Port)
	}
	if err := m.ports.Reserve(cfg.Name, explicit); err != nil {
		m.ports.Release(cfg.Name)
		return err
	}

	// Validate each instance now that ports are filled in.
	for _, inst := range cfg.Instances {
		if err := inst.Validate(); err != nil {
			m.ports.Release(cfg.Name)
			return fmt.Errorf("invalid instance: %w", err)
		}
	}

	if cfg.CreatedAt == "" {
		cfg.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	// Register one process per instance, building the env from its
	// kind-specific config.
	for _, inst := range cfg.Instances {
		m.pm.AddProcess(m.instanceToProcessConfig(cfg.Name, inst))
	}

	stored := cfg

	if m.storeDir != "" {
		if err := SaveProject(m.storeDir, &stored); err != nil {
			for _, inst := range cfg.Instances {
				_ = m.pm.RemoveProcess(instanceProcessName(cfg.Name, inst.Name))
			}
			m.ports.Release(cfg.Name)
			return fmt.Errorf("persist project: %w", err)
		}
	}
	m.projects[cfg.Name] = &stored
	return nil
}

// instanceToProcessConfig builds the ProcessConfig that the
// ProcessManager runs for a given instance. Per-kind env builders fill
// in cloud + simulator + log-level details.
func (m *ProjectManager) instanceToProcessConfig(projectName string, inst Instance) ProcessConfig {
	name := instanceProcessName(projectName, inst.Name)
	switch inst.Kind {
	case InstanceKindSim:
		return ProcessConfig{
			Name:   name,
			Binary: SimulatorBinary(inst.Cloud),
			Env:    sliceToEnvMap(SimulatorEnv(inst.Cloud, inst.Port, inst.Config["log_level"])),
			Addr:   fmt.Sprintf(":%d", inst.Port),
			Type:   "simulator",
		}
	case InstanceKindBackend:
		// Backend connects to its sim sibling when one is declared.
		simPort := 0
		if inst.Sim != "" {
			if sib := m.findSiblingInstance(projectName, inst.Sim); sib != nil {
				simPort = sib.Port
			}
		}
		return ProcessConfig{
			Name:   name,
			Binary: BackendBinary(inst.Backend),
			Args:   BackendArgs(inst.Port, inst.Config["log_level"]),
			Env:    sliceToEnvMap(BackendEnv(inst.Cloud, inst.Backend, simPort, projectName)),
			Addr:   fmt.Sprintf(":%d", inst.Port),
			Type:   "backend",
		}
	case InstanceKindBleephub:
		env := map[string]string{
			"BLEEPHUB_PORT":      fmt.Sprintf("%d", inst.Port),
			"BLEEPHUB_LOG_LEVEL": inst.Config["log_level"],
		}
		for k, v := range inst.Config {
			env[k] = v
		}
		return ProcessConfig{
			Name:   name,
			Binary: "bleephub",
			Env:    env,
			Addr:   fmt.Sprintf(":%d", inst.Port),
			Type:   "bleephub",
		}
	}
	return ProcessConfig{Name: name, Addr: fmt.Sprintf(":%d", inst.Port)}
}

// findSiblingInstance returns the named instance from the same project,
// or nil. Caller must hold m.mu when reading m.projects.
func (m *ProjectManager) findSiblingInstance(projectName, instanceName string) *Instance {
	p, ok := m.projects[projectName]
	if !ok {
		return nil
	}
	for i := range p.Instances {
		if p.Instances[i].Name == instanceName {
			return &p.Instances[i]
		}
	}
	return nil
}

// Start brings up every instance in dependency order: sims first, then
// backends (which wait for their sim's health endpoint), then bleephub
// instances. Returns the first error and rolls back partial state.
func (m *ProjectManager) Start(name string) error {
	m.mu.Lock()
	cfg, ok := m.projects[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("project %q not found", name)
	}
	if err := m.tryLockOp(name, "starting"); err != nil {
		m.mu.Unlock()
		return err
	}
	instances := append([]Instance(nil), cfg.Instances...)
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.unlockOp(name); m.mu.Unlock() }()

	started := []string{}
	startOrder := orderInstancesForStart(instances)
	for _, inst := range startOrder {
		procName := instanceProcessName(name, inst.Name)
		if err := m.pm.Start(procName); err != nil {
			m.rollback(name, started)
			return fmt.Errorf("start %s/%s: %w", name, inst.Name, err)
		}
		if err := m.waitForInstanceHealth(inst); err != nil {
			_ = m.pm.Stop(procName)
			m.rollback(name, started)
			return fmt.Errorf("%s/%s health check: %w", name, inst.Name, err)
		}
		// Backends post-start: run cloud bootstrap (cluster create etc.)
		// against their sim sibling.
		if inst.Kind == InstanceKindBackend && inst.Sim != "" {
			sim := findInstanceByName(instances, inst.Sim)
			if sim != nil {
				simAddr := fmt.Sprintf("http://localhost:%d", sim.Port)
				if err := BootstrapSimulator(inst.Cloud, inst.Backend, simAddr, name, m.client); err != nil {
					_ = m.pm.Stop(procName)
					m.rollback(name, started)
					return fmt.Errorf("%s/%s bootstrap: %w", name, inst.Name, err)
				}
			}
		}
		started = append(started, procName)
	}
	return nil
}

// rollback stops every process in `started` in reverse order.
func (m *ProjectManager) rollback(projectName string, started []string) {
	for i := len(started) - 1; i >= 0; i-- {
		_ = m.pm.Stop(started[i])
	}
}

// orderInstancesForStart returns instances ordered so sims start before
// backends that reference them, and bleephub instances start last.
func orderInstancesForStart(instances []Instance) []Instance {
	sims := []Instance{}
	backends := []Instance{}
	bleephubs := []Instance{}
	for _, inst := range instances {
		switch inst.Kind {
		case InstanceKindSim:
			sims = append(sims, inst)
		case InstanceKindBackend:
			backends = append(backends, inst)
		case InstanceKindBleephub:
			bleephubs = append(bleephubs, inst)
		}
	}
	out := make([]Instance, 0, len(instances))
	out = append(out, sims...)
	out = append(out, backends...)
	out = append(out, bleephubs...)
	return out
}

func findInstanceByName(instances []Instance, name string) *Instance {
	for i := range instances {
		if instances[i].Name == name {
			return &instances[i]
		}
	}
	return nil
}

// waitForInstanceHealth polls the right health endpoint for the
// instance's kind.
func (m *ProjectManager) waitForInstanceHealth(inst Instance) error {
	addr := fmt.Sprintf("http://localhost:%d", inst.Port)
	switch inst.Kind {
	case InstanceKindSim, InstanceKindBleephub:
		return waitForHealth(m.client, addr, "/health", 10*time.Second)
	case InstanceKindBackend:
		return waitForHealth(m.client, addr, "/internal/v1/healthz", 15*time.Second)
	}
	return nil
}

// Stop performs reverse-order shutdown.
func (m *ProjectManager) Stop(name string) error {
	m.mu.Lock()
	cfg, ok := m.projects[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("project %q not found", name)
	}
	if err := m.tryLockOp(name, "stopping"); err != nil {
		m.mu.Unlock()
		return err
	}
	instances := append([]Instance(nil), cfg.Instances...)
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.unlockOp(name); m.mu.Unlock() }()

	return m.stopProcesses(name, instances)
}

// stopProcesses stops every instance in reverse-of-start order.
func (m *ProjectManager) stopProcesses(name string, instances []Instance) error {
	order := orderInstancesForStart(instances)
	var errs []error
	for i := len(order) - 1; i >= 0; i-- {
		if err := m.stopIfRunning(instanceProcessName(name, order[i].Name)); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Delete stops a project and removes its processes + persisted file.
func (m *ProjectManager) Delete(name string) error {
	m.mu.Lock()
	cfg, ok := m.projects[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("project %q not found", name)
	}
	if err := m.tryLockOp(name, "deleting"); err != nil {
		m.mu.Unlock()
		return err
	}
	instances := append([]Instance(nil), cfg.Instances...)
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.unlockOp(name); m.mu.Unlock() }()

	_ = m.stopProcesses(name, instances)
	for _, inst := range instances {
		_ = m.pm.RemoveProcess(instanceProcessName(name, inst.Name))
	}
	m.mu.Lock()
	delete(m.projects, name)
	m.mu.Unlock()
	m.ports.Release(name)
	if m.storeDir != "" {
		_ = DeleteProjectFile(m.storeDir, name)
	}
	return nil
}

// Get returns a project's status.
func (m *ProjectManager) Get(name string) (ProjectStatus, bool) {
	m.mu.Lock()
	cfg, ok := m.projects[name]
	if !ok {
		m.mu.Unlock()
		return ProjectStatus{}, false
	}
	cfgCopy := *cfg
	m.mu.Unlock()
	return m.buildStatus(cfgCopy), true
}

// List returns all projects with status.
func (m *ProjectManager) List() []ProjectStatus {
	m.mu.Lock()
	configs := make([]ProjectConfig, 0, len(m.projects))
	for _, cfg := range m.projects {
		configs = append(configs, *cfg)
	}
	m.mu.Unlock()

	statuses := make([]ProjectStatus, 0, len(configs))
	for _, cfg := range configs {
		statuses = append(statuses, m.buildStatus(cfg))
	}
	return statuses
}

// Logs returns logs for a specific instance, or aggregated across all.
// The `component` query param matches an instance's Name (or "all"/"").
func (m *ProjectManager) Logs(name, component string, lines int) ([]string, error) {
	m.mu.Lock()
	cfg, ok := m.projects[name]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("project %q not found", name)
	}
	instances := append([]Instance(nil), cfg.Instances...)
	m.mu.Unlock()

	if component == "" || component == "all" {
		var allLogs []string
		for _, inst := range instances {
			logs, err := m.pm.GetLogs(instanceProcessName(name, inst.Name), lines)
			if err == nil {
				for _, line := range logs {
					allLogs = append(allLogs, "["+inst.Name+"] "+line)
				}
			}
		}
		return allLogs, nil
	}

	for _, inst := range instances {
		if inst.Name == component {
			return m.pm.GetLogs(instanceProcessName(name, inst.Name), lines)
		}
	}
	return nil, fmt.Errorf("invalid component %q", component)
}

// Connection returns Docker/Podman connection info for the project's
// first backend instance. Returns an empty ProjectConnection when the
// project has no backend instance declared.
func (m *ProjectManager) Connection(name string) (ProjectConnection, error) {
	m.mu.Lock()
	cfg, ok := m.projects[name]
	if !ok {
		m.mu.Unlock()
		return ProjectConnection{}, fmt.Errorf("project %q not found", name)
	}
	c := *cfg
	m.mu.Unlock()

	var backend, sim *Instance
	for i := range c.Instances {
		inst := &c.Instances[i]
		if backend == nil && inst.Kind == InstanceKindBackend {
			backend = inst
		}
		if sim == nil && inst.Kind == InstanceKindSim {
			sim = inst
		}
	}
	if backend == nil {
		return ProjectConnection{}, nil
	}

	dockerHost := fmt.Sprintf("tcp://localhost:%d", backend.Port)
	conn := ProjectConnection{
		DockerHost:       dockerHost,
		EnvExport:        fmt.Sprintf("export DOCKER_HOST=%s", dockerHost),
		PodmanConnection: fmt.Sprintf("podman system connection add %s %s", c.Name, dockerHost),
		BackendAddr:      fmt.Sprintf("http://localhost:%d", backend.Port),
	}
	if sim != nil {
		conn.SimulatorAddr = fmt.Sprintf("http://localhost:%d", sim.Port)
	}
	return conn, nil
}

// StopAll stops all running projects.
func (m *ProjectManager) StopAll() {
	m.mu.Lock()
	configs := make([]ProjectConfig, 0, len(m.projects))
	for _, cfg := range m.projects {
		configs = append(configs, *cfg)
	}
	m.mu.Unlock()
	for _, cfg := range configs {
		_ = m.stopProcesses(cfg.Name, cfg.Instances)
	}
}

// LoadProject loads a persisted project config and registers its
// processes without starting them.
func (m *ProjectManager) LoadProject(cfg *ProjectConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	stored := *cfg
	m.projects[cfg.Name] = &stored

	ports := []int{}
	for _, inst := range cfg.Instances {
		ports = append(ports, inst.Port)
	}
	if err := m.ports.Reserve(cfg.Name, ports); err != nil {
		log.Printf("warning: port conflict loading project %q: %v", cfg.Name, err)
	}
	for _, inst := range cfg.Instances {
		m.pm.AddProcess(m.instanceToProcessConfig(cfg.Name, inst))
	}
}

// buildStatus aggregates per-instance process status into the project's
// status.
func (m *ProjectManager) buildStatus(cfg ProjectConfig) ProjectStatus {
	instanceStatuses := make([]ProjectInstanceStatus, 0, len(cfg.Instances))
	running, failed, starting, stopping := 0, 0, 0, 0
	for _, inst := range cfg.Instances {
		info, _ := m.pm.Get(instanceProcessName(cfg.Name, inst.Name))
		instanceStatuses = append(instanceStatuses, ProjectInstanceStatus{Instance: inst, Status: info.Status})
		switch info.Status {
		case "running":
			running++
		case "failed":
			failed++
		case "starting":
			starting++
		case "stopping":
			stopping++
		}
	}

	status := "stopped"
	switch {
	case stopping > 0:
		status = "stopping"
	case starting > 0:
		status = "starting"
	case running == len(cfg.Instances) && len(cfg.Instances) > 0:
		status = "running"
	case running > 0:
		status = "partial"
	case failed > 0:
		status = "failed"
	}
	return ProjectStatus{
		ProjectConfig: cfg,
		Status:        status,
		Instances:     instanceStatuses,
	}
}

// stopIfRunning stops a process, ignoring "not running" errors.
func (m *ProjectManager) stopIfRunning(name string) error {
	err := m.pm.Stop(name)
	if err != nil && strings.Contains(err.Error(), "is not running") {
		return nil
	}
	return err
}

// waitForHealth polls a health endpoint until it returns 200 or times out.
func waitForHealth(client *http.Client, addr, path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := client.Get(addr + path)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("health check %s%s timed out after %s", addr, path, timeout)
}

// sliceToEnvMap converts KEY=VALUE slices to a map.
func sliceToEnvMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		for i := 0; i < len(e); i++ {
			if e[i] == '=' {
				m[e[:i]] = e[i+1:]
				break
			}
		}
	}
	return m
}

// tryLockOp attempts to set an operation lock for a project. Must be called under m.mu.
func (m *ProjectManager) tryLockOp(name, op string) error {
	if current, ok := m.opLock[name]; ok {
		return fmt.Errorf("project %q is busy (%s)", name, current)
	}
	m.opLock[name] = op
	return nil
}

// unlockOp clears the operation lock for a project. Must be called under m.mu.
func (m *ProjectManager) unlockOp(name string) {
	delete(m.opLock, name)
}
