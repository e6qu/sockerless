package sdktests

import (
	"testing"
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

// TestActionsWorkflowDispatch is skipped: dispatch requires a workflow file
// discovered from a git push (handleDispatchWorkflow 404s when no WorkflowFile
// exists for the repo, and the SDK suite does not perform a git smart-HTTP push
// to seed .github/workflows/*.yml). Likewise GetWorkflowRunByID and
// ListWorkflowJobs require an actual run to exist. These are covered by
// bleephub's own in-process workflow tests (workflows_complex_test.go etc.).
func TestActionsWorkflowDispatch(t *testing.T) {
	t.Skip("dispatch + run-by-id + workflow-jobs require a git-pushed workflow file and an executed run; not seedable through the typed REST SDK without a git push")
}
