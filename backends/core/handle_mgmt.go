package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// handleHealthz returns a simple health check response.
func (s *BaseServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"component":      "backend",
		"uptime_seconds": int(time.Since(s.StartedAt).Seconds()),
	})
}

// handleMgmtStatus returns backend status information.
func (s *BaseServer) handleMgmtStatus(w http.ResponseWriter, r *http.Request) {
	containerCount := s.Store.Containers.Len()
	activeResources := len(s.Registry.ListActive())

	WriteJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"component":        "backend",
		"backend_type":     s.Desc.Name,
		"instance_id":      s.Desc.InstanceID,
		"uptime_seconds":   int(time.Since(s.StartedAt).Seconds()),
		"containers":       containerCount,
		"active_resources": activeResources,
		"context":          activeContextName(),
	})
}

// ContainerSummaryEntry is a read-only container summary for the management API.
type ContainerSummaryEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Created string `json:"created"`
	PodName string `json:"pod_name,omitempty"`
}

// handleContainerSummary returns a cloud-agnostic container summary.
func (s *BaseServer) handleContainerSummary(w http.ResponseWriter, r *http.Request) {
	containers := s.Store.Containers.List()
	entries := make([]ContainerSummaryEntry, 0, len(containers))
	for _, c := range containers {
		name := ""
		if len(c.Name) > 0 {
			name = c.Name
		}
		podName := ""
		if s.Store.Pods != nil {
			if pod, ok := s.Store.Pods.GetPodForContainer(c.ID); ok {
				podName = pod.Name
			}
		}
		entries = append(entries, ContainerSummaryEntry{
			ID:      c.ID,
			Name:    name,
			Image:   c.Config.Image,
			State:   c.State.Status,
			Created: c.Created,
			PodName: podName,
		})
	}
	WriteJSON(w, http.StatusOK, entries)
}

// handleMetrics returns collected request metrics.
func (s *BaseServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	snap := s.Metrics.Snapshot()
	snap.Uptime = int(time.Since(s.StartedAt).Seconds())
	snap.ActiveResources = len(s.Registry.ListActive())
	snap.Containers = s.Store.Containers.Len()
	WriteJSON(w, http.StatusOK, snap)
}

// CheckResult is a single health check result.
type CheckResult struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "ok" or "error"
	Detail string `json:"detail,omitempty"`
}

// HealthChecker is an interface for backend-specific health checks.
type HealthChecker interface {
	RunChecks(ctx context.Context) []CheckResult
}

// handleCheck runs health checks and returns results.
func (s *BaseServer) handleCheck(w http.ResponseWriter, r *http.Request) {
	var checks []CheckResult

	// Basic store check
	checks = append(checks, CheckResult{
		Name:   "store",
		Status: "ok",
		Detail: "in-memory store accessible",
	})

	// Registry check
	active := s.Registry.ListActive()
	checks = append(checks, CheckResult{
		Name:   "registry",
		Status: "ok",
		Detail: fmt.Sprintf("%d active resources", len(active)),
	})

	// Backend-specific checks
	if s.HealthChecker != nil {
		checks = append(checks, s.HealthChecker.RunChecks(r.Context())...)
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"checks": checks,
	})
}

// handleReload re-reads the active context config and applies env vars.
func (s *BaseServer) handleReload(w http.ResponseWriter, r *http.Request) {
	name := activeContextName()
	if name == "" {
		WriteJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"changed": 0,
			"message": "no active context",
		})
		return
	}

	path := contextConfigPath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		WriteJSON(w, http.StatusOK, map[string]any{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	var cfg contextConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		WriteJSON(w, http.StatusOK, map[string]any{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	changed := 0
	for k, v := range cfg.Env {
		current := os.Getenv(k)
		if current != v {
			_ = os.Setenv(k, v)
			changed++
		}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"context": name,
		"changed": changed,
	})
}
