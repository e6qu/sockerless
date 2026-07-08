package sdktests

import (
	"net/http"
	"testing"

	github "github.com/google/go-github/v88/github"
)

// TestActionsReadSurfaces covers the Actions read endpoints that decode cleanly
// against a fresh repo (no git push, no run history): ListWorkflows,
// ListRepositoryWorkflowRuns, ListArtifacts, GetCacheUsageForRepo, ListCaches.
// These assert the typed envelopes (Workflows / WorkflowRuns / ArtifactList /
// ActionsCacheUsage / ActionsCacheList) decode and report a zero count for an
// empty repo rather than erroring.
func TestActionsReadSurfaces(t *testing.T) {
	name := uniqueName("actions")
	createRepo(t, name)

	wfs, _, err := client.Actions.ListWorkflows(ctx(), "admin", name, nil)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if wfs == nil {
		t.Error("ListWorkflows returned nil envelope")
	}
	if wfs.GetTotalCount() != 0 {
		t.Errorf("fresh repo workflow count = %d, want 0", wfs.GetTotalCount())
	}

	runs, _, err := client.Actions.ListRepositoryWorkflowRuns(ctx(), "admin", name, nil)
	if err != nil {
		t.Fatalf("ListRepositoryWorkflowRuns: %v", err)
	}
	if runs == nil {
		t.Error("ListRepositoryWorkflowRuns returned nil envelope")
	}
	if runs.GetTotalCount() != 0 {
		t.Errorf("fresh repo run count = %d, want 0", runs.GetTotalCount())
	}

	arts, _, err := client.Actions.ListArtifacts(ctx(), "admin", name, nil)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if arts == nil {
		t.Error("ListArtifacts returned nil envelope")
	}
	if arts.GetTotalCount() != 0 {
		t.Errorf("fresh repo artifact count = %d, want 0", arts.GetTotalCount())
	}

	usage, _, err := client.Actions.GetCacheUsageForRepo(ctx(), "admin", name)
	if err != nil {
		t.Fatalf("GetCacheUsageForRepo: %v", err)
	}
	if usage == nil {
		t.Error("GetCacheUsageForRepo returned nil")
	}
	if usage.GetFullName() == "" {
		t.Errorf("cache usage full_name empty, want admin/%s", name)
	}

	caches, _, err := client.Actions.ListCaches(ctx(), "admin", name, nil)
	if err != nil {
		t.Fatalf("ListCaches: %v", err)
	}
	if caches == nil {
		t.Error("ListCaches returned nil envelope")
	}
	if caches.GetTotalCount() != 0 {
		t.Errorf("fresh repo cache count = %d, want 0", caches.GetTotalCount())
	}
}

func TestActionsWorkflowDispatch(t *testing.T) {
	name := uniqueName("actions-dispatch")
	createRepo(t, name)

	const workflowPath = ".github/workflows/ci.yml"
	const workflowYAML = `name: sdk-ci
on:
  workflow_dispatch:
    inputs:
      reason:
        description: why
        required: true
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo sdk
`
	commit := createSDKCommit(t, name, "add workflow", workflowPath, workflowYAML, nil)
	if _, _, err := client.Git.CreateRef(ctx(), "admin", name, github.CreateRef{
		Ref: "refs/heads/main",
		SHA: commit.GetSHA(),
	}); err != nil {
		t.Fatalf("Git.CreateRef(main): %v", err)
	}

	workflows, _, err := client.Actions.ListWorkflows(ctx(), "admin", name, nil)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if workflows.GetTotalCount() != 1 || len(workflows.Workflows) != 1 {
		t.Fatalf("workflow count = %d len=%d, want 1", workflows.GetTotalCount(), len(workflows.Workflows))
	}
	workflow := workflows.Workflows[0]
	if workflow.GetName() != "sdk-ci" || workflow.GetPath() != workflowPath {
		t.Fatalf("workflow = name %q path %q, want sdk-ci %s", workflow.GetName(), workflow.GetPath(), workflowPath)
	}

	_, resp, err := client.Actions.CreateWorkflowDispatchEventByFileName(ctx(), "admin", name, "ci.yml", github.CreateWorkflowDispatchEventRequest{
		Ref:    "refs/heads/main",
		Inputs: map[string]any{"reason": "sdk"},
	})
	if err != nil {
		t.Fatalf("CreateWorkflowDispatchEventByFileName: %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("dispatch status = %v, want %d", resp, http.StatusNoContent)
	}

	runs, _, err := client.Actions.ListRepositoryWorkflowRuns(ctx(), "admin", name, &github.ListWorkflowRunsOptions{
		Event: "workflow_dispatch",
	})
	if err != nil {
		t.Fatalf("ListRepositoryWorkflowRuns: %v", err)
	}
	if runs.GetTotalCount() != 1 || len(runs.WorkflowRuns) != 1 {
		t.Fatalf("run count = %d len=%d, want 1", runs.GetTotalCount(), len(runs.WorkflowRuns))
	}
	run := runs.WorkflowRuns[0]
	if run.GetID() == 0 {
		t.Fatal("workflow run returned empty ID")
	}
	if run.GetName() != "sdk-ci" || run.GetEvent() != "workflow_dispatch" || run.GetPath() != workflowPath {
		t.Fatalf("run = name %q event %q path %q, want sdk-ci workflow_dispatch %s", run.GetName(), run.GetEvent(), run.GetPath(), workflowPath)
	}
	if run.GetHeadBranch() != "main" {
		t.Fatalf("run head_branch = %q, want main", run.GetHeadBranch())
	}
	if run.GetRunAttempt() != 1 {
		t.Fatalf("run attempt = %d, want 1", run.GetRunAttempt())
	}

	gotRun, _, err := client.Actions.GetWorkflowRunByID(ctx(), "admin", name, run.GetID())
	if err != nil {
		t.Fatalf("GetWorkflowRunByID: %v", err)
	}
	if gotRun.GetID() != run.GetID() || gotRun.GetPath() != workflowPath {
		t.Fatalf("GetWorkflowRunByID = id %d path %q, want id %d path %s", gotRun.GetID(), gotRun.GetPath(), run.GetID(), workflowPath)
	}

	jobs, _, err := client.Actions.ListWorkflowJobs(ctx(), "admin", name, run.GetID(), nil)
	if err != nil {
		t.Fatalf("ListWorkflowJobs: %v", err)
	}
	if jobs.GetTotalCount() != 1 || len(jobs.Jobs) != 1 {
		t.Fatalf("job count = %d len=%d, want 1", jobs.GetTotalCount(), len(jobs.Jobs))
	}
	job := jobs.Jobs[0]
	if job.GetID() == 0 || job.GetRunID() != run.GetID() {
		t.Fatalf("job id/run_id = %d/%d, want nonzero/%d", job.GetID(), job.GetRunID(), run.GetID())
	}
	if job.GetName() != "build" || job.GetWorkflowName() != "sdk-ci" || job.GetRunAttempt() != 1 {
		t.Fatalf("job = name %q workflow %q attempt %d, want build sdk-ci 1", job.GetName(), job.GetWorkflowName(), job.GetRunAttempt())
	}
}
