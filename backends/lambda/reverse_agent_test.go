package lambda

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/sockerless/agent"
	core "github.com/sockerless/backend-core"
)

// TestReverseAgentServer_RegisterResolveDrop verifies the
// session-registry lifecycle: a WS client dials, the server registers
// it under the session_id, resolve() finds it, Close drops it.
func TestReverseAgentServer_RegisterResolveDrop(t *testing.T) {
	// Minimal Server shim — we don't need the full BaseServer stack for
	// this test, just the registry + the handler method. Logger is a
	// silent sink so handler debug calls don't panic.
	logger := zerolog.New(io.Discard)
	base := &core.BaseServer{Logger: logger, Mux: http.NewServeMux()}
	s := &Server{BaseServer: base, reverseAgents: newReverseAgentRegistry()}
	// Bind a real HTTP server to an ephemeral port so we can dial.
	srv := httptest.NewServer(nil)
	defer srv.Close()
	srv.Config.Handler = http.HandlerFunc(s.handleReverseAgentWS)

	u, _ := url.Parse(srv.URL)
	wsURL := "ws://" + u.Host + "/v1/lambda/reverse?session_id=c-abc"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	// Poll briefly for the async register.
	found := false
	for i := 0; i < 50; i++ {
		if _, ok := s.resolveReverseAgent("c-abc"); ok {
			found = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !found {
		t.Fatal("resolveReverseAgent never returned the registered session")
	}

	// disconnectReverseAgent drops + closes.
	s.disconnectReverseAgent("c-abc")
	if _, ok := s.resolveReverseAgent("c-abc"); ok {
		t.Error("session should be gone after disconnect")
	}
}

// TestReverseAgentServer_RouteMountedOnBaseMux verifies that the
// reverse-agent route is mounted on the same mux the BaseServer
// serves, so a real Lambda backend process answers
// /v1/lambda/reverse.
func TestReverseAgentServer_RouteMountedOnBaseMux(t *testing.T) {
	logger := zerolog.New(io.Discard)
	base := &core.BaseServer{Logger: logger, Mux: http.NewServeMux()}
	s := &Server{BaseServer: base, reverseAgents: newReverseAgentRegistry()}
	s.registerReverseAgentRoutes(logger)

	// Serve the base mux and dial the reverse WS route.
	srv := httptest.NewServer(base.Mux)
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	wsURL := "ws://" + u.Host + "/v1/lambda/reverse?session_id=e2e-sess"
	ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial %s: %v (status %v)", wsURL, err, resp)
	}
	defer ws.Close()

	// Session should be findable.
	found := false
	for i := 0; i < 50; i++ {
		if _, ok := s.resolveReverseAgent("e2e-sess"); ok {
			found = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !found {
		t.Fatal("session never registered via base mux route")
	}

	// Disconnect closes the WS + clears the registry.
	s.disconnectReverseAgent("e2e-sess")
	if _, ok := s.resolveReverseAgent("e2e-sess"); ok {
		t.Error("session should be gone after disconnect via base mux")
	}
}

// TestReverseAgentRegistry_ReplacesPriorSession verifies the reconnect
// path: a new WS dial under the same session_id closes the old conn.
func TestReverseAgentRegistry_ReplacesPriorSession(t *testing.T) {
	r := newReverseAgentRegistry()

	// Use fake ReverseAgentConns — the Close side-effect is what we
	// care about, and agent.NewReverseAgentConn requires a real
	// websocket, so we test via the registry's internal contract.
	ws1 := fakeWS(t)
	ws2 := fakeWS(t)
	rc1 := agent.NewReverseAgentConn(ws1)
	rc2 := agent.NewReverseAgentConn(ws2)

	r.register("dup", rc1)
	r.register("dup", rc2)

	if got, _ := r.resolve("dup"); got != rc2 {
		t.Errorf("resolve should return the newer session")
	}
	// rc1 should be closed — Done() chan is closed on Close.
	select {
	case <-rc1.Done():
	case <-time.After(time.Second):
		t.Error("first session was not closed on replacement")
	}
}

// fakeWS spins up a throwaway WebSocket server + dials it, returning
// the client side of the WS. Used only to exercise the registry's
// Close lifecycle without full Lambda setup.
func fakeWS(t *testing.T) *websocket.Conn {
	t.Helper()
	var client *websocket.Conn
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		// Keep server side open — the test controls close via the client.
		<-make(chan struct{})
		_ = ws
	}))
	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)
	wsURL := strings.Replace("ws://"+u.Host, "http://", "", 1)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("fakeWS dial: %v", err)
	}
	client = ws
	return client
}
