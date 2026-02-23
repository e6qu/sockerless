package gitlabhub

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Server is the gitlabhub HTTP server implementing the GitLab Runner
// coordinator API.
type Server struct {
	addr    string
	mux     *http.ServeMux
	logger  zerolog.Logger
	store   *Store
	metrics *Metrics
	maxConcurrentPipelines int
}

// NewServer creates a gitlabhub server with all routes registered.
func NewServer(addr string, logger zerolog.Logger) *Server {
	maxPL := 10
	if v := os.Getenv("GITLABHUB_MAX_PIPELINES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxPL = n
		}
	}

	s := &Server{
		addr:                   addr,
		mux:                    http.NewServeMux(),
		logger:                 logger,
		store:                  NewStore(),
		metrics:                NewMetrics(),
		maxConcurrentPipelines: maxPL,
	}
	s.registerRoutes()
	return s
}

// Store returns the server's in-memory store (for tests).
func (s *Server) Store() *Store { return s.store }

func (s *Server) registerRoutes() {
	// Health check
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// Runner registration (runners.go)
	s.registerRunnerRoutes()

	// Job request (jobs_request.go)
	s.registerJobRequestRoutes()

	// Job update + trace (jobs_update.go)
	s.registerJobUpdateRoutes()

	// Artifacts (artifacts.go)
	s.registerArtifactRoutes()

	// Cache (cache.go)
	s.registerCacheRoutes()

	// Variables/secrets API (secrets.go)
	s.registerSecretsRoutes()

	// Management API (pipeline_api.go)
	s.registerPipelineAPIRoutes()

	// Management: metrics + status
	s.mux.HandleFunc("GET /internal/metrics", s.handleInternalMetrics)
	s.mux.HandleFunc("GET /internal/status", s.handleInternalStatus)

	// Catch-all: tries smart HTTP git protocol, then logs unmatched
	s.mux.HandleFunc("/", s.handleCatchAll)
}

func (s *Server) handleCatchAll(w http.ResponseWriter, r *http.Request) {
	// Try smart HTTP git protocol
	if s.tryHandleGitRequest(w, r) {
		return
	}

	s.logger.Warn().
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Str("query", r.URL.RawQuery).
		Msg("UNHANDLED REQUEST")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "gitlabhub"})
}

func (s *Server) handleInternalMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.metrics.Snapshot())
}

func (s *Server) handleInternalStatus(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	activePLs := 0
	jobsByStatus := make(map[string]int)
	for _, pl := range s.store.Pipelines {
		if pl.Status == "running" || pl.Status == "pending" {
			activePLs++
		}
		for _, j := range pl.Jobs {
			jobsByStatus[j.Status]++
		}
	}
	runners := len(s.store.Runners)
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"active_pipelines":  activePLs,
		"jobs_by_status":    jobsByStatus,
		"registered_runners": runners,
		"uptime_seconds":    int(time.Since(s.metrics.StartedAt).Seconds()),
	})
}

// ListenAndServe starts the HTTP server (crash-only, no graceful shutdown).
func (s *Server) ListenAndServe() error {
	handler := otelhttp.NewHandler(s.loggingMiddleware(s.mux), "gitlabhub")

	srv := &http.Server{
		Addr:         s.addr,
		Handler:      handler,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	host, port, _ := net.SplitHostPort(s.addr)
	if host == "" {
		host = "localhost"
	}

	certFile := os.Getenv("GLH_TLS_CERT")
	keyFile := os.Getenv("GLH_TLS_KEY")
	if certFile != "" && keyFile != "" {
		s.logger.Info().Msgf("gitlabhub listening on https://%s:%s", host, port)
		return srv.ListenAndServeTLS(certFile, keyFile)
	}

	s.logger.Info().Msgf("gitlabhub listening on http://%s:%s", host, port)
	return srv.ListenAndServe()
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		s.logger.Debug().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", rw.status).
			Dur("dur", time.Since(start)).
			Msg("request")
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// writeJSON marshals v as JSON and writes it to w.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
