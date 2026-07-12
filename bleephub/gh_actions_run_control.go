package bleephub

// Workflow-run control extras: fork-PR run approval, force-cancel,
// single-job re-run, custom deployment protection rule reviews,
// per-attempt log download, and per-workflow-file timing. Each endpoint
// drives the real workflow engine state machine (dispatch, cancel,
// re-run attempts) the same way the sibling run endpoints do.

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) registerGHActionsRunControlRoutes() {
	s.route("POST /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/approve",
		s.requirePerm(scopeActions, permWrite, s.handleApproveWorkflowRun))
	s.route("POST /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/force-cancel",
		s.requirePerm(scopeActions, permWrite, s.handleForceCancelWorkflowRun))
	s.route("POST /api/v3/repos/{owner}/{repo}/actions/jobs/{job_id}/rerun",
		s.requirePerm(scopeActions, permWrite, s.handleRerunWorkflowJob))
	s.route("POST /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/deployment_protection_rule",
		s.requirePerm(scopeActions, permWrite, s.handleReviewCustomDeploymentProtectionRule))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/attempts/{attempt_number}/logs",
		s.handleRunAttemptLogs)
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/workflows/{workflow_id}/timing",
		s.handleWorkflowFileTiming)
}

// handleApproveWorkflowRun — POST .../runs/{run_id}/approve.
// Releases a run held in action_required by the fork-PR contributor
// approval gate: the run re-enters concurrency admission and its jobs
// dispatch. A run that isn't waiting for approval is refused (403),
// matching real GitHub.
func (s *Server) handleApproveWorkflowRun(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.Atoi(r.PathValue("run_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	wf := s.findWorkflowByRunIDInRepo(runID, repoFullName(r))
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.store.mu.Lock()
	if wf.Status != WorkflowStatusActionRequired {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusForbidden, "This workflow run is not waiting for approval")
		return
	}
	// Concurrency admission, deferred from submit time by the approval
	// gate: an active run in the same group either loses its lease
	// (cancel-in-progress) or queues this run behind it.
	var activeWf *Workflow
	if wf.ConcurrencyGroup != "" {
		for _, existing := range s.store.Workflows {
			if existing.ID != wf.ID && existing.ConcurrencyGroup == wf.ConcurrencyGroup &&
				existing.Status == WorkflowStatusRunning {
				activeWf = existing
				break
			}
		}
	}
	if activeWf != nil && !wf.CancelInProgress {
		wf.Status = WorkflowStatusPendingConcurrency
		s.store.persistWorkflowRecord(wf)
		s.store.mu.Unlock()
		writeJSON(w, http.StatusCreated, map[string]any{})
		return
	}
	wf.Status = WorkflowStatusRunning
	if wf.ConcurrencyGroup != "" {
		wf.ConcurrencyAcquiredAt = time.Now().UTC()
	}
	serverURL := ""
	defaultImage := ""
	if wf.Env != nil {
		serverURL = wf.Env["__serverURL"]
		defaultImage = wf.Env["__defaultImage"]
	}
	s.store.persistWorkflowRecord(wf)
	s.store.mu.Unlock()

	if activeWf != nil && wf.CancelInProgress {
		s.cancelWorkflow(activeWf)
	}
	s.startTimeoutWatcher(wf)
	if serverURL != "" {
		s.dispatchReadyJobs(r.Context(), wf, serverURL, defaultImage)
	}
	writeJSON(w, http.StatusCreated, map[string]any{})
}

// handleForceCancelWorkflowRun — POST .../runs/{run_id}/force-cancel.
// Cancels the run bypassing conditions that would otherwise let it
// continue (always()/cancelled() jobs, in-flight runners): every
// non-terminal job completes as cancelled immediately, running jobs are
// signalled, and the run finalizes with conclusion cancelled.
func (s *Server) handleForceCancelWorkflowRun(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.Atoi(r.PathValue("run_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	wf := s.findWorkflowByRunIDInRepo(runID, repoFullName(r))
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.mu.RLock()
	completed := wf.Status == WorkflowStatusCompleted
	s.store.mu.RUnlock()
	if completed {
		writeGHError(w, http.StatusConflict, "Cannot force cancel a workflow run that is completed.")
		return
	}
	s.forceCancelWorkflow(wf)
	writeJSON(w, http.StatusAccepted, map[string]any{})
}

// forceCancelWorkflow terminates every non-terminal job immediately —
// unlike cancelWorkflow it does not leave always()/cancelled() jobs
// eligible for dispatch and does not wait for runners to report back;
// running jobs are still signalled so their runners abort.
func (s *Server) forceCancelWorkflow(wf *Workflow) {
	s.store.mu.Lock()
	wf.CancelRequested = true
	cancelledJobIDs := map[string]bool{}
	var runningJobIDs []string
	for _, wfJob := range wf.Jobs {
		if wfJob.Status == JobStatusCompleted || wfJob.Status == JobStatusSkipped {
			continue
		}
		if job := s.store.Jobs[wfJob.JobID]; job != nil && job.AgentID != 0 && job.Status != "completed" {
			runningJobIDs = append(runningJobIDs, wfJob.JobID)
		}
		wfJob.Status = JobStatusCompleted
		wfJob.Result = ResultCancelled
		wfJob.CompletedAt = time.Now()
		cancelledJobIDs[wfJob.JobID] = true
		s.queueActionsEvent(evJobCompleted, wf, wfJob)
	}
	if len(cancelledJobIDs) > 0 {
		kept := s.store.PendingMessages[:0]
		for _, msg := range s.store.PendingMessages {
			if !cancelledJobIDs[msg.JobID] {
				kept = append(kept, msg)
			}
		}
		s.store.PendingMessages = kept
	}
	s.store.persistWorkflowRecord(wf)
	s.store.mu.Unlock()

	for _, jobID := range runningJobIDs {
		s.sendJobCancellation(jobID)
	}
	s.logger.Info().
		Str("workflow_id", wf.ID).
		Str("workflow_name", wf.Name).
		Int("signalled_running", len(runningJobIDs)).
		Msg("workflow force-cancellation requested")
	s.finalizeWorkflowIfDone(wf)
}

// handleRerunWorkflowJob — POST .../actions/jobs/{job_id}/rerun.
// Re-runs one job (and everything that depends on it) as a new run
// attempt; every other job carries its previous result over, exactly
// like rerun-failed-jobs carries successful jobs.
func (s *Server) handleRerunWorkflowJob(w http.ResponseWriter, r *http.Request) {
	jobID, err := strconv.ParseInt(r.PathValue("job_id"), 10, 64)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	wf, target := s.findJobByStableIDInRepo(jobID, repoFullName(r))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	// The request body ({enable_debug_logging}) is optional and nullable.
	var body struct {
		EnableDebugLogging bool `json:"enable_debug_logging"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	s.store.mu.RLock()
	inProgress := wf.Status != WorkflowStatusCompleted
	s.store.mu.RUnlock()
	if inProgress {
		writeGHError(w, http.StatusForbidden, "This workflow run is still in progress and its jobs cannot be re-run")
		return
	}

	repo := wf.RepoFullName
	if repo == "" {
		repo = repoFullName(r)
	}
	match, err := s.cachedWorkflowFileForRun(repo, wf)
	if err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	def, perr := ParseWorkflow([]byte(match.YAML))
	if perr != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "parse cached YAML: "+perr.Error())
		return
	}
	def = expandMatrixJobs(def)
	if def.Env == nil {
		def.Env = map[string]string{}
	}
	serverURL := s.baseURL(r)
	def.Env["__serverURL"] = serverURL
	def.Env["__defaultImage"] = ""

	// Carry over every job except the target and its transitive
	// dependents (they must re-run because their inputs change).
	rerunKeys := dependentJobKeys(wf, target.Key)
	carryOver := map[string]*WorkflowJob{}
	s.store.mu.RLock()
	for key, j := range wf.Jobs {
		if !rerunKeys[key] {
			carryOver[key] = j
		}
	}
	s.store.mu.RUnlock()

	if err := s.rerunWorkflowAsNewAttempt(r, wf, match, def, serverURL, carryOver); err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "rerun submit: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{})
}

// dependentJobKeys returns the target job key plus every job that
// transitively depends on it.
func dependentJobKeys(wf *Workflow, targetKey string) map[string]bool {
	out := map[string]bool{targetKey: true}
	for changed := true; changed; {
		changed = false
		for key, j := range wf.Jobs {
			if out[key] {
				continue
			}
			for _, dep := range j.Needs {
				if out[dep] {
					out[key] = true
					changed = true
					break
				}
			}
		}
	}
	return out
}

// handleReviewCustomDeploymentProtectionRule — POST
// .../runs/{run_id}/deployment_protection_rule. A protection-rule
// reviewer approves or rejects the run's pending deployment to the
// named environment (releasing or failing the waiting jobs), or leaves
// a comment without deciding (state omitted → recorded as pending).
func (s *Server) handleReviewCustomDeploymentProtectionRule(w http.ResponseWriter, r *http.Request) {
	_, wf := s.lookupRunFromPath(r)
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var body struct {
		EnvironmentName string `json:"environment_name"`
		State           string `json:"state"`
		Comment         string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Invalid request body")
		return
	}
	if body.EnvironmentName == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "environment_name is required")
		return
	}
	switch body.State {
	case "approved", "rejected":
	case "":
		if body.Comment == "" {
			writeGHError(w, http.StatusUnprocessableEntity, "comment is required when state is not provided")
			return
		}
	default:
		writeGHError(w, http.StatusUnprocessableEntity, "state must be approved or rejected")
		return
	}

	s.store.mu.RLock()
	envID := 0
	for _, p := range wf.PendingDeployments {
		if p.EnvName == body.EnvironmentName {
			envID = p.EnvID
			break
		}
	}
	s.store.mu.RUnlock()
	if envID == 0 {
		writeGHError(w, http.StatusUnprocessableEntity,
			"environment "+body.EnvironmentName+" has no pending deployment for this run")
		return
	}

	reviewer := ghUserFromContext(r.Context())
	if body.State == "" {
		// Comment-only review: recorded against the run without
		// resolving the pending deployment.
		reviewerID := 0
		if reviewer != nil {
			reviewerID = reviewer.ID
		}
		s.store.mu.Lock()
		wf.EnvApprovals = append(wf.EnvApprovals, &EnvApproval{
			State:     "pending",
			Comment:   body.Comment,
			UserID:    reviewerID,
			EnvIDs:    []int{envID},
			EnvNames:  []string{body.EnvironmentName},
			CreatedAt: time.Now().UTC(),
		})
		s.store.persistWorkflowRecord(wf)
		s.store.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.applyDeploymentReview(r.Context(), wf, []int{envID}, body.State, body.Comment, reviewer)
	w.WriteHeader(http.StatusNoContent)
}

// handleRunAttemptLogs — GET .../runs/{run_id}/attempts/{n}/logs.
// Serves the resolved attempt's log archive in the same layout as the
// run-level logs endpoint (real GitHub redirects to a signed URL;
// bleephub returns the zip directly, matching .../runs/{run_id}/logs).
func (s *Server) handleRunAttemptLogs(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.Atoi(r.PathValue("run_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	attempt, err := strconv.Atoi(r.PathValue("attempt_number"))
	if err != nil || attempt < 1 {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	wf := s.findRunAttempt(runID, attempt, repoFullName(r))
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.writeRunLogsZip(r.Context(), w, wf, runID)
}

// handleWorkflowFileTiming — GET .../actions/workflows/{workflow_id}/timing.
// Computes the workflow file's billable usage from the run history:
// the summed job durations of every run (including archived attempts)
// produced from the file. Bleephub jobs run on Linux runners, so the
// usage accrues under UBUNTU.
func (s *Server) handleWorkflowFileTiming(w http.ResponseWriter, r *http.Request) {
	repo := repoFullName(r)
	s.store.DiscoverWorkflowFilesFromGit(repo)
	file := s.resolveWorkflowFile(repo, r.PathValue("workflow_id"))
	if file == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var totalMs int64
	sumRun := func(wf *Workflow) {
		if wf.RepoFullName != "" && wf.RepoFullName != repo {
			return
		}
		if wf.Name != file.Name {
			return
		}
		for _, j := range wf.Jobs {
			if j.StartedAt.IsZero() {
				continue
			}
			end := j.CompletedAt
			if end.IsZero() {
				end = time.Now()
			}
			totalMs += end.Sub(j.StartedAt).Milliseconds()
		}
	}
	s.store.mu.RLock()
	for _, wf := range s.store.Workflows {
		sumRun(wf)
	}
	for _, attempts := range s.store.WorkflowAttempts {
		for _, wf := range attempts {
			sumRun(wf)
		}
	}
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"billable": map[string]any{
			"UBUNTU": map[string]any{
				"total_ms": totalMs,
			},
		},
	})
}
