package bleephub

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) registerRunServiceRoutes() {
	// Acquire / renew / complete job requests
	s.route("GET /_apis/v1/AgentRequest/{poolId}/{requestId}", s.handleGetRequest)
	s.route("PATCH /_apis/v1/AgentRequest/{poolId}/{requestId}", s.handleRenewRequest)
	s.route("PUT /_apis/v1/AgentRequest/{poolId}/{requestId}", s.handleRenewRequest)
	s.route("DELETE /_apis/v1/AgentRequest/{poolId}/{requestId}", s.handleCompleteRequest)

	// FinishJob (runner reports job completion)
	s.route("POST /_apis/v1/FinishJob/{scopeId}/{hubName}/{planId}", s.handleFinishJob)

	// Job events (legacy)
	s.route("PUT /_apis/v1/plans/{planId}/events", s.handleJobEvents)

	// CustomerIntelligence (telemetry, accept and discard)
	s.route("POST /_apis/v1/tasks", s.handleTelemetry)
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

	// The body would carry status fields but bleephub doesn't consume
	// them today; drain explicitly so it's obvious there's no decode.
	_, _ = io.Copy(io.Discard, r.Body)

	s.store.mu.Lock()
	startedRunning := false
	if job.Status == "queued" {
		job.Status = "running"
		startedRunning = true
	}
	job.LockedUntil = time.Now().Add(1 * time.Hour)
	// Mirror the runner pickup onto the workflow job: the jobs API and
	// the checks layer report in_progress from this moment.
	if startedRunning {
		for _, wf := range s.store.Workflows {
			if wfJob, ok := findWorkflowJobByID(wf, job.ID); ok {
				if wfJob.Status == JobStatusQueued {
					wfJob.Status = JobStatusRunning
					s.queueActionsEvent(evJobInProgress, wf, wfJob)
				}
				break
			}
		}
	}
	// Snapshot the fields read below while the lock is held; other
	// goroutines (the broker, completion) mutate job.Status/LockedUntil.
	jobStatusSnap := job.Status
	lockedUntilSnap := job.LockedUntil
	jobPlanID := job.PlanID
	jobIDSnap := job.ID
	s.store.mu.Unlock()

	s.logger.Info().
		Str("method", r.Method).
		Int64("requestId", reqID).
		Str("status", jobStatusSnap).
		Msg("renew/update request")

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requestId":   reqID,
		"lockedUntil": lockedUntilSnap.UTC().Format(time.RFC3339),
		"planId":      jobPlanID,
		"jobId":       jobIDSnap,
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
	// Snapshot under the lock: job fields are concurrently written by the
	// broker (e.g. recordJobAgentLocked sets AgentID), so reads after the
	// unlock must use locals, not the shared *Job.
	jobIDSnap := job.ID
	jobResultSnap := job.Result
	s.store.mu.Unlock()

	s.logger.Info().
		Int64("requestId", reqID).
		Str("job_id", jobIDSnap).
		Str("result", result).
		Msg("job request completed (DELETE)")

	// Notify workflow engine of job completion
	s.onJobCompleted(r.Context(), jobIDSnap, jobResultSnap)

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleFinishJob(w http.ResponseWriter, r *http.Request) {
	planID := r.PathValue("planId")

	var body map[string]interface{}
	if !decodeJSONBody(w, r, &body) {
		return
	}

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
		// Snapshot under the lock: the broker concurrently mutates these
		// fields (recordJobAgentLocked writes AgentID), so every read after
		// the unlock must come from a local, not the shared *Job.
		jobIDSnap := job.ID
		jobResultSnap := job.Result
		jobAgentSnap := job.AgentID
		s.store.mu.Unlock()
		s.logger.Info().Str("jobId", jobIDSnap).Str("result", jobResultSnap).Msg("job status updated")

		// Capture output variables from the runner
		s.captureJobOutputs(jobIDSnap, body)

		// Notify workflow engine of job completion
		s.onJobCompleted(r.Context(), jobIDSnap, jobResultSnap)

		// Ephemeral runners exist for exactly one job — real GitHub
		// auto-deregisters them after it completes (the dispatcher's
		// one-runner-per-job model depends on the registration not
		// lingering as an offline zombie).
		s.removeEphemeralAgent(jobAgentSnap)
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

	s.store.mu.Lock()
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

	if wfJob == nil || wfJob.Def == nil {
		s.store.mu.Unlock()
		return
	}

	resolved := resolveJobOutputs(outputVars, wfJob.Def.Outputs)
	for k, v := range resolved {
		wfJob.Outputs[k] = v
	}
	s.store.mu.Unlock()

	if len(resolved) > 0 {
		s.logger.Info().
			Str("jobId", jobID).
			Interface("outputs", resolved).
			Msg("job outputs captured")
	}
}

// findWorkflowJobByID scans a workflow's jobs for the given engine job
// UUID. Callers hold the store lock.
func findWorkflowJobByID(wf *Workflow, jobID string) (*WorkflowJob, bool) {
	for _, wfJob := range wf.Jobs {
		if wfJob.JobID == jobID {
			return wfJob, true
		}
	}
	return nil, false
}

func (s *Server) handleJobEvents(w http.ResponseWriter, r *http.Request) {
	planID := r.PathValue("planId")

	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid job event body: "+err.Error(), http.StatusBadRequest)
		return
	}

	eventName, _ := body["name"].(string)
	s.logger.Debug().Str("planId", planID).Str("event", eventName).Msg("job event")

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug().Msg("telemetry event (discarded)")
	w.WriteHeader(http.StatusOK)
}
