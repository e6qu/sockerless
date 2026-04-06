package ecs

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsecs "github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/gorilla/websocket"
	"github.com/sockerless/api"
)

// cloudExecStart executes a command inside an ECS task using the
// ExecuteCommand API (backed by SSM Session Manager). It returns an
// io.ReadWriteCloser that bridges the SSM WebSocket session.
func (s *Server) cloudExecStart(exec *api.ExecInstance, c *api.Container) (io.ReadWriteCloser, error) {
	ecsState, ok := s.ECS.Get(c.ID)
	if !ok || ecsState.TaskARN == "" {
		return nil, fmt.Errorf("no ECS task associated with container %s", c.ID[:12])
	}

	cluster := s.config.Cluster
	if ecsState.ClusterARN != "" {
		cluster = ecsState.ClusterARN
	}

	// Build the full command string from the exec process config.
	// ECS ExecuteCommand takes a single command string.
	parts := []string{}
	if exec.ProcessConfig.Entrypoint != "" {
		parts = append(parts, exec.ProcessConfig.Entrypoint)
	}
	parts = append(parts, exec.ProcessConfig.Arguments...)
	cmd := strings.Join(parts, " ")

	result, err := s.aws.ECS.ExecuteCommand(s.ctx(), &awsecs.ExecuteCommandInput{
		Cluster:     aws.String(cluster),
		Task:        aws.String(ecsState.TaskARN),
		Command:     aws.String(cmd),
		Interactive: true,
	})
	if err != nil {
		return nil, fmt.Errorf("ECS ExecuteCommand failed: %w", err)
	}

	if result.Session == nil || result.Session.StreamUrl == nil {
		return nil, fmt.Errorf("ECS ExecuteCommand returned no session")
	}

	streamURL := aws.ToString(result.Session.StreamUrl)
	s.Logger.Debug().
		Str("container", c.ID[:12]).
		Str("stream_url", streamURL).
		Msg("connecting to ECS exec session")

	// Dial the SSM Session Manager WebSocket endpoint.
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.DialContext(s.ctx(), streamURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to exec session WebSocket: %w", err)
	}

	// Set a close handler so that when the server closes the connection,
	// the next Read() call returns promptly.
	conn.SetCloseHandler(func(code int, text string) error {
		_ = conn.SetReadDeadline(time.Now())
		return nil
	})

	return newWSBridge(conn), nil
}

// wsBridge adapts a gorilla/websocket.Conn to io.ReadWriteCloser.
type wsBridge struct {
	conn   *websocket.Conn
	mu     sync.Mutex
	reader io.Reader // current message reader
}

func newWSBridge(conn *websocket.Conn) *wsBridge {
	return &wsBridge{conn: conn}
}

// Read reads from the WebSocket, consuming binary/text messages sequentially.
func (w *wsBridge) Read(p []byte) (int, error) {
	for {
		if w.reader != nil {
			n, err := w.reader.Read(p)
			if err == io.EOF {
				w.reader = nil
				if n > 0 {
					return n, nil
				}
				continue
			}
			return n, err
		}

		_, r, err := w.conn.NextReader()
		if err != nil {
			return 0, err
		}
		w.reader = r
	}
}

// Write sends a binary message on the WebSocket.
func (w *wsBridge) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Close closes the underlying WebSocket connection.
func (w *wsBridge) Close() error {
	return w.conn.Close()
}

// LocalAddr satisfies net.Conn if needed (returns nil).
func (w *wsBridge) LocalAddr() net.Addr  { return nil }
func (w *wsBridge) RemoteAddr() net.Addr { return nil }
