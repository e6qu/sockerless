package bleephub

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) registerRunServiceRoutes() {
	// Acquire / renew / complete job requests
	s.mux.HandleFunc("GET /_apis/v1/AgentRequest/{poolId}/{requestId}", s.handleGetRequest)
	s.mux.HandleFunc("PATCH /_apis/v1/AgentRequest/{poolId}/{requestId}", s.handleRenewRequest)
	s.mux.HandleFunc("PUT /_apis/v1/AgentRequest/{poolId}/{requestId}", s.handleRenewRequest)
	s.mux.HandleFunc("DELETE /_apis/v1/AgentRequest/{poolId}/{requestId}", s.handleCompleteRequest)

	// FinishJob (runner reports job completion)
	s.mux.HandleFunc("POST /_apis/v1/FinishJob/{scopeId}/{hubName}/{planId}", s.handleFinishJob)

	// Job events (legacy)
	s.mux.HandleFunc("PUT /_apis/v1/plans/{planId}/events", s.handleJobEvents)

	// CustomerIntelligence (telemetry, accept and discard)
	s.mux.HandleFunc("POST /_apis/v1/tasks", s.handleTelemetry)
}

func (s *Server) handleGetRequest(w http.ResponseWriter, r *http.Request) {
	reqID, err := strconv.ParseInt(r.PathValue("requestId"), 10, 64)
	if err != nil {
		http.Error(w, "invalid request ID", http.StatusBadRequest)
		return
	}

	job := s.lookupJobByRequestID(reqID)
	if job == nil {
		http.Error(w, "request not found", http.StatusNotFound)
		return
	}

	s.logger.Debug().Int64("requestId", reqID).Msg("get request")

	// Return the full job message as the request details
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(job.Message))
}

func (s *Server) handleRenewRequest(w http.ResponseWriter, r *http.Request) {
	reqID, err := strconv.ParseInt(r.PathValue("requestId"), 10, 64)
	if err != nil {
		http.Error(w, "invalid request ID", http.StatusBadRequest)
		return
	}

	job := s.lookupJobByRequestID(reqID)
	if job == nil {
		http.Error(w, "request not found", http.StatusNotFound)
		return
	}

	// Parse request body for status updates
	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)

	s.store.mu.Lock()
	if job.Status == "queued" {
		job.Status = "running"
	}
	job.LockedUntil = time.Now().Add(1 * time.Hour)
	s.store.mu.Unlock()

	s.logger.Info().
		Str("method", r.Method).
		Int64("requestId", reqID).
		Str("status", job.Status).
		Msg("renew/update request")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId":   reqID,
		"lockedUntil": job.LockedUntil.UTC().Format(time.RFC3339),
		"planId":      job.PlanID,
		"jobId":       job.ID,
	})
}

func (s *Server) handleCompleteRequest(w http.ResponseWriter, r *http.Request) {
	reqID, err := strconv.ParseInt(r.PathValue("requestId"), 10, 64)
	if err != nil {
		http.Error(w, "invalid request ID", http.StatusBadRequest)
		return
	}

	result := r.URL.Query().Get("result")

	job := s.lookupJobByRequestID(reqID)
	if job == nil {
		http.Error(w, "request not found", http.StatusNotFound)
		return
	}

	s.store.mu.Lock()
	job.Status = "completed"
	if result != "" {
		job.Result = result
	}
	s.store.mu.Unlock()

	s.logger.Info().
		Int64("requestId", reqID).
		Str("job_id", job.ID).
		Str("result", result).
		Msg("job request completed (DELETE)")

	// Notify workflow engine of job completion
	s.onJobCompleted(r.Context(), job.ID, job.Result)

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleFinishJob(w http.ResponseWriter, r *http.Request) {
	planID := r.PathValue("planId")

	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)

	result, _ := body["result"].(string)
	jobID, _ := body["jobId"].(string)

	s.logger.Info().
		Str("planId", planID).
		Str("jobId", jobID).
		Str("result", result).
		Msg("job finished")

	// Update job status — try both plan ID lookup and job ID lookup
	job := s.lookupJobByPlanID(planID)
	if job == nil && jobID != "" {
		s.store.mu.RLock()
		job = s.store.Jobs[jobID]
		s.store.mu.RUnlock()
	}
	if job != nil {
		s.store.mu.Lock()
		job.Status = "completed"
		if result != "" {
			job.Result = result
		} else {
			job.Result = "Succeeded"
		}
		s.store.mu.Unlock()
		s.logger.Info().Str("jobId", job.ID).Str("result", job.Result).Msg("job status updated")

		// Capture output variables from the runner
		s.captureJobOutputs(job.ID, body)

		// Notify workflow engine of job completion
		s.onJobCompleted(r.Context(), job.ID, job.Result)
	} else {
		s.logger.Warn().Str("planId", planID).Msg("could not find job for finish")
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

// captureJobOutputs resolves output variables from a runner body and stores
// them on the corresponding WorkflowJob.
func (s *Server) captureJobOutputs(jobID string, body map[string]interface{}) {
	outputVars := extractOutputVariables(body)
	if len(outputVars) == 0 {
		return
	}

	s.store.mu.RLock()
	var wfJob *WorkflowJob
	for _, wf := range s.store.Workflows {
		if j, ok := wf.Jobs[""]; ok && j.JobID == jobID {
			wfJob = j
			break
		}
		for _, j := range wf.Jobs {
			if j.JobID == jobID {
				wfJob = j
				break
			}
		}
		if wfJob != nil {
			break
		}
	}
	s.store.mu.RUnlock()

	if wfJob == nil || wfJob.Def == nil {
		return
	}

	resolved := resolveJobOutputs(outputVars, wfJob.Def.Outputs)
	if len(resolved) > 0 {
		for k, v := range resolved {
			wfJob.Outputs[k] = v
		}
		s.logger.Info().
			Str("jobId", jobID).
			Interface("outputs", resolved).
			Msg("job outputs captured")
	}
}

func (s *Server) handleJobEvents(w http.ResponseWriter, r *http.Request) {
	planID := r.PathValue("planId")

	var body map[string]interface{}
	json.NewDecoder(r.Body).Decode(&body)

	eventName, _ := body["name"].(string)
	s.logger.Debug().Str("planId", planID).Str("event", eventName).Msg("job event")

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug().Msg("telemetry event (discarded)")
	w.WriteHeader(http.StatusOK)
}
