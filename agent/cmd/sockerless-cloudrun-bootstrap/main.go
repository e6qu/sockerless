// sockerless-cloudrun-bootstrap is the entrypoint binary injected into
// Cloud Run Service container images by the sockerless cloudrun backend.
// It serves an HTTP server on $PORT (Cloud Run sets this) and runs the
// caller's argv as a subprocess per request.
//
// Two request shapes are supported:
//
//  1. Default invoke (empty body or non-JSON): runs the env-baked
//     SOCKERLESS_USER_ENTRYPOINT + SOCKERLESS_USER_CMD as a subprocess
//     and returns combined stdout in the response body. Mirrors the gcf
//     bootstrap shape — used for `docker run <image>` semantics.
//
//  2. Exec envelope (body parses as
//     {"sockerless":{"exec":{"argv":[...],"workdir":"...","env":[...],
//     "stdin":"<base64>","tty":bool}}}): runs the envelope's argv
//     instead of the env-baked cmd, with the envelope's stdin piped to
//     the subprocess. Returns
//     {"sockerlessExecResult":{"exitCode":N,"stdout":"<base64>",
//     "stderr":"<base64>"}}. This is the Path B model from
//     specs/CLOUD_RESOURCE_MAPPING.md § Lesson 8 — what the cloudrun
//     backend's ContainerExec POSTs for each `docker exec` call.
//
// The bootstrap is a long-lived HTTP server because Cloud Run Service
// keeps a min_instance_count=1 instance warm; subsequent `docker exec`
// calls reuse the same instance for the lifetime of the container.
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
)

const (
	envPort           = "PORT"
	envUserEntrypoint = "SOCKERLESS_USER_ENTRYPOINT" // base64(JSON-encoded argv)
	envUserCmd        = "SOCKERLESS_USER_CMD"        // base64(JSON-encoded argv)
	envUserWorkdir    = "SOCKERLESS_USER_WORKDIR"    // chdir target (optional)
)

// execEnvelopeRequest is the JSON shape the cloudrun backend POSTs for
// each `docker exec`. Identical to the lambda backend's envelope so the
// wire format is cross-cloud consistent.
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

// invokeMu serializes invocations. Cloud Run instances may serve
// concurrent requests when concurrency > 1; sockerless's docker model
// is one-cmd-per-container so we serialize at the bootstrap level.
var invokeMu sync.Mutex

func main() {
	port := os.Getenv(envPort)
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleInvoke)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	fmt.Fprintf(os.Stderr, "sockerless-cloudrun-bootstrap: listening on :%s\n", port)
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-cloudrun-bootstrap: server exited: %v\n", err)
		os.Exit(1)
	}
}

// handleInvoke dispatches: if the request body parses as an
// execEnvelope, runs envelope.argv (Path B); otherwise runs the
// env-baked SOCKERLESS_USER_* cmd (default invoke).
func handleInvoke(w http.ResponseWriter, r *http.Request) {
	invokeMu.Lock()
	defer invokeMu.Unlock()

	body, _ := io.ReadAll(r.Body)
	defer r.Body.Close()

	if env, ok := parseExecEnvelope(body); ok {
		runExecEnvelope(w, env)
		return
	}
	runDefaultInvoke(w)
}

// parseExecEnvelope returns the parsed envelope when the body is a
// well-formed JSON object with `sockerless.exec.argv` non-empty.
// Anything else (empty body, raw bytes, JSON without the envelope
// shape) → false so the default invoke path runs.
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
// response body. Used by the cloudrun backend's Path B docker exec.
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
		fmt.Fprintf(os.Stderr, "sockerless-cloudrun-bootstrap: exec argv=%v exit=%d err=%v\n", env.Argv, exitCode, err)
	} else {
		fmt.Fprintf(os.Stderr, "sockerless-cloudrun-bootstrap: exec argv=%v exit=0 stdout=%dB stderr=%dB\n", env.Argv, stdout.Len(), stderr.Len())
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

// runDefaultInvoke runs the env-baked SOCKERLESS_USER_* cmd as a
// subprocess and returns combined stdout in the response body. Used
// for `docker run <image>` semantics where the container's CMD runs
// once per invocation.
func runDefaultInvoke(w http.ResponseWriter) {
	argv := append(parseUserArgv(envUserEntrypoint), parseUserArgv(envUserCmd)...)
	if len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "sockerless-cloudrun-bootstrap: no user entrypoint/cmd configured")
		http.Error(w, "no entrypoint configured", http.StatusInternalServerError)
		return
	}

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
		fmt.Fprintf(os.Stderr, "sockerless-cloudrun-bootstrap: subprocess argv=%v exit=%d err=%v\n", argv, exitCode, err)
		w.Header().Set("X-Sockerless-Exit-Code", strconv.Itoa(exitCode))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(stdout.Bytes())
		_, _ = w.Write(stderr.Bytes())
		return
	}
	fmt.Fprintf(os.Stderr, "sockerless-cloudrun-bootstrap: subprocess argv=%v exit=0 stdout=%dB\n", argv, stdout.Len())

	w.Header().Set("X-Sockerless-Exit-Code", "0")
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(stdout.Bytes())
}

// parseUserArgv decodes a base64(JSON) env-var into a []string. Empty
// / missing / undecodable → nil. Mirrors the gcf + lambda bootstrap
// encoding for cross-cloud consistency.
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

// quoteArgv single-quotes each argv entry so the joined line is safe
// for `/bin/sh -c`. Embedded single quotes are escaped via the
// classic `'\”` idiom.
func quoteArgv(argv []string) []string {
	out := make([]string, len(argv))
	for i, a := range argv {
		out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return out
}
