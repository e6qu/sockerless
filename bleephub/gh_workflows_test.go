package bleephub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	memfs "github.com/go-git/go-billy/v5/memfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const sampleWorkflowYAML = `name: ci
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`

// commitWorkflowYAMLToStorage commits a single workflow YAML at HEAD
// of the given repo's git storage. Mirrors the pattern in
// webhooks_test.go's pushTestCommit but skips the HTTP push — we only
// need the commit visible to the discovery walk.
func commitWorkflowYAMLToStorage(t *testing.T, s *Server, repoFullName, path, body string) {
	t.Helper()
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		t.Fatalf("expected owner/repo, got %q", repoFullName)
	}
	// CreateRepo derives the repo's full name from owner.Login + name.
	// To make GetGitStorage(parts[0], parts[1]) hit, create a user whose
	// Login matches the test-fixture owner instead of using the default
	// admin user.
	s.store.mu.Lock()
	user := &User{ID: s.store.NextUser, Login: parts[0], Type: "User", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	s.store.NextUser++
	s.store.Users[user.ID] = user
	s.store.UsersByLogin[user.Login] = user
	s.store.mu.Unlock()
	s.store.CreateRepo(user, parts[1], "", false) // creates the GitStorage entry too
	storer := s.store.GetGitStorage(parts[0], parts[1])
	if storer == nil {
		t.Fatalf("no git storage for %s after CreateRepo", repoFullName)
	}
	fs := memfs.New()
	repo, err := git.Init(storer, fs)
	if err != nil {
		// already initialised by CreateRepo; reopen instead
		repo, err = git.Open(storer, fs)
		if err != nil {
			t.Fatalf("init/open repo: %v", err)
		}
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if err := fs.MkdirAll(strings.TrimSuffix(path, "/"+lastPathSegment(path)), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f, err := fs.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	_, _ = f.Write([]byte(body))
	_ = f.Close()
	if _, err := wt.Add(path); err != nil {
		t.Fatalf("git add %s: %v", path, err)
	}
	if _, err := wt.Commit("add "+path, &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@t", When: time.Now()},
	}); err != nil {
		t.Fatalf("git commit: %v", err)
	}
}

func TestWorkflows_DiscoverFromGitStorage(t *testing.T) {
	s := newTestServer()
	s.registerGHWorkflowsRoutes()
	commitWorkflowYAMLToStorage(t, s, "octo/repo", ".github/workflows/ci.yml", sampleWorkflowYAML)

	w := runRequest(s, "GET", "/api/v3/repos/octo/repo/actions/workflows")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		TotalCount int              `json:"total_count"`
		Workflows  []map[string]any `json:"workflows"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalCount != 1 {
		t.Errorf("total_count = %d, want 1", resp.TotalCount)
	}
	if resp.Workflows[0]["name"] != "ci" {
		t.Errorf("name = %v, want ci", resp.Workflows[0]["name"])
	}
	if resp.Workflows[0]["path"] != ".github/workflows/ci.yml" {
		t.Errorf("path = %v", resp.Workflows[0]["path"])
	}
	if resp.Workflows[0]["state"] != "active" {
		t.Errorf("state = %v, want active", resp.Workflows[0]["state"])
	}
}

func TestWorkflows_AutoRegisterOnSubmit(t *testing.T) {
	s := newTestServer()
	s.registerJobRoutes()
	s.registerGHWorkflowsRoutes()

	body, _ := json.Marshal(map[string]any{
		"workflow": sampleWorkflowYAML,
		"repo":     "octo/repo",
	})
	req := httptest.NewRequest("POST", "/api/v3/bleephub/workflow", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("submit status = %d, body = %s", w.Code, w.Body.String())
	}

	listW := runRequest(s, "GET", "/api/v3/repos/octo/repo/actions/workflows")
	var resp struct {
		TotalCount int              `json:"total_count"`
		Workflows  []map[string]any `json:"workflows"`
	}
	json.Unmarshal(listW.Body.Bytes(), &resp)
	if resp.TotalCount != 1 {
		t.Fatalf("total_count = %d, want 1 (submit must auto-register)", resp.TotalCount)
	}
	if resp.Workflows[0]["name"] != "ci" {
		t.Errorf("name = %v", resp.Workflows[0]["name"])
	}
}

func TestWorkflows_GetByID_AndByFilename(t *testing.T) {
	s := newTestServer()
	s.registerGHWorkflowsRoutes()
	wf := s.store.RegisterWorkflowFile("octo/repo", ".github/workflows/ci.yml", "ci", sampleWorkflowYAML, "submitted")

	// Numeric ID lookup.
	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/workflows/%d", wf.ID))
	if w.Code != http.StatusOK {
		t.Fatalf("by-id status = %d", w.Code)
	}
	// Filename lookup.
	w2 := runRequest(s, "GET", "/api/v3/repos/octo/repo/actions/workflows/ci.yml")
	if w2.Code != http.StatusOK {
		t.Fatalf("by-filename status = %d", w2.Code)
	}
	// Full path lookup.
	w3 := runRequest(s, "GET", "/api/v3/repos/octo/repo/actions/workflows/.github%2Fworkflows%2Fci.yml")
	if w3.Code != http.StatusOK {
		t.Errorf("by-full-path status = %d", w3.Code)
	}
}

func TestWorkflows_Get_NotFound(t *testing.T) {
	s := newTestServer()
	s.registerGHWorkflowsRoutes()
	w := runRequest(s, "GET", "/api/v3/repos/octo/repo/actions/workflows/9999")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestWorkflows_ListRunsForFile(t *testing.T) {
	s := newTestServer()
	s.registerGHWorkflowsRoutes()
	wf := s.store.RegisterWorkflowFile("octo/repo", ".github/workflows/ci.yml", "ci", sampleWorkflowYAML, "submitted")

	// Seed two runs whose internal Workflow.Name matches the file's name.
	runA, _ := seedRun(t, s, "octo/repo", "completed", "success")
	runA.Name = "ci"
	runB, _ := seedRun(t, s, "octo/repo", "running", "")
	runB.Name = "ci"
	// Plus one unrelated run with a different name.
	runC, _ := seedRun(t, s, "octo/repo", "completed", "success")
	runC.Name = "release"

	w := runRequest(s, "GET", fmt.Sprintf("/api/v3/repos/octo/repo/actions/workflows/%d/runs", wf.ID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp struct {
		TotalCount   int              `json:"total_count"`
		WorkflowRuns []map[string]any `json:"workflow_runs"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.TotalCount != 2 {
		t.Errorf("total_count = %d, want 2 (release run filtered out)", resp.TotalCount)
	}
}

func TestWorkflows_Dispatch(t *testing.T) {
	s := newTestServer()
	s.registerGHWorkflowsRoutes()
	wf := s.store.RegisterWorkflowFile("octo/repo", ".github/workflows/ci.yml", "ci", sampleWorkflowYAML, "submitted")

	body := []byte(`{"ref":"refs/heads/main","inputs":{"reason":"manual"}}`)
	req := httptest.NewRequest("POST",
		fmt.Sprintf("/api/v3/repos/octo/repo/actions/workflows/%d/dispatches", wf.ID),
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	// Dispatch must have created a run.
	s.store.mu.RLock()
	count := 0
	for _, run := range s.store.Workflows {
		if run.Name == "ci" {
			count++
		}
	}
	s.store.mu.RUnlock()
	if count != 1 {
		t.Errorf("after dispatch: %d ci runs, want 1", count)
	}
}

func TestWorkflows_Dispatch_NoYAMLCached(t *testing.T) {
	s := newTestServer()
	s.registerGHWorkflowsRoutes()
	// Register a WorkflowFile with empty YAML (mimics a discovery
	// edge case where the file was indexed without contents).
	wf := s.store.RegisterWorkflowFile("octo/repo", ".github/workflows/ci.yml", "ci", "", "discovered")

	w := runRequest(s, "POST",
		fmt.Sprintf("/api/v3/repos/octo/repo/actions/workflows/%d/dispatches", wf.ID))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 when YAML body is empty", w.Code)
	}
}

func TestWorkflows_Rerun_ViaCachedYAML(t *testing.T) {
	// The rerun handler dispatches through the WorkflowFile cache: register
	// a file matching the run's name, then POST rerun, expect 201 Created
	// instead of 422 (the no-cached-yaml fallback).
	s := newTestServer()
	s.registerGHActionsRoutes()
	s.registerGHWorkflowsRoutes()
	s.store.RegisterWorkflowFile("octo/repo", ".github/workflows/ci.yml", "ci", sampleWorkflowYAML, "submitted")

	run, _ := seedRun(t, s, "octo/repo", "completed", "success")
	run.Name = "ci"

	w := runRequest(s, "POST", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d/rerun", run.RunID))
	if w.Code != http.StatusCreated {
		t.Errorf("rerun status = %d, want 201 (cached YAML present); body=%s", w.Code, w.Body.String())
	}
}

func TestStableWorkflowFileID_Deterministic(t *testing.T) {
	a := stableWorkflowFileID("octo/repo", ".github/workflows/ci.yml")
	b := stableWorkflowFileID("octo/repo", ".github/workflows/ci.yml")
	c := stableWorkflowFileID("octo/repo", ".github/workflows/release.yml")
	if a != b {
		t.Errorf("not deterministic")
	}
	if a == c {
		t.Errorf("collision on distinct paths")
	}
	if a < 0 || c < 0 {
		t.Errorf("negative IDs returned")
	}
}

func TestIsWorkflowYAMLPath(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{".github/workflows/ci.yml", true},
		{".github/workflows/release.yaml", true},
		{".github/workflows/sub/nested.yml", false}, // GitHub doesn't recurse
		{".github/workflows/", false},
		{".github/dependabot.yml", false},
		{"README.md", false},
	}
	for _, c := range cases {
		if got := isWorkflowYAMLPath(c.in); got != c.want {
			t.Errorf("isWorkflowYAMLPath(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
