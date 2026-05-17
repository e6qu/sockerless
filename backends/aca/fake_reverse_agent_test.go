package aca

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/gorilla/websocket"
)

// dialFakeReverseAgent opens a WebSocket to the backend's
// `/v1/aca/reverse` endpoint as if it were the in-App/Job bootstrap.
// Returns a Cleanup that closes the conn. Used by integration tests
// to satisfy P168.3 ContainerStart wait-for-agent. BUG-1066.
func dialFakeReverseAgent(t *testing.T, containerID string) func() {
	t.Helper()
	u := &url.URL{
		Scheme:   "ws",
		Host:     fmt.Sprintf("localhost:%d", backendPort),
		Path:     "/v1/aca/reverse",
		RawQuery: "session_id=" + containerID,
	}
	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("fake reverse-agent dial: %v", err)
	}
	return func() { _ = ws.Close() }
}
