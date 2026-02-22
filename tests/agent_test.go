package tests

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// agentMessage mirrors the agent's Message struct for test use.
type agentMessage struct {
	Type    string   `json:"type"`
	ID      string   `json:"id,omitempty"`
	Cmd     []string `json:"cmd,omitempty"`
	Env     []string `json:"env,omitempty"`
	WorkDir string   `json:"workdir,omitempty"`
	Tty     bool     `json:"tty,omitempty"`
	Data    string   `json:"data,omitempty"`
	Signal  string   `json:"signal,omitempty"`
	Code    *int     `json:"code,omitempty"`
	Message string   `json:"message,omitempty"`
	Status  string   `json:"status,omitempty"`
	Width   int      `json:"width,omitempty"`
	Height  int      `json:"height,omitempty"`
}

var (
	agentAddr string
	agentCmd  *exec.Cmd
)

// startAgent builds and starts the agent binary for tests.
func startAgent(t *testing.T, keepAlive bool, args ...string) (addr string, cleanup func()) {
	t.Helper()

	// Build agent
	agentDir := findModuleDir("agent")
	buildCmd := exec.Command("go", "build", "-o", "sockerless-agent-test", "./cmd/sockerless-agent/")
	buildCmd.Dir = agentDir
	buildCmd.Stdout = os.Stderr
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build agent: %v", err)
	}

	port := findFreePort()
	addr = fmt.Sprintf("localhost:%d", port)

	cmdArgs := []string{"--addr", fmt.Sprintf(":%d", port), "--log-level", "debug"}
	if keepAlive {
		cmdArgs = append(cmdArgs, "--keep-alive")
		cmdArgs = append(cmdArgs, "--")
		cmdArgs = append(cmdArgs, args...)
	}

	agentBin := agentDir + "/sockerless-agent-test"
	cmd := exec.Command(agentBin, cmdArgs...)
	cmd.Env = append(os.Environ(), "SOCKERLESS_AGENT_TOKEN=testtoken")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start agent: %v", err)
	}

	// Wait for health endpoint
	healthURL := fmt.Sprintf("http://%s/health", addr)
	if err := waitForReady(healthURL, 10*time.Second); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatalf("agent not ready: %v", err)
	}

	return addr, func() {
		cmd.Process.Kill()
		cmd.Wait()
		os.Remove(agentBin)
	}
}

// dialAgent opens a WebSocket connection to the agent.
func dialAgent(t *testing.T, addr string) *websocket.Conn {
	t.Helper()
	header := http.Header{}
	header.Set("Authorization", "Bearer testtoken")
	conn, _, err := websocket.DefaultDialer.Dial(
		fmt.Sprintf("ws://%s/ws", addr),
		header,
	)
	if err != nil {
		t.Fatalf("failed to dial agent: %v", err)
	}
	return conn
}

// readMessage reads a single message from the WebSocket, skipping messages not for the given session.
func readMessage(t *testing.T, conn *websocket.Conn, sessionID string, timeout time.Duration) agentMessage {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(timeout))
	defer conn.SetReadDeadline(time.Time{})

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read message: %v", err)
		}
		var msg agentMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("failed to unmarshal message: %v", err)
		}
		if sessionID == "" || msg.ID == sessionID {
			return msg
		}
	}
}

// collectOutput reads messages until an exit message, collecting stdout/stderr.
func collectOutput(t *testing.T, conn *websocket.Conn, sessionID string, timeout time.Duration) (stdout, stderr string, exitCode int) {
	t.Helper()
	var stdoutBuf, stderrBuf strings.Builder
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		msg := readMessage(t, conn, sessionID, time.Until(deadline))
		switch msg.Type {
		case "stdout":
			data, _ := base64.StdEncoding.DecodeString(msg.Data)
			stdoutBuf.Write(data)
		case "stderr":
			data, _ := base64.StdEncoding.DecodeString(msg.Data)
			stderrBuf.Write(data)
		case "exit":
			if msg.Code != nil {
				return stdoutBuf.String(), stderrBuf.String(), *msg.Code
			}
			return stdoutBuf.String(), stderrBuf.String(), -1
		case "error":
			t.Fatalf("received error: %s", msg.Message)
		}
	}
	t.Fatal("timeout waiting for exit message")
	return
}

// --- Tests ---

func TestAgentHealthEndpoint(t *testing.T) {
	addr, cleanup := startAgent(t, false)
	defer cleanup()

	resp, err := http.Get(fmt.Sprintf("http://%s/health", addr))
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", result["status"])
	}
}

func TestAgentAuthRequired(t *testing.T) {
	addr, cleanup := startAgent(t, false)
	defer cleanup()

	// No auth header — should fail
	_, _, err := websocket.DefaultDialer.Dial(
		fmt.Sprintf("ws://%s/ws", addr),
		nil,
	)
	if err == nil {
		t.Fatal("expected auth error, got nil")
	}

	// Wrong token — should fail
	header := http.Header{}
	header.Set("Authorization", "Bearer wrongtoken")
	_, _, err = websocket.DefaultDialer.Dial(
		fmt.Sprintf("ws://%s/ws", addr),
		header,
	)
	if err == nil {
		t.Fatal("expected auth error for wrong token, got nil")
	}

	// Health should work without auth
	resp, err := http.Get(fmt.Sprintf("http://%s/health", addr))
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("health expected 200, got %d", resp.StatusCode)
	}
}

func TestAgentExecSimple(t *testing.T) {
	addr, cleanup := startAgent(t, false)
	defer cleanup()

	conn := dialAgent(t, addr)
	defer conn.Close()

	// Execute 'echo hello'
	conn.WriteJSON(agentMessage{
		Type: "exec",
		ID:   "test-1",
		Cmd:  []string{"echo", "hello"},
	})

	stdout, _, exitCode := collectOutput(t, conn, "test-1", 5*time.Second)
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if strings.TrimSpace(stdout) != "hello" {
		t.Errorf("expected 'hello', got %q", stdout)
	}
}

func TestAgentExecStdin(t *testing.T) {
	addr, cleanup := startAgent(t, false)
	defer cleanup()

	conn := dialAgent(t, addr)
	defer conn.Close()

	// Execute 'cat' which reads from stdin
	conn.WriteJSON(agentMessage{
		Type: "exec",
		ID:   "test-stdin",
		Cmd:  []string{"cat"},
	})

	// Send some data to stdin
	inputData := "hello from stdin\n"
	conn.WriteJSON(agentMessage{
		Type: "stdin",
		ID:   "test-stdin",
		Data: base64.StdEncoding.EncodeToString([]byte(inputData)),
	})

	// Close stdin to let cat exit
	conn.WriteJSON(agentMessage{
		Type: "close_stdin",
		ID:   "test-stdin",
	})

	stdout, _, exitCode := collectOutput(t, conn, "test-stdin", 5*time.Second)
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if strings.TrimSpace(stdout) != "hello from stdin" {
		t.Errorf("expected 'hello from stdin', got %q", stdout)
	}
}

func TestAgentExecWithEnv(t *testing.T) {
	addr, cleanup := startAgent(t, false)
	defer cleanup()

	conn := dialAgent(t, addr)
	defer conn.Close()

	conn.WriteJSON(agentMessage{
		Type: "exec",
		ID:   "test-env",
		Cmd:  []string{"sh", "-c", "echo $MY_TEST_VAR"},
		Env:  []string{"MY_TEST_VAR=test_value_123"},
	})

	stdout, _, exitCode := collectOutput(t, conn, "test-env", 5*time.Second)
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if strings.TrimSpace(stdout) != "test_value_123" {
		t.Errorf("expected 'test_value_123', got %q", stdout)
	}
}

func TestAgentExecWithWorkDir(t *testing.T) {
	addr, cleanup := startAgent(t, false)
	defer cleanup()

	conn := dialAgent(t, addr)
	defer conn.Close()

	conn.WriteJSON(agentMessage{
		Type:    "exec",
		ID:      "test-workdir",
		Cmd:     []string{"pwd"},
		WorkDir: "/tmp",
	})

	stdout, _, exitCode := collectOutput(t, conn, "test-workdir", 5*time.Second)
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	// On macOS /tmp -> /private/tmp
	out := strings.TrimSpace(stdout)
	if out != "/tmp" && out != "/private/tmp" {
		t.Errorf("expected '/tmp' or '/private/tmp', got %q", out)
	}
}

func TestAgentExecNonZeroExit(t *testing.T) {
	addr, cleanup := startAgent(t, false)
	defer cleanup()

	conn := dialAgent(t, addr)
	defer conn.Close()

	conn.WriteJSON(agentMessage{
		Type: "exec",
		ID:   "test-exit",
		Cmd:  []string{"sh", "-c", "exit 42"},
	})

	_, _, exitCode := collectOutput(t, conn, "test-exit", 5*time.Second)
	if exitCode != 42 {
		t.Errorf("expected exit code 42, got %d", exitCode)
	}
}

func TestAgentExecStderr(t *testing.T) {
	addr, cleanup := startAgent(t, false)
	defer cleanup()

	conn := dialAgent(t, addr)
	defer conn.Close()

	conn.WriteJSON(agentMessage{
		Type: "exec",
		ID:   "test-stderr",
		Cmd:  []string{"sh", "-c", "echo error_output >&2"},
	})

	_, stderr, exitCode := collectOutput(t, conn, "test-stderr", 5*time.Second)
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if strings.TrimSpace(stderr) != "error_output" {
		t.Errorf("expected 'error_output' on stderr, got %q", stderr)
	}
}

func TestAgentExecSignal(t *testing.T) {
	addr, cleanup := startAgent(t, false)
	defer cleanup()

	conn := dialAgent(t, addr)
	defer conn.Close()

	// Start a long-running process
	conn.WriteJSON(agentMessage{
		Type: "exec",
		ID:   "test-signal",
		Cmd:  []string{"sleep", "60"},
	})

	// Give it time to start
	time.Sleep(200 * time.Millisecond)

	// Send SIGTERM
	conn.WriteJSON(agentMessage{
		Type:   "signal",
		ID:     "test-signal",
		Signal: "SIGTERM",
	})

	// Should exit with signal
	_, _, exitCode := collectOutput(t, conn, "test-signal", 5*time.Second)
	// SIGTERM typically results in exit code -1 or 143
	if exitCode == 0 {
		t.Error("expected non-zero exit code after SIGTERM")
	}
}

func TestAgentExecConcurrent(t *testing.T) {
	addr, cleanup := startAgent(t, false)
	defer cleanup()

	conn := dialAgent(t, addr)
	defer conn.Close()

	// Launch 3 concurrent exec sessions
	sessions := []struct {
		id  string
		cmd []string
		exp string
	}{
		{"concurrent-1", []string{"echo", "one"}, "one"},
		{"concurrent-2", []string{"echo", "two"}, "two"},
		{"concurrent-3", []string{"echo", "three"}, "three"},
	}

	for _, s := range sessions {
		conn.WriteJSON(agentMessage{
			Type: "exec",
			ID:   s.id,
			Cmd:  s.cmd,
		})
	}

	// Collect all results
	results := make(map[string]string)
	var mu sync.Mutex
	exitCodes := make(map[string]int)

	deadline := time.Now().Add(10 * time.Second)
	conn.SetReadDeadline(deadline)

	for len(results) < 3 && time.Now().Before(deadline) {
		_, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var msg agentMessage
		json.Unmarshal(data, &msg)

		mu.Lock()
		switch msg.Type {
		case "stdout":
			decoded, _ := base64.StdEncoding.DecodeString(msg.Data)
			results[msg.ID] += string(decoded)
		case "exit":
			if msg.Code != nil {
				exitCodes[msg.ID] = *msg.Code
			}
		}
		mu.Unlock()

		if len(exitCodes) >= 3 {
			break
		}
	}
	conn.SetReadDeadline(time.Time{})

	for _, s := range sessions {
		if exitCodes[s.id] != 0 {
			t.Errorf("session %s: expected exit code 0, got %d", s.id, exitCodes[s.id])
		}
		if strings.TrimSpace(results[s.id]) != s.exp {
			t.Errorf("session %s: expected %q, got %q", s.id, s.exp, strings.TrimSpace(results[s.id]))
		}
	}
}

func TestAgentKeepAlive(t *testing.T) {
	addr, cleanup := startAgent(t, true, "sh", "-c", "echo main_process_output && sleep 30")
	defer cleanup()

	// Health endpoint should show the process
	resp, err := http.Get(fmt.Sprintf("http://%s/health", addr))
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if result["exited"] != false {
		t.Errorf("expected process not exited, got %v", result["exited"])
	}
	pid, ok := result["pid"].(float64)
	if !ok || pid == 0 {
		t.Errorf("expected valid pid, got %v", result["pid"])
	}

	// Exec should work alongside main process
	conn := dialAgent(t, addr)
	defer conn.Close()

	conn.WriteJSON(agentMessage{
		Type: "exec",
		ID:   "ka-exec",
		Cmd:  []string{"echo", "exec_while_keepalive"},
	})

	stdout, _, exitCode := collectOutput(t, conn, "ka-exec", 5*time.Second)
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if strings.TrimSpace(stdout) != "exec_while_keepalive" {
		t.Errorf("expected 'exec_while_keepalive', got %q", stdout)
	}
}

func TestAgentAttach(t *testing.T) {
	addr, cleanup := startAgent(t, true, "sh", "-c", "echo attach_output && sleep 30")
	defer cleanup()

	// Give the process time to produce output
	time.Sleep(500 * time.Millisecond)

	conn := dialAgent(t, addr)
	defer conn.Close()

	// Attach to main process
	conn.WriteJSON(agentMessage{
		Type: "attach",
		ID:   "test-attach",
	})

	// Should receive buffered output
	msg := readMessage(t, conn, "test-attach", 5*time.Second)
	if msg.Type != "stdout" {
		t.Fatalf("expected stdout message, got %s", msg.Type)
	}

	data, _ := base64.StdEncoding.DecodeString(msg.Data)
	if !strings.Contains(string(data), "attach_output") {
		t.Errorf("expected buffered output to contain 'attach_output', got %q", string(data))
	}
}

func TestAgentAttachStdin(t *testing.T) {
	addr, cleanup := startAgent(t, true, "cat")
	defer cleanup()

	conn := dialAgent(t, addr)
	defer conn.Close()

	// Attach to main process
	conn.WriteJSON(agentMessage{
		Type: "attach",
		ID:   "test-attach-stdin",
	})

	// Write to stdin
	conn.WriteJSON(agentMessage{
		Type: "stdin",
		ID:   "test-attach-stdin",
		Data: base64.StdEncoding.EncodeToString([]byte("hello_attach\n")),
	})

	// Should get echoed output back
	msg := readMessage(t, conn, "test-attach-stdin", 5*time.Second)
	if msg.Type != "stdout" {
		t.Fatalf("expected stdout, got %s", msg.Type)
	}

	data, _ := base64.StdEncoding.DecodeString(msg.Data)
	if !strings.Contains(string(data), "hello_attach") {
		t.Errorf("expected 'hello_attach' in output, got %q", string(data))
	}
}

func TestAgentExecErrorHandling(t *testing.T) {
	addr, cleanup := startAgent(t, false)
	defer cleanup()

	conn := dialAgent(t, addr)
	defer conn.Close()

	// Exec with no command
	conn.WriteJSON(agentMessage{
		Type: "exec",
		ID:   "test-no-cmd",
	})

	msg := readMessage(t, conn, "test-no-cmd", 5*time.Second)
	if msg.Type != "error" {
		t.Errorf("expected error message, got %s", msg.Type)
	}

	// Exec with non-existent binary
	conn.WriteJSON(agentMessage{
		Type: "exec",
		ID:   "test-bad-cmd",
		Cmd:  []string{"/nonexistent/binary"},
	})

	msg = readMessage(t, conn, "test-bad-cmd", 5*time.Second)
	if msg.Type != "error" {
		t.Errorf("expected error message for bad command, got %s", msg.Type)
	}
}

func TestAgentAttachNoMainProcess(t *testing.T) {
	addr, cleanup := startAgent(t, false) // No keep-alive = no main process
	defer cleanup()

	conn := dialAgent(t, addr)
	defer conn.Close()

	// Attempt to attach when there's no main process
	conn.WriteJSON(agentMessage{
		Type: "attach",
		ID:   "test-no-mp",
	})

	msg := readMessage(t, conn, "test-no-mp", 5*time.Second)
	if msg.Type != "error" {
		t.Errorf("expected error for attach without main process, got %s", msg.Type)
	}
}
