//go:build gitlab_runner_live

package gitlab_runner_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runnersinternal "github.com/sockerless/tests/runners/internal"
)

// Real GitLab Runner harness against live AWS.
// Build-tag-gated (`gitlab_runner_live`). Run via:
//
//	go test -v -tags gitlab_runner_live -run TestGitLab_ECS_Hello \
//	  -timeout 30m ./tests/runners/gitlab
//	go test -v -tags gitlab_runner_live -run TestGitLab_Lambda_Hello \
//	  -timeout 30m ./tests/runners/gitlab
//
// Wiring + token strategy in docs/RUNNERS.md. Each run:
//   1. Reads the GitLab PAT from the macOS Keychain.
//   2. Resolves the project ID for SOCKERLESS_GL_PROJECT.
//   3. Self-heals — deletes any leftover sockerless-* runners.
//   4. Creates a runner via POST /api/v4/user/runners (modern API);
//      receives an authentication token.
//   5. Registers gitlab-runner with --executor docker --docker-host.
//   6. Commits the .gitlab-ci.yml to a throwaway branch via API;
//      triggers a pipeline on that branch via POST /projects/:id/pipeline.
//   7. Polls until success.
//   8. Cleanup — gitlab-runner unregister, DELETE /runners/:id.

const (
	defaultGLProject = "e6qu/sockerless"
	defaultGLURL     = "https://gitlab.com"
	pollInterval     = 5 * time.Second
)

// TestGitLab_ECS_Hello — gitlab-runner docker executor pointed at sockerless-ECS.
func TestGitLab_ECS_Hello(t *testing.T) {
	runCell(t, cellConfig{
		Tag:          "sockerless-ecs",
		BranchPrefix: "sockerless-ecs",
		PipelineYAML: pipelineYAML("sockerless-ecs",
			`    - echo "hello from sockerless ecs"`,
			`    - env | sort`,
		),
		DefaultDockerHost: "tcp://localhost:3375",
	})
}

// TestGitLab_Lambda_Hello — gitlab-runner docker executor pointed at sockerless-Lambda.
func TestGitLab_Lambda_Hello(t *testing.T) {
	runCell(t, cellConfig{
		Tag:          "sockerless-lambda",
		BranchPrefix: "sockerless-lambda",
		PipelineYAML: pipelineYAML("sockerless-lambda",
			`    - echo "hello from sockerless lambda"`,
			`    - date -u`,
		),
		DefaultDockerHost: "tcp://localhost:3376",
	})
}

type cellConfig struct {
	Tag               string
	BranchPrefix      string
	PipelineYAML      string
	DefaultDockerHost string
}

func runCell(t *testing.T, c cellConfig) {
	if _, err := exec.LookPath("gitlab-runner"); err != nil {
		t.Skipf("gitlab-runner not installed (brew install gitlab-runner): %v", err)
	}
	projectPath := envOr("SOCKERLESS_GL_PROJECT", defaultGLProject)
	dockerHost := envOr("SOCKERLESS_DOCKER_HOST", c.DefaultDockerHost)
	if os.Getenv("DOCKER_HOST") == "" {
		os.Setenv("DOCKER_HOST", dockerHost)
	}

	pat, err := runnersinternal.GitLabPAT()
	if err != nil {
		t.Skipf("GitLab PAT unavailable: %v", err)
	}
	defer zero(pat)

	pingDocker(t, dockerHost)

	projectID, err := runnersinternal.ResolveGitLabProjectID(pat, projectPath)
	if err != nil {
		t.Fatalf("resolve project: %v", err)
	}

	if err := runnersinternal.CleanupOldGitLabRunners(pat, projectID, "sockerless-"); err != nil {
		t.Logf("warning: pre-run cleanup failed: %v", err)
	}

	description := fmt.Sprintf("sockerless-%s-%s", c.Tag, runnersinternal.Timestamp())
	runner, err := runnersinternal.CreateGitLabRunner(pat, projectID, description, []string{c.Tag, "sockerless"})
	if err != nil {
		t.Fatalf("create GitLab runner: %v", err)
	}
	t.Logf("registered GitLab runner %d (%s)", runner.ID, description)
	t.Cleanup(func() {
		if err := runnersinternal.DeleteGitLabRunner(pat, runner.ID); err != nil {
			t.Logf("delete GitLab runner %d: %v", runner.ID, err)
		}
	})

	workdir := t.TempDir()
	configPath := filepath.Join(workdir, "config.toml")
	registerRunner(t, runner.Token, dockerHost, description, configPath)
	stop := startRunner(t, configPath)
	t.Cleanup(stop)

	branch := fmt.Sprintf("%s-%s", c.BranchPrefix, runnersinternal.Timestamp())
	defaultBr := defaultBranch(t, pat, projectPath)
	createBranch(t, pat, projectPath, branch, defaultBr)
	t.Cleanup(func() { deleteBranch(t, pat, projectPath, branch) })

	commitPipeline(t, pat, projectPath, branch, c.PipelineYAML)
	pipelineID := triggerPipeline(t, pat, projectPath, branch)
	status := waitForPipeline(t, pat, projectPath, pipelineID, 15*time.Minute)
	if status != "success" {
		t.Fatalf("pipeline %d concluded with %q, expected success", pipelineID, status)
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
		t.Fatalf("Sockerless daemon at %s returned %d", host, resp.StatusCode)
	}
}

func registerRunner(t *testing.T, authToken, dockerHost, description, configPath string) {
	t.Helper()
	// --docker-helper-image: point gitlab-runner at a sockerless-
	// routable copy of gitlab-runner-helper (mirrored to live ECR by
	// the harness setup). Default `registry.gitlab.com/...` isn't
	// supported by ECR pull-through without Secrets Manager auth.
	// Sockerless's ECR auth path makes the manifest fetch authenticate
	// via ECR's GetAuthorizationToken before pulling.
	helperImage := envOr("SOCKERLESS_GL_HELPER_IMAGE",
		"729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live:gitlab-runner-helper-amd64")
	cmd := exec.Command("gitlab-runner", "register", "--non-interactive",
		"--url", defaultGLURL,
		"--token", authToken,
		"--executor", "docker",
		"--docker-image", "alpine:latest",
		"--docker-host", dockerHost,
		"--docker-helper-image", helperImage,
		"--description", description,
		"--config", configPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("gitlab-runner register: %v\n%s", err, out)
	}
}

func startRunner(t *testing.T, configPath string) func() {
	t.Helper()
	// `--debug` surfaces the per-stage transitions + executor-level
	// errors. Without it, the harness only sees WARNING+ — making
	// silent step_script skips invisible.
	cmd := exec.Command("gitlab-runner", "--debug", "run", "--config", configPath)
	cmd.Stdout = testLogWriter{t: t, prefix: "runner: "}
	cmd.Stderr = testLogWriter{t: t, prefix: "runner: "}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start gitlab-runner: %v", err)
	}
	time.Sleep(5 * time.Second)
	return func() {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(os.Interrupt)
		}
		_ = cmd.Wait()
	}
}

func defaultBranch(t *testing.T, pat []byte, project string) string {
	t.Helper()
	endpoint := defaultGLURL + "/api/v4/projects/" + url.PathEscape(project)
	req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	req.Header.Set("PRIVATE-TOKEN", string(pat))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("fetch project: %v", err)
	}
	defer resp.Body.Close()
	var p struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil || p.DefaultBranch == "" {
		return "main"
	}
	return p.DefaultBranch
}

func createBranch(t *testing.T, pat []byte, project, branch, ref string) {
	t.Helper()
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/repository/branches?branch=%s&ref=%s",
		defaultGLURL, url.PathEscape(project), url.QueryEscape(branch), url.QueryEscape(ref))
	req, _ := http.NewRequest(http.MethodPost, endpoint, nil)
	req.Header.Set("PRIVATE-TOKEN", string(pat))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create branch %s: %v", branch, err)
	}
	resp.Body.Close()
	if resp.StatusCode != 201 && resp.StatusCode != 400 { // 400 = already exists
		t.Fatalf("create branch %s: HTTP %d", branch, resp.StatusCode)
	}
}

func deleteBranch(t *testing.T, pat []byte, project, branch string) {
	t.Helper()
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/repository/branches/%s",
		defaultGLURL, url.PathEscape(project), url.PathEscape(branch))
	req, _ := http.NewRequest(http.MethodDelete, endpoint, nil)
	req.Header.Set("PRIVATE-TOKEN", string(pat))
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

func commitPipeline(t *testing.T, pat []byte, project, branch, body string) {
	t.Helper()
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/repository/files/%s",
		defaultGLURL, url.PathEscape(project), url.PathEscape(".gitlab-ci.yml"))
	payload := map[string]string{
		"branch":         branch,
		"content":        body,
		"commit_message": "test: harness pipeline",
	}
	bodyBytes, _ := json.Marshal(payload)
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		req, _ := http.NewRequest(method, endpoint, strings.NewReader(string(bodyBytes)))
		req.Header.Set("PRIVATE-TOKEN", string(pat))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("commit pipeline (%s): %v", method, err)
		}
		resp.Body.Close()
		if resp.StatusCode == 200 || resp.StatusCode == 201 {
			return
		}
	}
	t.Fatal("commit pipeline: both POST and PUT failed")
}

func triggerPipeline(t *testing.T, pat []byte, project, branch string) int64 {
	t.Helper()
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/pipeline?ref=%s",
		defaultGLURL, url.PathEscape(project), url.QueryEscape(branch))
	req, _ := http.NewRequest(http.MethodPost, endpoint, nil)
	req.Header.Set("PRIVATE-TOKEN", string(pat))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("trigger pipeline: %v", err)
	}
	defer resp.Body.Close()
	var p struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatalf("parse pipeline response: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("pipeline trigger returned id=0")
	}
	return p.ID
}

func waitForPipeline(t *testing.T, pat []byte, project string, id int64, timeout time.Duration) string {
	t.Helper()
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/pipelines/%d",
		defaultGLURL, url.PathEscape(project), id)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
		req.Header.Set("PRIVATE-TOKEN", string(pat))
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			var p struct {
				Status string `json:"status"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&p)
			resp.Body.Close()
			switch p.Status {
			case "success", "failed", "canceled", "skipped":
				return p.Status
			}
			t.Logf("pipeline %d: status=%s", id, p.Status)
		}
		time.Sleep(pollInterval)
	}
	t.Fatalf("pipeline %d did not complete within %s", id, timeout)
	return ""
}

func pipelineYAML(tag string, scriptLines ...string) string {
	return fmt.Sprintf(`hello:
  image: alpine:latest
  tags:
    - %s
  script:
%s
`, tag, strings.Join(scriptLines, "\n"))
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
