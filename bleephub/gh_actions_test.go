package bleephub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

// seedRun installs a Workflow + WorkflowJob in the store and returns
// the full URL the GitHub-shape /actions/runs/{run_id} is keyed off.
// All fields needed by workflowRunJSON / workflowJobJSON are populated.
func seedRun(t *testing.T, s *Server, repo string, status, result string) (*Workflow, *WorkflowJob) {
	t.Helper()
	s.store.mu.Lock()
	runID := s.store.NextRunID
	s.store.NextRunID++
	jobID := uuid.New().String()
	wf := &Workflow{
		ID:           uuid.New().String(),
		Name:         "ci",
		RunID:        runID,
		RunNumber:    runID,
		Status:       status,
		Result:       result,
		CreatedAt:    time.Now(),
		EventName:    "push",
		Ref:          "refs/heads/main",
		Sha:          "abcdef0123456789abcdef0123456789abcdef01",
		RepoFullName: repo,
		Jobs:         map[string]*WorkflowJob{},
	}
	wfJob := &WorkflowJob{
		Key:         "build",
		JobID:       jobID,
		DisplayName: "Build",
		Status:      "completed",
		Result:      "success",
		StartedAt:   time.Now(),
	}
	wf.Jobs["build"] = wfJob
	s.store.Workflows[wf.ID] = wf
	s.store.LogLines[jobID] = []string{"line one", "line two\n"}
	s.store.mu.Unlock()
	return wf, wfJob
}

// runRequest exercises a route through the server's mux (so the path-
// pattern + handler wiring is also covered). Returns the recorder.
func runRequest(s *Server, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	return w
}

func TestActionsRuns_List(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	wf1, _ := seedRun(t, s, "octo/repo", "running", "")
	wf2, _ := seedRun(t, s, "octo/repo", "completed", "success")
	_, _ = seedRun(t, s, "other/repo", "completed", "success")

	w := runRequest(s, "GET", "/api/v3/repos/octo/repo/actions/runs")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		TotalCount   int              `json:"total_count"`
		WorkflowRuns []map[string]any `json:"workflow_runs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalCount != 2 {
		t.Errorf("total_count = %d, want 2 (other/repo run filtered out)", resp.TotalCount)
	}
	if len(resp.WorkflowRuns) != 2 {
		t.Errorf("workflow_runs len = %d, want 2", len(resp.WorkflowRuns))
	}

	gotIDs := map[float64]bool{}
	for _, r := range resp.WorkflowRuns {
		gotIDs[r["id"].(float64)] = true
	}
	if !gotIDs[float64(wf1.RunID)] || !gotIDs[float64(wf2.RunID)] {
		t.Errorf("missing expected run IDs in response: %v", gotIDs)
	}
}

func TestActionsRuns_List_StatusFilter(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	seedRun(t, s, "octo/repo", "running", "")
	seedRun(t, s, "octo/repo", "completed", "success")

	w := runRequest(s, "GET", "/api/v3/repos/octo/repo/actions/runs?status=in_progress")
	var resp struct {
		TotalCount   int              `json:"total_count"`
		WorkflowRuns []map[string]any `json:"workflow_runs"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.TotalCount != 1 {
		t.Errorf("status=in_progress filter: total_count = %d, want 1", resp.TotalCount)
	}
	if got := resp.WorkflowRuns[0]["status"]; got != "in_progress" {
		t.Errorf("filtered run status = %v, want in_progress", got)
	}
}

func TestActionsRuns_Get(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	wf, _ := seedRun(t, s, "octo/repo", "completed", "success")

	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d", wf.RunID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got map[string]any
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["id"].(float64) != float64(wf.RunID) {
		t.Errorf("id mismatch")
	}
	if got["status"] != "completed" {
		t.Errorf("status = %v", got["status"])
	}
	if got["conclusion"] != "success" {
		t.Errorf("conclusion = %v", got["conclusion"])
	}
	if got["head_branch"] != "main" {
		t.Errorf("head_branch = %v", got["head_branch"])
	}
}

func TestActionsRuns_Get_NotFound(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	w := runRequest(s, "GET", "/api/v3/repos/octo/repo/actions/runs/9999")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestActionsRunJobs_List(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	wf, wfJob := seedRun(t, s, "octo/repo", "completed", "success")

	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d/jobs", wf.RunID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	// Decode into typed struct so int64 IDs survive round-trip without
	// the float64 precision loss `map[string]any` would impose.
	var resp struct {
		TotalCount int `json:"total_count"`
		Jobs       []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalCount != 1 {
		t.Errorf("total_count = %d, want 1", resp.TotalCount)
	}
	if resp.Jobs[0].ID != stableJobID(wfJob.JobID) {
		t.Errorf("job id mismatch: got %d, want %d", resp.Jobs[0].ID, stableJobID(wfJob.JobID))
	}
	if resp.Jobs[0].Name != "Build" {
		t.Errorf("job name = %v", resp.Jobs[0].Name)
	}
}

func TestActionsJobs_Get(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	_, wfJob := seedRun(t, s, "octo/repo", "completed", "success")
	id := stableJobID(wfJob.JobID)

	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/jobs/%d", id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestActionsJobs_Logs(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	_, wfJob := seedRun(t, s, "octo/repo", "completed", "success")
	id := stableJobID(wfJob.JobID)

	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/jobs/%d/logs", id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body, _ := io.ReadAll(w.Body)
	got := string(body)
	if got != "line one\nline two\n" {
		t.Errorf("logs body = %q, want \"line one\\nline two\\n\"", got)
	}
}

func TestActionsRuns_Cancel(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	wf, _ := seedRun(t, s, "octo/repo", "running", "")
	wf.Jobs["build"].Status = "queued"

	w := runRequest(s, "POST", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d/cancel", wf.RunID))
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	if wf.Status != "completed" || wf.Result != "cancelled" {
		t.Errorf("after cancel: status=%s result=%s", wf.Status, wf.Result)
	}
}

func TestActionsRuns_Rerun_NotImplemented(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	wf, _ := seedRun(t, s, "octo/repo", "completed", "success")

	w := runRequest(s, "POST", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d/rerun", wf.RunID))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 (Phase 130 doesn't ship dispatch)", w.Code)
	}
}

func TestActionsRuns_Delete(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	wf, _ := seedRun(t, s, "octo/repo", "completed", "success")

	w := runRequest(s, "DELETE", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d", wf.RunID))
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
	if s.findWorkflowByRunID(wf.RunID) != nil {
		t.Error("workflow should be deleted from store")
	}
}

func TestActionsRunners_List(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	s.store.mu.Lock()
	s.store.Agents[1] = &Agent{
		ID: 1, Name: "runner-a", OSDescription: "Linux", Status: "online",
		Labels: []Label{{ID: 10, Name: "self-hosted", Type: "system"}, {ID: 11, Name: "linux", Type: "custom"}},
	}
	s.store.Agents[2] = &Agent{ID: 2, Name: "runner-b", OSDescription: "Darwin", Status: "offline"}
	s.store.mu.Unlock()

	w := runRequest(s, "GET", "/api/v3/repos/octo/repo/actions/runners")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp struct {
		TotalCount int              `json:"total_count"`
		Runners    []map[string]any `json:"runners"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.TotalCount != 2 {
		t.Errorf("total_count = %d, want 2", resp.TotalCount)
	}
	// Validate one runner's full shape.
	var found *map[string]any
	for i := range resp.Runners {
		if resp.Runners[i]["name"] == "runner-a" {
			found = &resp.Runners[i]
			break
		}
	}
	if found == nil {
		t.Fatal("runner-a not in response")
	}
	r := *found
	if r["os"] != "linux" {
		t.Errorf("os = %v, want linux", r["os"])
	}
	if r["status"] != "online" {
		t.Errorf("status = %v, want online", r["status"])
	}
	labels := r["labels"].([]any)
	if len(labels) != 2 {
		t.Errorf("labels len = %d, want 2", len(labels))
	}
	// system → read-only mapping
	for _, l := range labels {
		lm := l.(map[string]any)
		if lm["name"] == "self-hosted" && lm["type"] != "read-only" {
			t.Errorf("self-hosted label type = %v, want read-only", lm["type"])
		}
		if lm["name"] == "linux" && lm["type"] != "custom" {
			t.Errorf("linux label type = %v, want custom", lm["type"])
		}
	}
}

func TestActionsRunners_Delete(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	s.store.mu.Lock()
	s.store.Agents[42] = &Agent{ID: 42, Name: "to-delete", Status: "online"}
	s.store.mu.Unlock()

	w := runRequest(s, "DELETE", "/api/v3/repos/octo/repo/actions/runners/42")
	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", w.Code)
	}
	s.store.mu.RLock()
	_, exists := s.store.Agents[42]
	s.store.mu.RUnlock()
	if exists {
		t.Error("runner 42 should be deleted")
	}
}

func TestActionsRunners_Delete_NotFound(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	w := runRequest(s, "DELETE", "/api/v3/repos/octo/repo/actions/runners/9999")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestStableJobID_DeterministicAndPositive(t *testing.T) {
	// The cleanup join + the GitHub-int-shape contract both rely on a
	// stable, positive int64 derived from the WorkflowJob UUID.
	a := stableJobID("d3b07384-d113-440a-9b46-2c2eb6c0e1d2")
	b := stableJobID("d3b07384-d113-440a-9b46-2c2eb6c0e1d2")
	c := stableJobID("00000000-0000-0000-0000-000000000000")
	if a != b {
		t.Errorf("not deterministic: %d vs %d", a, b)
	}
	if a < 0 || c < 0 {
		t.Errorf("negative ID returned: a=%d c=%d", a, c)
	}
	if a == c {
		t.Errorf("collision on distinct UUIDs")
	}
}
