package agent

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var testUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// mockAgentServer creates a WebSocket server that echoes exec commands.
// It reads exec messages, runs a simple echo of the command, and sends exit.
func mockAgentServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		defer conn.Close()

		mu := &sync.Mutex{}

		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var msg Message
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}

			switch msg.Type {
			case TypeExec:
				// Simulate: send stdout with the command, then exit 0
				output := ""
				for i, c := range msg.Cmd {
					if i > 0 {
						output += " "
					}
					output += c
				}
				output += "\n"

				mu.Lock()
				conn.WriteJSON(Message{
					Type: TypeStdout,
					ID:   msg.ID,
					Data: base64.StdEncoding.EncodeToString([]byte(output)),
				})
				conn.WriteJSON(Message{
					Type: TypeExit,
					ID:   msg.ID,
					Code: intPtr(0),
				})
				mu.Unlock()

			case TypeAttach:
				mu.Lock()
				conn.WriteJSON(Message{
					Type: TypeStdout,
					ID:   msg.ID,
					Data: base64.StdEncoding.EncodeToString([]byte("attached\n")),
				})
				conn.WriteJSON(Message{
					Type: TypeExit,
					ID:   msg.ID,
					Code: intPtr(0),
				})
				mu.Unlock()

			case TypeStdin, TypeCloseStdin:
				// ignore
			}
		}
	}))
}

func TestReverseAgentConnBridgeExec(t *testing.T) {
	server := mockAgentServer(t)
	defer server.Close()

	wsURL := "ws" + server.URL[4:] // http:// -> ws://
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	rc := NewReverseAgentConn(ws)
	defer rc.Close()

	// Create a pipe to act as the Docker client connection
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	var exitCode int
	done := make(chan struct{})
	go func() {
		exitCode = rc.BridgeExec(serverConn, "test-1", []string{"echo", "hello"}, nil, "", false)
		close(done)
	}()

	// Read from client side — expect Docker mux framed output
	header := make([]byte, 8)
	if _, err := clientConn.Read(header); err != nil {
		t.Fatalf("read header: %v", err)
	}

	if header[0] != 1 {
		t.Errorf("expected stdout stream (1), got %d", header[0])
	}

	size := binary.BigEndian.Uint32(header[4:])
	payload := make([]byte, size)
	if _, err := clientConn.Read(payload); err != nil {
		t.Fatalf("read payload: %v", err)
	}

	if string(payload) != "echo hello\n" {
		t.Errorf("expected 'echo hello\\n', got %q", string(payload))
	}

	// Close client conn to unblock stdin reader
	clientConn.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("BridgeExec did not return")
	}

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestReverseAgentConnBridgeExecTTY(t *testing.T) {
	server := mockAgentServer(t)
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	rc := NewReverseAgentConn(ws)
	defer rc.Close()

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	var exitCode int
	done := make(chan struct{})
	go func() {
		exitCode = rc.BridgeExec(serverConn, "test-tty", []string{"echo", "hi"}, nil, "", true)
		close(done)
	}()

	// In TTY mode, output is raw (no mux header)
	buf := make([]byte, 256)
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if string(buf[:n]) != "echo hi\n" {
		t.Errorf("expected 'echo hi\\n', got %q", string(buf[:n]))
	}

	clientConn.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("BridgeExec did not return")
	}

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestReverseAgentConnConcurrentSessions(t *testing.T) {
	server := mockAgentServer(t)
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	rc := NewReverseAgentConn(ws)
	defer rc.Close()

	var wg sync.WaitGroup
	results := make([]int, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			clientConn, serverConn := net.Pipe()
			defer clientConn.Close()

			done := make(chan struct{})
			go func() {
				results[idx] = rc.BridgeExec(serverConn, "concurrent-"+string(rune('a'+idx)), []string{"echo", "test"}, nil, "", true)
				close(done)
			}()

			// Read output
			buf := make([]byte, 256)
			clientConn.Read(buf)
			clientConn.Close()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Errorf("session %d did not complete", idx)
			}
		}(i)
	}

	wg.Wait()

	for i, code := range results {
		if code != 0 {
			t.Errorf("session %d: expected exit code 0, got %d", i, code)
		}
	}
}

func TestReverseAgentConnDone(t *testing.T) {
	server := mockAgentServer(t)
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	rc := NewReverseAgentConn(ws)

	// Close should cause done to be signaled
	rc.Close()

	select {
	case <-rc.Done():
		// expected
	case <-time.After(time.Second):
		t.Fatal("Done() not signaled after Close()")
	}
}

func TestReverseAgentConnBridgeAttach(t *testing.T) {
	server := mockAgentServer(t)
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}

	rc := NewReverseAgentConn(ws)
	defer rc.Close()

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	var exitCode int
	done := make(chan struct{})
	go func() {
		exitCode = rc.BridgeAttach(serverConn, "attach-1", true)
		close(done)
	}()

	buf := make([]byte, 256)
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if string(buf[:n]) != "attached\n" {
		t.Errorf("expected 'attached\\n', got %q", string(buf[:n]))
	}

	clientConn.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("BridgeAttach did not return")
	}

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}
