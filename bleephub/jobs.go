package bleephub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func (s *Server) registerJobRoutes() {
	s.mux.HandleFunc("POST /api/v3/bleephub/submit", s.handleSubmitJob)
	s.mux.HandleFunc("GET /api/v3/bleephub/jobs/{jobId}", s.handleGetJobStatus)

	// Workflow YAML submission
	s.mux.HandleFunc("POST /api/v3/bleephub/workflow", s.handleSubmitWorkflow)
	s.mux.HandleFunc("GET /api/v3/bleephub/workflows/{workflowId}", s.handleGetWorkflowStatus)

	// ActionDownloadInfo — runner requests download URLs for actions (handler in actions.go)
	s.mux.HandleFunc("POST /_apis/v1/ActionDownloadInfo/{scopeId}/{hubName}/{planId}", s.handleActionDownloadInfo)

	// Tasks endpoint (runner may request task definitions)
	s.mux.HandleFunc("GET /_apis/v1/tasks/{taskId}/{versionString}", s.handleGetTask)
}

// SubmitRequest is the simplified job submission format.
type SubmitRequest struct {
	Image string       `json:"image"`
	Steps []SubmitStep `json:"steps"`
}

// SubmitStep is a simplified step.
type SubmitStep struct {
	Run string `json:"run"`
}

func (s *Server) handleSubmitJob(w http.ResponseWriter, r *http.Request) {
	var req SubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Image == "" {
		req.Image = "alpine:latest"
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	serverURL := scheme + "://" + r.Host

	jobID := uuid.New().String()
	planID := uuid.New().String()
	timelineID := uuid.New().String()
	requestID := s.nextRequestID()

	msg := buildJobMessage(serverURL, jobID, planID, timelineID, requestID, &req)
	msgJSON, _ := json.Marshal(msg)

	job := &Job{
		ID:          jobID,
		RequestID:   requestID,
		PlanID:      planID,
		TimelineID:  timelineID,
		Status:      "queued",
		Message:     string(msgJSON),
		LockedUntil: time.Now().Add(1 * time.Hour),
	}

	s.store.mu.Lock()
	s.store.Jobs[jobID] = job
	s.store.mu.Unlock()

	// Build the envelope message
	envelope := &TaskAgentMessage{
		MessageID:   s.nextMessageID(),
		MessageType: "PipelineAgentJobRequest",
		Body:        string(msgJSON),
	}

	if !s.sendMessageToAgent(envelope) {
		s.logger.Warn().Str("jobId", jobID).Msg("no runner available, job queued")
	}

	s.logger.Info().Str("jobId", jobID).Int64("requestId", requestID).Msg("job submitted")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"jobId":     jobID,
		"requestId": requestID,
		"status":    "queued",
	})
}

func (s *Server) handleGetJobStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")

	s.store.mu.RLock()
	job, ok := s.store.Jobs[jobID]
	s.store.mu.RUnlock()

	if !ok {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"jobId":  job.ID,
		"status": job.Status,
		"result": job.Result,
	})
}

// WorkflowSubmitRequest is the workflow YAML submission format.
type WorkflowSubmitRequest struct {
	Workflow string `json:"workflow"` // raw YAML
	Image    string `json:"image"`    // default container image
}

func (s *Server) handleSubmitWorkflow(w http.ResponseWriter, r *http.Request) {
	var req WorkflowSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Workflow == "" {
		http.Error(w, "workflow YAML required", http.StatusBadRequest)
		return
	}

	wfDef, err := ParseWorkflow([]byte(req.Workflow))
	if err != nil {
		http.Error(w, "parse workflow: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Image == "" {
		req.Image = "alpine:latest"
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	serverURL := scheme + "://" + r.Host

	// Expand matrix strategies
	expandedDef := expandMatrixJobs(wfDef)

	// Store serverURL for re-dispatch after job completion
	if expandedDef.Env == nil {
		expandedDef.Env = make(map[string]string)
	}
	expandedDef.Env["__serverURL"] = serverURL
	expandedDef.Env["__defaultImage"] = req.Image

	workflow, err := s.submitWorkflow(serverURL, expandedDef, req.Image)
	if err != nil {
		http.Error(w, "submit: "+err.Error(), http.StatusBadRequest)
		return
	}

	s.logger.Info().
		Str("workflowId", workflow.ID).
		Int("jobs", len(workflow.Jobs)).
		Msg("workflow submitted")

	// Build response with job info
	jobs := make(map[string]interface{}, len(workflow.Jobs))
	for key, wfJob := range workflow.Jobs {
		jobs[key] = map[string]interface{}{
			"jobId":  wfJob.JobID,
			"status": wfJob.Status,
			"name":   wfJob.DisplayName,
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"workflowId": workflow.ID,
		"jobs":       jobs,
		"status":     workflow.Status,
	})
}

func (s *Server) handleGetWorkflowStatus(w http.ResponseWriter, r *http.Request) {
	wfID := r.PathValue("workflowId")

	s.store.mu.RLock()
	wf, ok := s.store.Workflows[wfID]
	s.store.mu.RUnlock()

	if !ok {
		http.Error(w, "workflow not found", http.StatusNotFound)
		return
	}

	jobs := make(map[string]interface{}, len(wf.Jobs))
	for key, wfJob := range wf.Jobs {
		jobs[key] = map[string]interface{}{
			"jobId":  wfJob.JobID,
			"status": wfJob.Status,
			"result": wfJob.Result,
			"name":   wfJob.DisplayName,
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"workflowId": wf.ID,
		"status":     wf.Status,
		"result":     wf.Result,
		"jobs":       jobs,
	})
}

// expandMatrixJobs expands matrix strategies in a WorkflowDef, creating
// multiple job entries per matrix combination.
func expandMatrixJobs(wf *WorkflowDef) *WorkflowDef {
	expanded := &WorkflowDef{
		Name: wf.Name,
		Env:  wf.Env,
		Jobs: make(map[string]*JobDef),
	}

	for key, jd := range wf.Jobs {
		if jd.Strategy == nil || len(jd.Strategy.Matrix.Values) == 0 {
			expanded.Jobs[key] = jd
			continue
		}

		combos := ExpandMatrix(&jd.Strategy.Matrix)
		if len(combos) == 0 {
			expanded.Jobs[key] = jd
			continue
		}

		for i, combo := range combos {
			newKey := fmt.Sprintf("%s_%d", key, i)
			newJD := *jd // shallow copy
			newJD.Name = MatrixJobName(key, combo)
			// Store matrix values in a way the workflow engine can use
			// We'll use a convention: the expanded jobs get the same needs
			// but their own key
			expanded.Jobs[newKey] = &newJD

			// We need to track matrix values — stash them so submitWorkflow can set them.
			// Use a special env prefix since MatrixValues lives on WorkflowJob.
			if newJD.Env == nil {
				newJD.Env = make(map[string]string)
			}
			for mk, mv := range combo {
				newJD.Env["__matrix_"+mk] = fmt.Sprintf("%v", mv)
			}
		}

		// Update needs references: any job that depends on the original key
		// should depend on ALL expanded keys
		expandedKeys := make([]string, 0, len(combos))
		for i := range combos {
			expandedKeys = append(expandedKeys, fmt.Sprintf("%s_%d", key, i))
		}
		for _, otherJD := range expanded.Jobs {
			newNeeds := make([]string, 0, len(otherJD.Needs))
			for _, dep := range otherJD.Needs {
				if dep == key {
					newNeeds = append(newNeeds, expandedKeys...)
				} else {
					newNeeds = append(newNeeds, dep)
				}
			}
			otherJD.Needs = newNeeds
		}
	}

	return expanded
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("taskId")
	s.logger.Debug().Str("taskId", taskID).Msg("task definition requested")
	http.Error(w, "task not found", http.StatusNotFound)
}

// buildJobMessage builds the AgentJobRequestMessage in the Azure DevOps format
// that the official GitHub Actions runner expects.
// Format matches ChristopherHX/runner.server's PipelineContextData + TemplateToken serialization.
func buildJobMessage(serverURL, jobID, planID, timelineID string, requestID int64, req *SubmitRequest) map[string]interface{} {
	scopeID := uuid.New().String()

	// Build steps — only user-defined steps. The runner adds setup/cleanup internally.
	steps := make([]map[string]interface{}, 0, len(req.Steps))

	for i, step := range req.Steps {
		stepID := uuid.New().String()
		displayName := fmt.Sprintf("Run %s", truncateDisplay(step.Run, 40))
		// Step type "action" (ActionStepType.Action) with ScriptReference
		// Inputs must be TemplateToken MappingToken: {"type":2,"map":[{"Key":k,"Value":v},...]}
		contextName := fmt.Sprintf("__run_%d", i+1)
		steps = append(steps, map[string]interface{}{
			"type": "action",
			"id":   stepID,
			"name": contextName,
			"reference": map[string]interface{}{
				"type": "script",
			},
			"displayNameToken": displayName,
			"contextName":      contextName,
			"condition":        "success()",
			"inputs": map[string]interface{}{
				"type": 2,
				"map": []interface{}{
					map[string]interface{}{
						"Key":   map[string]interface{}{"type": 0, "lit": "script"},
						"Value": map[string]interface{}{"type": 0, "lit": step.Run},
					},
				},
			},
		})
	}

	// Generate a proper JWT for the job token
	jobToken := makeJWT(scopeID, "actions")

	// Build the full message matching runner.server format
	return map[string]interface{}{
		"messageType": "PipelineAgentJobRequest",
		"plan": map[string]interface{}{
			"scopeIdentifier": scopeID,
			"planId":          planID,
			"planType":        "free",
			"planGroup":       "free",
			"version":         12,
			"owner": map[string]interface{}{
				"id":   0,
				"name": "Community",
			},
		},
		"timeline": map[string]interface{}{
			"id":       timelineID,
			"changeId": 1,
			"location": nil,
		},
		"jobId":          jobID,
		"jobDisplayName": "test",
		"jobName":        "test",
		"requestId":      requestID,
		"lockedUntil":    "0001-01-01T00:00:00",
		// jobContainer: bare string for simple image reference
		"jobContainer":         req.Image,
		"jobServiceContainers": nil,
		"jobOutputs":           nil,
		"resources": map[string]interface{}{
			"endpoints": []map[string]interface{}{
				{
					"name": "SystemVssConnection",
					"url":  serverURL + "/",
					"authorization": map[string]interface{}{
						"scheme": "OAuth",
						"parameters": map[string]string{
							"AccessToken": jobToken,
						},
					},
					"data": map[string]string{
						"CacheServerUrl":    serverURL + "/",
						"ResultsServiceUrl": serverURL + "/",
					},
					"isShared": false,
					"isReady":  true,
				},
			},
			"repositories": []interface{}{},
			"containers":   []interface{}{},
		},
		// contextData uses PipelineContextData format:
		// String values = bare JSON strings
		// Dictionary = {"t": 2, "d": [{"k":"key","v":<value>}, ...]}
		"contextData": map[string]interface{}{
			"github": dictContextData(
				"server_url", serverURL,
				"api_url", serverURL,
				"repository", "bleephub/test",
				"repository_owner", "bleephub",
				"run_id", "1",
				"run_number", "1",
				"workflow", "test",
				"job", "test",
				"event_name", "push",
				"sha", "0000000000000000000000000000000000000000",
				"ref", "refs/heads/main",
				"action", "__run",
				"workspace", "/github/workspace",
				"token", jobToken,
			),
			"runner": dictContextData(
				"os", "Linux",
				"arch", "ARM64",
				"name", "test-runner",
				"tool_cache", "/opt/hostedtoolcache",
				"temp", "/home/runner/work/_temp",
			),
			"env":      dictContextData(),
			"vars":     dictContextData(),
			"needs":    dictContextData(),
			"inputs":   nil,
			"matrix":   nil,
			"strategy": nil,
		},
		"variables": map[string]interface{}{
			"system.github.job":                        varVal("test"),
			"system.github.runid":                      varVal("1"),
			"system.github.token":                      varSecret(jobToken),
			"github_token":                             varSecret(jobToken),
			"system.phaseDisplayName":                  varVal("test"),
			"system.runnerGroupName":                   varVal("Default"),
			"DistributedTask.NewActionMetadata":        varVal("true"),
			"DistributedTask.EnableCompositeActions":   varVal("true"),
		},
		"mask":                  []interface{}{},
		"steps":                 steps,
		"workspace":             map[string]interface{}{},
		"defaults":              nil,
		"environmentVariables":  nil,
		"actionsEnvironment":    nil,
		"fileTable":             []string{".github/workflows/test.yml"},
	}
}

// dictContextData builds a PipelineContextData DictionaryContextData.
// Args are alternating key, value strings.
func dictContextData(kvs ...string) map[string]interface{} {
	entries := make([]map[string]interface{}, 0, len(kvs)/2)
	for i := 0; i+1 < len(kvs); i += 2 {
		entries = append(entries, map[string]interface{}{
			"k": kvs[i],
			"v": kvs[i+1], // String values are bare JSON strings
		})
	}
	return map[string]interface{}{
		"t": 2,
		"d": entries,
	}
}

func varVal(value string) map[string]interface{} {
	return map[string]interface{}{
		"value":    value,
		"isSecret": false,
	}
}

func varSecret(value string) map[string]interface{} {
	return map[string]interface{}{
		"value":    value,
		"isSecret": true,
	}
}

func truncateDisplay(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
