package bleeplab

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// handleRunnerVerify validates a runner authentication token. gitlab-runner
// calls this on `register --token` and at startup. 200 = valid; 403 = unknown.
func (s *Server) handleRunnerVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	rn, ok := s.runners[req.Token]
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "403 Forbidden"})
		return
	}
	// Real GitLab returns the runner identity on verify — emit the real
	// runner id, not a fabricated 0.
	writeJSON(w, http.StatusOK, map[string]any{"id": rn.ID, "token": req.Token})
}

// handleRunnerRegister is the legacy registration-token flow
// (`POST /api/v4/runners` with a registration token) — it mints and returns
// a new runner authentication token.
func (s *Server) handleRunnerRegister(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rn := s.newRunnerLocked()
	writeJSON(w, http.StatusCreated, map[string]any{"id": rn.ID, "token": rn.Token})
}

// handleRunnerUnregister deletes a runner by its auth token.
func (s *Server) handleRunnerUnregister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	delete(s.runners, req.Token)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// handleUserRunnerCreate is the modern runner-creation flow
// (`POST /api/v4/user/runners`) the orchestrator uses; returns {id, token}.
func (s *Server) handleUserRunnerCreate(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rn := s.newRunnerLocked()
	writeJSON(w, http.StatusCreated, map[string]any{"id": rn.ID, "token": rn.Token})
}

func (s *Server) newRunnerLocked() *Runner {
	id := s.nextID
	s.nextID++
	rn := &Runner{ID: id, Token: newToken("glrt", id)}
	s.runners[rn.Token] = rn
	return rn
}

// handleJobRequest is the runner's poll loop. With a valid runner token and a
// queued job, it returns 201 + the job payload; otherwise 204 No Content.
func (s *Server) handleJobRequest(w http.ResponseWriter, r *http.Request) {
	var req jobRequest
	if err := readJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	_, known := s.runners[req.Token]
	if !known {
		s.mu.Unlock()
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "403 Forbidden"})
		return
	}
	job := s.dequeueJobLocked()
	lastUpdate := s.nextID
	s.mu.Unlock()

	w.Header().Set("X-GitLab-Last-Update", strconv.Itoa(lastUpdate))
	if job == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	job.mu.Lock()
	job.Status = "running"
	job.mu.Unlock()
	s.setPipelineStatus(job.ProjectID, "running")

	gitBase := s.externalURL
	if gitBase == "" {
		gitBase = "http://" + r.Host
	}
	s.logger.Info().Int("job", job.ID).Str("name", job.Name).Str("stage", job.Stage).Str("runner", req.Token).Msg("job claimed by runner")
	writeJSON(w, http.StatusCreated, s.buildJobResponse(job, gitBase))
}

// handleJobUpdate processes the runner's terminal/intermediate status update
// (`PUT /api/v4/jobs/:id`). Body carries the job token + state (+ optional
// full trace).
func (s *Server) handleJobUpdate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	var req struct {
		Token string `json:"token"`
		State string `json:"state"`
		Trace string `json:"trace"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	job, ok := s.jobs[id]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "404 job not found", http.StatusNotFound)
		return
	}
	if job.Token != req.Token {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "403 Forbidden"})
		return
	}
	if req.Trace != "" {
		job.setTrace(req.Trace)
	}
	if req.State != "" {
		job.mu.Lock()
		job.Status = req.State
		job.mu.Unlock()
		s.logger.Info().Int("job", id).Str("state", req.State).Msg("job state updated")
		s.onJobTerminal(job, req.State)
	}
	w.Header().Set("Job-Status", req.State)
	w.WriteHeader(http.StatusOK)
}

// handleJobTrace appends an incremental trace chunk
// (`PATCH /api/v4/jobs/:id/trace`). Returns 202 + the new accepted Range, the
// contract gitlab-runner uses to advance its send offset.
func (s *Server) handleJobTrace(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	s.mu.Lock()
	job, ok := s.jobs[id]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "404 job not found", http.StatusNotFound)
		return
	}
	if tok := r.Header.Get("JOB-TOKEN"); tok != "" && tok != job.Token {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "403 Forbidden"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		// A failed/truncated read must not be appended and acknowledged —
		// returning 202 + an advanced Range would tell gitlab-runner the
		// bytes were durably stored and move its send offset past data we
		// never kept, corrupting the trace.
		http.Error(w, "400 failed to read trace chunk", http.StatusBadRequest)
		return
	}

	// Content-Range: "start-end" — the byte offset this chunk begins at.
	start := job.traceLen()
	if cr := r.Header.Get("Content-Range"); cr != "" {
		if dash := strings.IndexByte(cr, '-'); dash > 0 {
			if v, err := strconv.Atoi(strings.TrimSpace(cr[:dash])); err == nil {
				start = v
			}
		}
	}
	// Idempotent append: only extend if this chunk starts exactly at the
	// current end (gitlab-runner resends from its last accepted offset).
	if start == job.traceLen() {
		job.appendTrace(string(body))
	} else if start < job.traceLen() {
		// Overlapping resend — accept the suffix beyond what we already have.
		have := job.traceLen()
		if overlap := have - start; overlap >= 0 && overlap <= len(body) {
			job.appendTrace(string(body[overlap:]))
		}
	}

	newLen := job.traceLen()
	w.Header().Set("Range", fmt.Sprintf("0-%d", newLen))
	job.mu.Lock()
	status := job.Status
	job.mu.Unlock()
	w.Header().Set("Job-Status", status)
	w.WriteHeader(http.StatusAccepted)
}

// buildDependenciesLocked builds the runner-API `dependencies` list: the jobs
// in earlier stages of the same pipeline whose artifacts this job should
// download before running. With no explicit `dependencies:` the job depends on
// every earlier-stage job (GitLab's default); an explicit list restricts it.
// Each entry carries the dependency's token + artifacts_file metadata so the
// runner fetches `GET /api/v4/jobs/:id/artifacts` with that token. Caller holds
// s.mu.
func (s *Server) buildDependenciesLocked(job *Job) []jobDependency {
	deps := []jobDependency{}
	pl, ok := s.pipelines[job.PipelineID]
	if !ok {
		return deps
	}
	stageIndex := func(stage string) int {
		for i, st := range pl.Stages {
			if st == stage {
				return i
			}
		}
		return -1
	}
	myStage := stageIndex(job.Stage)
	if myStage < 0 {
		return deps
	}
	explicit := job.Dependencies != nil
	want := make(map[string]bool, len(job.Dependencies))
	for _, d := range job.Dependencies {
		want[d] = true
	}
	for _, jid := range pl.JobIDs {
		cand := s.jobs[jid]
		if cand == nil || cand.ID == job.ID {
			continue
		}
		if ci := stageIndex(cand.Stage); ci < 0 || ci >= myStage {
			continue
		}
		if explicit && !want[cand.Name] {
			continue
		}
		cand.mu.Lock()
		size, fn := cand.ArtifactSize, cand.ArtifactFilename
		cand.mu.Unlock()
		if size <= 0 {
			continue
		}
		if fn == "" {
			fn = "artifacts.zip"
		}
		deps = append(deps, jobDependency{
			ID:            cand.ID,
			Name:          cand.Name,
			Token:         cand.Token,
			ArtifactsFile: &artifactFile{Filename: fn, Size: size},
		})
	}
	return deps
}

// buildJobResponse renders a Job into the runner-API 201 wire shape. gitBase
// is the base URL the runner clones the project repo from (git_info.repo_url),
// reachable from the job/helper container.
func (s *Server) buildJobResponse(job *Job, gitBase string) jobResponse {
	var steps []jobStep
	if len(job.BeforeS) > 0 || len(job.Script) > 0 {
		steps = append(steps, jobStep{
			Name:    "script",
			Script:  append(append([]string{}, job.BeforeS...), job.Script...),
			Timeout: 3600,
			When:    "on_success",
		})
	}
	if len(job.AfterS) > 0 {
		steps = append(steps, jobStep{
			Name:    "after_script",
			Script:  job.AfterS,
			Timeout: 3600,
			When:    "always",
		})
	}
	var img *jobImage
	if job.Image != "" {
		img = &jobImage{Name: job.Image}
	}
	var services []jobImage
	for _, sv := range job.Services {
		services = append(services, jobImage{Name: sv.Name, Alias: sv.Alias})
	}
	proj, projPath := "", ""
	var deps []jobDependency
	s.mu.Lock()
	if p, ok := s.projects[job.ProjectID]; ok {
		proj = p.Name
		projPath = p.Path
	}
	deps = s.buildDependenciesLocked(job)
	s.mu.Unlock()

	artifacts := job.Artifacts
	if artifacts == nil {
		artifacts = []jobArtifactSpec{}
	}

	repoURL := ""
	if projPath != "" {
		repoURL = strings.TrimSuffix(gitBase, "/") + "/" + projPath + ".git"
	}

	vars := append([]JobVariable{
		{Key: "CI_JOB_ID", Value: strconv.Itoa(job.ID), Public: true},
		{Key: "CI_JOB_NAME", Value: job.Name, Public: true},
		{Key: "CI_JOB_STAGE", Value: job.Stage, Public: true},
		{Key: "CI_PROJECT_ID", Value: strconv.Itoa(job.ProjectID), Public: true},
		{Key: "CI_PROJECT_NAME", Value: proj, Public: true},
		{Key: "CI_PROJECT_PATH", Value: projPath, Public: true},
		{Key: "CI_COMMIT_REF_NAME", Value: job.Ref, Public: true},
		{Key: "CI_COMMIT_SHA", Value: job.SHA, Public: true},
		// The artifact uploader/downloader runs inside the job/helper
		// container and hits CI_API_V4_URL — which must be reachable from
		// there, not the runner process's loopback API URL. Same coordinate
		// as git_info.repo_url (host.docker.internal in the harness).
		{Key: "CI_API_V4_URL", Value: strings.TrimSuffix(gitBase, "/") + "/api/v4", Public: true},
	}, job.Variables...)

	return jobResponse{
		ID:            job.ID,
		Token:         job.Token,
		AllowGitFetch: false,
		JobInfo:       jobInfo{Name: job.Name, Stage: job.Stage, ProjectID: job.ProjectID, ProjectName: proj},
		GitInfo: gitInfo{
			RepoURL:  repoURL,
			Ref:      job.Ref,
			SHA:      job.SHA,
			RefType:  "branch",
			Refspecs: []string{"+refs/heads/" + job.Ref + ":refs/remotes/origin/" + job.Ref},
		},
		RunnerInfo:   runnerInfo{Timeout: 3600},
		Variables:    vars,
		Steps:        steps,
		Image:        img,
		Services:     services,
		Artifacts:    artifacts,
		Cache:        []any{},
		Credentials:  []any{},
		Dependencies: deps,
		// `features` is a mixed-type object: trace_sections is a bool, but
		// failure_reasons is a []JobFailureReason — so it can't be
		// map[string]bool. We advertise only trace_sections; omitting
		// failure_reasons leaves it empty, which the runner accepts.
		Features: map[string]any{"trace_sections": true},
	}
}
