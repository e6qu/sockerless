package main

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	sim "github.com/sockerless/simulator"
)

// Process handle tracker for Container Apps Jobs real execution
var acaProcessHandles sync.Map // map[execID]*sim.ProcessHandle

// ContainerAppJob represents an Azure Container Apps Job resource.
type ContainerAppJob struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Location   string            `json:"location"`
	Tags       map[string]string `json:"tags,omitempty"`
	Properties JobProperties     `json:"properties"`
}

// JobProperties holds the properties of a Container Apps Job.
type JobProperties struct {
	ProvisioningState   string            `json:"provisioningState"`
	EnvironmentID       string            `json:"environmentId,omitempty"`
	WorkloadProfileName string            `json:"workloadProfileName,omitempty"`
	Configuration       *JobConfiguration `json:"configuration,omitempty"`
	Template            *JobTemplate      `json:"template,omitempty"`
}

// JobConfiguration holds the configuration of a Container Apps Job.
type JobConfiguration struct {
	ReplicaTimeout    int               `json:"replicaTimeout,omitempty"`
	ReplicaRetryLimit int               `json:"replicaRetryLimit,omitempty"`
	TriggerType       string            `json:"triggerType,omitempty"`
	Secrets           []JobSecret       `json:"secrets,omitempty"`
	Registries        []JobRegistry     `json:"registries,omitempty"`
	ManualTrigger     *ManualTrigger    `json:"manualTriggerConfig,omitempty"`
	ScheduleTrigger   *ScheduleTrigger  `json:"scheduleTriggerConfig,omitempty"`
	EventTrigger      *EventTrigger     `json:"eventTriggerConfig,omitempty"`
}

// ManualTrigger holds manual trigger configuration.
type ManualTrigger struct {
	Parallelism            int `json:"parallelism,omitempty"`
	ReplicaCompletionCount int `json:"replicaCompletionCount,omitempty"`
}

// ScheduleTrigger holds schedule trigger configuration.
type ScheduleTrigger struct {
	CronExpression         string `json:"cronExpression,omitempty"`
	Parallelism            int    `json:"parallelism,omitempty"`
	ReplicaCompletionCount int    `json:"replicaCompletionCount,omitempty"`
}

// EventTrigger holds event trigger configuration.
type EventTrigger struct {
	Parallelism            int `json:"parallelism,omitempty"`
	ReplicaCompletionCount int `json:"replicaCompletionCount,omitempty"`
}

// JobSecret holds a secret reference for a Container Apps Job.
type JobSecret struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	Identity    string `json:"identity,omitempty"`
	KeyVaultURL string `json:"keyVaultUrl,omitempty"`
}

// JobRegistry holds a container registry reference for a Container Apps Job.
type JobRegistry struct {
	Server            string `json:"server"`
	Username          string `json:"username,omitempty"`
	PasswordSecretRef string `json:"passwordSecretRef,omitempty"`
	Identity          string `json:"identity,omitempty"`
}

// JobTemplate holds the template of a Container Apps Job.
type JobTemplate struct {
	Containers     []JobContainer `json:"containers,omitempty"`
	InitContainers []JobContainer `json:"initContainers,omitempty"`
	Volumes        []JobVolume    `json:"volumes,omitempty"`
}

// JobContainer holds a container definition for a Container Apps Job.
type JobContainer struct {
	Name         string               `json:"name"`
	Image        string               `json:"image"`
	Command      []string             `json:"command,omitempty"`
	Args         []string             `json:"args,omitempty"`
	Env          []EnvVar             `json:"env,omitempty"`
	Resources    *ResourceRequirements `json:"resources,omitempty"`
	VolumeMounts []VolumeMount        `json:"volumeMounts,omitempty"`
}

// EnvVar holds an environment variable.
type EnvVar struct {
	Name      string `json:"name"`
	Value     string `json:"value,omitempty"`
	SecretRef string `json:"secretRef,omitempty"`
}

// ResourceRequirements holds resource requirements for a container.
type ResourceRequirements struct {
	CPU              float64 `json:"cpu,omitempty"`
	Memory           string  `json:"memory,omitempty"`
	EphemeralStorage string  `json:"ephemeralStorage,omitempty"`
}

// VolumeMount holds a volume mount for a container.
type VolumeMount struct {
	VolumeName string `json:"volumeName"`
	MountPath  string `json:"mountPath"`
	SubPath    string `json:"subPath,omitempty"`
}

// JobVolume holds a volume definition for a Container Apps Job.
type JobVolume struct {
	Name        string `json:"name"`
	StorageType string `json:"storageType,omitempty"`
	StorageName string `json:"storageName,omitempty"`
}

// JobExecution represents a running or completed execution of a Container Apps Job.
// Uses flat format matching the Azure SDK's expected JSON structure.
type JobExecution struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Type      string       `json:"type,omitempty"`
	Status    string       `json:"status"`
	StartTime string       `json:"startTime"`
	EndTime   string       `json:"endTime,omitempty"`
	Template  *JobTemplate `json:"template,omitempty"`
}

// Package-level stores for dashboard access.
var acaJobs *sim.StateStore[ContainerAppJob]
var acaExecutions *sim.StateStore[JobExecution]

func registerContainerApps(srv *sim.Server) {
	jobs := sim.NewStateStore[ContainerAppJob]()
	executions := sim.NewStateStore[JobExecution]()
	acaJobs = jobs
	acaExecutions = executions

	const basePath = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.App"

	// PUT - Create or update job
	srv.HandleFunc("PUT "+basePath+"/jobs/{jobName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "jobName")

		var req ContainerAppJob
		if err := sim.ReadJSON(r, &req); err != nil {
			sim.AzureError(w, "InvalidRequestContent", "Failed to parse request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.Location == "" {
			sim.AzureError(w, "InvalidRequestContent", "The 'location' property is required.", http.StatusBadRequest)
			return
		}

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/jobs/%s", sub, rg, name)

		_, exists := jobs.Get(resourceID)

		job := ContainerAppJob{
			ID:       resourceID,
			Name:     name,
			Type:     "Microsoft.App/jobs",
			Location: req.Location,
			Tags:     req.Tags,
			Properties: JobProperties{
				ProvisioningState:   "Succeeded",
				EnvironmentID:       req.Properties.EnvironmentID,
				WorkloadProfileName: req.Properties.WorkloadProfileName,
				Configuration:       req.Properties.Configuration,
				Template:            req.Properties.Template,
			},
		}

		if job.Properties.Configuration != nil && job.Properties.Configuration.TriggerType == "" {
			job.Properties.Configuration.TriggerType = "Manual"
		}

		jobs.Put(resourceID, job)

		if exists {
			sim.WriteJSON(w, http.StatusOK, job)
		} else {
			sim.WriteJSON(w, http.StatusCreated, job)
		}
	})

	// GET - Get job
	srv.HandleFunc("GET "+basePath+"/jobs/{jobName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "jobName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/jobs/%s", sub, rg, name)

		job, ok := jobs.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.App/jobs/%s' under resource group '%s' was not found.", name, rg)
			return
		}

		sim.WriteJSON(w, http.StatusOK, job)
	})

	// GET - List jobs
	srv.HandleFunc("GET "+basePath+"/jobs", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		prefix := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/jobs/", sub, rg)

		filtered := jobs.Filter(func(j ContainerAppJob) bool {
			return strings.HasPrefix(j.ID, prefix)
		})

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"value": filtered,
		})
	})

	// DELETE - Delete job
	srv.HandleFunc("DELETE "+basePath+"/jobs/{jobName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "jobName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/jobs/%s", sub, rg, name)

		if jobs.Delete(resourceID) {
			// Also delete associated executions
			execs := executions.Filter(func(e JobExecution) bool {
				return strings.HasPrefix(e.ID, resourceID+"/executions/")
			})
			for _, e := range execs {
				executions.Delete(e.ID)
			}
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	})

	// POST - Start execution
	srv.HandleFunc("POST "+basePath+"/jobs/{jobName}/start", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "jobName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/jobs/%s", sub, rg, name)

		job, ok := jobs.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.App/jobs/%s' under resource group '%s' was not found.", name, rg)
			return
		}

		// Read optional override template
		var override struct {
			Containers     []JobContainer `json:"containers,omitempty"`
			InitContainers []JobContainer `json:"initContainers,omitempty"`
		}
		_ = sim.ReadJSON(r, &override)

		execName := fmt.Sprintf("%s-%s", name, randomSuffix(7))
		execID := fmt.Sprintf("%s/executions/%s", resourceID, execName)

		template := job.Properties.Template
		if len(override.Containers) > 0 {
			template = &JobTemplate{
				Containers:     override.Containers,
				InitContainers: override.InitContainers,
			}
		}

		exec := JobExecution{
			ID:        execID,
			Name:      execName,
			Type:      "Microsoft.App/jobs/executions",
			Status:    "Running",
			StartTime: time.Now().UTC().Format(time.RFC3339),
			Template:  template,
		}

		executions.Put(execID, exec)

		// Inject log entry for execution start
		injectContainerAppLog(name, "Container started")

		// Auto-stop execution after replica timeout or process exit
		replicaTimeout := 0
		if job.Properties.Configuration != nil {
			replicaTimeout = job.Properties.Configuration.ReplicaTimeout
		}
		go func(id, jobShortName string, replicaTimeout int, tmpl *JobTemplate) {
			timeout := 1800 * time.Second // Azure default
			if replicaTimeout > 0 {
				timeout = time.Duration(replicaTimeout) * time.Second
			}

			// Build command from first container
			var fullCmd []string
			var cmdEnv map[string]string
			if tmpl != nil {
				for _, c := range tmpl.Containers {
					fullCmd = append(fullCmd, c.Command...)
					fullCmd = append(fullCmd, c.Args...)
					if len(c.Env) > 0 {
						cmdEnv = make(map[string]string, len(c.Env))
						for _, ev := range c.Env {
							cmdEnv[ev.Name] = ev.Value
						}
					}
					break // first container only
				}
			}

			succeeded := true
			if len(fullCmd) > 0 {
				// Real process execution
				sink := &acaLogSink{jobName: jobShortName}
				handle := sim.StartProcess(sim.ProcessConfig{
					Command: fullCmd,
					Env:     cmdEnv,
					Timeout: timeout,
				}, sink)
				acaProcessHandles.Store(id, handle)
				result := handle.Wait()
				acaProcessHandles.Delete(id)
				succeeded = result.ExitCode == 0
			} else {
				// No command — sleep for timeout (preserves current behavior)
				time.Sleep(timeout)
			}

			completed := false
			executions.Update(id, func(e *JobExecution) {
				if e.Status != "Running" {
					return
				}
				completed = true
				if succeeded {
					e.Status = "Succeeded"
				} else {
					e.Status = "Failed"
				}
				e.EndTime = time.Now().UTC().Format(time.RFC3339)
			})
			if completed {
				injectContainerAppLog(jobShortName, "Execution completed successfully")
			}
		}(execID, name, replicaTimeout, template)

		// Return 202 with Location header for LRO polling.
		// The Azure SDK's BeginStart uses FinalStateViaLocation,
		// so it polls the Location URL to get the final result.
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		execURL := fmt.Sprintf("%s://%s%s?api-version=%s",
			scheme, r.Host, execID, r.URL.Query().Get("api-version"))
		w.Header().Set("Location", execURL)
		sim.WriteJSON(w, http.StatusAccepted, map[string]string{
			"name": execName,
			"id":   execID,
		})
	})

	// GET - List executions
	srv.HandleFunc("GET "+basePath+"/jobs/{jobName}/executions", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		name := sim.PathParam(r, "jobName")

		resourceID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/jobs/%s", sub, rg, name)

		_, ok := jobs.Get(resourceID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The Resource 'Microsoft.App/jobs/%s' under resource group '%s' was not found.", name, rg)
			return
		}

		filtered := executions.Filter(func(e JobExecution) bool {
			return strings.HasPrefix(e.ID, resourceID+"/executions/")
		})

		sim.WriteJSON(w, http.StatusOK, map[string]any{
			"value": filtered,
		})
	})

	// GET - Get execution
	srv.HandleFunc("GET "+basePath+"/jobs/{jobName}/executions/{execName}", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		jobName := sim.PathParam(r, "jobName")
		execName := sim.PathParam(r, "execName")

		execID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/jobs/%s/executions/%s",
			sub, rg, jobName, execName)

		exec, ok := executions.Get(execID)
		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The execution '%s' for job '%s' was not found.", execName, jobName)
			return
		}

		sim.WriteJSON(w, http.StatusOK, exec)
	})

	// POST - Stop execution
	srv.HandleFunc("POST "+basePath+"/jobs/{jobName}/executions/{execName}/stop", func(w http.ResponseWriter, r *http.Request) {
		sub := sim.PathParam(r, "subscriptionId")
		rg := sim.PathParam(r, "resourceGroupName")
		jobName := sim.PathParam(r, "jobName")
		execName := sim.PathParam(r, "execName")

		execID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/jobs/%s/executions/%s",
			sub, rg, jobName, execName)

		// Cancel running process if any
		if v, ok := acaProcessHandles.LoadAndDelete(execID); ok {
			v.(*sim.ProcessHandle).Cancel()
		}

		ok := executions.Update(execID, func(e *JobExecution) {
			e.Status = "Stopped"
			e.EndTime = time.Now().UTC().Format(time.RFC3339)
		})
		if ok {
			injectContainerAppLog(jobName, "Execution stopped")
		}

		if !ok {
			sim.AzureErrorf(w, "ResourceNotFound", http.StatusNotFound,
				"The execution '%s' for job '%s' was not found.", execName, jobName)
			return
		}

		w.WriteHeader(http.StatusOK)
	})
}

// randomSuffix generates a random alphanumeric suffix of the given length.
func randomSuffix(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	rand.Read(b)
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b)
}

// acaLogSink implements sim.LogSink and writes log lines to Log Analytics.
type acaLogSink struct {
	jobName string
}

func (s *acaLogSink) WriteLog(line sim.LogLine) {
	injectContainerAppLog(s.jobName, line.Text)
}
