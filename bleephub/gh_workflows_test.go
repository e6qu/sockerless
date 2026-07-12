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
	"github.com/go-git/go-git/v5/plumbing"
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
func commitWorkflowYAMLToStorage(t *testing.T, s *Server, repoFullName, path, body string) string {
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
	commitHash, err := wt.Commit("add "+path, &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@t", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("git commit: %v", err)
	}
	mainRef := plumbing.NewBranchReferenceName("main")
	if err := storer.SetReference(plumbing.NewHashReference(mainRef, commitHash)); err != nil {
		t.Fatalf("set main ref: %v", err)
	}
	if err := storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, mainRef)); err != nil {
		t.Fatalf("set HEAD: %v", err)
	}
	return commitHash.String()
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
	admin := s.store.LookupUserByLogin("admin")
	org := s.store.CreateOrg(admin, "octo", "octo", "")
	if org == nil {
		t.Fatal("create org octo")
	}
	repo := s.store.CreateOrgRepo(org, admin, "repo", "", false)
	if repo == nil {
		t.Fatal("create repo octo/repo")
	}
	stor := s.store.GetGitStorage("octo", "repo")
	if stor == nil {
		t.Fatal("git storage for octo/repo")
	}
	if _, err := initRepoWithFiles(stor, repo.DefaultBranch, "seed workflow", map[string]string{
		".github/workflows/ci.yml": sampleWorkflowYAML,
	}, repoSignature(admin.Login, "bleephub@local")); err != nil {
		t.Fatalf("seed workflow git state: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"workflow": sampleWorkflowYAML,
		"repo":     "octo/repo",
		"image":    "alpine:latest",
	})
	req := httptest.NewRequest("POST", "/internal/exec/workflow", bytes.NewReader(body))
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
	// The workflow must declare workflow_dispatch (and the provided
	// inputs) or real GitHub 422s the dispatch.
	const dispatchableYAML = `name: ci
on:
  push:
  workflow_dispatch:
    inputs:
      reason:
        description: why
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`
	wantSHA := commitWorkflowYAMLToStorage(t, s, "octo/repo", ".github/workflows/ci.yml", dispatchableYAML)
	s.store.DiscoverWorkflowFilesFromGit("octo/repo")
	wf := s.resolveWorkflowFile("octo/repo", "ci.yml")
	if wf == nil {
		t.Fatal("workflow file was not discovered from git storage")
	}

	body := []byte(`{"ref":"refs/heads/main","inputs":{"reason":"manual"}}`)
	req := httptest.NewRequest("POST",
		fmt.Sprintf("/api/v3/repos/octo/repo/actions/workflows/%d/dispatches", wf.ID),
		bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
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

	var run *Workflow
	s.store.mu.RLock()
	for _, candidate := range s.store.Workflows {
		if candidate.Name == "ci" {
			run = candidate
			break
		}
	}
	if run == nil || len(run.Jobs) != 1 {
		s.store.mu.RUnlock()
		t.Fatalf("dispatch run jobs = %v, want one job", run)
	}
	if run.Sha != wantSHA {
		s.store.mu.RUnlock()
		t.Fatalf("dispatch run sha = %q, want committed workflow sha %q", run.Sha, wantSHA)
	}
	var job *WorkflowJob
	for _, candidate := range run.Jobs {
		job = candidate
		break
	}
	s.store.mu.RUnlock()
	msg, err := s.buildJobMessageFromDef("http://example.test", run, job, "plan", "timeline", 1, run.Env["__defaultImage"])
	if err != nil {
		t.Fatal(err)
	}
	if msg["jobContainer"] != nil {
		t.Fatalf("workflow_dispatch jobContainer = %#v, want nil for a workflow without container", msg["jobContainer"])
	}
}

func TestWorkflows_DispatchResolvesGitHubRefInputs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		refBody string
		wantRef string
	}{
		{name: "branch name", refBody: "main", wantRef: "refs/heads/main"},
		{name: "tag name", refBody: "v1.0.0", wantRef: "refs/tags/v1.0.0"},
		{name: "commit SHA", refBody: "", wantRef: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer()
			s.registerGHWorkflowsRoutes()
			const dispatchableYAML = `name: ci
on:
  workflow_dispatch:
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`
			wantSHA := commitWorkflowYAMLToStorage(t, s, "octo/repo", ".github/workflows/ci.yml", dispatchableYAML)
			stor := s.store.GetGitStorage("octo", "repo")
			if err := stor.SetReference(plumbing.NewHashReference(plumbing.NewTagReferenceName("v1.0.0"), plumbing.NewHash(wantSHA))); err != nil {
				t.Fatalf("set tag ref: %v", err)
			}
			s.store.DiscoverWorkflowFilesFromGit("octo/repo")
			wf := s.resolveWorkflowFile("octo/repo", "ci.yml")
			if wf == nil {
				t.Fatal("workflow file was not discovered from git storage")
			}

			refBody := tc.refBody
			wantRef := tc.wantRef
			if tc.name == "commit SHA" {
				refBody = wantSHA
				wantRef = wantSHA
			}
			body := []byte(fmt.Sprintf(`{"ref":%q}`, refBody))
			req := httptest.NewRequest("POST",
				fmt.Sprintf("/api/v3/repos/octo/repo/actions/workflows/%d/dispatches", wf.ID),
				bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+defaultToken)
			w := httptest.NewRecorder()
			s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
			if w.Code != http.StatusNoContent {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}

			s.store.mu.RLock()
			defer s.store.mu.RUnlock()
			for _, run := range s.store.Workflows {
				if run.Name != "ci" {
					continue
				}
				if run.Ref != wantRef {
					t.Fatalf("dispatch ref = %q, want %q", run.Ref, wantRef)
				}
				if run.Sha != wantSHA {
					t.Fatalf("dispatch sha = %q, want %q", run.Sha, wantSHA)
				}
				return
			}
			t.Fatal("dispatch did not create a ci workflow run")
		})
	}
}

func TestWorkflows_DispatchRejectsUnresolvedRef(t *testing.T) {
	s := newTestServer()
	s.registerGHWorkflowsRoutes()
	commitWorkflowYAMLToStorage(t, s, "octo/repo", ".github/workflows/ci.yml", `name: ci
on:
  workflow_dispatch:
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`)
	s.store.DiscoverWorkflowFilesFromGit("octo/repo")
	wf := s.resolveWorkflowFile("octo/repo", "ci.yml")
	if wf == nil {
		t.Fatal("workflow file was not discovered from git storage")
	}

	body := []byte(`{"ref":"refs/heads/missing"}`)
	req := httptest.NewRequest("POST",
		fmt.Sprintf("/api/v3/repos/octo/repo/actions/workflows/%d/dispatches", wf.ID),
		bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "No ref found for: refs/heads/missing") {
		t.Fatalf("body = %s, want missing ref error", w.Body.String())
	}
}

func TestWorkflows_Dispatch_NoYAMLCached(t *testing.T) {
	s := newTestServer()
	s.registerGHWorkflowsRoutes()
	// Register a WorkflowFile with empty YAML (mimics a discovery
	// edge case where the file was indexed without contents).
	wf := s.store.RegisterWorkflowFile("octo/repo", ".github/workflows/ci.yml", "ci", "", "discovered")

	w := runAuthedRequest(s, "POST",
		fmt.Sprintf("/api/v3/repos/octo/repo/actions/workflows/%d/dispatches", wf.ID))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 when YAML body is empty", w.Code)
	}
}

func TestWorkflows_Rerun_ViaCachedYAML(t *testing.T) {
	// The rerun handler dispatches through the WorkflowFile cache: register
	// a file matching the run's name, then POST rerun, expect 201 Created
	// instead of the fail-loud no-cached-yaml 422.
	s := newTestServer()
	s.registerGHActionsRoutes()
	s.registerGHWorkflowsRoutes()
	s.store.RegisterWorkflowFile("octo/repo", ".github/workflows/ci.yml", "ci", sampleWorkflowYAML, "submitted")

	run, _ := seedRun(t, s, "octo/repo", "completed", "success")
	run.Name = "ci"

	w := runAuthedRequest(s, "POST", fmt.Sprintf("/api/v3/repos/octo/repo/actions/runs/%d/rerun", run.RunID))
	if w.Code != http.StatusCreated {
		t.Errorf("rerun status = %d, want 201 (cached YAML present); body=%s", w.Code, w.Body.String())
	}
}

func TestRepositoryDispatchPayload_IncludesBranch(t *testing.T) {
	repo := &Repo{FullName: "octo/repo", DefaultBranch: "trunk"}
	user := &User{Login: "octocat"}
	payload := repositoryDispatchPayload(repo, user, "deploy", map[string]interface{}{"v": "1"})
	if payload["branch"] != "trunk" {
		t.Errorf("branch = %v, want trunk (repo default branch)", payload["branch"])
	}
	if payload["action"] != "deploy" || payload["event_type"] != "deploy" {
		t.Errorf("action/event_type mismatch: %v", payload)
	}
	if payload["client_payload"] == nil || payload["repository"] == nil || payload["sender"] == nil {
		t.Errorf("missing standard fields: %v", payload)
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
