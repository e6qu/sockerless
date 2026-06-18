package agent

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// reverseAgentPongWait bounds how long the backend will wait for any inbound
// frame (a data message OR a pong) before declaring the WebSocket dead and
// tearing it down. reverseAgentPingPeriod (< pongWait) is how often the
// backend sends a keepalive ping. Without this, a connection whose
// server→client direction silently stops delivering (e.g. a half-open
// connection behind a NAT/proxy) is never detected — an in-flight
// CollectExec would block forever instead of failing via rc.done so the
// caller can retry. The bootstrap (gorilla client) auto-answers pings during
// its serve-loop ReadMessage, so a healthy/idle connection stays alive.
const (
	reverseAgentPongWait   = 30 * time.Second
	reverseAgentPingPeriod = 10 * time.Second
)

// AgentExecErrorExitCode is the exit code surfaced when the agent reports
// a TypeError for an exec (e.g. a workspace-restore failure in a PreExec
// hook) instead of a normal process exit. Distinct from 126 (no session
// registered); mirrors the shell convention for "fatal, command did not
// run to a normal exit".
const AgentExecErrorExitCode = 255

// ReverseAgentConn wraps a single persistent WebSocket connection for reverse
// (callback) mode. Unlike AgentConn which creates one connection per exec,
// ReverseAgentConn multiplexes concurrent exec sessions over the same connection
// using session IDs to route messages.
type ReverseAgentConn struct {
	ws        *websocket.Conn
	mu        sync.Mutex // protects writes
	sessions  sync.Map   // map[string]chan Message
	done      chan struct{}
	closeOnce sync.Once

	// OnSystemMessage fires for connection-level messages that don't
	// belong to any session (msg.ID == ""). Currently the only such
	// type is TypeLifetimeExpired. May be nil.
	OnSystemMessage func(Message)

	// OnDroppedMessage fires when an inbound session message is dropped
	// because the session's buffered channel is full (the bridge consumer
	// fell behind). A dropped stdout/stderr frame silently corrupts the
	// exec stream, so the backend wires this to a Warn log. May be nil.
	OnDroppedMessage func(Message)
}

// NewReverseAgentConn wraps an existing WebSocket connection and starts the
// read loop that dispatches incoming messages to registered sessions.
// Callers that need to observe connection-level (no-ID) system messages
// like TypeLifetimeExpired MUST use NewReverseAgentConnWithSystemHandler
// instead — assigning OnSystemMessage after this constructor returns
// races with messages arriving in the first scheduler quantum.
func NewReverseAgentConn(ws *websocket.Conn) *ReverseAgentConn {
	rc := &ReverseAgentConn{
		ws:   ws,
		done: make(chan struct{}),
	}
	rc.startKeepalive()
	go rc.readLoop()
	return rc
}

// NewReverseAgentConnWithSystemHandler wraps an existing WebSocket
// connection, registers the connection-level system-message handler,
// and only then starts the read loop. Closes the race window where a
// TypeLifetimeExpired arriving immediately after the WS handshake
// would be dropped because OnSystemMessage hadn't been assigned yet.
func NewReverseAgentConnWithSystemHandler(ws *websocket.Conn, handler func(Message)) *ReverseAgentConn {
	return NewReverseAgentConnWithHandlers(ws, handler, nil)
}

// NewReverseAgentConnWithHandlers wraps a WebSocket connection, registers BOTH
// the connection-level system-message handler and the dropped-message handler,
// and only then starts the read loop. Either handler assigned AFTER the
// constructor returns races with messages arriving in the first scheduler
// quantum (the readLoop reads the fields concurrently), so both must be set up
// front.
func NewReverseAgentConnWithHandlers(ws *websocket.Conn, system, dropped func(Message)) *ReverseAgentConn {
	rc := &ReverseAgentConn{
		ws:               ws,
		done:             make(chan struct{}),
		OnSystemMessage:  system,
		OnDroppedMessage: dropped,
	}
	rc.startKeepalive()
	go rc.readLoop()
	return rc
}

// startKeepalive arms the read deadline + pong handler and launches the ping
// ticker. Together with the per-message deadline refresh in readLoop, this
// detects a connection whose server→client direction has silently stopped
// delivering and tears it down (closing rc.done) so blocked callers fail and
// retry instead of hanging. Must be called before readLoop starts.
func (rc *ReverseAgentConn) startKeepalive() {
	_ = rc.ws.SetReadDeadline(time.Now().Add(reverseAgentPongWait))
	rc.ws.SetPongHandler(func(string) error {
		return rc.ws.SetReadDeadline(time.Now().Add(reverseAgentPongWait))
	})
	go func() {
		t := time.NewTicker(reverseAgentPingPeriod)
		defer t.Stop()
		for {
			select {
			case <-rc.done:
				return
			case <-t.C:
				// WriteControl is safe to call concurrently with the
				// SendJSON writers (gorilla documents control writes as
				// concurrency-safe with NextWriter/WriteMessage).
				if err := rc.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(reverseAgentPingPeriod)); err != nil {
					return
				}
			}
		}
	}()
}

// readLoop reads messages from the WebSocket and dispatches them to the
// appropriate session channel based on the message ID.
func (rc *ReverseAgentConn) readLoop() {
	defer rc.closeOnce.Do(func() { close(rc.done) })
	for {
		_, data, err := rc.ws.ReadMessage()
		if err != nil {
			return
		}
		// Any inbound frame proves the link is alive — extend the deadline.
		_ = rc.ws.SetReadDeadline(time.Now().Add(reverseAgentPongWait))

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		if msg.ID == "" {
			if rc.OnSystemMessage != nil {
				rc.OnSystemMessage(msg)
			}
			continue
		}

		if ch, ok := rc.sessions.Load(msg.ID); ok {
			select {
			case ch.(chan Message) <- msg:
			default:
				// Session channel full — the bridge consumer fell behind.
				// Dropping silently corrupts the exec stream, so surface it.
				if rc.OnDroppedMessage != nil {
					rc.OnDroppedMessage(msg)
				}
			}
		}
	}
}

// SendJSON sends a JSON message over the WebSocket in a thread-safe manner.
func (rc *ReverseAgentConn) SendJSON(msg Message) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return rc.ws.WriteJSON(msg)
}

// SendLifetimeExpired writes a TypeLifetimeExpired system message
// (msg.ID == "") to a raw websocket connection. Used by FaaS
// bootstraps just before the platform's max invocation deadline
// would force-kill the function, so sockerless can mark the
// container Stopped with reason FaaSPodLifetimeExceeded and surface
// operator-guidance on the next ExecStart instead of a generic
// 500 / hung exec.
//
// `mu` serialises writes per gorilla/websocket's single-writer
// contract; bootstraps already maintain such a mutex shared with
// serveReverseAgent + sendHeartbeats. Pass the same mutex.
func SendLifetimeExpired(ws *websocket.Conn, mu *sync.Mutex) error {
	mu.Lock()
	defer mu.Unlock()
	return ws.WriteJSON(Message{Type: TypeLifetimeExpired})
}

// BridgeExec sends an exec command and bridges the session bidirectionally
// with the given connection using Docker mux protocol framing.
// This is the multiplexed equivalent of AgentConn.BridgeExec.
func (rc *ReverseAgentConn) BridgeExec(conn net.Conn, sessionID string, cmd []string, env []string, workdir string, tty bool) int {
	// Register session channel
	ch := make(chan Message, 64)
	rc.sessions.Store(sessionID, ch)
	defer rc.sessions.Delete(sessionID)

	// Send exec message. If even this fails the WebSocket is gone — the
	// bridge would otherwise block forever waiting for output that can
	// never arrive, so fail fast with a surfaced error.
	if err := rc.SendJSON(Message{
		Type:    TypeExec,
		ID:      sessionID,
		Cmd:     cmd,
		Env:     env,
		WorkDir: workdir,
		Tty:     tty,
	}); err != nil {
		errLine := []byte("sockerless: failed to dispatch exec to agent: " + err.Error() + "\n")
		if tty {
			_, _ = conn.Write(errLine)
		} else {
			frame := make([]byte, 8+len(errLine))
			frame[0] = 2 // stderr
			binary.BigEndian.PutUint32(frame[4:], uint32(len(errLine)))
			copy(frame[8:], errLine)
			_, _ = conn.Write(frame)
		}
		return AgentExecErrorExitCode
	}

	return rc.bridge(conn, sessionID, ch, tty)
}

// CollectExec runs a one-shot command on the remote agent and returns
// (stdout, stderr, exit code). Unlike BridgeExec there's no caller
// connection to multiplex — output is accumulated in memory and
// returned when the remote process exits. Intended for backend-driven
// introspection calls like `docker top` / `docker container stat`
// where we need the output as a value, not a streamed proxy.
func (rc *ReverseAgentConn) CollectExec(sessionID string, cmd []string, env []string, workdir string) (stdout, stderr []byte, exitCode int, err error) {
	ch := make(chan Message, 64)
	rc.sessions.Store(sessionID, ch)
	defer rc.sessions.Delete(sessionID)

	if err = rc.SendJSON(Message{
		Type:    TypeExec,
		ID:      sessionID,
		Cmd:     cmd,
		Env:     env,
		WorkDir: workdir,
	}); err != nil {
		return nil, nil, -1, err
	}

	for {
		select {
		case msg := <-ch:
			switch msg.Type {
			case TypeStdout:
				if b, derr := base64.StdEncoding.DecodeString(msg.Data); derr == nil {
					stdout = append(stdout, b...)
				}
			case TypeStderr:
				if b, derr := base64.StdEncoding.DecodeString(msg.Data); derr == nil {
					stderr = append(stderr, b...)
				}
			case TypeExit:
				if msg.Code != nil {
					exitCode = *msg.Code
				}
				return stdout, stderr, exitCode, nil
			case TypeError:
				return stdout, stderr, -1, fmt.Errorf("agent error: %s", msg.Message)
			}
		case <-rc.done:
			return stdout, stderr, -1, fmt.Errorf("agent connection closed")
		}
	}
}

// CollectExecWithStdin runs a one-shot command and feeds the given
// stdin bytes via TypeStdin messages before waiting for the exit.
// Returns (stdout, stderr, exit, err) once the remote process finishes.
// Needed by `docker cp` host→container: the tar body streams in as
// stdin to a `tar -xf - -C <dst>` exec inside the container.
func (rc *ReverseAgentConn) CollectExecWithStdin(sessionID string, cmd, env []string, workdir string, stdin []byte) (stdout, stderr []byte, exitCode int, err error) {
	ch := make(chan Message, 64)
	rc.sessions.Store(sessionID, ch)
	defer rc.sessions.Delete(sessionID)

	if err = rc.SendJSON(Message{
		Type:    TypeExec,
		ID:      sessionID,
		Cmd:     cmd,
		Env:     env,
		WorkDir: workdir,
	}); err != nil {
		return nil, nil, -1, err
	}

	// Stream stdin in 32 KiB chunks. The agent router forwards each chunk
	// to the subprocess's stdin pipe before the exit message fires.
	const chunkSize = 32 * 1024
	for off := 0; off < len(stdin); off += chunkSize {
		end := off + chunkSize
		if end > len(stdin) {
			end = len(stdin)
		}
		if err = rc.SendJSON(Message{
			Type: TypeStdin,
			ID:   sessionID,
			Data: base64.StdEncoding.EncodeToString(stdin[off:end]),
		}); err != nil {
			return nil, nil, -1, err
		}
	}
	if err = rc.SendJSON(Message{Type: TypeCloseStdin, ID: sessionID}); err != nil {
		return nil, nil, -1, err
	}

	for {
		select {
		case msg := <-ch:
			switch msg.Type {
			case TypeStdout:
				if b, derr := base64.StdEncoding.DecodeString(msg.Data); derr == nil {
					stdout = append(stdout, b...)
				}
			case TypeStderr:
				if b, derr := base64.StdEncoding.DecodeString(msg.Data); derr == nil {
					stderr = append(stderr, b...)
				}
			case TypeExit:
				if msg.Code != nil {
					exitCode = *msg.Code
				}
				return stdout, stderr, exitCode, nil
			case TypeError:
				return stdout, stderr, -1, fmt.Errorf("agent error: %s", msg.Message)
			}
		case <-rc.done:
			return stdout, stderr, -1, fmt.Errorf("agent connection closed")
		}
	}
}

// BridgeAttach sends an attach command and bridges the session bidirectionally.
func (rc *ReverseAgentConn) BridgeAttach(conn net.Conn, sessionID string, tty bool) int {
	// Register session channel
	ch := make(chan Message, 64)
	rc.sessions.Store(sessionID, ch)
	defer rc.sessions.Delete(sessionID)

	// Send attach message
	_ = rc.SendJSON(Message{
		Type: TypeAttach,
		ID:   sessionID,
	})

	return rc.bridge(conn, sessionID, ch, tty)
}

// bridge handles bidirectional streaming between a raw connection and the
// reverse agent connection using the session's message channel.
func (rc *ReverseAgentConn) bridge(conn net.Conn, sessionID string, ch chan Message, tty bool) int {
	done := make(chan int, 1)

	// Client -> Agent: read stdin from connection, send to agent as base64
	go func() {
		defer func() {
			_ = rc.SendJSON(Message{
				Type: TypeCloseStdin,
				ID:   sessionID,
			})
		}()
		buf := make([]byte, 32*1024)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				_ = rc.SendJSON(Message{
					Type: TypeStdin,
					ID:   sessionID,
					Data: base64.StdEncoding.EncodeToString(buf[:n]),
				})
			}
			if err != nil {
				return
			}
		}
	}()

	// Agent -> Client: read from session channel, write to connection with mux framing
	go func() {
		defer func() {
			if len(done) == 0 {
				done <- -1
			}
		}()
		for {
			select {
			case msg := <-ch:
				switch msg.Type {
				case TypeStdout, TypeStderr:
					decoded, err := base64.StdEncoding.DecodeString(msg.Data)
					if err != nil {
						continue
					}
					if tty {
						_, _ = conn.Write(decoded)
					} else {
						stream := byte(1) // stdout
						if msg.Type == TypeStderr {
							stream = 2
						}
						frame := make([]byte, 8+len(decoded))
						frame[0] = stream
						binary.BigEndian.PutUint32(frame[4:], uint32(len(decoded)))
						copy(frame[8:], decoded)
						_, _ = conn.Write(frame)
					}
				case TypeExit:
					code := 0
					if msg.Code != nil {
						code = *msg.Code
					}
					done <- code
					return
				case TypeError:
					// Surface the agent-reported error to the caller's
					// stderr stream so `docker exec` / the runner prints it,
					// and fail with a non-zero exit. Without this the stream
					// just closes and the caller sees an opaque exit 255 with
					// the real cause (e.g. a workspace-restore failure)
					// stranded in the workload's own logs.
					if msg.Message != "" {
						errLine := []byte("sockerless: agent exec error: " + msg.Message + "\n")
						if tty {
							_, _ = conn.Write(errLine)
						} else {
							frame := make([]byte, 8+len(errLine))
							frame[0] = 2 // stderr
							binary.BigEndian.PutUint32(frame[4:], uint32(len(errLine)))
							copy(frame[8:], errLine)
							_, _ = conn.Write(frame)
						}
					}
					done <- AgentExecErrorExitCode
					return
				}
			case <-rc.done:
				return
			}
		}
	}()

	select {
	case code := <-done:
		return code
	case <-rc.done:
		return -1
	}
}

// Close closes the underlying WebSocket connection.
func (rc *ReverseAgentConn) Close() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.closeOnce.Do(func() { close(rc.done) })
	return rc.ws.Close()
}

// Done returns a channel that is closed when the connection is lost.
func (rc *ReverseAgentConn) Done() <-chan struct{} {
	return rc.done
}

// ensure ReverseAgentConn satisfies io.Closer
var _ io.Closer = (*ReverseAgentConn)(nil)
