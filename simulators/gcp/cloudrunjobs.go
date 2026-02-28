package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// Cloud Run Jobs v2 types

// Job represents a Cloud Run Job resource.
type Job struct {
	Name               string             `json:"name"`
	UID                string             `json:"uid"`
	Generation         int64              `json:"generation,string"`
	Labels             map[string]string  `json:"labels,omitempty"`
	Annotations        map[string]string  `json:"annotations,omitempty"`
	CreateTime         string             `json:"createTime"`
	UpdateTime         string             `json:"updateTime"`
	LaunchStage        string             `json:"launchStage,omitempty"`
	Template           *ExecutionTemplate `json:"template"`
	TerminalCondition  *Condition         `json:"terminalCondition,omitempty"`
	Conditions         []Condition        `json:"conditions,omitempty"`
	ExecutionCount     int32              `json:"executionCount"`
	Reconciling        bool               `json:"reconciling"`
}

// ExecutionTemplate holds the template for creating executions.
type ExecutionTemplate struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Parallelism int32             `json:"parallelism"`
	TaskCount   int32             `json:"taskCount"`
	Template    *TaskTemplate     `json:"template"`
}

// TaskTemplate holds the template for creating tasks within an execution.
type TaskTemplate struct {
	Containers     []Container `json:"containers,omitempty"`
	Volumes        []Volume    `json:"volumes,omitempty"`
	MaxRetries     int32       `json:"maxRetries"`
	Timeout        string      `json:"timeout,omitempty"`
	ServiceAccount string      `json:"serviceAccount,omitempty"`
}

// Container represents a container within a task.
type Container struct {
	Name         string               `json:"name,omitempty"`
	Image        string               `json:"image"`
	Command      []string             `json:"command,omitempty"`
	Args         []string             `json:"args,omitempty"`
	Env          []EnvVar             `json:"env,omitempty"`
	Resources    *ResourceRequirements `json:"resources,omitempty"`
	Ports        []ContainerPort      `json:"ports,omitempty"`
	VolumeMounts []VolumeMount        `json:"volumeMounts,omitempty"`
}

// EnvVar represents an environment variable.
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ResourceRequirements describes compute resource requirements.
type ResourceRequirements struct {
	Limits map[string]string `json:"limits,omitempty"`
}

// ContainerPort represents a port on a container.
type ContainerPort struct {
	Name          string `json:"name,omitempty"`
	ContainerPort int32  `json:"containerPort"`
}

// Volume represents a volume available to containers.
type Volume struct {
	Name string `json:"name"`
}

// VolumeMount represents a volume mount in a container.
type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
}

// Execution represents a Cloud Run Job execution.
type Execution struct {
	Name           string             `json:"name"`
	UID            string             `json:"uid"`
	Generation     int64              `json:"generation,string"`
	Labels         map[string]string  `json:"labels,omitempty"`
	CreateTime     string             `json:"createTime"`
	StartTime      string             `json:"startTime,omitempty"`
	CompletionTime string             `json:"completionTime,omitempty"`
	RunningCount   int32              `json:"runningCount"`
	SucceededCount int32              `json:"succeededCount"`
	FailedCount    int32              `json:"failedCount"`
	CancelledCount int32             `json:"cancelledCount"`
	Conditions     []Condition        `json:"conditions,omitempty"`
	TaskCount      int32              `json:"taskCount"`
	Template       *TaskTemplate      `json:"template,omitempty"`
	Reconciling    bool               `json:"reconciling"`
}

// Condition represents a status condition on a resource.
type Condition struct {
	Type               string `json:"type"`
	State              string `json:"state"`
	Message            string `json:"message,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

// Operation represents a long-running operation.
type Operation struct {
	Name     string         `json:"name"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Done     bool           `json:"done"`
	Response any            `json:"response,omitempty"`
	Error    *OperationError `json:"error,omitempty"`
}

// OperationError represents an error from a long-running operation.
type OperationError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func newLRO(project, location string, resource any, typeName string) Operation {
	opID := generateUUID()
	// Convert resource to a map and add @type for protobuf Any compatibility.
	// GCP REST clients expect the response field to be a google.protobuf.Any
	// which requires @type in the JSON representation.
	var responseMap map[string]any
	if resource != nil {
		data, _ := json.Marshal(resource)
		json.Unmarshal(data, &responseMap)
		responseMap["@type"] = typeName
	} else {
		responseMap = map[string]any{"@type": typeName}
	}
	return Operation{
		Name: fmt.Sprintf("projects/%s/locations/%s/operations/%s", project, location, opID),
		Metadata: map[string]any{
			"@type": typeName,
		},
		Done:     true,
		Response: responseMap,
	}
}

// Agent subprocess tracker for Cloud Run Jobs
var crjAgentProcs sync.Map // map[execName]*exec.Cmd

func registerCloudRunJobs(srv *sim.Server) {
	jobs := sim.NewStateStore[Job]()
	executions := sim.NewStateStore[Execution]()

	// Create job
	srv.HandleFunc("POST /v2/projects/{project}/locations/{location}/jobs", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		jobID := r.URL.Query().Get("jobId")
		if jobID == "" {
			sim.GCPError(w, http.StatusBadRequest, "jobId query parameter is required", "INVALID_ARGUMENT")
			return
		}

		var job Job
		if err := sim.ReadJSON(r, &job); err != nil {
			sim.GCPErrorf(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: %v", err)
			return
		}

		name := fmt.Sprintf("projects/%s/locations/%s/jobs/%s", project, location, jobID)
		if _, exists := jobs.Get(name); exists {
			sim.GCPErrorf(w, http.StatusConflict, "ALREADY_EXISTS", "job %q already exists", name)
			return
		}

		now := nowTimestamp()
		job.Name = name
		job.UID = generateUUID()
		job.Generation = 1
		job.CreateTime = now
		job.UpdateTime = now
		if job.LaunchStage == "" {
			job.LaunchStage = "GA"
		}
		job.TerminalCondition = &Condition{
			Type:               "Ready",
			State:              "CONDITION_SUCCEEDED",
			LastTransitionTime: now,
		}
		job.Conditions = []Condition{
			{Type: "Ready", State: "CONDITION_SUCCEEDED", LastTransitionTime: now},
		}
		job.Reconciling = false

		// Set defaults on template
		if job.Template != nil {
			if job.Template.Parallelism == 0 {
				job.Template.Parallelism = 1
			}
			if job.Template.TaskCount == 0 {
				job.Template.TaskCount = 1
			}
		}

		jobs.Put(name, job)

		lro := newLRO(project, location, job, "type.googleapis.com/google.cloud.run.v2.Job")
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// Get job
	srv.HandleFunc("GET /v2/projects/{project}/locations/{location}/jobs/{job}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		jobID := sim.PathParam(r, "job")

		// Reject if the path has extra segments (executions)
		if strings.Contains(r.URL.Path, "/executions") {
			return // let the executions handler deal with it
		}

		name := fmt.Sprintf("projects/%s/locations/%s/jobs/%s", project, location, jobID)
		job, ok := jobs.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "job %q not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, job)
	})

	// List jobs
	srv.HandleFunc("GET /v2/projects/{project}/locations/{location}/jobs", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		prefix := fmt.Sprintf("projects/%s/locations/%s/jobs/", project, location)

		result := jobs.Filter(func(j Job) bool {
			return strings.HasPrefix(j.Name, prefix)
		})

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"jobs": result,
		})
	})

	// Delete job
	srv.HandleFunc("DELETE /v2/projects/{project}/locations/{location}/jobs/{job}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		jobID := sim.PathParam(r, "job")
		name := fmt.Sprintf("projects/%s/locations/%s/jobs/%s", project, location, jobID)

		job, ok := jobs.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "job %q not found", name)
			return
		}

		jobs.Delete(name)

		// Also delete associated executions (and stop any agents)
		execs := executions.Filter(func(e Execution) bool {
			return strings.HasPrefix(e.Name, name+"/executions/")
		})
		for _, e := range execs {
			crjStopAgentProcess(&crjAgentProcs, e.Name)
			executions.Delete(e.Name)
		}

		lro := newLRO(project, location, job, "type.googleapis.com/google.cloud.run.v2.Job")
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// Run job (create execution)
	srv.HandleFunc("POST /v2/projects/{project}/locations/{location}/jobs/{jobAction}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		jobAction := sim.PathParam(r, "jobAction")
		jobID, _, _ := strings.Cut(jobAction, ":")
		name := fmt.Sprintf("projects/%s/locations/%s/jobs/%s", project, location, jobID)

		job, ok := jobs.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "job %q not found", name)
			return
		}

		now := nowTimestamp()
		execID := generateUUID()
		execName := fmt.Sprintf("%s/executions/%s", name, execID)

		var taskCount int32 = 1
		var tmpl *TaskTemplate
		if job.Template != nil {
			taskCount = job.Template.TaskCount
			if taskCount == 0 {
				taskCount = 1
			}
			tmpl = job.Template.Template
		}

		exec := Execution{
			Name:         execName,
			UID:          generateUUID(),
			Generation:   1,
			Labels:       job.Labels,
			CreateTime:   now,
			StartTime:    now,
			RunningCount: taskCount,
			TaskCount:    taskCount,
			Template:     tmpl,
			Conditions: []Condition{
				{Type: "Ready", State: "CONDITION_PENDING", LastTransitionTime: now},
			},
			Reconciling: true,
		}

		executions.Put(execName, exec)

		// Inject log entries for the execution
		injectCloudRunJobLog(project, jobID, "Container started")

		// Start agent subprocess if a container has a callback URL configured
		callbackURL := crjGetAgentCallbackURL(job)
		if callbackURL != "" {
			crjStartAgentProcess(&crjAgentProcs, execName, callbackURL)
		}

		// Auto-complete execution after task timeout (only if no agent)
		go func(id string, tc int32, hasAgent bool, proj, job string, taskTmpl *TaskTemplate) {
			if hasAgent {
				// Agent-managed: don't auto-complete. Backend will cancel when done.
				return
			}
			timeout := 600 * time.Second // GCP default
			if taskTmpl != nil && taskTmpl.Timeout != "" {
				if d, err := time.ParseDuration(taskTmpl.Timeout); err == nil {
					timeout = d
				}
			}
			time.Sleep(timeout)
			completed := false
			executions.Update(id, func(e *Execution) {
				if e.RunningCount == 0 {
					return
				}
				completed = true
				completionTime := nowTimestamp()
				e.CompletionTime = completionTime
				e.RunningCount = 0
				e.SucceededCount = tc
				e.Conditions = []Condition{
					{Type: "Ready", State: "CONDITION_SUCCEEDED", LastTransitionTime: completionTime},
					{Type: "Completed", State: "CONDITION_SUCCEEDED", LastTransitionTime: completionTime},
				}
				e.Reconciling = false
			})
			if completed {
				injectCloudRunJobLog(proj, job, "Execution completed successfully")
			}
		}(execName, taskCount, callbackURL != "", project, jobID, tmpl)

		// Increment execution count on the job
		jobs.Update(name, func(j *Job) {
			j.ExecutionCount++
			j.UpdateTime = now
		})

		lro := newLRO(project, location, exec, "type.googleapis.com/google.cloud.run.v2.Execution")
		sim.WriteJSON(w, http.StatusOK, lro)
	})

	// Get execution
	srv.HandleFunc("GET /v2/projects/{project}/locations/{location}/jobs/{job}/executions/{execution}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		jobID := sim.PathParam(r, "job")
		execID := sim.PathParam(r, "execution")
		name := fmt.Sprintf("projects/%s/locations/%s/jobs/%s/executions/%s", project, location, jobID, execID)

		exec, ok := executions.Get(name)
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "execution %q not found", name)
			return
		}
		sim.WriteJSON(w, http.StatusOK, exec)
	})

	// List executions
	srv.HandleFunc("GET /v2/projects/{project}/locations/{location}/jobs/{job}/executions", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		jobID := sim.PathParam(r, "job")
		prefix := fmt.Sprintf("projects/%s/locations/%s/jobs/%s/executions/", project, location, jobID)

		result := executions.Filter(func(e Execution) bool {
			return strings.HasPrefix(e.Name, prefix)
		})

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"executions": result,
		})
	})

	// Cancel execution (note: also used for stop by the backend)
	srv.HandleFunc("POST /v2/projects/{project}/locations/{location}/jobs/{job}/executions/{execAction}", func(w http.ResponseWriter, r *http.Request) {
		project := sim.PathParam(r, "project")
		location := sim.PathParam(r, "location")
		jobID := sim.PathParam(r, "job")
		execAction := sim.PathParam(r, "execAction")
		execID, _, _ := strings.Cut(execAction, ":")
		name := fmt.Sprintf("projects/%s/locations/%s/jobs/%s/executions/%s", project, location, jobID, execID)

		// Stop agent subprocess if running
		crjStopAgentProcess(&crjAgentProcs, name)

		ok := executions.Update(name, func(e *Execution) {
			now := nowTimestamp()
			e.CompletionTime = now
			e.CancelledCount = e.RunningCount
			e.RunningCount = 0
			e.Conditions = []Condition{
				{Type: "Ready", State: "CONDITION_SUCCEEDED", LastTransitionTime: now},
				{Type: "Completed", State: "CONDITION_SUCCEEDED", LastTransitionTime: now, Reason: "Cancelled"},
			}
			e.Reconciling = false
		})
		if ok {
			injectCloudRunJobLog(project, jobID, "Execution cancelled")
		}
		if !ok {
			sim.GCPErrorf(w, http.StatusNotFound, "NOT_FOUND", "execution %q not found", name)
			return
		}

		exec, _ := executions.Get(name)
		lro := newLRO(project, location, exec, "type.googleapis.com/google.cloud.run.v2.Execution")
		sim.WriteJSON(w, http.StatusOK, lro)
	})
}

// injectCloudRunJobLog writes a log entry to the Cloud Logging store for a
// Cloud Run Job execution, using the same resource type and labels that the
// backend's log filter expects.
func injectCloudRunJobLog(project, jobName, text string) {
	logName := fmt.Sprintf("projects/%s/logs/run.googleapis.com%%2Fstdout", project)
	writeLogEntries(logName, &MonitoredResource{
		Type:   "cloud_run_job",
		Labels: map[string]string{"job_name": jobName},
	}, nil, []LogEntry{{TextPayload: text}})
}

// crjGetAgentCallbackURL extracts the agent callback URL from the job's
// container environment variables, if present.
func crjGetAgentCallbackURL(job Job) string {
	if job.Template == nil || job.Template.Template == nil {
		return ""
	}
	for _, c := range job.Template.Template.Containers {
		for _, env := range c.Env {
			if env.Name == "SOCKERLESS_AGENT_CALLBACK_URL" {
				return env.Value
			}
		}
	}
	return ""
}

// crjStartAgentProcess starts a sockerless-agent subprocess that dials back to the
// backend at callbackURL. The subprocess is tracked in procs for later cleanup.
func crjStartAgentProcess(procs *sync.Map, key, callbackURL string) {
	if _, loaded := procs.Load(key); loaded {
		return
	}

	agentBin, err := exec.LookPath("sockerless-agent")
	if err != nil {
		log.Printf("[cloudrun-jobs] agent binary not found in PATH, skipping agent start for %s", key)
		return
	}

	cmd := exec.Command(agentBin, "--callback", callbackURL, "--keep-alive", "--", "tail", "-f", "/dev/null")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Printf("[cloudrun-jobs] failed to start agent for %s: %v", key, err)
		return
	}

	log.Printf("[cloudrun-jobs] started agent subprocess for %s (pid=%d)", key, cmd.Process.Pid)
	procs.Store(key, cmd)

	go func() {
		_ = cmd.Wait()
		procs.Delete(key)
	}()
}

// crjStopAgentProcess kills an agent subprocess for the given key.
func crjStopAgentProcess(procs *sync.Map, key string) {
	v, ok := procs.LoadAndDelete(key)
	if !ok {
		return
	}
	cmd := v.(*exec.Cmd)
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
		log.Printf("[cloudrun-jobs] stopped agent subprocess for %s", key)
	}
}
