package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// ManagedProcess represents a Sockerless component process managed by the admin server.
type ManagedProcess struct {
	Name      string
	Binary    string   // resolved path to binary
	Args      []string // e.g., ["-addr", ":9100"]
	Env       []string // additional env vars (KEY=VALUE)
	Addr      string   // expected listen address
	Type      string   // backend, frontend, simulator, coordinator
	Status    string   // stopped, starting, running, failed, stopping
	PID       int
	StartedAt time.Time
	ExitCode  int
	Logs      *RingBuffer

	cmd    *exec.Cmd
	cancel context.CancelFunc
	done   chan struct{} // closed when process exits
}

// ProcessInfo is the API-facing representation of a managed process.
type ProcessInfo struct {
	Name      string `json:"name"`
	Binary    string `json:"binary"`
	Status    string `json:"status"`
	PID       int    `json:"pid"`
	Addr      string `json:"addr"`
	StartedAt string `json:"started_at"`
	ExitCode  int    `json:"exit_code"`
	Type      string `json:"type"`
}

// ProcessManager manages Sockerless component processes.
type ProcessManager struct {
	mu        sync.Mutex
	processes map[string]*ManagedProcess
	reg       *Registry
}

// NewProcessManager creates a new ProcessManager.
func NewProcessManager(reg *Registry) *ProcessManager {
	return &ProcessManager{
		processes: make(map[string]*ManagedProcess),
		reg:       reg,
	}
}

// AddProcess registers a process definition from config.
func (pm *ProcessManager) AddProcess(cfg ProcessConfig) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	env := make([]string, 0, len(cfg.Env))
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}

	pm.processes[cfg.Name] = &ManagedProcess{
		Name:   cfg.Name,
		Binary: cfg.Binary,
		Args:   cfg.Args,
		Env:    env,
		Addr:   cfg.Addr,
		Type:   cfg.Type,
		Status: "stopped",
		Logs:   NewRingBuffer(1000),
	}
}

// Start starts a managed process by name.
func (pm *ProcessManager) Start(name string) error {
	pm.mu.Lock()
	proc, ok := pm.processes[name]
	if !ok {
		pm.mu.Unlock()
		return fmt.Errorf("process %q not found", name)
	}
	if proc.Status == "running" || proc.Status == "starting" {
		pm.mu.Unlock()
		return fmt.Errorf("process %q is already %s", name, proc.Status)
	}

	proc.Logs.Reset()

	// Resolve binary
	binPath, err := exec.LookPath(proc.Binary)
	if err != nil {
		pm.mu.Unlock()
		return fmt.Errorf("binary %q not found: %w", proc.Binary, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binPath, proc.Args...)
	cmd.Env = append(cmd.Environ(), proc.Env...)

	// Capture stdout+stderr to ring buffer
	w := io.MultiWriter(proc.Logs)
	cmd.Stdout = w
	cmd.Stderr = w

	proc.Status = "starting"
	proc.cancel = cancel
	proc.cmd = cmd
	proc.ExitCode = 0
	proc.done = make(chan struct{})
	pm.mu.Unlock()

	if err := cmd.Start(); err != nil {
		pm.mu.Lock()
		proc.Status = "failed"
		proc.cmd = nil
		proc.cancel = nil
		close(proc.done)
		pm.mu.Unlock()
		cancel()
		return fmt.Errorf("failed to start %q: %w", name, err)
	}

	pm.mu.Lock()
	proc.PID = cmd.Process.Pid
	proc.StartedAt = time.Now()
	proc.Status = "running"
	doneCh := proc.done
	pm.mu.Unlock()

	// Auto-register in component registry
	if pm.reg != nil {
		pm.reg.Add(Component{
			Name:   name,
			Type:   proc.Type,
			Addr:   normalizeAddr(proc.Addr),
			Health: "unknown",
		})
	}

	// Monitor process exit in background
	go func() {
		err := cmd.Wait()
		pm.mu.Lock()
		if proc.Status == "stopping" {
			proc.Status = "stopped"
		} else {
			proc.Status = "failed"
		}
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				proc.ExitCode = exitErr.ExitCode()
			}
		} else {
			proc.ExitCode = 0
		}
		proc.cmd = nil
		cancelFn := proc.cancel
		proc.cancel = nil
		pm.mu.Unlock()

		if cancelFn != nil {
			cancelFn()
		}

		close(doneCh)

		// Update registry health
		if pm.reg != nil {
			if c, ok := pm.reg.Get(name); ok {
				c.Health = "down"
				pm.reg.Add(c)
			}
		}
	}()

	return nil
}

// Stop stops a managed process by name.
func (pm *ProcessManager) Stop(name string) error {
	pm.mu.Lock()
	proc, ok := pm.processes[name]
	if !ok {
		pm.mu.Unlock()
		return fmt.Errorf("process %q not found", name)
	}
	if proc.Status != "running" {
		pm.mu.Unlock()
		return fmt.Errorf("process %q is not running (status: %s)", name, proc.Status)
	}

	proc.Status = "stopping"
	cmd := proc.cmd
	doneCh := proc.done
	pm.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		// Send SIGTERM
		_ = cmd.Process.Signal(syscall.SIGTERM)

		// Wait for process to exit (monitored by background goroutine)
		select {
		case <-doneCh:
			// Process exited
		case <-time.After(5 * time.Second):
			// Force kill
			_ = cmd.Process.Kill()
			<-doneCh
		}
	}

	pm.mu.Lock()
	// Only clean up if process hasn't been re-started while we waited
	if proc.done == doneCh {
		proc.PID = 0
		cancelFn := proc.cancel
		proc.cancel = nil
		pm.mu.Unlock()
		if cancelFn != nil {
			cancelFn()
		}
	} else {
		pm.mu.Unlock()
	}

	return nil
}

// StopAll stops all running managed processes.
func (pm *ProcessManager) StopAll() {
	pm.mu.Lock()
	names := make([]string, 0)
	for name, proc := range pm.processes {
		if proc.Status == "running" {
			names = append(names, name)
		}
	}
	pm.mu.Unlock()

	for _, name := range names {
		_ = pm.Stop(name)
	}
}

// List returns info about all managed processes.
func (pm *ProcessManager) List() []ProcessInfo {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	list := make([]ProcessInfo, 0, len(pm.processes))
	for _, proc := range pm.processes {
		startedAt := ""
		if !proc.StartedAt.IsZero() {
			startedAt = proc.StartedAt.UTC().Format(time.RFC3339)
		}
		list = append(list, ProcessInfo{
			Name:      proc.Name,
			Binary:    proc.Binary,
			Status:    proc.Status,
			PID:       proc.PID,
			Addr:      proc.Addr,
			StartedAt: startedAt,
			ExitCode:  proc.ExitCode,
			Type:      proc.Type,
		})
	}
	return list
}

// GetLogs returns the last n log lines for a process.
func (pm *ProcessManager) GetLogs(name string, n int) ([]string, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	proc, ok := pm.processes[name]
	if !ok {
		return nil, fmt.Errorf("process %q not found", name)
	}
	return proc.Logs.Lines(n), nil
}

// RemoveProcess removes a process definition (must be stopped).
func (pm *ProcessManager) RemoveProcess(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	proc, ok := pm.processes[name]
	if !ok {
		return fmt.Errorf("process %q not found", name)
	}
	if proc.Status == "running" || proc.Status == "starting" {
		return fmt.Errorf("process %q is still %s", name, proc.Status)
	}
	delete(pm.processes, name)
	return nil
}

// Get returns info about a single process.
func (pm *ProcessManager) Get(name string) (ProcessInfo, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	proc, ok := pm.processes[name]
	if !ok {
		return ProcessInfo{}, false
	}
	startedAt := ""
	if !proc.StartedAt.IsZero() {
		startedAt = proc.StartedAt.UTC().Format(time.RFC3339)
	}
	return ProcessInfo{
		Name:      proc.Name,
		Binary:    proc.Binary,
		Status:    proc.Status,
		PID:       proc.PID,
		Addr:      proc.Addr,
		StartedAt: startedAt,
		ExitCode:  proc.ExitCode,
		Type:      proc.Type,
	}, true
}
