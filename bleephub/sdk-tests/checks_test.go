package sdktests

import (
	"testing"

	github "github.com/google/go-github/v88/github"
)

// TestChecks covers CreateCheckRun / GetCheckRun / UpdateCheckRun and
// CreateCheckSuite / GetCheckSuite. bleephub permission-gates these on the
// "checks" scope; the admin PAT carries it, so no App JWT is needed.
func TestChecks(t *testing.T) {
	name := uniqueName("checks")
	createRepo(t, name)

	const headSHA = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"

	run, _, err := client.Checks.CreateCheckRun(ctx(), "admin", name, github.CreateCheckRunOptions{
		Name:    "ci/build",
		HeadSHA: headSHA,
		Status:  github.Ptr("in_progress"),
	})
	if err != nil {
		t.Fatalf("CreateCheckRun: %v", err)
	}
	if run.GetID() == 0 {
		t.Error("check run ID is zero")
	}
	if run.GetName() != "ci/build" {
		t.Errorf("check run name = %q, want ci/build", run.GetName())
	}
	if run.GetHeadSHA() != headSHA {
		t.Errorf("check run head_sha = %q, want %q", run.GetHeadSHA(), headSHA)
	}
	if run.GetStatus() != "in_progress" {
		t.Errorf("check run status = %q, want in_progress", run.GetStatus())
	}
	id := run.GetID()

	got, _, err := client.Checks.GetCheckRun(ctx(), "admin", name, id)
	if err != nil {
		t.Fatalf("GetCheckRun: %v", err)
	}
	if got.GetID() != id {
		t.Errorf("GetCheckRun ID = %d, want %d", got.GetID(), id)
	}

	updated, _, err := client.Checks.UpdateCheckRun(ctx(), "admin", name, id, github.UpdateCheckRunOptions{
		Name:       "ci/build",
		Status:     github.Ptr("completed"),
		Conclusion: github.Ptr("success"),
	})
	if err != nil {
		t.Fatalf("UpdateCheckRun: %v", err)
	}
	if updated.GetStatus() != "completed" {
		t.Errorf("updated status = %q, want completed", updated.GetStatus())
	}
	if updated.GetConclusion() != "success" {
		t.Errorf("updated conclusion = %q, want success", updated.GetConclusion())
	}

	// Check suite
	suite, _, err := client.Checks.CreateCheckSuite(ctx(), "admin", name, github.CreateCheckSuiteOptions{
		HeadSHA: headSHA,
	})
	if err != nil {
		t.Fatalf("CreateCheckSuite: %v", err)
	}
	if suite.GetID() == 0 {
		t.Error("check suite ID is zero")
	}
	if suite.GetHeadSHA() != headSHA {
		t.Errorf("check suite head_sha = %q, want %q", suite.GetHeadSHA(), headSHA)
	}

	gotSuite, _, err := client.Checks.GetCheckSuite(ctx(), "admin", name, suite.GetID())
	if err != nil {
		t.Fatalf("GetCheckSuite: %v", err)
	}
	if gotSuite.GetID() != suite.GetID() {
		t.Errorf("GetCheckSuite ID = %d, want %d", gotSuite.GetID(), suite.GetID())
	}
}
