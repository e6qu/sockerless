package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// lambdaInvocation represents a single pending Lambda invocation served
// over the Runtime API. Matches the real Lambda contract: a unique
// request ID, the payload the handler polls via /next, a deadline, and
// channels for the handler's /response or /error reply.
type lambdaInvocation struct {
	RequestID   string
	FunctionArn string
	Payload     []byte
	DeadlineMs  int64
	TraceID     string

	// Single-slot queue: /next reads once; /response or /error writes once.
	delivered bool
	mu        sync.Mutex

	done     chan struct{} // closed when response or error received
	response []byte
	errorObj []byte // JSON error payload when /error was called
}

// runtimeAPISidecar is a per-invocation HTTP server that implements the
// AWS Lambda Runtime API for one container. Matches real Lambda where
// each running function container has its own dedicated Runtime API on
// 127.0.0.1:9001; in the simulator it runs on the host and the
// container reaches it via host.docker.internal.
type runtimeAPISidecar struct {
	inv      *lambdaInvocation
	listener net.Listener
	server   *http.Server
	addr     string // "host.docker.internal:<port>" — what the container sees
}

// startRuntimeAPISidecar binds a free port on all interfaces, mounts
// the Runtime API routes, and starts serving in a background
// goroutine. Must bind to 0.0.0.0 (not 127.0.0.1) because the
// function container reaches back via the docker bridge gateway on
// Linux (172.17.0.1 / host.docker.internal), which is a different
// interface than loopback. Returns the sidecar so the caller can pass
// its address into the container and shut the sidecar down after the
// invocation completes.
func startRuntimeAPISidecar(inv *lambdaInvocation) (*runtimeAPISidecar, error) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("runtime API listen: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	mux := http.NewServeMux()
	s := &runtimeAPISidecar{
		inv:      inv,
		listener: ln,
		addr:     fmt.Sprintf("%s:%d", runtimeAPIHost(), port),
	}

	// GET /2018-06-01/runtime/invocation/next
	mux.HandleFunc("GET /2018-06-01/runtime/invocation/next", s.handleNext)
	// POST /2018-06-01/runtime/invocation/{id}/response
	mux.HandleFunc("POST /2018-06-01/runtime/invocation/{id}/response", s.handleResponse)
	// POST /2018-06-01/runtime/invocation/{id}/error
	mux.HandleFunc("POST /2018-06-01/runtime/invocation/{id}/error", s.handleInvocationError)
	// POST /2018-06-01/runtime/init/error
	mux.HandleFunc("POST /2018-06-01/runtime/init/error", s.handleInitError)

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  0, // /next is long-poll; no timeout
		WriteTimeout: 0,
	}

	go func() {
		_ = s.server.Serve(ln)
	}()

	return s, nil
}

// Shutdown closes the sidecar listener after the invocation completes.
func (s *runtimeAPISidecar) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.server.Shutdown(ctx)
}

// ContainerAddr returns the address the container should use to reach
// this sidecar (via the Docker host-gateway alias).
func (s *runtimeAPISidecar) ContainerAddr() string {
	return s.addr
}

// handleNext serves GET /2018-06-01/runtime/invocation/next. Blocks
// until the invocation payload is ready (with this design it's already
// queued when the container starts, so it returns immediately the first
// time and hangs on subsequent calls until the server is shut down).
func (s *runtimeAPISidecar) handleNext(w http.ResponseWriter, r *http.Request) {
	s.inv.mu.Lock()
	if s.inv.delivered {
		s.inv.mu.Unlock()
		// Hold the connection open until the sidecar shuts down — real
		// Lambda blocks /next until the next invocation arrives. Our
		// per-invocation sidecar only serves one, so further polls wait
		// for shutdown.
		<-r.Context().Done()
		return
	}
	s.inv.delivered = true
	s.inv.mu.Unlock()

	w.Header().Set("Lambda-Runtime-Aws-Request-Id", s.inv.RequestID)
	w.Header().Set("Lambda-Runtime-Invoked-Function-Arn", s.inv.FunctionArn)
	w.Header().Set("Lambda-Runtime-Deadline-Ms", fmt.Sprintf("%d", s.inv.DeadlineMs))
	if s.inv.TraceID != "" {
		w.Header().Set("Lambda-Runtime-Trace-Id", s.inv.TraceID)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(s.inv.Payload)
}

// handleResponse serves POST /2018-06-01/runtime/invocation/{id}/response.
func (s *runtimeAPISidecar) handleResponse(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id != s.inv.RequestID {
		sim.AWSErrorf(w, "InvalidRequestID", http.StatusBadRequest,
			"Invocation with id %s doesn't exist", id)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sim.AWSError(w, "InvalidRequestBody", "Failed to read response body", http.StatusBadRequest)
		return
	}
	s.inv.response = body
	select {
	case <-s.inv.done:
		// Already signaled (e.g. duplicate response); ignore.
	default:
		close(s.inv.done)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"OK"}`))
}

// handleInvocationError serves POST /2018-06-01/runtime/invocation/{id}/error.
func (s *runtimeAPISidecar) handleInvocationError(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id != s.inv.RequestID {
		sim.AWSErrorf(w, "InvalidRequestID", http.StatusBadRequest,
			"Invocation with id %s doesn't exist", id)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sim.AWSError(w, "InvalidRequestBody", "Failed to read error body", http.StatusBadRequest)
		return
	}
	s.inv.errorObj = body
	select {
	case <-s.inv.done:
	default:
		close(s.inv.done)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"OK"}`))
}

// handleInitError serves POST /2018-06-01/runtime/init/error.
func (s *runtimeAPISidecar) handleInitError(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sim.AWSError(w, "InvalidRequestBody", "Failed to read error body", http.StatusBadRequest)
		return
	}
	s.inv.errorObj = body
	select {
	case <-s.inv.done:
	default:
		close(s.inv.done)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"OK"}`))
}

// runtimeAPIHost returns the hostname the container uses to reach the
// simulator host. Override via SIM_LAMBDA_RUNTIME_HOST; default
// host.docker.internal which Docker Desktop maps automatically,
// Podman 4+ maps automatically, and Linux Docker maps via the
// `--add-host ...:host-gateway` entry in ContainerConfig.ExtraHosts
// (see runtimeAPIExtraHosts).
func runtimeAPIHost() string {
	if v := os.Getenv("SIM_LAMBDA_RUNTIME_HOST"); v != "" {
		return v
	}
	return "host.docker.internal"
}

// runtimeAPIExtraHosts returns the Docker --add-host entries needed
// for the container to resolve host.docker.internal. Podman 4+ and
// Docker Desktop expose it natively; Linux Docker needs the magic
// `host-gateway` replacement. Podman doesn't support that magic value
// and will error if passed, so we skip ExtraHosts on Podman.
func runtimeAPIExtraHosts() []string {
	info := strings.ToLower(sim.RuntimeInfo())
	if strings.Contains(info, "podman") {
		// Podman already exposes host.docker.internal + host.containers.internal.
		return nil
	}
	return []string{"host.docker.internal:host-gateway"}
}

// invokeLambdaViaRuntimeAPI launches the function container with
// AWS_LAMBDA_RUNTIME_API pointing at a per-invocation sidecar, feeds
// the payload via /next, and returns whatever the handler posts back
// to /response (or /error → X-Amz-Function-Error: Unhandled).
//
// Returns: responseBody, unhandledError (true if /error was posted),
// exitCode from the container. Unhandled errors come back as proper
// Lambda error JSON even when the container itself exits 0.
func invokeLambdaViaRuntimeAPI(fn LambdaFunction, payload []byte) ([]byte, bool, int) {
	if fn.Code == nil || fn.Code.ImageUri == "" {
		// Zip-package path: no container, no Runtime API — return the
		// synthetic "{}" that control-plane-only tests expect plus
		// inject the standard log stream via the existing helper.
		injectLambdaLogs(fn.FunctionName)
		return []byte("{}"), false, 0
	}

	// Build invocation + sidecar.
	requestID := generateUUID()
	timeoutSec := fn.Timeout
	if timeoutSec == 0 {
		timeoutSec = 3
	}
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)

	inv := &lambdaInvocation{
		RequestID:   requestID,
		FunctionArn: fn.FunctionArn,
		Payload:     payload,
		DeadlineMs:  deadline.UnixMilli(),
		TraceID:     generateUUID(),
		done:        make(chan struct{}),
	}

	sidecar, err := startRuntimeAPISidecar(inv)
	if err != nil {
		errBody := lambdaErrorPayload(fmt.Sprintf("Runtime API sidecar start failed: %v", err))
		return errBody, true, 1
	}
	defer sidecar.Shutdown()

	// CloudWatch log group + stream + START log entry.
	logGroup, logStream, logKey, startMs := injectLambdaInvokeLogs(fn.FunctionName, requestID)

	// Extract entrypoint/command/env from the function's ImageConfig
	// the same way the old sync path did.
	var entrypoint, args []string
	if fn.ImageConfig != nil {
		entrypoint = fn.ImageConfig.EntryPoint
		args = fn.ImageConfig.Command
	}
	cmdEnv := map[string]string{
		"AWS_LAMBDA_RUNTIME_API":          sidecar.ContainerAddr(),
		"AWS_LAMBDA_FUNCTION_NAME":        fn.FunctionName,
		"AWS_LAMBDA_FUNCTION_VERSION":     fn.Version,
		"AWS_LAMBDA_FUNCTION_MEMORY_SIZE": fmt.Sprintf("%d", fn.MemorySize),
		"AWS_REGION":                      "us-east-1",
		"AWS_DEFAULT_REGION":              "us-east-1",
		"AWS_LAMBDA_LOG_GROUP_NAME":       logGroup,
		"AWS_LAMBDA_LOG_STREAM_NAME":      logStream,
		"_HANDLER":                        fn.Handler,
		"AWS_LAMBDA_INITIALIZATION_TYPE":  "on-demand",
	}
	if fn.Environment != nil {
		for k, v := range fn.Environment.Variables {
			cmdEnv[k] = v
		}
	}

	sink := &lambdaLogSink{logGroup: logGroup, logStream: logStream}
	var stderr bytes.Buffer
	collectSink := sim.FuncSink(func(line sim.LogLine) {
		sink.WriteLog(line)
		if line.Stream == "stderr" {
			stderr.WriteString(line.Text)
			stderr.WriteByte('\n')
		}
	})

	handle, err := sim.StartContainerSync(sim.ContainerConfig{
		Image:   sim.ResolveLocalImage(fn.Code.ImageUri),
		Command: entrypoint,
		Args:    args,
		Env:     cmdEnv,
		// Timeout is enforced by the sidecar (waiting for /response or
		// error with a deadline); the container itself is given a
		// generous wall-clock budget so slow handlers still surface a
		// proper Lambda timeout instead of a container-level kill.
		Timeout:    time.Duration(timeoutSec+5) * time.Second,
		Name:       fmt.Sprintf("sockerless-sim-aws-lambda-%s", requestID[:12]),
		Labels:     map[string]string{"sockerless-sim-lambda": requestID},
		ExtraHosts: runtimeAPIExtraHosts(),
	}, collectSink)
	if err != nil {
		endMs := time.Now().UnixMilli()
		appendLambdaLog(logKey, endMs, fmt.Sprintf("ERROR RequestId: %s Container start failed: %v", requestID, err))
		appendLambdaLog(logKey, endMs+1, fmt.Sprintf("END RequestId: %s", requestID))
		return lambdaErrorPayload(fmt.Sprintf("Container start failed: %v", err)), true, 1
	}
	lambdaProcessHandles.Store(requestID, handle)
	defer lambdaProcessHandles.Delete(requestID)

	// Race: handler posts /response|/error, or container exits without
	// posting, or deadline expires.
	var (
		result    []byte
		unhandled bool
		exitCode  int
	)
	waitForContainer := make(chan int, 1)
	go func() {
		res := handle.Wait()
		waitForContainer <- res.ExitCode
	}()

	timer := time.NewTimer(time.Duration(timeoutSec) * time.Second)
	defer timer.Stop()

	select {
	case <-inv.done:
		if inv.errorObj != nil {
			result = inv.errorObj
			unhandled = true
			exitCode = 0
		} else {
			result = inv.response
			if len(result) == 0 {
				result = []byte("{}")
			}
		}
		// Let the container exit on its own so logs drain fully; fall
		// back to cancelling after a short grace window if the handler
		// hangs after posting /response.
		select {
		case exitCode = <-waitForContainer:
		case <-time.After(3 * time.Second):
			handle.Cancel()
			exitCode = <-waitForContainer
		}
	case exitCode = <-waitForContainer:
		// Container exited without calling /response — runtime error.
		result = lambdaErrorPayload(fmt.Sprintf("Runtime exited without providing a reason (exit %d): %s", exitCode, strings.TrimSpace(stderr.String())))
		unhandled = true
	case <-timer.C:
		// Deadline expired.
		handle.Cancel()
		<-waitForContainer
		result = lambdaErrorPayload(fmt.Sprintf("Task timed out after %d.00 seconds", timeoutSec))
		unhandled = true
		exitCode = 1
	}

	// Inject END + REPORT log entries.
	endMs := time.Now().UnixMilli()
	durationMs := float64(time.Since(time.UnixMilli(startMs)).Microseconds()) / 1000.0
	appendLambdaLog(logKey, endMs, fmt.Sprintf("END RequestId: %s", requestID))
	appendLambdaLog(logKey, endMs+1, fmt.Sprintf(
		"REPORT RequestId: %s\tDuration: %.2f ms\tBilled Duration: %d ms\tMemory Size: %d MB\tMax Memory Used: %d MB",
		requestID, durationMs, int64(durationMs)+1, fn.MemorySize, fn.MemorySize/2))
	if unhandled {
		appendLambdaLog(logKey, endMs+2, fmt.Sprintf("ERROR RequestId: %s %s", requestID, strings.TrimSpace(string(result))))
	}

	return result, unhandled, exitCode
}

// injectLambdaInvokeLogs sets up the CloudWatch log group + stream and
// writes the START entry for one invocation. Returns the metadata the
// caller needs to append subsequent entries.
func injectLambdaInvokeLogs(functionName, requestID string) (logGroup, logStream, logKey string, startMs int64) {
	logGroup = fmt.Sprintf("/aws/lambda/%s", functionName)
	now := time.Now()
	startMs = now.UnixMilli()

	if _, exists := cwLogGroups.Get(logGroup); !exists {
		cwLogGroups.Put(logGroup, CWLogGroup{
			LogGroupName: logGroup,
			Arn:          cwLogGroupArn(logGroup),
			CreationTime: startMs,
		})
	}

	hexBytes := make([]byte, 8)
	if _, err := rand.Read(hexBytes); err != nil {
		hexBytes = []byte{0, 0, 0, 0, 0, 0, 0, 0}
	}
	logStream = fmt.Sprintf("%s/[$LATEST]%s", now.Format("2006/01/02"), hex.EncodeToString(hexBytes))
	logKey = cwEventsKey(logGroup, logStream)

	cwLogStreams.Put(logKey, CWLogStream{
		LogStreamName:       logStream,
		LogGroupName:        logGroup,
		CreationTime:        startMs,
		FirstEventTimestamp: startMs,
		LastEventTimestamp:  startMs,
		Arn:                 cwLogStreamArn(logGroup, logStream),
		UploadSequenceToken: "1",
	})
	cwLogEvents.Put(logKey, []CWLogEvent{
		{Timestamp: startMs, Message: fmt.Sprintf("START RequestId: %s Version: $LATEST", requestID), IngestionTime: startMs},
	})
	return
}

// appendLambdaLog adds one event to a stream.
func appendLambdaLog(logKey string, ts int64, msg string) {
	cwLogEvents.Update(logKey, func(events *[]CWLogEvent) {
		*events = append(*events, CWLogEvent{Timestamp: ts, Message: msg, IngestionTime: ts})
	})
}

// lambdaErrorPayload renders a Lambda-style error JSON body.
func lambdaErrorPayload(msg string) []byte {
	body, _ := json.Marshal(map[string]string{
		"errorMessage": msg,
		"errorType":    "Runtime.ExitError",
	})
	return body
}
