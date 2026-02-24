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

// Workflow represents a running multi-job workflow.
type Workflow struct {
	ID               string                  `json:"id"`
	Name             string                  `json:"name"`
	RunID            int                     `json:"runId"`
	RunNumber        int                     `json:"runNumber"`
	Jobs             map[string]*WorkflowJob `json:"jobs"`
	Env              map[string]string       `json:"env,omitempty"`
	Status           string                  `json:"status"` // "running", "completed", "pending_concurrency"
	Result           string                  `json:"result"` // "success", "failure", "cancelled"
	CreatedAt        time.Time               `json:"createdAt"`
	MaxParallel      int                     `json:"-"` // per-matrix-group limit
	cancelTimeout    func()                  // stops the timeout watcher goroutine
	EventName        string                  `json:"eventName,omitempty"`
	Ref              string                  `json:"ref,omitempty"`
	Sha              string                  `json:"sha,omitempty"`
	RepoFullName     string                  `json:"repoFullName,omitempty"`
	Inputs           map[string]string       `json:"inputs,omitempty"`
	ConcurrencyGroup string                  `json:"concurrencyGroup,omitempty"`
	CancelInProgress bool                    `json:"-"`
}

// WorkflowJob represents a single job within a workflow.
type WorkflowJob struct {
	Key             string                 `json:"key"`     // YAML key
	JobID           string                 `json:"jobId"`   // UUID, used as Job.ID
	DisplayName     string                 `json:"displayName"`
	Needs           []string               `json:"needs,omitempty"`
	Status          string                 `json:"status"` // "pending", "queued", "running", "completed", "skipped"
	Result          string                 `json:"result"` // "success", "failure", "cancelled", "skipped"
	Outputs         map[string]string      `json:"outputs,omitempty"`
	MatrixValues    map[string]interface{} `json:"matrix,omitempty"`
	ContinueOnError bool                   `json:"continueOnError,omitempty"`
	StartedAt       time.Time              `json:"startedAt,omitempty"`
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
func (s *Server) submitWorkflow(ctx context.Context, serverURL string, wf *WorkflowDef, defaultImage string, eventMeta ...*WorkflowEventMeta) (*Workflow, error) {
	ctx, span := otel.Tracer("bleephub").Start(ctx, "submitWorkflow",
		trace.WithAttributes(attribute.String("workflow.name", wf.Name)))
	defer span.End()
	// Validate no cycles in the job dependency graph
	if err := validateJobGraph(wf); err != nil {
		return nil, err
	}

	s.store.mu.Lock()
	runID := s.store.NextRunID
	s.store.NextRunID++
	s.store.mu.Unlock()

	workflow := &Workflow{
		ID:        uuid.New().String(),
		Name:      wf.Name,
		RunID:     runID,
		RunNumber: runID,
		Jobs:      make(map[string]*WorkflowJob),
		Env:       wf.Env,
		Status:    "running",
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
			Status:          "pending",
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

	// Handle concurrency control
	if workflow.ConcurrencyGroup != "" {
		s.store.mu.RLock()
		var activeWf *Workflow
		for _, existing := range s.store.Workflows {
			if existing.ID == workflow.ID {
				continue
			}
			if existing.ConcurrencyGroup == workflow.ConcurrencyGroup &&
				existing.Status == "running" {
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
				workflow.Status = "pending_concurrency"
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
			if wfJob.Status != "pending" {
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
				if depJob.Status == "completed" || depJob.Status == "skipped" {
					if depJob.Result != "success" && !depJob.ContinueOnError {
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
						exprCtx.DepResults[dep] = depJob.Result
					}
				}
				if wf.EventName != "" {
					exprCtx.Values["github.event_name"] = wf.EventName
				}
				if wf.Ref != "" {
					exprCtx.Values["github.ref"] = wf.Ref
				}

				if !EvalExpr(wfJob.Def.If, exprCtx) {
					wfJob.Status = "skipped"
					wfJob.Result = "skipped"
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
				wfJob.Status = "skipped"
				wfJob.Result = "skipped"
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
					if (j.Status == "queued" || j.Status == "running") && j.MatrixGroup == wfJob.MatrixGroup && wfJob.MatrixGroup != "" {
						active++
					}
				}
				if active >= wf.MaxParallel {
					continue // Skip dispatch, will retry when a job completes
				}
			}

			// Mark as queued now so max-parallel checks in this iteration see it
			wfJob.Status = "queued"
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

	foundJob.Status = "completed"
	foundJob.Result = normalizeResult(result)

	// Matrix fail-fast: if this job failed and it's in a matrix group, cancel siblings
	if foundJob.Result == "failure" && foundJob.MatrixGroup != "" {
		failFast := true // default per GitHub Actions spec
		if foundJob.Def != nil && foundJob.Def.Strategy != nil && foundJob.Def.Strategy.FailFast != nil {
			failFast = *foundJob.Def.Strategy.FailFast
		}
		if failFast {
			for _, sibling := range foundWf.Jobs {
				if sibling.Key == foundJob.Key {
					continue
				}
				if sibling.MatrixGroup != foundJob.MatrixGroup {
					continue
				}
				if sibling.Status == "pending" || sibling.Status == "queued" {
					sibling.Status = "completed"
					sibling.Result = "cancelled"
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
		s.metrics.RecordJobCompletion(foundJob.Result, duration)
	}

	s.logger.Info().
		Str("workflow_id", foundWf.ID).
		Str("workflow_name", foundWf.Name).
		Str("job_key", foundJob.Key).
		Str("job_id", foundJob.JobID).
		Str("result", foundJob.Result).
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
		if wfJob.Status != "completed" && wfJob.Status != "skipped" {
			allDone = false
		}
		if wfJob.Result == "failure" || wfJob.Result == "cancelled" {
			anyFailed = true
		}
	}

	if allDone {
		foundWf.Status = "completed"
		if anyFailed {
			foundWf.Result = "failure"
		} else {
			foundWf.Result = "success"
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
			Str("result", foundWf.Result).
			Int64("duration_ms", duration.Milliseconds()).
			Msg("workflow completed")

		// Check for pending-concurrency workflows in the same group
		if concurrencyGroup != "" {
			s.startPendingConcurrencyWorkflow(concurrencyGroup)
		}
	}
}

// cancelWorkflow cancels all pending/queued jobs and marks the workflow as cancelled.
func (s *Server) cancelWorkflow(wf *Workflow) {
	s.store.mu.Lock()
	for _, wfJob := range wf.Jobs {
		if wfJob.Status == "pending" || wfJob.Status == "queued" {
			wfJob.Status = "completed"
			wfJob.Result = "cancelled"
		}
	}
	wf.Status = "completed"
	wf.Result = "cancelled"
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
		if wf.ConcurrencyGroup == group && wf.Status == "pending_concurrency" {
			if pendingWf == nil || wf.CreatedAt.Before(pendingWf.CreatedAt) {
				pendingWf = wf
			}
		}
	}

	if pendingWf == nil {
		s.store.mu.Unlock()
		return
	}

	pendingWf.Status = "running"
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
	if wf.Status == "completed" {
		s.store.mu.Unlock()
		return
	}
	now := time.Now()
	var timedOut bool
	for _, wfJob := range wf.Jobs {
		if wfJob.Status != "queued" && wfJob.Status != "running" {
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
			wfJob.Status = "completed"
			wfJob.Result = "cancelled"
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
