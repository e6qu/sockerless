package gcf

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/gorilla/websocket"
)

// dialFakeReverseAgent opens a WebSocket connection to the backend's
// `/v1/gcf/reverse` endpoint as if it were the in-function bootstrap.
// Returns a Cleanup that closes the conn. Used by integration tests
// to satisfy the P168.3 ContainerStart wait-for-reverse-agent
// contract — sim doesn't run the workload image, so no real bootstrap
// dial happens, and ContainerStart would otherwise time out at 90s.
//
// BUG-1066 follow-up: the broader migration of every test calling
// ContainerStart is staged. This helper exists so individual tests
// can opt in as they're migrated.
func dialFakeReverseAgent(t *testing.T, containerID string) func() {
	t.Helper()
	u := &url.URL{
		Scheme:   "ws",
		Host:     fmt.Sprintf("localhost:%d", backendPort),
		Path:     "/v1/gcf/reverse",
		RawQuery: "session_id=" + containerID,
	}
	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("fake reverse-agent dial: %v", err)
	}
	return func() { _ = ws.Close() }
}
