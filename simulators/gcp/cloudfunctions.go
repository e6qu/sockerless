package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// Cloud Functions v2 types

// Function represents a Cloud Functions v2 function.
type Function struct {
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	BuildConfig   *BuildConfig      `json:"buildConfig,omitempty"`
	ServiceConfig *ServiceConfig    `json:"serviceConfig,omitempty"`
	State         string            `json:"state"`
	CreateTime    string            `json:"createTime"`
	UpdateTime    string            `json:"updateTime"`
	Labels        map[string]string `json:"labels,omitempty"`
	Environment   string            `json:"environment,omitempty"`
}

// BuildConfig holds the build configuration for a function.
type BuildConfig struct {
	Runtime          string `json:"runtime,omitempty"`
	EntryPoint       string `json:"entryPoint,omitempty"`
	Source           any    `json:"source,omitempty"`
	DockerRepository string `json:"dockerRepository,omitempty"`
}

// ServiceConfig holds the service configuration for a function.
type ServiceConfig struct {
	Uri                  string            `json:"uri,omitempty"`
	Service              string            `json:"service,omitempty"` // Underlying Cloud Run service name (Gen2)
	TimeoutSeconds       int               `json:"timeoutSeconds,omitempty"`
	AvailableMemory      string            `json:"availableMemory,omitempty"`
	MaxInstanceCount     int               `json:"maxInstanceCount,omitempty"`
	MinInstanceCount     int               `json:"minInstanceCount,omitempty"`
	EnvironmentVariables map[string]string `json:"environmentVariables,omitempty"`
	SimCommand           []string          `json:"simCommand,omitempty"` // Simulator-only: command to execute on invoke
}

// Package-level store for dashboard access.
var gcfFunctions sim.Store[Function]

func registerCloudFunctions(srv *sim.Server) {
	functions := sim.MakeStore[Function](srv.DB(), "gcf_functions")
	gcfFunctions = functions

	// Create function
	srv.HandleFunc("POST /v2/projects/{project}/locations/{location}/functions", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		functionID := r.URL.Query().Get("functionId")
		if functionID == "" {
			sim.GCPError(w, http.StatusBadRequest, "functionId query parameter is required", "INVALID_ARGUMENT")
			return
		}

		var fn Function
		if err := sim.ReadJSON(r, &fn); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

		name := fmt.Sprintf("projects/%s/locations/%s/functions/%s", project, location, functionID)
		if _, exists := functions.Get(name); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "function %q already exists", name)
			return
		}

		now := nowTimestamp()
		fn.Name = name
		fn.State = "ACTIVE"
		fn.CreateTime = now
		fn.UpdateTime = now
		if fn.Environment == "" {
			fn.Environment = "GEN_2"
		}
		if fn.ServiceConfig == nil {
			fn.ServiceConfig = &ServiceConfig{}
		}
		// Use the simulator's own address as the function URL for invocations
		fn.ServiceConfig.Uri = fmt.Sprintf("http://%s/v2-functions-invoke/%s", r.Host, functionID)

		// Cloud Functions Gen2 are backed by a Cloud Run service that
		// real GCP creates server-side as part of CreateFunction. The
		// gcf overlay-and-swap path relies on `fn.ServiceConfig.Service`
		// being populated so it can call `Run.Services.GetService` /
		// `UpdateService` to swap the throwaway Buildpacks image with
		// the real overlay. Mirror that linkage here: stamp the
		// service name onto the function, and seed a backing ServiceV2
		// row so subsequent Get/PATCH on the service round-trip.
		stubImage := ""
		if fn.BuildConfig != nil {
			stubImage = fn.BuildConfig.DockerRepository
		}
		backingService := seedServiceV2Defaults(ServiceV2{
			Template: &RevisionTemplate{
				Containers: []Container{{Name: functionID, Image: stubImage}},
			},
		}, project, location, functionID)
		fn.ServiceConfig.Service = backingService.Name
		crv2Services.Put(backingService.Name, backingService)

		functions.Put(name, fn)

		lro := newLRO(project, location, fn, "type.googleapis.com/google.cloud.functions.v2.Function")
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// Get function
	srv.HandleFunc("GET /v2/projects/{project}/locations/{location}/functions/{function}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		functionID := sim.PathParam(r, "function")
		name := fmt.Sprintf("projects/%s/locations/%s/functions/%s", project, location, functionID)

		fn, ok := functions.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "function %q not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, fn)
	})

	// List functions
	srv.HandleFunc("GET /v2/projects/{project}/locations/{location}/functions", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		prefix := fmt.Sprintf("projects/%s/locations/%s/functions/", project, location)
		filter := r.URL.Query().Get("filter")

		result := functions.Filter(func(fn Function) bool {
			if !strings.HasPrefix(fn.Name, prefix) {
				return false
			}
			return matchesFunctionFilter(&fn, filter)
		})
		if result == nil {
			result = []Function{}
		}

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"functions": result,
		})
	})

	// Invoke function (simulator-only endpoint)
	srv.HandleFunc("POST /v2-functions-invoke/{functionID}", func(w http.ResponseWriter, r *http.Request) {
		functionID := sim.PathParam(r, "functionID")

		// Find the function by scanning for a matching functionID suffix
		var fn *Function
		for _, f := range functions.List() {
			if strings.HasSuffix(f.Name, "/functions/"+functionID) {
				f := f // copy
				fn = &f
				break
			}
		}

		responseBody := []byte("{}")
		if fn != nil {
			project := strings.Split(fn.Name, "/")[1] // projects/{project}/...

			// Check for SOCKERLESS_CMD env var (cloud-native) or SimCommand fallback
			simCmd := false
			if fn.ServiceConfig != nil {
				if _, ok := fn.ServiceConfig.EnvironmentVariables["SOCKERLESS_CMD"]; ok {
					simCmd = true
				}
				if !simCmd && len(fn.ServiceConfig.SimCommand) > 0 {
					simCmd = true
				}
			}

			if simCmd {
				var exitCode int
				responseBody, exitCode = invokeCloudFunctionProcess(fn, project, functionID)
				if exitCode != 0 {
					// Real Cloud Functions returns HTTP error when function crashes
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write(responseBody)
					return
				}
			} else {
				injectCloudFunctionLog(project, functionID, "Function invoked")
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responseBody)
	})

	// Delete function
	srv.HandleFunc("DELETE /v2/projects/{project}/locations/{location}/functions/{function}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		functionID := sim.PathParam(r, "function")
		name := fmt.Sprintf("projects/%s/locations/%s/functions/%s", project, location, functionID)

		fn, ok := functions.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "function %q not found", name)
			return
		}

		functions.Delete(name)

		lro := newLRO(project, location, fn, "type.googleapis.com/google.cloud.functions.v2.Function")
		sim.WriteJSON(w, http.StatusOK, lro)
	})
}

// invokeCloudFunctionProcess executes a Cloud Function invocation. Two
// paths:
//
//   - Image path (cloud-faithful): the function is backed by a real
//     Cloud Run service whose container image is the sockerless overlay.
//     The overlay's ENTRYPOINT is the bootstrap HTTP server, which on
//     each request runs the user's entrypoint+cmd as a subprocess and
//     returns the captured output. The sim mirrors this by starting
//     the overlay container, POSTing to its bootstrap, reading the
//     response, then stopping the container — which is what real Cloud
//     Run Functions Gen2 does on every invocation. Exit code rides in
//     the `X-Sockerless-Exit-Code` header.
//
//   - Process path (sim-only test convenience): the function has no
//     image and a `simCommand` set on its ServiceConfig. The sim runs
//     the command as a host process. Used by SDK tests that want to
//     verify Cloud Functions invocation semantics without staging an
//     overlay image.
func invokeCloudFunctionProcess(fn *Function, project, functionID string) ([]byte, int) {
	// Container image lives on the underlying Cloud Run service —
	// Cloud Functions Gen2 are backed by a Run service, and the gcf
	// backend's overlay-and-swap path lands the real image there via
	// `Run.Services.UpdateService`. Read it back from there; the sim
	// has no other source of truth for what to execute.
	var image string
	if fn.ServiceConfig != nil && fn.ServiceConfig.Service != "" {
		if svc, ok := crv2Services.Get(fn.ServiceConfig.Service); ok {
			if svc.Template != nil && len(svc.Template.Containers) > 0 {
				image = svc.Template.Containers[0].Image
			}
		}
	}

	timeout := 60 * time.Second // GCP default
	if fn.ServiceConfig != nil && fn.ServiceConfig.TimeoutSeconds > 0 {
		timeout = time.Duration(fn.ServiceConfig.TimeoutSeconds) * time.Second
	}

	sink := &cfLogSink{project: project, functionName: functionID}

	if image != "" {
		// Cloud-faithful: HTTP-invoke the overlay's bootstrap.
		body, exitCode, err := invokeOverlayContainerHTTP(image, functionID, timeout, sink)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[sim-gcf] invocation error fn=%s img=%s: %v\n", functionID, image, err)
			injectCloudFunctionLog(project, functionID,
				fmt.Sprintf("Function invocation error: %v", err))
			return []byte(fmt.Sprintf(`{"error":%q}`, err.Error())), 1
		}
		if exitCode != 0 {
			fmt.Fprintf(os.Stderr, "[sim-gcf] non-zero exit fn=%s img=%s exit=%d body=%q\n", functionID, image, exitCode, string(body))
			injectCloudFunctionLog(project, functionID,
				fmt.Sprintf("Function exited with code %d body=%q", exitCode, string(body)))
		}
		return body, exitCode
	}

	// Process path: SimCommand-based (SDK tests). Decode any user
	// entrypoint/cmd from base64-JSON env vars first; fall back to
	// SimCommand if neither is set.
	var entrypoint, userCmd []string
	if fn.ServiceConfig != nil {
		if epB64, ok := fn.ServiceConfig.EnvironmentVariables["SOCKERLESS_USER_ENTRYPOINT"]; ok {
			if decoded, err := base64.StdEncoding.DecodeString(epB64); err == nil {
				_ = json.Unmarshal(decoded, &entrypoint)
			}
		}
		if cmdB64, ok := fn.ServiceConfig.EnvironmentVariables["SOCKERLESS_USER_CMD"]; ok {
			if decoded, err := base64.StdEncoding.DecodeString(cmdB64); err == nil {
				_ = json.Unmarshal(decoded, &userCmd)
			}
		}
		if len(entrypoint) == 0 && len(userCmd) == 0 {
			userCmd = fn.ServiceConfig.SimCommand
		}
	}

	if len(entrypoint) == 0 && len(userCmd) == 0 {
		// Nothing to invoke — function is essentially a stub.
		return []byte("{}"), 0
	}

	var cmdEnv map[string]string
	if fn.ServiceConfig != nil && fn.ServiceConfig.EnvironmentVariables != nil {
		cmdEnv = fn.ServiceConfig.EnvironmentVariables
	}

	var stdout bytes.Buffer
	collectSink := sim.FuncSink(func(line sim.LogLine) {
		sink.WriteLog(line)
		if line.Stream == "stdout" {
			stdout.WriteString(line.Text)
			stdout.WriteByte('\n')
		}
	})

	procCmd := append([]string{}, entrypoint...)
	procCmd = append(procCmd, userCmd...)
	handle := sim.StartProcess(sim.ProcessConfig{
		Command: procCmd,
		Env:     cmdEnv,
		Timeout: timeout,
	}, collectSink)
	result := handle.Wait()

	if result.ExitCode != 0 {
		injectCloudFunctionLog(project, functionID,
			fmt.Sprintf("Function execution error: process exited with code %d", result.ExitCode))
	}

	output := strings.TrimRight(stdout.String(), "\n")
	if output == "" {
		return []byte("{}"), result.ExitCode
	}
	return []byte(output), result.ExitCode
}

// cfLogSink implements sim.LogSink and writes log lines to Cloud Logging
// for Cloud Function invocations.
type cfLogSink struct {
	project      string
	functionName string
}

func (s *cfLogSink) WriteLog(line sim.LogLine) {
	injectCloudFunctionLog(s.project, s.functionName, line.Text)
}

// matchesFunctionFilter evaluates a Cloud Functions ListFunctions
// `filter` query against a Function. Supports the subset the gcf
// backend uses for pool-claim and allocation lookup:
//
//   - `labels.<key>:"<value>"` — Cloud Logging-style "has" / substring
//     match against the label value (the `:` operator).
//   - `labels.<key>="<value>"` — exact match.
//   - `-labels.<key>:*` — negation + wildcard: clause matches when the
//     label is unset or empty (i.e. the function is "free" of an
//     allocation claim, used by claimFreeFunction).
//   - Multiple clauses joined by ` AND `.
//
// Empty filter matches every Function. Real Cloud Functions supports
// the full Cloud Logging filter syntax; this is the operator subset
// the backend exercises today.
func matchesFunctionFilter(fn *Function, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	for _, raw := range strings.Split(filter, " AND ") {
		clause := strings.TrimSpace(raw)
		if clause == "" {
			continue
		}
		negate := false
		if strings.HasPrefix(clause, "-") {
			negate = true
			clause = clause[1:]
		}
		// Wildcard form `labels.<key>:*` — clause is true when the
		// label is set to anything non-empty. With `-` prefix, true
		// when the label is unset/empty.
		if strings.HasSuffix(clause, ":*") {
			field := strings.TrimSuffix(clause, ":*")
			val := lookupFunctionField(fn, field)
			present := val != ""
			matched := present
			if negate {
				matched = !present
			}
			if !matched {
				return false
			}
			continue
		}
		c := parseClause(clause)
		val := lookupFunctionField(fn, c.field)
		matched := false
		switch c.op {
		case opEq:
			matched = val == c.value
		case opHas:
			matched = strings.Contains(val, c.value)
		default:
			// Functions don't have ordered fields the backend
			// filters on — > / >= are unsupported here.
			matched = false
		}
		if negate {
			matched = !matched
		}
		if !matched {
			return false
		}
	}
	return true
}

// lookupFunctionField resolves a dot-notation field path on a Function.
// Currently supports `labels.<key>` and `name`; extend as the backend
// surfaces new filter shapes.
func lookupFunctionField(fn *Function, field string) string {
	if strings.HasPrefix(field, "labels.") {
		key := field[len("labels."):]
		if fn.Labels != nil {
			return fn.Labels[key]
		}
		return ""
	}
	switch field {
	case "name":
		return fn.Name
	case "state":
		return string(fn.State)
	}
	return ""
}

// injectCloudFunctionLog writes a log entry to the Cloud Logging store for a
// Cloud Function invocation, using the resource type and labels that the
// Cloud Functions backend's log filter expects.
func injectCloudFunctionLog(project, functionName, text string) {
	logName := fmt.Sprintf("projects/%s/logs/run.googleapis.com%%2Fstdout", project)
	writeLogEntries(logName, &MonitoredResource{
		Type:   "cloud_run_revision",
		Labels: map[string]string{"service_name": functionName},
	}, nil, []LogEntry{{TextPayload: text}})
}

// invokeOverlayContainerHTTP runs the cloud-faithful invocation flow:
// start the overlay container detached, wait for the bootstrap HTTP
// server to be ready on its assigned host port, POST to it, read the
// response body + the bootstrap-set `X-Sockerless-Exit-Code` header,
// then stop and remove the container.
//
// This mirrors what real Cloud Run does for every Cloud Functions Gen2
// invocation: route the request to the underlying container's HTTP
// listener and return the response. The exit code header is set by
// `sockerless-gcf-bootstrap` so the docker-shell perceives the
// underlying subprocess's true exit status (matters for `docker run
// --rm <fail>` semantics where 1 should propagate, etc.).
//
// The container is short-lived per invocation (start → POST → stop).
// That keeps the sim's container-state footprint bounded — at most one
// in-flight invocation container per concurrent request — and matches
// docker-run-style one-shot semantics. Real Cloud Run keeps containers
// warm across invocations; the sim's per-invocation lifecycle is a
// simplification that doesn't change the semantic contract (the same
// command is run, the same output is returned).
//
// Errors are returned only for infrastructure failures (image pull,
// container start, networking). Subprocess non-zero exit is NOT an
// error — it surfaces via the `exitCode` return value.
func invokeOverlayContainerHTTP(image, functionID string, timeout time.Duration, sink sim.LogSink) (responseBody []byte, exitCode int, err error) {
	cli := sim.DockerClient()
	if cli == nil {
		return nil, -1, fmt.Errorf("docker client not initialized")
	}

	localImage := sim.ResolveLocalImage(image)

	// Bootstrap listens on $PORT (defaults 8080). Bind to a random host
	// port so concurrent invocations on the same host don't collide.
	hostPort, err := pickFreeTCPPort()
	if err != nil {
		return nil, -1, fmt.Errorf("pick free port: %w", err)
	}

	containerName := fmt.Sprintf("sockerless-sim-gcf-%s-%d", functionID, hostPort)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	containerID, err := sim.StartHTTPContainer(ctx, sim.HTTPContainerConfig{
		Image:    localImage,
		HostPort: hostPort,
		Env: map[string]string{
			"PORT": "8080",
		},
		Name: containerName,
		Labels: map[string]string{
			"sockerless-sim-function": functionID,
		},
	})
	if err != nil {
		return nil, -1, fmt.Errorf("start overlay container: %w", err)
	}
	defer sim.StopAndRemoveContainer(containerID)

	// Stream container logs to Cloud Logging in the background. Uses
	// the same sink as the process path so test assertions on
	// `gcpFunctionLogMessages` find the bootstrap's stdout/stderr (the
	// user subprocess output is written to the bootstrap's own
	// stdout/stderr via io.MultiWriter — see agent/cmd/sockerless-gcf-
	// bootstrap/main.go::handleInvoke).
	logStreamCtx, logStreamCancel := context.WithCancel(context.Background())
	defer logStreamCancel()
	go sim.StreamContainerLogs(logStreamCtx, containerID, sink)

	// Wait for the bootstrap to start serving HTTP. Bootstrap prints
	// "sockerless-gcf-bootstrap: listening on :8080" then calls
	// ListenAndServe — once the listener is up, any TCP dial succeeds.
	bootstrapURL := fmt.Sprintf("http://127.0.0.1:%d/", hostPort)
	if err := waitForHTTP(ctx, bootstrapURL, 30*time.Second); err != nil {
		return nil, -1, fmt.Errorf("bootstrap not ready at %s: %w", bootstrapURL, err)
	}

	// POST the invocation. Empty body matches what the gcf backend's
	// invokeFunction sends (it calls POST with nil body).
	httpClient := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, "POST", bootstrapURL, nil)
	if err != nil {
		return nil, -1, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, -1, fmt.Errorf("invoke bootstrap: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Exit code propagation: bootstrap sets X-Sockerless-Exit-Code on
	// non-zero exit so the calling docker-shell perceives the
	// underlying subprocess's true status. Successful invocations
	// (HTTP 200) imply exit 0 even without the header.
	exitCode = 0
	if hdr := resp.Header.Get("X-Sockerless-Exit-Code"); hdr != "" {
		if n, parseErr := strconv.Atoi(hdr); parseErr == nil {
			exitCode = n
		}
	} else if resp.StatusCode >= 400 {
		// Bootstrap omitted the header but returned an error status —
		// surface a non-zero exit so the caller treats this as a
		// failed invocation rather than a silent success.
		exitCode = 1
	}

	return body, exitCode, nil
}

// pickFreeTCPPort opens a transient TCP listener to discover a
// free port number, then closes it. The OS may reassign the port
// before the caller binds it (TOCTOU); on a single-host sim this is
// vanishingly rare and reusing it is safe.
func pickFreeTCPPort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port, nil
}

// waitForHTTP polls `url` until any response is received (2xx, 4xx,
// 5xx — all OK) or the deadline elapses. Used to detect that the
// container's HTTP server has bound to its port and is accepting
// connections; the response status doesn't matter, only that the
// server answered.
func waitForHTTP(ctx context.Context, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1 * time.Second}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout after %s", timeout)
}
