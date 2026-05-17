// sockerless-gcf-bootstrap is the entrypoint binary injected into
// Cloud Run Functions container images by the sockerless gcf backend.
// It serves an HTTP server on $PORT (Cloud Functions Gen2 sets this)
// and, on every incoming request, runs the user's declared
// entrypoint+cmd as a subprocess, captures stdout+stderr, and returns
// the output as the HTTP response body.
//
// Architecture note: Cloud Functions Gen2 wraps a Cloud Run service.
// Sockerless invokes the function via HTTPS POST to ServiceConfig.Uri;
// this bootstrap turns that single invocation into a one-shot
// subprocess execution of the user's image's CMD/ENTRYPOINT — making
// arbitrary images (alpine, busybox, etc.) usable as functions
// without the Functions Framework. See `feedback_faas_container_mode.md`.
//
// Pod mode: when SOCKERLESS_POD_CONTAINERS is set, the bootstrap acts
// as a supervisor (PID 1) for the merged-rootfs overlay. It forks one
// chroot'd subprocess per pod member; non-main members run in the
// background for the lifetime of the invocation, the main member runs
// in the foreground and its stdout becomes the HTTP response body.
// See specs/CLOUD_RESOURCE_MAPPING.md § "Podman pods on FaaS backends".
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sockerless/agent"
)

// execEnvelopeRequest is the Path B exec request shape (see
// specs/CLOUD_RESOURCE_MAPPING.md § Lesson 8). Identical to the
// cloudrun + lambda bootstrap envelope so the wire format is
// cross-cloud consistent. When handleInvoke sees a request body
// matching this shape, it runs envelope.argv instead of the env-baked
// SOCKERLESS_USER_* cmd.
type execEnvelopeRequest struct {
	Sockerless struct {
		Exec execEnvelopeExec `json:"exec"`
	} `json:"sockerless"`
}

type execEnvelopeExec struct {
	Argv    []string `json:"argv"`
	Tty     bool     `json:"tty,omitempty"`
	Workdir string   `json:"workdir,omitempty"`
	Env     []string `json:"env,omitempty"`
	Stdin   string   `json:"stdin,omitempty"` // base64
}

type execEnvelopeResponse struct {
	SockerlessExecResult struct {
		ExitCode int    `json:"exitCode"`
		Stdout   string `json:"stdout"` // base64
		Stderr   string `json:"stderr"` // base64
	} `json:"sockerlessExecResult"`
}

// quoteArgv returns each argument single-quoted (with embedded single
// quotes escaped) so the result, joined by spaces, is safe to pass as a
// single command line to /bin/sh -c. Mirrors the standard shell-quoting
// idiom used in scripted command construction.
func quoteArgv(argv []string) []string {
	out := make([]string, len(argv))
	for i, a := range argv {
		out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return out
}

// Env vars the bootstrap consults. Same shape as the Lambda bootstrap
// for cross-cloud consistency: argv lists are base64(JSON) so every
// byte round-trips cleanly through `ENV KEY=VALUE`.
const (
	envPort           = "PORT"
	envUserEntrypoint = "SOCKERLESS_USER_ENTRYPOINT" // base64(JSON-encoded argv)
	envUserCmd        = "SOCKERLESS_USER_CMD"        // base64(JSON-encoded argv)
	envUserWorkdir    = "SOCKERLESS_USER_WORKDIR"    // chdir target (optional)
	// envPodContainers carries the pod manifest: base64(JSON) of
	// []PodMember. When set, the bootstrap runs in supervisor mode.
	envPodContainers = "SOCKERLESS_POD_CONTAINERS"
	// envPodMain identifies which pod member's stdout becomes the HTTP
	// response body. Other members run in the background. Defaults to
	// the last entry in the manifest if unset.
	envPodMain = "SOCKERLESS_POD_MAIN"
	// SOCKERLESS_HOST_ALIASES = comma-separated list of hostnames that
	// should resolve to 127.0.0.1 (sidecar containers in the same Cloud
	// Run Service revision share loopback). Sourced by the gcf backend
	// from the standard Docker NetworkingConfig.EndpointsConfig.Aliases
	// of every sibling container in the same docker user-defined network.
	envHostAliases = "SOCKERLESS_HOST_ALIASES"
	// SOCKERLESS_DNS_SEARCH_DOMAIN = single DNS suffix appended to the
	// `search` line of /etc/resolv.conf at bootstrap startup. Empty →
	// no /etc/resolv.conf write. Lets short hostnames in the same
	// network resolve to per-name A records the network-discovery driver
	// registers under the same suffix.
	envDNSSearchDomain = "SOCKERLESS_DNS_SEARCH_DOMAIN"
	// SOCKERLESS_SIDECAR=1 marks this container as a non-ingress sidecar
	// in a Cloud Run multi-container revision. Sidecars do NOT bind the
	// PORT HTTP server (only one container per revision can — the
	// ingress one). Instead the bootstrap just exec's the user CMD as a
	// foreground subprocess so the sidecar's process is what keeps the
	// container alive (e.g. postgres). /etc/hosts injection still runs
	// for sidecars so they can resolve sibling aliases too.
	envSidecar = "SOCKERLESS_SIDECAR"
	// SOCKERLESS_JOB_TIMEOUT_SECONDS sets the hard cap on a single
	// workload subprocess (sidecar/default-invoke mode) or a single
	// exec-envelope call. Default: 3600 (1 h). At timeout: SIGTERM →
	// 30s grace → SIGKILL; bootstrap reports exit code 124.
	envJobTimeoutSeconds = "SOCKERLESS_JOB_TIMEOUT_SECONDS"
	// SOCKERLESS_CALLBACK_URL — reverse-agent WebSocket URL (required
	// per Phase 168; backend's ExecStart fails loud without an agent).
	envCallbackURL = "SOCKERLESS_CALLBACK_URL"
	// SOCKERLESS_CONTAINER_ID — session_id for the reverse-agent WS.
	envContainerID = "SOCKERLESS_CONTAINER_ID"
)

const (
	jobTimeoutDefaultSeconds = 3600
	jobTimeoutGracePeriod    = 30
	jobTimeoutExitCode       = 124
)

// PodMember describes one container inside a pod. The supervisor runs
// each member as a chroot'd child of the function's PID 1. Per
// specs/CLOUD_RESOURCE_MAPPING.md § "Podman pods on FaaS backends",
// mount-ns isolation is degraded to chroot path-isolation and PID-ns
// is shared; the spec documents this explicitly so operators can detect
// via `docker inspect`.
type PodMember struct {
	Name       string   `json:"name"`
	Root       string   `json:"root"`                 // absolute path inside the merged rootfs ("/containers/<name>")
	Entrypoint []string `json:"entrypoint,omitempty"` // image's ENTRYPOINT or user override
	Cmd        []string `json:"cmd,omitempty"`        // image's CMD or user override
	Env        []string `json:"env,omitempty"`        // additional env (KEY=VALUE)
	Workdir    string   `json:"workdir,omitempty"`
}

// invokeMu serializes invocations. Sockerless's docker-run semantics
// are one-shot per container; concurrent invocations would duplicate
// output across the same container's stdout. Serialising at this
// layer keeps the single-container guarantee even if Cloud Functions'
// MaxInstanceRequestConcurrency is bumped above 1.
var invokeMu sync.Mutex

// persistVols holds the parsed SOCKERLESS_PERSIST_VOLUMES config so
// every handleInvoke can run saveAll without re-parsing. Initialized
// once at startup; nil-or-empty means persistence is disabled.
// Ports the cloudrun bootstrap's persist module to gcf so bind volumes
// carry across the multi-pod-Service stage transitions gitlab-runner
// v17 docker executor produces with multi-image-per-job.
var persistVols []persistVolume

// syncMounts holds the parsed SOCKERLESS_SYNC_MOUNTS — `volumeName ->
// mountPath` set by the JOB pod-Service materializer. Used at exec
// time to resolve where each per-exec gcs-sync object should restore.
var syncMounts map[string]string

func main() {
	// Same shape as cloudrun bootstrap — emit to BOTH stdout and stderr
	// at the very top of main() so Cloud Logging captures the binary's
	// first instruction even if one stream is lost. Sidecar mode
	// triggers below; the second log line proves the post-sidecar-check
	// execution path.
	fmt.Fprintf(os.Stdout, "sockerless-gcf-bootstrap: MAIN ENTRY pid=%d args=%v PORT=%q SOCKERLESS_SIDECAR=%q\n",
		os.Getpid(), os.Args, os.Getenv("PORT"), os.Getenv(envSidecar))
	fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: MAIN ENTRY pid=%d args=%v PORT=%q SOCKERLESS_SIDECAR=%q\n",
		os.Getpid(), os.Args, os.Getenv("PORT"), os.Getenv(envSidecar))

	if err := writeHostAliases(os.Getenv(envHostAliases)); err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: write host aliases: %v\n", err)
	}
	if err := writeDNSSearchDomain(os.Getenv(envDNSSearchDomain)); err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: write dns search domain: %v\n", err)
	}

	// Sidecar mode: skip the HTTP server (only the ingress container
	// binds PORT in a Cloud Run multi-container revision) and just
	// exec the user CMD as a foreground subprocess. Mirrors the
	// cloudrun bootstrap's sidecar handling — same shape so a
	// multi-container Cloud Run Service revision behaves identically
	// regardless of which backend deployed it.
	if os.Getenv(envSidecar) != "" {
		fmt.Fprintln(os.Stderr, "sockerless-gcf-bootstrap: sidecar mode — exec user CMD")
		runSidecar()
		return
	}

	persistVols = parsePersistVolumes(os.Getenv(envPersistVolumes))
	if len(persistVols) > 0 {
		if err := restoreAll(context.Background(), persistVols); err != nil {
			fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: persist restore failed: %v\n", err)
			os.Exit(1)
		}
	}

	// Parse SOCKERLESS_SYNC_MOUNTS once at startup.
	mounts, syncErr := parseSyncMounts(os.Getenv(envSyncMounts))
	if syncErr != nil {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: SOCKERLESS_SYNC_MOUNTS parse error: %v\n", syncErr)
		os.Exit(1)
	}
	syncMounts = mounts
	if len(syncMounts) > 0 {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: parsed %d sync mounts: %v\n", len(syncMounts), syncMounts)
	}

	port := os.Getenv(envPort)
	if port == "" {
		port = "8080"
	}

	// Reverse-agent dial-back (Phase 168). Required for `docker exec`
	// from the backend.
	callbackURL := os.Getenv(envCallbackURL)
	containerID := os.Getenv(envContainerID)
	if callbackURL != "" && containerID != "" {
		conn, err := agent.DialReverseAgent(callbackURL, containerID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: reverse-agent dial failed: %v\n", err)
			os.Exit(1)
		}
		connMu := &sync.Mutex{}
		go agent.ServeReverseAgent(conn, connMu)
		go agent.StartHeartbeats(conn, connMu)
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: reverse-agent connected to %s (session=%s)\n", callbackURL, containerID)
	} else {
		fmt.Fprintln(os.Stderr, "sockerless-gcf-bootstrap: SOCKERLESS_CALLBACK_URL or SOCKERLESS_CONTAINER_ID empty — reverse-agent disabled")
	}

	// Pod mode: pre-warm the supervisor so background members start
	// once per function instance, not once per invocation. The HTTP
	// handler then runs the main member's entrypoint as a foreground
	// subprocess on each request.
	if pod, ok := parsePodManifest(); ok {
		printPodDegradationWarning()
		startPodSidecars(pod)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleInvoke)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: listening on :%s\n", port)
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: server exited: %v\n", err)
		os.Exit(1)
	}
}

// handleInvoke runs the user's entrypoint+cmd as a subprocess and writes
// the combined stdout+stderr to both the response body and the function's
// own stdout/stderr (so Cloud Logging captures it under the run.googleapis.com
// stdout/stderr log names — that's how the gcf backend's
// buildCloudLogsFetcher surfaces them through `docker logs`).
//
// Status: 200 on subprocess exit 0; 500 on non-zero. Exit code rides
// in the `X-Sockerless-Exit-Code` header for the gcf backend's
// containers.go to decode (it currently maps via core.HTTPStatusToExitCode).
//
// Pod mode: the main pod member runs as the foreground subprocess.
// Non-main members are already running as long-lived sidecars (started
// by main()). Sidecar stdout/stderr is teed to the supervisor's
// stdout with a `[<name>]` line prefix so Cloud Logging captures it
// under the function's log stream.
func handleInvoke(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: handleInvoke ENTRY method=%s path=%s remote=%s contentLength=%d\n", r.Method, r.URL.Path, r.RemoteAddr, r.ContentLength)
	invokeMu.Lock()
	defer invokeMu.Unlock()

	body, _ := io.ReadAll(r.Body)
	defer r.Body.Close()
	fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: handleInvoke body_bytes=%d\n", len(body))

	// Buffer the response so saveAll can run after the subprocess
	// completes but before the wire response goes out. A save failure
	// replaces the buffered response with an exit-code=1 shape — silent
	// data loss between stages would surface as confusing build errors.
	buf := newBufferedResponse()
	env, isEnvelope := parseExecEnvelope(body)
	fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: handleInvoke isEnvelope=%t argv_count=%d\n", isEnvelope, len(env.Argv))

	// Per-exec gcs-sync restore. Hints come via envelope.Env (the
	// default-invoke + pod paths don't carry per-exec sync triples —
	// only ExecStart sets them). Parse + restore before running the
	// subprocess so the workspace reflects whatever the runner-task
	// uploaded for this exec.
	syncVols, syncErr := parseSyncVolumes(extractSyncVolumesEnv(env.Env), syncMounts)
	if syncErr != nil {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: SOCKERLESS_SYNC_VOLUMES parse error: %v\n", syncErr)
		writeSaveFailure(w, syncErr, isEnvelope)
		return
	}
	if len(syncVols) > 0 {
		if err := restoreSyncAll(context.Background(), syncVols); err != nil {
			fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: sync restore failed: %v\n", err)
			writeSaveFailure(w, err, isEnvelope)
			return
		}
	}

	switch {
	case isEnvelope:
		runExecEnvelope(buf, env)
	case hasPodManifest():
		runPodInvoke(buf)
	default:
		runDefaultInvoke(buf)
	}

	if len(persistVols) > 0 {
		if err := saveAll(context.Background(), persistVols); err != nil {
			fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: persist save failed: %v\n", err)
			writeSaveFailure(w, err, isEnvelope)
			return
		}
	}
	if len(syncVols) > 0 {
		if err := saveSyncAll(context.Background(), syncVols); err != nil {
			fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: sync save failed: %v\n", err)
			writeSaveFailure(w, err, isEnvelope)
			return
		}
	}
	buf.flushTo(w)
}

// extractSyncVolumesEnv finds the SOCKERLESS_SYNC_VOLUMES entry in the
// envelope's Env slice (KEY=value strings). Returns "" if absent.
func extractSyncVolumesEnv(env []string) string {
	prefix := envSyncVolumes + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return e[len(prefix):]
		}
	}
	return ""
}

// hasPodManifest mirrors parsePodManifest()'s presence check without
// re-parsing — used to route handleInvoke between pod, envelope, and
// default-invoke paths.
func hasPodManifest() bool {
	_, ok := parsePodManifest()
	return ok
}

// runPodInvoke runs the pod main member as a foreground subprocess.
// Wraps runPodMain so handleInvoke's switch can route to it without
// duplicating the pickPodMain failure-handling boilerplate.
func runPodInvoke(w http.ResponseWriter) {
	pod, _ := parsePodManifest()
	main, found := pickPodMain(pod)
	if !found {
		writeBootstrapFailure(w, "no main container in pod manifest")
		return
	}
	runPodMain(w, main)
}

// runDefaultInvoke runs the env-baked SOCKERLESS_USER_* command via
// `/bin/sh -c` and writes the response (header + body) to w. Pulled
// out of handleInvoke so the buffered-response wrapping can sit in
// one place.
func runDefaultInvoke(w http.ResponseWriter) {
	argv := append(parseUserArgv(envUserEntrypoint), parseUserArgv(envUserCmd)...)
	if len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "sockerless-gcf-bootstrap: no user entrypoint/cmd configured")
		writeBootstrapFailure(w, "no entrypoint configured")
		return
	}

	// Resolve argv[0] via /bin/sh's PATH lookup. Go's exec.LookPath has been
	// observed to return /usr/bin/echo on alpine even when only /bin/echo
	// exists (kernel ENOENT at fork/exec). Routing through `/bin/sh -c` lets
	// busybox-shell resolve the binary correctly across alpine/distro variants.
	shellLine := strings.Join(quoteArgv(argv), " ")
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("/bin/sh", "-c", shellLine) //nolint:gosec // argv operator-controlled
	cmd.Stdout = io.MultiWriter(&stdout, os.Stdout)
	cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)
	if wd := os.Getenv(envUserWorkdir); wd != "" {
		cmd.Dir = wd
	}

	timeout := jobTimeoutFromEnv()
	exitCode, timedOut, err := runWithTimeout(cmd, timeout, "default-invoke")
	if timedOut {
		exitCode = jobTimeoutExitCode
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: subprocess argv=%v timed out after %ds; exit=%d\n",
			argv, timeout, exitCode)
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: subprocess argv=%v exit=%d err=%v\n", argv, exitCode, err)
	} else {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: subprocess argv=%v exit=0 stdout=%dB\n", argv, stdout.Len())
	}

	// Always 200; the cloudrun/gcf backend reads X-Sockerless-Exit-Code
	// for failure signaling. HTTP 5xx is reserved for unexpected panics
	// (no designated 5xx contract here — Docker's exec API maps to the
	// header pattern we already use).
	w.Header().Set("X-Sockerless-Exit-Code", strconv.Itoa(exitCode))
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(stdout.Bytes())
	if exitCode != 0 {
		_, _ = w.Write(stderr.Bytes())
	}
}

// bufferedResponse captures handler output so saveAll can run before
// the wire response is committed. Implements http.ResponseWriter.
type bufferedResponse struct {
	headers http.Header
	code    int
	body    bytes.Buffer
}

func newBufferedResponse() *bufferedResponse {
	return &bufferedResponse{headers: http.Header{}, code: http.StatusOK}
}

func (b *bufferedResponse) Header() http.Header         { return b.headers }
func (b *bufferedResponse) WriteHeader(code int)        { b.code = code }
func (b *bufferedResponse) Write(p []byte) (int, error) { return b.body.Write(p) }

func (b *bufferedResponse) flushTo(w http.ResponseWriter) {
	for k, v := range b.headers {
		w.Header()[k] = v
	}
	w.WriteHeader(b.code)
	_, _ = w.Write(b.body.Bytes())
}

// writeSaveFailure overwrites the buffered exec response with a
// failure shape matching the path that ran. envelope=true → JSON
// envelope with exitCode=1 + stderr carrying the error; envelope=false
// → text body with X-Sockerless-Exit-Code=1 header. HTTP status stays
// 200 in both cases so the backend's ExitCode parsing (envelope JSON
// or X-Sockerless-Exit-Code header) is the single source of truth.
func writeSaveFailure(w http.ResponseWriter, err error, envelope bool) {
	msg := "sockerless-gcf-bootstrap: persist save: " + err.Error()
	if envelope {
		var res execEnvelopeResponse
		res.SockerlessExecResult.ExitCode = 1
		res.SockerlessExecResult.Stderr = base64.StdEncoding.EncodeToString([]byte(msg))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Sockerless-Exit-Code", "1")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(res)
		return
	}
	w.Header().Set("X-Sockerless-Exit-Code", "1")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(msg))
}

// writeBootstrapFailure reports an operator-misconfiguration as a
// docker-style failed exec (200 + X-Sockerless-Exit-Code=1 + body).
// 5xx is reserved for unexpected panics.
func writeBootstrapFailure(w http.ResponseWriter, msg string) {
	w.Header().Set("X-Sockerless-Exit-Code", "1")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("sockerless-gcf-bootstrap: " + msg + "\n"))
}

// parseExecEnvelope returns the parsed envelope when the body is a
// well-formed JSON object with sockerless.exec.argv non-empty. Anything
// else (empty body, raw bytes, JSON without the envelope shape) → false
// so the default invoke path runs. Same shape as
// sockerless-cloudrun-bootstrap + sockerless-lambda-bootstrap.
func parseExecEnvelope(body []byte) (execEnvelopeExec, bool) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '{' {
		return execEnvelopeExec{}, false
	}
	var req execEnvelopeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return execEnvelopeExec{}, false
	}
	if len(req.Sockerless.Exec.Argv) == 0 {
		return execEnvelopeExec{}, false
	}
	return req.Sockerless.Exec, true
}

// runExecEnvelope runs envelope.argv with envelope.stdin piped to the
// subprocess and returns {exitCode, stdout, stderr} (base64) in the
// response body. Used by the gcf backend's Path B docker exec.
func runExecEnvelope(w http.ResponseWriter, env execEnvelopeExec) {
	shellLine := strings.Join(quoteArgv(env.Argv), " ")
	cmd := exec.Command("/bin/sh", "-c", shellLine) //nolint:gosec // argv operator-controlled
	if env.Workdir != "" {
		cmd.Dir = env.Workdir
	}
	if len(env.Env) > 0 {
		cmd.Env = append(append([]string{}, os.Environ()...), env.Env...)
	}
	if env.Stdin != "" {
		stdinBytes, err := base64.StdEncoding.DecodeString(env.Stdin)
		if err != nil {
			http.Error(w, "stdin base64 decode: "+err.Error(), http.StatusBadRequest)
			return
		}
		cmd.Stdin = bytes.NewReader(stdinBytes)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = io.MultiWriter(&stdout, os.Stdout)
	cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

	timeout := jobTimeoutFromEnv()
	exitCode, timedOut, err := runWithTimeout(cmd, timeout, "exec")
	if timedOut {
		exitCode = jobTimeoutExitCode
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: exec argv=%v timed out after %ds; exit=%d\n",
			env.Argv, timeout, exitCode)
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: exec argv=%v exit=%d err=%v\n", env.Argv, exitCode, err)
	} else {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: exec argv=%v exit=0 stdout=%dB stderr=%dB\n", env.Argv, stdout.Len(), stderr.Len())
	}

	stderrBytes := stderr.Bytes()
	// ENOSPC override only when the subprocess actually failed
	// (BUG-1062). Otherwise a successful command that mentions
	// the marker on stderr gets force-coerced to 28.
	if exitCode != 0 && agent.DetectENOSPC(stderrBytes) {
		exitCode = agent.ENOSPCExitCode
		stderrBytes = agent.AnnotateENOSPC(stderrBytes, "gcf")
	}

	var res execEnvelopeResponse
	res.SockerlessExecResult.ExitCode = exitCode
	res.SockerlessExecResult.Stdout = base64.StdEncoding.EncodeToString(stdout.Bytes())
	res.SockerlessExecResult.Stderr = base64.StdEncoding.EncodeToString(stderrBytes)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Sockerless-Exit-Code", strconv.Itoa(exitCode))
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}

// parseUserArgv returns the argv list the backend encoded into the
// given env var as base64(JSON). Empty / missing → nil. Mirrors the
// Lambda bootstrap's parseUserArgv exactly so encoding stays cross-cloud
// consistent.
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

// — Pod supervisor —

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
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: SOCKERLESS_POD_CONTAINERS base64 decode: %v\n", err)
		return nil, false
	}
	var out []PodMember
	if err := json.Unmarshal(decoded, &out); err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: SOCKERLESS_POD_CONTAINERS JSON unmarshal: %v\n", err)
		return nil, false
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// pickPodMain returns the pod member that should run as the foreground
// subprocess on each invocation. Identified by SOCKERLESS_POD_MAIN
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
// backends — Why we don't fake the isolation". Operators reading
// `docker logs <pod>` see the trade-off they're accepting.
func printPodDegradationWarning() {
	fmt.Fprintln(os.Stderr, "sockerless-gcf-bootstrap: WARNING — pod uses degraded namespace isolation:")
	fmt.Fprintln(os.Stderr, "  mount-ns: shared (chroot only — would require CAP_SYS_ADMIN)")
	fmt.Fprintln(os.Stderr, "  pid-ns:   shared (would require CAP_SYS_ADMIN)")
	fmt.Fprintln(os.Stderr, "  net-ns:   shared per podman default")
	fmt.Fprintln(os.Stderr, "  ipc-ns:   shared per podman default")
	fmt.Fprintln(os.Stderr, "  uts-ns:   shared per podman default")
}

// startPodSidecars launches every non-main pod member as a long-lived
// background subprocess. Per spec, sidecars share net+IPC+UTS with
// each other and with the main member (single Linux container);
// mount-ns is approximated via chroot to the merged-rootfs subdir;
// PID-ns is shared. The supervisor (this process) tees per-sidecar
// stdout to the function's stdout with a `[<name>]` prefix so Cloud
// Logging captures peer output under one log stream.
//
// Sidecars are not restarted; if one exits, its log line records the
// exit and the others keep running until the function instance is
// scaled down.
func startPodSidecars(pod []PodMember) {
	main, _ := pickPodMain(pod)
	for _, m := range pod {
		if m.Name == main.Name {
			continue
		}
		go runPodSidecar(m)
	}
	// Give sidecars ~250ms to enter their accept loops (postgres-style
	// services binding to localhost:PORT). This is deliberately short:
	// the main subprocess will retry localhost connect itself; the
	// sleep just shaves the first-invoke latency where the main
	// connects before the sidecar's listen() returns.
	time.Sleep(250 * time.Millisecond)
}

// runPodSidecar launches a single pod member as a chroot'd child and
// pipes its stdout/stderr through the supervisor with a per-container
// line prefix.
func runPodSidecar(m PodMember) {
	cmd, err := buildPodMemberCmd(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: pod sidecar %q: %v\n", m.Name, err)
		return
	}
	prefix := fmt.Sprintf("[%s] ", m.Name)
	cmd.Stdout = newPrefixWriter(os.Stdout, prefix)
	cmd.Stderr = newPrefixWriter(os.Stderr, prefix)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: pod sidecar %q start failed: %v\n", m.Name, err)
		return
	}
	fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: pod sidecar %q started pid=%d\n", m.Name, cmd.Process.Pid)
	// Wait in a goroutine so we surface exit even though we never
	// reap; the kernel will eventually clean up when the supervisor
	// itself exits at function-instance shutdown.
	go func() {
		err := cmd.Wait()
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: pod sidecar %q exited err=%v\n", m.Name, err)
	}()
}

// runPodMain runs the main pod member as a foreground subprocess and
// writes its stdout to the HTTP response body. Stderr is teed to the
// supervisor's stderr (Cloud Logging) but does NOT enter the response
// — matches the single-container path's `Content-Type: text/plain`
// stdout-only response shape.
func runPodMain(w http.ResponseWriter, m PodMember) {
	cmd, err := buildPodMemberCmd(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: pod main %q: %v\n", m.Name, err)
		writeBootstrapFailure(w, fmt.Sprintf("pod main %q: %v", m.Name, err))
		return
	}
	prefix := fmt.Sprintf("[%s] ", m.Name)
	var stdout bytes.Buffer
	cmd.Stdout = io.MultiWriter(&stdout, newPrefixWriter(os.Stdout, prefix))
	cmd.Stderr = newPrefixWriter(os.Stderr, prefix)
	exitCode := 0
	if err := cmd.Run(); err != nil {
		exitCode = 1
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: pod main %q exit=%d err=%v\n", m.Name, exitCode, err)
	} else {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: pod main %q exit=0 stdout=%dB\n", m.Name, stdout.Len())
	}
	w.Header().Set("X-Sockerless-Exit-Code", strconv.Itoa(exitCode))
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(stdout.Bytes())
}

// buildPodMemberCmd assembles the *exec.Cmd that runs a pod member's
// entrypoint+cmd inside its chroot. The chroot is applied via
// SysProcAttr.Chroot so the child only sees the merged rootfs subdir
// (`/containers/<name>` in the supervisor's view).
//
// The chroot is the mount-ns approximation — see spec §
// "Mount-ns approximation via chroot per child": path-based isolation
// only, not a real mount-ns. We surface this honestly via
// `docker inspect.HostConfig.MountNamespaceMode = "shared-degraded"`.
func buildPodMemberCmd(m PodMember) (*exec.Cmd, error) {
	argv := append([]string{}, m.Entrypoint...)
	argv = append(argv, m.Cmd...)
	if len(argv) == 0 {
		return nil, fmt.Errorf("member %q has no entrypoint or cmd", m.Name)
	}
	if m.Root == "" {
		return nil, fmt.Errorf("member %q has no chroot root", m.Name)
	}
	// Run via /bin/sh inside the chroot so PATH lookup matches the
	// container's filesystem. The shell binary itself must exist at
	// /bin/sh inside the chroot — true for all common base images
	// (alpine: /bin/sh → busybox; debian: /bin/sh → dash).
	shellLine := strings.Join(quoteArgv(argv), " ")
	cmd := exec.Command("/bin/sh", "-c", shellLine) //nolint:gosec // operator-controlled
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

// prefixWriter prefixes every line written through it with `prefix`.
// Used to label per-sidecar log output in the supervisor's combined
// stream so Cloud Logging shows `[postgres] LOG: …` etc.
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

// writeHostAliases appends `127.0.0.1 <alias>` lines to /etc/hosts for
// each comma-separated alias in `raw`. This is how the gcf backend
// makes Docker-network DNS aliases resolve to loopback when sibling
// containers are deployed as Cloud Run multi-container sidecars (sharing
// the same loopback). Empty raw → no-op. Mirrors the cloudrun bootstrap.
func writeHostAliases(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var aliases []string
	for _, a := range strings.Split(raw, ",") {
		if a = strings.TrimSpace(a); a != "" {
			aliases = append(aliases, a)
		}
	}
	if len(aliases) == 0 {
		return nil
	}
	f, err := os.OpenFile("/etc/hosts", os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	line := "127.0.0.1\t" + strings.Join(aliases, " ") + "\n"
	if _, err := f.WriteString(line); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: /etc/hosts += %q\n", line)
	return nil
}

// writeDNSSearchDomain appends the given suffix to the `search` line of
// /etc/resolv.conf so short-name lookups within the network resolve. If
// the file already has a `search` line we extend it with the suffix; if
// not we add a fresh `search <suffix>` line. Empty raw → no-op.
func writeDNSSearchDomain(raw string) error {
	suffix := strings.TrimSpace(raw)
	if suffix == "" {
		return nil
	}
	const path = "/etc/resolv.conf"
	body, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := strings.Split(string(body), "\n")
	updated := false
	for i, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "search" {
			for _, f := range fields[1:] {
				if f == suffix {
					return nil
				}
			}
			lines[i] = strings.TrimRight(line, " \t") + " " + suffix
			updated = true
			break
		}
	}
	if !updated {
		lines = append(lines, "search "+suffix)
	}
	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: /etc/resolv.conf search += %q\n", suffix)
	return nil
}

// runSidecar exec's the env-baked SOCKERLESS_USER_* command as a
// foreground subprocess and waits for it to exit. Used in Cloud Run
// multi-container revisions where this container is NOT the ingress
// (only the ingress binds PORT). The sidecar's process (e.g.
// postgres) is what keeps the container alive — its TCP port (5432
// etc.) is reachable from the ingress container via shared loopback.
func runSidecar() {
	argv := append(parseUserArgv(envUserEntrypoint), parseUserArgv(envUserCmd)...)
	if len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "sockerless-gcf-bootstrap: sidecar has no user entrypoint/cmd configured")
		os.Exit(1)
	}
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // operator-controlled argv
	if wd := os.Getenv(envUserWorkdir); wd != "" {
		cmd.Dir = wd
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil
	fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: sidecar exec argv=%v workdir=%q\n", argv, cmd.Dir)
	timeout := jobTimeoutFromEnv()
	exitCode, timedOut, err := runWithTimeout(cmd, timeout, "sidecar")
	if timedOut {
		os.Exit(jobTimeoutExitCode)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: sidecar subprocess exit=%d err=%v\n", exitCode, err)
		os.Exit(exitCode)
	}
	fmt.Fprintln(os.Stderr, "sockerless-gcf-bootstrap: sidecar subprocess exit=0")
}

// jobTimeoutFromEnv parses SOCKERLESS_JOB_TIMEOUT_SECONDS into a
// duration in seconds. Empty/invalid → default. Negative → 0 (disabled).
func jobTimeoutFromEnv() int {
	raw := strings.TrimSpace(os.Getenv(envJobTimeoutSeconds))
	if raw == "" {
		return jobTimeoutDefaultSeconds
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: %s=%q invalid; using default %d\n",
			envJobTimeoutSeconds, raw, jobTimeoutDefaultSeconds)
		return jobTimeoutDefaultSeconds
	}
	if n < 0 {
		return 0
	}
	return n
}

// runWithTimeout runs cmd with the configured job timeout. On timeout:
// SIGTERM → grace period → SIGKILL → returns timedOut=true. Caller
// should set exit code to jobTimeoutExitCode. timeoutSeconds<=0 disables.
func runWithTimeout(cmd *exec.Cmd, timeoutSeconds int, label string) (exitCode int, timedOut bool, err error) {
	if err := cmd.Start(); err != nil {
		return -1, false, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	if timeoutSeconds <= 0 {
		err := <-done
		return extractExitCode(err), false, err
	}
	timer := time.NewTimer(time.Duration(timeoutSeconds) * time.Second)
	defer timer.Stop()
	select {
	case err := <-done:
		return extractExitCode(err), false, err
	case <-timer.C:
		fmt.Fprintf(os.Stderr,
			"sockerless-gcf-bootstrap: %s workload timed out after %d seconds; sending SIGTERM\n",
			label, timeoutSeconds)
		_ = cmd.Process.Signal(syscall.SIGTERM)
		grace := time.NewTimer(time.Duration(jobTimeoutGracePeriod) * time.Second)
		defer grace.Stop()
		select {
		case <-done:
			fmt.Fprintf(os.Stderr,
				"sockerless-gcf-bootstrap: %s workload exited within grace period; bootstrap exiting %d\n",
				label, jobTimeoutExitCode)
		case <-grace.C:
			fmt.Fprintf(os.Stderr,
				"sockerless-gcf-bootstrap: %s workload did not exit after %ds grace; sending SIGKILL\n",
				label, jobTimeoutGracePeriod)
			_ = cmd.Process.Kill()
			<-done
			fmt.Fprintf(os.Stderr,
				"sockerless-gcf-bootstrap: %s bootstrap exiting %d\n",
				label, jobTimeoutExitCode)
		}
		return jobTimeoutExitCode, true, nil
	}
}

func extractExitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return 1
}
