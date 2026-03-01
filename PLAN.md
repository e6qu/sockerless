# Sockerless — Roadmap

> Phases 1-67, 69-74 complete (664 tasks). Phase 68 in progress. This document covers current and future work.
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

## Completed Phases (1-72)

Technical decisions from all phases are recorded in `DECISIONS.md`. Detailed per-task logs in `_tasks/done/`.

| Phase | Summary |
|---|---|
| 1-10 | Foundation: 3 cloud simulators (AWS/GCP/Azure), 8 backends, agent bridge, Docker REST API frontend |
| 11-34 | WASM sandbox, E2E tests (217+154), driver interfaces, gitlab-ci-local 252, Docker build |
| 35-42 | bleephub: GitHub API server + runner internal API + multi-job engine. 190 unit tests |
| 43-56 | CLI, crash safety, pods, service containers, production Docker API (TLS/auth/logs/DNS/restart/events/filters/export/commit) |
| 57-59 | Production GitHub Actions: multi-job, matrix, secrets, expressions, cancellation, concurrency, artifacts |
| 60-61 | Production GitLab CI: gitlabhub coordinator, DAG engine, expressions, extends, include, parallel, retry, DinD |
| 62-63 | Docker API hardening + Compose E2E: HEALTHCHECK, volumes, mounts, prune, directives, race fixes. 249→255 core tests |
| 64-65 | bleephub: Webhooks (HMAC-SHA256, async delivery, CI trigger) + GitHub Apps (JWT, installation tokens). 293 tests |
| 66 | Optional OpenTelemetry tracing: OTLP HTTP, otelhttp middleware, context propagation, workflow/pipeline spans |
| 67 | Network Isolation: IPAllocator, SyntheticNetworkDriver, Linux NetnsManager, 14 new tests |
| 69 | ARM64/Multi-Arch: goreleaser 15 builds, gitlabhub Dockerfile, docker.yml 7 images, ARM64 CI |
| 70 | Simulator Fidelity: real process execution, structured logs, SDK/CLI/Terraform compat. 24 tasks |
| 71 | SDK/CLI Verification: FaaS real execution, CLI log tests, README quick-starts, arithmetic evaluator. 19 tasks |
| 72 | Full-Stack E2E Tests: forward-agent + FaaS arithmetic through Docker API, central multi-backend tests. 15 tasks |
| 73 | UI Foundation: Bun/Vite/React 19/Tailwind 4 monorepo, shared core, SPAHandler, memory backend dashboard. 15 tasks |
| 74 | All Backend Dashboards: shared BackendApp, 9 new SPAs (6 cloud + docker backend + docker frontend), mgmt endpoints. 12 tasks |

---

## Phase 68 — Multi-Tenant Backend Pools (In Progress)

**Goal:** Named pools of backends with scheduling and resource limits. Each pool has a backend type, concurrency limit, and queue. Requests are routed to pools by label or explicit selection.

### P68-A: Pool Infrastructure (~6 tasks)

| Task | Status | Description |
|---|---|---|
| P68-001 | ✅ | **Pool configuration** — JSON config: pool name, backend type, max concurrency, queue size |
| P68-002 | | **Pool registry** — in-memory registry of pools, each with its own `BaseServer` + `Store` |
| P68-003 | | **Request router** — route Docker API requests to pools by label (`com.sockerless.pool`) or default pool |
| P68-004 | | **Concurrency limiter** — per-pool semaphore, queue overflow returns 429 |
| P68-005 | | **Pool lifecycle** — create/destroy pools at runtime via management API |
| P68-006 | | **Pool metrics** — per-pool container count, queue depth, utilization exposed on `/internal/metrics` |

### P68-B: Scheduling + Tests (~4 tasks)

| Task | Status | Description |
|---|---|---|
| P68-007 | | **Round-robin scheduling** — distribute requests across pool instances when pool has multiple backends |
| P68-008 | | **Resource limits** — per-pool max containers, max total memory (advisory, enforced at scheduling time) |
| P68-009 | | **Unit + integration tests** — pool CRUD, routing, concurrency limits, queue overflow, metrics |
| P68-010 | | **Save final state** |

---

## Phase 74 — All Backend Dashboards + Docker Frontend

**Goal:** Roll out dashboard to all 7 remaining backends + Docker frontend. Each shares `core.BaseServer` and the same management API.

| Task | Status | Description |
|---|---|---|
| P74-001 | ✅ | **Extract shared pages + BackendApp** — Moved 4 pages to `@sockerless/ui-core/pages`, created `BackendApp` component |
| P74-002 | ✅ | **BackendInfoCard component** — Backend-type-specific details from `/internal/v1/status`, 2 tests |
| P74-003 | ✅ | **ECS backend SPA + embed** |
| P74-004 | ✅ | **Lambda backend SPA + embed** |
| P74-005 | ✅ | **CloudRun backend SPA + embed** |
| P74-006 | ✅ | **GCF backend SPA + embed** |
| P74-007 | ✅ | **ACA backend SPA + embed** |
| P74-008 | ✅ | **AZF backend SPA + embed** |
| P74-009 | ✅ | **Docker backend SPA + embed** — Management endpoints + SPA on Docker backend (non-BaseServer) |
| P74-010 | ✅ | **Docker frontend mgmt SPA** — Custom SPA on MgmtServer with `/healthz`, `/status`, `/metrics` |
| P74-011 | ✅ | **Build integration + tests + CI** — Makefile expanded (18 new targets), CI updated with `-tags noui` |
| P74-012 | ✅ | **Verification + state save** — 10 SPAs build, 16 Vitest tests pass, 9 Go noui builds pass |

---

## Phase 75 — Simulator Dashboards (AWS, GCP, Azure)

**Goal:** Add dashboards to cloud simulators showing simulated resources. Browser calls simulator's own cloud APIs (same-origin).

| Task | Status | Description |
|---|---|---|
| P75-001 | | **Simulator SPA handler** — Add `SPAHandler()` to simulator shared server, register at `/ui/` |
| P75-002 | | **Simulator core components** — `SimulatorShell.tsx`, cloud resource types, cloud API client functions |
| P75-003 | | **AWS SPA** — Pages: ECS tasks, Lambda functions, ECR repos, S3 buckets, CloudWatch logs |
| P75-004 | | **AWS API client** — Wrappers calling simulator AWS APIs |
| P75-005 | | **AWS embed** — `simulators/aws/ui_embed.go`, register in `main.go` |
| P75-006 | | **GCP SPA** — Pages: Cloud Run Jobs, Functions, Logging, AR, GCS |
| P75-007 | | **GCP API client** — Wrappers for GCP REST APIs |
| P75-008 | | **GCP embed** — `simulators/gcp/ui_embed.go` |
| P75-009 | | **Azure SPA** — Pages: Container Apps, Functions, ACR, Files, Monitor |
| P75-010 | | **Azure API client** — Wrappers for Azure ARM REST APIs |
| P75-011 | | **Azure embed** — `simulators/azure/ui_embed.go` |
| P75-012 | | **Tests** — Per-simulator resource list renders with mock data. 12+ tests |
| P75-013 | | **State save** |

---

## Phase 76 — bleephub Dashboard (GitHub Actions)

**Goal:** Dashboard for the GitHub Actions coordinator — workflows, jobs, runners, logs.

| Task | Status | Description |
|---|---|---|
| P76-001 | | **bleephub API types** — TS types for Workflow, Job, Session, Repo, Webhook |
| P76-002 | | **New management endpoints** — `/internal/workflows`, `/internal/sessions`, `/internal/repos` |
| P76-003 | | **bleephub API client** — Wrappers for management + GitHub API endpoints |
| P76-004 | | **bleephub SPA** — Pages: Overview, Workflows, Jobs, Runners |
| P76-005 | | **Workflow list + detail pages** — Status badges, job timeline, step-by-step view |
| P76-006 | | **LogViewer component** — Shared `LogViewer.tsx` in core with ANSI color support |
| P76-007 | | **Runner session view** — Connected agents, message queue status |
| P76-008 | | **Metrics dashboard** — Workflow submissions/min, completion rates, goroutines, heap |
| P76-009 | | **bleephub embed** — `bleephub/ui_embed.go` + serve at `/ui/` before catch-all |
| P76-010 | | **Tests** — Workflow list, job timeline, log viewer. 10+ tests |
| P76-011 | | **State save** |

---

## Phase 77 — gitlabhub Dashboard (GitLab CI)

**Goal:** Dashboard for the GitLab CI coordinator — pipelines, stages, jobs, runners.

| Task | Status | Description |
|---|---|---|
| P77-001 | | **gitlabhub API types** — TS types for Pipeline, Job, Runner, Stage |
| P77-002 | | **New management endpoints** — `/internal/pipelines`, `/internal/runners` |
| P77-003 | | **gitlabhub API client** — Wrappers for management endpoints |
| P77-004 | | **gitlabhub SPA** — Pages: Overview, Pipelines, Jobs, Runners |
| P77-005 | | **Pipeline list + stage view** — Stages as columns, jobs as rows, DAG dependency lines |
| P77-006 | | **Job log viewer** — Reuse `LogViewer.tsx`, trace reconstruction from incremental uploads |
| P77-007 | | **Runner management view** — Registered runners, status, job history |
| P77-008 | | **gitlabhub embed** — `gitlabhub/ui_embed.go` + modify `server.go` |
| P77-009 | | **Tests** — Pipeline list, stage view, log viewer. 8+ tests |
| P77-010 | | **State save** |

---

## Phase 78 — Polish, Dark Mode, Cross-Component UX

**Goal:** Cross-cutting UI polish: dark mode, error handling, accessibility, performance, documentation.

| Task | Status | Description |
|---|---|---|
| P78-001 | | **Dark mode** — Tailwind class-based strategy, localStorage preference, toggle in header |
| P78-002 | | **Design system tokens** — Shared color palette, spacing, typography, per-component theme variants |
| P78-003 | | **Error handling UX** — Connection lost indicator, retry buttons, stale data warnings |
| P78-004 | | **Container detail modal** — Click container → inspect data, streaming logs, actions |
| P78-005 | | **Auto-refresh controls** — Global toggle, configurable interval, Page Visibility API pause |
| P78-006 | | **Performance audit** — Bundle size < 200KB gzipped per SPA, code splitting, build time < 30s |
| P78-007 | | **Accessibility** — Keyboard nav, ARIA labels, color contrast (light + dark) |
| P78-008 | | **E2E smoke test** — Go test: start memory backend, fetch `/ui/`, verify React root + API coexistence |
| P78-009 | | **Documentation** — `ui/README.md`: dev setup, architecture, component catalog. Update root README |
| P78-010 | | **Final state save** |

---

## Future Ideas (Not Scheduled)

- **WASI Preview 2** — component model with async I/O; would enable real subprocesses in WASM sandbox
- **GraphQL subscriptions** — real-time event streaming for live PR/issue updates
- **Full GitHub App permissions** — per-installation permission scoping (read/write per resource type)
- **Webhook delivery UI** — web dashboard for inspecting webhook deliveries
- **Cost controls** — per-pool spending limits, cloud cost tracking, auto-shutdown on budget
