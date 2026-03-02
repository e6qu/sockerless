package gitlabhub

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestListPipelinesEmpty(t *testing.T) {
	s := newTestServer(t)
	rr := doRequest(s, "GET", "/internal/pipelines", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var pipelines []pipelineView
	json.NewDecoder(rr.Body).Decode(&pipelines)
	if len(pipelines) != 0 {
		t.Fatalf("expected 0 pipelines, got %d", len(pipelines))
	}
}

func TestListPipelinesWithData(t *testing.T) {
	s := newTestServer(t)

	// Create project and pipeline
	proj := s.store.CreateProject("test-project")
	s.store.mu.Lock()
	pl := &Pipeline{
		ID:        s.store.NextPipeline,
		ProjectID: proj.ID,
		Status:    "running",
		Result:    "",
		Ref:       "main",
		Sha:       "abc123",
		Stages:    []string{"build", "test"},
		Jobs:      make(map[string]*PipelineJob),
		CreatedAt: time.Now(),
	}
	s.store.NextPipeline++
	s.store.Pipelines[pl.ID] = pl
	s.store.mu.Unlock()

	rr := doRequest(s, "GET", "/internal/pipelines", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var pipelines []pipelineView
	json.NewDecoder(rr.Body).Decode(&pipelines)
	if len(pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d", len(pipelines))
	}
	if pipelines[0].ProjectName != "test-project" {
		t.Fatalf("expected project_name=test-project, got %s", pipelines[0].ProjectName)
	}
	if pipelines[0].Status != "running" {
		t.Fatalf("expected status=running, got %s", pipelines[0].Status)
	}
}

func TestGetPipeline(t *testing.T) {
	s := newTestServer(t)

	proj := s.store.CreateProject("my-project")
	s.store.mu.Lock()
	pl := &Pipeline{
		ID:        s.store.NextPipeline,
		ProjectID: proj.ID,
		Status:    "success",
		Result:    "success",
		Ref:       "main",
		Sha:       "def456",
		Stages:    []string{"build"},
		Jobs: map[string]*PipelineJob{
			"compile": {
				ID:         1,
				PipelineID: s.store.NextPipeline,
				Name:       "compile",
				Stage:      "build",
				Status:     "success",
				Result:     "success",
			},
		},
		CreatedAt: time.Now(),
	}
	s.store.NextPipeline++
	s.store.Pipelines[pl.ID] = pl
	s.store.mu.Unlock()

	rr := doRequest(s, "GET", "/internal/pipelines/1", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var view pipelineView
	json.NewDecoder(rr.Body).Decode(&view)
	if view.ProjectName != "my-project" {
		t.Fatalf("expected project_name=my-project, got %s", view.ProjectName)
	}
	if len(view.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(view.Jobs))
	}
}

func TestGetPipelineNotFound(t *testing.T) {
	s := newTestServer(t)
	rr := doRequest(s, "GET", "/internal/pipelines/999", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestGetPipelineLogs(t *testing.T) {
	s := newTestServer(t)

	proj := s.store.CreateProject("log-project")
	s.store.mu.Lock()
	pl := &Pipeline{
		ID:        s.store.NextPipeline,
		ProjectID: proj.ID,
		Status:    "success",
		Ref:       "main",
		Sha:       "abc",
		Stages:    []string{"test"},
		Jobs: map[string]*PipelineJob{
			"unit": {
				ID:         1,
				PipelineID: s.store.NextPipeline,
				Name:       "unit",
				Stage:      "test",
				Status:     "success",
				TraceData:  []byte("line1\nline2\nline3"),
			},
		},
		CreatedAt: time.Now(),
	}
	s.store.NextPipeline++
	s.store.Pipelines[pl.ID] = pl
	s.store.mu.Unlock()

	rr := doRequest(s, "GET", "/internal/pipelines/1/logs", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var logs map[string][]string
	json.NewDecoder(rr.Body).Decode(&logs)
	lines, ok := logs["unit"]
	if !ok {
		t.Fatal("expected log lines for job 'unit'")
	}
	if len(lines) != 3 {
		t.Fatalf("expected 3 log lines, got %d", len(lines))
	}
}

func TestListRunners(t *testing.T) {
	s := newTestServer(t)

	s.store.RegisterRunner("runner-1", []string{"docker"})
	s.store.RegisterRunner("runner-2", []string{"shell", "linux"})

	rr := doRequest(s, "GET", "/internal/runners", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var runners []runnerView
	json.NewDecoder(rr.Body).Decode(&runners)
	if len(runners) != 2 {
		t.Fatalf("expected 2 runners, got %d", len(runners))
	}
	// Should be sorted by ID
	if runners[0].ID > runners[1].ID {
		t.Fatal("expected runners sorted by ID")
	}
}

func TestListProjects(t *testing.T) {
	s := newTestServer(t)

	s.store.CreateProject("project-a")
	s.store.CreateProject("project-b")

	rr := doRequest(s, "GET", "/internal/projects", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var projects []projectView
	json.NewDecoder(rr.Body).Decode(&projects)
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
}
