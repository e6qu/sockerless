package bleeplab

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/rs/zerolog"
)

// TestInternalAPI drives the read-only dashboard surface end to end: seed a
// project + commit + runner + pipeline through the public GitLab API, then
// assert the /internal projections the embedded UI consumes.
func TestInternalAPI(t *testing.T) {
	s := NewServer(":0", zerolog.Nop())
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	_, body := do(t, ts, http.MethodPost, "/api/v4/projects", map[string]any{"name": "demo"}, nil)
	var proj struct {
		ID int `json:"id"`
	}
	mustJSON(t, body, &proj)
	ci := "stages: [build, test]\nbuild-job:\n  stage: build\n  script: [make]\ntest-job:\n  stage: test\n  script: [run]\n"
	do(t, ts, http.MethodPost, "/api/v4/projects/"+strconv.Itoa(proj.ID)+"/repository/commits",
		map[string]any{"branch": "main", "actions": []map[string]string{{"file_path": ".gitlab-ci.yml", "content": ci}}}, nil)
	do(t, ts, http.MethodPost, "/api/v4/user/runners", map[string]any{"runner_type": "project_type"}, nil)
	_, body = do(t, ts, http.MethodPost, "/api/v4/projects/"+strconv.Itoa(proj.ID)+"/pipeline", map[string]any{"ref": "main"}, nil)
	var pl struct {
		ID int `json:"id"`
	}
	mustJSON(t, body, &pl)

	// /internal/status
	_, body = do(t, ts, http.MethodGet, "/internal/status", nil, nil)
	var st statusView
	mustJSON(t, body, &st)
	if st.Projects != 1 || st.Pipelines != 1 || st.Jobs != 2 || st.ConnectedRunners != 1 {
		t.Fatalf("status mismatch: %+v", st)
	}

	// /internal/projects
	_, body = do(t, ts, http.MethodGet, "/internal/projects", nil, nil)
	var projects []projectView
	mustJSON(t, body, &projects)
	if len(projects) != 1 || projects[0].Path != "root/demo" || projects[0].Pipelines != 1 {
		t.Fatalf("projects mismatch: %+v", projects)
	}

	// /internal/pipelines/{id} → jobs
	_, body = do(t, ts, http.MethodGet, "/internal/pipelines/"+strconv.Itoa(pl.ID), nil, nil)
	var detail pipelineDetailView
	mustJSON(t, body, &detail)
	if detail.ProjectName != "demo" || len(detail.JobList) != 2 {
		t.Fatalf("pipeline detail mismatch: %+v", detail)
	}

	// /internal/storage defaults to in-memory backends
	_, body = do(t, ts, http.MethodGet, "/internal/storage", nil, nil)
	var storage storageView
	mustJSON(t, body, &storage)
	if storage.Git.Backend != "memory" || storage.Artifacts.Backend != "memory" {
		t.Fatalf("storage mismatch: %+v", storage)
	}

	// /internal/runners
	_, body = do(t, ts, http.MethodGet, "/internal/runners", nil, nil)
	var runners []json.RawMessage
	mustJSON(t, body, &runners)
	if len(runners) != 1 {
		t.Fatalf("expected 1 runner, got %d", len(runners))
	}
}
