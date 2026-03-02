package gitlabhub

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
)

func (s *Server) registerMgmtRoutes() {
	s.mux.HandleFunc("GET /internal/pipelines", s.handleListPipelines)
	s.mux.HandleFunc("GET /internal/pipelines/{id}", s.handleGetPipeline)
	s.mux.HandleFunc("GET /internal/pipelines/{id}/logs", s.handleGetPipelineLogs)
	s.mux.HandleFunc("GET /internal/runners", s.handleListRunners)
	s.mux.HandleFunc("GET /internal/projects", s.handleListProjects)
}

// pipelineView is the JSON representation of a pipeline for the management API.
type pipelineView struct {
	ID          int                       `json:"id"`
	ProjectID   int                       `json:"project_id"`
	ProjectName string                    `json:"project_name"`
	Status      string                    `json:"status"`
	Result      string                    `json:"result"`
	Ref         string                    `json:"ref"`
	Sha         string                    `json:"sha"`
	Stages      []string                  `json:"stages"`
	Jobs        map[string]*pipelineJobView `json:"jobs"`
	CreatedAt   string                    `json:"created_at"`
}

// pipelineJobView is the JSON representation of a job for the management API.
type pipelineJobView struct {
	ID            int      `json:"id"`
	PipelineID    int      `json:"pipeline_id"`
	Name          string   `json:"name"`
	Stage         string   `json:"stage"`
	Status        string   `json:"status"`
	Result        string   `json:"result"`
	AllowFailure  bool     `json:"allow_failure"`
	When          string   `json:"when"`
	Needs         []string `json:"needs,omitempty"`
	StartedAt     string   `json:"started_at,omitempty"`
	RetryCount    int      `json:"retry_count,omitempty"`
	MaxRetries    int      `json:"max_retries,omitempty"`
	Timeout       int      `json:"timeout,omitempty"`
	MatrixGroup   string   `json:"matrix_group,omitempty"`
	ResourceGroup string   `json:"resource_group,omitempty"`
}

func jobToView(j *PipelineJob) *pipelineJobView {
	v := &pipelineJobView{
		ID:            j.ID,
		PipelineID:    j.PipelineID,
		Name:          j.Name,
		Stage:         j.Stage,
		Status:        j.Status,
		Result:        j.Result,
		AllowFailure:  j.AllowFailure,
		When:          j.When,
		Needs:         j.Needs,
		RetryCount:    j.RetryCount,
		MaxRetries:    j.MaxRetries,
		Timeout:       j.Timeout,
		MatrixGroup:   j.MatrixGroup,
		ResourceGroup: j.ResourceGroup,
	}
	if !j.StartedAt.IsZero() {
		v.StartedAt = j.StartedAt.Format("2006-01-02T15:04:05Z")
	}
	return v
}

func (s *Server) pipelineToView(pl *Pipeline) pipelineView {
	jobs := make(map[string]*pipelineJobView, len(pl.Jobs))
	for k, j := range pl.Jobs {
		jobs[k] = jobToView(j)
	}
	projectName := ""
	if p := s.store.Projects[pl.ProjectID]; p != nil {
		projectName = p.Name
	}
	return pipelineView{
		ID:          pl.ID,
		ProjectID:   pl.ProjectID,
		ProjectName: projectName,
		Status:      pl.Status,
		Result:      pl.Result,
		Ref:         pl.Ref,
		Sha:         pl.Sha,
		Stages:      pl.Stages,
		Jobs:        jobs,
		CreatedAt:   pl.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func (s *Server) handleListPipelines(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	pipelines := make([]pipelineView, 0, len(s.store.Pipelines))
	for _, pl := range s.store.Pipelines {
		pipelines = append(pipelines, s.pipelineToView(pl))
	}
	s.store.mu.RUnlock()

	sort.Slice(pipelines, func(i, j int) bool {
		return pipelines[i].CreatedAt > pipelines[j].CreatedAt
	})

	writeJSON(w, http.StatusOK, pipelines)
}

func (s *Server) handleGetPipeline(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid pipeline id", http.StatusBadRequest)
		return
	}

	s.store.mu.RLock()
	pl, ok := s.store.Pipelines[id]
	if !ok {
		s.store.mu.RUnlock()
		http.Error(w, "pipeline not found", http.StatusNotFound)
		return
	}
	view := s.pipelineToView(pl)
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGetPipelineLogs(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid pipeline id", http.StatusBadRequest)
		return
	}

	s.store.mu.RLock()
	pl, ok := s.store.Pipelines[id]
	if !ok {
		s.store.mu.RUnlock()
		http.Error(w, "pipeline not found", http.StatusNotFound)
		return
	}

	logs := make(map[string][]string, len(pl.Jobs))
	for _, job := range pl.Jobs {
		if len(job.TraceData) > 0 {
			lines := strings.Split(string(job.TraceData), "\n")
			logs[job.Name] = lines
		}
	}
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, logs)
}

// runnerView is the JSON representation of a runner for the management API.
type runnerView struct {
	ID          int      `json:"id"`
	Description string   `json:"description"`
	Active      bool     `json:"active"`
	Tags        []string `json:"tag_list"`
}

func (s *Server) handleListRunners(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	runners := make([]runnerView, 0, len(s.store.Runners))
	for _, runner := range s.store.Runners {
		runners = append(runners, runnerView{
			ID:          runner.ID,
			Description: runner.Description,
			Active:      runner.Active,
			Tags:        runner.Tags,
		})
	}
	s.store.mu.RUnlock()

	sort.Slice(runners, func(i, j int) bool {
		return runners[i].ID < runners[j].ID
	})

	writeJSON(w, http.StatusOK, runners)
}

// projectView is the JSON representation of a project for the management API.
type projectView struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	projects := make([]projectView, 0, len(s.store.Projects))
	for _, p := range s.store.Projects {
		projects = append(projects, projectView{
			ID:   p.ID,
			Name: p.Name,
		})
	}
	s.store.mu.RUnlock()

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].ID < projects[j].ID
	})

	writeJSON(w, http.StatusOK, projects)
}
