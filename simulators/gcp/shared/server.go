package simulator

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
)

// ServiceHandler handles requests for a single simulated cloud service.
type ServiceHandler interface {
	// ServiceName returns the display name of the service (e.g., "ECS", "CloudRun").
	ServiceName() string
}

// Server is the main simulator HTTP server.
type Server struct {
	config  Config
	logger  zerolog.Logger
	mux     *http.ServeMux
	handler http.Handler
}

// NewServer creates a new simulator server with the given configuration.
func NewServer(cfg Config) *Server {
	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		Level(level).
		With().
		Timestamp().
		Str("provider", cfg.Provider).
		Logger()

	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{
			"status":   "ok",
			"provider": cfg.Provider,
		})
	})

	// Build middleware chain
	var handler http.Handler = mux
	handler = AuthPassthroughMiddleware(cfg.Provider)(handler)
	handler = LoggingMiddleware(logger, cfg.Provider)(handler)
	handler = RequestIDMiddleware(cfg.Provider)(handler)

	return &Server{
		config:  cfg,
		logger:  logger,
		mux:     mux,
		handler: handler,
	}
}

// Handle registers a pattern on the server's mux.
func (s *Server) Handle(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
}

// HandleFunc registers a handler function on the server's mux.
func (s *Server) HandleFunc(pattern string, handler http.HandlerFunc) {
	s.mux.HandleFunc(pattern, handler)
}

// Mux returns the underlying ServeMux for direct registration.
func (s *Server) Mux() *http.ServeMux {
	return s.mux
}

// Logger returns the server's logger for use by service handlers.
func (s *Server) Logger() zerolog.Logger {
	return s.logger
}

// ListenAndServe starts the server and blocks until shutdown.
// It listens for SIGTERM and SIGINT for graceful shutdown.
func (s *Server) ListenAndServe() error {
	srv := &http.Server{
		Addr:         s.config.ListenAddr,
		Handler:      s.handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	done := make(chan error, 1)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		sig := <-sigCh
		s.logger.Info().Str("signal", sig.String()).Msg("shutting down")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		done <- srv.Shutdown(ctx)
	}()

	// Print startup banner
	s.printBanner()

	// Start server
	var err error
	if s.config.TLSCert != "" && s.config.TLSKey != "" {
		s.logger.Info().
			Str("addr", s.config.ListenAddr).
			Msg("starting HTTPS server")
		err = srv.ListenAndServeTLS(s.config.TLSCert, s.config.TLSKey)
	} else {
		s.logger.Info().
			Str("addr", s.config.ListenAddr).
			Msg("starting HTTP server")
		err = srv.ListenAndServe()
	}

	if err == http.ErrServerClosed {
		return <-done
	}
	return err
}

func (s *Server) printBanner() {
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  Sockerless %s Simulator\n", s.config.Provider)
	fmt.Fprintf(os.Stderr, "  Listening on %s\n", s.config.ListenAddr)
	switch s.config.Provider {
	case "aws":
		fmt.Fprintf(os.Stderr, "  SDK config: AWS_ENDPOINT_URL=http://localhost%s\n", s.config.ListenAddr)
	case "gcp":
		fmt.Fprintf(os.Stderr, "  SDK config: option.WithEndpoint(\"http://localhost%s\")\n", s.config.ListenAddr)
	case "azure":
		fmt.Fprintf(os.Stderr, "  SDK config: custom cloud.Configuration with endpoint http://localhost%s\n", s.config.ListenAddr)
	}
	fmt.Fprintf(os.Stderr, "\n")
}
