package gitlabhub

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// submitPipeline creates a Pipeline from a PipelineDef and begins dispatching jobs.
func (s *Server) submitPipeline(ctx context.Context, project *Project, def *PipelineDef, serverURL, defaultImage string) (*Pipeline, error) {
	ctx, span := otel.Tracer("gitlabhub").Start(ctx, "submitPipeline",
		trace.WithAttributes(attribute.String("pipeline.project", project.Name)))
	defer span.End()
	if err := validatePipelineGraph(def); err != nil {
		return nil, err
	}

	s.store.mu.Lock()
	pipelineID := s.store.NextPipeline
	s.store.NextPipeline++
	s.store.mu.Unlock()

	// Get commit sha from git repo
	sha := "0000000000000000000000000000000000000000"
	stor := s.store.GetGitStorage(project.Name)
	if stor != nil {
		if ref, err := stor.Reference("refs/heads/main"); err == nil {
			sha = ref.Hash().String()
		}
	}

	pipeline := &Pipeline{
		ID:        pipelineID,
		ProjectID: project.ID,
		Status:    "running",
		Ref:       "main",
		Sha:       sha,
		Jobs:      make(map[string]*PipelineJob),
		Stages:    def.Stages,
		Def:       def,
		CreatedAt: time.Now(),
		ServerURL: serverURL,
		Image:     defaultImage,
	}

	// Create PipelineJob entries for each job definition
	s.store.mu.Lock()
	for name, jobDef := range def.Jobs {
		jobID := s.store.NextJob
		s.store.NextJob++

		pj := &PipelineJob{
			ID:           jobID,
			PipelineID:   pipelineID,
			ProjectID:    project.ID,
			Name:         name,
			Stage:        jobDef.Stage,
			Status:       "created",
			AllowFailure: jobDef.AllowFailure,
			When:         jobDef.When,
			Needs:        jobDef.Needs,
			Token:        generateJobToken(),
			MatrixGroup:  jobDef.MatrixGroup,
		}

		// Set resource group if defined
		if jobDef.ResourceGroup != "" {
			pj.ResourceGroup = jobDef.ResourceGroup
		}

		if jobDef.Timeout > 0 {
			pj.Timeout = jobDef.Timeout
		}
		if jobDef.Retry != nil {
			pj.MaxRetries = jobDef.Retry.Max
		}

		// Evaluate rules
		if len(jobDef.Rules) > 0 {
			when := evaluateRules(jobDef.Rules, jobDef.Variables)
			if when == "never" {
				pj.Status = "skipped"
				pj.Result = "skipped"
			} else {
				pj.When = when
			}
		}

		pipeline.Jobs[name] = pj
		s.store.Jobs[jobID] = pj
	}
	s.store.Pipelines[pipelineID] = pipeline
	s.store.mu.Unlock()

	if s.metrics != nil {
		s.metrics.RecordPipelineSubmit()
	}

	s.logger.Info().
		Int("pipeline_id", pipelineID).
		Int("project_id", project.ID).
		Int("num_jobs", len(pipeline.Jobs)).
		Msg("pipeline submitted")

	// Dispatch first stage
	s.dispatchReadyJobs(ctx, pipeline)

	return pipeline, nil
}

// dispatchReadyJobs finds jobs whose dependencies are satisfied and enqueues them.
// Stage-ordered: all jobs in stage N must complete before stage N+1.
// DAG `needs:` overrides stage ordering.
func (s *Server) dispatchReadyJobs(ctx context.Context, pipeline *Pipeline) {
	_, span := otel.Tracer("gitlabhub").Start(ctx, "dispatchReadyJobs",
		trace.WithAttributes(attribute.Int("pipeline.id", pipeline.ID)))
	defer span.End()
	for {
		changed := false
		for _, job := range pipeline.Jobs {
			if job.Status != "created" {
				continue
			}

			// If job has explicit needs (DAG mode), check those
			if len(job.Needs) > 0 {
				allReady := true
				anyFailed := false
				for _, dep := range job.Needs {
					depJob, ok := pipeline.Jobs[dep]
					if !ok {
						allReady = false
						break
					}
					if isTerminalStatus(depJob.Status) {
						if depJob.Status == "failed" && !depJob.AllowFailure {
							anyFailed = true
						}
						continue
					}
					allReady = false
					break
				}
				if !allReady {
					continue
				}
				if anyFailed && job.When == "on_success" {
					job.Status = "skipped"
					job.Result = "skipped"
					s.logger.Info().Str("job", job.Name).Msg("skipping job (dependency failed)")
					changed = true
					continue
				}
			} else {
				// Stage-based ordering: all jobs in previous stages must be done
				if !s.allPriorStagesComplete(pipeline, job.Stage) {
					continue
				}
				// Check if any prior stage job failed
				if s.anyPriorStageFailed(pipeline, job.Stage) && job.When == "on_success" {
					job.Status = "skipped"
					job.Result = "skipped"
					s.logger.Info().Str("job", job.Name).Msg("skipping job (prior stage failed)")
					changed = true
					continue
				}
			}

			// Check resource group
			if job.ResourceGroup != "" {
				if s.isResourceGroupBusy(pipeline, job) {
					continue
				}
			}

			// Dispatch job
			job.Status = "pending"
			s.store.EnqueueJob(job.ID)
			s.logger.Info().
				Int("job_id", job.ID).
				Str("name", job.Name).
				Str("stage", job.Stage).
				Msg("job enqueued")
			changed = true
		}
		if !changed {
			break
		}
	}
}

// onJobCompleted is called when a job finishes. Updates pipeline and dispatches next jobs.
func (s *Server) onJobCompleted(ctx context.Context, jobID int, status string) {
	ctx, span := otel.Tracer("gitlabhub").Start(ctx, "onJobCompleted",
		trace.WithAttributes(
			attribute.Int("job.id", jobID),
			attribute.String("job.status", status)))
	defer span.End()
	s.store.mu.RLock()
	var foundPipeline *Pipeline
	var foundJob *PipelineJob
	for _, pl := range s.store.Pipelines {
		for _, pj := range pl.Jobs {
			if pj.ID == jobID {
				foundPipeline = pl
				foundJob = pj
				break
			}
		}
		if foundPipeline != nil {
			break
		}
	}
	s.store.mu.RUnlock()

	if foundPipeline == nil {
		return
	}

	s.logger.Info().
		Int("pipeline_id", foundPipeline.ID).
		Int("job_id", foundJob.ID).
		Str("job_name", foundJob.Name).
		Str("status", status).
		Msg("pipeline job completed")

	// Check retry
	if status == "failed" && foundJob.RetryCount < foundJob.MaxRetries {
		s.store.mu.Lock()
		foundJob.RetryCount++
		foundJob.Status = "created"
		foundJob.Result = ""
		foundJob.TraceData = nil
		s.store.mu.Unlock()

		s.logger.Info().
			Int("job_id", foundJob.ID).
			Str("name", foundJob.Name).
			Int("retry", foundJob.RetryCount).
			Int("max", foundJob.MaxRetries).
			Msg("retrying failed job")

		s.dispatchReadyJobs(ctx, foundPipeline)
		return
	}

	// Dispatch any newly-ready jobs
	s.dispatchReadyJobs(ctx, foundPipeline)

	// Check if all jobs are done
	allDone := true
	anyFailed := false
	for _, pj := range foundPipeline.Jobs {
		if !isTerminalStatus(pj.Status) {
			allDone = false
		}
		if pj.Status == "failed" && !pj.AllowFailure {
			anyFailed = true
		}
	}

	if allDone {
		s.store.mu.Lock()
		if anyFailed {
			foundPipeline.Status = "failed"
			foundPipeline.Result = "failed"
		} else {
			foundPipeline.Status = "success"
			foundPipeline.Result = "success"
		}
		s.store.mu.Unlock()

		if s.metrics != nil {
			s.metrics.RecordPipelineComplete()
		}

		duration := time.Since(foundPipeline.CreatedAt)
		s.logger.Info().
			Int("pipeline_id", foundPipeline.ID).
			Str("result", foundPipeline.Result).
			Int64("duration_ms", duration.Milliseconds()).
			Msg("pipeline completed")
	}
}

// cancelPipeline cancels all non-terminal jobs in a pipeline and sets pipeline status to canceled.
func (s *Server) cancelPipeline(pipeline *Pipeline) {
	s.store.mu.Lock()
	for _, job := range pipeline.Jobs {
		if !isTerminalStatus(job.Status) {
			job.Status = "canceled"
			job.Result = "canceled"
		}
	}
	pipeline.Status = "canceled"
	pipeline.Result = "canceled"
	s.store.mu.Unlock()

	// Remove pending jobs from queue
	s.store.mu.Lock()
	var remaining []int
	for _, id := range s.store.PendingJobs {
		found := false
		for _, j := range pipeline.Jobs {
			if j.ID == id {
				found = true
				break
			}
		}
		if !found {
			remaining = append(remaining, id)
		}
	}
	s.store.PendingJobs = remaining
	s.store.mu.Unlock()

	s.logger.Info().
		Int("pipeline_id", pipeline.ID).
		Msg("pipeline canceled")
}

// isResourceGroupBusy checks if another job in the same resource group is currently running or pending.
func (s *Server) isResourceGroupBusy(pipeline *Pipeline, job *PipelineJob) bool {
	for _, other := range pipeline.Jobs {
		if other.ID == job.ID {
			continue
		}
		if other.ResourceGroup == job.ResourceGroup {
			if other.Status == "running" || other.Status == "pending" {
				return true
			}
		}
	}
	return false
}

// allPriorStagesComplete checks if all jobs in stages before the given stage are done.
func (s *Server) allPriorStagesComplete(pipeline *Pipeline, stage string) bool {
	targetIdx := stageIndex(pipeline.Stages, stage)
	if targetIdx <= 0 {
		return true // first stage or not found
	}
	for _, job := range pipeline.Jobs {
		jobIdx := stageIndex(pipeline.Stages, job.Stage)
		if jobIdx < targetIdx && !isTerminalStatus(job.Status) {
			return false
		}
	}
	return true
}

// anyPriorStageFailed checks if any job in prior stages failed (not allow_failure).
func (s *Server) anyPriorStageFailed(pipeline *Pipeline, stage string) bool {
	targetIdx := stageIndex(pipeline.Stages, stage)
	for _, job := range pipeline.Jobs {
		jobIdx := stageIndex(pipeline.Stages, job.Stage)
		if jobIdx < targetIdx && job.Status == "failed" && !job.AllowFailure {
			return true
		}
	}
	return false
}

func isTerminalStatus(status string) bool {
	return status == "success" || status == "failed" || status == "canceled" || status == "skipped"
}

// evaluateRules processes rule conditions and returns the effective "when" value.
func evaluateRules(rules []RuleDef, variables map[string]string) string {
	ctx := &ExprContext{Variables: variables}
	for _, rule := range rules {
		if rule.If != "" {
			if EvalGitLabExpr(rule.If, ctx) {
				when := rule.When
				if when == "" {
					when = "on_success"
				}
				return when
			}
			continue // this rule didn't match, try next
		}
		// Rule without if: always matches
		if rule.When != "" {
			return rule.When
		}
		return "on_success"
	}
	// No rules matched → "never" (GitLab default: if no rule matches, job is excluded)
	return "never"
}

// validatePipelineGraph checks for cycles in the job dependency graph.
func validatePipelineGraph(def *PipelineDef) error {
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

		jd, ok := def.Jobs[key]
		if !ok {
			return fmt.Errorf("job %q references unknown dependency", key)
		}
		for _, dep := range jd.Needs {
			if _, ok := def.Jobs[dep]; !ok {
				return fmt.Errorf("job %q needs unknown job %q", key, dep)
			}
			if err := visit(dep); err != nil {
				return err
			}
		}
		visited[key] = 2
		return nil
	}

	for key := range def.Jobs {
		if err := visit(key); err != nil {
			return err
		}
	}
	return nil
}

// generateJobToken creates a random job token.
func generateJobToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
