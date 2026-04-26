# GitHub Actions real runner harness

End-to-end test that registers a real `actions/runner` against a GitHub repo, points it at sockerless via `DOCKER_HOST`, dispatches a workflow, and asserts the run completes successfully.

## Prerequisites

- **GitHub repo** that you have admin access to (for self-hosted runner registration).
- **GitHub PAT** with `repo` + `workflow` scopes (or the runner registration token directly).
- **`gh` CLI** authenticated in the same shell (`gh auth status`).
- **sockerless backend** already running, with `DOCKER_HOST` set to it. The harness does not start the backend — that's the operator's responsibility (typically via `manual-tests/01-infrastructure.md` or a local docker daemon).

## Env vars

| Var | Required | Purpose |
|---|---|---|
| `SOCKERLESS_GH_RUNNER_TOKEN` | yes | runner registration token from `gh api -X POST /repos/{repo}/actions/runners/registration-token` |
| `SOCKERLESS_GH_REPO` | yes | full repo path, e.g. `e6qu/sockerless-runner-test` |
| `SOCKERLESS_GH_RUNNER_LABELS` | no, defaults to `sockerless,sockerless-ecs` | comma-separated labels for the runner |
| `SOCKERLESS_GH_RUNNER_VERSION` | no, defaults to a known-good version | actions/runner release tag |
| `DOCKER_HOST` | yes | already pointing at the running sockerless backend |

If the test runs without `SOCKERLESS_GH_RUNNER_TOKEN` set, it skips with a one-line note. Sim CI runs this as a no-op.

## How to run

```bash
# 1. Provision live infra and start sockerless (per manual-tests/01-infrastructure.md).
# 2. Get a runner registration token.
export SOCKERLESS_GH_RUNNER_TOKEN=$(gh api -X POST /repos/$REPO/actions/runners/registration-token --jq .token)
export SOCKERLESS_GH_REPO=$REPO

# 3. Run the harness.
cd tests/runners/github
go test -v -tags github_runner_live -run TestRealGitHubRunner -timeout 30m
```

## Workflows

Sample workflow YAMLs live in [`workflows/`](workflows/) and get committed to the target repo before the test runs (the harness handles that):

- [`hello-ecs.yml`](workflows/hello-ecs.yml) — minimal `echo` job to verify end-to-end plumbing.
- [`gotest-ecs.yml`](workflows/gotest-ecs.yml) — a Go module build + test inside a container, exercising multi-step exec.

Add more workflows to fill out the canonical sweep (matrix builds, services, artifacts, secrets, fail-fast, log streaming) per [PLAN.md § Phase 106](../../../PLAN.md).

## What gets verified

- **Runner registration succeeds** — `actions/runner config.sh` exits 0, runner shows online in the repo.
- **Workflow dispatch returns a run ID** via `gh api -X POST /repos/.../dispatches`.
- **Run completes with `conclusion=success`** within the timeout.
- **Per-job logs** contain the expected output (e.g. `hello world`).

Failures get filed as bugs (BUG-836+ once the harness lands), fixed in-phase per the no-defer rule.

## Cleanup

The harness unregisters the runner on exit (defer). If the test crashes mid-run, manually remove the runner via the repo settings page or:

```bash
gh api -X DELETE /repos/$REPO/actions/runners/$ID
```

Live cloud infra teardown is the operator's responsibility — see [manual-tests/01-infrastructure.md](../../../manual-tests/01-infrastructure.md).
