package docker

import (
	"context"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/docker/docker/api/types/container"
	core "github.com/sockerless/backend-core"
)

// mgmtState tracks management metrics for the Docker backend.
type mgmtState struct {
	startedAt    time.Time
	requestCount atomic.Int64
}

// registerMgmt registers management API endpoints on the server's mux.
func (s *Server) registerMgmt() {
	s.mgmt = &mgmtState{startedAt: time.Now()}

	s.mux.HandleFunc("GET /internal/v1/healthz", s.handleMgmtHealthz)
	s.mux.HandleFunc("GET /internal/v1/status", s.handleMgmtStatus)
	s.mux.HandleFunc("GET /internal/v1/metrics", s.handleMgmtMetrics)
	s.mux.HandleFunc("GET /internal/v1/containers/summary", s.handleMgmtContainersSummary)
	s.mux.HandleFunc("GET /internal/v1/check", s.handleMgmtCheck)
	s.mux.HandleFunc("GET /internal/v1/resources", s.handleMgmtResources)
}

func (s *Server) handleMgmtHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	status := "ok"
	if _, err := s.docker.Ping(ctx); err != nil {
		status = "degraded"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":         status,
		"component":      "backend",
		"uptime_seconds": int(time.Since(s.mgmt.startedAt).Seconds()),
	})
}

func (s *Server) handleMgmtStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	containers := 0
	if list, err := s.docker.ContainerList(ctx, container.ListOptions{All: true}); err == nil {
		containers = len(list)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"component":        "backend",
		"backend_type":     "docker",
		"instance_id":      "docker-local",
		"uptime_seconds":   int(time.Since(s.mgmt.startedAt).Seconds()),
		"containers":       containers,
		"active_resources": 0,
		"context":          "",
	})
}

func (s *Server) handleMgmtMetrics(w http.ResponseWriter, r *http.Request) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	writeJSON(w, http.StatusOK, map[string]any{
		"requests":       map[string]int{},
		"latency_ms":     map[string]any{},
		"goroutines":     runtime.NumGoroutine(),
		"heap_alloc_mb":  float64(memStats.HeapAlloc) / (1024 * 1024),
		"uptime_seconds": int(time.Since(s.mgmt.startedAt).Seconds()),
	})
}

func (s *Server) handleMgmtContainersSummary(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	list, err := s.docker.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	type summary struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Image   string `json:"image"`
		State   string `json:"state"`
		Created string `json:"created"`
	}

	result := make([]summary, 0, len(list))
	for _, c := range list {
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0]
			if len(name) > 0 && name[0] == '/' {
				name = name[1:]
			}
		}
		result = append(result, summary{
			ID:      c.ID,
			Name:    name,
			Image:   c.Image,
			State:   c.State,
			Created: time.Unix(c.Created, 0).UTC().Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleMgmtCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	checks := []map[string]string{}
	if _, err := s.docker.Ping(ctx); err != nil {
		checks = append(checks, map[string]string{
			"name":   "docker_daemon",
			"status": "error",
			"detail": err.Error(),
		})
	} else {
		checks = append(checks, map[string]string{
			"name":   "docker_daemon",
			"status": "ok",
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"checks": checks})
}

func (s *Server) handleMgmtResources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []core.ResourceEntry{})
}

