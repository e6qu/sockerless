package core

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/sockerless/agent"
)

// ReverseAgentRegistry tracks live reverse-agent WebSocket sessions —
// one per running container that has a reverse agent connected. Keyed
// by sockerless container ID (which the bootstrap sends as `session_id`
// on upgrade). Shared across every cloud backend that hosts an
// in-container dial-back agent (Lambda, Cloud Run, ACA, GCF, AZF).
type ReverseAgentRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*agent.ReverseAgentConn
	// waiters maps container ID -> channel closed by Register once
	// the session for that ID lands. ContainerStart uses
	// WaitForAgent to block on this so the first ExecStart can't
	// race the bootstrap dial-back.
	waiters map[string]chan struct{}
	// lifetimeExpired tracks containers whose in-FaaS bootstrap
	// signalled it's about to hit the platform's max invocation
	// deadline. Set by MarkLifetimeExpired; checked by ExecStart so
	// the operator sees an actionable error instead of a generic
	// timeout or 500 (BUG-1053).
	lifetimeExpired map[string]struct{}
}

// NewReverseAgentRegistry creates an empty registry.
func NewReverseAgentRegistry() *ReverseAgentRegistry {
	return &ReverseAgentRegistry{
		sessions:        map[string]*agent.ReverseAgentConn{},
		waiters:         map[string]chan struct{}{},
		lifetimeExpired: map[string]struct{}{},
	}
}

// MarkLifetimeExpired records that the in-container bootstrap for
// `id` is about to be torn down by its FaaS platform (Lambda 15min,
// GCF Gen2 60min, etc.). Called by HandleReverseAgentWS when the
// bootstrap sends agent.TypeLifetimeExpired. Inspected by ExecStart
// so the operator gets a clear "use long-lived backend instead"
// error rather than a generic 500 / hung exec.
func (r *ReverseAgentRegistry) MarkLifetimeExpired(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lifetimeExpired[id] = struct{}{}
}

// IsLifetimeExpired reports whether the container has been marked
// past its FaaS pod lifetime cap.
func (r *ReverseAgentRegistry) IsLifetimeExpired(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.lifetimeExpired[id]
	return ok
}

// Register saves a session keyed by container ID, closing any prior
// session under the same ID (the new dial wins — matches a reconnect
// resume model). Wakes any WaitForAgent callers for this id.
func (r *ReverseAgentRegistry) Register(id string, conn *agent.ReverseAgentConn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.sessions[id]; ok {
		_ = old.Close()
	}
	r.sessions[id] = conn
	if ch, ok := r.waiters[id]; ok {
		close(ch)
		delete(r.waiters, id)
	}
}

// WaitForAgent blocks until a session for `id` is registered or `ctx`
// is cancelled (context.WithTimeout is the typical caller pattern).
// Returns nil on registration, ctx.Err() on timeout / cancellation.
// Fast-paths when the session is already registered.
//
// Designed for cloud-backend ContainerStart paths: after the
// function/revision is "Active" the bootstrap still needs to start
// inside the container and dial back; the first ExecStart cannot
// proceed until the WebSocket is up. Per the no-fallback rule there
// is no exec path that doesn't require the agent.
func (r *ReverseAgentRegistry) WaitForAgent(ctx context.Context, id string) error {
	r.mu.Lock()
	if _, ok := r.sessions[id]; ok {
		r.mu.Unlock()
		return nil
	}
	ch, ok := r.waiters[id]
	if !ok {
		ch = make(chan struct{})
		r.waiters[id] = ch
	}
	r.mu.Unlock()

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		r.mu.Lock()
		if cur, ok := r.waiters[id]; ok && cur == ch {
			delete(r.waiters, id)
		}
		r.mu.Unlock()
		return ctx.Err()
	}
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
// Also clears the lifetime-expired marker so subsequent Register
// (e.g. a new container reusing the same ID after restart) starts
// fresh.
func (r *ReverseAgentRegistry) Drop(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.sessions[id]; ok {
		_ = c.Close()
		delete(r.sessions, id)
	}
	delete(r.lifetimeExpired, id)
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
		rc.OnSystemMessage = func(m agent.Message) {
			if m.Type == agent.TypeLifetimeExpired {
				reg.MarkLifetimeExpired(sessionID)
				logger.Warn().Str("container", sessionID).Msg("reverse-agent reported FaaS pod lifetime expiring; future exec will return operator-guidance error")
			}
		}
		reg.Register(sessionID, rc)
		logger.Debug().Str("session_id", sessionID).Msg("reverse-agent session registered")

		<-rc.Done()
		reg.Drop(sessionID)
		logger.Debug().Str("session_id", sessionID).Msg("reverse-agent session dropped")
	}
}

// TmpfsSizeFromEnv returns the per-backend tmpfs default size in MiB
// from `SOCKERLESS_<BACKEND>_TMPFS_SIZE_MIB` (default 2048 MiB).
// Invalid / non-positive values fail loud (no clamping). Consumed by
// backends that register `MemoryDriver` as a default storage backing
// (cloudrun + cloudrun-functions + ACA after Phase 168).
func TmpfsSizeFromEnv(backend string) (int, error) {
	name := "SOCKERLESS_" + strings.ToUpper(backend) + "_TMPFS_SIZE_MIB"
	raw := os.Getenv(name)
	if raw == "" {
		return 2048, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s=%q: %w (expected integer MiB)", name, raw, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("invalid %s=%d: must be positive MiB (no zero / negative — fail loud)", name, n)
	}
	return n, nil
}

// BootstrapTimeoutFromEnv returns the per-backend reverse-agent
// bootstrap-dial-back timeout. `SOCKERLESS_<BACKEND>_BOOTSTRAP_TIMEOUT_SEC`
// overrides; default 90 seconds. Invalid / non-positive values fail loud
// at the call site rather than silently clamping (no-fallback rule).
//
// Used by every FaaS-style backend's ContainerStart to bound the wait
// for the in-container bootstrap to register a reverse-agent.
func BootstrapTimeoutFromEnv(backend string) (time.Duration, error) {
	name := "SOCKERLESS_" + strings.ToUpper(backend) + "_BOOTSTRAP_TIMEOUT_SEC"
	raw := os.Getenv(name)
	if raw == "" {
		return 90 * time.Second, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s=%q: %w (expected integer seconds)", name, raw, err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("invalid %s=%d: must be positive seconds (no zero / negative timeout — fail loud)", name, n)
	}
	return time.Duration(n) * time.Second, nil
}

// ErrNoReverseAgent surfaces when a container has no live reverse-agent
// session. ContainerExec/Attach returns it (or exit code 126) when the
// bootstrap hasn't dialed yet, or the container was killed.
var ErrNoReverseAgent = errors.New("no reverse-agent session registered for container")

// ErrBootstrapNoPIDFile is returned by pause/unpause when the bootstrap
// inside the container does not write the main-PID file to the expected
// path. Callers translate this to a NotImplementedError that names the
// missing convention explicitly.
var ErrBootstrapNoPIDFile = errors.New("bootstrap did not write main-PID file")

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

// RunAndCapture executes a one-shot command in the container over the
// reverse-agent WS and collects stdout/stderr/exit-code. Returns
// (nil, nil, -1, ErrNoReverseAgent) if no session is registered. Used
// by ContainerTop / ContainerStatPath / ContainerChanges which need
// command output as a value rather than a streamed proxy.
func (r *ReverseAgentRegistry) RunAndCapture(containerID, sessionID string, cmd, env []string, workdir string) (stdout, stderr []byte, exitCode int, err error) {
	rc, ok := r.Resolve(containerID)
	if !ok {
		return nil, nil, -1, ErrNoReverseAgent
	}
	return rc.CollectExec(sessionID, cmd, env, workdir)
}

// RunAndCaptureWithStdin is RunAndCapture + stdin streaming. Used by
// docker cp host→container which pipes the tar body into
// `tar -xf - -C <dst>` inside the container.
func (r *ReverseAgentRegistry) RunAndCaptureWithStdin(containerID, sessionID string, cmd, env []string, workdir string, stdin []byte) (stdout, stderr []byte, exitCode int, err error) {
	rc, ok := r.Resolve(containerID)
	if !ok {
		return nil, nil, -1, ErrNoReverseAgent
	}
	return rc.CollectExecWithStdin(sessionID, cmd, env, workdir, stdin)
}

// LogBytes / LogSubscribe / LogUnsubscribe satisfy the StreamDriver
// interface but are no-ops for the reverse-agent path — log content
// comes from the cloud-native log store, not the attach channel.
func (d *ReverseAgentStreamDriver) LogBytes(_ string) []byte             { return nil }
func (d *ReverseAgentStreamDriver) LogSubscribe(_, _ string) chan []byte { return nil }
func (d *ReverseAgentStreamDriver) LogUnsubscribe(_, _ string)           {}
