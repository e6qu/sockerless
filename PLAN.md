# Sockerless — Roadmap

> Phases 1-56 complete (461 tasks). This document covers future work.
>
> **Production target:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners (GitHub Actions from github.com, GitLab CI from gitlab.com), and custom SDK clients — backed by real cloud infrastructure (AWS, GCP, Azure).

## Guiding Principles

1. **Docker API fidelity** — The frontend must match Docker's REST API exactly. CI runners should not need patching.
2. **Real execution** — Simulators and backends must actually run commands and produce real output. Synthetic/echo mode is a last resort.
3. **External validation** — Correctness is proven by running unmodified external test suites (`act`, `gitlab-runner`, `gitlab-ci-local`, `gh` CLI, cloud SDKs/CLIs/Terraform).
4. **No new frontend abstractions** — The Docker REST API is the only interface. No Kubernetes, no Podman, no custom APIs.
5. **Driver-first handlers** — All handler code must operate through driver interfaces, never through direct `Store.Processes` access or `ProcessFactory` checks. If a handler needs an operation the driver doesn't expose, extend the interface.
6. **LLM-editable files** — Keep source files under 400 lines. Split by responsibility, not just by resource type. Each file should be fully comprehensible in a short context window without reading other files.
7. **GitHub API fidelity** — bleephub must match the real GitHub API closely enough that unmodified `gh` CLI commands work against it. Validated against GitHub's OpenAPI spec and the official CLI.
8. **State persistence** — Every task must end with a state save: update `PLAN.md` (mark task done), `STATUS.md` (test counts), `WHAT_WE_DID.md` (append summary), `MEMORY.md` (add learnings), and `_tasks/done/` (completion log). This ensures crash-recoverable progress tracking.

---

## Completed Phases (1-56)

Technical decisions from all phases are recorded in `DECISIONS.md`. Detailed per-task logs in `_tasks/done/`.

| Phase | Summary |
|---|---|
| 1-10 | Foundation: 3 cloud simulators (AWS/GCP/Azure), 8 backends, agent bridge, Docker REST API frontend |
| 11-20 | WASM sandbox (wazero + mvdan.cc/sh + go-busybox), process execution, archive ops, bind mounts |
| 21-30 | E2E test suites (GitHub 217 + GitLab 154), driver interfaces (DriverSet), gitlab-ci-local 252 |
| 31 | Enhanced WASM sandbox: 12 new builtins, pwd fix. 46 sandbox tests |
| 32 | Driver interface completion (6 new methods, 0 bypasses) + code splitting (all files <400 lines) |
| 33 | Service containers: health check infra, NetworkingConfig, port reporting |
| 34 | Docker build: Dockerfile parser (FROM/COPY/ENV/CMD/WORKDIR/ARG/LABEL/EXPOSE), build context injection |
| 35-42 | bleephub: GitHub API server — runner internal API, users/auth/GraphQL, git repos, orgs/teams/RBAC, issues/PRs, API conformance, `gh` CLI test, multi-job engine, matrix, artifacts/cache. 190 unit tests |
| 43-44 | Crash safety: unified cloud tagging (5 tags, 3 formats), resource registry, auto-save, atomic writes, startup recovery |
| 45-46 | Pod API: PodContext/PodRegistry, `/libpod/pods/*`, implicit grouping, deferred start, ECS/CR/ACA multi-container |
| 47 | sockerless CLI + context management (`~/.sockerless/contexts/`) |
| 48-50 | Private management API (healthz/status/metrics/check/reload), server lifecycle, CLI server control |
| 51 | CI service containers: bleephub `services:` parsing, health wait, E2E workflows |
| 52 | Upstream test expansion: gitlab-ci-local 36, GitHub E2E 31, GitLab E2E 22 |
| 53 | Production Docker API: TLS, Docker auth config, registry credential chain, tmpfs |
| 54 | Production Docker API: log filter/follow, ExtraHosts, peer DNS, restart policy |
| 55 | Production Docker API: EventBus, network disconnect, container update, volume filters |
| 56 | Docker API polish: list limit/sort, ancestor/network/health/before/since filters, export, commit, push stub, flushingCopy |

---

## Phase 57 — Production GitHub Actions: Multi-Job, Scaling, & Validation

**Goal:** Multi-job workflows, concurrent execution, cross-backend validation.

| Task | Description |
|---|---|
| P57-001 | **Multi-job workflows.** `needs:` dependencies, matrix, `outputs:`. Save state |
| P57-002 | **Concurrency and queueing.** Multiple runners, resource limits. Save state |
| P57-003 | **Logging and observability.** Unified structured logging. Save state |
| P57-004 | **Validation matrix.** Real-world workflows across all backends. Save state |
| P57-005 | **Save final state** |

**Verification:** Public GitHub repo CI workflows pass through self-hosted runners on ECS identically to GitHub-hosted runners.

---

## Phase 58 — Production GitLab CI: Runner Setup & Single-Job Pipelines

**Goal:** GitLab Runner (docker executor) on real cloud infrastructure via Sockerless.

| Task | Description |
|---|---|
| P58-001 | **Runner registration automation.** Save state |
| P58-002 | **Helper image compatibility.** Save state |
| P58-003 | **Git clone via helper.** Save state |
| P58-004 | **Artifact upload/download.** Save state |
| P58-005 | **Cache support.** Save state |
| P58-006 | **Service containers.** Save state |
| P58-007 | **Secrets and variables.** Save state |
| P58-008 | **Save final state** |

**Verification:** Single-job GitLab CI pipeline with git clone, build, test, artifacts, cache, services, secrets runs on at least 3 cloud backends.

---

## Phase 59 — Production GitLab CI: Advanced Pipelines, Scaling, & Validation

**Goal:** Multi-stage pipelines, DinD, autoscaling, cross-backend validation.

| Task | Description |
|---|---|
| P59-001 | **Multi-stage pipelines.** Save state |
| P59-002 | **Docker-in-Docker (DinD).** Save state |
| P59-003 | **Runner autoscaling.** Save state |
| P59-004 | **Logging and observability.** Save state |
| P59-005 | **Validation matrix.** Save state |
| P59-006 | **Save final state** |

**Verification:** gitlab.com project CI pipelines pass through self-hosted runner on Cloud Run identically to shared runners.

---

## Phase 60 — Docker API Hardening for Production

**Goal:** Fix Docker API gaps and edge cases discovered during production validation (Phases 57-59).

| Task | Description |
|---|---|
| P60-001 | **Docker build production paths.** Multi-stage, build args, .dockerignore, BuildKit. Save state |
| P60-002 | **Volume lifecycle.** Named volumes, bind mounts, tmpfs. Save state |
| P60-003 | **Network fidelity.** DNS resolution, aliases, exposed ports. Save state |
| P60-004 | **Container restart and retry.** Restart policies, OOM, cloud failures. Save state |
| P60-005 | **Streaming fidelity.** `docker logs -f`, attach, interactive exec. Save state |
| P60-006 | **Large file handling.** docker cp, image layers, log streams. Save state |
| P60-007 | **Concurrent operations.** Parallel containers, race/deadlock safety. Save state |
| P60-008 | **Error propagation.** Cloud API errors → Docker API errors. Save state |
| P60-009 | **Save final state** |

**Verification:** All production validation matrices pass at 100%.

---

## Phase 61 — Production Hardening and Operations

**Goal:** Make Sockerless production-ready for continuous operation: monitoring, alerting, upgrade procedures, security, and documentation.

| Task | Description |
|---|---|
| P61-001 | **Health checks and readiness probes.** Save state |
| P61-002 | **Metrics and monitoring.** Prometheus + Grafana. Save state |
| P61-003 | **Alerting.** Save state |
| P61-004 | **Security audit.** Save state |
| P61-005 | **TLS everywhere.** Save state |
| P61-006 | **Upgrade procedures.** Save state |
| P61-007 | **Cost controls.** Save state |
| P61-008 | **Operations guide.** Save state |
| P61-009 | **Save final state** |

**Verification:** Sockerless runs continuously for 7 days under mixed load. No leaked resources, no crashes. Monitoring and alerts work correctly.

---

## Future Ideas (Not Scheduled)

- **WASI Preview 2** — component model with async I/O; would enable real subprocesses in WASM sandbox
- **Real network isolation** — network namespaces on Linux for true container networking
- **OpenTelemetry tracing** — distributed tracing across frontend → backend → simulator → agent
- **Multi-tenant mode** — backend pools with scheduling and resource limits
- **ARM/multi-arch support** — WASM is arch-independent; agent and simulators may need cross-compilation
- **Webhooks** — bleephub sends webhook events on push, PR, issue changes (enables CI trigger testing)
- **GitHub Apps** — app installation, JWT auth, installation tokens (enables testing GitHub App-based workflows)
- **GraphQL subscriptions** — real-time event streaming for live PR/issue updates
