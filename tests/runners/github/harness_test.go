//go:build github_runner_live

package github_runner_test

import (
	"context"
	"encoding/base64"
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

// TestGitHub_ECS_Hello — cell 1 of the 4-cell matrix.
//
// Architecture: runner runs as a Fargate task in the live ECS
// cluster, with sockerless-backend-ecs baked into the same image and
// listening on `tcp://localhost:3375` inside the task. The runner's
// `docker create -v /home/runner/_work:/__w alpine` (from the
// `container: alpine:latest` directive in `hello-ecs.yml`) flows
// through sockerless, which translates the host bind mount to the
// shared EFS access point and dispatches the alpine sub-task to ECS
// with the same EFS volume mounted at `/__w`. Both runner-task and
// sub-task see the same workspace via EFS — `container:` works
// end-to-end on Fargate.
//
// Required env (read from the live ECS terragrunt outputs):
//   - SOCKERLESS_ECS_TEST_REGION (default eu-west-1)
//   - SOCKERLESS_ECS_TEST_CLUSTER (default sockerless-live)
//   - SOCKERLESS_ECS_TEST_TASK_DEFINITION (default sockerless-live-runner)
//   - SOCKERLESS_ECS_TEST_SUBNETS (comma-separated, must be in the cluster's VPC)
//   - SOCKERLESS_ECS_TEST_SECURITY_GROUPS (comma-separated)
func TestGitHub_ECS_Hello(t *testing.T) {
	subnets := os.Getenv("SOCKERLESS_ECS_TEST_SUBNETS")
	sgs := os.Getenv("SOCKERLESS_ECS_TEST_SECURITY_GROUPS")
	if subnets == "" || sgs == "" {
		t.Skip("SOCKERLESS_ECS_TEST_SUBNETS / SOCKERLESS_ECS_TEST_SECURITY_GROUPS not set; cell 1 needs live infra. Run `terragrunt output` and export them.")
	}
	runCell(t, cellConfig{
		Label:             "sockerless-ecs",
		WorkflowFile:      "hello-ecs.yml",
		ECSTaskDefinition: envOr("SOCKERLESS_ECS_TEST_TASK_DEFINITION", "sockerless-live-runner"),
		ECSRegion:         envOr("SOCKERLESS_ECS_TEST_REGION", "eu-west-1"),
		ECSCluster:        envOr("SOCKERLESS_ECS_TEST_CLUSTER", "sockerless-live"),
		ECSSubnets:        subnets,
		ECSSecurityGroups: sgs,
	})
}

// TestGitHub_Lambda_Hello — cell 2 of the 4-cell matrix.
//
// Architecture: runner runs as a Lambda invocation. Sockerless-
// backend-ecs is baked into the runner image; the bootstrap starts
// it on localhost:3375 inside the Lambda execution environment.
// `docker create` calls flow through sockerless to ECS Fargate
// sub-task spawns (sub-tasks dispatch to ECS, not back to Lambda,
// to avoid Lambda-in-Lambda recursion). Lambda's 15-minute hard
// cap means cell 2 is restricted to short workflows.
//
// Required env (Lambda runner-function deployed via
// `terraform/modules/lambda/runner.tf`):
//   - SOCKERLESS_LAMBDA_TEST_REGION (default eu-west-1)
//   - SOCKERLESS_LAMBDA_TEST_FUNCTION (default sockerless-live-runner)
func TestGitHub_Lambda_Hello(t *testing.T) {
	region := envOr("SOCKERLESS_LAMBDA_TEST_REGION", "eu-west-1")
	function := envOr("SOCKERLESS_LAMBDA_TEST_FUNCTION", "sockerless-live-runner")
	if function == "" {
		t.Skip("SOCKERLESS_LAMBDA_TEST_FUNCTION not set; cell 2 needs the runner-Lambda live infra")
	}
	runCell(t, cellConfig{
		Label:          "sockerless-lambda",
		WorkflowFile:   "hello-lambda.yml",
		LambdaFunction: function,
		LambdaRegion:   region,
	})
}

type cellConfig struct {
	Label string
	// Name of a workflow file already present on `main`. Dispatched
	// with `ref=main` (or `ref=<branch>` if WorkflowYAML is set, in
	// which case the throwaway branch's content of this same path
	// runs instead).
	WorkflowFile string
	// Optional workflow YAML body to commit to a throwaway branch at
	// `WorkflowFile`'s path; when set, dispatch fires with the
	// throwaway branch as ref. Useful for cell variants that need
	// different workflow content without polluting main.
	WorkflowYAML string
	// Default DOCKER_HOST when spawning the runner as a *local*
	// container (the dev-mode path against local Podman / Docker
	// Desktop). Ignored when ECSTaskDefinition is set.
	DefaultDockerHost string
	// When set, runCell skips the local-Podman runner-image build +
	// docker-run, and instead spawns the runner via AWS ECS RunTask
	// using the named pre-registered task definition family. The
	// runner runs in Fargate; sockerless is baked into the runner
	// image and listens on the task's localhost. Cell exits when the
	// ephemeral task stops.
	ECSTaskDefinition string
	// AWS region for the ECS dispatch. Required when ECSTaskDefinition is set.
	ECSRegion string
	// ECS cluster name for the dispatch. Required when ECSTaskDefinition is set.
	ECSCluster string
	// Subnets for awsvpc network mode. Comma-separated. Required when ECSTaskDefinition is set.
	ECSSubnets string
	// Security groups for awsvpc network mode. Comma-separated. Required when ECSTaskDefinition is set.
	ECSSecurityGroups string
	// LambdaFunction (when set) takes precedence over both local-
	// Podman and ECSTaskDefinition paths: runCell invokes the named
	// Lambda function asynchronously with the registration token /
	// labels / repo URL in the event payload. The runner-Lambda's
	// bootstrap polls the Runtime API for the next invocation,
	// configures + runs actions/runner --ephemeral, and exits.
	LambdaFunction string
	// AWS region for the Lambda invocation. Required when LambdaFunction is set.
	LambdaRegion string
}

func runCell(t *testing.T, c cellConfig) {
	repo := envOr("SOCKERLESS_GH_REPO", defaultRepo)

	pat, err := runnersinternal.GitHubPAT()
	if err != nil {
		t.Skipf("GitHub PAT unavailable: %v", err)
	}
	defer zero(pat)

	if c.ECSTaskDefinition == "" {
		// Local dev mode: requires a local docker daemon (Podman/Docker
		// Desktop). The cloud-mode path uses ECS RunTask and doesn't
		// need a local docker.
		if _, err := exec.LookPath("docker"); err != nil {
			t.Skipf("docker CLI required to run the runner container: %v", err)
		}
		dockerHost := envOr("SOCKERLESS_DOCKER_HOST", c.DefaultDockerHost)
		pingDocker(t, dockerHost)
	}

	if err := runnersinternal.CleanupOldGitHubRunners(repo, "sockerless-"); err != nil {
		t.Logf("warning: pre-run cleanup of old runners failed: %v", err)
	}

	// Resolve workflow source — either an inline YAML committed to a
	// per-cell throwaway branch, or a pre-existing file on main.
	var workflowFile, dispatchRef string
	if c.WorkflowYAML != "" {
		workflowFile = c.WorkflowFile
		branch := fmt.Sprintf("sockerless-gh-%s-%s", c.Label, runnersinternal.Timestamp())
		mainSHA := resolveBranchSHA(t, repo, "main")
		createGitHubBranch(t, repo, branch, mainSHA)
		t.Cleanup(func() { deleteGitHubBranch(t, repo, branch) })
		commitGitHubFile(t, repo, branch,
			".github/workflows/"+workflowFile, c.WorkflowYAML,
			fmt.Sprintf("test: harness workflow for %s", c.Label))
		dispatchRef = branch
	} else {
		workflowFile = c.WorkflowFile
		dispatchRef = "main"
	}

	cancelLeftoverRuns(t, repo, workflowFile)

	regToken, err := runnersinternal.MintGitHubRegistrationToken(repo)
	if err != nil {
		t.Fatalf("mint registration token: %v", err)
	}

	runnerName := fmt.Sprintf("sockerless-%s-%s", c.Label, runnersinternal.Timestamp())
	t.Logf("runner name: %s", runnerName)

	if c.LambdaFunction != "" {
		invokeLambdaRunner(t, c, runnerName, repo, regToken)
	} else if c.ECSTaskDefinition != "" {
		taskARN := runEcsRunnerTask(t, c, runnerName, repo, regToken)
		t.Cleanup(func() { stopEcsRunnerTask(t, c, taskARN) })
	} else {
		dockerHost := envOr("SOCKERLESS_DOCKER_HOST", c.DefaultDockerHost)
		buildRunnerImage(t)
		containerID := startRunnerContainer(t, runnerName, repo, regToken, c.Label, dockerHost)
		t.Cleanup(func() { stopRunnerContainer(t, containerID) })
	}

	// Wait for the runner to register itself with GitHub. Until it
	// shows up in the runners list, dispatch can't route the job.
	waitForRunnerRegistration(t, repo, runnerName, 5*time.Minute)

	runID := dispatchWorkflow(t, repo, workflowFile, dispatchRef)
	t.Logf("workflow run URL: https://github.com/%s/actions/runs/%d", repo, runID)
	conclusion := waitForRun(t, repo, runID, 15*time.Minute)
	if conclusion != "success" {
		t.Fatalf("workflow run %d concluded with %q, expected success.\nLogs at https://github.com/%s/actions/runs/%d", runID, conclusion, repo, runID)
	}
}

// runEcsRunnerTask dispatches the runner-task to ECS Fargate via
// `aws ecs run-task` with container overrides for the per-cell env
// vars (REG_TOKEN / RUNNER_NAME / RUNNER_LABELS / RUNNER_REPO_URL).
// Returns the task ARN. Caller is responsible for cleanup via
// stopEcsRunnerTask.
func runEcsRunnerTask(t *testing.T, c cellConfig, runnerName, repo, regToken string) string {
	t.Helper()
	if _, err := exec.LookPath("aws"); err != nil {
		t.Fatalf("aws CLI required for ECS dispatch: %v", err)
	}
	overrides := fmt.Sprintf(`{
		"containerOverrides":[
			{
				"name":"runner",
				"environment":[
					{"name":"RUNNER_REPO_URL","value":"https://github.com/%s"},
					{"name":"RUNNER_TOKEN","value":"%s"},
					{"name":"RUNNER_NAME","value":"%s"},
					{"name":"RUNNER_LABELS","value":"%s,sockerless"}
				]
			}
		]
	}`, repo, regToken, runnerName, c.Label)

	subnetsJSON := strings.Join(quoteCSV(c.ECSSubnets), ",")
	sgsJSON := strings.Join(quoteCSV(c.ECSSecurityGroups), ",")
	netCfg := fmt.Sprintf(`{"awsvpcConfiguration":{"subnets":[%s],"securityGroups":[%s],"assignPublicIp":"DISABLED"}}`,
		subnetsJSON, sgsJSON)

	out, err := exec.Command("aws", "ecs", "run-task",
		"--region", c.ECSRegion,
		"--cluster", c.ECSCluster,
		"--task-definition", c.ECSTaskDefinition,
		"--launch-type", "FARGATE",
		"--network-configuration", netCfg,
		"--overrides", overrides,
		"--output", "json",
	).Output()
	if err != nil {
		t.Fatalf("aws ecs run-task: %v\n--overrides %s\n--network-configuration %s",
			extendErr(err), overrides, netCfg)
	}
	var resp struct {
		Tasks []struct {
			TaskArn string `json:"taskArn"`
		} `json:"tasks"`
		Failures []map[string]any `json:"failures"`
	}
	if jerr := json.Unmarshal(out, &resp); jerr != nil {
		t.Fatalf("parse run-task response: %v\n%s", jerr, out)
	}
	if len(resp.Failures) > 0 {
		t.Fatalf("ecs run-task failures: %v", resp.Failures)
	}
	if len(resp.Tasks) == 0 {
		t.Fatalf("ecs run-task: no task in response: %s", out)
	}
	taskARN := resp.Tasks[0].TaskArn
	t.Logf("ECS RunTask started: %s", taskARN)

	// Tail container logs in the background for visibility.
	go tailEcsTaskLogs(t, c, taskARN)

	return taskARN
}

// stopEcsRunnerTask sends `aws ecs stop-task` for graceful shutdown.
// The runner is `--ephemeral` so it usually exits on its own, but
// stop-task ensures cleanup if the test crashes mid-run.
func stopEcsRunnerTask(t *testing.T, c cellConfig, taskARN string) {
	t.Helper()
	out, err := exec.Command("aws", "ecs", "stop-task",
		"--region", c.ECSRegion,
		"--cluster", c.ECSCluster,
		"--task", taskARN,
		"--reason", "harness cleanup",
	).CombinedOutput()
	if err != nil {
		t.Logf("stop-task %s (best-effort): %v\n%s", taskARN, err, out)
	}
}

// tailEcsTaskLogs streams the runner container's CloudWatch logs to
// the test output. Best-effort — uses `aws logs tail` which is
// simpler than the full GetLogEvents pagination.
func tailEcsTaskLogs(t *testing.T, c cellConfig, taskARN string) {
	parts := strings.Split(taskARN, "/")
	if len(parts) == 0 {
		return
	}
	taskID := parts[len(parts)-1]
	logStream := "runner/runner/" + taskID
	logGroup := "/sockerless/live/containers"
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "aws", "logs", "tail",
		"--region", c.ECSRegion,
		"--follow",
		"--log-stream-names", logStream,
		logGroup,
	)
	cmd.Stdout = testLogWriter{t: t, prefix: "ecs-runner: "}
	cmd.Stderr = testLogWriter{t: t, prefix: "ecs-runner: "}
	_ = cmd.Run()
}

func quoteCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, "\""+p+"\"")
	}
	return out
}

// extendErr enriches an exec.ExitError with the captured stderr so
// tests print useful diagnostics instead of just "exit status 254".
func extendErr(err error) error {
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
	}
	return err
}

// invokeLambdaRunner dispatches the runner-Lambda function via
// `aws lambda invoke --invocation-type Event` (async) with the
// per-cell registration token / labels / repo URL in the payload.
// The bootstrap inside the Lambda picks up the event from the
// Runtime API, runs `actions/runner --ephemeral` for one job, and
// exits. The harness then waits for runner registration + workflow
// completion via the standard polling paths. No cleanup function
// — the Lambda invocation is self-terminating once the runner
// exits.
func invokeLambdaRunner(t *testing.T, c cellConfig, runnerName, repo, regToken string) {
	t.Helper()
	if _, err := exec.LookPath("aws"); err != nil {
		t.Fatalf("aws CLI required for Lambda dispatch: %v", err)
	}
	payload := fmt.Sprintf(`{"runner_repo_url":"https://github.com/%s","runner_token":"%s","runner_name":"%s","runner_labels":"%s,sockerless"}`,
		repo, regToken, runnerName, c.Label)

	tmp, err := os.CreateTemp("", "lambda-out-*.json")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	tmp.Close()
	t.Cleanup(func() { _ = os.Remove(tmp.Name()) })

	cmd := exec.Command("aws", "lambda", "invoke",
		"--region", c.LambdaRegion,
		"--function-name", c.LambdaFunction,
		"--invocation-type", "Event",
		"--cli-binary-format", "raw-in-base64-out",
		"--payload", payload,
		tmp.Name(),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("aws lambda invoke: %v\n%s", err, out)
	}
	t.Logf("Lambda invocation queued: function=%s runner=%s", c.LambdaFunction, runnerName)
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
	// `host.docker.internal` is auto-resolved on both Docker Desktop
	// and Podman 5.x (and Podman also auto-adds `host.containers.internal`).
	// No `--add-host` flag is needed; in fact, `--add-host
	// host.docker.internal:host-gateway` *fails* on Podman 5.x with
	// "host containers internal IP address is empty" because Podman
	// doesn't honor the host-gateway literal — it overrides the
	// auto-provided alias and then can't resolve the literal.
	innerHost := strings.Replace(dockerHost, "localhost", "host.docker.internal", 1)
	innerHost = strings.Replace(innerHost, "127.0.0.1", "host.docker.internal", 1)

	args := []string{"run", "-d", "--rm",
		"--name", name,
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

func dispatchWorkflow(t *testing.T, repo, file, ref string) int64 {
	t.Helper()
	disp := exec.Command("gh", "api", "-X", "POST",
		"/repos/"+repo+"/actions/workflows/"+file+"/dispatches",
		"-f", "ref="+ref,
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

// resolveBranchSHA returns the head commit SHA of the given branch.
func resolveBranchSHA(t *testing.T, repo, branch string) string {
	t.Helper()
	out, err := exec.Command("gh", "api",
		"/repos/"+repo+"/git/refs/heads/"+branch,
		"--jq", ".object.sha",
	).Output()
	if err != nil {
		t.Fatalf("resolve %s sha: %v", branch, err)
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		t.Fatalf("resolve %s sha: empty result", branch)
	}
	return sha
}

// createGitHubBranch creates a new branch on the repo at the given
// commit SHA. Uses the Refs API (POST /repos/.../git/refs).
func createGitHubBranch(t *testing.T, repo, branch, fromSHA string) {
	t.Helper()
	cmd := exec.Command("gh", "api", "-X", "POST",
		"/repos/"+repo+"/git/refs",
		"-f", "ref=refs/heads/"+branch,
		"-f", "sha="+fromSHA,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create branch %s: %v\n%s", branch, err, out)
	}
	t.Logf("created throwaway branch %s @ %s", branch, fromSHA[:8])
}

// deleteGitHubBranch removes a branch via the Refs API. Best-effort —
// logs but doesn't fail the test if the branch is already gone.
func deleteGitHubBranch(t *testing.T, repo, branch string) {
	t.Helper()
	cmd := exec.Command("gh", "api", "-X", "DELETE",
		"/repos/"+repo+"/git/refs/heads/"+branch,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("delete branch %s (best-effort): %v\n%s", branch, err, out)
		return
	}
	t.Logf("deleted throwaway branch %s", branch)
}

// commitGitHubFile commits a single file's content to a branch via
// the Contents API (PUT /repos/.../contents/{path}). Uses base64 for
// the content body. Tries POST first (for new files); if the path
// already exists, fetches the file's blob SHA and retries with the
// `sha` parameter (which the API requires for updates).
func commitGitHubFile(t *testing.T, repo, branch, path, content, message string) {
	t.Helper()
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	args := []string{"api", "-X", "PUT",
		"/repos/" + repo + "/contents/" + path,
		"-f", "branch=" + branch,
		"-f", "message=" + message,
		"-f", "content=" + encoded,
	}
	if out, err := exec.Command("gh", args...).CombinedOutput(); err != nil {
		// 422 means the file already exists; need to provide the
		// existing blob SHA. (Throwaway branches branched from main
		// shouldn't have this file, but be defensive.)
		shaOut, shaErr := exec.Command("gh", "api",
			"/repos/"+repo+"/contents/"+path+"?ref="+branch,
			"--jq", ".sha",
		).Output()
		if shaErr != nil {
			t.Fatalf("commit file %s on %s: %v\n%s", path, branch, err, out)
		}
		args = append(args, "-f", "sha="+strings.TrimSpace(string(shaOut)))
		if out2, err2 := exec.Command("gh", args...).CombinedOutput(); err2 != nil {
			t.Fatalf("commit file %s on %s (retry with sha): %v\n%s", path, branch, err2, out2)
		}
	}
	t.Logf("committed %s on branch %s", path, branch)
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
