package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// InstanceStatus is the runtime view of a single instance: whether
// the process is alive (PID file points to a running process) and
// whether `/v1/health` returns 2xx within a short timeout.
//
// Foundation for Phase 86 (health surface) and consumed by Phase 80
// (admin UI showing per-row state). Components stay decoupled —
// status uses only what they already expose.
type InstanceStatus struct {
	Project string `json:"project"`
	Name    string `json:"name"`
	// Running: process matched by `.stack-pids/<name>.pid` is alive.
	Running bool `json:"running"`
	// PID is the recorded PID; 0 if the pidfile is missing.
	PID int `json:"pid"`
	// Health is one of "ok", "unhealthy", "unknown" (no /v1/health
	// surface, or the probe didn't complete in time).
	Health string `json:"health"`
	// HealthDetail surfaces the last error from the /v1/health probe
	// when Health == "unhealthy" — saves the operator a click in the
	// admin UI to figure out why.
	HealthDetail string `json:"health_detail,omitempty"`
}

// readInstanceStatus inspects the PID file + probes /v1/health.
// Best-effort: missing pidfile + missing /v1/health both produce
// "unknown" rather than erroring.
func readInstanceStatus(inst Instance) InstanceStatus {
	out := InstanceStatus{
		Name:   inst.Name,
		Health: "unknown",
	}
	pid, alive := readPidStatus(inst.Name)
	out.PID = pid
	out.Running = alive
	if !alive {
		return out
	}
	if inst.Port <= 0 {
		return out
	}
	healthy, detail := probeHealth(inst.Port)
	if healthy {
		out.Health = "ok"
	} else if detail != "" {
		out.Health = "unhealthy"
		out.HealthDetail = detail
	}
	return out
}

func readPidStatus(name string) (pid int, alive bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return 0, false
	}
	pidfile := filepath.Join(cwd, ".stack-pids", name+".pid")
	data, err := os.ReadFile(pidfile)
	if err != nil {
		return 0, false
	}
	pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return pid, false
	}
	// On unix, FindProcess always succeeds; Signal(0) probes for life.
	if err := proc.Signal(syscall0()); err != nil {
		return pid, false
	}
	return pid, true
}

func probeHealth(port int) (ok bool, detail string) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("http://localhost:%d/v1/health", port), nil)
	if err != nil {
		return false, ""
	}
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, ""
	}
	return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
}
