package bleeplab

// pipeline.go owns the in-memory CI execution model: turning a parsed
// .gitlab-ci.yml into ordered jobs, enqueuing them stage-by-stage as the
// previous stage succeeds, and rolling the pipeline status up from its jobs.

// createPipelineLocked builds a pipeline + its jobs from the project's stored
// CI config and enqueues the first stage. Caller holds s.mu.
func (s *Server) createPipelineLocked(p *Project, ref, sha string) (*Pipeline, error) {
	cfg, err := parseCIConfig(p.CIConfig)
	if err != nil {
		return nil, err
	}
	pl := &Pipeline{
		ID:        s.nextID,
		ProjectID: p.ID,
		Ref:       ref,
		SHA:       sha,
		Status:    "pending",
		Stages:    cfg.Stages,
	}
	s.nextID++
	s.pipelines[pl.ID] = pl

	for _, jc := range cfg.Jobs {
		job := &Job{
			ID:           s.nextID,
			Token:        newToken("gljt", s.nextID),
			Name:         jc.Name,
			Stage:        jc.Stage,
			Status:       "created",
			Script:       jc.Script,
			BeforeS:      jc.Before,
			AfterS:       jc.After,
			Image:        jc.Image,
			Services:     jc.Services,
			Variables:    jc.Variables,
			Artifacts:    jc.Artifacts,
			Dependencies: jc.Dependencies,
			ProjectID:    p.ID,
			PipelineID:   pl.ID,
			Ref:          ref,
			SHA:          sha,
		}
		s.nextID++
		s.jobs[job.ID] = job
		pl.JobIDs = append(pl.JobIDs, job.ID)
	}

	s.enqueueStageLocked(pl)
	return pl, nil
}

// enqueueStageLocked moves every `created` job in the pipeline's current
// stage to `pending` and appends it to the runner queue. If the current
// stage has no jobs it advances until it finds one or runs out of stages.
func (s *Server) enqueueStageLocked(pl *Pipeline) {
	for pl.stageIdx < len(pl.Stages) {
		stage := pl.Stages[pl.stageIdx]
		found := false
		for _, jid := range pl.JobIDs {
			job := s.jobs[jid]
			if job.Stage != stage || job.Status != "created" {
				continue
			}
			job.mu.Lock()
			job.Status = "pending"
			job.mu.Unlock()
			s.queue = append(s.queue, jid)
			found = true
		}
		if found {
			pl.Status = "running"
			return
		}
		pl.stageIdx++ // empty stage — skip
	}
	// No stages left with jobs: the pipeline is complete.
	if pl.Status == "running" || pl.Status == "pending" {
		pl.Status = "success"
	}
}

// dequeueJobLocked pops the next queued job. Caller holds s.mu.
func (s *Server) dequeueJobLocked() *Job {
	for len(s.queue) > 0 {
		id := s.queue[0]
		s.queue = s.queue[1:]
		if job, ok := s.jobs[id]; ok {
			return job
		}
	}
	return nil
}

// onJobTerminal advances or fails the pipeline when a job reaches a terminal
// state. A failed job fails the whole pipeline; when every job in the current
// stage has succeeded, the next stage is enqueued.
func (s *Server) onJobTerminal(job *Job, state string) {
	if state != "success" && state != "failed" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pl, ok := s.pipelines[job.PipelineID]
	if !ok {
		return
	}
	if state == "failed" {
		pl.Status = "failed"
		s.logger.Info().Int("pipeline", pl.ID).Int("job", job.ID).Msg("pipeline failed (job failed)")
		return
	}
	// Success: is the current stage fully done?
	stage := pl.Stages[min(pl.stageIdx, len(pl.Stages)-1)]
	for _, jid := range pl.JobIDs {
		j := s.jobs[jid]
		if j.Stage != stage {
			continue
		}
		j.mu.Lock()
		st := j.Status
		j.mu.Unlock()
		if st != "success" && st != "failed" {
			return // stage still has live jobs
		}
	}
	// Stage complete — advance to the next stage.
	pl.stageIdx++
	s.enqueueStageLocked(pl)
	if pl.Status == "success" {
		s.logger.Info().Int("pipeline", pl.ID).Msg("pipeline succeeded")
	}
}

// setPipelineStatus marks the most recent pipeline of a project to status
// when it is still pending (a job got claimed). Best-effort; the real status
// roll-up happens in onJobTerminal.
func (s *Server) setPipelineStatus(projectID int, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, pl := range s.pipelines {
		if pl.ProjectID == projectID && pl.Status == "pending" {
			pl.Status = status
		}
	}
}
