package bleeplab

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"
)

// internal_api.go serves a thin read-only aggregation surface the embedded UI
// consumes for its dashboard — projections over the in-memory control-plane
// state that have no clean GitLab public-API equivalent (e.g. "every pipeline
// across every project"). Resource detail still comes from the public /api/v4
// surface where it exists. Read-only; never mutates state.

type statusView struct {
	Projects          int            `json:"projects"`
	Pipelines         int            `json:"pipelines"`
	PipelinesByStatus map[string]int `json:"pipelines_by_status"`
	Jobs              int            `json:"jobs"`
	JobsByStatus      map[string]int `json:"jobs_by_status"`
	ConnectedRunners  int            `json:"connected_runners"`
	UptimeSeconds     int            `json:"uptime_seconds"`
}

type projectView struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	DefaultBranch string `json:"default_branch"`
	SHA           string `json:"sha"`
	Pipelines     int    `json:"pipelines"`
}

type pipelineView struct {
	ID          int      `json:"id"`
	ProjectID   int      `json:"project_id"`
	ProjectName string   `json:"project_name"`
	Ref         string   `json:"ref"`
	SHA         string   `json:"sha"`
	Status      string   `json:"status"`
	Stages      []string `json:"stages"`
	Jobs        int      `json:"jobs"`
}

type jobView struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	Stage            string `json:"stage"`
	Status           string `json:"status"`
	Ref              string `json:"ref"`
	SHA              string `json:"sha"`
	ProjectID        int    `json:"project_id"`
	ArtifactSize     int64  `json:"artifact_size"`
	ArtifactFilename string `json:"artifact_filename,omitempty"`
	Trace            string `json:"trace,omitempty"`
}

type pipelineDetailView struct {
	pipelineView
	JobList []jobView `json:"job_list"`
}

type storageView struct {
	Git       backendView `json:"git"`
	Artifacts backendView `json:"artifacts"`
}

type backendView struct {
	Backend string `json:"backend"` // s3 | filesystem | memory
	Detail  string `json:"detail"`
}

func (s *Server) handleInternalStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	v := statusView{
		Projects:          len(s.projects),
		Pipelines:         len(s.pipelines),
		PipelinesByStatus: map[string]int{},
		Jobs:              len(s.jobs),
		JobsByStatus:      map[string]int{},
		ConnectedRunners:  len(s.runners),
		UptimeSeconds:     int(time.Since(s.started).Seconds()),
	}
	for _, pl := range s.pipelines {
		v.PipelinesByStatus[pl.Status]++
	}
	for _, j := range s.jobs {
		j.mu.Lock()
		v.JobsByStatus[j.Status]++
		j.mu.Unlock()
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleInternalProjects(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	counts := map[int]int{}
	for _, pl := range s.pipelines {
		counts[pl.ProjectID]++
	}
	out := make([]projectView, 0, len(s.projects))
	for _, p := range s.projects {
		out = append(out, projectView{
			ID: p.ID, Name: p.Name, Path: p.Path,
			DefaultBranch: p.Ref, SHA: p.SHA, Pipelines: counts[p.ID],
		})
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleInternalPipelines(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	out := make([]pipelineView, 0, len(s.pipelines))
	for _, pl := range s.pipelines {
		out = append(out, s.pipelineViewLocked(pl))
	}
	s.mu.Unlock()
	// Newest first — pipeline IDs increase monotonically.
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) pipelineViewLocked(pl *Pipeline) pipelineView {
	name := ""
	if p, ok := s.projects[pl.ProjectID]; ok {
		name = p.Name
	}
	return pipelineView{
		ID: pl.ID, ProjectID: pl.ProjectID, ProjectName: name,
		Ref: pl.Ref, SHA: pl.SHA, Status: pl.Status,
		Stages: pl.Stages, Jobs: len(pl.JobIDs),
	}
}

func (s *Server) handleInternalPipeline(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	s.mu.Lock()
	pl, ok := s.pipelines[id]
	if !ok {
		s.mu.Unlock()
		http.Error(w, "404 pipeline not found", http.StatusNotFound)
		return
	}
	detail := pipelineDetailView{pipelineView: s.pipelineViewLocked(pl)}
	for _, jid := range pl.JobIDs {
		if j := s.jobs[jid]; j != nil {
			detail.JobList = append(detail.JobList, s.jobViewLocked(j, false))
		}
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) jobViewLocked(j *Job, withTrace bool) jobView {
	j.mu.Lock()
	defer j.mu.Unlock()
	v := jobView{
		ID: j.ID, Name: j.Name, Stage: j.Stage, Status: j.Status,
		Ref: j.Ref, SHA: j.SHA, ProjectID: j.ProjectID,
		ArtifactSize: j.ArtifactSize, ArtifactFilename: j.ArtifactFilename,
	}
	if withTrace {
		v.Trace = j.trace.String()
	}
	return v
}

func (s *Server) handleInternalJob(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	s.mu.Lock()
	j, ok := s.jobs[id]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "404 job not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, s.jobViewLocked(j, true))
}

// handleInternalArtifactDownload serves a job's artifact archive to the UI.
// Unlike the runner-facing `/api/v4/jobs/{id}/artifacts` route it is NOT
// JOB-TOKEN gated, so a browser anchor can download it directly; it carries
// the recorded artifact filename via Content-Disposition.
func (s *Server) handleInternalArtifactDownload(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	s.mu.Lock()
	job, ok := s.jobs[id]
	s.mu.Unlock()
	if !ok {
		http.Error(w, "404 job not found", http.StatusNotFound)
		return
	}
	store, err := s.getArtifactStore(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, ok, err := store.Get(artifactKey(id))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "404 artifact not found", http.StatusNotFound)
		return
	}
	job.mu.Lock()
	filename := job.ArtifactFilename
	job.mu.Unlock()
	if filename == "" {
		filename = "artifacts.zip"
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleInternalRunners(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	type runnerView struct {
		ID    int    `json:"id"`
		Token string `json:"token"`
	}
	out := make([]runnerView, 0, len(s.runners))
	for _, rn := range s.runners {
		out = append(out, runnerView{ID: rn.ID, Token: rn.Token})
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleInternalStorage(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, storageView{
		Git:       gitBackendView(),
		Artifacts: artifactsBackendView(),
	})
}

func gitBackendView() backendView {
	if bucket := os.Getenv("BLEEPLAB_S3_BUCKET"); bucket != "" {
		return backendView{Backend: "s3", Detail: bucket}
	}
	if dir := GitDataDir(); dir != "" {
		return backendView{Backend: "filesystem", Detail: dir}
	}
	return backendView{Backend: "memory", Detail: "in-process"}
}

func artifactsBackendView() backendView {
	if bucket := os.Getenv("BLEEPLAB_S3_BUCKET"); bucket != "" {
		return backendView{Backend: "s3", Detail: bucket + " (artifacts/)"}
	}
	if dir := os.Getenv("BLEEPLAB_ARTIFACTS_DIR"); dir != "" {
		return backendView{Backend: "filesystem", Detail: dir}
	}
	return backendView{Backend: "memory", Detail: "in-process"}
}
