# Real CI runner integration

End-to-end tests that run a *real* CI runner binary (not a Docker-API replay) against sockerless via `DOCKER_HOST`. Two suites:

- [`github/`](github/) — `actions/runner` registered with a real GitHub repo or organisation, executing real workflow runs.
- [`gitlab/`](gitlab/) — `gitlab-runner` with the docker executor, registered against gitlab.com (project-scoped) or the project's `origin-gitlab` mirror.

The synthetic Docker-API replay tests live alongside in [`tests/github_runner_e2e_test.go`](../github_runner_e2e_test.go) and [`tests/gitlab_runner_e2e_test.go`](../gitlab_runner_e2e_test.go); those exercise the call sequence the runners make, not the runner binaries themselves.

## When to run

- Before tagging a release that touches the cross-backend driver framework or any backend's exec/logs/attach paths.
- After landing a backend change that's been tested only against the simulator — to verify the runner's actual call sequence against live cloud infra.
- Periodically as a regression sweep.

The synthetic tests + sim-parity matrix cover wire-format correctness; these harnesses are the "does the real runner actually work" check.

## Architecture

Both harnesses follow the same shape:

1. **Skip unless env vars are set.** Real-runner tests need credentials + a target repo / project; absent that, the test is a no-op.
2. **Start sockerless** against the configured backend (typically ECS for AWS or Cloud Run for GCP), expose `DOCKER_HOST=tcp://localhost:<port>`.
3. **Download the runner binary** if not cached locally.
4. **Register the runner** with the real CI service using a registration token.
5. **Trigger a workflow / pipeline** — for GH this dispatches via `gh api`; for GitLab this pushes a tag or commits to a branch.
6. **Poll for completion** via the CI API — record the per-job status, exit code, log excerpts.
7. **Tear down** — unregister the runner, stop sockerless, optionally `terragrunt destroy` the live infra.

ECS is the default backend for everything (long-running, exec-able, multi-step). Lambda is opt-in for fast one-shots (≤15 min, no service container, no `docker attach`) — selected via runner labels.

## Backend routing

Runner labels select the backend per workflow / pipeline:

| Label / tag | Backend | Use case |
|---|---|---|
| `sockerless-ecs` | ECS | Default — most jobs, services, multi-step builds |
| `sockerless-lambda` | Lambda | Fast one-shots (lint, fast unit tests, container actions) |

Today this is implemented via **per-backend daemons**: one sockerless instance per backend, each on its own port, with the runner host running one self-hosted runner per `runs-on:` label and DOCKER_HOST pointing at the matching port. **Phase 68** (Multi-Tenant Backend Pools, queued in [PLAN.md](../../PLAN.md)) replaces this with label-based dispatch in a single sockerless daemon.

## Cost / posture

Time-boxed manual runs: provision live infra, run the canonical sweep, tear down. Per-cloud `null_resource sockerless_runtime_sweep` makes `terragrunt destroy` self-sufficient. AWS root-account key state is tracked in [`STATUS.md`](../../STATUS.md); reactivate before a run, deactivate after.

## Cross-links

- [PLAN.md § Phase 106](../../PLAN.md) — GH runner integration architecture
- [PLAN.md § Phase 107](../../PLAN.md) — GitLab runner integration architecture
- [docs/GITHUB_RUNNER_SAAS.md](../../docs/GITHUB_RUNNER_SAAS.md), [docs/GITHUB_RUNNER.md](../../docs/GITHUB_RUNNER.md)
- [docs/GITLAB_RUNNER_DOCKER.md](../../docs/GITLAB_RUNNER_DOCKER.md), [docs/GITLAB_RUNNER_SAAS.md](../../docs/GITLAB_RUNNER_SAAS.md)
- [docs/runner-capability-matrix.md](../../docs/runner-capability-matrix.md)
- [manual-tests/02-aws-runbook.md § Track J](../../manual-tests/02-aws-runbook.md) — runner integration row in the AWS canonical sweep
