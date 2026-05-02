// Phase 120 cells 5-8 live-GCP harness. Build-tag-gated so the default
// `go test ./...` doesn't try to dispatch real workflows.
//
// All four cells use the docker-executor pattern (no k8s, no GKE, no
// ARC). Cells 5+6 (gh) are dispatched via the existing
// github-runner-dispatcher (Phase 110a). Cells 7+8 (gl) are picked up
// by long-lived sockerless-managed gitlab-runner containers.
//
// Each test:
//
//  1. Triggers the workflow / pipeline.
//  2. Polls until completion (success or failure).
//  3. Asserts terminal status == success.
//  4. Captures the run/pipeline URL into the test output for STATUS.md.
//
// Run with:
//
//	export SOCKERLESS_GH_REPO=e6qu/sockerless
//	export SOCKERLESS_GL_PROJECT=e6qu/sockerless
//	go test -v -tags=gcp_runner_live -run 'TestCell5' -timeout 30m ./...
//
//go:build gcp_runner_live

package gcpcells

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const pollInterval = 10 * time.Second

func ghRepo(t *testing.T) string {
	v := os.Getenv("SOCKERLESS_GH_REPO")
	if v == "" {
		t.Skip("SOCKERLESS_GH_REPO not set")
	}
	return v
}

func glProject(t *testing.T) string {
	v := os.Getenv("SOCKERLESS_GL_PROJECT")
	if v == "" {
		t.Skip("SOCKERLESS_GL_PROJECT not set")
	}
	return v
}

// TestCell5_GH_Cloudrun dispatches cell-5-cloudrun.yml via `gh
// workflow run` and waits for completion. Captures the run URL.
func TestCell5_GH_Cloudrun(t *testing.T) {
	repo := ghRepo(t)
	dispatchGHWorkflow(t, repo, "cell-5-cloudrun.yml")
	url := waitGHRun(t, repo, "cell-5-cloudrun.yml", 25*time.Minute)
	t.Logf("CELL 5 GREEN: %s", url)
}

// TestCell6_GH_Gcf dispatches cell-6-gcf.yml via gh workflow run.
func TestCell6_GH_Gcf(t *testing.T) {
	repo := ghRepo(t)
	dispatchGHWorkflow(t, repo, "cell-6-gcf.yml")
	url := waitGHRun(t, repo, "cell-6-gcf.yml", 25*time.Minute)
	t.Logf("CELL 6 GREEN: %s", url)
}

// TestCell7_GL_Cloudrun creates a one-off pipeline on the configured
// GitLab project pointed at tests/runners/gitlab/cell-7-cloudrun.yml.
// Requires `glab` CLI authenticated against the project.
func TestCell7_GL_Cloudrun(t *testing.T) {
	project := glProject(t)
	pipelineID := triggerGLPipeline(t, project, "tests/runners/gitlab/cell-7-cloudrun.yml")
	url := waitGLPipeline(t, project, pipelineID, 25*time.Minute)
	t.Logf("CELL 7 GREEN: %s", url)
}

// TestCell8_GL_Gcf same as cell 7 but with the gcf-routed cell file.
func TestCell8_GL_Gcf(t *testing.T) {
	project := glProject(t)
	pipelineID := triggerGLPipeline(t, project, "tests/runners/gitlab/cell-8-gcf.yml")
	url := waitGLPipeline(t, project, pipelineID, 25*time.Minute)
	t.Logf("CELL 8 GREEN: %s", url)
}

// — gh helpers —

func dispatchGHWorkflow(t *testing.T, repo, workflow string) {
	t.Helper()
	cmd := exec.Command("gh", "workflow", "run", workflow, "-R", repo)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gh workflow run %s: %v: %s", workflow, err, out)
	}
}

func waitGHRun(t *testing.T, repo, workflow string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)
		cmd := exec.Command("gh", "run", "list", "-R", repo, "--workflow", workflow, "--limit", "1", "--json", "status,conclusion,url")
		out, err := cmd.Output()
		if err != nil {
			t.Logf("gh run list (will retry): %v", err)
			continue
		}
		var rows []struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			URL        string `json:"url"`
		}
		if err := json.Unmarshal(out, &rows); err != nil || len(rows) == 0 {
			continue
		}
		row := rows[0]
		t.Logf("gh run status=%s conclusion=%s url=%s", row.Status, row.Conclusion, row.URL)
		if row.Status == "completed" {
			if row.Conclusion != "success" {
				t.Fatalf("workflow %s concluded %s; URL: %s", workflow, row.Conclusion, row.URL)
			}
			return row.URL
		}
	}
	t.Fatalf("workflow %s timed out after %s", workflow, timeout)
	return ""
}

// — glab helpers —

func triggerGLPipeline(t *testing.T, project, ciFile string) string {
	t.Helper()
	cmd := exec.Command("glab", "ci", "run", "-R", project, "-f", ciFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("glab ci run %s: %v: %s", ciFile, err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if i := strings.LastIndex(line, "/"); i >= 0 {
			tail := strings.TrimSpace(line[i+1:])
			if _, perr := fmt.Sscanf(tail, "%d", new(int)); perr == nil {
				return tail
			}
		}
	}
	t.Fatalf("could not parse pipeline ID from glab output: %s", out)
	return ""
}

func waitGLPipeline(t *testing.T, project, pipelineID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)
		cmd := exec.Command("glab", "ci", "view", pipelineID, "-R", project, "-F", "json")
		out, err := cmd.Output()
		if err != nil {
			t.Logf("glab ci view (will retry): %v", err)
			continue
		}
		var row struct {
			Status string `json:"status"`
			WebURL string `json:"web_url"`
		}
		if err := json.Unmarshal(out, &row); err != nil {
			continue
		}
		t.Logf("glab pipeline status=%s url=%s", row.Status, row.WebURL)
		switch row.Status {
		case "success":
			return row.WebURL
		case "failed", "canceled", "skipped":
			t.Fatalf("pipeline %s status=%s; URL: %s", pipelineID, row.Status, row.WebURL)
		}
	}
	t.Fatalf("pipeline %s timed out after %s", pipelineID, timeout)
	return ""
}
