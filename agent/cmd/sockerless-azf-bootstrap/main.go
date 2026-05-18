// sockerless-azf-bootstrap is the entrypoint binary baked into Azure
// Functions custom-container overlay images. It serves the platform HTTP
// trigger, runs the user's Docker argv as a subprocess, and keeps a
// reverse-agent WebSocket open for docker exec / cp / attach.
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
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sockerless/agent"
)

const (
	envPort           = "PORT"
	envWebsitesPort   = "WEBSITES_PORT"
	envUserEntrypoint = "SOCKERLESS_USER_ENTRYPOINT"
	envUserCmd        = "SOCKERLESS_USER_CMD"
	envUserWorkdir    = "SOCKERLESS_USER_WORKDIR"
	envCallbackURL    = "SOCKERLESS_CALLBACK_URL"
	envContainerID    = "SOCKERLESS_CONTAINER_ID"
	envJobTimeout     = "SOCKERLESS_JOB_TIMEOUT_SECONDS"
)

const (
	defaultPort              = "8080"
	defaultJobTimeoutSeconds = 600
	timeoutExitCode          = 124
)

func main() {
	fmt.Fprintf(os.Stdout, "sockerless-azf-bootstrap: MAIN ENTRY pid=%d args=%v PORT=%q WEBSITES_PORT=%q\n",
		os.Getpid(), os.Args, os.Getenv(envPort), os.Getenv(envWebsitesPort))
	fmt.Fprintf(os.Stderr, "sockerless-azf-bootstrap: MAIN ENTRY pid=%d args=%v PORT=%q WEBSITES_PORT=%q\n",
		os.Getpid(), os.Args, os.Getenv(envPort), os.Getenv(envWebsitesPort))

	callbackURL := os.Getenv(envCallbackURL)
	containerID := os.Getenv(envContainerID)
	if callbackURL != "" && containerID != "" {
		conn, err := agent.DialReverseAgent(callbackURL, containerID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sockerless-azf-bootstrap: reverse-agent dial failed: %v\n", err)
			os.Exit(1)
		}
		connMu := &sync.Mutex{}
		go agent.ServeReverseAgent(conn, connMu)
		go agent.StartHeartbeats(conn, connMu)
		go sendLifetimeExpiredOnSIGTERM(conn, connMu)
		fmt.Fprintf(os.Stderr, "sockerless-azf-bootstrap: reverse-agent connected to %s (session=%s)\n", callbackURL, containerID)
	} else {
		fmt.Fprintln(os.Stderr, "sockerless-azf-bootstrap: SOCKERLESS_CALLBACK_URL or SOCKERLESS_CONTAINER_ID empty - reverse-agent disabled")
	}

	port := os.Getenv(envPort)
	if port == "" {
		port = os.Getenv(envWebsitesPort)
	}
	if port == "" {
		port = defaultPort
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleInvoke)
	mux.HandleFunc("/api/function", handleInvoke)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}
	fmt.Fprintf(os.Stderr, "sockerless-azf-bootstrap: listening on :%s\n", port)
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "sockerless-azf-bootstrap: server exited: %v\n", err)
		os.Exit(1)
	}
}

func sendLifetimeExpiredOnSIGTERM(conn *websocket.Conn, connMu *sync.Mutex) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	<-sigCh
	done := make(chan error, 1)
	go func() {
		done <- agent.SendLifetimeExpired(conn, connMu)
	}()
	select {
	case err := <-done:
		if err != nil {
			fmt.Fprintf(os.Stderr, "sockerless-azf-bootstrap: send lifetime_expired on SIGTERM failed: %v\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "sockerless-azf-bootstrap: sent lifetime_expired on SIGTERM")
		}
	case <-time.After(2 * time.Second):
		fmt.Fprintln(os.Stderr, "sockerless-azf-bootstrap: timed out sending lifetime_expired on SIGTERM")
	}
	os.Exit(0)
}

func handleInvoke(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}

	argv, err := userArgv()
	if err != nil {
		writeTextResult(w, 1, []byte(err.Error()+"\n"))
		return
	}
	if len(argv) == 0 {
		writeTextResult(w, 0, []byte("{}"))
		return
	}

	ctx := r.Context()
	if timeout := jobTimeout(); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	if wd := os.Getenv(envUserWorkdir); wd != "" {
		cmd.Dir = wd
	}
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = io.MultiWriter(&stdout, os.Stdout)
	cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)

	err = cmd.Run()
	exitCode := 0
	if ctx.Err() == context.DeadlineExceeded {
		exitCode = timeoutExitCode
	} else if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
			stderr.WriteString(err.Error())
			stderr.WriteByte('\n')
		}
	}

	body := stdout.Bytes()
	if len(body) == 0 && len(stderr.Bytes()) > 0 {
		body = stderr.Bytes()
	}
	writeTextResult(w, exitCode, body)
}

func userArgv() ([]string, error) {
	entrypoint, err := decodeArgvEnv(envUserEntrypoint)
	if err != nil {
		return nil, err
	}
	cmd, err := decodeArgvEnv(envUserCmd)
	if err != nil {
		return nil, err
	}
	argv := append([]string{}, entrypoint...)
	argv = append(argv, cmd...)
	return argv, nil
}

func decodeArgvEnv(name string) ([]string, error) {
	v := os.Getenv(name)
	if v == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("%s is not valid base64: %w", name, err)
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%s is not JSON argv: %w", name, err)
	}
	return out, nil
}

func jobTimeout() time.Duration {
	v := os.Getenv(envJobTimeout)
	if v == "" {
		return defaultJobTimeoutSeconds * time.Second
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

func writeTextResult(w http.ResponseWriter, exitCode int, body []byte) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Sockerless-Exit-Code", strconv.Itoa(exitCode))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
