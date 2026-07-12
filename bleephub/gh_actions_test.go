package bleephub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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
		Status:       WorkflowStatus(status),
		Result:       Result(result),
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
	s.registerTimelineRoutes()
	_, wfJob := seedRun(t, s, "octo/repo", "completed", "success")
	planID, timelineID := linkJobToPlan(t, s, wfJob)
	logID := createLogFile(t, s, planID)
	uploadLogBlock(t, s, planID, logID, []byte("uploaded line one\nuploaded line two\n"))
	patchTimelineRecords(t, s, planID, timelineID, true, []map[string]any{
		{"id": uuid.New().String(), "type": "Task", "name": "logs", "order": 1,
			"state": "completed", "result": "succeeded", "log": map[string]any{"id": logID}},
	})
	id := stableJobID(wfJob.JobID)

	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/jobs/%d/logs", id))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body, _ := io.ReadAll(w.Body)
	got := string(body)
	if got != "uploaded line one\nuploaded line two\n" {
		t.Errorf("logs body = %q, want uploaded log bytes", got)
	}
}

func TestActionsRunAndJobEndpointsScopeIDsToPathRepository(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	s.registerGHActionsExtrasRoutes()
	admin := s.store.LookupUserByLogin("admin")
	s.store.CreateRepo(admin, "repo-a", "", false)
	s.store.CreateRepo(admin, "repo-b", "", false)
	wf, wfJob := seedRun(t, s, "admin/repo-a", "completed", "success")
	seedFinalizedArtifact(s, 1, wf, "logs", time.Now())
	jobID := stableJobID(wfJob.JobID)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "get run",
			method: http.MethodGet,
			path:   fmt.Sprintf("/api/v3/repos/admin/repo-b/actions/runs/%d", wf.RunID),
		},
		{
			name:   "list run jobs",
			method: http.MethodGet,
			path:   fmt.Sprintf("/api/v3/repos/admin/repo-b/actions/runs/%d/jobs", wf.RunID),
		},
		{
			name:   "get job",
			method: http.MethodGet,
			path:   fmt.Sprintf("/api/v3/repos/admin/repo-b/actions/jobs/%d", jobID),
		},
		{
			name:   "get job logs",
			method: http.MethodGet,
			path:   fmt.Sprintf("/api/v3/repos/admin/repo-b/actions/jobs/%d/logs", jobID),
		},
		{
			name:   "list run artifacts",
			method: http.MethodGet,
			path:   fmt.Sprintf("/api/v3/repos/admin/repo-b/actions/runs/%d/artifacts", wf.RunID),
		},
		{
			name:   "delete run",
			method: http.MethodDelete,
			path:   fmt.Sprintf("/api/v3/repos/admin/repo-b/actions/runs/%d", wf.RunID),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := runAuthedRequest(s, tc.method, tc.path)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
			}
		})
	}
	if got := s.findWorkflowByRunID(wf.RunID); got == nil {
		t.Fatal("wrong-repository delete removed the original workflow run")
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

func TestActionsRuns_RerunWithoutCachedYAMLFailsLoud(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	wf, _ := seedRun(t, s, "octo/repo", "completed", "success")

	w := runAuthedRequest(s, "POST", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d/rerun", wf.RunID))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 when the run has no cached workflow YAML", w.Code)
	}
}

func TestActionsRuns_RerunUsesOriginatingWorkflowFile(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	repo := "octo/repo"
	target := registerSameNamedWorkflowFiles(t, s, repo)

	wf, _ := seedRun(t, s, repo, "completed", "success")
	wf.Name = "shared"
	wf.WorkflowFileID = target.ID
	wf.WorkflowFilePath = target.Path

	w := runAuthedRequest(s, "POST", fmt.Sprintf("/api/v3/repos/%s/actions/runs/%d/rerun", repo, wf.RunID))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	got := s.findWorkflowByRunID(wf.RunID)
	if got == nil {
		t.Fatal("rerun did not create a live attempt")
	}
	if got.WorkflowFileID != target.ID || got.WorkflowFilePath != target.Path {
		t.Fatalf("workflow file = %d %q, want %d %q", got.WorkflowFileID, got.WorkflowFilePath, target.ID, target.Path)
	}
	if _, ok := got.Jobs["target"]; !ok {
		t.Fatalf("rerun jobs = %#v, want target job from originating workflow file", got.Jobs)
	}
	if _, ok := got.Jobs["wrong"]; ok {
		t.Fatalf("rerun used same-named non-origin workflow file: %#v", got.Jobs)
	}
}

func TestActionsRuns_RerunFailedJobsUsesOriginatingWorkflowFile(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	s.registerGHActionsExtrasRoutes()
	repo := "octo/repo"
	target := registerSameNamedWorkflowFiles(t, s, repo)

	wf, _ := seedRun(t, s, repo, "completed", "failure")
	wf.Name = "shared"
	wf.WorkflowFileID = target.ID
	wf.WorkflowFilePath = target.Path
	wf.Jobs["build"].Result = ResultFailure

	w := runAuthedRequest(s, "POST", fmt.Sprintf("/api/v3/repos/%s/actions/runs/%d/rerun-failed-jobs", repo, wf.RunID))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	got := s.findWorkflowByRunID(wf.RunID)
	if got == nil {
		t.Fatal("rerun-failed-jobs did not create a live attempt")
	}
	if got.WorkflowFileID != target.ID || got.WorkflowFilePath != target.Path {
		t.Fatalf("workflow file = %d %q, want %d %q", got.WorkflowFileID, got.WorkflowFilePath, target.ID, target.Path)
	}
	if _, ok := got.Jobs["target"]; !ok {
		t.Fatalf("rerun-failed-jobs jobs = %#v, want target job from originating workflow file", got.Jobs)
	}
	if _, ok := got.Jobs["wrong"]; ok {
		t.Fatalf("rerun-failed-jobs used same-named non-origin workflow file: %#v", got.Jobs)
	}
}

func TestActionsJobs_RerunUsesOriginatingWorkflowFile(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRunControlRoutes()
	repo := "octo/repo"
	target := registerSameNamedWorkflowFiles(t, s, repo)

	wf, job := seedRun(t, s, repo, "completed", "failure")
	wf.Name = "shared"
	wf.WorkflowFileID = target.ID
	wf.WorkflowFilePath = target.Path
	job.Result = ResultFailure

	jobID := stableJobID(job.JobID)
	w := runAuthedRequest(s, "POST", fmt.Sprintf("/api/v3/repos/%s/actions/jobs/%d/rerun", repo, jobID))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	got := s.findWorkflowByRunID(wf.RunID)
	if got == nil {
		t.Fatal("job rerun did not create a live attempt")
	}
	if got.WorkflowFileID != target.ID || got.WorkflowFilePath != target.Path {
		t.Fatalf("workflow file = %d %q, want %d %q", got.WorkflowFileID, got.WorkflowFilePath, target.ID, target.Path)
	}
	if _, ok := got.Jobs["target"]; !ok {
		t.Fatalf("job rerun jobs = %#v, want target job from originating workflow file", got.Jobs)
	}
	if _, ok := got.Jobs["wrong"]; ok {
		t.Fatalf("job rerun used same-named non-origin workflow file: %#v", got.Jobs)
	}
}

func TestActionsRuns_RerunAmbiguousNameFailsLoud(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	repo := "octo/repo"
	s.store.RegisterWorkflowFile(repo, ".github/workflows/a.yml", "shared", "name: shared\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo a\n", "submitted")
	s.store.RegisterWorkflowFile(repo, ".github/workflows/b.yml", "shared", "name: shared\njobs:\n  b:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo b\n", "submitted")

	wf, _ := seedRun(t, s, repo, "completed", "success")
	wf.Name = "shared"

	w := runAuthedRequest(s, "POST", fmt.Sprintf("/api/v3/repos/%s/actions/runs/%d/rerun", repo, wf.RunID))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body=%s", w.Code, w.Body.String())
	}
}

func registerSameNamedWorkflowFiles(t *testing.T, s *Server, repo string) *WorkflowFile {
	t.Helper()
	s.store.RegisterWorkflowFile(repo, ".github/workflows/a.yml", "shared", "name: shared\njobs:\n  wrong:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo wrong\n", "submitted")
	s.store.RegisterWorkflowFile(repo, ".github/workflows/b.yml", "shared", "name: shared\njobs:\n  target:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo target\n", "submitted")
	files := s.store.ListWorkflowFiles(repo)
	if len(files) != 2 {
		t.Fatalf("workflow files = %d, want 2", len(files))
	}
	return files[1]
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

	// The runner list requires administration:read — authenticate.
	w := runAuthedRequest(s, "GET", "/api/v3/repos/octo/repo/actions/runners")
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

	workflowPath := ".github/workflows/ci.yml"
	wantFileID := stableWorkflowFileID("octo/repo", workflowPath)
	for i := 0; int64(wf.RunID) == wantFileID; i++ {
		workflowPath = fmt.Sprintf(".github/workflows/ci-%d.yml", i)
		wantFileID = stableWorkflowFileID("octo/repo", workflowPath)
	}
	wf.WorkflowFileID = wantFileID
	wf.WorkflowFilePath = workflowPath

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
	if got.Path != workflowPath {
		t.Errorf("path = %q, want %q", got.Path, workflowPath)
	}
	wantURL := fmt.Sprintf("http://example.com/api/v3/repos/octo/repo/actions/workflows/%d", wantFileID)
	if got.WorkflowURL != wantURL {
		t.Errorf("workflow_url = %q, want %q", got.WorkflowURL, wantURL)
	}
}

func TestActionsJob_StepsAndCompletedAt(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	s.registerTimelineRoutes()
	_, wfJob := seedRun(t, s, "octo/repo", "completed", "success")
	s.store.mu.Lock()
	wfJob.CompletedAt = wfJob.StartedAt.Add(30 * time.Second)
	s.store.mu.Unlock()
	planID, timelineID := linkJobToPlan(t, s, wfJob)

	// Report the job + step records the way the runner does (job record
	// plus one Task record per step, PATCHed through the timeline route).
	jobRec := uuid.New().String()
	patchTimelineRecords(t, s, planID, timelineID, true, []map[string]any{
		{"id": jobRec, "type": "Job", "name": "Build", "order": 1,
			"state": "completed", "result": "succeeded"},
		{"id": uuid.New().String(), "parentId": jobRec, "type": "Task",
			"name": "Checkout", "refName": "step1", "order": 1,
			"state": "completed", "result": "succeeded",
			"startTime":  "2026-06-12T10:00:00.1234567Z",
			"finishTime": "2026-06-12T10:00:05.7654321Z"},
		{"id": uuid.New().String(), "parentId": jobRec, "type": "Task",
			"name": "Run go test ./...", "refName": "step2", "order": 2,
			"state": "completed", "result": "succeeded",
			"startTime":  "2026-06-12T10:00:06Z",
			"finishTime": "2026-06-12T10:00:30Z"},
	})

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
	// The runner's fractional-second timestamps come back normalized to
	// GitHub's second-resolution RFC3339.
	if got.Steps[0].StartedAt != nil && *got.Steps[0].StartedAt != "2026-06-12T10:00:00Z" {
		t.Errorf("step 0 started_at = %q, want 2026-06-12T10:00:00Z", *got.Steps[0].StartedAt)
	}
}

func TestActionsRunners_ExtraFields(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	s.store.mu.Lock()
	s.store.Agents[7] = &Agent{ID: 7, Name: "r", OSDescription: "Linux", Status: "online", Version: "2.300.0"}
	s.store.mu.Unlock()

	w := runAuthedRequest(s, "GET", "/api/v3/repos/octo/repo/actions/runners")
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

// ghDo issues an arbitrary-method request against the shared test server.
func ghDo(t *testing.T, method, path, token string, body interface{}) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, testBaseURL+path, bodyReader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// Full deployment-review flow over REST: configure a reviewer-protected
// environment, run a workflow targeting it, observe the waiting run +
// pending deployment, approve, and read back the recorded approval.
func TestActionsPendingDeploymentReviewFlow(t *testing.T) {
	repo := "admin/envflow"
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{"name": "envflow"})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("create repo: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Configure the environment with a required reviewer (admin = id 1).
	resp = ghDo(t, "PUT", "/api/v3/repos/"+repo+"/environments/production", defaultToken,
		map[string]interface{}{"reviewers": []map[string]interface{}{{"type": "User", "id": 1}}})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("put environment: %d", resp.StatusCode)
	}
	envData := decodeJSON(t, resp)
	envID := int(envData["id"].(float64))
	rules, _ := envData["protection_rules"].([]interface{})
	if len(rules) != 1 {
		t.Fatalf("protection_rules = %v, want the required_reviewers rule", envData["protection_rules"])
	}

	workflowYAML := `
name: deploy
on: push
jobs:
  release:
    runs-on: ubuntu-latest
    environment: production
    steps:
      - run: echo deploy
`
	resp = ghPut(t, "/api/v3/repos/"+repo+"/contents/.github/workflows/deploy.yml", defaultToken, map[string]interface{}{
		"message": "add deployment workflow",
		"content": base64.StdEncoding.EncodeToString([]byte(workflowYAML)),
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create workflow file: %d %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	// Run a workflow with a job targeting the environment.
	resp = ghPost(t, "/internal/exec/workflow", defaultToken, map[string]interface{}{
		"repo":     repo,
		"workflow": workflowYAML,
		"image":    "alpine:latest",
	})
	if resp.StatusCode != 200 && resp.StatusCode != 201 && resp.StatusCode != 202 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("submit workflow: %d %s", resp.StatusCode, body)
	}
	submitData := decodeJSON(t, resp)
	if submitData["status"] != "waiting" {
		t.Fatalf("submit status = %v, want waiting", submitData["status"])
	}

	// Resolve the run's numeric id from the runs list.
	resp = ghGet(t, "/api/v3/repos/"+repo+"/actions/runs", defaultToken)
	runsData := decodeJSON(t, resp)
	runsList, _ := runsData["workflow_runs"].([]interface{})
	if len(runsList) == 0 {
		t.Fatalf("no workflow runs listed")
	}
	runID := int(runsList[0].(map[string]interface{})["id"].(float64))
	runPath := fmt.Sprintf("/api/v3/repos/%s/actions/runs/%d", repo, runID)

	// The run waits for review.
	resp = ghGet(t, runPath, defaultToken)
	runData := decodeJSON(t, resp)
	if runData["status"] != "waiting" {
		t.Fatalf("run status = %v, want waiting", runData["status"])
	}

	// Pending deployments lists the protected environment.
	resp = ghGet(t, runPath+"/pending_deployments", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("pending_deployments: %d", resp.StatusCode)
	}
	var pending []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&pending); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(pending) != 1 {
		t.Fatalf("pending = %v, want 1 entry", pending)
	}
	pendingEnv := pending[0]["environment"].(map[string]interface{})
	if int(pendingEnv["id"].(float64)) != envID || pendingEnv["name"] != "production" {
		t.Fatalf("pending environment = %v", pendingEnv)
	}

	// Approve.
	resp = ghPost(t, runPath+"/pending_deployments", defaultToken, map[string]interface{}{
		"environment_ids": []int{envID},
		"state":           "approved",
		"comment":         "ship it",
	})
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("approve: %d %s", resp.StatusCode, body)
	}
	var deployments []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&deployments); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(deployments) != 1 || deployments[0]["environment"] != "production" {
		t.Fatalf("approval deployments = %v", deployments)
	}

	// The run resumed (no longer waiting) and the approval is recorded.
	resp = ghGet(t, runPath, defaultToken)
	runData = decodeJSON(t, resp)
	if runData["status"] == "waiting" {
		t.Fatalf("run still waiting after approval")
	}

	resp = ghGet(t, runPath+"/approvals", defaultToken)
	var approvals []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&approvals); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(approvals) != 1 || approvals[0]["state"] != "approved" || approvals[0]["comment"] != "ship it" {
		t.Fatalf("approvals = %v", approvals)
	}
	user, _ := approvals[0]["user"].(map[string]interface{})
	if user == nil || user["login"] != "admin" {
		t.Fatalf("approval user = %v", approvals[0]["user"])
	}
}

func createTestOrg(t *testing.T) string {
	t.Helper()
	login := "test-org-actions-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	createOrgViaAdminAPI(t, login, "Test Org Actions")
	return login
}

func createTestRepo(t *testing.T) string {
	t.Helper()
	name := "test-repo-actions-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":    name,
		"private": false,
	})
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create repo: %d %s", resp.StatusCode, body)
	}
	resp.Body.Close()
	return "admin/" + name
}

func TestActionsPermissions_Org_GetSet(t *testing.T) {
	org := createTestOrg(t)

	resp := ghGet(t, "/api/v3/orgs/"+org+"/actions/permissions", defaultToken)
	data := decodeJSONWithStatus(t, resp, 200)
	if data["enabled_repositories"] != "all" {
		t.Errorf("enabled_repositories = %v, want all", data["enabled_repositories"])
	}
	if data["allowed_actions"] != "all" {
		t.Errorf("allowed_actions = %v, want all", data["allowed_actions"])
	}

	putResp := ghPut(t, "/api/v3/orgs/"+org+"/actions/permissions", defaultToken, map[string]interface{}{
		"enabled_repositories": "selected",
		"allowed_actions":      "selected",
	})
	data = decodeJSONWithStatus(t, putResp, 200)
	if data["enabled_repositories"] != "selected" {
		t.Errorf("enabled_repositories after put = %v, want selected", data["enabled_repositories"])
	}
	if data["selected_repositories_url"] == nil {
		t.Error("expected selected_repositories_url when enabled_repositories=selected")
	}
	if data["selected_actions_url"] == nil {
		t.Error("expected selected_actions_url when allowed_actions=selected")
	}
}

func TestActionsPermissions_Org_SelectedRepos(t *testing.T) {
	org := createTestOrg(t)
	repoKey := createTestRepo(t)

	// Resolve repo numeric id.
	repoResp := ghGet(t, "/api/v3/repos/"+repoKey, defaultToken)
	repoData := decodeJSONWithStatus(t, repoResp, 200)
	repoID := int(repoData["id"].(float64))

	putResp := ghPut(t, "/api/v3/orgs/"+org+"/actions/permissions", defaultToken, map[string]interface{}{
		"enabled_repositories": "selected",
	})
	decodeJSONWithStatus(t, putResp, 200)

	setResp := ghPut(t, "/api/v3/orgs/"+org+"/actions/permissions/repositories", defaultToken, map[string]interface{}{
		"selected_repository_ids": []int{repoID},
	})
	requireStatus(t, setResp, 204)

	listResp := ghGet(t, "/api/v3/orgs/"+org+"/actions/permissions/repositories", defaultToken)
	listData := decodeJSONWithStatus(t, listResp, 200)
	repos, _ := listData["repositories"].([]interface{})
	if len(repos) != 1 {
		t.Fatalf("selected repos = %d, want 1", len(repos))
	}

	addResp := ghPut(t, fmt.Sprintf("/api/v3/orgs/%s/actions/permissions/repositories/%d", org, repoID), defaultToken, nil)
	requireStatus(t, addResp, 204)

	delResp := ghDelete(t, fmt.Sprintf("/api/v3/orgs/%s/actions/permissions/repositories/%d", org, repoID), defaultToken)
	requireStatus(t, delResp, 204)

	listResp = ghGet(t, "/api/v3/orgs/"+org+"/actions/permissions/repositories", defaultToken)
	listData = decodeJSON(t, listResp)
	repos, _ = listData["repositories"].([]interface{})
	if len(repos) != 0 {
		t.Errorf("selected repos after remove = %d, want 0", len(repos))
	}
}

func TestActionsPermissions_Org_AllowedActions(t *testing.T) {
	org := createTestOrg(t)

	putResp := ghPut(t, "/api/v3/orgs/"+org+"/actions/permissions/selected-actions", defaultToken, map[string]interface{}{
		"github_owned_allowed": true,
		"verified_allowed":     true,
		"patterns_allowed":     []string{"octo-org/"},
	})
	data := decodeJSONWithStatus(t, putResp, 200)
	if data["github_owned_allowed"] != true {
		t.Errorf("github_owned_allowed = %v", data["github_owned_allowed"])
	}
	patterns, _ := data["patterns_allowed"].([]interface{})
	if len(patterns) != 1 || patterns[0] != "octo-org/" {
		t.Errorf("patterns_allowed = %v", patterns)
	}

	getResp := ghGet(t, "/api/v3/orgs/"+org+"/actions/permissions/selected-actions", defaultToken)
	decodeJSONWithStatus(t, getResp, 200)
}

func TestActionsPermissions_Org_WorkflowPermissions(t *testing.T) {
	org := createTestOrg(t)

	putResp := ghPut(t, "/api/v3/orgs/"+org+"/actions/permissions/workflow", defaultToken, map[string]interface{}{
		"default_workflow_permissions":     "write",
		"can_approve_pull_request_reviews": true,
	})
	data := decodeJSONWithStatus(t, putResp, 200)
	if data["default_workflow_permissions"] != "write" {
		t.Errorf("default_workflow_permissions = %v", data["default_workflow_permissions"])
	}
	if data["can_approve_pull_request_reviews"] != true {
		t.Errorf("can_approve_pull_request_reviews = %v", data["can_approve_pull_request_reviews"])
	}
}

func TestActionsPermissions_Org_CacheLimits(t *testing.T) {
	org := createTestOrg(t)

	retResp := ghPut(t, "/api/v3/orgs/"+org+"/actions/cache/retention-limit", defaultToken, map[string]interface{}{
		"retention_limit_in_days": 45,
	})
	data := decodeJSONWithStatus(t, retResp, 200)
	if data["retention_limit_in_days"] != float64(45) {
		t.Errorf("retention_limit_in_days = %v", data["retention_limit_in_days"])
	}

	storResp := ghPut(t, "/api/v3/orgs/"+org+"/actions/cache/storage-limit", defaultToken, map[string]interface{}{
		"storage_limit_in_bytes": 1024 * 1024 * 1024,
	})
	data = decodeJSONWithStatus(t, storResp, 200)
	if data["storage_limit_in_bytes"] != float64(1024*1024*1024) {
		t.Errorf("storage_limit_in_bytes = %v", data["storage_limit_in_bytes"])
	}
}

func TestActionsPermissions_Repo_GetSet(t *testing.T) {
	repo := createTestRepo(t)

	resp := ghGet(t, "/api/v3/repos/"+repo+"/actions/permissions", defaultToken)
	data := decodeJSONWithStatus(t, resp, 200)
	if data["enabled"] != true {
		t.Errorf("enabled = %v, want true", data["enabled"])
	}

	putResp := ghPut(t, "/api/v3/repos/"+repo+"/actions/permissions", defaultToken, map[string]interface{}{
		"enabled":         false,
		"allowed_actions": "selected",
	})
	data = decodeJSONWithStatus(t, putResp, 200)
	if data["enabled"] != false {
		t.Errorf("enabled after put = %v, want false", data["enabled"])
	}
	if data["selected_actions_url"] == nil {
		t.Error("expected selected_actions_url when allowed_actions=selected")
	}
}

func TestActionsPermissions_Repo_AccessLevel(t *testing.T) {
	repo := createTestRepo(t)

	putResp := ghPut(t, "/api/v3/repos/"+repo+"/actions/permissions/access", defaultToken, map[string]interface{}{
		"access_level": "organization",
	})
	data := decodeJSONWithStatus(t, putResp, 200)
	if data["access_level"] != "organization" {
		t.Errorf("access_level = %v", data["access_level"])
	}
}

func TestActionsPermissions_Repo_AllowedActions(t *testing.T) {
	repo := createTestRepo(t)

	putResp := ghPut(t, "/api/v3/repos/"+repo+"/actions/permissions/selected-actions", defaultToken, map[string]interface{}{
		"github_owned_allowed": true,
		"patterns_allowed":     []string{"actions/"},
	})
	data := decodeJSONWithStatus(t, putResp, 200)
	patterns, _ := data["patterns_allowed"].([]interface{})
	if len(patterns) != 1 {
		t.Errorf("patterns_allowed = %v", patterns)
	}
}

func TestActionsPermissions_Repo_WorkflowPermissions(t *testing.T) {
	repo := createTestRepo(t)

	putResp := ghPut(t, "/api/v3/repos/"+repo+"/actions/permissions/workflow", defaultToken, map[string]interface{}{
		"default_workflow_permissions":     "write",
		"can_approve_pull_request_reviews": true,
	})
	data := decodeJSONWithStatus(t, putResp, 200)
	if data["default_workflow_permissions"] != "write" {
		t.Errorf("default_workflow_permissions = %v", data["default_workflow_permissions"])
	}
}

func TestActionsPermissions_Repo_ForkPRSettings(t *testing.T) {
	repo := createTestRepo(t)

	putResp := ghPut(t, "/api/v3/repos/"+repo+"/actions/permissions/fork-pr-contributor-approval", defaultToken, map[string]interface{}{
		"require_approval": "all",
	})
	data := decodeJSONWithStatus(t, putResp, 200)
	if data["require_approval"] != "all" {
		t.Errorf("require_approval = %v", data["require_approval"])
	}

	putResp = ghPut(t, "/api/v3/repos/"+repo+"/actions/permissions/fork-pr-workflows-private-repos", defaultToken, map[string]interface{}{
		"policy": "run",
	})
	data = decodeJSONWithStatus(t, putResp, 200)
	if data["policy"] != "run" {
		t.Errorf("policy = %v", data["policy"])
	}
}

func TestActionsPermissions_Repo_ArtifactAndLogRetention(t *testing.T) {
	repo := createTestRepo(t)

	putResp := ghPut(t, "/api/v3/repos/"+repo+"/actions/permissions/artifact-and-log-retention", defaultToken, map[string]interface{}{
		"artifact_and_log_retention_days": 30,
	})
	data := decodeJSONWithStatus(t, putResp, 200)
	if data["artifact_and_log_retention_days"] != float64(30) {
		t.Errorf("artifact_and_log_retention_days = %v", data["artifact_and_log_retention_days"])
	}
}

func TestActionsPermissions_Repo_CacheLimits(t *testing.T) {
	repo := createTestRepo(t)

	retResp := ghPut(t, "/api/v3/repos/"+repo+"/actions/cache/retention-limit", defaultToken, map[string]interface{}{
		"retention_limit_in_days": 60,
	})
	data := decodeJSONWithStatus(t, retResp, 200)
	if data["retention_limit_in_days"] != float64(60) {
		t.Errorf("retention_limit_in_days = %v", data["retention_limit_in_days"])
	}

	storResp := ghPut(t, "/api/v3/repos/"+repo+"/actions/cache/storage-limit", defaultToken, map[string]interface{}{
		"storage_limit_in_bytes": 500 * 1024 * 1024,
	})
	data = decodeJSONWithStatus(t, storResp, 200)
	if data["storage_limit_in_bytes"] != float64(500*1024*1024) {
		t.Errorf("storage_limit_in_bytes = %v", data["storage_limit_in_bytes"])
	}
}

func TestActionsRunLogs_Delete(t *testing.T) {
	s := newTestServer()
	s.registerGHActionsRoutes()
	s.registerGHActionsPermissionsRoutes()
	s.registerTimelineRoutes()
	repo := "admin/rr-logs"
	s.store.CreateRepo(s.store.LookupUserByLogin("admin"), "rr-logs", "", false)
	wf, wfJob := seedRun(t, s, repo, "completed", "success")
	planID, _ := linkJobToPlan(t, s, wfJob)
	// Seed a timeline record with a log file so deletion has something to remove.
	s.store.mu.Lock()
	s.store.TimelineRecords[planID] = append(s.store.TimelineRecords[planID], &TimelineRecord{
		Type: "Task",
		Name: "Step",
		Log:  &TimelineLogRef{ID: 1},
	})
	s.store.LogFiles[1] = []byte("step log")
	s.store.mu.Unlock()

	w := runAuthedRequest(s, "DELETE", fmt.Sprintf("/api/v3/repos/%s/actions/runs/%d/logs", repo, wf.RunID))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete logs status = %d, body=%s", w.Code, w.Body.String())
	}
	s.store.mu.RLock()
	_, hasLogLines := s.store.LogLines[wfJob.JobID]
	_, hasTimeline := s.store.TimelineRecords[planID]
	_, hasLogFile := s.store.LogFiles[1]
	s.store.mu.RUnlock()
	if hasLogLines {
		t.Error("log lines should be deleted")
	}
	if hasTimeline {
		t.Error("timeline records should be deleted")
	}
	if hasLogFile {
		t.Error("log file should be deleted")
	}
}

func decodeJSONWithStatus(t *testing.T, resp *http.Response, want int) map[string]interface{} {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want %d: %s", resp.StatusCode, want, body)
	}
	return decodeJSON(t, resp)
}

func requireStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want %d: %s", resp.StatusCode, want, body)
	}
	resp.Body.Close()
}
