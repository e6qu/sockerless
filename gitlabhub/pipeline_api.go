package gitlabhub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

func (s *Server) registerPipelineAPIRoutes() {
	s.mux.HandleFunc("POST /api/v3/gitlabhub/pipeline", s.handleSubmitPipeline)
	s.mux.HandleFunc("GET /api/v3/gitlabhub/pipelines/{id}", s.handleGetPipelineStatus)
	s.mux.HandleFunc("POST /api/v3/gitlabhub/pipelines/{id}/cancel", s.handleCancelPipeline)
}

// handleSubmitPipeline handles POST /api/v3/gitlabhub/pipeline.
// Management API: accepts raw .gitlab-ci.yml YAML, creates project + git repo + pipeline.
func (s *Server) handleSubmitPipeline(w http.ResponseWriter, r *http.Request) {
	var req PipelineSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Pipeline == "" {
		http.Error(w, "pipeline YAML is required", http.StatusBadRequest)
		return
	}

	// Check concurrent pipeline limit
	s.store.mu.RLock()
	activePLs := 0
	for _, pl := range s.store.Pipelines {
		if pl.Status == "running" || pl.Status == "pending" {
			activePLs++
		}
	}
	s.store.mu.RUnlock()

	if activePLs >= s.maxConcurrentPipelines {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "max concurrent pipelines reached",
		})
		return
	}

	// Create an in-memory project
	projectName := fmt.Sprintf("project-%d", s.store.NextProject)
	project := s.store.CreateProject(projectName)

	// Create a git repo with the pipeline files.
	// The repo must exist before include resolution so local files can be read.
	repoFiles := map[string]string{
		".gitlab-ci.yml": req.Pipeline,
	}
	// Also store any extra files passed in the request
	for path, content := range req.Files {
		repoFiles[path] = content
	}

	sha, err := s.createProjectRepo(projectName, repoFiles)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("create git repo: %s", err),
		})
		return
	}

	// Resolve include:local: directives using the git storage
	gitStor := s.store.GetGitStorage(projectName)
	resolvedYAML, err := ResolveIncludes([]byte(req.Pipeline), gitStor)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("resolve includes: %s", err),
		})
		return
	}

	// Parse the pipeline YAML (after include resolution)
	def, err := ParsePipeline(resolvedYAML)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("parse pipeline: %s", err),
		})
		return
	}

	s.logger.Info().
		Str("project", projectName).
		Str("sha", sha[:8]).
		Int("num_jobs", len(def.Jobs)).
		Msg("pipeline submitted via management API")

	// Determine server URL (from Host header or default)
	serverURL := r.Host
	if serverURL == "" {
		serverURL = "localhost"
	}

	// Submit the pipeline
	pipeline, err := s.submitPipeline(r.Context(), project, def, serverURL, req.Image)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("submit pipeline: %s", err),
		})
		return
	}

	// Build response
	jobViews := make(map[string]*PipelineJobView)
	for name, job := range pipeline.Jobs {
		jobViews[name] = &PipelineJobView{
			ID:     job.ID,
			Name:   job.Name,
			Stage:  job.Stage,
			Status: job.Status,
		}
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"pipelineId": pipeline.ID,
		"status":     pipeline.Status,
		"jobs":       jobViews,
	})
}

// handleGetPipelineStatus handles GET /api/v3/gitlabhub/pipelines/:id.
func (s *Server) handleGetPipelineStatus(w http.ResponseWriter, r *http.Request) {
	pipelineID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid pipeline ID", http.StatusBadRequest)
		return
	}

	pipeline := s.store.GetPipeline(pipelineID)
	if pipeline == nil {
		http.Error(w, "pipeline not found", http.StatusNotFound)
		return
	}

	jobViews := make(map[string]*PipelineJobView)
	for name, job := range pipeline.Jobs {
		jobViews[name] = &PipelineJobView{
			ID:     job.ID,
			Name:   job.Name,
			Stage:  job.Stage,
			Status: job.Status,
			Result: job.Result,
		}
	}

	writeJSON(w, http.StatusOK, PipelineStatusResponse{
		ID:        pipeline.ID,
		Status:    pipeline.Status,
		Result:    pipeline.Result,
		Jobs:      jobViews,
		CreatedAt: pipeline.CreatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

// handleCancelPipeline handles POST /api/v3/gitlabhub/pipelines/:id/cancel.
func (s *Server) handleCancelPipeline(w http.ResponseWriter, r *http.Request) {
	pipelineID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid pipeline ID", http.StatusBadRequest)
		return
	}

	pipeline := s.store.GetPipeline(pipelineID)
	if pipeline == nil {
		http.Error(w, "pipeline not found", http.StatusNotFound)
		return
	}

	s.cancelPipeline(pipeline)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":     pipeline.ID,
		"status": pipeline.Status,
	})
}

// parseJSON decodes a JSON body into v.
func parseJSON(body io.Reader, v interface{}) error {
	return json.NewDecoder(body).Decode(v)
}
