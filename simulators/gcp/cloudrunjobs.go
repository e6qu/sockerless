package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// Cloud Run Jobs v2 types

// Job represents a Cloud Run Job resource.
type Job struct {
	Name                   string              `json:"name"`
	UID                    string              `json:"uid"`
	Generation             int64               `json:"generation,string"`
	Labels                 map[string]string   `json:"labels,omitempty"`
	Annotations            map[string]string   `json:"annotations,omitempty"`
	CreateTime             string              `json:"createTime"`
	UpdateTime             string              `json:"updateTime"`
	LaunchStage            string              `json:"launchStage,omitempty"`
	Template               *ExecutionTemplate  `json:"template"`
	TerminalCondition      *Condition          `json:"terminalCondition,omitempty"`
	Conditions             []Condition         `json:"conditions,omitempty"`
	LatestCreatedExecution *ExecutionReference `json:"latestCreatedExecution,omitempty"`
	ExecutionCount         int32               `json:"executionCount"`
	Reconciling            bool                `json:"reconciling"`
}

// ExecutionReference holds a reference to the latest execution of a job.
type ExecutionReference struct {
	Name           string `json:"name"`
	CreateTime     string `json:"createTime"`
	CompletionTime string `json:"completionTime,omitempty"`
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
	Name         string                `json:"name,omitempty"`
	Image        string                `json:"image"`
	Command      []string              `json:"command,omitempty"`
	Args         []string              `json:"args,omitempty"`
	Env          []EnvVar              `json:"env,omitempty"`
	Resources    *ResourceRequirements `json:"resources,omitempty"`
	Ports        []ContainerPort       `json:"ports,omitempty"`
	VolumeMounts []VolumeMount         `json:"volumeMounts,omitempty"`
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

// Volume represents a volume available to containers. Cloud Run's real
// API supports gcs / secret / cloudSqlInstance / nfs sources; we
// implement gcs here — additional sources stay nil-able so
// serialisation round-trips match real API responses.
type Volume struct {
	Name   string              `json:"name"`
	Gcs    *GcsVolumeSource    `json:"gcs,omitempty"`
	Nfs    *NfsVolumeSource    `json:"nfs,omitempty"`
	Secret *SecretVolumeSource `json:"secret,omitempty"`
}

// GcsVolumeSource mirrors google.cloud.run.v2.GCSVolumeSource.
type GcsVolumeSource struct {
	Bucket       string   `json:"bucket"`
	ReadOnly     bool     `json:"readOnly,omitempty"`
	MountOptions []string `json:"mountOptions,omitempty"`
}

// NfsVolumeSource mirrors google.cloud.run.v2.NFSVolumeSource.
type NfsVolumeSource struct {
	Server   string `json:"server"`
	Path     string `json:"path"`
	ReadOnly bool   `json:"readOnly,omitempty"`
}

// SecretVolumeSource mirrors google.cloud.run.v2.SecretVolumeSource.
type SecretVolumeSource struct {
	Secret      string `json:"secret"`
	DefaultMode int32  `json:"defaultMode,omitempty"`
}

// VolumeMount represents a volume mount in a container.
type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
}

// Execution represents a Cloud Run Job execution.
type Execution struct {
	Name           string            `json:"name"`
	UID            string            `json:"uid"`
	Generation     int64             `json:"generation,string"`
	Labels         map[string]string `json:"labels,omitempty"`
	CreateTime     string            `json:"createTime"`
	StartTime      string            `json:"startTime,omitempty"`
	CompletionTime string            `json:"completionTime,omitempty"`
	RunningCount   int32             `json:"runningCount"`
	SucceededCount int32             `json:"succeededCount"`
	FailedCount    int32             `json:"failedCount"`
	CancelledCount int32             `json:"cancelledCount"`
	Conditions     []Condition       `json:"conditions,omitempty"`
	TaskCount      int32             `json:"taskCount"`
	Template       *TaskTemplate     `json:"template,omitempty"`
	Reconciling    bool              `json:"reconciling"`
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
	Name     string          `json:"name"`
	Metadata map[string]any  `json:"metadata,omitempty"`
	Done     bool            `json:"done"`
	Response any             `json:"response,omitempty"`
	Error    *OperationError `json:"error,omitempty"`
}

// OperationError represents an error from a long-running operation.
type OperationError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
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

	// Derive target from the resource's name field if available
	var target string
	if responseMap != nil {
		if n, ok := responseMap["name"].(string); ok {
			target = n
		}
	}

	return Operation{
		Name: fmt.Sprintf("projects/%s/locations/%s/operations/%s", project, location, opID),
		Metadata: map[string]any{
			"createTime": nowTimestamp(),
			"target":     target,
		},
		Done:     true,
		Response: responseMap,
	}
}

// Container handle tracker for Cloud Run Jobs real execution
var crjProcessHandles sync.Map // map[execName]*sim.ContainerHandle

// Package-level stores for dashboard access.
var crjJobs sim.Store[Job]

func registerCloudRunJobs(srv *sim.Server) {
	jobs := sim.MakeStore[Job](srv.DB(), "crj_jobs")
	executions := sim.MakeStore[Execution](srv.DB(), "crj_executions")
	crjJobs = jobs

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

		// Also delete associated executions
		execs := executions.Filter(func(e Execution) bool {
			return strings.HasPrefix(e.Name, name+"/executions/")
		})
		for _, e := range execs {
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

		// Auto-complete execution after task timeout or process exit
		go func(id string, tc int32, proj, job string, taskTmpl *TaskTemplate) {
			timeout := 600 * time.Second // GCP default
			if taskTmpl != nil && taskTmpl.Timeout != "" {
				if d, err := time.ParseDuration(taskTmpl.Timeout); err == nil {
					timeout = d
				}
			}

			// Build container config from first container in template
			var image string
			var entrypoint, args []string
			var cmdEnv map[string]string
			var binds []string
			if taskTmpl != nil && len(taskTmpl.Containers) > 0 {
				c := taskTmpl.Containers[0]
				image = c.Image
				entrypoint = c.Command
				args = c.Args
				if len(c.Env) > 0 {
					cmdEnv = make(map[string]string, len(c.Env))
					for _, ev := range c.Env {
						cmdEnv[ev.Name] = ev.Value
					}
				}
				// Translate GCS-backed Volume + VolumeMount pairs to real
				// host bind mounts. The GCS slice backs each bucket with
				// a host directory under $SIM_GCS_DATA_DIR so Cloud Run
				// tasks launched by this sim see real persistent files.
				if taskTmpl.Volumes != nil && len(c.VolumeMounts) > 0 {
					volByName := make(map[string]Volume, len(taskTmpl.Volumes))
					for _, v := range taskTmpl.Volumes {
						volByName[v.Name] = v
					}
					for _, mp := range c.VolumeMounts {
						v, ok := volByName[mp.Name]
						if !ok || v.Gcs == nil || v.Gcs.Bucket == "" {
							continue
						}
						bind := GCSBucketHostDir(v.Gcs.Bucket) + ":" + mp.MountPath
						if v.Gcs.ReadOnly {
							bind += ":ro"
						}
						binds = append(binds, bind)
					}
				}
			}

			succeeded := true
			if image != "" {
				// Real container execution
				sink := &crjLogSink{project: proj, jobName: job}
				execShort := id
				if parts := strings.Split(id, "/"); len(parts) > 0 {
					last := parts[len(parts)-1]
					if len(last) > 12 {
						execShort = last[:12]
					} else {
						execShort = last
					}
				}
				localImage := sim.ResolveLocalImage(image)
				handle, err := sim.StartContainerSync(sim.ContainerConfig{
					Image:   localImage,
					Command: entrypoint,
					Args:    args,
					Env:     cmdEnv,
					Timeout: timeout,
					Name:    fmt.Sprintf("sockerless-sim-gcp-job-%s", execShort),
					Labels:  map[string]string{"sockerless-sim-execution": id},
					Binds:   binds,
				}, sink)
				if err != nil {
					fmt.Fprintf(os.Stderr, "ERROR: failed to start container for execution: image=%s err=%v\n", image, err)
					succeeded = false
				} else {
					crjProcessHandles.Store(id, handle)
					result := handle.Wait()
					crjProcessHandles.Delete(id)
					succeeded = result.ExitCode == 0
				}
			}

			completed := false
			executions.Update(id, func(e *Execution) {
				if e.RunningCount == 0 {
					return
				}
				completed = true
				completionTime := nowTimestamp()
				e.CompletionTime = completionTime
				e.RunningCount = 0
				if succeeded {
					e.SucceededCount = tc
				} else {
					e.FailedCount = tc
				}
				state := "CONDITION_SUCCEEDED"
				reason := ""
				if !succeeded {
					state = "CONDITION_FAILED"
					reason = "NonZeroExitCode"
				}
				e.Conditions = []Condition{
					{Type: "Ready", State: state, LastTransitionTime: completionTime, Reason: reason},
					{Type: "Completed", State: state, LastTransitionTime: completionTime, Reason: reason},
				}
				e.Reconciling = false
			})
			if completed {
				// Update the job's latestCreatedExecution with completion time
				if jobKey, _, ok := strings.Cut(id, "/executions/"); ok {
					jobs.Update(jobKey, func(j *Job) {
						if j.LatestCreatedExecution != nil && j.LatestCreatedExecution.Name == id {
							j.LatestCreatedExecution.CompletionTime = nowTimestamp()
						}
					})
				}
				// Match the actual outcome (the previous behaviour
				// always injected "Execution completed successfully"
				// regardless of `succeeded`, masking failed jobs as
				// fake-success in the log stream and breaking tests
				// like TestCloudRunArithmeticInvalid that assert on
				// the failure marker).
				if succeeded {
					injectCloudRunJobLog(proj, job, "Execution completed successfully")
				} else {
					injectCloudRunJobLog(proj, job, "Execution failed")
				}
			}
		}(execName, taskCount, project, jobID, tmpl)

		// Update job with execution count and latest execution reference
		jobs.Update(name, func(j *Job) {
			j.ExecutionCount++
			j.UpdateTime = now
			j.LatestCreatedExecution = &ExecutionReference{
				Name:       execName,
				CreateTime: now,
			}
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

		// Cancel running container if any
		if v, ok := crjProcessHandles.LoadAndDelete(name); ok {
			handle := v.(*sim.ContainerHandle)
			sim.StopContainer(handle.ContainerID)
			handle.Cancel()
		}

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
			// Update the job's latestCreatedExecution with completion time
			jobName := fmt.Sprintf("projects/%s/locations/%s/jobs/%s", project, location, jobID)
			jobs.Update(jobName, func(j *Job) {
				if j.LatestCreatedExecution != nil && j.LatestCreatedExecution.Name == name {
					j.LatestCreatedExecution.CompletionTime = nowTimestamp()
				}
			})
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

// crjLogSink implements sim.LogSink and writes log lines to Cloud Logging.
type crjLogSink struct {
	project string
	jobName string
}

func (s *crjLogSink) WriteLog(line sim.LogLine) {
	injectCloudRunJobLog(s.project, s.jobName, line.Text)
}
