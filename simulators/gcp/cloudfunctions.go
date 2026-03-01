package main

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

	sim "github.com/sockerless/simulator"
)

// Cloud Functions v2 types

// Function represents a Cloud Functions v2 function.
type Function struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	BuildConfig *BuildConfig      `json:"buildConfig,omitempty"`
	ServiceConfig *ServiceConfig  `json:"serviceConfig,omitempty"`
	State       string            `json:"state"`
	CreateTime  string            `json:"createTime"`
	UpdateTime  string            `json:"updateTime"`
	Labels      map[string]string `json:"labels,omitempty"`
	Environment string            `json:"environment,omitempty"`
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
	TimeoutSeconds       int               `json:"timeoutSeconds,omitempty"`
	AvailableMemory      string            `json:"availableMemory,omitempty"`
	MaxInstanceCount     int               `json:"maxInstanceCount,omitempty"`
	MinInstanceCount     int               `json:"minInstanceCount,omitempty"`
	EnvironmentVariables map[string]string `json:"environmentVariables,omitempty"`
	SimCommand           []string          `json:"simCommand,omitempty"` // Simulator-only: command to execute on invoke
}

func registerCloudFunctions(srv *sim.Server) {
	functions := sim.NewStateStore[Function]()

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

		result := functions.Filter(func(fn Function) bool {
			return strings.HasPrefix(fn.Name, prefix)
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

			// Real execution when SimCommand is set
			if fn.ServiceConfig != nil && len(fn.ServiceConfig.SimCommand) > 0 {
				responseBody = invokeCloudFunctionProcess(fn, project, functionID)
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

// invokeCloudFunctionProcess executes a Cloud Function's SimCommand via sim.StartProcess
// and returns the stdout output as the response body.
func invokeCloudFunctionProcess(fn *Function, project, functionID string) []byte {
	cmd := fn.ServiceConfig.SimCommand
	if len(cmd) == 0 {
		return []byte("{}")
	}

	var cmdEnv map[string]string
	if fn.ServiceConfig.EnvironmentVariables != nil {
		cmdEnv = fn.ServiceConfig.EnvironmentVariables
	}

	timeout := 60 * time.Second // GCP default
	if fn.ServiceConfig.TimeoutSeconds > 0 {
		timeout = time.Duration(fn.ServiceConfig.TimeoutSeconds) * time.Second
	}

	sink := &cfLogSink{project: project, functionName: functionID}
	var stdout bytes.Buffer
	collectSink := sim.FuncSink(func(line sim.LogLine) {
		sink.WriteLog(line)
		if line.Stream == "stdout" {
			stdout.WriteString(line.Text)
			stdout.WriteByte('\n')
		}
	})

	handle := sim.StartProcess(sim.ProcessConfig{
		Command: cmd,
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
		return []byte("{}")
	}
	return []byte(output)
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

