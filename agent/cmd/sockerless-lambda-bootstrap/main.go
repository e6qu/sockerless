// sockerless-lambda-bootstrap is the entrypoint binary injected into
// Lambda container images by the sockerless Lambda backend. It wraps a
// user's entrypoint so the Lambda function can serve both (a) the
// user's declared workload and (b) a reverse-agent WebSocket that
// sockerless uses to proxy "docker exec" / "docker attach" into the
// running invocation.
//
// Architecture (Phase 86 D.1):
//  1. A long-lived reverse-agent connects back to sockerless's Lambda
//     backend over WebSocket (SOCKERLESS_CALLBACK_URL). The backend
//     forwards docker exec / attach frames here; this process handles
//     them via the agent.Exec* helpers.
//  2. In parallel, the Runtime-API loop polls
//     $AWS_LAMBDA_RUNTIME_API/2018-06-01/runtime/invocation/next, spawns
//     the user's declared entrypoint+cmd as a subprocess, captures
//     stdout, and posts the result to /response (or /error on non-zero
//     exit).
//  3. Heartbeats travel over the reverse-agent connection (WebSocket
//     ping) every 20s so the backend knows the session is alive.
//
// See docs/LAMBDA_EXEC_DESIGN.md for the design doc.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sockerless/agent"
)

// Env vars the bootstrap consults.
const (
	envRuntimeAPI     = "AWS_LAMBDA_RUNTIME_API"
	envCallbackURL    = "SOCKERLESS_CALLBACK_URL"
	envContainerID    = "SOCKERLESS_CONTAINER_ID"
	envUserEntrypoint = "SOCKERLESS_USER_ENTRYPOINT" // colon-separated list
	envUserCmd        = "SOCKERLESS_USER_CMD"        // colon-separated list
)

const (
	runtimeAPIPath   = "/2018-06-01/runtime/invocation"
	runtimeInitError = "/2018-06-01/runtime/init/error"
	heartbeatPeriod  = 20 * time.Second
)

func main() {
	runtimeAPI := os.Getenv(envRuntimeAPI)
	if runtimeAPI == "" {
		// Not running under Lambda — exec the user entrypoint directly.
		runUserProcessStandalone()
		return
	}

	base := "http://" + runtimeAPI
	callbackURL := os.Getenv(envCallbackURL)
	containerID := os.Getenv(envContainerID)

	// Start the reverse-agent in the background if a callback URL is
	// configured. If the dial fails at init, post a /runtime/init/error
	// and exit non-zero so Lambda can recover.
	var rc *agent.ReverseAgentConn
	if callbackURL != "" && containerID != "" {
		var err error
		rc, err = dialReverseAgent(callbackURL, containerID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bootstrap: reverse-agent dial failed: %v\n", err)
			postInitError(base, err.Error())
			os.Exit(1)
		}
		go serveReverseAgent(rc)
		go sendHeartbeats(rc)
		defer func() { _ = rc.Close() }()
	}

	// Runtime-API polling loop.
	for {
		if err := handleOneInvocation(base); err != nil {
			fmt.Fprintf(os.Stderr, "bootstrap: invocation error: %v\n", err)
			// Runtime API errors on /next usually mean Lambda is
			// shutting us down; exit cleanly.
			return
		}
	}
}

// handleOneInvocation blocks on /next, spawns the user entrypoint with
// the invocation payload as stdin, and posts the result to /response or
// /error. The deadline header is enforced as a subprocess-level
// context timeout.
func handleOneInvocation(base string) error {
	resp, err := http.Get(base + runtimeAPIPath + "/next")
	if err != nil {
		return fmt.Errorf("GET /next: %w", err)
	}
	defer resp.Body.Close()

	requestID := resp.Header.Get("Lambda-Runtime-Aws-Request-Id")
	if requestID == "" {
		return fmt.Errorf("missing Lambda-Runtime-Aws-Request-Id header")
	}
	deadlineMs := resp.Header.Get("Lambda-Runtime-Deadline-Ms")

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read /next payload: %w", err)
	}

	ctx, cancel := contextWithDeadlineMs(deadlineMs)
	defer cancel()

	stdout, stderr, exitCode := runUserInvocation(ctx, payload)

	if exitCode != 0 {
		errPayload := buildErrorPayload(stderr, exitCode)
		return postResult(base, requestID, "/error", errPayload)
	}
	return postResult(base, requestID, "/response", stdout)
}

// runUserInvocation runs the user's declared entrypoint+cmd with the
// invocation payload piped on stdin. Captures stdout + stderr and
// returns the exit code. Cancelled by the deadline context.
func runUserInvocation(ctx context.Context, payload []byte) (stdout, stderr []byte, exitCode int) {
	argv := append(splitColon(os.Getenv(envUserEntrypoint)), splitColon(os.Getenv(envUserCmd))...)
	if len(argv) == 0 {
		// Nothing to run; echo the payload as the response (matches the
		// testdata handler semantics).
		return payload, nil, 0
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = os.Environ()
	cmd.Stdin = bytes.NewReader(payload)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return outBuf.Bytes(), errBuf.Bytes(), ee.ExitCode()
		}
		return outBuf.Bytes(), errBuf.Bytes(), 1
	}
	return outBuf.Bytes(), errBuf.Bytes(), 0
}

// postResult posts `body` to the Runtime API `/response` or `/error`
// endpoint for the given request ID.
func postResult(base, requestID, suffix string, body []byte) error {
	resp, err := http.Post(
		fmt.Sprintf("%s%s/%s%s", base, runtimeAPIPath, requestID, suffix),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("POST %s: %w", suffix, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

// buildErrorPayload wraps a non-zero exit into the Runtime API's
// documented error envelope.
func buildErrorPayload(stderr []byte, exitCode int) []byte {
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		msg = fmt.Sprintf("user process exited %d", exitCode)
	}
	body, _ := json.Marshal(map[string]any{
		"errorMessage": msg,
		"errorType":    "HandlerError",
	})
	return body
}

// postInitError sends a /runtime/init/error when we can't start the
// reverse-agent or otherwise fail at container init.
func postInitError(base, msg string) {
	body, _ := json.Marshal(map[string]any{
		"errorMessage": msg,
		"errorType":    "InitError",
	})
	resp, err := http.Post(
		base+runtimeInitError,
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
}

// dialReverseAgent opens the long-lived WebSocket to the sockerless
// Lambda backend and returns the wrapped ReverseAgentConn.
func dialReverseAgent(callbackURL, containerID string) (*agent.ReverseAgentConn, error) {
	u, err := url.Parse(callbackURL)
	if err != nil {
		return nil, fmt.Errorf("parse callback URL: %w", err)
	}
	q := u.Query()
	q.Set("session_id", containerID)
	u.RawQuery = q.Encode()
	ws, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", u.String(), err)
	}
	return agent.NewReverseAgentConn(ws), nil
}

// serveReverseAgent handles exec / attach frames arriving over the
// reverse-agent connection. The Message routing is already done by the
// ReverseAgentConn read-loop — we pick up session-scoped requests via
// the registered sessions map. Since a session registers as it arrives,
// we mainly need to make sure the conn lives until the process exits.
func serveReverseAgent(rc *agent.ReverseAgentConn) {
	<-rc.Done()
}

// sendHeartbeats pings the backend every heartbeatPeriod so it knows
// this session is alive. Exits when the connection closes.
func sendHeartbeats(rc *agent.ReverseAgentConn) {
	t := time.NewTicker(heartbeatPeriod)
	defer t.Stop()
	for {
		select {
		case <-rc.Done():
			return
		case <-t.C:
			_ = rc.SendJSON(agent.Message{Type: agent.TypeHealth})
		}
	}
}

// contextWithDeadlineMs returns a context that expires at the given
// millisecond epoch deadline, or a fallback no-deadline context if the
// header was missing.
func contextWithDeadlineMs(deadlineMs string) (context.Context, context.CancelFunc) {
	if deadlineMs == "" {
		return context.WithCancel(context.Background())
	}
	var epochMs int64
	if _, err := fmt.Sscanf(deadlineMs, "%d", &epochMs); err != nil {
		return context.WithCancel(context.Background())
	}
	return context.WithDeadline(context.Background(), time.UnixMilli(epochMs))
}

// runUserProcessStandalone execs the user's declared entrypoint + cmd
// with no Lambda framing. Used when the binary is run outside Lambda
// (local container smoke tests, image-inject integration tests).
func runUserProcessStandalone() {
	argv := append(splitColon(os.Getenv(envUserEntrypoint)), splitColon(os.Getenv(envUserCmd))...)
	if len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "sockerless-lambda-bootstrap: no user entrypoint/cmd configured")
		os.Exit(0)
	}
	bin, err := exec.LookPath(argv[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-lambda-bootstrap: exec %q: %v\n", argv[0], err)
		os.Exit(127)
	}
	if err := syscall.Exec(bin, argv, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-lambda-bootstrap: exec %q: %v\n", bin, err)
		os.Exit(126)
	}
}

func splitColon(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ":")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
