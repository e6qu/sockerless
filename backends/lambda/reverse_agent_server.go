package lambda

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/sockerless/agent"
)

// reverseAgentRegistry tracks live reverse-agent WebSocket sessions —
// one per running Lambda container that has a reverse agent connected.
// Keyed by sockerless container ID (which the bootstrap sends as
// `session_id` on upgrade).
type reverseAgentRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*agent.ReverseAgentConn
}

func newReverseAgentRegistry() *reverseAgentRegistry {
	return &reverseAgentRegistry{sessions: map[string]*agent.ReverseAgentConn{}}
}

func (r *reverseAgentRegistry) register(id string, conn *agent.ReverseAgentConn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// If a prior session exists under this ID, close it — the new dial
	// wins (matches the reconnect-with-same-session-id resume model).
	if old, ok := r.sessions[id]; ok {
		_ = old.Close()
	}
	r.sessions[id] = conn
}

func (r *reverseAgentRegistry) resolve(id string) (*agent.ReverseAgentConn, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.sessions[id]
	return c, ok
}

func (r *reverseAgentRegistry) drop(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.sessions[id]; ok {
		_ = c.Close()
		delete(r.sessions, id)
	}
}

// wsUpgrader used by the reverse-agent WebSocket endpoint. Origin check
// is permissive — the bootstrap dials from inside a Lambda container
// with no browser-enforced Origin semantics.
var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// handleReverseAgentWS upgrades the request to a WebSocket and
// registers the session in the registry so subsequent `docker exec` /
// `docker attach` calls can look it up by container ID. The connection
// is kept alive until the client closes; on close, the session is
// dropped from the registry.
func (s *Server) handleReverseAgentWS(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(w, "session_id query parameter is required", http.StatusBadRequest)
		return
	}

	ws, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrader already wrote the error response.
		return
	}
	rc := agent.NewReverseAgentConn(ws)
	s.reverseAgents.register(sessionID, rc)
	s.Logger.Debug().Str("session_id", sessionID).Msg("reverse-agent session registered")

	// Block until the client closes. On exit, drop the session.
	<-rc.Done()
	s.reverseAgents.drop(sessionID)
	s.Logger.Debug().Str("session_id", sessionID).Msg("reverse-agent session dropped")
}

// resolveReverseAgent looks up the live reverse-agent session for the
// given container ID. Returns (nil, false) if no session is registered.
// Used by ContainerExec / ContainerAttach to find the right WebSocket
// to multiplex the docker client's exec frames onto.
func (s *Server) resolveReverseAgent(containerID string) (*agent.ReverseAgentConn, bool) {
	return s.reverseAgents.resolve(containerID)
}

// registerReverseAgentRoutes mounts the reverse-agent WebSocket endpoint
// on the base server mux. Called from NewServer.
func (s *Server) registerReverseAgentRoutes(_ zerolog.Logger) {
	s.Mux.HandleFunc("/v1/lambda/reverse", s.handleReverseAgentWS)
}
