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
)

// Real GitLab runner harness. Gated by the `gitlab_runner_live` build
// tag. Run via:
//
//   go test -v -tags gitlab_runner_live -run TestRealGitLabRunner -timeout 30m
//
// Prereqs in tests/runners/gitlab/README.md.

const (
	defaultGLURL  = "https://gitlab.com"
	defaultGLTags = "sockerless,sockerless-ecs"
	pollInterval  = 5 * time.Second
)

func TestRealGitLabRunner(t *testing.T) {
	regToken := os.Getenv("SOCKERLESS_GL_RUNNER_TOKEN")
	project := os.Getenv("SOCKERLESS_GL_PROJECT")
	apiToken := os.Getenv("SOCKERLESS_GL_API_TOKEN")
	if regToken == "" || project == "" {
		t.Skip("SOCKERLESS_GL_RUNNER_TOKEN + SOCKERLESS_GL_PROJECT not set — skipping")
	}
	if apiToken == "" {
		t.Skip("SOCKERLESS_GL_API_TOKEN not set — needed for pipeline triggering")
	}
	if os.Getenv("DOCKER_HOST") == "" {
		t.Fatal("DOCKER_HOST must be set to a running sockerless instance")
	}

	glURL := envOr("SOCKERLESS_GL_URL", defaultGLURL)
	tags := envOr("SOCKERLESS_GL_RUNNER_TAGS", defaultGLTags)

	workdir := t.TempDir()
	t.Logf("runner workdir: %s", workdir)

	configPath := filepath.Join(workdir, "config.toml")
	authToken := registerRunner(t, glURL, regToken, tags, configPath)
	t.Cleanup(func() { unregisterRunner(t, glURL, authToken, configPath) })

	runnerCancel := startRunner(t, configPath)
	t.Cleanup(runnerCancel)

	commitPipeline(t, glURL, apiToken, project, helloPipelineYAML(tags))
	pipelineID := triggerPipeline(t, glURL, apiToken, project)
	status := waitForPipeline(t, glURL, apiToken, project, pipelineID, 15*time.Minute)
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

// registerRunner shells out to `gitlab-runner register --non-interactive`
// and writes a config.toml at the supplied path. Returns the runner's
// long-lived auth token (parsed from the config file) so the cleanup
// step can unregister it.
func registerRunner(t *testing.T, glURL, regToken, tags, configPath string) string {
	t.Helper()
	dockerHost := os.Getenv("DOCKER_HOST")
	cmd := exec.Command("gitlab-runner", "register", "--non-interactive",
		"--url", glURL,
		"--registration-token", regToken,
		"--executor", "docker",
		"--docker-image", "alpine:latest",
		"--docker-host", dockerHost,
		"--tag-list", tags,
		"--description", "sockerless-test-runner-"+timestamp(),
		"--locked", "false",
		"--access-level", "not_protected",
		"--config", configPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("register runner: %v\n%s", err, out)
	}
	return parseRunnerToken(t, configPath)
}

// parseRunnerToken pulls the `token = "..."` line out of a config.toml.
// Hand-parsed because the standard `tomlite` libs aren't a dep here.
func parseRunnerToken(t *testing.T, configPath string) string {
	t.Helper()
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "token = ") {
			return strings.Trim(strings.TrimPrefix(line, "token = "), `"`)
		}
	}
	t.Fatal("config.toml missing runner token")
	return ""
}

func unregisterRunner(t *testing.T, glURL, authToken, configPath string) {
	t.Helper()
	if authToken == "" {
		return
	}
	cmd := exec.Command("gitlab-runner", "unregister",
		"--url", glURL,
		"--token", authToken,
		"--config", configPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("unregister runner: %v\n%s", err, out)
	}
}

func startRunner(t *testing.T, configPath string) func() {
	t.Helper()
	cmd := exec.Command("gitlab-runner", "run", "--config", configPath)
	cmd.Stdout = testLogWriter{t: t, prefix: "runner: "}
	cmd.Stderr = testLogWriter{t: t, prefix: "runner: "}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start runner: %v", err)
	}
	time.Sleep(5 * time.Second) // give it a moment to settle
	return func() {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(os.Interrupt)
		}
		_ = cmd.Wait()
	}
}

// commitPipeline writes .gitlab-ci.yml to the default branch via the
// GitLab Repository Files API.
func commitPipeline(t *testing.T, glURL, apiToken, project, body string) {
	t.Helper()
	encoded := url.PathEscape(".gitlab-ci.yml")
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/repository/files/%s",
		glURL, url.PathEscape(project), encoded)

	branch := defaultBranch(t, glURL, apiToken, project)
	payload := map[string]string{
		"branch":         branch,
		"content":        body,
		"commit_message": "test: harness pipeline update",
	}
	body1 := mustJSON(payload)

	// Try update (PUT). If the file doesn't exist, fall back to POST
	// (create). This is two paths because GitLab returns 400 for
	// PUT-on-missing instead of an explicit signal.
	for _, method := range []string{http.MethodPut, http.MethodPost} {
		req, _ := http.NewRequest(method, endpoint, strings.NewReader(body1))
		req.Header.Set("PRIVATE-TOKEN", apiToken)
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
	t.Fatal("commit pipeline: both PUT and POST failed")
}

func defaultBranch(t *testing.T, glURL, apiToken, project string) string {
	t.Helper()
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s", glURL, url.PathEscape(project))
	req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	req.Header.Set("PRIVATE-TOKEN", apiToken)
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

func triggerPipeline(t *testing.T, glURL, apiToken, project string) int64 {
	t.Helper()
	branch := defaultBranch(t, glURL, apiToken, project)
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/pipeline?ref=%s",
		glURL, url.PathEscape(project), url.QueryEscape(branch))
	req, _ := http.NewRequest(http.MethodPost, endpoint, nil)
	req.Header.Set("PRIVATE-TOKEN", apiToken)
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

func waitForPipeline(t *testing.T, glURL, apiToken, project string, id int64, timeout time.Duration) string {
	t.Helper()
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/pipelines/%d",
		glURL, url.PathEscape(project), id)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest(http.MethodGet, endpoint, nil)
		req.Header.Set("PRIVATE-TOKEN", apiToken)
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

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func helloPipelineYAML(tags string) string {
	primary := strings.Split(tags, ",")[0]
	return fmt.Sprintf(`hello:
  image: alpine:latest
  tags:
    - %s
  script:
    - echo "hello from sockerless"
    - env | sort
`, primary)
}

func timestamp() string {
	return time.Now().UTC().Format("20060102-150405")
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
