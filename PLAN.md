# Sockerless — Roadmap

> Phases 1-67, 69 complete (578 tasks). This document covers future work.
>
> **Production target:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners (GitHub Actions from github.com, GitLab CI from gitlab.com), and custom SDK clients — backed by real cloud infrastructure (AWS, GCP, Azure).

## Guiding Principles

1. **Docker API fidelity** — The frontend must match Docker's REST API exactly. CI runners should not need patching.
2. **Real execution** — Simulators and backends must actually run commands and produce real output. Synthetic/echo mode is a last resort.
3. **External validation** — Correctness is proven by running unmodified external test suites (`act`, `gitlab-runner`, `gitlab-ci-local`, `gh` CLI, cloud SDKs/CLIs/Terraform).
4. **No new frontend abstractions** — The Docker REST API is the only interface. No Kubernetes, no Podman, no custom APIs.
5. **Driver-first handlers** — All handler code must operate through driver interfaces, never through direct `Store.Processes` access or `ProcessFactory` checks.
6. **LLM-editable files** — Keep source files under 400 lines.
7. **GitHub API fidelity** — bleephub must match the real GitHub API closely enough that unmodified `gh` CLI commands work against it.
8. **State persistence** — Every task must end with a state save: update `PLAN.md` (mark task done), `STATUS.md` (test counts), `WHAT_WE_DID.md` (append summary), `MEMORY.md` (add learnings), and `_tasks/done/` (completion log).

---

## Completed Phases (1-63)

Technical decisions from all phases are recorded in `DECISIONS.md`. Detailed per-task logs in `_tasks/done/`.

| Phase | Summary |
|---|---|
| 1-10 | Foundation: 3 cloud simulators (AWS/GCP/Azure), 8 backends, agent bridge, Docker REST API frontend |
| 11-20 | WASM sandbox (wazero + mvdan.cc/sh + go-busybox), process execution, archive ops, bind mounts |
| 21-34 | E2E tests (GitHub 217 + GitLab 154), driver interfaces, gitlab-ci-local 252, sandbox builtins, Docker build |
| 35-42 | bleephub: GitHub API server + runner internal API + multi-job engine. 190 unit tests |
| 43-52 | Crash safety, pod API, CLI, management API, service containers, upstream test expansion |
| 53-56 | Production Docker API: TLS, auth, logs, DNS, restart, events, filters, export, commit |
| 57-59 | Production GitHub Actions: multi-job, matrix, secrets, expressions, cancellation, concurrency, artifacts |
| 60-61 | Production GitLab CI: gitlabhub coordinator, DAG engine, expressions, extends, include, parallel, retry, DinD |
| 62-63 | Docker API hardening + Compose E2E: HEALTHCHECK, volumes, mounts, prune, SHELL/STOPSIGNAL/VOLUME directives, race fixes. 249 core tests |
| 64 | Webhooks for bleephub: per-repo CRUD, HMAC-SHA256 signing, async delivery with retry, delivery log API, event payloads (push/PR/issues/ping), CI trigger via push/PR events. 270 bleephub tests |
| 65 | GitHub Apps for bleephub: App store + RSA keygen, RS256 JWT sign/verify, installation tokens (ghs_), auth middleware (JWT + ghs_ + PAT), 9 REST endpoints, manifest code flow. 293 bleephub tests |
| 66 | Optional OpenTelemetry tracing: InitTracer in 4 modules (OTLP HTTP exporter, no-op when env unset), otelhttp middleware on all 4 servers, context propagation through BackendClient (11 methods), workflow/pipeline engine spans. 8 new tests |
| 67 | Network Isolation: IPAllocator, SyntheticNetworkDriver, Linux NetnsManager, LinuxNetworkDriver wrapper, refactored handlers to driver pattern. 14 new tests, 255 core tests |
| 69 | ARM64 / Multi-Arch Completion: goreleaser 8→15 builds (added 6 cloud backends + gitlabhub), gitlabhub Dockerfile.release, docker.yml 6→7 images, CI build-check 8→15 binaries + ARM64 cross-compile job |

---

## Phase 68 — (Next)

---

## Phase 68 — Multi-Tenant Backend Pools

**Goal:** Named pools of backends with scheduling and resource limits. Each pool has a backend type, concurrency limit, and queue. Requests are routed to pools by label or explicit selection.

### P68-A: Pool Infrastructure (~6 tasks)

| Task | Description |
|---|---|
| P68-001 | **Pool configuration** — YAML/JSON config: pool name, backend type, max concurrency, queue size |
| P68-002 | **Pool registry** — in-memory registry of pools, each with its own `BaseServer` + `Store` |
| P68-003 | **Request router** — route Docker API requests to pools by label (`com.sockerless.pool`) or default pool |
| P68-004 | **Concurrency limiter** — per-pool semaphore, queue overflow returns 429 |
| P68-005 | **Pool lifecycle** — create/destroy pools at runtime via management API |
| P68-006 | **Pool metrics** — per-pool container count, queue depth, utilization exposed on `/internal/metrics` |

### P68-B: Scheduling + Tests (~4 tasks)

| Task | Description |
|---|---|
| P68-007 | **Round-robin scheduling** — distribute requests across pool instances when pool has multiple backends |
| P68-008 | **Resource limits** — per-pool max containers, max total memory (advisory, enforced at scheduling time) |
| P68-009 | **Unit + integration tests** — pool CRUD, routing, concurrency limits, queue overflow, metrics |
| P68-010 | **Save final state** |

**Verification:** Requests route to correct pool, concurrency limits enforced, queue overflow returns 429. Tests pass.

---

## Phase 69 — ARM64 / Multi-Arch Completion ✅

**Goal:** Fill remaining multi-arch gaps: missing goreleaser builds, gitlabhub Docker image, ARM64 CI verification.

| Task | Status | Description |
|---|---|---|
| P69-001 | ✅ | **Goreleaser builds** — added 7 missing builds (6 cloud backends + gitlabhub), total 15×2×2 = 60 binaries |
| P69-002 | ✅ | **Gitlabhub Dockerfile.release** — golang:1.24-alpine builder → alpine:3.20 runtime |
| P69-003 | ✅ | **Docker workflow** — added gitlabhub to docker.yml matrix (7 images total) |
| P69-004 | ✅ | **ARM64 CI job** — new `build-check-arm64` job in ci.yml, all 15 binaries |
| P69-005 | ✅ | **Expanded build-check** — existing job now builds all 15 binaries (was 8) |
| P69-006 | ✅ | **Save final state** |

---

## Future Ideas (Not Scheduled)

- **WASI Preview 2** — component model with async I/O; would enable real subprocesses in WASM sandbox
- **GraphQL subscriptions** — real-time event streaming for live PR/issue updates
- **Full GitHub App permissions** — per-installation permission scoping (read/write per resource type)
- **Webhook delivery UI** — web dashboard for inspecting webhook deliveries
- **Cost controls** — per-pool spending limits, cloud cost tracking, auto-shutdown on budget
