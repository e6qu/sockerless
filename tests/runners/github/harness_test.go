//go:build github_runner_live

package github_runner_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	runnersinternal "github.com/sockerless/tests/runners/internal"
)

// Real GitHub Actions runner harness — the 4-cell matrix's GH side.
// Build-tag-gated (`github_runner_live`) so default `go test ./...`
// doesn't try to download the runner. Run via:
//
//	go test -v -tags github_runner_live -run TestGitHub_ECS_Hello \
//	  -timeout 30m ./tests/runners/github
//	go test -v -tags github_runner_live -run TestGitHub_Lambda_Hello \
//	  -timeout 30m ./tests/runners/github
//
// Wiring + token strategy in docs/RUNNERS.md. Each cell:
//   1. Reads the GitHub PAT via `gh auth token` (keychain-backed).
//   2. Mints a registration token via the API.
//   3. Self-heals — deletes any leftover sockerless-* runners from
//      a previous crash.
//   4. Downloads + configures + starts an ephemeral runner.
//   5. Dispatches the workflow_dispatch, polls until success.
//   6. Cleans up — runner.remove + cancel context + log capture.
//
// Each cell points at its own DOCKER_HOST (the ECS Sockerless daemon
// on :3375 vs the Lambda daemon on :3376). Both daemons + live AWS
// infra come up via manual-tests/01-infrastructure.md before the test
// runs — the harness does not provision them.

const (
	// Default to a release we've verified darwin-arm64 + linux-x64
	// assets exist for. Override via SOCKERLESS_GH_RUNNER_VERSION.
	defaultRunnerVersion = "2.334.0"
	defaultRepo          = "e6qu/sockerless"
	pollInterval         = 5 * time.Second
)

// TestGitHub_ECS_Hello — cell 1 of 4. Runs the hello-ecs workflow on
// a self-hosted runner labelled `sockerless-ecs`, pointed at the ECS
// Sockerless daemon.
func TestGitHub_ECS_Hello(t *testing.T) {
	runCell(t, cellConfig{
		Label:    "sockerless-ecs",
		Workflow: "hello-ecs.yml",
		WorkflowYAML: workflowYAML("hello-ecs", "sockerless-ecs",
			`      - run: echo "hello from sockerless ecs"`,
			`      - run: env | sort`,
		),
		DefaultDockerHost: "tcp://localhost:3375",
	})
}

// TestGitHub_Lambda_Hello — cell 2 of 4. Same shape, Lambda label.
func TestGitHub_Lambda_Hello(t *testing.T) {
	runCell(t, cellConfig{
		Label:    "sockerless-lambda",
		Workflow: "hello-lambda.yml",
		WorkflowYAML: workflowYAML("hello-lambda", "sockerless-lambda",
			`      - run: echo "hello from sockerless lambda"`,
			`      - run: date -u`,
		),
		DefaultDockerHost: "tcp://localhost:3376",
	})
}

type cellConfig struct {
	Label             string // self-hosted label the workflow's runs-on requests
	Workflow          string // file name under .github/workflows/
	WorkflowYAML      string // body to commit
	DefaultDockerHost string // overridden by SOCKERLESS_DOCKER_HOST env if set
}

func runCell(t *testing.T, c cellConfig) {
	repo := envOr("SOCKERLESS_GH_REPO", defaultRepo)
	dockerHost := envOr("SOCKERLESS_DOCKER_HOST", c.DefaultDockerHost)
	if os.Getenv("DOCKER_HOST") == "" {
		os.Setenv("DOCKER_HOST", dockerHost)
	}

	pat, err := runnersinternal.GitHubPAT()
	if err != nil {
		t.Skipf("GitHub PAT unavailable: %v", err)
	}
	defer zero(pat)

	pingDocker(t, dockerHost)

	if err := runnersinternal.CleanupOldGitHubRunners(repo, "sockerless-"); err != nil {
		t.Logf("warning: pre-run cleanup of old runners failed: %v", err)
	}

	regToken, err := runnersinternal.MintGitHubRegistrationToken(repo)
	if err != nil {
		t.Fatalf("mint registration token: %v", err)
	}
	defer func() {
		if rmTok, err := runnersinternal.MintGitHubRemoveToken(repo); err == nil {
			t.Logf("issued removal token (length %d) for any orphan runner", len(rmTok))
		}
	}()

	runnerName := fmt.Sprintf("sockerless-%s-%s", c.Label, runnersinternal.Timestamp())
	workdir := t.TempDir()
	t.Logf("runner workdir: %s", workdir)
	t.Logf("runner name: %s", runnerName)

	runnerBin := downloadRunner(t, workdir, defaultRunnerVersion)
	configureRunner(t, runnerBin, repo, regToken, c.Label, runnerName)
	cancel := startRunner(t, runnerBin)
	t.Cleanup(func() {
		cancel()
		removeRunner(t, runnerBin, repo)
	})

	commitWorkflow(t, repo, c.Workflow, c.WorkflowYAML)
	runID := dispatchWorkflow(t, repo, c.Workflow)
	conclusion := waitForRun(t, repo, runID, 15*time.Minute)
	if conclusion != "success" {
		logs := fetchRunLogs(t, repo, runID)
		t.Fatalf("workflow run %d concluded with %q, expected success.\nLogs:\n%s", runID, conclusion, logs)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func pingDocker(t *testing.T, host string) {
	t.Helper()
	pingURL := strings.Replace(host, "tcp://", "http://", 1) + "/_ping"
	resp, err := http.Get(pingURL)
	if err != nil {
		t.Fatalf("Sockerless daemon at %s unreachable: %v", host, err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("Sockerless daemon at %s returned %d on /_ping", host, resp.StatusCode)
	}
}

func downloadRunner(t *testing.T, workdir, version string) string {
	t.Helper()
	runnerDir := filepath.Join(workdir, "runner")
	if err := os.MkdirAll(runnerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// actions/runner uses `osx` (not `darwin`) in its release-asset
	// naming convention for macOS, and `x64` (not `amd64`) for Intel.
	// Translate Go's runtime values to match the asset path.
	osTag := runtime.GOOS
	if osTag == "darwin" {
		osTag = "osx"
	}
	archTag := runtime.GOARCH
	if archTag == "amd64" {
		archTag = "x64"
	}
	url := fmt.Sprintf("https://github.com/actions/runner/releases/download/v%s/actions-runner-%s-%s-%s.tar.gz",
		version, osTag, archTag, version)
	t.Logf("downloading runner: %s", url)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("download runner: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("download runner: HTTP %d", resp.StatusCode)
	}
	tarPath := filepath.Join(runnerDir, "runner.tar.gz")
	f, err := os.Create(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	cmd := exec.Command("tar", "xzf", tarPath, "-C", runnerDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("extract runner: %v\n%s", err, out)
	}
	return filepath.Join(runnerDir, "config.sh")
}

func configureRunner(t *testing.T, runnerBin, repo, token, label, name string) {
	t.Helper()
	cmd := exec.Command(runnerBin,
		"--url", "https://github.com/"+repo,
		"--token", token,
		"--unattended",
		"--ephemeral",
		"--labels", label+",sockerless",
		"--name", name,
		"--replace",
	)
	cmd.Dir = filepath.Dir(runnerBin)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("configure runner: %v\n%s", err, out)
	}
}

func startRunner(t *testing.T, runnerBin string) func() {
	t.Helper()
	runScript := filepath.Join(filepath.Dir(runnerBin), "run.sh")
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, runScript)
	cmd.Dir = filepath.Dir(runnerBin)
	cmd.Stdout = testLogWriter{t: t, prefix: "runner: "}
	cmd.Stderr = testLogWriter{t: t, prefix: "runner: "}
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start runner: %v", err)
	}
	time.Sleep(10 * time.Second) // settle window before workflow dispatch
	return func() {
		cancel()
		_ = cmd.Wait()
	}
}

func removeRunner(t *testing.T, runnerBin, repo string) {
	t.Helper()
	rmTok, err := runnersinternal.MintGitHubRemoveToken(repo)
	if err != nil {
		t.Logf("removal token fetch failed (runner left for GH auto-cleanup): %v", err)
		return
	}
	cmd := exec.Command(runnerBin, "remove", "--token", rmTok, "--unattended")
	cmd.Dir = filepath.Dir(runnerBin)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("runner remove warning: %v\n%s", err, out)
	}
}

func commitWorkflow(t *testing.T, repo, name, body string) {
	t.Helper()
	path := ".github/workflows/" + name
	var sha string
	if out, err := exec.Command("gh", "api", "/repos/"+repo+"/contents/"+path, "--jq", ".sha").Output(); err == nil {
		sha = strings.TrimSpace(string(out))
	}
	args := []string{
		"api", "-X", "PUT", "/repos/" + repo + "/contents/" + path,
		"-f", "message=test: harness workflow update",
		"-f", "content=" + base64.StdEncoding.EncodeToString([]byte(body)),
	}
	if sha != "" {
		args = append(args, "-f", "sha="+sha)
	}
	if out, err := exec.Command("gh", args...).CombinedOutput(); err != nil {
		t.Fatalf("commit workflow: %v\n%s", err, out)
	}
}

func dispatchWorkflow(t *testing.T, repo, file string) int64 {
	t.Helper()
	disp := exec.Command("gh", "api", "-X", "POST",
		"/repos/"+repo+"/actions/workflows/"+file+"/dispatches",
		"-f", "ref=main",
	)
	if out, err := disp.CombinedOutput(); err != nil {
		t.Fatalf("dispatch workflow: %v\n%s", err, out)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		out, err := exec.Command("gh", "api",
			"/repos/"+repo+"/actions/workflows/"+file+"/runs?per_page=1",
		).Output()
		if err == nil {
			var resp struct {
				WorkflowRuns []struct {
					ID int64 `json:"id"`
				} `json:"workflow_runs"`
			}
			if jerr := json.Unmarshal(out, &resp); jerr == nil && len(resp.WorkflowRuns) > 0 {
				return resp.WorkflowRuns[0].ID
			}
		}
		time.Sleep(pollInterval)
	}
	t.Fatal("dispatch workflow: no run ID found within 2 minutes")
	return 0
}

func waitForRun(t *testing.T, repo string, runID int64, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := exec.Command("gh", "api", fmt.Sprintf("/repos/%s/actions/runs/%d", repo, runID)).Output()
		if err == nil {
			var resp struct {
				Status     string `json:"status"`
				Conclusion string `json:"conclusion"`
			}
			if jerr := json.Unmarshal(out, &resp); jerr == nil {
				if resp.Status == "completed" {
					return resp.Conclusion
				}
				t.Logf("run %d: status=%s", runID, resp.Status)
			}
		}
		time.Sleep(pollInterval)
	}
	t.Fatalf("workflow run %d did not complete within %s", runID, timeout)
	return ""
}

func fetchRunLogs(t *testing.T, repo string, runID int64) string {
	t.Helper()
	out, err := exec.Command("gh", "api", fmt.Sprintf("/repos/%s/actions/runs/%d/logs", repo, runID)).Output()
	if err != nil {
		return fmt.Sprintf("(failed to fetch logs: %v)", err)
	}
	return string(out)
}

func workflowYAML(name, label string, steps ...string) string {
	return fmt.Sprintf(`name: %s
on:
  workflow_dispatch:
  pull_request:
    paths:
      - tests/runners/**
jobs:
  hello:
    runs-on: [self-hosted, %s]
    container:
      image: alpine:latest
    steps:
%s
`, name, label, strings.Join(steps, "\n"))
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

type testLogWriter struct {
	t      *testing.T
	prefix string
}

func (w testLogWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line == "" {
			continue
		}
		w.t.Log(w.prefix + line)
	}
	return len(p), nil
}
