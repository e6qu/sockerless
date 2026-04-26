//go:build github_runner_live

package github_runner_test

import (
	"context"
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
)

// Real GitHub Actions runner harness. Gated by the `github_runner_live`
// build tag so the regular `go test ./...` sweep doesn't try to
// download a runner. Run via:
//
//   go test -v -tags github_runner_live -run TestRealGitHubRunner -timeout 30m
//
// Prereqs in tests/runners/github/README.md.

const (
	defaultRunnerVersion = "2.319.1"
	defaultLabels        = "sockerless,sockerless-ecs"
	pollInterval         = 5 * time.Second
)

func TestRealGitHubRunner(t *testing.T) {
	token := os.Getenv("SOCKERLESS_GH_RUNNER_TOKEN")
	repo := os.Getenv("SOCKERLESS_GH_REPO")
	if token == "" || repo == "" {
		t.Skip("SOCKERLESS_GH_RUNNER_TOKEN + SOCKERLESS_GH_REPO not set — skipping real-runner harness")
	}
	if os.Getenv("DOCKER_HOST") == "" {
		t.Fatal("DOCKER_HOST must be set to a running sockerless instance")
	}
	if _, err := exec.LookPath("gh"); err != nil {
		t.Fatalf("gh CLI required: %v", err)
	}

	version := envOr("SOCKERLESS_GH_RUNNER_VERSION", defaultRunnerVersion)
	labels := envOr("SOCKERLESS_GH_RUNNER_LABELS", defaultLabels)

	workdir := t.TempDir()
	t.Logf("runner workdir: %s", workdir)

	runnerBin := downloadRunner(t, workdir, version)
	configureRunner(t, workdir, runnerBin, repo, token, labels)
	runnerCancel := startRunner(t, workdir, runnerBin)
	t.Cleanup(runnerCancel)

	commitWorkflow(t, repo, "hello-ecs.yml", helloWorkflowYAML(labels))
	runID := dispatchWorkflow(t, repo, "hello-ecs.yml")
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

// downloadRunner fetches the actions/runner tarball into workdir/runner
// and extracts it. Returns the path to config.sh.
func downloadRunner(t *testing.T, workdir, version string) string {
	t.Helper()
	runnerDir := filepath.Join(workdir, "runner")
	if err := os.MkdirAll(runnerDir, 0o755); err != nil {
		t.Fatal(err)
	}

	osTag := runtime.GOOS
	archTag := runtime.GOARCH
	if archTag == "amd64" {
		archTag = "x64"
	}
	url := fmt.Sprintf("https://github.com/actions/runner/releases/download/v%s/actions-runner-%s-%s-%s.tar.gz",
		version, osTag, archTag, version)
	t.Logf("downloading runner from %s", url)

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

func configureRunner(t *testing.T, workdir, runnerBin, repo, token, labels string) {
	t.Helper()
	cmd := exec.Command(runnerBin,
		"--url", "https://github.com/"+repo,
		"--token", token,
		"--unattended",
		"--labels", labels,
		"--name", "sockerless-test-runner-"+timestamp(),
		"--replace",
	)
	cmd.Dir = filepath.Dir(runnerBin)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("configure runner: %v\n%s", err, out)
	}
}

// startRunner kicks off `./run.sh` in a goroutine and returns a
// cancel function that stops the runner cleanly.
func startRunner(t *testing.T, workdir, runnerBin string) func() {
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

	// Give the runner a moment to come online before tests dispatch
	// workflows. Real polling against the GitHub API would be more
	// robust; for a first-pass harness this short fixed sleep is fine.
	time.Sleep(10 * time.Second)

	return func() {
		cancel()
		_ = cmd.Wait()
		// Unregister the runner so the repo doesn't accumulate stale
		// entries.
		removeToken := getRemovalToken(t, t.Name())
		if removeToken != "" {
			unregister := exec.Command(runnerBin, "remove", "--token", removeToken, "--unattended")
			unregister.Dir = filepath.Dir(runnerBin)
			_, _ = unregister.CombinedOutput()
		}
	}
}

func getRemovalToken(t *testing.T, _ string) string {
	repo := os.Getenv("SOCKERLESS_GH_REPO")
	out, err := exec.Command("gh", "api", "-X", "POST", "/repos/"+repo+"/actions/runners/remove-token", "--jq", ".token").Output()
	if err != nil {
		t.Logf("removal token fetch failed: %v", err)
		return ""
	}
	return strings.TrimSpace(string(out))
}

// commitWorkflow writes the workflow YAML to .github/workflows/<name>
// in the target repo via the GitHub contents API. If the file already
// exists, it's updated (PATCH); otherwise created (PUT-equivalent
// through gh api).
func commitWorkflow(t *testing.T, repo, name, body string) {
	t.Helper()
	path := ".github/workflows/" + name

	// Look up the current SHA if the file exists, so we can update it.
	var sha string
	if out, err := exec.Command("gh", "api", "/repos/"+repo+"/contents/"+path, "--jq", ".sha").Output(); err == nil {
		sha = strings.TrimSpace(string(out))
	}

	args := []string{
		"api", "-X", "PUT", "/repos/" + repo + "/contents/" + path,
		"-f", "message=test: harness workflow update",
		"-f", "content=" + base64Encode(body),
	}
	if sha != "" {
		args = append(args, "-f", "sha="+sha)
	}
	cmd := exec.Command("gh", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit workflow: %v\n%s", err, out)
	}
}

func base64Encode(s string) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var out strings.Builder
	in := []byte(s)
	for i := 0; i < len(in); i += 3 {
		end := i + 3
		if end > len(in) {
			end = len(in)
		}
		chunk := in[i:end]
		var n uint32
		for j := 0; j < 3; j++ {
			n <<= 8
			if j < len(chunk) {
				n |= uint32(chunk[j])
			}
		}
		out.WriteByte(alphabet[(n>>18)&0x3f])
		out.WriteByte(alphabet[(n>>12)&0x3f])
		if len(chunk) > 1 {
			out.WriteByte(alphabet[(n>>6)&0x3f])
		} else {
			out.WriteByte('=')
		}
		if len(chunk) > 2 {
			out.WriteByte(alphabet[n&0x3f])
		} else {
			out.WriteByte('=')
		}
	}
	return out.String()
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

	// GitHub doesn't return the run ID from the dispatch call; poll the
	// recent-runs endpoint and pick the newest one for our workflow.
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		out, err := exec.Command("gh", "api",
			"/repos/"+repo+"/actions/workflows/"+file+"/runs?per_page=1",
		).Output()
		if err == nil {
			var resp struct {
				WorkflowRuns []struct {
					ID         int64  `json:"id"`
					Status     string `json:"status"`
					Conclusion string `json:"conclusion"`
					CreatedAt  string `json:"created_at"`
				} `json:"workflow_runs"`
			}
			if jsonErr := json.Unmarshal(out, &resp); jsonErr == nil && len(resp.WorkflowRuns) > 0 {
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
			if jsonErr := json.Unmarshal(out, &resp); jsonErr == nil {
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

func helloWorkflowYAML(labels string) string {
	primary := strings.Split(labels, ",")[0]
	return fmt.Sprintf(`name: hello-ecs
on:
  workflow_dispatch:
jobs:
  hello:
    runs-on: [self-hosted, %s]
    container:
      image: alpine:latest
    steps:
      - run: echo "hello from sockerless"
      - run: env | sort
`, primary)
}

func timestamp() string {
	return time.Now().UTC().Format("20060102-150405")
}

// testLogWriter pipes runner stdout/stderr lines to the test logger so
// the harness emits them in real time.
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
