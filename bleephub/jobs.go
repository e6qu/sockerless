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

	// ActionDownloadInfo — runner requests download URLs for actions
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

func (s *Server) handleActionDownloadInfo(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug().Msg("action download info requested")
	// Return empty actions — run: steps don't need action downloads
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"actions": map[string]interface{}{},
	})
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
