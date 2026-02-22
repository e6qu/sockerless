package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// Config holds agent configuration.
type Config struct {
	Addr        string
	Token       string
	KeepAlive   bool
	CallbackURL string // reverse connect URL (FaaS mode)
	Args        []string // main process args (after --)
	Env         []string // extra environment variables
}

// Server is the agent WebSocket server.
type Server struct {
	config   Config
	logger   zerolog.Logger
	registry *SessionRegistry
	mp       *MainProcess    // non-nil in keep-alive mode
	hc       *HealthChecker  // non-nil if health check configured
	upgrader websocket.Upgrader
}

// NewServer creates a new agent server.
func NewServer(config Config, logger zerolog.Logger) *Server {
	return &Server{
		config:   config,
		logger:   logger,
		registry: NewSessionRegistry(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// ListenAndServe starts the agent server.
func (s *Server) ListenAndServe() error {
	// Start main process in keep-alive mode
	if s.config.KeepAlive {
		mp, err := NewMainProcess(s.logger, s.config.Args, s.config.Env)
		if err != nil {
			return err
		}
		s.mp = mp
		s.logger.Info().Int("pid", mp.Pid()).Strs("args", s.config.Args).Msg("main process started")

		// Reap zombies (PID 1 behavior)
		go s.reapZombies()

		// Exit when main process exits (if not serving requests)
		go func() {
			<-mp.Done()
			code := 0
			if c := mp.ExitCode(); c != nil {
				code = *c
			}
			s.logger.Info().Int("exitCode", code).Msg("main process exited")
		}()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /ws", s.handleWebSocket)

	handler := authMiddleware(s.config.Token, mux)

	s.logger.Info().Str("addr", s.config.Addr).Msg("agent server starting")
	return http.ListenAndServe(s.config.Addr, handler)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"status": "ok",
	}

	if s.mp != nil {
		resp["pid"] = s.mp.Pid()
		if code := s.mp.ExitCode(); code != nil {
			resp["exited"] = true
			resp["exitCode"] = *code
		} else {
			resp["exited"] = false
		}
	}

	if s.hc != nil {
		resp["health"] = map[string]any{
			"status":        s.hc.Status(),
			"failingStreak": s.hc.FailingStreak(),
			"log":           s.hc.Log(),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error().Err(err).Msg("websocket upgrade failed")
		return
	}
	defer func() { _ = conn.Close() }()
	defer s.registry.CleanupConn(conn)

	s.logger.Debug().Str("remote", r.RemoteAddr).Msg("websocket connection established")

	connMu := &sync.Mutex{}
	router := NewRouter(s.registry, s.mp, s.logger)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				s.logger.Debug().Err(err).Msg("websocket read error")
			}
			return
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			s.logger.Warn().Err(err).Msg("invalid message")
			continue
		}

		router.Handle(&msg, conn, connMu)
	}
}

func (s *Server) reapZombies() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGCHLD)
	for range sigCh {
		for {
			var status syscall.WaitStatus
			pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
			if pid <= 0 || err != nil {
				break
			}
		}
	}
}

// ReverseConnect operates the agent in callback mode: instead of listening for
// incoming connections, it dials out to the backend at callbackURL and handles
// messages on that connection. Reconnects with backoff on disconnect.
func (s *Server) ReverseConnect(callbackURL string) error {
	// Start main process in keep-alive mode
	if s.config.KeepAlive {
		mp, err := NewMainProcess(s.logger, s.config.Args, s.config.Env)
		if err != nil {
			return err
		}
		s.mp = mp
		s.logger.Info().Int("pid", mp.Pid()).Strs("args", s.config.Args).Msg("main process started")

		go s.reapZombies()
	}

	// Build WebSocket URL from callback URL
	wsURL := callbackURL
	if len(wsURL) > 4 && wsURL[:5] == "http:" {
		wsURL = "ws:" + wsURL[5:]
	} else if len(wsURL) > 5 && wsURL[:6] == "https:" {
		wsURL = "wss:" + wsURL[6:]
	}

	header := http.Header{}
	if s.config.Token != "" {
		header.Set("Authorization", "Bearer "+s.config.Token)
	}

	const maxRetries = 10
	backoff := time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			s.logger.Info().Int("attempt", attempt+1).Dur("backoff", backoff).Msg("reconnecting to backend")
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}

		s.logger.Info().Str("url", wsURL).Msg("dialing backend for reverse connection")

		conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
		if err != nil {
			s.logger.Error().Err(err).Msg("failed to dial backend")
			// If main process has exited, stop retrying
			if s.mp != nil {
				select {
				case <-s.mp.Done():
					return fmt.Errorf("main process exited, stopping reverse connect")
				default:
				}
			}
			continue
		}

		s.logger.Info().Msg("reverse connection established")
		backoff = time.Second // reset on success

		// Handle messages on this connection
		err = s.serveReverseConn(conn)
		_ = conn.Close()

		if err != nil {
			s.logger.Warn().Err(err).Msg("reverse connection lost")
		}

		// If main process has exited, stop
		if s.mp != nil {
			select {
			case <-s.mp.Done():
				s.logger.Info().Msg("main process exited, stopping reverse connect")
				return nil
			default:
			}
		}
	}

	return fmt.Errorf("reverse connect failed after %d retries", maxRetries)
}

// serveReverseConn handles messages on a single reverse WebSocket connection.
func (s *Server) serveReverseConn(conn *websocket.Conn) error {
	defer s.registry.CleanupConn(conn)

	connMu := &sync.Mutex{}
	router := NewRouter(s.registry, s.mp, s.logger)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			s.logger.Warn().Err(err).Msg("invalid message on reverse connection")
			continue
		}

		router.Handle(&msg, conn, connMu)
	}
}

