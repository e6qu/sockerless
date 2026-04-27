//go:build github_runner_live

package github_runner_test

import (
	"context"
	"encoding/json"
	"fmt"
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
// The runner runs inside a *Linux container* on the host (Docker
// Desktop / colima provides the Linux VM); the container is built
// from tests/runners/github/dockerfile/. GitHub Actions' `container:`
// directive only works on Linux runners, so a darwin-native runner
// can't drive the canonical workloads. Sockerless's host arch is
// irrelevant — the runner is just a docker *client* that points at
// the sockerless daemon via DOCKER_HOST.
//
// Build-tag-gated (`github_runner_live`). Run via:
//
//	go test -v -tags github_runner_live -run TestGitHub_ECS_Hello \
//	  -timeout 30m ./tests/runners/github
//	go test -v -tags github_runner_live -run TestGitHub_Lambda_Hello \
//	  -timeout 30m ./tests/runners/github
//
// Wiring + token strategy in docs/RUNNERS.md.

const (
	defaultRunnerVersion = "2.334.0"
	defaultRepo          = "e6qu/sockerless"
	pollInterval         = 5 * time.Second
	runnerImageTag       = "sockerless-actions-runner:local"
)

func TestGitHub_ECS_Hello(t *testing.T) {
	runCell(t, cellConfig{
		Label:             "sockerless-ecs",
		Workflow:          "hello-ecs.yml",
		DefaultDockerHost: "tcp://localhost:3375",
	})
}

func TestGitHub_Lambda_Hello(t *testing.T) {
	runCell(t, cellConfig{
		Label:             "sockerless-lambda",
		Workflow:          "hello-lambda.yml",
		DefaultDockerHost: "tcp://localhost:3376",
	})
}

type cellConfig struct {
	Label             string
	Workflow          string
	DefaultDockerHost string
}

func runCell(t *testing.T, c cellConfig) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker CLI required to run the runner container: %v", err)
	}
	repo := envOr("SOCKERLESS_GH_REPO", defaultRepo)
	dockerHost := envOr("SOCKERLESS_DOCKER_HOST", c.DefaultDockerHost)

	pat, err := runnersinternal.GitHubPAT()
	if err != nil {
		t.Skipf("GitHub PAT unavailable: %v", err)
	}
	defer zero(pat)

	pingDocker(t, dockerHost)

	if err := runnersinternal.CleanupOldGitHubRunners(repo, "sockerless-"); err != nil {
		t.Logf("warning: pre-run cleanup of old runners failed: %v", err)
	}
	cancelLeftoverRuns(t, repo, c.Workflow)

	regToken, err := runnersinternal.MintGitHubRegistrationToken(repo)
	if err != nil {
		t.Fatalf("mint registration token: %v", err)
	}

	runnerName := fmt.Sprintf("sockerless-%s-%s", c.Label, runnersinternal.Timestamp())
	t.Logf("runner name: %s", runnerName)

	buildRunnerImage(t)
	containerID := startRunnerContainer(t, runnerName, repo, regToken, c.Label, dockerHost)
	t.Cleanup(func() { stopRunnerContainer(t, containerID) })

	// Wait for the runner to register itself with GitHub. Until it
	// shows up in the runners list, dispatch can't route the job.
	waitForRunnerRegistration(t, repo, runnerName, 90*time.Second)

	runID := dispatchWorkflow(t, repo, c.Workflow)
	conclusion := waitForRun(t, repo, runID, 15*time.Minute)
	if conclusion != "success" {
		t.Fatalf("workflow run %d concluded with %q, expected success.\nLogs at https://github.com/%s/actions/runs/%d", runID, conclusion, repo, runID)
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
	resp, err := exec.Command("curl", "-fsS", pingURL).Output()
	if err != nil {
		t.Fatalf("Sockerless daemon at %s unreachable: %v", host, err)
	}
	t.Logf("Sockerless ping: %s", strings.TrimSpace(string(resp)))
}

func buildRunnerImage(t *testing.T) {
	t.Helper()
	dockerfileDir := dockerfileDir(t)
	// Match the runner-binary arch to the local Docker VM arch.
	// Docker Desktop on Apple Silicon ships a linux/arm64 VM; on
	// Intel it's linux/amd64. Mapping runtime.GOARCH (the host's Go
	// arch — same as Docker Desktop's VM arch in practice) → AWS-
	// runner asset suffix.
	targetArch := runtime.GOARCH
	if targetArch == "amd64" {
		targetArch = "x64"
	}
	cmd := exec.Command("docker", "build",
		"--build-arg", "TARGETARCH="+targetArch,
		"--build-arg", "RUNNER_VERSION="+defaultRunnerVersion,
		"-t", runnerImageTag,
		dockerfileDir,
	)
	cmd.Stdout = testLogWriter{t: t, prefix: "build: "}
	cmd.Stderr = testLogWriter{t: t, prefix: "build: "}
	if err := cmd.Run(); err != nil {
		t.Fatalf("docker build runner image: %v", err)
	}
	t.Logf("built runner image %s for linux/%s", runnerImageTag, targetArch)
}

func dockerfileDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Walk up until we find tests/runners/github/dockerfile.
	cur := wd
	for i := 0; i < 5; i++ {
		candidate := filepath.Join(cur, "tests", "runners", "github", "dockerfile")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		// Also check sibling-relative for `go test` in package dir.
		candidate = filepath.Join(cur, "dockerfile")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		cur = filepath.Dir(cur)
	}
	t.Fatalf("could not locate runner dockerfile dir from %s", wd)
	return ""
}

func startRunnerContainer(t *testing.T, name, repo, regToken, label, dockerHost string) string {
	t.Helper()
	// `host.docker.internal` resolves to the host laptop from inside
	// the container on Docker Desktop / colima. Translate the harness's
	// localhost-form DOCKER_HOST into that for the runner container's
	// internal use.
	innerHost := strings.Replace(dockerHost, "localhost", "host.docker.internal", 1)
	innerHost = strings.Replace(innerHost, "127.0.0.1", "host.docker.internal", 1)

	args := []string{"run", "-d", "--rm",
		"--name", name,
		"--add-host", "host.docker.internal:host-gateway",
		"-e", "RUNNER_REPO_URL=https://github.com/" + repo,
		"-e", "RUNNER_TOKEN=" + regToken,
		"-e", "RUNNER_NAME=" + name,
		"-e", "RUNNER_LABELS=" + label + ",sockerless",
		"-e", "DOCKER_HOST=" + innerHost,
		runnerImageTag,
	}
	out, err := exec.Command("docker", args...).Output()
	if err != nil {
		t.Fatalf("docker run runner: %v", err)
	}
	id := strings.TrimSpace(string(out))
	t.Logf("started runner container %s (%s)", name, id[:12])

	// Stream container logs for visibility.
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		cmd := exec.CommandContext(ctx, "docker", "logs", "-f", id)
		cmd.Stdout = testLogWriter{t: t, prefix: "runner: "}
		cmd.Stderr = testLogWriter{t: t, prefix: "runner: "}
		_ = cmd.Run()
	}()
	return id
}

func stopRunnerContainer(t *testing.T, id string) {
	t.Helper()
	if id == "" {
		return
	}
	if out, err := exec.Command("docker", "stop", id).CombinedOutput(); err != nil {
		t.Logf("docker stop %s: %v\n%s", id, err, out)
	}
}

func waitForRunnerRegistration(t *testing.T, repo, name string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := exec.Command("gh", "api",
			"/repos/"+repo+"/actions/runners",
			"--jq", `[.runners[] | select(.name=="`+name+`" and .status=="online")] | length`,
		).Output()
		if err == nil && strings.TrimSpace(string(out)) == "1" {
			t.Logf("runner %s registered + online", name)
			return
		}
		time.Sleep(pollInterval)
	}
	t.Fatalf("runner %s did not appear online within %s", name, timeout)
}

// cancelLeftoverRuns forces `cancelled` on any queued/in-progress runs
// for this workflow that an earlier crashed harness left behind, so a
// fresh ephemeral runner doesn't get poached by an old job. Each
// previous harness session would otherwise leave a queued workflow_dispatch
// run waiting for its (now-deregistered) runner.
func cancelLeftoverRuns(t *testing.T, repo, workflow string) {
	t.Helper()
	out, err := exec.Command("gh", "api",
		"/repos/"+repo+"/actions/workflows/"+workflow+"/runs?status=queued&per_page=20",
		"--jq", "[.workflow_runs[] | .id]",
	).Output()
	if err != nil {
		return
	}
	var ids []int64
	_ = json.Unmarshal(out, &ids)
	for _, id := range ids {
		_ = exec.Command("gh", "api", "-X", "POST",
			fmt.Sprintf("/repos/%s/actions/runs/%d/cancel", repo, id)).Run()
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
	dispatchedAt := time.Now().Add(-30 * time.Second) // 30s slack for clock skew
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		out, err := exec.Command("gh", "api",
			"/repos/"+repo+"/actions/workflows/"+file+"/runs?per_page=5",
		).Output()
		if err == nil {
			var resp struct {
				WorkflowRuns []struct {
					ID        int64     `json:"id"`
					Event     string    `json:"event"`
					CreatedAt time.Time `json:"created_at"`
				} `json:"workflow_runs"`
			}
			if jerr := json.Unmarshal(out, &resp); jerr == nil {
				for _, r := range resp.WorkflowRuns {
					if r.Event == "workflow_dispatch" && r.CreatedAt.After(dispatchedAt) {
						return r.ID
					}
				}
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
