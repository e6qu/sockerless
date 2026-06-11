package bleephub

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// WorkflowStatus is the lifecycle state of a [Workflow].
type WorkflowStatus string

const (
	WorkflowStatusRunning            WorkflowStatus = "running"
	WorkflowStatusCompleted          WorkflowStatus = "completed"
	WorkflowStatusPendingConcurrency WorkflowStatus = "pending_concurrency"
	// WorkflowStatusWaiting holds runs whose environment-targeting jobs
	// await a deployment review (required reviewers on the environment).
	WorkflowStatusWaiting WorkflowStatus = "waiting"
)

// JobStatus is the lifecycle state of a [WorkflowJob].
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusSkipped   JobStatus = "skipped"
	// JobStatusWaiting holds jobs targeting a reviewer-protected
	// environment until the run's pending deployment is approved.
	JobStatusWaiting JobStatus = "waiting"
)

// Result is the terminal outcome of a workflow or job. The empty value
// means in-flight (no outcome yet).
type Result string

const (
	ResultNone      Result = ""
	ResultSuccess   Result = "success"
	ResultFailure   Result = "failure"
	ResultCancelled Result = "cancelled"
	ResultSkipped   Result = "skipped"
)

// Workflow represents a running multi-job workflow.
type Workflow struct {
	ID        string                  `json:"id"`
	Name      string                  `json:"name"`
	RunID     int                     `json:"runId"`
	RunNumber int                     `json:"runNumber"`
	Jobs      map[string]*WorkflowJob `json:"jobs"`
	Env       map[string]string       `json:"env,omitempty"`
	Status    WorkflowStatus          `json:"status"` // "running", "completed", "pending_concurrency"
	// PendingDeployments holds one record per reviewer-protected
	// environment the run is waiting on; EnvApprovals records every
	// approve/reject review submitted for the run.
	PendingDeployments []*PendingDeployment `json:"pendingDeployments,omitempty"`
	EnvApprovals       []*EnvApproval       `json:"envApprovals,omitempty"`
	Result             Result               `json:"result"` // "success", "failure", "cancelled"
	CreatedAt          time.Time            `json:"createdAt"`
	MaxParallel        int                  `json:"-"` // per-matrix-group limit
	cancelTimeout      func()               // stops the timeout watcher goroutine
	EventName          string               `json:"eventName,omitempty"`
	Ref                string               `json:"ref,omitempty"`
	Sha                string               `json:"sha,omitempty"`
	RepoFullName       string               `json:"repoFullName,omitempty"`
	Inputs             map[string]string    `json:"inputs,omitempty"`
	ConcurrencyGroup   string               `json:"concurrencyGroup,omitempty"`
	CancelInProgress   bool                 `json:"-"`

	// WorkflowFileID / WorkflowFilePath identify the originating workflow
	// FILE (the YAML on disk), which is stable across every run produced
	// from it. GitHub's WorkflowRun.workflow_id and .path reference the
	// file, not the run, so these must be carried separately from RunID.
	// Populated at submit/dispatch time by resolving the registered
	// [WorkflowFile] for (repo, name); zero/"" when no backing file is
	// known yet (resolved lazily in workflowRunJSON).
	WorkflowFileID   int64  `json:"workflowFileId,omitempty"`
	WorkflowFilePath string `json:"workflowFilePath,omitempty"`
}

// WorkflowJob represents a single job within a workflow.
type WorkflowJob struct {
	Key             string                 `json:"key"`   // YAML key
	JobID           string                 `json:"jobId"` // UUID, used as Job.ID
	DisplayName     string                 `json:"displayName"`
	Needs           []string               `json:"needs,omitempty"`
	Status          JobStatus              `json:"status"` // "pending", "queued", "running", "completed", "skipped"
	Result          Result                 `json:"result"` // "success", "failure", "cancelled", "skipped"
	Outputs         map[string]string      `json:"outputs,omitempty"`
	MatrixValues    map[string]interface{} `json:"matrix,omitempty"`
	ContinueOnError bool                   `json:"continueOnError,omitempty"`
	StartedAt       time.Time              `json:"startedAt,omitempty"`
	CompletedAt     time.Time              `json:"completedAt,omitempty"`
	MatrixGroup     string                 `json:"matrixGroup,omitempty"`
	Def             *JobDef                `json:"-"`
}

// WorkflowEventMeta carries event metadata to be set on the workflow before dispatch.
type WorkflowEventMeta struct {
	EventName string
	Ref       string
	Sha       string
	Repo      string
	Inputs    map[string]string
}

// submitWorkflow creates a Workflow from a WorkflowDef and begins dispatching jobs.

// PendingDeployment is one reviewer-protected environment a run waits on.
type PendingDeployment struct {
	EnvID              int       `json:"envId"`
	EnvName            string    `json:"envName"`
	WaitTimerStartedAt time.Time `json:"waitTimerStartedAt"`
}

// EnvApproval is one submitted deployment review (approve or reject).
type EnvApproval struct {
	State     string    `json:"state"` // approved | rejected
	Comment   string    `json:"comment"`
	UserID    int       `json:"userId"`
	EnvIDs    []int     `json:"envIds"`
	EnvNames  []string  `json:"envNames"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Server) submitWorkflow(ctx context.Context, serverURL string, wf *WorkflowDef, defaultImage string, eventMeta ...*WorkflowEventMeta) (*Workflow, error) {
	ctx, span := otel.Tracer("bleephub").Start(ctx, "submitWorkflow",
		trace.WithAttributes(attribute.String("workflow.name", wf.Name)))
	defer span.End()
	// Validate no cycles in the job dependency graph
	if err := validateJobGraph(wf); err != nil {
		return nil, err
	}

	runID := s.store.ReserveRunID()

	workflow := &Workflow{
		ID:        uuid.New().String(),
		Name:      wf.Name,
		RunID:     runID,
		RunNumber: runID,
		Jobs:      make(map[string]*WorkflowJob),
		Env:       wf.Env,
		Status:    WorkflowStatusRunning,
		CreatedAt: time.Now(),
	}

	if workflow.Name == "" {
		workflow.Name = "workflow"
	}

	// Apply concurrency from WorkflowDef
	if wf.Concurrency != nil {
		workflow.ConcurrencyGroup = wf.Concurrency.Group
		workflow.CancelInProgress = wf.Concurrency.CancelInProgress
	}

	// Create WorkflowJobs for each JobDef
	for key, jd := range wf.Jobs {
		wfJob := &WorkflowJob{
			Key:             key,
			JobID:           uuid.New().String(),
			DisplayName:     key,
			Needs:           jd.Needs,
			Status:          JobStatusPending,
			Outputs:         make(map[string]string),
			ContinueOnError: jd.ContinueOnError,
			Def:             jd,
		}
		if jd.Name != "" {
			wfJob.DisplayName = jd.Name
		}

		// Extract matrix values from __matrix_ env prefix (set by expandMatrixJobs)
		if jd.Env != nil {
			matrixVals := make(map[string]interface{})
			for k, v := range jd.Env {
				if len(k) > 9 && k[:9] == "__matrix_" {
					matrixVals[k[9:]] = v
				}
			}
			if len(matrixVals) > 0 {
				wfJob.MatrixValues = matrixVals
			}
		}

		// Detect matrix group from key pattern (e.g., "test_0", "test_1" → group "test")
		if idx := strings.LastIndex(key, "_"); idx > 0 {
			suffix := key[idx+1:]
			if _, err := fmt.Sscanf(suffix, "%d", new(int)); err == nil {
				wfJob.MatrixGroup = key[:idx]
			}
		}

		// Track max-parallel from strategy
		if jd.Strategy != nil && jd.Strategy.MaxParallel > 0 && wfJob.MatrixGroup != "" {
			workflow.MaxParallel = jd.Strategy.MaxParallel
		}

		workflow.Jobs[key] = wfJob
	}

	// Apply event metadata before any goroutines can observe the workflow
	if len(eventMeta) > 0 && eventMeta[0] != nil {
		m := eventMeta[0]
		workflow.EventName = m.EventName
		workflow.Ref = m.Ref
		workflow.Sha = m.Sha
		workflow.RepoFullName = m.Repo
		workflow.Inputs = m.Inputs
	}

	// Resolve the originating workflow FILE so the GitHub-shape run object
	// can reference its stable id + real path (workflow_id / workflow_url /
	// path), which are constant across every run produced from the file.
	workflow.WorkflowFileID, workflow.WorkflowFilePath = s.resolveWorkflowFileForRun(workflow)

	// Handle concurrency control
	if workflow.ConcurrencyGroup != "" {
		s.store.mu.RLock()
		var activeWf *Workflow
		for _, existing := range s.store.Workflows {
			if existing.ID == workflow.ID {
				continue
			}
			if existing.ConcurrencyGroup == workflow.ConcurrencyGroup &&
				existing.Status == WorkflowStatusRunning {
				activeWf = existing
				break
			}
		}
		s.store.mu.RUnlock()

		if activeWf != nil {
			if workflow.CancelInProgress {
				// Cancel the old workflow
				s.cancelWorkflow(activeWf)
			} else {
				// Queue this workflow behind the active one
				workflow.Status = WorkflowStatusPendingConcurrency
				s.store.mu.Lock()
				s.store.Workflows[workflow.ID] = workflow
				s.store.mu.Unlock()
				return workflow, nil
			}
		}
	}

	// Store the workflow
	s.store.mu.Lock()
	s.store.Workflows[workflow.ID] = workflow
	s.store.mu.Unlock()

	if s.metrics != nil {
		s.metrics.RecordWorkflowSubmit()
	}

	// Start timeout watcher goroutine
	s.startTimeoutWatcher(workflow)

	// Dispatch root jobs (no dependencies)
	s.dispatchReadyJobs(ctx, workflow, serverURL, defaultImage)

	return workflow, nil
}

// resolveWorkflowFileForRun finds the registered [WorkflowFile] that
// produced this run (matched by repo + workflow name) and returns its
// stable id and real path. When no backing file is registered yet, it
// derives a deterministic stable id from (repo, conventional-path) and a
// best-known path so workflow_id / path stay constant across reruns of
// the same workflow even before the file lands in git.
func (s *Server) resolveWorkflowFileForRun(wf *Workflow) (int64, string) {
	repo := wf.RepoFullName
	if repo != "" {
		s.store.DiscoverWorkflowFilesFromGit(repo)
		for _, f := range s.store.ListWorkflowFiles(repo) {
			if f.Name == wf.Name {
				return f.ID, f.Path
			}
		}
	}
	// No registered file: derive a stable id from the conventional path so
	// the run still reports a constant workflow_id across reruns.
	path := ".github/workflows/" + wf.Name + ".yml"
	return stableWorkflowFileID(repo, path), path
}

// dispatchReadyJobs finds pending jobs whose dependencies are all satisfied
// and dispatches them to the runner. Loops until stable (skipping cascades).
func (s *Server) dispatchReadyJobs(ctx context.Context, wf *Workflow, serverURL string, defaultImage string) {
	ctx, span := otel.Tracer("bleephub").Start(ctx, "dispatchReadyJobs",
		trace.WithAttributes(attribute.String("workflow.id", wf.ID)))
	defer span.End()
	for {
		// Hold write lock while evaluating and updating job statuses
		s.store.mu.Lock()
		changed := false
		var toDispatch []*WorkflowJob
		for _, wfJob := range wf.Jobs {
			if wfJob.Status != JobStatusPending {
				continue
			}

			// Check all dependencies are completed
			allDepsOk := true
			anyDepFailed := false
			for _, dep := range wfJob.Needs {
				depJob, ok := wf.Jobs[dep]
				if !ok {
					allDepsOk = false
					break
				}
				if depJob.Status == JobStatusCompleted || depJob.Status == JobStatusSkipped {
					if depJob.Result != ResultSuccess && !depJob.ContinueOnError {
						anyDepFailed = true
					}
					continue
				}
				allDepsOk = false
				break
			}

			if !allDepsOk {
				continue
			}

			// Evaluate job-level if: condition
			if wfJob.Def != nil && wfJob.Def.If != "" {
				hasAlways, hasFailure := ExprContainsStatusFunction(wfJob.Def.If)
				exprCtx := &ExprContext{
					DepResults: make(map[string]string, len(wfJob.Needs)),
					Values:     make(map[string]string),
				}
				for _, dep := range wfJob.Needs {
					if depJob, ok := wf.Jobs[dep]; ok {
						exprCtx.DepResults[dep] = string(depJob.Result)
					}
				}
				if wf.EventName != "" {
					exprCtx.Values["github.event_name"] = wf.EventName
				}
				if wf.Ref != "" {
					exprCtx.Values["github.ref"] = wf.Ref
				}

				if !EvalExpr(wfJob.Def.If, exprCtx) {
					wfJob.Status = JobStatusSkipped
					wfJob.Result = ResultSkipped
					s.logger.Info().Str("job", wfJob.Key).Str("if", wfJob.Def.If).Msg("skipping job (if: false)")
					changed = true
					continue
				}

				// If expression contains always() or failure(), override dep-failure skip
				if hasAlways || hasFailure {
					anyDepFailed = false
				}
			}

			// If any dependency failed (and not continue-on-error), skip this job
			if anyDepFailed {
				wfJob.Status = JobStatusSkipped
				wfJob.Result = ResultSkipped
				s.logger.Info().Str("job", wfJob.Key).Msg("skipping job (dependency failed)")
				changed = true
				continue
			}

			// Enforce max-parallel: count running/queued jobs in same matrix group
			if wf.MaxParallel > 0 {
				active := 0
				for _, j := range wf.Jobs {
					if j.Key == wfJob.Key {
						continue
					}
					if (j.Status == JobStatusQueued || j.Status == JobStatusRunning) && j.MatrixGroup == wfJob.MatrixGroup && wfJob.MatrixGroup != "" {
						active++
					}
				}
				if active >= wf.MaxParallel {
					continue // Skip dispatch, will retry when a job completes
				}
			}

			// Environment protection: a job targeting an environment
			// with required reviewers waits for a deployment review
			// (approve via POST .../runs/{id}/pending_deployments).
			if envName := jobEnvironmentName(wfJob); envName != "" && !envApproved(wf, envName) {
				if env := s.protectedEnvironment(wf, envName); env != nil {
					wfJob.Status = JobStatusWaiting
					addPendingDeployment(wf, env)
					if wf.Status == WorkflowStatusRunning {
						wf.Status = WorkflowStatusWaiting
					}
					s.logger.Info().Str("job", wfJob.Key).Str("environment", envName).
						Msg("job waiting for deployment review")
					changed = true
					continue
				}
			}

			// Mark as queued now so max-parallel checks in this iteration see it
			wfJob.Status = JobStatusQueued
			wfJob.StartedAt = time.Now()
			toDispatch = append(toDispatch, wfJob)
			changed = true
		}
		s.store.mu.Unlock()

		// Dispatch collected jobs outside the lock (dispatchWorkflowJob acquires its own locks)
		for _, wfJob := range toDispatch {
			s.dispatchWorkflowJob(ctx, wf, wfJob, serverURL, defaultImage)
		}

		if !changed {
			break
		}
	}
}

// dispatchWorkflowJob builds and sends a job message to the runner.
func (s *Server) dispatchWorkflowJob(ctx context.Context, wf *Workflow, wfJob *WorkflowJob, serverURL, defaultImage string) {
	_, span := otel.Tracer("bleephub").Start(ctx, "dispatchWorkflowJob",
		trace.WithAttributes(
			attribute.String("workflow.id", wf.ID),
			attribute.String("job.key", wfJob.Key)))
	defer span.End()
	planID := uuid.New().String()
	timelineID := uuid.New().String()
	requestID := s.nextRequestID()

	msg := s.buildJobMessageFromDef(serverURL, wf, wfJob, planID, timelineID, requestID, defaultImage)
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		s.logger.Error().Err(err).Str("job", wfJob.Key).Msg("failed to marshal job message")
		return
	}

	job := &Job{
		ID:          wfJob.JobID,
		RequestID:   requestID,
		PlanID:      planID,
		TimelineID:  timelineID,
		Status:      "queued",
		Message:     string(msgJSON),
		LockedUntil: time.Now().Add(1 * time.Hour),
	}

	s.store.mu.Lock()
	s.store.Jobs[wfJob.JobID] = job
	s.store.mu.Unlock()

	envelope := &TaskAgentMessage{
		MessageID:   s.nextMessageID(),
		MessageType: "PipelineAgentJobRequest",
		Body:        string(msgJSON),
	}

	if !s.sendMessageToAgent(envelope) {
		s.requeuePendingMessage(envelope)
		s.logger.Warn().Str("jobId", wfJob.JobID).Msg("no runner available, workflow job queued (pending)")
	}

	if s.metrics != nil {
		s.metrics.RecordJobDispatch()
	}

	s.logger.Info().
		Str("workflow", wf.ID).
		Str("job", wfJob.Key).
		Str("jobId", wfJob.JobID).
		Msg("workflow job dispatched")
}

// onJobCompleted is called when a job finishes. It updates the workflow
// and dispatches any newly-ready dependent jobs.
func (s *Server) onJobCompleted(ctx context.Context, jobID, result string) {
	ctx, span := otel.Tracer("bleephub").Start(ctx, "onJobCompleted",
		trace.WithAttributes(
			attribute.String("job.id", jobID),
			attribute.String("job.result", result)))
	defer span.End()

	// Find the workflow and job under write lock, update status atomically
	s.store.mu.Lock()
	var foundWf *Workflow
	var foundJob *WorkflowJob
	for _, wf := range s.store.Workflows {
		for _, wfJob := range wf.Jobs {
			if wfJob.JobID == jobID {
				foundWf = wf
				foundJob = wfJob
				break
			}
		}
		if foundWf != nil {
			break
		}
	}

	if foundWf == nil {
		s.store.mu.Unlock()
		return // Not a workflow job
	}

	foundJob.Status = JobStatusCompleted
	foundJob.Result = Result(normalizeResult(result))
	foundJob.CompletedAt = time.Now()

	// Matrix fail-fast: if this job failed and it's in a matrix group, cancel siblings
	if foundJob.Result == ResultFailure && foundJob.MatrixGroup != "" {
		if foundJob.Def.FailFast() {
			for _, sibling := range foundWf.Jobs {
				if sibling.Key == foundJob.Key {
					continue
				}
				if sibling.MatrixGroup != foundJob.MatrixGroup {
					continue
				}
				if sibling.Status == JobStatusPending || sibling.Status == JobStatusQueued {
					sibling.Status = JobStatusCompleted
					sibling.Result = ResultCancelled
					sibling.CompletedAt = time.Now()
					s.logger.Info().
						Str("job", sibling.Key).
						Str("reason", "fail-fast").
						Msg("cancelling matrix sibling")
				}
			}
		}
	}
	s.store.mu.Unlock()

	if s.metrics != nil {
		duration := time.Since(foundWf.CreatedAt)
		s.metrics.RecordJobCompletion(string(foundJob.Result), duration)
	}

	s.logger.Info().
		Str("workflow_id", foundWf.ID).
		Str("workflow_name", foundWf.Name).
		Str("job_key", foundJob.Key).
		Str("job_id", foundJob.JobID).
		Str("result", string(foundJob.Result)).
		Msg("workflow job completed")

	// Dispatch any newly-ready jobs (this may also mark some as skipped)
	if foundWf.Env != nil {
		if serverURL, ok := foundWf.Env["__serverURL"]; ok {
			defaultImage := foundWf.Env["__defaultImage"]
			s.dispatchReadyJobs(ctx, foundWf, serverURL, defaultImage)
		}
	}

	// Check if all jobs are done (after dispatch, which may skip dependents)
	s.store.mu.Lock()
	allDone := true
	anyFailed := false
	for _, wfJob := range foundWf.Jobs {
		if wfJob.Status != JobStatusCompleted && wfJob.Status != JobStatusSkipped {
			allDone = false
		}
		if wfJob.Result == ResultFailure || wfJob.Result == ResultCancelled {
			anyFailed = true
		}
	}

	if allDone {
		foundWf.Status = WorkflowStatusCompleted
		if anyFailed {
			foundWf.Result = ResultFailure
		} else {
			foundWf.Result = ResultSuccess
		}
	}
	concurrencyGroup := foundWf.ConcurrencyGroup
	s.store.mu.Unlock()

	if allDone {
		if s.metrics != nil {
			s.metrics.RecordWorkflowComplete()
		}
		if foundWf.cancelTimeout != nil {
			foundWf.cancelTimeout()
		}
		duration := time.Since(foundWf.CreatedAt)
		s.logger.Info().
			Str("workflow_id", foundWf.ID).
			Str("workflow_name", foundWf.Name).
			Str("result", string(foundWf.Result)).
			Int64("duration_ms", duration.Milliseconds()).
			Msg("workflow completed")

		// Check for pending-concurrency workflows in the same group
		if concurrencyGroup != "" {
			s.startPendingConcurrencyWorkflow(concurrencyGroup)
		}
	}
}

// jobEnvironmentName resolves a job's target environment, tolerating a
// nil Def (directly-seeded test jobs).
func jobEnvironmentName(wfJob *WorkflowJob) string {
	if wfJob.Def == nil {
		return ""
	}
	return wfJob.Def.EnvironmentName()
}

// envApproved reports whether an approved review covering the
// environment has been submitted for this run.
func envApproved(wf *Workflow, envName string) bool {
	for _, a := range wf.EnvApprovals {
		if a.State != "approved" {
			continue
		}
		for _, name := range a.EnvNames {
			if name == envName {
				return true
			}
		}
	}
	return false
}

// protectedEnvironment returns the run repo's environment when it exists
// and carries required reviewers; environments without reviewers (or
// runs without a repo) don't gate dispatch. Referencing an environment
// auto-creates it, matching real GitHub.
func (s *Server) protectedEnvironment(wf *Workflow, envName string) *Environment {
	if wf.RepoFullName == "" {
		return nil
	}
	repo := s.store.ReposByName[wf.RepoFullName]
	if repo == nil {
		return nil
	}
	env := s.store.Deployments.GetEnvironment(repo.ID, envName)
	if env == nil {
		env = s.store.Deployments.UpsertEnvironment(repo.ID, envName)
	}
	if len(env.Reviewers) == 0 {
		return nil
	}
	return env
}

// addPendingDeployment records the run's wait on an environment exactly once.
func addPendingDeployment(wf *Workflow, env *Environment) {
	for _, p := range wf.PendingDeployments {
		if p.EnvID == env.ID {
			return
		}
	}
	wf.PendingDeployments = append(wf.PendingDeployments, &PendingDeployment{
		EnvID:              env.ID,
		EnvName:            env.Name,
		WaitTimerStartedAt: time.Now().UTC(),
	})
}

// applyDeploymentReview resolves a submitted review against the run's
// pending deployments: approved environments release their waiting jobs
// back to pending and dispatch resumes; rejected environments fail their
// waiting jobs and the run finalizes when nothing else is in flight.
// Returns the environment names the review covered.
func (s *Server) applyDeploymentReview(ctx context.Context, wf *Workflow, envIDs []int, state, comment string, reviewer *User) []string {
	s.store.mu.Lock()
	covered := map[string]bool{}
	var names []string
	remaining := wf.PendingDeployments[:0]
	for _, p := range wf.PendingDeployments {
		matched := false
		for _, id := range envIDs {
			if p.EnvID == id {
				matched = true
				break
			}
		}
		if matched {
			covered[p.EnvName] = true
			names = append(names, p.EnvName)
		} else {
			remaining = append(remaining, p)
		}
	}
	wf.PendingDeployments = remaining

	reviewerID := 0
	if reviewer != nil {
		reviewerID = reviewer.ID
	}
	wf.EnvApprovals = append(wf.EnvApprovals, &EnvApproval{
		State:     state,
		Comment:   comment,
		UserID:    reviewerID,
		EnvIDs:    append([]int(nil), envIDs...),
		EnvNames:  append([]string(nil), names...),
		CreatedAt: time.Now().UTC(),
	})

	for _, wfJob := range wf.Jobs {
		if wfJob.Status != JobStatusWaiting || !covered[jobEnvironmentName(wfJob)] {
			continue
		}
		if state == "approved" {
			wfJob.Status = JobStatusPending
		} else {
			wfJob.Status = JobStatusCompleted
			wfJob.Result = ResultFailure
			wfJob.CompletedAt = time.Now()
		}
	}
	if len(wf.PendingDeployments) == 0 && wf.Status == WorkflowStatusWaiting {
		wf.Status = WorkflowStatusRunning
	}
	serverURL := ""
	defaultImage := ""
	if wf.Env != nil {
		serverURL = wf.Env["__serverURL"]
		defaultImage = wf.Env["__defaultImage"]
	}
	s.store.mu.Unlock()

	if state == "approved" && serverURL != "" {
		s.dispatchReadyJobs(ctx, wf, serverURL, defaultImage)
	}
	s.finalizeWorkflowIfDone(wf)
	return names
}

// finalizeWorkflowIfDone completes the run when every job has reached a
// terminal state — the same check onJobCompleted performs after each
// job, needed independently when a rejection fails jobs without any job
// completion event.
func (s *Server) finalizeWorkflowIfDone(wf *Workflow) {
	s.store.mu.Lock()
	allDone := true
	anyFailed := false
	for _, wfJob := range wf.Jobs {
		if wfJob.Status != JobStatusCompleted && wfJob.Status != JobStatusSkipped {
			allDone = false
		}
		if wfJob.Result == ResultFailure || wfJob.Result == ResultCancelled {
			anyFailed = true
		}
	}
	if allDone && wf.Status != WorkflowStatusCompleted {
		wf.Status = WorkflowStatusCompleted
		if anyFailed {
			wf.Result = ResultFailure
		} else {
			wf.Result = ResultSuccess
		}
	} else {
		allDone = false
	}
	concurrencyGroup := wf.ConcurrencyGroup
	s.store.mu.Unlock()

	if allDone {
		if s.metrics != nil {
			s.metrics.RecordWorkflowComplete()
		}
		if wf.cancelTimeout != nil {
			wf.cancelTimeout()
		}
		if concurrencyGroup != "" {
			s.startPendingConcurrencyWorkflow(concurrencyGroup)
		}
	}
}

// cancelWorkflow cancels all pending/queued jobs and marks the workflow as cancelled.
func (s *Server) cancelWorkflow(wf *Workflow) {
	s.store.mu.Lock()
	for _, wfJob := range wf.Jobs {
		if wfJob.Status == JobStatusPending || wfJob.Status == JobStatusQueued {
			wfJob.Status = JobStatusCompleted
			wfJob.Result = ResultCancelled
			wfJob.CompletedAt = time.Now()
		}
	}
	wf.Status = WorkflowStatusCompleted
	wf.Result = ResultCancelled
	s.store.mu.Unlock()

	if wf.cancelTimeout != nil {
		wf.cancelTimeout()
	}
	if s.metrics != nil {
		s.metrics.RecordWorkflowComplete()
	}
	s.logger.Info().
		Str("workflow_id", wf.ID).
		Str("workflow_name", wf.Name).
		Msg("workflow cancelled")
}

// startPendingConcurrencyWorkflow finds and starts the next pending-concurrency
// workflow in the given concurrency group.
func (s *Server) startPendingConcurrencyWorkflow(group string) {
	s.store.mu.Lock()
	var pendingWf *Workflow
	for _, wf := range s.store.Workflows {
		if wf.ConcurrencyGroup == group && wf.Status == WorkflowStatusPendingConcurrency {
			if pendingWf == nil || wf.CreatedAt.Before(pendingWf.CreatedAt) {
				pendingWf = wf
			}
		}
	}

	if pendingWf == nil {
		s.store.mu.Unlock()
		return
	}

	pendingWf.Status = WorkflowStatusRunning
	s.store.mu.Unlock()

	if s.metrics != nil {
		s.metrics.RecordWorkflowSubmit()
	}
	s.startTimeoutWatcher(pendingWf)

	if pendingWf.Env != nil {
		if serverURL, ok := pendingWf.Env["__serverURL"]; ok {
			defaultImage := pendingWf.Env["__defaultImage"]
			s.dispatchReadyJobs(context.Background(), pendingWf, serverURL, defaultImage)
		}
	}
}

// normalizeResult converts runner result strings to consistent format.
func normalizeResult(result string) string {
	switch result {
	case "Succeeded", "succeeded":
		return "success"
	case "Failed", "failed":
		return "failure"
	case "Cancelled", "cancelled":
		return "cancelled"
	default:
		if result == "" {
			return "success"
		}
		return result
	}
}

// startTimeoutWatcher starts a goroutine that periodically checks for timed-out jobs.
func (s *Server) startTimeoutWatcher(wf *Workflow) {
	ctx, cancel := context.WithCancel(context.Background())
	wf.cancelTimeout = cancel

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.checkJobTimeouts(wf)
			}
		}
	}()
}

// checkJobTimeouts cancels jobs that have exceeded their timeout.
func (s *Server) checkJobTimeouts(wf *Workflow) {
	s.store.mu.Lock()
	if wf.Status == WorkflowStatusCompleted {
		s.store.mu.Unlock()
		return
	}
	now := time.Now()
	var timedOut bool
	for _, wfJob := range wf.Jobs {
		if wfJob.Status != JobStatusQueued && wfJob.Status != JobStatusRunning {
			continue
		}
		if wfJob.StartedAt.IsZero() {
			continue
		}
		timeout := 360 // default 6 hours
		if wfJob.Def != nil && wfJob.Def.TimeoutMinutes > 0 {
			timeout = wfJob.Def.TimeoutMinutes
		}
		if now.Sub(wfJob.StartedAt) > time.Duration(timeout)*time.Minute {
			s.logger.Warn().
				Str("workflow_id", wf.ID).
				Str("job_key", wfJob.Key).
				Int("timeout_minutes", timeout).
				Msg("job timed out, marking cancelled")
			wfJob.Status = JobStatusCompleted
			wfJob.Result = ResultCancelled
			wfJob.CompletedAt = now
			timedOut = true
		}
	}
	s.store.mu.Unlock()

	// Re-dispatch to handle dependents (outside lock since dispatchReadyJobs acquires locks)
	if timedOut {
		if wf.Env != nil {
			if serverURL, ok := wf.Env["__serverURL"]; ok {
				s.dispatchReadyJobs(context.Background(), wf, serverURL, wf.Env["__defaultImage"])
			}
		}
	}
}

// validateJobGraph checks for cycles in the job dependency graph.
func validateJobGraph(wf *WorkflowDef) error {
	// Topological sort via DFS
	visited := make(map[string]int) // 0=unvisited, 1=visiting, 2=visited
	var visit func(key string) error
	visit = func(key string) error {
		if visited[key] == 2 {
			return nil
		}
		if visited[key] == 1 {
			return fmt.Errorf("cycle detected involving job %q", key)
		}
		visited[key] = 1

		jd, ok := wf.Jobs[key]
		if !ok {
			return fmt.Errorf("job %q references unknown dependency", key)
		}
		for _, dep := range jd.Needs {
			if _, ok := wf.Jobs[dep]; !ok {
				return fmt.Errorf("job %q needs unknown job %q", key, dep)
			}
			if err := visit(dep); err != nil {
				return err
			}
		}
		visited[key] = 2
		return nil
	}

	for key := range wf.Jobs {
		if err := visit(key); err != nil {
			return err
		}
	}
	return nil
}
