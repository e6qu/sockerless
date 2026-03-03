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

// ProjectManager manages projects with orchestrated lifecycle.
type ProjectManager struct {
	mu       sync.Mutex
	projects map[string]*ProjectConfig
	opLock   map[string]string // per-project operation guard (name -> op)
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
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Create creates a new project and registers its 3 processes.
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
	if !IsValidCloud(cfg.Cloud) {
		return fmt.Errorf("invalid cloud %q", cfg.Cloud)
	}
	if !IsValidBackend(cfg.Cloud, cfg.Backend) {
		return fmt.Errorf("invalid backend %q for cloud %q", cfg.Backend, cfg.Cloud)
	}

	// Allocate ports for any that are 0 (auto)
	needed := 0
	if cfg.SimPort == 0 {
		needed++
	}
	if cfg.BackendPort == 0 {
		needed++
	}
	if cfg.FrontendPort == 0 {
		needed++
	}
	if cfg.FrontendMgmtPort == 0 {
		needed++
	}

	if needed > 0 {
		ports, err := m.ports.Allocate(cfg.Name, needed)
		if err != nil {
			return fmt.Errorf("port allocation: %w", err)
		}
		idx := 0
		if cfg.SimPort == 0 {
			cfg.SimPort = ports[idx]
			idx++
		}
		if cfg.BackendPort == 0 {
			cfg.BackendPort = ports[idx]
			idx++
		}
		if cfg.FrontendPort == 0 {
			cfg.FrontendPort = ports[idx]
			idx++
		}
		if cfg.FrontendMgmtPort == 0 {
			cfg.FrontendMgmtPort = ports[idx]
		}
	}

	// Reserve explicit ports (non-zero ports not auto-allocated above)
	var explicitPorts []int
	for _, p := range []int{cfg.SimPort, cfg.BackendPort, cfg.FrontendPort, cfg.FrontendMgmtPort} {
		if p > 0 {
			explicitPorts = append(explicitPorts, p)
		}
	}
	if len(explicitPorts) > 0 {
		if err := m.ports.Reserve(cfg.Name, explicitPorts); err != nil {
			m.ports.Release(cfg.Name)
			return err
		}
	}

	if cfg.CreatedAt == "" {
		cfg.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	// Register 3 processes
	simName, backendName, frontendName := processNames(cfg.Name)

	simEnvSlice := SimulatorEnv(cfg.Cloud, cfg.SimPort, cfg.LogLevel)
	simEnvMap := sliceToEnvMap(simEnvSlice)
	m.pm.AddProcess(ProcessConfig{
		Name:   simName,
		Binary: SimulatorBinary(cfg.Cloud),
		Env:    simEnvMap,
		Addr:   fmt.Sprintf(":%d", cfg.SimPort),
		Type:   "simulator",
	})

	backendEnvSlice := BackendEnv(cfg.Cloud, cfg.Backend, cfg.SimPort, cfg.Name)
	backendEnvMap := sliceToEnvMap(backendEnvSlice)
	m.pm.AddProcess(ProcessConfig{
		Name:   backendName,
		Binary: BackendBinary(cfg.Backend),
		Args:   BackendArgs(cfg.BackendPort, cfg.LogLevel),
		Env:    backendEnvMap,
		Addr:   fmt.Sprintf(":%d", cfg.BackendPort),
		Type:   "backend",
	})

	m.pm.AddProcess(ProcessConfig{
		Name:   frontendName,
		Binary: "sockerless-frontend-docker",
		Args:   FrontendArgs(cfg.FrontendPort, cfg.BackendPort, cfg.FrontendMgmtPort, cfg.LogLevel),
		Addr:   fmt.Sprintf(":%d", cfg.FrontendPort),
		Type:   "frontend",
	})

	stored := cfg

	// Persist before in-memory registration so failures can be rolled back
	if m.storeDir != "" {
		if err := SaveProject(m.storeDir, &stored); err != nil {
			_ = m.pm.RemoveProcess(frontendName)
			_ = m.pm.RemoveProcess(backendName)
			_ = m.pm.RemoveProcess(simName)
			m.ports.Release(cfg.Name)
			return fmt.Errorf("persist project: %w", err)
		}
	}
	m.projects[cfg.Name] = &stored

	return nil
}

// Start performs orchestrated startup of a project's 3 processes.
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
	simName, backendName, frontendName := processNames(name)
	cloud := cfg.Cloud
	backend := cfg.Backend
	simPort := cfg.SimPort
	backendPort := cfg.BackendPort
	frontendMgmtPort := cfg.FrontendMgmtPort
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.unlockOp(name); m.mu.Unlock() }()

	// 1. Start simulator
	if err := m.pm.Start(simName); err != nil {
		return fmt.Errorf("start simulator: %w", err)
	}

	simAddr := fmt.Sprintf("http://localhost:%d", simPort)
	if err := waitForHealth(m.client, simAddr, "/health", 10*time.Second); err != nil {
		_ = m.pm.Stop(simName)
		return fmt.Errorf("simulator health check: %w", err)
	}

	// 2. Bootstrap (ECS cluster creation, etc.)
	if err := BootstrapSimulator(cloud, backend, simAddr, name, m.client); err != nil {
		_ = m.pm.Stop(simName)
		return fmt.Errorf("bootstrap: %w", err)
	}

	// 3. Start backend
	if err := m.pm.Start(backendName); err != nil {
		_ = m.pm.Stop(simName)
		return fmt.Errorf("start backend: %w", err)
	}

	backendAddr := fmt.Sprintf("http://localhost:%d", backendPort)
	if err := waitForHealth(m.client, backendAddr, "/internal/v1/healthz", 15*time.Second); err != nil {
		_ = m.pm.Stop(backendName)
		_ = m.pm.Stop(simName)
		return fmt.Errorf("backend health check: %w", err)
	}

	// 4. Start frontend
	if err := m.pm.Start(frontendName); err != nil {
		_ = m.pm.Stop(backendName)
		_ = m.pm.Stop(simName)
		return fmt.Errorf("start frontend: %w", err)
	}

	mgmtAddr := fmt.Sprintf("http://localhost:%d", frontendMgmtPort)
	if err := waitForHealth(m.client, mgmtAddr, "/healthz", 10*time.Second); err != nil {
		_ = m.pm.Stop(frontendName)
		_ = m.pm.Stop(backendName)
		_ = m.pm.Stop(simName)
		return fmt.Errorf("frontend health check: %w", err)
	}

	return nil
}

// Stop performs reverse-order shutdown of a project's processes.
func (m *ProjectManager) Stop(name string) error {
	m.mu.Lock()
	_, ok := m.projects[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("project %q not found", name)
	}
	if err := m.tryLockOp(name, "stopping"); err != nil {
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.unlockOp(name); m.mu.Unlock() }()

	return m.stopProcesses(name)
}

// stopProcesses stops a project's 3 processes in reverse order without acquiring the op lock.
func (m *ProjectManager) stopProcesses(name string) error {
	simName, backendName, frontendName := processNames(name)

	// Stop in reverse order: frontend → backend → simulator
	var errs []error
	if err := m.stopIfRunning(frontendName); err != nil {
		errs = append(errs, err)
	}
	if err := m.stopIfRunning(backendName); err != nil {
		errs = append(errs, err)
	}
	if err := m.stopIfRunning(simName); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Delete stops and removes a project and its processes.
func (m *ProjectManager) Delete(name string) error {
	m.mu.Lock()
	_, ok := m.projects[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("project %q not found", name)
	}
	if err := m.tryLockOp(name, "deleting"); err != nil {
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.unlockOp(name); m.mu.Unlock() }()

	// Stop if running (using stopProcesses directly to avoid re-acquiring op lock)
	_ = m.stopProcesses(name)

	simName, backendName, frontendName := processNames(name)

	// Remove processes
	_ = m.pm.RemoveProcess(frontendName)
	_ = m.pm.RemoveProcess(backendName)
	_ = m.pm.RemoveProcess(simName)

	m.mu.Lock()
	delete(m.projects, name)
	m.mu.Unlock()

	// Release ports
	m.ports.Release(name)

	// Delete persisted file
	if m.storeDir != "" {
		_ = DeleteProjectFile(m.storeDir, name)
	}

	return nil
}

// Get returns the status of a project.
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

// Logs returns logs for a project component.
func (m *ProjectManager) Logs(name, component string, lines int) ([]string, error) {
	m.mu.Lock()
	_, ok := m.projects[name]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("project %q not found", name)
	}
	m.mu.Unlock()

	simName, backendName, frontendName := processNames(name)

	var procName string
	switch component {
	case "sim", "simulator":
		procName = simName
	case "backend":
		procName = backendName
	case "frontend":
		procName = frontendName
	case "", "all":
		// Aggregate all logs
		var allLogs []string
		for _, pn := range []string{simName, backendName, frontendName} {
			logs, err := m.pm.GetLogs(pn, lines)
			if err == nil {
				for _, line := range logs {
					allLogs = append(allLogs, "["+componentLabel(pn, name)+"] "+line)
				}
			}
		}
		return allLogs, nil
	default:
		return nil, fmt.Errorf("invalid component %q", component)
	}

	return m.pm.GetLogs(procName, lines)
}

// Connection returns Docker/Podman connection info for a project.
func (m *ProjectManager) Connection(name string) (ProjectConnection, error) {
	m.mu.Lock()
	cfg, ok := m.projects[name]
	if !ok {
		m.mu.Unlock()
		return ProjectConnection{}, fmt.Errorf("project %q not found", name)
	}
	c := *cfg
	m.mu.Unlock()

	dockerHost := fmt.Sprintf("tcp://localhost:%d", c.FrontendPort)
	return ProjectConnection{
		DockerHost:       dockerHost,
		EnvExport:        fmt.Sprintf("export DOCKER_HOST=%s", dockerHost),
		PodmanConnection: fmt.Sprintf("podman system connection add %s %s", c.Name, dockerHost),
		SimulatorAddr:    fmt.Sprintf("http://localhost:%d", c.SimPort),
		BackendAddr:      fmt.Sprintf("http://localhost:%d", c.BackendPort),
		FrontendAddr:     fmt.Sprintf("http://localhost:%d", c.FrontendPort),
		FrontendMgmtAddr: fmt.Sprintf("http://localhost:%d", c.FrontendMgmtPort),
	}, nil
}

// StopAll stops all running projects.
func (m *ProjectManager) StopAll() {
	m.mu.Lock()
	names := make([]string, 0, len(m.projects))
	for name := range m.projects {
		names = append(names, name)
	}
	m.mu.Unlock()

	for _, name := range names {
		_ = m.stopProcesses(name)
	}
}

// LoadProject loads a persisted project config and registers its processes.
func (m *ProjectManager) LoadProject(cfg *ProjectConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stored := *cfg
	m.projects[cfg.Name] = &stored

	// Reserve ports
	if err := m.ports.Reserve(cfg.Name, []int{cfg.SimPort, cfg.BackendPort, cfg.FrontendPort, cfg.FrontendMgmtPort}); err != nil {
		log.Printf("warning: port conflict loading project %q: %v", cfg.Name, err)
	}

	// Register processes
	simName, backendName, frontendName := processNames(cfg.Name)

	simEnvSlice := SimulatorEnv(cfg.Cloud, cfg.SimPort, cfg.LogLevel)
	simEnvMap := sliceToEnvMap(simEnvSlice)
	m.pm.AddProcess(ProcessConfig{
		Name:   simName,
		Binary: SimulatorBinary(cfg.Cloud),
		Env:    simEnvMap,
		Addr:   fmt.Sprintf(":%d", cfg.SimPort),
		Type:   "simulator",
	})

	backendEnvSlice := BackendEnv(cfg.Cloud, cfg.Backend, cfg.SimPort, cfg.Name)
	backendEnvMap := sliceToEnvMap(backendEnvSlice)
	m.pm.AddProcess(ProcessConfig{
		Name:   backendName,
		Binary: BackendBinary(cfg.Backend),
		Args:   BackendArgs(cfg.BackendPort, cfg.LogLevel),
		Env:    backendEnvMap,
		Addr:   fmt.Sprintf(":%d", cfg.BackendPort),
		Type:   "backend",
	})

	m.pm.AddProcess(ProcessConfig{
		Name:   frontendName,
		Binary: "sockerless-frontend-docker",
		Args:   FrontendArgs(cfg.FrontendPort, cfg.BackendPort, cfg.FrontendMgmtPort, cfg.LogLevel),
		Addr:   fmt.Sprintf(":%d", cfg.FrontendPort),
		Type:   "frontend",
	})
}

// buildStatus builds a ProjectStatus from a config by checking process states.
func (m *ProjectManager) buildStatus(cfg ProjectConfig) ProjectStatus {
	simName, backendName, frontendName := processNames(cfg.Name)

	simInfo, _ := m.pm.Get(simName)
	backendInfo, _ := m.pm.Get(backendName)
	frontendInfo, _ := m.pm.Get(frontendName)

	status := "stopped"
	running := 0
	failed := 0
	for _, s := range []string{simInfo.Status, backendInfo.Status, frontendInfo.Status} {
		if s == "running" {
			running++
		}
		if s == "failed" {
			failed++
		}
	}

	if running == 3 {
		status = "running"
	} else if running > 0 {
		status = "partial"
	} else if failed > 0 {
		status = "failed"
	}

	// Check if any is starting
	for _, s := range []string{simInfo.Status, backendInfo.Status, frontendInfo.Status} {
		if s == "starting" {
			status = "starting"
			break
		}
	}

	// Check if any is stopping
	for _, s := range []string{simInfo.Status, backendInfo.Status, frontendInfo.Status} {
		if s == "stopping" {
			status = "stopping"
			break
		}
	}

	return ProjectStatus{
		ProjectConfig:  cfg,
		Status:         status,
		SimStatus:      simInfo.Status,
		BackendStatus:  backendInfo.Status,
		FrontendStatus: frontendInfo.Status,
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

// componentLabel returns a short label for a process name.
func componentLabel(procName, projectName string) string {
	simName, backendName, _ := processNames(projectName)
	switch procName {
	case simName:
		return "sim"
	case backendName:
		return "backend"
	default:
		return "frontend"
	}
}
