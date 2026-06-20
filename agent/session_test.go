package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
)

// mockSession implements Session for testing.
type mockSession struct {
	id     string
	closed bool
}

func (m *mockSession) ID() string                     { return m.id }
func (m *mockSession) WriteStdin(data []byte) error   { return nil }
func (m *mockSession) CloseStdin() error              { return nil }
func (m *mockSession) Signal(sig string) error        { return nil }
func (m *mockSession) Resize(width, height int) error { return nil }
func (m *mockSession) Close()                         { m.closed = true }

func TestSessionRegistryRegisterGet(t *testing.T) {
	r := NewSessionRegistry()
	s := &mockSession{id: "test-1"}

	// Need a real websocket.Conn as map key — create a minimal one
	conn := makeTestWSConn(t)
	defer conn.Close()

	r.Register(s, conn)

	got, ok := r.Get("test-1")
	if !ok {
		t.Fatal("session not found")
	}
	if got.ID() != "test-1" {
		t.Fatalf("expected ID test-1, got %s", got.ID())
	}
}

func TestSessionRegistryCleanupConn(t *testing.T) {
	r := NewSessionRegistry()
	s1 := &mockSession{id: "s1"}
	s2 := &mockSession{id: "s2"}

	conn := makeTestWSConn(t)
	defer conn.Close()

	r.Register(s1, conn)
	r.Register(s2, conn)

	r.CleanupConn(conn)

	if _, ok := r.Get("s1"); ok {
		t.Fatal("s1 should have been removed")
	}
	if _, ok := r.Get("s2"); ok {
		t.Fatal("s2 should have been removed")
	}
	if !s1.closed {
		t.Fatal("s1 should have been closed")
	}
	if !s2.closed {
		t.Fatal("s2 should have been closed")
	}
}

func TestSessionRegistryForget(t *testing.T) {
	r := NewSessionRegistry()
	s := &mockSession{id: "f1"}

	conn := makeTestWSConn(t)
	defer conn.Close()

	r.Register(s, conn)

	r.Forget("f1")

	if _, ok := r.Get("f1"); ok {
		t.Fatal("f1 should have been forgotten")
	}
	// Forget must NOT call Close() — the process already exited.
	if s.closed {
		t.Fatal("Forget must not Close the session (process already exited)")
	}

	// The id must also be removed from the per-conn slice so CleanupConn
	// doesn't re-process a stale entry.
	r.mu.RLock()
	ids := r.connSessions[conn]
	r.mu.RUnlock()
	for _, id := range ids {
		if id == "f1" {
			t.Fatal("forgotten id still present in connSessions")
		}
	}

	// Forgetting an unknown id is a no-op.
	r.Forget("does-not-exist")
}

func TestSessionRegistryForgetThenCleanupConn(t *testing.T) {
	r := NewSessionRegistry()
	s1 := &mockSession{id: "g1"}
	s2 := &mockSession{id: "g2"}

	conn := makeTestWSConn(t)
	defer conn.Close()

	r.Register(s1, conn)
	r.Register(s2, conn)

	// g1 finished and was forgotten; only g2 remains for the connection.
	r.Forget("g1")
	r.CleanupConn(conn)

	if _, ok := r.Get("g2"); ok {
		t.Fatal("g2 should be removed by CleanupConn")
	}
	if !s2.closed {
		t.Fatal("g2 should have been Closed by CleanupConn")
	}
	// g1 was already gone and must not have been Closed by either path.
	if s1.closed {
		t.Fatal("g1 was forgotten (process exited) and must never be Closed")
	}
}

// makeTestWSConn creates a websocket.Conn for use as a map key in tests.
func makeTestWSConn(t *testing.T) *websocket.Conn {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		// Keep connection alive until client closes
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				c.Close()
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	wsURL := "ws" + srv.URL[4:]
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}
