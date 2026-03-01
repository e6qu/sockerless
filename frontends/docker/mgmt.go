package frontend

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// MgmtServer is the management API server (separate from Docker API).
type MgmtServer struct {
	logger       zerolog.Logger
	startedAt    time.Time
	dockerAddr   string
	backendAddr  string
	mux          *http.ServeMux
	requestCount atomic.Int64
}

// NewMgmtServer creates a new management API server.
func NewMgmtServer(logger zerolog.Logger, dockerAddr, backendAddr string) *MgmtServer {
	m := &MgmtServer{
		logger:      logger,
		startedAt:   time.Now(),
		dockerAddr:  dockerAddr,
		backendAddr: backendAddr,
		mux:         http.NewServeMux(),
	}
	m.mux.HandleFunc("GET /healthz", m.handleHealthz)
	m.mux.HandleFunc("GET /status", m.handleStatus)
	m.mux.HandleFunc("GET /metrics", m.handleMetrics)
	m.mux.HandleFunc("POST /reload", m.handleReload)
	registerUI(m)
	return m
}

func (m *MgmtServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":         "ok",
		"component":      "frontend",
		"uptime_seconds": int(time.Since(m.startedAt).Seconds()),
	})
}

func (m *MgmtServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":         "ok",
		"component":      "frontend",
		"docker_addr":    m.dockerAddr,
		"backend_addr":   m.backendAddr,
		"uptime_seconds": int(time.Since(m.startedAt).Seconds()),
	})
}

func (m *MgmtServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"component":       "frontend",
		"uptime_seconds":  int(time.Since(m.startedAt).Seconds()),
		"docker_requests": m.requestCount.Load(),
		"goroutines":      runtime.NumGoroutine(),
		"heap_alloc_mb":   float64(memStats.HeapAlloc) / (1024 * 1024),
	})
}

func (m *MgmtServer) handleReload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
	})
}

// IncrementRequests increments the Docker API request counter.
func (m *MgmtServer) IncrementRequests() {
	m.requestCount.Add(1)
}

// ListenAndServe starts the management HTTP server.
// If certFile and keyFile are both non-empty, the listener uses TLS.
func (m *MgmtServer) ListenAndServe(addr, certFile, keyFile string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: m.mux,
	}
	m.logger.Info().Str("addr", addr).Msg("starting management API")
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return err
		}
		srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
		return srv.ListenAndServeTLS("", "")
	}
	return srv.ListenAndServe()
}
