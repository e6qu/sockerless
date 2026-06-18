package bleeplab

import (
	"net/http"
	"strconv"
)

// handleProjectCreate creates a project (`POST /api/v4/projects`).
func (s *Server) handleProjectCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		// Real GitLab rejects a project create with no name.
		http.Error(w, `{"message":{"name":["can't be blank"]}}`, http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	id := s.nextID
	s.nextID++
	p := &Project{ID: id, Name: req.Name, Path: "root/" + req.Name, Ref: "main"}
	s.projects[id] = p
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "name": req.Name, "path_with_namespace": p.Path,
		"default_branch": "main",
	})
}

// handleCommitCreate stores files committed to a project. bleeplab only
// cares about `.gitlab-ci.yml` — it becomes the project's pipeline config.
func (s *Server) handleCommitCreate(w http.ResponseWriter, r *http.Request) {
	pid, _ := strconv.Atoi(r.PathValue("id"))
	var req struct {
		Branch  string `json:"branch"`
		Actions []struct {
			FilePath string `json:"file_path"`
			Content  string `json:"content"`
		} `json:"actions"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	p, ok := s.projects[pid]
	if ok {
		for _, a := range req.Actions {
			if a.FilePath == ".gitlab-ci.yml" {
				p.CIConfig = a.Content
			}
		}
		if req.Branch != "" {
			p.Ref = req.Branch
		}
	}
	repoPath, branch := "", ""
	if ok {
		repoPath, branch = p.Path, p.Ref
	}
	s.mu.Unlock()
	if !ok {
		http.Error(w, "404 project not found", http.StatusNotFound)
		return
	}

	// Materialize the commit in the project's git repo so a real gitlab-runner
	// can clone it (creating CI_PROJECT_DIR). The runner clones the same repo
	// it would on gitlab.com — bleeplab differs only in coordinates.
	files := make(map[string]string, len(req.Actions))
	for _, a := range req.Actions {
		files[a.FilePath] = a.Content
	}
	sha, err := s.seedCommit(r.Context(), repoPath, branch, files)
	if err != nil {
		s.logger.Error().Err(err).Str("repo", repoPath).Msg("seed commit failed")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	p.SHA = sha
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, map[string]any{"id": sha, "short_id": sha[:8]})
}

// handlePipelineCreate triggers a pipeline (`POST /api/v4/projects/:id/pipeline`).
func (s *Server) handlePipelineCreate(w http.ResponseWriter, r *http.Request) {
	pid, _ := strconv.Atoi(r.PathValue("id"))
	var req struct {
		Ref string `json:"ref"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	p, ok := s.projects[pid]
	if !ok {
		s.mu.Unlock()
		http.Error(w, "404 project not found", http.StatusNotFound)
		return
	}
	ref := req.Ref
	if ref == "" {
		ref = p.Ref
	}
	sha := p.SHA
	if sha == "" {
		sha = commitSHA(p.CIConfig)
	}
	pl, err := s.createPipelineLocked(p, ref, sha)
	s.mu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	s.logger.Info().Int("pipeline", pl.ID).Int("project", pid).Int("jobs", len(pl.JobIDs)).Msg("pipeline created")
	writeJSON(w, http.StatusCreated, map[string]any{"id": pl.ID, "status": pl.Status, "ref": ref})
}

func (s *Server) handlePipelineList(w http.ResponseWriter, r *http.Request) {
	pid, _ := strconv.Atoi(r.PathValue("id"))
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []map[string]any
	for _, pl := range s.pipelines {
		if pl.ProjectID == pid {
			out = append(out, map[string]any{"id": pl.ID, "status": pl.Status, "ref": pl.Ref})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handlePipelineGet(w http.ResponseWriter, r *http.Request) {
	plID, _ := strconv.Atoi(r.PathValue("pid"))
	s.mu.Lock()
	pl, ok := s.pipelines[plID]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "404 pipeline not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": pl.ID, "status": pl.Status, "ref": pl.Ref})
}

func (s *Server) handlePipelineJobs(w http.ResponseWriter, r *http.Request) {
	plID, _ := strconv.Atoi(r.PathValue("pid"))
	s.mu.Lock()
	defer s.mu.Unlock()
	pl, ok := s.pipelines[plID]
	if !ok {
		http.Error(w, "404 pipeline not found", http.StatusNotFound)
		return
	}
	var out []map[string]any
	for _, jid := range pl.JobIDs {
		j := s.jobs[jid]
		j.mu.Lock()
		out = append(out, map[string]any{"id": j.ID, "name": j.Name, "stage": j.Stage, "status": j.Status})
		j.mu.Unlock()
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleProjectJobTrace(w http.ResponseWriter, r *http.Request) {
	jid, _ := strconv.Atoi(r.PathValue("jid"))
	s.mu.Lock()
	job, ok := s.jobs[jid]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "404 job not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(job.traceString()))
}
