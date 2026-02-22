package agent

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"sync"

	"github.com/gorilla/websocket"
)

// ReverseAgentConn wraps a single persistent WebSocket connection for reverse
// (callback) mode. Unlike AgentConn which creates one connection per exec,
// ReverseAgentConn multiplexes concurrent exec sessions over the same connection
// using session IDs to route messages.
type ReverseAgentConn struct {
	ws       *websocket.Conn
	mu       sync.Mutex           // protects writes
	sessions sync.Map             // map[string]chan Message
	done     chan struct{}
	closeOnce sync.Once
}

// NewReverseAgentConn wraps an existing WebSocket connection and starts the
// read loop that dispatches incoming messages to registered sessions.
func NewReverseAgentConn(ws *websocket.Conn) *ReverseAgentConn {
	rc := &ReverseAgentConn{
		ws:   ws,
		done: make(chan struct{}),
	}
	go rc.readLoop()
	return rc
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

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		if msg.ID == "" {
			continue
		}

		if ch, ok := rc.sessions.Load(msg.ID); ok {
			select {
			case ch.(chan Message) <- msg:
			default:
				// Session channel full, drop message
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

// BridgeExec sends an exec command and bridges the session bidirectionally
// with the given connection using Docker mux protocol framing.
// This is the multiplexed equivalent of AgentConn.BridgeExec.
func (rc *ReverseAgentConn) BridgeExec(conn net.Conn, sessionID string, cmd []string, env []string, workdir string, tty bool) int {
	// Register session channel
	ch := make(chan Message, 64)
	rc.sessions.Store(sessionID, ch)
	defer rc.sessions.Delete(sessionID)

	// Send exec message
	_ = rc.SendJSON(Message{
		Type:    TypeExec,
		ID:      sessionID,
		Cmd:     cmd,
		Env:     env,
		WorkDir: workdir,
		Tty:     tty,
	})

	return rc.bridge(conn, sessionID, ch, tty)
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
						header := make([]byte, 8)
						header[0] = stream
						binary.BigEndian.PutUint32(header[4:], uint32(len(decoded)))
						_, _ = conn.Write(header)
						_, _ = conn.Write(decoded)
					}
				case TypeExit:
					code := 0
					if msg.Code != nil {
						code = *msg.Code
					}
					done <- code
					return
				case TypeError:
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
