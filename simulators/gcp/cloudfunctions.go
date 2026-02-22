package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"

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
}

// Agent subprocess tracker
var gcfAgentProcs sync.Map // map[functionName]*exec.Cmd

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

		// Start agent subprocess if the function has a callback URL configured
		if fn != nil {
			if callbackURL := gcfGetAgentCallbackURL(*fn); callbackURL != "" {
				gcfStartAgentProcess(functionID, callbackURL)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
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

		// Kill any agent subprocess for this function
		gcfStopAgentProcess(functionID)

		lro := newLRO(project, location, fn, "type.googleapis.com/google.cloud.functions.v2.Function")
		sim.WriteJSON(w, http.StatusOK, lro)
	})
}

// gcfGetAgentCallbackURL extracts the agent callback URL from the function's
// environment variables, if present.
func gcfGetAgentCallbackURL(fn Function) string {
	if fn.ServiceConfig == nil || fn.ServiceConfig.EnvironmentVariables == nil {
		return ""
	}
	return fn.ServiceConfig.EnvironmentVariables["SOCKERLESS_AGENT_CALLBACK_URL"]
}

func gcfStartAgentProcess(key, callbackURL string) {
	if _, loaded := gcfAgentProcs.Load(key); loaded {
		return
	}

	agentBin, err := exec.LookPath("sockerless-agent")
	if err != nil {
		log.Printf("[gcf] agent binary not found in PATH, skipping agent start for %s", key)
		return
	}

	cmd := exec.Command(agentBin, "--callback", callbackURL, "--keep-alive", "--", "tail", "-f", "/dev/null")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Printf("[gcf] failed to start agent for %s: %v", key, err)
		return
	}

	log.Printf("[gcf] started agent subprocess for %s (pid=%d)", key, cmd.Process.Pid)
	gcfAgentProcs.Store(key, cmd)

	go func() {
		_ = cmd.Wait()
		gcfAgentProcs.Delete(key)
	}()
}

func gcfStopAgentProcess(key string) {
	v, ok := gcfAgentProcs.LoadAndDelete(key)
	if !ok {
		return
	}
	cmd := v.(*exec.Cmd)
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
		log.Printf("[gcf] stopped agent subprocess for %s", key)
	}
}
