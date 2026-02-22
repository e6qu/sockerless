package core

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sockerless/agent"
)

func makeTestReverseConn(t *testing.T) (*agent.ReverseAgentConn, *httptest.Server) {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		// Just hold the connection open
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))

	wsURL := "ws" + server.URL[4:]
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return agent.NewReverseAgentConn(ws), server
}

func TestAgentRegistryRegisterAndGet(t *testing.T) {
	registry := NewAgentRegistry()
	conn, server := makeTestReverseConn(t)
	defer server.Close()
	defer conn.Close()

	registry.Register("container-1", conn)

	got := registry.Get("container-1")
	if got != conn {
		t.Error("expected to get back the registered connection")
	}

	got = registry.Get("nonexistent")
	if got != nil {
		t.Error("expected nil for unknown container")
	}
}

func TestAgentRegistryRemove(t *testing.T) {
	registry := NewAgentRegistry()
	conn, server := makeTestReverseConn(t)
	defer server.Close()

	registry.Register("container-1", conn)
	registry.Remove("container-1")

	got := registry.Get("container-1")
	if got != nil {
		t.Error("expected nil after removal")
	}

	// Double remove should not panic
	registry.Remove("container-1")
}

func TestAgentRegistryWaitForAgentPreRegistered(t *testing.T) {
	registry := NewAgentRegistry()
	conn, server := makeTestReverseConn(t)
	defer server.Close()
	defer conn.Close()

	registry.Register("container-1", conn)

	// Should return immediately
	err := registry.WaitForAgent("container-1", 100*time.Millisecond)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestAgentRegistryWaitForAgentLateRegistration(t *testing.T) {
	registry := NewAgentRegistry()
	conn, server := makeTestReverseConn(t)
	defer server.Close()
	defer conn.Close()

	done := make(chan error)
	go func() {
		done <- registry.WaitForAgent("container-2", 5*time.Second)
	}()

	// Register after a short delay
	time.Sleep(50 * time.Millisecond)
	registry.Register("container-2", conn)

	err := <-done
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestAgentRegistryWaitForAgentTimeout(t *testing.T) {
	registry := NewAgentRegistry()

	err := registry.WaitForAgent("container-3", 50*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}
