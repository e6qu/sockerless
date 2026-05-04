// sockerless-lambda-bootstrap is the entrypoint binary injected into
// Lambda container images by the sockerless Lambda backend. It wraps a
// user's entrypoint so the Lambda function can serve both (a) the
// user's declared workload and (b) a reverse-agent WebSocket that
// sockerless uses to proxy "docker exec" / "docker attach" into the
// running invocation.
//
// Architecture:
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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/sockerless/agent"
)

// Env vars the bootstrap consults. The argv lists arrive as
// base64(JSON) so every byte round-trips cleanly through
// `ENV KEY=VALUE` without Dockerfile or shell quoting.
const (
	envRuntimeAPI     = "AWS_LAMBDA_RUNTIME_API"
	envCallbackURL    = "SOCKERLESS_CALLBACK_URL"
	envContainerID    = "SOCKERLESS_CONTAINER_ID"
	envUserEntrypoint = "SOCKERLESS_USER_ENTRYPOINT"   // base64(JSON-encoded argv)
	envUserCmd        = "SOCKERLESS_USER_CMD"          // base64(JSON-encoded argv)
	envBindLinks      = "SOCKERLESS_LAMBDA_BIND_LINKS" // CSV of `<dst>=<mnt-target>` pairs
	envUserWorkdir    = "SOCKERLESS_USER_WORKDIR"      // chdir target for the user subprocess
	// envPodContainers carries the pod manifest: base64(JSON) of
	// []PodMember. When set, the bootstrap runs in supervisor mode:
	// non-main pod members start as long-lived sidecars at init; the
	// main member runs as the per-invocation foreground subprocess
	// driven by the Runtime API loop.
	envPodContainers = "SOCKERLESS_POD_CONTAINERS"
	// envPodMain identifies which pod member's argv replaces the
	// single-container SOCKERLESS_USER_ENTRYPOINT/CMD pair as the
	// foreground per-invocation subprocess. Defaults to the last
	// entry in the manifest if unset (CI runners start sidecars
	// first and the main step container last).
	envPodMain = "SOCKERLESS_POD_MAIN"
)

// PodMember describes one container inside a pod. The supervisor runs
// each member as a chroot'd child of the function's PID 1. Per
// specs/CLOUD_RESOURCE_MAPPING.md § "Podman pods on FaaS backends",
// mount-ns is degraded to chroot path-isolation and PID-ns is shared
// (Lambda execution environment doesn't grant CAP_SYS_ADMIN); spec
// documents this explicitly so operators can detect via `docker inspect`.
//
// Mirrors the gcf bootstrap's PodMember exactly so the wire shape is
// stable across the two FaaS backends.
type PodMember struct {
	Name        string   `json:"name"`
	Root        string   `json:"root"`
	Entrypoint  []string `json:"entrypoint,omitempty"`
	Cmd         []string `json:"cmd,omitempty"`
	Env         []string `json:"env,omitempty"`
	Workdir     string   `json:"workdir,omitempty"`
	ContainerID string   `json:"container_id,omitempty"` // unused in bootstrap; carried for backend round-trip
	Image       string   `json:"image,omitempty"`        // unused in bootstrap; carried for backend round-trip
}

const (
	runtimeAPIPath   = "/2018-06-01/runtime/invocation"
	runtimeInitError = "/2018-06-01/runtime/init/error"
	heartbeatPeriod  = 20 * time.Second
)

func main() {
	// Materialise bind-mount symlinks before anything else so the
	// reverse-agent and the user entrypoint both see the expected
	// container paths. Lambda enforces a single FileSystemConfig at
	// `/mnt/...`; sockerless's bind translation collapses Docker `-v`
	// targets into symlinks pointing at the shared mount. See
	// `specs/CLOUD_RESOURCE_MAPPING.md` § "Lambda bind-mount translation".
	if err := materialiseBindLinks(os.Getenv(envBindLinks)); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: materialise bind links: %v\n", err)
		os.Exit(1)
	}

	// Pod mode: when SOCKERLESS_POD_CONTAINERS is set the bootstrap
	// runs as a supervisor for the merged-rootfs overlay. Sidecars
	// (non-main pod members) start ONCE here at function-instance
	// init — Lambda has no per-request init like HTTP, so the
	// init-time start is the analogue of gcf's pre-warm. The main
	// member's argv replaces the standard SOCKERLESS_USER_*
	// per-invocation argv via getUserArgv() below.
	if pod, ok := parsePodManifest(); ok {
		printPodDegradationWarning()
		startPodSidecars(pod)
	}

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
	// configured. This uses the same router + session registry as the
	// standalone sockerless-agent, so TypeExec messages from the
	// Lambda backend spawn subprocesses inside this container and
	// stream stdout/stderr/exit back over the WebSocket.
	if callbackURL != "" && containerID != "" {
		conn, err := dialReverseAgent(callbackURL, containerID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bootstrap: reverse-agent dial failed: %v\n", err)
			postInitError(base, err.Error())
			os.Exit(1)
		}
		// Every goroutine that writes to conn must hold connMu —
		// gorilla/websocket requires serialised writes.
		connMu := &sync.Mutex{}
		go serveReverseAgent(conn, connMu)
		go sendHeartbeats(conn, connMu)
		defer func() { _ = conn.Close() }()
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
// error. The deadline header is enforced as a subprocess-level
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

// execEnvelope is the JSON shape sockerless's lambda backend uses to
// dispatch `docker exec` via `lambda.Invoke` when the reverse-agent
// path isn't wired (e.g. in-Lambda sockerless with no fronting API
// Gateway). Documented in `specs/CLOUD_RESOURCE_MAPPING.md` § Lambda
// exec semantics — Path B.
type execEnvelope struct {
	Sockerless struct {
		Exec *struct {
			Argv    []string `json:"argv"`
			Tty     bool     `json:"tty,omitempty"`
			Workdir string   `json:"workdir,omitempty"`
			Env     []string `json:"env,omitempty"`
			Stdin   string   `json:"stdin,omitempty"` // base64-encoded
		} `json:"exec,omitempty"`
	} `json:"sockerless,omitempty"`
}

// execResult is the JSON shape the bootstrap returns when the Invoke
// payload was an exec envelope. Matches `specs/CLOUD_RESOURCE_MAPPING.md`
// § Lambda exec semantics — Path B response shape.
type execResult struct {
	SockerlessExecResult struct {
		ExitCode int    `json:"exitCode"`
		Stdout   string `json:"stdout"` // base64-encoded
		Stderr   string `json:"stderr"` // base64-encoded
	} `json:"sockerlessExecResult"`
}

// runUserInvocation runs the user's declared entrypoint+cmd with the
// invocation payload piped on stdin. Captures stdout + stderr and
// returns the exit code. Cancelled by the deadline context.
//
// When the payload parses as an `execEnvelope` with a non-nil `exec`
// field, the bootstrap instead spawns that argv with the envelope's
// stdin/env/workdir and returns an `execResult` JSON response. This is
// how sockerless's lambda backend implements `docker exec` against a
// Lambda-deployed container without needing a long-lived inbound
// channel back to sockerless.
func runUserInvocation(ctx context.Context, payload []byte) (stdout, stderr []byte, exitCode int) {
	if env, ok := parseExecEnvelope(payload); ok {
		return runExecInvocation(ctx, env)
	}

	// Pod mode: when a pod manifest is configured, the main member's
	// argv (and chroot/workdir/env) replaces the single-container
	// SOCKERLESS_USER_* pair as the per-invocation foreground process.
	// Sidecars are already running from main()'s init-time pre-warm.
	if pod, ok := parsePodManifest(); ok {
		if main, found := pickPodMain(pod); found {
			return runPodMainInvocation(ctx, main, payload)
		}
	}

	argv := append(parseUserArgv(envUserEntrypoint), parseUserArgv(envUserCmd)...)
	if len(argv) == 0 {
		// Nothing to run; echo the payload as the response (matches the
		// testdata handler semantics).
		return payload, nil, 0
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = os.Environ()
	cmd.Stdin = bytes.NewReader(payload)
	if wd := os.Getenv(envUserWorkdir); wd != "" {
		cmd.Dir = wd
	}
	// Tee the subprocess's output into two places: the buffer that
	// becomes the /response body, and the bootstrap's own
	// stdout/stderr. The second destination is what the CONTAINER's
	// log driver sees — without it, Docker (and therefore CloudWatch
	// in the sim, or the backend's ContainerLogs in production) never
	// observes user-process output.
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(&outBuf, os.Stdout)
	cmd.Stderr = io.MultiWriter(&errBuf, os.Stderr)
	if err := cmd.Start(); err != nil {
		return nil, nil, 1
	}
	// Publish the user-process PID so reverse-agent pause/unpause can
	// SIGSTOP/SIGCONT it. The path is shared with backend-core via
	// the well-known mainPIDFilePath constant.
	writeMainPIDFile(cmd.Process.Pid)
	defer removeMainPIDFile()
	if err := cmd.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return outBuf.Bytes(), errBuf.Bytes(), ee.ExitCode()
		}
		return outBuf.Bytes(), errBuf.Bytes(), 1
	}
	return outBuf.Bytes(), errBuf.Bytes(), 0
}

// truncateBytes returns at most n bytes of b. Used to bound diagnostic
// log lines without dropping meaningful prefixes.
func truncateBytes(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}

// parseExecEnvelope decodes payload as an exec envelope. Returns
// (envelope, true) when the payload is valid JSON containing a
// non-nil sockerless.exec object; (zero, false) otherwise so callers
// fall through to the standard "user entrypoint" path.
func parseExecEnvelope(payload []byte) (execEnvelope, bool) {
	if len(payload) == 0 || payload[0] != '{' {
		return execEnvelope{}, false
	}
	var env execEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return execEnvelope{}, false
	}
	if env.Sockerless.Exec == nil || len(env.Sockerless.Exec.Argv) == 0 {
		return execEnvelope{}, false
	}
	return env, true
}

// runExecInvocation handles a Path B exec request. The argv runs as a
// subprocess with the envelope's stdin / env / workdir; the result is
// returned as a JSON `execResult` to the Lambda Runtime API so
// sockerless's lambda backend can decode + tunnel into the docker exec
// attach connection.
func runExecInvocation(ctx context.Context, env execEnvelope) (stdout, stderr []byte, exitCode int) {
	spec := env.Sockerless.Exec
	fmt.Fprintf(os.Stderr, "bootstrap exec: argv=%v workdir=%q env=%d-vars stdin=%d-bytes\n", spec.Argv, spec.Workdir, len(spec.Env), len(spec.Stdin))
	cmd := exec.CommandContext(ctx, spec.Argv[0], spec.Argv[1:]...)
	cmd.Env = os.Environ()
	if len(spec.Env) > 0 {
		// Append per-exec env atop the inherited environment — Docker's
		// exec semantics allow extra env vars without dropping the
		// container's defaults.
		cmd.Env = append(cmd.Env, spec.Env...)
	}
	if spec.Workdir != "" {
		cmd.Dir = spec.Workdir
	} else if wd := os.Getenv(envUserWorkdir); wd != "" {
		cmd.Dir = wd
	}
	if spec.Stdin != "" {
		stdinBytes, err := base64.StdEncoding.DecodeString(spec.Stdin)
		if err == nil {
			cmd.Stdin = bytes.NewReader(stdinBytes)
		}
	}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(&outBuf, os.Stdout)
	cmd.Stderr = io.MultiWriter(&errBuf, os.Stderr)

	code := 0
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
			fmt.Fprintf(os.Stderr, "bootstrap exec: exit %d (stderr=%q)\n", code, truncateBytes(errBuf.Bytes(), 256))
		} else {
			code = 1
			fmt.Fprintf(os.Stderr, "bootstrap exec: cmd.Run failed: %v\n", err)
		}
	}

	// Encode the result as JSON so the lambda backend's exec-attach
	// adapter can recover it from the `lambda.Invoke` response.
	res := execResult{}
	res.SockerlessExecResult.ExitCode = code
	res.SockerlessExecResult.Stdout = base64.StdEncoding.EncodeToString(outBuf.Bytes())
	res.SockerlessExecResult.Stderr = base64.StdEncoding.EncodeToString(errBuf.Bytes())
	body, _ := json.Marshal(res)
	// Lambda's invocation contract returns one body — we put the JSON
	// response in stdout (the /response payload), leave stderr empty,
	// and set the function-level exit code to 0 so Lambda doesn't
	// classify the invocation as a function-error. The actual exec exit
	// code travels inside the JSON body.
	return body, nil, 0
}

// mainPIDFilePath is the path the bootstrap writes the user-process
// PID to. Backend-core's RunContainerPauseViaAgent reads from this
// path to send SIGSTOP/SIGCONT.
const mainPIDFilePath = "/tmp/.sockerless-mainpid"

func writeMainPIDFile(pid int) {
	_ = os.WriteFile(mainPIDFilePath, []byte(fmt.Sprintf("%d", pid)), 0o644)
}

func removeMainPIDFile() {
	_ = os.Remove(mainPIDFilePath)
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
// Lambda backend. Returns a raw *websocket.Conn — the caller feeds it
// into the agent.Router via serveReverseAgent.
func dialReverseAgent(callbackURL, containerID string) (*websocket.Conn, error) {
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
	return ws, nil
}

// serveReverseAgent reads messages from the WebSocket and dispatches
// them to an agent.Router. This is the "server side" of the
// reverse-agent protocol — inbound TypeExec / TypeAttach messages
// spawn subprocesses in this container and stream stdout back over
// the WS.
func serveReverseAgent(conn *websocket.Conn, connMu *sync.Mutex) {
	logger := zerolog.New(os.Stderr).With().Str("component", "bootstrap-reverse-agent").Logger()
	registry := agent.NewSessionRegistry()
	router := agent.NewRouter(registry, nil, logger)
	defer registry.CleanupConn(conn)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg agent.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		router.Handle(&msg, conn, connMu)
	}
}

// sendHeartbeats writes a ping frame every heartbeatPeriod so the
// backend knows the container is alive between invocations. Exits
// when the WS is closed. connMu is shared with serveReverseAgent so
// pings can't interleave with response frames — gorilla/websocket
// requires serialised writes on a single conn.
func sendHeartbeats(conn *websocket.Conn, connMu *sync.Mutex) {
	t := time.NewTicker(heartbeatPeriod)
	defer t.Stop()
	for range t.C {
		connMu.Lock()
		err := conn.WriteMessage(websocket.PingMessage, nil)
		connMu.Unlock()
		if err != nil {
			return
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
	argv := append(parseUserArgv(envUserEntrypoint), parseUserArgv(envUserCmd)...)
	if len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "sockerless-lambda-bootstrap: no user entrypoint/cmd configured")
		os.Exit(0)
	}
	if wd := os.Getenv(envUserWorkdir); wd != "" {
		if err := os.Chdir(wd); err != nil {
			fmt.Fprintf(os.Stderr, "sockerless-lambda-bootstrap: chdir %q: %v\n", wd, err)
			os.Exit(126)
		}
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

// materialiseBindLinks creates the symlinks declared in
// `SOCKERLESS_LAMBDA_BIND_LINKS` so the user entrypoint sees Docker's
// `-v src:dst` semantics on top of Lambda's single-FileSystemConfig
// constraint. The env carries CSV of `<dst>=<mnt-target>` pairs; the
// Lambda backend emits these from `fileSystemConfigsForBinds`. See
// `specs/CLOUD_RESOURCE_MAPPING.md` § "Lambda bind-mount translation".
//
// Idempotent: re-running with the same env (Lambda execution-environment
// reuse across invocations) leaves the symlinks in their declared state.
// Existing files / directories at `dst` are removed and replaced with
// the symlink — Lambda's image filesystem doesn't carry user state, so
// any pre-existing entry was created by the bootstrap itself.
func materialiseBindLinks(spec string) error {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	for _, entry := range strings.Split(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		eq := strings.IndexByte(entry, '=')
		if eq < 0 {
			return fmt.Errorf("invalid bind link %q: expected <dst>=<target>", entry)
		}
		dst := strings.TrimSpace(entry[:eq])
		target := strings.TrimSpace(entry[eq+1:])
		if dst == "" || target == "" {
			return fmt.Errorf("invalid bind link %q: empty dst or target", entry)
		}
		if !filepath.IsAbs(dst) {
			return fmt.Errorf("bind link dst %q must be an absolute path", dst)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("mkdir parent of %s: %w", dst, err)
		}
		if cur, err := os.Readlink(dst); err == nil && cur == target {
			continue
		}
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("remove existing %s: %w", dst, err)
		}
		if err := os.Symlink(target, dst); err != nil {
			return fmt.Errorf("symlink %s → %s: %w", dst, target, err)
		}
	}
	return nil
}

// parseUserArgv returns the argv list the backend encoded into the
// given env var as base64(JSON). Empty / missing → nil.
func parseUserArgv(key string) []string {
	raw := os.Getenv(key)
	if raw == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}
	var out []string
	if err := json.Unmarshal(decoded, &out); err != nil {
		return nil
	}
	return out
}

// — Pod supervisor (mirror of gcf bootstrap) —

// parsePodManifest decodes SOCKERLESS_POD_CONTAINERS into a slice of
// PodMember. Returns (nil, false) when the env var is unset or
// undecodable so callers fall through to single-container mode.
func parsePodManifest() ([]PodMember, bool) {
	raw := os.Getenv(envPodContainers)
	if raw == "" {
		return nil, false
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: SOCKERLESS_POD_CONTAINERS base64 decode: %v\n", err)
		return nil, false
	}
	var out []PodMember
	if err := json.Unmarshal(decoded, &out); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: SOCKERLESS_POD_CONTAINERS JSON unmarshal: %v\n", err)
		return nil, false
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// pickPodMain returns the pod member that should run as the
// per-invocation foreground subprocess. Identified by SOCKERLESS_POD_MAIN
// (matched against PodMember.Name) or, when unset, the last entry in
// the manifest — gitlab-runner / github-runner start sidecars first
// and the main step container last, so the trailing entry is the
// natural default.
func pickPodMain(pod []PodMember) (PodMember, bool) {
	if len(pod) == 0 {
		return PodMember{}, false
	}
	if want := os.Getenv(envPodMain); want != "" {
		for _, m := range pod {
			if m.Name == want {
				return m, true
			}
		}
	}
	return pod[len(pod)-1], true
}

// printPodDegradationWarning writes the honest namespace-isolation
// disclaimer to stderr at startup, per spec § "Podman pods on FaaS
// backends — Why we don't fake the isolation".
func printPodDegradationWarning() {
	fmt.Fprintln(os.Stderr, "bootstrap: WARNING — pod uses degraded namespace isolation:")
	fmt.Fprintln(os.Stderr, "  mount-ns: shared (chroot only — would require CAP_SYS_ADMIN)")
	fmt.Fprintln(os.Stderr, "  pid-ns:   shared (would require CAP_SYS_ADMIN)")
	fmt.Fprintln(os.Stderr, "  net-ns:   shared per podman default")
	fmt.Fprintln(os.Stderr, "  ipc-ns:   shared per podman default")
	fmt.Fprintln(os.Stderr, "  uts-ns:   shared per podman default")
}

// startPodSidecars launches every non-main pod member as a long-lived
// background subprocess at function-instance init. Lambda's execution
// model means the function instance stays warm across consecutive
// invocations, so a single sidecar start lasts the lifetime of the
// instance — Lambda's own scale-down kills the supervisor + children
// together. No restart logic; if a sidecar dies its log line records
// the exit and the next invocation runs against a degraded pod.
func startPodSidecars(pod []PodMember) {
	main, _ := pickPodMain(pod)
	for _, m := range pod {
		if m.Name == main.Name {
			continue
		}
		go runPodSidecar(m)
	}
	// Brief settle window so postgres-style services bind their
	// localhost ports before the first invocation runs the main
	// member's argv. The main subprocess can do its own retry; this
	// just shaves the first-invoke latency.
	time.Sleep(250 * time.Millisecond)
}

// runPodSidecar launches a single pod member as a chroot'd child and
// pipes its stdout/stderr through the supervisor with a per-container
// `[<name>] ` line prefix. Cloud Logging captures peer output under
// the function's stream.
func runPodSidecar(m PodMember) {
	cmd, err := buildPodMemberCmd(m, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: pod sidecar %q: %v\n", m.Name, err)
		return
	}
	prefix := fmt.Sprintf("[%s] ", m.Name)
	cmd.Stdout = newPrefixWriter(os.Stdout, prefix)
	cmd.Stderr = newPrefixWriter(os.Stderr, prefix)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: pod sidecar %q start failed: %v\n", m.Name, err)
		return
	}
	fmt.Fprintf(os.Stderr, "bootstrap: pod sidecar %q started pid=%d\n", m.Name, cmd.Process.Pid)
	go func() {
		err := cmd.Wait()
		fmt.Fprintf(os.Stderr, "bootstrap: pod sidecar %q exited err=%v\n", m.Name, err)
	}()
}

// runPodMainInvocation runs the pod's main member as a per-invocation
// foreground subprocess with the Lambda invoke payload as stdin. The
// subprocess's stdout becomes the /response body; stderr is teed to
// the supervisor's stderr (CloudWatch).
func runPodMainInvocation(ctx context.Context, m PodMember, payload []byte) (stdout, stderr []byte, exitCode int) {
	cmd, err := buildPodMemberCmd(m, ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: pod main %q: %v\n", m.Name, err)
		return nil, []byte(err.Error()), 1
	}
	cmd.Stdin = bytes.NewReader(payload)
	prefix := fmt.Sprintf("[%s] ", m.Name)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(&outBuf, newPrefixWriter(os.Stdout, prefix))
	cmd.Stderr = io.MultiWriter(&errBuf, newPrefixWriter(os.Stderr, prefix))
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap: pod main %q start failed: %v\n", m.Name, err)
		return nil, []byte(err.Error()), 1
	}
	writeMainPIDFile(cmd.Process.Pid)
	defer removeMainPIDFile()
	if err := cmd.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return outBuf.Bytes(), errBuf.Bytes(), ee.ExitCode()
		}
		return outBuf.Bytes(), errBuf.Bytes(), 1
	}
	return outBuf.Bytes(), errBuf.Bytes(), 0
}

// buildPodMemberCmd assembles the *exec.Cmd that runs a pod member's
// entrypoint+cmd inside its chroot. Per spec § "Mount-ns approximation
// via chroot per child", chroot gives path-based isolation only — not
// a real mount-ns. Surfaced via `docker inspect ... HostConfig.PidMode`
// + Config.Labels["sockerless.namespace.*"] in the lambda backend's
// cloud_state path.
//
// ctx is optional (sidecars run unbounded; the main subprocess passes
// the Runtime API deadline context).
func buildPodMemberCmd(m PodMember, ctx context.Context) (*exec.Cmd, error) {
	argv := append([]string{}, m.Entrypoint...)
	argv = append(argv, m.Cmd...)
	if len(argv) == 0 {
		return nil, fmt.Errorf("member %q has no entrypoint or cmd", m.Name)
	}
	if m.Root == "" {
		return nil, fmt.Errorf("member %q has no chroot root", m.Name)
	}
	// Run via /bin/sh inside the chroot so PATH lookup matches the
	// container's filesystem. /bin/sh exists in all common base images
	// (alpine: /bin/sh → busybox; debian: /bin/sh → dash).
	shellLine := strings.Join(podMemberQuoteArgv(argv), " ")
	var cmd *exec.Cmd
	if ctx != nil {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", shellLine) //nolint:gosec // operator-controlled
	} else {
		cmd = exec.Command("/bin/sh", "-c", shellLine) //nolint:gosec // operator-controlled
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Chroot: m.Root,
	}
	if m.Workdir != "" {
		cmd.Dir = m.Workdir
	} else {
		cmd.Dir = "/"
	}
	if len(m.Env) > 0 {
		cmd.Env = append(append([]string{}, os.Environ()...), m.Env...)
	}
	return cmd, nil
}

// podMemberQuoteArgv single-quotes each argument so the result, joined
// by spaces, is safe to pass as one command line to /bin/sh -c. Shell
// metachars in the user's argv stay literal. Renamed from gcf's
// quoteArgv so the lambda bootstrap doesn't shadow any existing helper.
func podMemberQuoteArgv(argv []string) []string {
	out := make([]string, len(argv))
	for i, a := range argv {
		out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return out
}

// prefixWriter prefixes every line written through it with `prefix`.
// Used to label per-sidecar log output in the supervisor's combined
// stream so CloudWatch shows `[postgres] LOG: …` etc.
type prefixWriter struct {
	w        io.Writer
	prefix   string
	mu       sync.Mutex
	atLineSt bool
}

func newPrefixWriter(w io.Writer, prefix string) *prefixWriter {
	return &prefixWriter{w: w, prefix: prefix, atLineSt: true}
}

func (p *prefixWriter) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	written := 0
	for len(b) > 0 {
		if p.atLineSt {
			if _, err := p.w.Write([]byte(p.prefix)); err != nil {
				return written, err
			}
			p.atLineSt = false
		}
		nl := bytes.IndexByte(b, '\n')
		if nl < 0 {
			n, err := p.w.Write(b)
			written += n
			return written, err
		}
		n, err := p.w.Write(b[:nl+1])
		written += n
		if err != nil {
			return written, err
		}
		b = b[nl+1:]
		p.atLineSt = true
	}
	return written, nil
}
