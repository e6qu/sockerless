package aca

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sockerless/api"
)

// cloudExecStart executes a command inside an ACA container using the
// Azure Container Apps exec API. The API returns a WebSocket URL for the
// interactive session, which we bridge as an io.ReadWriteCloser.
func (s *Server) cloudExecStart(exec *api.ExecInstance, c *api.Container) (io.ReadWriteCloser, error) {
	acaState, ok := s.resolveACAState(s.ctx(), c.ID)
	if !ok || acaState.JobName == "" {
		return nil, fmt.Errorf("no ACA job associated with container %s", c.ID[:12])
	}

	if acaState.ExecutionName == "" {
		return nil, fmt.Errorf("no ACA execution associated with container %s", c.ID[:12])
	}

	// Build the command from the exec process config.
	cmd := exec.ProcessConfig.Entrypoint
	if cmd == "" && len(exec.ProcessConfig.Arguments) > 0 {
		cmd = exec.ProcessConfig.Arguments[0]
	}

	// The ACA exec API is a custom REST endpoint. Since the armappcontainers
	// SDK does not expose a direct exec method, we construct the URL manually.
	// In production, this would be:
	//   POST https://management.azure.com/subscriptions/{sub}/resourceGroups/{rg}/
	//        providers/Microsoft.App/jobs/{job}/executions/{exec}/exec?api-version=2024-03-01
	// The response contains a WebSocket URL to connect to the interactive session.
	execURL := fmt.Sprintf(
		"wss://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/jobs/%s/executions/%s/exec",
		s.config.SubscriptionID, s.config.ResourceGroup,
		acaState.JobName, acaState.ExecutionName,
	)

	// If using a custom endpoint (simulator), adjust the URL.
	if s.config.EndpointURL != "" {
		execURL = fmt.Sprintf("%s/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/jobs/%s/executions/%s/exec",
			s.config.EndpointURL, s.config.SubscriptionID, s.config.ResourceGroup,
			acaState.JobName, acaState.ExecutionName,
		)
	}

	s.Logger.Debug().
		Str("container", c.ID[:12]).
		Str("job", acaState.JobName).
		Str("execution", acaState.ExecutionName).
		Str("command", cmd).
		Msg("connecting to ACA exec session")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.DialContext(s.ctx(), execURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ACA exec WebSocket: %w", err)
	}

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
