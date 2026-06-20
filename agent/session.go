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

// Remove removes a session from the registry, calling Close() to tear down
// its process. Used by CleanupConn for whole-connection teardown, where the
// process may still be running.
func (r *SessionRegistry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.forgetLocked(id, true)
}

// Forget removes a finished session from the registry WITHOUT calling
// Close() — the process has already exited, so there is no child to
// SIGKILL. Called from waitAndNotify after the exit frame is sent so a
// keep-alive connection serving many execs over a container's lifetime
// doesn't accumulate a stale *ExecSession per exec (and so a reused
// session ID can't collide with a dead entry).
func (r *SessionRegistry) Forget(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.forgetLocked(id, false)
}

// forgetLocked deletes the session from sessions and removes its id from
// every connSessions slice. Must be called with r.mu held. When close is
// true the session's Close() is invoked (caller still holds the lock, as
// Remove always has).
func (r *SessionRegistry) forgetLocked(id string, close bool) {
	s, ok := r.sessions[id]
	if !ok {
		return
	}
	if close {
		s.Close()
	}
	delete(r.sessions, id)
	for conn, ids := range r.connSessions {
		for i, sid := range ids {
			if sid == id {
				r.connSessions[conn] = append(ids[:i], ids[i+1:]...)
				break
			}
		}
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
