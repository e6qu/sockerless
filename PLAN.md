# Sockerless — Roadmap

> 85 phases complete (756 tasks). 583 bugs fixed, 0 open.
>
> **Goal:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners — backed by real cloud infrastructure (AWS, GCP, Azure).

## Guiding Principles

1. **Docker API fidelity** — match Docker's REST API exactly
2. **Real execution** — simulators and backends actually run commands
3. **External validation** — proven by unmodified external test suites
4. **No new frontend abstractions** — Docker REST API is the only interface
5. **Driver-first handlers** — all handler code through driver interfaces
6. **LLM-editable files** — source files under 400 lines
7. **GitHub API fidelity** — bleephub works with unmodified `gh` CLI
8. **State persistence** — every task ends with state save

---

## Phase 68 — Multi-Tenant Backend Pools (In Progress)

Named pools of backends with scheduling and resource limits.

| Task | Status | Description |
|---|---|---|
| P68-001 | done | Pool configuration (JSON config) |
| P68-002 | | Pool registry (in-memory, each with own BaseServer + Store) |
| P68-003 | | Request router (route by label or default pool) |
| P68-004 | | Concurrency limiter (per-pool semaphore, 429 on overflow) |
| P68-005 | | Pool lifecycle (create/destroy at runtime via management API) |
| P68-006 | | Pool metrics (per-pool stats on `/internal/metrics`) |
| P68-007 | | Round-robin scheduling (multi-backend pools) |
| P68-008 | | Resource limits (max containers, max memory per pool) |
| P68-009 | | Unit + integration tests |
| P68-010 | | Save final state |

---

## Phase 78 — UI Polish

Dark mode, design tokens, error handling UX, container detail modal, auto-refresh, performance audit, accessibility, E2E smoke, documentation.

---

## Phase 86 — Complete Runner Support (ECS + Lambda × github.com + gitlab.com SaaS)

Close the gap between "Docker API passes E2E in simulator mode" and "official `actions/runner` + `gitlab-runner` binaries run real CI jobs on real AWS against real github.com / gitlab.com." No `-wasm` / synthetic shortcuts: services, custom images, and docker build must work for real. Lambda 15-min cap accepted.

Work partitioned into **no-AWS-credentials** (can be done now, verified in simulator) and **needs-AWS** (verified against live accounts).

### No-AWS track

| Task | Status | Description |
|---|---|---|
| P86-001 | done | Dropped `-wasm` / `-faas` variant-routing. `get_test_variant` and `should_skip_for_faas` removed from `tests/e2e-live-tests/lib.sh`; orchestrators use test names directly |
| P86-002 | done | Pruned `services` / `custom-image` from `ALL_WORKFLOWS` / `ALL_PIPELINES` (files removed in daeff00). Renamed `container-action-faas.yml` → `container-action.yml`. `docs/runner-capability-matrix.md` added as TBD template |
| P86-003 | done | Per-hostname Cloud Map services + `DnsSearchDomains` on task def. Old shared `containers` service removed. Unit tests + `docs/ECS_SERVICES_DESIGN.md`. Full DNS end-to-end verification belongs to the live-AWS track |
| P86-004 | done | Lambda `ContainerStop` / `ContainerKill` clamp function timeout + disconnect reverse-agent stub + close wait channel. `docker logs -f` now lazy-resolves the CloudWatch log stream so follow-mode returns output even when opened before the Lambda has been invoked |
| P86-005 | done (skeleton) | Design doc `docs/LAMBDA_EXEC_DESIGN.md`; `sockerless-lambda-bootstrap` binary skeleton; `backends/lambda/image_inject.go` + 3 unit tests; `LambdaConfig.CallbackURL` env-wired. Full Runtime API loop deferred to AWS track |
| P86-006 | | Add `--runner official` switch to E2E harnesses to target unmodified `actions/runner` and `gitlab-runner` binaries (vs `act` / self-hosted GitLab CE) |
| P86-007 | done | Docs: `docs/ECS_LIVE_SETUP.md`, `docs/GITHUB_RUNNER_SAAS.md`, `docs/GITLAB_RUNNER_SAAS.md` added; pointers from the existing `GITHUB_RUNNER.md` and `GITLAB_RUNNER_DOCKER.md` to the SaaS versions |
| P86-008 | done | Unit tests for `searchDomainsForContainer` (4), `RenderOverlayDockerfile` (3); integration test `TestLambdaContainerStopUnblocksWait`, `TestLambdaContainerLogsFollowLazyStream`. Lambda test-main now runs unit tests when integration off |

### Bug-fix track (no AWS; unblocks AWS track)

| Task | Status | Description |
|---|---|---|
| P86-015 | | Fix BUG-693: port `backends/lambda/image_resolve.go` resolver to ECS. `backends/ecs/image_resolve.go` + call from `ContainerCreate` before storing image on container. Unit test + `taskdef` test asserting ECR URI in rendered def |
| P86-016 | | Fix BUG-692: `backends/core/handle_containers_query.go:handleContainerAttach` returns immediately because cloud backends' `ContainerAttach` gives an EOF pipe (no local Docker to attach to). Diagnose precisely via simulator repro, then either (a) block the pipe on container exit, (b) stream `ContainerLogs` through the attach stream, or (c) hold the hijacked conn open until the container transitions to STOPPED |
| P86-017 | | Simulator E2E coverage: a regression test that drives `docker run --rm alpine echo x` against ECS backend in simulator mode and asserts success. This would have caught BUG-692 in CI; make sure it's now part of the default test matrix |

### Needs-AWS track (manual session 2, blocked until P86-015/016/017 land)

| Task | Status | Description |
|---|---|---|
| P86-009 | partial | Session 1 2026-04-19: Terraform up, 34 resources, zero-residue teardown verified. Session 2 re-provisions after bug fixes |
| P86-010 | blocked-on-bugs | Runbook 1 — ECS smoke + services DNS |
| P86-011 | blocked-on-bugs | Runbook 4 — `actions/runner` × real github.com; 3 shapes (shell, `container:`, `services:`). Requires user-provided PAT + test repo |
| P86-012 | blocked-on-bugs | Runbook 5 — `gitlab-runner` × real gitlab.com; same matrix. Requires runner token + test project |
| P86-013 | blocked-on-bugs | Runbook 2 — Lambda live Docker CLI baseline. Writes `docs/PLAN_LAMBDA_MANUAL_TESTING.md` Round-1 |
| P86-018 | blocked-on-bugs | Runbook 3a — implement Lambda Runtime-API loop (bootstrap), overlay-image builder, reverse-agent callback registry. Unit-tested without AWS |
| P86-019 | blocked-on-bugs | Runbook 3b — live validation of Lambda `docker exec` via agent-as-handler. Requires ngrok (or equivalent public callback URL) |
| P86-014 | partial | Final state save after session 2 closes. Residue audit, capability matrix live-column, STATUS/WHAT_WE_DID/MEMORY updates |

---

## Future Ideas

- GraphQL subscriptions for real-time event streaming
- Full GitHub App permission scoping
- Webhook delivery UI
- Cost controls (per-pool spending limits, auto-shutdown)
