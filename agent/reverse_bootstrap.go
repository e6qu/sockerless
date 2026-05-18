package agent

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// BootstrapHeartbeatPeriod is how often the bootstrap pings the
// backend over the reverse-agent WS so the backend knows the
// container is alive. Kept short relative to typical FaaS idle
// timeouts (Lambda 15min, Cloud Run/GCF ~hours).
const BootstrapHeartbeatPeriod = 20 * time.Second

// DialReverseAgent opens the long-lived WebSocket to the sockerless
// backend's `/v1/<backend>/reverse?session_id=<containerID>` endpoint.
// Returns a raw *websocket.Conn — the caller wires it into
// ServeReverseAgent + StartHeartbeats. Used by every FaaS bootstrap
// (lambda, cloudrun, gcf, azf) so the workload container can receive
// exec / attach / stream messages from the backend.
func DialReverseAgent(callbackURL, containerID string) (*websocket.Conn, error) {
	u, err := url.Parse(callbackURL)
	if err != nil {
		return nil, fmt.Errorf("parse callback URL %q: %w", callbackURL, err)
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return nil, fmt.Errorf("callback URL %q must be ws:// or wss:// (got scheme %q)", callbackURL, u.Scheme)
	}
	q := u.Query()
	q.Set("session_id", containerID)
	u.RawQuery = q.Encode()
	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", u.String(), err)
	}
	return ws, nil
}

// ServeReverseAgent reads messages from the reverse-agent WebSocket
// and dispatches them to a fresh `Router`. Blocks until the WS closes.
// connMu is shared with StartHeartbeats so writes don't interleave —
// gorilla/websocket requires serialised single-writer access.
//
// Bootstraps call this in a goroutine immediately after a successful
// DialReverseAgent. The session registry is bootstrap-private; one
// registry per WS connection.
func ServeReverseAgent(conn *websocket.Conn, connMu *sync.Mutex) {
	logger := zerolog.New(os.Stderr).With().Str("component", "bootstrap-reverse-agent").Logger()
	registry := NewSessionRegistry()
	router := NewRouter(registry, nil, logger)
	defer registry.CleanupConn(conn)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		router.Handle(&msg, conn, connMu)
	}
}

// StartHeartbeats writes a ping frame every BootstrapHeartbeatPeriod
// so the backend knows the container is alive between invocations
// (especially relevant for Cloud Run / GCF where the container can
// idle minutes between requests). Exits when the WS is closed.
// connMu must be the same mutex passed to ServeReverseAgent.
func StartHeartbeats(conn *websocket.Conn, connMu *sync.Mutex) {
	t := time.NewTicker(BootstrapHeartbeatPeriod)
	defer t.Stop()
	for range t.C {
		connMu.Lock()
		err := conn.WriteMessage(websocket.PingMessage, nil)
		connMu.Unlock()
		if err != nil {
			return
		}
	}
}
