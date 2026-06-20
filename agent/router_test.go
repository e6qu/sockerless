package agent

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// routerTestServer serves a single WebSocket connection through a real Router
// (with a real SessionRegistry, no main process, no reaper) so a test can run
// real execs and then inspect the registry. Returns the registry so the test
// can assert sessions are forgotten after the process exits.
func routerTestServer(t *testing.T) (*httptest.Server, *SessionRegistry) {
	t.Helper()
	registry := NewSessionRegistry()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		defer registry.CleanupConn(conn)
		connMu := &sync.Mutex{}
		router := NewRouter(registry, nil, nil, testLogger())
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
	}))
	t.Cleanup(srv.Close)
	return srv, registry
}

func dialRouter(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+srv.URL[4:]+"/ws", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return conn
}

// readExit drains output messages for id until the exit frame, returning its code.
func readExit(t *testing.T, conn *websocket.Conn, id string) (stdout string, code int) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil || msg.ID != id {
			continue
		}
		switch msg.Type {
		case TypeStdout:
			b, _ := base64.StdEncoding.DecodeString(msg.Data)
			stdout += string(b)
		case TypeExit:
			if msg.Code != nil {
				code = *msg.Code
			}
			return stdout, code
		case TypeError:
			t.Fatalf("unexpected error frame: %s", msg.Message)
		}
	}
}

// TestRouterForgetsFinishedSessions runs many execs over one keep-alive
// connection and asserts the registry doesn't accumulate finished sessions —
// the leak the Forget() hook closes.
func TestRouterForgetsFinishedSessions(t *testing.T) {
	srv, registry := routerTestServer(t)
	conn := dialRouter(t, srv)
	defer conn.Close()

	const n = 25
	for i := 0; i < n; i++ {
		id := "exec-" + string(rune('a'+i%26)) + string(rune('0'+i/10))
		if err := conn.WriteJSON(Message{Type: TypeExec, ID: id, Cmd: []string{"echo", id}}); err != nil {
			t.Fatalf("write exec: %v", err)
		}
		out, code := readExit(t, conn, id)
		if code != 0 {
			t.Fatalf("exec %s: expected exit 0, got %d", id, code)
		}
		if strings.TrimSpace(out) != id {
			t.Fatalf("exec %s: expected %q, got %q", id, id, strings.TrimSpace(out))
		}
	}

	// After the exit frames, every session must have been Forgotten. Allow a
	// brief settle for the onExit hook which runs after the exit frame send.
	deadline := time.Now().Add(2 * time.Second)
	for {
		registry.mu.RLock()
		nSessions := len(registry.sessions)
		var nConnIDs int
		for _, ids := range registry.connSessions {
			nConnIDs += len(ids)
		}
		registry.mu.RUnlock()
		if nSessions == 0 && nConnIDs == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("registry leaked sessions: sessions=%d connIDs=%d (want 0,0)", nSessions, nConnIDs)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestRouterExecExitCodes confirms exec exit codes survive (the per-PID reaper
// concern is only active under PID 1; on the host/non-reaper path cmd.Wait()
// must report real codes). A successful exec is 0; a failing one is non-zero.
func TestRouterExecExitCodes(t *testing.T) {
	srv, _ := routerTestServer(t)
	conn := dialRouter(t, srv)
	defer conn.Close()

	if err := conn.WriteJSON(Message{Type: TypeExec, ID: "ok", Cmd: []string{"true"}}); err != nil {
		t.Fatal(err)
	}
	if _, code := readExit(t, conn, "ok"); code != 0 {
		t.Fatalf("true: expected 0, got %d", code)
	}

	if err := conn.WriteJSON(Message{Type: TypeExec, ID: "fail", Cmd: []string{"sh", "-c", "exit 7"}}); err != nil {
		t.Fatal(err)
	}
	if _, code := readExit(t, conn, "fail"); code != 7 {
		t.Fatalf("exit 7: expected 7, got %d", code)
	}
}
