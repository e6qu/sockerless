package lambda

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/sockerless/agent"
	core "github.com/sockerless/backend-core"
)

// TestLambdaExec_EndToEnd_OverReverseAgent is the end-to-end proof
// for docker exec routing: a fake "bootstrap" dials the backend's
// /v1/lambda/reverse endpoint, the backend's lambdaExecDriver routes
// an exec through the reverse-agent session, and the fake bootstrap
// echoes back the command's canned stdout + exit code. The test runs
// entirely in-process — no docker build, no simulator, no live AWS.
func TestLambdaExec_EndToEnd_OverReverseAgent(t *testing.T) {
	logger := zerolog.New(io.Discard)

	// Minimal Lambda Server — BaseServer's full wiring isn't needed
	// for the exec-driver round-trip; the exec driver only touches the
	// reverse-agent registry + WS.
	base := &core.BaseServer{Logger: logger, Mux: http.NewServeMux()}
	s := &Server{
		BaseServer:    base,
		reverseAgents: newReverseAgentRegistry(),
	}
	s.registerReverseAgentRoutes(logger)
	s.Drivers.Exec = &lambdaExecDriver{registry: s.reverseAgents, logger: logger}

	// Serve the reverse-agent endpoint.
	srv := httptest.NewServer(base.Mux)
	defer srv.Close()

	// --- Fake bootstrap: dial the reverse-agent WS and echo exec frames. ---
	containerID := "c-e2e-1"
	u, _ := url.Parse(srv.URL)
	wsURL := "ws://" + u.Host + "/v1/lambda/reverse?session_id=" + containerID
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("bootstrap dial: %v", err)
	}
	defer ws.Close()

	// Wait for the backend's WS upgrade handler to register the
	// session. We don't wrap ws on this side — the bootstrap goroutine
	// below owns all reads from the client WS.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := s.resolveReverseAgent(containerID); ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, ok := s.resolveReverseAgent(containerID); !ok {
		t.Fatal("backend never registered the reverse-agent session")
	}

	// Spawn a bootstrap-side goroutine that reads from the WS, handles
	// exec messages, and writes stdout + exit frames back. Matches
	// what the real sockerless-lambda-bootstrap will do once connected.
	bootstrapDone := make(chan struct{})
	go func() {
		defer close(bootstrapDone)
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			var msg agent.Message
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			if msg.Type != agent.TypeExec {
				continue
			}
			// Pretend to run msg.Cmd — echo its args joined by spaces.
			out := ""
			for i, a := range msg.Cmd {
				if i > 0 {
					out += " "
				}
				out += a
			}
			out += "\n"
			_ = ws.WriteJSON(agent.Message{
				Type: agent.TypeStdout,
				ID:   msg.ID,
				Data: base64.StdEncoding.EncodeToString([]byte(out)),
			})
			zero := 0
			_ = ws.WriteJSON(agent.Message{
				Type: agent.TypeExit,
				ID:   msg.ID,
				Code: &zero,
			})
		}
	}()

	// --- Client side: simulate the docker exec path. ---
	// We pass a piped net.Conn as the exec transport; the driver
	// writes Docker-mux-framed stdout into it and returns the exit
	// code when the bootstrap sends `exit`.
	clientEnd, driverEnd := net.Pipe()

	var exitCode int
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		exitCode = s.Drivers.Exec.Exec(
			context.Background(),
			containerID,
			"exec-1",
			[]string{"echo", "hello-from-lambda-exec"},
			nil,
			"",
			false, // no TTY -> Docker-mux framing
			driverEnd,
		)
		_ = driverEnd.Close()
	}()

	// Read the mux-framed stdout from the client side.
	var gotStdout []byte
	readBuf := make([]byte, 1024)
readLoop:
	for {
		_ = clientEnd.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := clientEnd.Read(readBuf)
		if n > 0 {
			// Docker multiplexed frame: 8-byte header, [0]=stream, [4-7]=length
			i := 0
			for i < n {
				if i+8 > n {
					break
				}
				l := int(readBuf[i+4])<<24 | int(readBuf[i+5])<<16 | int(readBuf[i+6])<<8 | int(readBuf[i+7])
				payloadStart := i + 8
				payloadEnd := payloadStart + l
				if payloadEnd > n {
					payloadEnd = n
				}
				gotStdout = append(gotStdout, readBuf[payloadStart:payloadEnd]...)
				i = payloadEnd
			}
		}
		if err != nil {
			break readLoop
		}
	}

	wg.Wait()

	if exitCode != 0 {
		t.Errorf("want exit 0, got %d", exitCode)
	}
	if string(gotStdout) != "echo hello-from-lambda-exec\n" {
		t.Errorf("unexpected exec stdout: got %q", string(gotStdout))
	}

	// --- Disconnect path: killing the container drops the session. ---
	s.disconnectReverseAgent(containerID)
	if _, ok := s.resolveReverseAgent(containerID); ok {
		t.Error("session should be gone after disconnectReverseAgent")
	}

	// A second exec against the killed container returns 126
	// (no-reverse-agent-session), matching the "NoSuchContainer"
	// behaviour at the API edge once the handler translates it.
	_, driverEnd2 := net.Pipe()
	code2 := s.Drivers.Exec.Exec(
		context.Background(),
		containerID,
		"exec-2",
		[]string{"echo", "after-kill"},
		nil, "", false, driverEnd2,
	)
	if code2 != 126 {
		t.Errorf("post-kill exec want 126, got %d", code2)
	}

	select {
	case <-bootstrapDone:
	case <-time.After(2 * time.Second):
		// bootstrap goroutine exits when the WS is torn down — may
		// linger briefly after disconnect. Don't fail the test.
	}
}
