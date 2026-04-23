package core

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/sockerless/agent"
)

// ReverseAgentRegistry tracks live reverse-agent WebSocket sessions —
// one per running container that has a reverse agent connected. Keyed
// by sockerless container ID (which the bootstrap sends as `session_id`
// on upgrade).
//
// Phase 96 (BUG-745): originally lived in the Lambda backend. Lifted
// into core so Cloud Run Jobs, ACA Jobs (and anywhere else that can
// host an in-container dial-back agent) share the same machinery.
type ReverseAgentRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*agent.ReverseAgentConn
}

// NewReverseAgentRegistry creates an empty registry.
func NewReverseAgentRegistry() *ReverseAgentRegistry {
	return &ReverseAgentRegistry{sessions: map[string]*agent.ReverseAgentConn{}}
}

// Register saves a session keyed by container ID, closing any prior
// session under the same ID (the new dial wins — matches a reconnect
// resume model).
func (r *ReverseAgentRegistry) Register(id string, conn *agent.ReverseAgentConn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.sessions[id]; ok {
		_ = old.Close()
	}
	r.sessions[id] = conn
}

// Resolve returns the live reverse-agent connection for a container,
// or (nil, false) if no session is registered.
func (r *ReverseAgentRegistry) Resolve(id string) (*agent.ReverseAgentConn, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.sessions[id]
	return c, ok
}

// Drop closes + removes a session (used by ContainerStop/Remove).
func (r *ReverseAgentRegistry) Drop(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.sessions[id]; ok {
		_ = c.Close()
		delete(r.sessions, id)
	}
}

// wsUpgrader used by every reverse-agent WebSocket endpoint. Origin
// check is permissive — bootstraps dial from inside containers with no
// browser-enforced Origin semantics.
var reverseAgentUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// HandleReverseAgentWS returns an http.HandlerFunc that upgrades to a
// WebSocket and registers the session. Call it with a per-backend
// registry + logger and mount under `/v1/<backend>/reverse`.
func HandleReverseAgentWS(reg *ReverseAgentRegistry, logger zerolog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.URL.Query().Get("session_id")
		if sessionID == "" {
			http.Error(w, "session_id query parameter is required", http.StatusBadRequest)
			return
		}
		ws, err := reverseAgentUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		rc := agent.NewReverseAgentConn(ws)
		reg.Register(sessionID, rc)
		logger.Debug().Str("session_id", sessionID).Msg("reverse-agent session registered")

		<-rc.Done()
		reg.Drop(sessionID)
		logger.Debug().Str("session_id", sessionID).Msg("reverse-agent session dropped")
	}
}

// ErrNoReverseAgent surfaces when a container has no live reverse-agent
// session. ContainerExec/Attach returns it (or exit code 126) when the
// bootstrap hasn't dialed yet, or the container was killed.
var ErrNoReverseAgent = errors.New("no reverse-agent session registered for container")

// ReverseAgentExecDriver routes `docker exec` through the registry.
// Returns exit code 126 when no session is registered — matches
// Docker's convention for "command cannot execute".
type ReverseAgentExecDriver struct {
	Registry *ReverseAgentRegistry
	Logger   zerolog.Logger
}

// Exec runs the command on the container's reverse agent. Blocks until
// the child process exits on the bootstrap side.
func (d *ReverseAgentExecDriver) Exec(
	_ context.Context,
	containerID string,
	execID string,
	cmd []string,
	env []string,
	workDir string,
	tty bool,
	conn net.Conn,
) int {
	if d.Registry == nil {
		d.Logger.Warn().Str("container", containerID).Msg("no reverse-agent registry")
		return 126
	}
	rc, ok := d.Registry.Resolve(containerID)
	if !ok {
		d.Logger.Warn().Str("container", containerID).Msg("no reverse-agent session (container not up, or killed)")
		return 126
	}
	return rc.BridgeExec(conn, execID, cmd, env, workDir, tty)
}

// ReverseAgentStreamDriver wires `docker attach` through the reverse-
// agent when connected. `docker logs` (non-follow) still reads from
// the backend's log path; this driver fills in Attach only.
type ReverseAgentStreamDriver struct {
	Registry *ReverseAgentRegistry
	Logger   zerolog.Logger
}

// Attach pipes the bidirectional stream onto the bootstrap's subprocess.
func (d *ReverseAgentStreamDriver) Attach(_ context.Context, containerID string, tty bool, conn net.Conn) error {
	if d.Registry == nil {
		return ErrNoReverseAgent
	}
	rc, ok := d.Registry.Resolve(containerID)
	if !ok {
		return ErrNoReverseAgent
	}
	rc.BridgeAttach(conn, "attach-"+containerID, tty)
	return nil
}

// LogBytes / LogSubscribe / LogUnsubscribe satisfy the StreamDriver
// interface but are no-ops for the reverse-agent path — log content
// comes from the cloud-native log store, not the attach channel.
func (d *ReverseAgentStreamDriver) LogBytes(_ string) []byte             { return nil }
func (d *ReverseAgentStreamDriver) LogSubscribe(_, _ string) chan []byte { return nil }
func (d *ReverseAgentStreamDriver) LogUnsubscribe(_, _ string)           {}
