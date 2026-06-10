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

func runAuthedRequest(s *Server, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	return w
}

func seedFinalizedArtifact(s *Server, id int64, wf *Workflow, name string, createdAt time.Time) {
	s.artifactStore.mu.Lock()
	s.artifactStore.artifacts[id] = &Artifact{
		ID:                   id,
		Name:                 name,
		Size:                 int64(len("artifact-data")),
		Data:                 []byte("artifact-data"),
		Finalized:            true,
		RunID:                wf.ID,
		GitHubRunID:          wf.RunID,
		RepoFullName:         wf.RepoFullName,
		WorkflowRunBackendID: wf.ID,
		CreatedAt:            createdAt,
	}
	if id >= s.artifactStore.nextID {
		s.artifactStore.nextID = id + 1
	}
	s.artifactStore.mu.Unlock()
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

func TestActionsArtifacts_ListRunArtifacts(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	s.registerGHActionsExtrasRoutes()
	s.store.CreateRepo(s.store.LookupUserByLogin("admin"), "repo", "", false)
	wf, _ := seedRun(t, s, "admin/repo", "completed", "success")
	other, _ := seedRun(t, s, "admin/repo", "completed", "success")
	seedFinalizedArtifact(s, 1, wf, "logs", time.Now().Add(-time.Minute))
	seedFinalizedArtifact(s, 2, other, "other-run", time.Now())

	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/admin/repo/actions/runs/%d/artifacts", wf.RunID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		TotalCount int `json:"total_count"`
		Artifacts  []struct {
			ID                 int64  `json:"id"`
			Name               string `json:"name"`
			SizeInBytes        int64  `json:"size_in_bytes"`
			ArchiveDownloadURL string `json:"archive_download_url"`
			Digest             string `json:"digest"`
			WorkflowRun        struct {
				ID           int64  `json:"id"`
				HeadBranch   string `json:"head_branch"`
				HeadSHA      string `json:"head_sha"`
				RepositoryID int64  `json:"repository_id"`
				HeadRepoID   int64  `json:"head_repository_id"`
			} `json:"workflow_run"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalCount != 1 || len(resp.Artifacts) != 1 {
		t.Fatalf("artifact count = total:%d len:%d, want 1", resp.TotalCount, len(resp.Artifacts))
	}
	artifact := resp.Artifacts[0]
	if artifact.ID != 1 || artifact.Name != "logs" || artifact.SizeInBytes != int64(len("artifact-data")) {
		t.Fatalf("artifact payload mismatch: %+v", artifact)
	}
	if artifact.ArchiveDownloadURL != "http://example.com/api/v3/repos/admin/repo/actions/artifacts/1/zip" {
		t.Fatalf("archive_download_url = %q", artifact.ArchiveDownloadURL)
	}
	if artifact.Digest == "" {
		t.Fatal("digest is empty")
	}
	if artifact.WorkflowRun.ID != int64(wf.RunID) || artifact.WorkflowRun.HeadBranch != "main" || artifact.WorkflowRun.HeadSHA != wf.Sha {
		t.Fatalf("workflow_run mismatch: %+v", artifact.WorkflowRun)
	}
	if artifact.WorkflowRun.RepositoryID == 0 || artifact.WorkflowRun.HeadRepoID == 0 {
		t.Fatalf("repository IDs not populated: %+v", artifact.WorkflowRun)
	}
}

func TestActionsArtifacts_ListRepoArtifactsWithNameFilter(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsExtrasRoutes()
	wf, _ := seedRun(t, s, "admin/repo", "completed", "success")
	otherRepo, _ := seedRun(t, s, "other/repo", "completed", "success")
	now := time.Now()
	seedFinalizedArtifact(s, 1, wf, "logs", now.Add(-2*time.Minute))
	seedFinalizedArtifact(s, 2, wf, "coverage", now)
	seedFinalizedArtifact(s, 3, otherRepo, "logs", now.Add(time.Minute))

	w := runRequest(s, "GET", "/api/v3/repos/admin/repo/actions/artifacts?name=logs")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		TotalCount int `json:"total_count"`
		Artifacts  []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalCount != 1 || len(resp.Artifacts) != 1 || resp.Artifacts[0].ID != 1 {
		t.Fatalf("filtered artifacts = %+v, total=%d; want only repo artifact 1", resp.Artifacts, resp.TotalCount)
	}
}

func TestActionsArtifacts_GetDownloadAndDelete(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsExtrasRoutes()
	wf, _ := seedRun(t, s, "admin/repo", "completed", "success")
	seedFinalizedArtifact(s, 1, wf, "logs", time.Now())

	getResp := runRequest(s, "GET", "/api/v3/repos/admin/repo/actions/artifacts/1")
	if getResp.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getResp.Code, getResp.Body.String())
	}
	var artifact struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(getResp.Body.Bytes(), &artifact); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if artifact.ID != 1 || artifact.Name != "logs" {
		t.Fatalf("artifact = %+v, want id=1 name=logs", artifact)
	}

	downloadResp := runRequest(s, "GET", "/api/v3/repos/admin/repo/actions/artifacts/1/zip")
	if downloadResp.Code != http.StatusFound {
		t.Fatalf("download status = %d, want 302; body=%s", downloadResp.Code, downloadResp.Body.String())
	}
	if got := downloadResp.Header().Get("Location"); got != "http://example.com/_apis/v1/artifacts/1/download" {
		t.Fatalf("download Location = %q", got)
	}

	deleteResp := runAuthedRequest(s, "DELETE", "/api/v3/repos/admin/repo/actions/artifacts/1")
	if deleteResp.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body=%s", deleteResp.Code, deleteResp.Body.String())
	}
	afterDelete := runRequest(s, "GET", "/api/v3/repos/admin/repo/actions/artifacts/1")
	if afterDelete.Code != http.StatusNotFound {
		t.Fatalf("after delete status = %d, want 404", afterDelete.Code)
	}
}

func TestActionsRuns_Cancel(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	wf, _ := seedRun(t, s, "octo/repo", "running", "")
	wf.Jobs["build"].Status = "queued"

	w := runAuthedRequest(s, "POST", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d/cancel", wf.RunID))
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

	w := runAuthedRequest(s, "POST", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d/rerun", wf.RunID))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 — rerun is unimplemented and must surface that, not silently succeed", w.Code)
	}
}

func TestActionsRuns_Delete(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	wf, _ := seedRun(t, s, "octo/repo", "completed", "success")

	w := runAuthedRequest(s, "DELETE", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d", wf.RunID))
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

	w := runAuthedRequest(s, "DELETE", "/api/v3/repos/octo/repo/actions/runners/42")
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
	w := runAuthedRequest(s, "DELETE", "/api/v3/repos/octo/repo/actions/runners/9999")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestActionsRun_WorkflowFileReferences(t *testing.T) {
	// workflow_id / workflow_url / path must reference the originating
	// workflow FILE (stable across runs), never the per-run RunID.
	s := newTestServer()
	s.registerGHActionsRoutes()
	wf, _ := seedRun(t, s, "octo/repo", "completed", "success")

	wantFileID := stableWorkflowFileID("octo/repo", ".github/workflows/ci.yml")
	if int64(wf.RunID) == wantFileID {
		t.Skip("RunID coincidentally equals derived file id; cannot distinguish")
	}

	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d", wf.RunID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got struct {
		ID          int64  `json:"id"`
		WorkflowID  int64  `json:"workflow_id"`
		Path        string `json:"path"`
		WorkflowURL string `json:"workflow_url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != int64(wf.RunID) {
		t.Errorf("id = %d, want run id %d", got.ID, wf.RunID)
	}
	if got.WorkflowID != wantFileID {
		t.Errorf("workflow_id = %d, want file id %d (not run id %d)", got.WorkflowID, wantFileID, wf.RunID)
	}
	if got.WorkflowID == int64(wf.RunID) {
		t.Errorf("workflow_id must not equal run id %d", wf.RunID)
	}
	if got.Path != ".github/workflows/ci.yml" {
		t.Errorf("path = %q, want .github/workflows/ci.yml", got.Path)
	}
	wantURL := fmt.Sprintf("http://example.com/api/v3/repos/octo/repo/actions/workflows/%d", wantFileID)
	if got.WorkflowURL != wantURL {
		t.Errorf("workflow_url = %q, want %q", got.WorkflowURL, wantURL)
	}
}

func TestActionsJob_StepsAndCompletedAt(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	_, wfJob := seedRun(t, s, "octo/repo", "completed", "success")
	// Give the job real step definitions + a completion time so the
	// synthesized step array and completed_at have something to reflect.
	s.store.mu.Lock()
	wfJob.Def = &JobDef{Steps: []StepDef{
		{Name: "Checkout", Uses: "actions/checkout@v4"},
		{Run: "go test ./..."},
	}}
	wfJob.CompletedAt = wfJob.StartedAt.Add(30 * time.Second)
	s.store.mu.Unlock()

	id := stableJobID(wfJob.JobID)
	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/jobs/%d", id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var got struct {
		CompletedAt *string `json:"completed_at"`
		Steps       []struct {
			Name        string  `json:"name"`
			Status      string  `json:"status"`
			Conclusion  string  `json:"conclusion"`
			Number      int     `json:"number"`
			StartedAt   *string `json:"started_at"`
			CompletedAt *string `json:"completed_at"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.CompletedAt == nil || *got.CompletedAt == "" {
		t.Fatal("completed_at must be set for a completed job")
	}
	if len(got.Steps) != 2 {
		t.Fatalf("steps len = %d, want 2", len(got.Steps))
	}
	if got.Steps[0].Name != "Checkout" || got.Steps[0].Number != 1 {
		t.Errorf("step 0 = %+v, want name=Checkout number=1", got.Steps[0])
	}
	if got.Steps[1].Name != "Run go test ./..." || got.Steps[1].Number != 2 {
		t.Errorf("step 1 = %+v, want name='Run go test ./...' number=2", got.Steps[1])
	}
	for i, st := range got.Steps {
		if st.Status != "completed" {
			t.Errorf("step %d status = %q, want completed", i, st.Status)
		}
		if st.Conclusion != "success" {
			t.Errorf("step %d conclusion = %q, want success", i, st.Conclusion)
		}
		if st.StartedAt == nil || st.CompletedAt == nil {
			t.Errorf("step %d timestamps not set: %+v", i, st)
		}
	}
}

func TestActionsRunners_ExtraFields(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	s.store.mu.Lock()
	s.store.Agents[7] = &Agent{ID: 7, Name: "r", OSDescription: "Linux", Status: "online", Version: "2.300.0"}
	s.store.mu.Unlock()

	w := runRequest(s, "GET", "/api/v3/repos/octo/repo/actions/runners")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp struct {
		Runners []struct {
			RunnerGroupID int64  `json:"runner_group_id"`
			Ephemeral     bool   `json:"ephemeral"`
			Version       string `json:"version"`
		} `json:"runners"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Runners) != 1 {
		t.Fatalf("runners len = %d, want 1", len(resp.Runners))
	}
	r := resp.Runners[0]
	if r.RunnerGroupID != 1 {
		t.Errorf("runner_group_id = %d, want 1", r.RunnerGroupID)
	}
	if r.Version != "2.300.0" {
		t.Errorf("version = %q, want 2.300.0", r.Version)
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
