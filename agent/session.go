package agent

import (
	"sync"

	"github.com/gorilla/websocket"
)

// Session represents an active exec or attach session.
type Session interface {
	// ID returns the session identifier.
	ID() string
	// WriteStdin writes data to the session's stdin.
	WriteStdin(data []byte) error
	// CloseStdin closes the session's stdin.
	CloseStdin() error
	// Signal sends a signal to the session's process.
	Signal(sig string) error
	// Resize resizes the session's TTY.
	Resize(width, height int) error
	// Close cleans up the session.
	Close()
}

// SessionRegistry manages active sessions and their WebSocket connections.
type SessionRegistry struct {
	mu       sync.RWMutex
	sessions map[string]Session
	// connSessions tracks which sessions belong to which WebSocket connection
	connSessions map[*websocket.Conn][]string
}

// NewSessionRegistry creates a new session registry.
func NewSessionRegistry() *SessionRegistry {
	return &SessionRegistry{
		sessions:     make(map[string]Session),
		connSessions: make(map[*websocket.Conn][]string),
	}
}

// Register adds a session to the registry.
func (r *SessionRegistry) Register(s Session, conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[s.ID()] = s
	r.connSessions[conn] = append(r.connSessions[conn], s.ID())
}

// Get returns a session by ID.
func (r *SessionRegistry) Get(id string) (Session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sessions[id]
	return s, ok
}

// Remove removes a session from the registry.
func (r *SessionRegistry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.sessions[id]; ok {
		s.Close()
		delete(r.sessions, id)
	}
}

// CleanupConn removes all sessions associated with a WebSocket connection.
func (r *SessionRegistry) CleanupConn(conn *websocket.Conn) {
	r.mu.Lock()
	ids := r.connSessions[conn]
	delete(r.connSessions, conn)
	r.mu.Unlock()

	for _, id := range ids {
		r.Remove(id)
	}
}
