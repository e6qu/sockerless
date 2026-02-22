package bleephub

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/graphql-go/graphql"
	"github.com/rs/zerolog"
)

// Server is the bleephub HTTP server implementing the GitHub Actions
// runner service API (GHES-style endpoints).
type Server struct {
	addr          string
	mux           *http.ServeMux
	logger        zerolog.Logger
	store         *Store
	graphqlSchema graphql.Schema
}

// NewServer creates a bleephub server with all routes registered.
func NewServer(addr string, logger zerolog.Logger) *Server {
	s := &Server{
		addr:   addr,
		mux:    http.NewServeMux(),
		logger: logger,
		store:  NewStore(),
	}
	s.store.SeedDefaultUser()
	s.initGraphQLSchema()
	s.registerRoutes()
	return s
}

// Store returns the server's in-memory store (for tests).
func (s *Server) Store() *Store { return s.store }

func (s *Server) registerRoutes() {
	// Health check
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// Auth + connection data (auth.go)
	s.registerAuthRoutes()

	// Agent management (agents.go)
	s.registerAgentRoutes()

	// Broker: sessions + message poll (broker.go)
	s.registerBrokerRoutes()

	// Job submission (jobs.go)
	s.registerJobRoutes()

	// Run service: acquire/renew/complete (run_service.go)
	s.registerRunServiceRoutes()

	// Timeline + logs (timeline.go)
	s.registerTimelineRoutes()

	// GitHub API: REST, GraphQL, OAuth (gh_*.go)
	s.registerGHRestRoutes()
	s.registerGHRepoRoutes()
	s.registerGHOAuthRoutes()
	s.registerGHGraphQLRoutes()

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
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "bleephub"})
}

// ListenAndServe starts the HTTP server with graceful shutdown.
func (s *Server) ListenAndServe() error {
	inner := s.prefixStripMiddleware(s.mux)
	ghWrapped := s.ghHeadersMiddleware(inner)
	handler := s.loggingMiddleware(ghWrapped)

	srv := &http.Server{
		Addr:         s.addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown on signal
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		sig := <-sigCh
		s.logger.Info().Str("signal", sig.String()).Msg("shutting down")
		srv.Close()
	}()

	// Resolve addr for log output
	host, port, _ := net.SplitHostPort(s.addr)
	if host == "" {
		host = "localhost"
	}

	// TLS support via environment variables
	certFile := os.Getenv("BPH_TLS_CERT")
	keyFile := os.Getenv("BPH_TLS_KEY")
	if certFile != "" && keyFile != "" {
		s.logger.Info().Msgf("bleephub listening on https://%s:%s", host, port)
		return srv.ListenAndServeTLS(certFile, keyFile)
	}

	s.logger.Info().Msgf("bleephub listening on http://%s:%s", host, port)
	return srv.ListenAndServe()
}

// prefixStripMiddleware removes any path segments before known API prefixes.
// The runner prepends the tenant URL path to all API calls, e.g.
// /owner/repo/_apis/... instead of /_apis/...
func (s *Server) prefixStripMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Strip everything before /_apis/ or /api/
		for _, prefix := range []string{"/_apis/", "/api/"} {
			if idx := strings.Index(path, prefix); idx > 0 {
				r.URL.Path = path[idx:]
				r.URL.RawPath = ""
				break
			}
		}
		next.ServeHTTP(w, r)
	})
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
