package core

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sockerless/agent"
	"github.com/sockerless/api"
)

// AgentRegistry manages reverse agent connections from FaaS functions.
// When an agent in callback mode dials the backend, its connection is stored
// here so that exec/attach requests can be routed to it.
type AgentRegistry struct {
	mu    sync.RWMutex
	conns map[string]*agent.ReverseAgentConn
	ready map[string]chan struct{}
	done  map[string]chan struct{} // closed when agent disconnects
}

// NewAgentRegistry creates a new empty agent registry.
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		conns: make(map[string]*agent.ReverseAgentConn),
		ready: make(map[string]chan struct{}),
		done:  make(map[string]chan struct{}),
	}
}

// Prepare creates the done channel for the given container ID so that
// WaitForDisconnect can block even before the agent connects. This must
// be called before starting the invoke goroutine in FaaS backends.
func (r *AgentRegistry) Prepare(containerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.done[containerID]; !ok {
		r.done[containerID] = make(chan struct{})
	}
}

// Register stores a reverse agent connection for the given container ID
// and signals any waiters.
func (r *AgentRegistry) Register(containerID string, conn *agent.ReverseAgentConn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conns[containerID] = conn
	// Create done channel only if not already prepared
	if _, ok := r.done[containerID]; !ok {
		r.done[containerID] = make(chan struct{})
	}
	if ch, ok := r.ready[containerID]; ok {
		close(ch)
		delete(r.ready, containerID)
	}
}

// Get returns the reverse agent connection for the given container ID, or nil.
func (r *AgentRegistry) Get(containerID string) *agent.ReverseAgentConn {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.conns[containerID]
}

// Remove removes and closes the reverse agent connection for the given container ID.
// It also signals any WaitForDisconnect waiters.
func (r *AgentRegistry) Remove(containerID string) {
	r.mu.Lock()
	conn := r.conns[containerID]
	delete(r.conns, containerID)
	doneCh := r.done[containerID]
	delete(r.done, containerID)
	r.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	if doneCh != nil {
		close(doneCh)
	}
}

// WaitForDisconnect blocks until the reverse agent for the given container ID
// disconnects (is removed). Returns immediately if no agent is registered.
func (r *AgentRegistry) WaitForDisconnect(containerID string, timeout time.Duration) error {
	r.mu.RLock()
	ch, ok := r.done[containerID]
	r.mu.RUnlock()
	if !ok {
		return nil
	}
	select {
	case <-ch:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for agent disconnect for container %s", containerID)
	}
}

// WaitForAgent blocks until an agent connects for the given container ID
// or the timeout expires. If the agent is already registered, returns immediately.
func (r *AgentRegistry) WaitForAgent(containerID string, timeout time.Duration) error {
	r.mu.Lock()
	if _, ok := r.conns[containerID]; ok {
		r.mu.Unlock()
		return nil
	}
	ch, ok := r.ready[containerID]
	if !ok {
		ch = make(chan struct{})
		r.ready[containerID] = ch
	}
	r.mu.Unlock()

	select {
	case <-ch:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for agent callback for container %s", containerID)
	}
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// handleAgentConnect handles incoming reverse agent connections.
// Route: GET /internal/v1/agent/connect?id=<containerID>&token=<token>
func (s *BaseServer) handleAgentConnect(w http.ResponseWriter, r *http.Request) {
	containerID := r.URL.Query().Get("id")
	token := r.URL.Query().Get("token")

	if containerID == "" {
		WriteError(w, &api.InvalidParameterError{Message: "missing id parameter"})
		return
	}

	// Validate token against the container's stored agent token
	c, ok := s.Store.Containers.Get(containerID)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: containerID})
		return
	}

	if c.AgentToken != "" && c.AgentToken != token {
		WriteJSON(w, http.StatusUnauthorized, api.ErrorResponse{Message: "unauthorized"})
		return
	}

	// Upgrade to WebSocket
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		s.Logger.Error().Err(err).Msg("websocket upgrade failed for agent callback")
		return
	}

	s.Logger.Info().Str("container", containerID).Msg("reverse agent connected")

	revConn := agent.NewReverseAgentConn(conn)
	s.AgentRegistry.Register(containerID, revConn)

	// Block until the connection closes — this keeps the HTTP handler alive
	<-revConn.Done()

	s.Logger.Info().Str("container", containerID).Msg("reverse agent disconnected")
	s.AgentRegistry.Remove(containerID)
}
