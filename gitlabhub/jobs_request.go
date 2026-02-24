package gitlabhub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s *Server) registerJobRequestRoutes() {
	s.mux.HandleFunc("POST /api/v4/jobs/request", s.handleJobRequest)
}

// handleJobRequest handles POST /api/v4/jobs/request.
// This is the long-poll endpoint: the runner polls this to receive jobs.
func (s *Server) handleJobRequest(w http.ResponseWriter, r *http.Request) {
	var req JobRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	runner := s.store.LookupRunnerByToken(req.Token)
	if runner == nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// Try to dequeue a pending job immediately
	jobID := s.store.DequeueJob()
	if jobID != 0 {
		job := s.store.GetJob(jobID)
		if job != nil {
			s.sendJobToRunner(w, job)
			return
		}
	}

	// Long-poll: wait up to 30s for a job
	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout.C:
			w.WriteHeader(http.StatusNoContent)
			return
		case <-ticker.C:
			jobID := s.store.DequeueJob()
			if jobID != 0 {
				job := s.store.GetJob(jobID)
				if job != nil {
					s.sendJobToRunner(w, job)
					return
				}
			}
		case <-r.Context().Done():
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
}

// sendJobToRunner builds the full JobResponse and writes it to the runner.
func (s *Server) sendJobToRunner(w http.ResponseWriter, job *PipelineJob) {
	// Find pipeline and job def
	pipeline := s.store.GetPipeline(job.PipelineID)
	if pipeline == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var jobDef *PipelineJobDef
	if pipeline.Def != nil {
		jobDef = pipeline.Def.Jobs[job.Name]
	}

	// Build the response
	resp := s.buildJobResponse(pipeline, job, jobDef)

	// Mark job as running
	s.store.mu.Lock()
	job.Status = "running"
	job.StartedAt = time.Now()
	s.store.mu.Unlock()

	if s.metrics != nil {
		s.metrics.RecordJobDispatch()
	}

	s.logger.Info().
		Int("job_id", job.ID).
		Str("name", job.Name).
		Str("stage", job.Stage).
		Msg("job dispatched to runner")

	writeJSON(w, http.StatusCreated, resp)
}

// buildJobResponse constructs the full JobResponse for a pipeline job.
func (s *Server) buildJobResponse(pipeline *Pipeline, job *PipelineJob, jobDef *PipelineJobDef) *JobResponse {
	project := s.store.GetProject(pipeline.ProjectID)
	projectName := ""
	if project != nil {
		projectName = project.Name
	}

	serverURL := pipeline.ServerURL

	// Determine image
	image := pipeline.Image
	if jobDef != nil && jobDef.Image != "" {
		image = jobDef.Image
	}
	if image == "" && pipeline.Def != nil && pipeline.Def.Image != "" {
		image = pipeline.Def.Image
	}

	// Determine timeout
	timeout := 3600 // default 1 hour
	if job.Timeout > 0 {
		timeout = job.Timeout
	}

	resp := &JobResponse{
		ID:            job.ID,
		Token:         job.Token,
		AllowGitFetch: true,
		JobInfo: JobInfo{
			Name:        job.Name,
			Stage:       job.Stage,
			ProjectID:   pipeline.ProjectID,
			ProjectName: projectName,
		},
		GitInfo: GitInfo{
			RepoURL:   fmt.Sprintf("http://%s/%s.git", serverURL, projectName),
			Ref:       pipeline.Ref,
			Sha:       pipeline.Sha,
			BeforeSha: "0000000000000000000000000000000000000000",
			RefType:   "branch",
			Refspecs:  []string{"+refs/heads/*:refs/remotes/origin/*"},
			Depth:     50,
		},
		RunnerInfo: RunnerInfoRef{
			Timeout: timeout,
		},
		Variables: s.buildJobVariables(pipeline, job, jobDef),
		Image:     ImageDef{Name: image},
		Features: FeaturesDef{
			TraceSections:  true,
			TraceChecksum:  true,
			TraceSize:      true,
			FailureReasons: true,
		},
	}

	// Build steps from script
	if jobDef != nil {
		// Build the main script step
		var fullScript []string
		fullScript = append(fullScript, jobDef.BeforeScript...)
		fullScript = append(fullScript, jobDef.Script...)
		if len(fullScript) > 0 {
			resp.Steps = append(resp.Steps, StepDef{
				Name:         "script",
				Script:       fullScript,
				Timeout:      timeout,
				When:         "on_success",
				AllowFailure: job.AllowFailure,
			})
		}

		// Build after_script step
		if len(jobDef.AfterScript) > 0 {
			resp.Steps = append(resp.Steps, StepDef{
				Name:         "after_script",
				Script:       jobDef.AfterScript,
				Timeout:      timeout,
				When:         "always",
				AllowFailure: true,
			})
		}

		// Services
		hasDinD := false
		for _, svc := range jobDef.Services {
			sd := ServiceDef{
				Name:       svc.Name,
				Alias:      svc.Alias,
				Entrypoint: svc.Entrypoint,
				Command:    svc.Command,
				Variables:  svc.Variables,
			}
			resp.Services = append(resp.Services, sd)
			if isDinDService(svc.Name) {
				hasDinD = true
			}
		}

		// Inject DinD variables if detected
		if hasDinD {
			resp.Variables = injectDinDVars(resp.Variables)
		}

		// Artifacts
		if jobDef.Artifacts != nil {
			artWhen := "on_success"
			if jobDef.Artifacts.When != "" {
				artWhen = jobDef.Artifacts.When
			}
			resp.Artifacts = append(resp.Artifacts, ArtifactDef{
				Name:     "default",
				Paths:    jobDef.Artifacts.Paths,
				When:     artWhen,
				ExpireIn: jobDef.Artifacts.ExpireIn,
				Type:     "archive",
				Format:   "zip",
			})
		}

		// Cache
		if jobDef.Cache != nil {
			cacheKey := jobDef.Cache.Key
			if cacheKey == "" {
				cacheKey = "default"
			}
			policy := jobDef.Cache.Policy
			if policy == "" {
				policy = "pull-push"
			}
			resp.Cache = append(resp.Cache, CacheDef{
				Key:   cacheKey,
				Paths: jobDef.Cache.Paths,
				Policy: policy,
			})
		}

		// Dependencies (artifacts from prior jobs)
		resp.Dependencies = s.buildDependencies(pipeline, job, jobDef)

		// Inject dotenv variables from dependencies
		resp.Variables = s.injectDotenvVars(pipeline, job, jobDef, resp.Variables)
	}

	return resp
}

// buildDependencies builds the dependencies array for artifact download.
func (s *Server) buildDependencies(pipeline *Pipeline, job *PipelineJob, jobDef *PipelineJobDef) []Dependency {
	// If explicit dependencies keyword used, use that. Otherwise, use all jobs from prior stages.
	var depJobNames []string
	if len(jobDef.Dependencies) > 0 {
		depJobNames = jobDef.Dependencies
	} else if len(jobDef.Needs) > 0 {
		depJobNames = jobDef.Needs
	} else {
		// All completed jobs from prior stages
		stageIdx := stageIndex(pipeline.Stages, job.Stage)
		for _, pj := range pipeline.Jobs {
			if stageIndex(pipeline.Stages, pj.Stage) < stageIdx && pj.Status == "success" {
				depJobNames = append(depJobNames, pj.Name)
			}
		}
	}

	var deps []Dependency
	for _, name := range depJobNames {
		pj, ok := pipeline.Jobs[name]
		if !ok {
			continue
		}
		artData := s.store.GetArtifact(pj.ID)
		if artData == nil {
			continue
		}
		deps = append(deps, Dependency{
			ID:    pj.ID,
			Name:  name,
			Token: pj.Token,
			ArtifactsFile: ArtifactsFile{
				Filename: "artifacts.zip",
				Size:     int64(len(artData)),
			},
		})
	}
	return deps
}

// injectDotenvVars adds dotenv variables from dependency jobs to the variable list.
func (s *Server) injectDotenvVars(pipeline *Pipeline, job *PipelineJob, jobDef *PipelineJobDef, vars []VariableDef) []VariableDef {
	// Collect dotenv vars from all dependency jobs
	depNames := jobDef.Dependencies
	if len(depNames) == 0 {
		depNames = jobDef.Needs
	}
	if len(depNames) == 0 {
		// All prior stage jobs
		stageIdx := stageIndex(pipeline.Stages, job.Stage)
		for _, pj := range pipeline.Jobs {
			if stageIndex(pipeline.Stages, pj.Stage) < stageIdx {
				depNames = append(depNames, pj.Name)
			}
		}
	}

	for _, name := range depNames {
		pj, ok := pipeline.Jobs[name]
		if !ok || pj.DotenvVars == nil {
			continue
		}
		for k, v := range pj.DotenvVars {
			vars = append(vars, VariableDef{
				Key:    k,
				Value:  v,
				Public: true,
			})
		}
	}
	return vars
}

func stageIndex(stages []string, stage string) int {
	for i, s := range stages {
		if s == stage {
			return i
		}
	}
	return -1
}

// isDinDService checks if a service image is Docker-in-Docker.
func isDinDService(image string) bool {
	// Match docker:dind, docker:*-dind, docker:latest-dind, docker:24.0-dind, etc.
	if image == "docker:dind" {
		return true
	}
	// Check for docker:<tag>-dind pattern
	if strings.HasPrefix(image, "docker:") && strings.HasSuffix(image, "-dind") {
		return true
	}
	return false
}

// injectDinDVars adds Docker-in-Docker environment variables to the job variables.
func injectDinDVars(vars []VariableDef) []VariableDef {
	dindVars := map[string]string{
		"DOCKER_HOST":        "tcp://docker:2375",
		"DOCKER_TLS_CERTDIR": "",
		"DOCKER_DRIVER":      "overlay2",
	}
	for key, val := range dindVars {
		// Don't override if already set
		found := false
		for _, v := range vars {
			if v.Key == key {
				found = true
				break
			}
		}
		if !found {
			vars = append(vars, VariableDef{
				Key:    key,
				Value:  val,
				Public: true,
			})
		}
	}
	return vars
}
