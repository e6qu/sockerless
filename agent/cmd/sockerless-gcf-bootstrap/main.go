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

func main() {
	port := os.Getenv(envPort)
	if port == "" {
		port = "8080"
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
	invokeMu.Lock()
	defer invokeMu.Unlock()

	// Path B exec envelope check (Phase 122g): if the request body
	// parses as {sockerless:{exec:{argv}}} then we run that envelope's
	// argv instead of the env-baked SOCKERLESS_USER_* cmd. Used by gcf
	// backend ContainerExec for `docker exec`.
	body, _ := io.ReadAll(r.Body)
	defer r.Body.Close()
	if env, ok := parseExecEnvelope(body); ok {
		runExecEnvelope(w, env)
		return
	}

	if pod, ok := parsePodManifest(); ok {
		main, found := pickPodMain(pod)
		if !found {
			http.Error(w, "no main container in pod manifest", http.StatusInternalServerError)
			return
		}
		runPodMain(w, main)
		return
	}

	argv := append(parseUserArgv(envUserEntrypoint), parseUserArgv(envUserCmd)...)
	if len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "sockerless-gcf-bootstrap: no user entrypoint/cmd configured")
		http.Error(w, "no entrypoint configured", http.StatusInternalServerError)
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

	if err := cmd.Run(); err != nil {
		exitCode := 1
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: subprocess argv=%v exit=%d err=%v\n", argv, exitCode, err)
		w.Header().Set("X-Sockerless-Exit-Code", strconv.Itoa(exitCode))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(stdout.Bytes())
		_, _ = w.Write(stderr.Bytes())
		return
	}
	fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: subprocess argv=%v exit=0 stdout=%dB\n", argv, stdout.Len())

	w.Header().Set("X-Sockerless-Exit-Code", "0")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(stdout.Bytes())
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

	exitCode := 0
	if err := cmd.Run(); err != nil {
		exitCode = 1
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: exec argv=%v exit=%d err=%v\n", env.Argv, exitCode, err)
	} else {
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: exec argv=%v exit=0 stdout=%dB stderr=%dB\n", env.Argv, stdout.Len(), stderr.Len())
	}

	var res execEnvelopeResponse
	res.SockerlessExecResult.ExitCode = exitCode
	res.SockerlessExecResult.Stdout = base64.StdEncoding.EncodeToString(stdout.Bytes())
	res.SockerlessExecResult.Stderr = base64.StdEncoding.EncodeToString(stderr.Bytes())

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
		http.Error(w, "pod main: "+err.Error(), http.StatusInternalServerError)
		return
	}
	prefix := fmt.Sprintf("[%s] ", m.Name)
	var stdout bytes.Buffer
	cmd.Stdout = io.MultiWriter(&stdout, newPrefixWriter(os.Stdout, prefix))
	cmd.Stderr = newPrefixWriter(os.Stderr, prefix)
	if err := cmd.Run(); err != nil {
		exitCode := 1
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: pod main %q exit=%d err=%v\n", m.Name, exitCode, err)
		w.Header().Set("X-Sockerless-Exit-Code", strconv.Itoa(exitCode))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(stdout.Bytes())
		return
	}
	fmt.Fprintf(os.Stderr, "sockerless-gcf-bootstrap: pod main %q exit=0 stdout=%dB\n", m.Name, stdout.Len())
	w.Header().Set("X-Sockerless-Exit-Code", "0")
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
