package aca

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
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

	// Build the command from the exec process config. ACA's console
	// exec API takes the command as a query parameter (matches the
	// behaviour of `az containerapp exec --command`).
	argv := append([]string{exec.ProcessConfig.Entrypoint}, exec.ProcessConfig.Arguments...)
	command := strings.Join(argv, " ")

	// The ACA exec API is a custom REST endpoint. Since the armappcontainers
	// SDK does not expose a direct exec method, we construct the URL manually.
	// Production:
	//   wss://management.azure.com/subscriptions/{sub}/resourceGroups/{rg}/
	//        providers/Microsoft.App/jobs/{job}/executions/{exec}/exec
	//        ?api-version=2024-03-01&command=<urlencoded>
	scheme := "wss"
	host := "management.azure.com"
	if s.config.EndpointURL != "" {
		u, perr := url.Parse(s.config.EndpointURL)
		if perr != nil {
			return nil, fmt.Errorf("parse EndpointURL: %w", perr)
		}
		host = u.Host
		switch u.Scheme {
		case "http":
			scheme = "ws"
		case "https":
			scheme = "wss"
		}
	}
	q := url.Values{}
	q.Set("api-version", "2024-03-01")
	q.Set("command", command)
	execURL := fmt.Sprintf(
		"%s://%s/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/jobs/%s/executions/%s/exec?%s",
		scheme, host,
		s.config.SubscriptionID, s.config.ResourceGroup,
		acaState.JobName, acaState.ExecutionName,
		q.Encode(),
	)

	s.Logger.Debug().
		Str("container", c.ID[:12]).
		Str("job", acaState.JobName).
		Str("execution", acaState.ExecutionName).
		Str("command", command).
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
