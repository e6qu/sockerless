# GitLab runner real harness

End-to-end test that registers a real `gitlab-runner` (docker executor) against a GitLab project, points it at sockerless via `runners.docker.host`, triggers a pipeline, and asserts it completes successfully.

## Prerequisites

- **GitLab project** with CI/CD enabled. Either gitlab.com or the project's [`origin-gitlab` mirror](../../../STATUS.md). The mirror needs CI enabled (a one-time settings flip if it's currently push-only).
- **Project access token** with `api` scope (or a runner registration token from the project's CI/CD settings).
- **`gitlab-runner` binary** — the harness downloads it if not cached.
- **`glab` CLI** authenticated, OR `curl` + a personal access token with `api` scope.
- **sockerless backend** already running with `DOCKER_HOST` set.

## Env vars

| Var | Required | Purpose |
|---|---|---|
| `SOCKERLESS_GL_RUNNER_TOKEN` | yes | runner registration token |
| `SOCKERLESS_GL_PROJECT` | yes | project path, e.g. `e6qu/sockerless-runner-test` |
| `SOCKERLESS_GL_URL` | no, defaults to `https://gitlab.com` | GitLab instance URL |
| `SOCKERLESS_GL_RUNNER_TAGS` | no, defaults to `sockerless,sockerless-ecs` | runner tags |
| `SOCKERLESS_GL_API_TOKEN` | yes (for pipeline triggering) | personal access token with `api` scope |
| `DOCKER_HOST` | yes | already pointing at the running sockerless backend |

If `SOCKERLESS_GL_RUNNER_TOKEN` isn't set, the test skips. Sim CI runs this as a no-op.

## How to run

```bash
# 1. Provision live infra and start sockerless.
# 2. Get a runner registration token from project Settings → CI/CD → Runners.
export SOCKERLESS_GL_RUNNER_TOKEN=<token>
export SOCKERLESS_GL_PROJECT=$PROJECT
export SOCKERLESS_GL_API_TOKEN=$PAT

# 3. Run the harness.
cd tests/runners/gitlab
go test -v -tags gitlab_runner_live -run TestRealGitLabRunner -timeout 30m
```

## Pipelines

Sample `.gitlab-ci.yml` snippets live in [`pipelines/`](pipelines/). The harness commits one to the project before triggering a pipeline.

## Mirror-side prep (one-time)

If using the `origin-gitlab` mirror:

1. In the GitLab project Settings → General → Visibility, enable **CI/CD**.
2. Settings → CI/CD → Runners → expand **Specific runners** → click **New project runner** → copy the registration token.
3. Set `SOCKERLESS_GL_PROJECT` to the mirror's path.

Alternative: spin up a self-hosted GitLab CE in a container for fully-isolated runs. Heavier but no external dependencies.

## Cleanup

The harness unregisters the runner on exit. Manual cleanup if needed:

```bash
gitlab-runner unregister --url $SOCKERLESS_GL_URL --token <runner-auth-token>
```
