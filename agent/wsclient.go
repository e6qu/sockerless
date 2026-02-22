package agent

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// AgentConn wraps a WebSocket connection to an agent, providing
// bidirectional bridging between a raw TCP connection (Docker mux protocol)
// and the agent's WebSocket protocol.
type AgentConn struct {
	ws     *websocket.Conn
	mu     sync.Mutex
	closed bool
}

// Dial opens a WebSocket connection to the agent at the given address.
func Dial(agentAddr, agentToken string) (*AgentConn, error) {
	header := http.Header{}
	if agentToken != "" {
		header.Set("Authorization", "Bearer "+agentToken)
	}
	conn, _, err := websocket.DefaultDialer.Dial(
		fmt.Sprintf("ws://%s/ws", agentAddr),
		header,
	)
	if err != nil {
		return nil, err
	}
	return &AgentConn{ws: conn}, nil
}

// Close closes the WebSocket connection.
func (c *AgentConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return c.ws.Close()
}

// sendJSON sends a JSON message to the agent.
func (c *AgentConn) sendJSON(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return io.ErrClosedPipe
	}
	return c.ws.WriteJSON(msg)
}

// BridgeExec sends an exec command to the agent and bridges the session
// bidirectionally with the given connection using Docker mux protocol framing.
// The sessionID, cmd, env, workdir, and tty configure the exec session.
// This function blocks until the exec session exits or the connection closes.
func (c *AgentConn) BridgeExec(conn net.Conn, sessionID string, cmd []string, env []string, workdir string, tty bool) (exitCode int) {
	// Send exec message
	_ = c.sendJSON(Message{
		Type:    TypeExec,
		ID:      sessionID,
		Cmd:     cmd,
		Env:     env,
		WorkDir: workdir,
		Tty:     tty,
	})

	return c.bridge(conn, sessionID, tty)
}

// BridgeAttach sends an attach command to the agent and bridges the session
// bidirectionally with the given connection using Docker mux protocol framing.
func (c *AgentConn) BridgeAttach(conn net.Conn, sessionID string, tty bool) (exitCode int) {
	// Send attach message
	_ = c.sendJSON(Message{
		Type: TypeAttach,
		ID:   sessionID,
	})

	return c.bridge(conn, sessionID, tty)
}

// bridge handles bidirectional streaming between a raw connection and the agent WebSocket.
func (c *AgentConn) bridge(conn net.Conn, sessionID string, tty bool) int {
	done := make(chan int, 1)

	// Client → Agent: read stdin from connection, send to agent as base64
	go func() {
		defer func() {
			_ = c.sendJSON(Message{
				Type: TypeCloseStdin,
				ID:   sessionID,
			})
		}()
		buf := make([]byte, 32*1024)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				_ = c.sendJSON(Message{
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

	// Agent → Client: read from agent WebSocket, write to connection with mux framing
	go func() {
		defer func() {
			if len(done) == 0 {
				done <- -1
			}
		}()
		for {
			_, data, err := c.ws.ReadMessage()
			if err != nil {
				return
			}

			var msg Message
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}

			if msg.ID != sessionID {
				continue
			}

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
		}
	}()

	return <-done
}
