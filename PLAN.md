# Sockerless — Roadmap

> Phases 1-67, 69-72 complete (637 tasks). Phase 68 in progress, Phase 72 complete. This document covers current and future work.
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
| 70 | Simulator Fidelity: real process execution, structured logs, correct status enums, SDK/CLI/Terraform compat |
| 71 | SDK/CLI Verification & Documentation: FaaS real execution, CLI execution+log tests, README quick-starts |
| 72A | Full-Stack E2E Tests: forward-agent arithmetic integration tests (ECS/CloudRun/ACA), fast-exit fix |
| 72B | FaaS real execution via Docker API: Lambda/GCF/AZF invoke with container Cmd, X-Sim-Command header |
| 72C | FaaS arithmetic E2E tests (Lambda/GCF/AZF), ECS test name collision fix |
| 72D | Central arithmetic E2E tests (shell arithmetic, exec-in-container) |

---

## Phase 71 — SDK/CLI Verification & Documentation (Complete)

**Goal:** Close three gaps: (1) FaaS services lack real execution, (2) CLI tests don't verify execution or logs, (3) READMEs lack usage examples.

### Milestone A: FaaS Real Execution (P71-001 → P71-006)

| Task | Status | Description |
|---|---|---|
| P71-001 | ✅ | Lambda real execution — `invokeLambdaProcess` via `sim.StartProcess()`, `lambdaLogSink` to CloudWatch |
| P71-002 | ✅ | Lambda execution SDK tests — `InvokeExecutesCommand`, `InvokeNonZeroExit`, `InvokeLogsToCloudWatch` |
| P71-003 | ✅ | Cloud Functions real execution — `SimCommand` on `ServiceConfig`, `cfLogSink` to Cloud Logging |
| P71-004 | ✅ | Cloud Functions execution SDK tests — `InvokeExecutesCommand`, `InvokeNonZeroExit`, `InvokeLogsRealOutput` |
| P71-005 | ✅ | Azure Functions real execution — `SimCommand` on `SiteConfig`, `funcLogSink` to AppTraces |
| P71-006 | ✅ | Azure Functions execution SDK tests — `InvokeExecutesCommand`, `InvokeNonZeroExit`, `InvokeLogsRealOutput` |

### Milestone B: CLI Execution & Log Verification (P71-007 → P71-012)

| Task | Status | Description |
|---|---|---|
| P71-007 | ✅ | AWS CLI — ECS `RunTaskAndCheckLogs`, `RunTaskNonZeroExit` |
| P71-008 | ✅ | AWS CLI — Lambda `InvokeAndCheckLogs` |
| P71-009 | ✅ | GCP CLI — Cloud Run `RunJobAndCheckLogs`, `RunJobFailure` |
| P71-010 | ✅ | GCP CLI — Cloud Functions `InvokeAndCheckLogs` |
| P71-011 | ✅ | Azure CLI — Container Apps `StartAndCheckLogs`, `StartFailure` |
| P71-012 | ✅ | Azure CLI — Functions `InvokeAndCheckLogs` |

### Milestone C: README Documentation (P71-013 → P71-015)

| Task | Status | Description |
|---|---|---|
| P71-013 | ✅ | AWS README quick-start — ECS, Lambda, CloudWatch, ECR, S3 |
| P71-014 | ✅ | GCP README quick-start — Cloud Run Jobs, Cloud Functions, Cloud Logging, AR, GCS |
| P71-015 | ✅ | Azure README quick-start — Container Apps Jobs, Azure Functions, Log Analytics, ACR, Storage |

### Milestone D: Non-Trivial Arithmetic Evaluator Tests (P71-016 → P71-019)

| Task | Status | Description |
|---|---|---|
| P71-016 | ✅ | Arithmetic evaluator program — recursive-descent parser in `simulators/testdata/eval-arithmetic/` |
| P71-017 | ✅ | SDK arithmetic tests — 7 per cloud (4 FaaS + 3 container), 21 total |
| P71-018 | ✅ | CLI arithmetic tests — 2 per cloud (container service), 6 total |
| P71-019 | ✅ | Cross-cloud verification + state save |

---

## Phase 70 — Simulator Fidelity (Complete)

**Goal:** Bring all three cloud simulators to production quality — real process execution, structured log queries, correct status enums, and full SDK/CLI/Terraform compatibility.

### Milestone 1: AWS Fidelity (P70-001 → P70-006)

| Task | Status | Description |
|---|---|---|
| P70-001 | ✅ | Lambda log stream auto-creation |
| P70-002 | ✅ | CloudWatch GetLogEvents pagination tokens |
| P70-003 | ✅ | DescribeTasks nil ExitCode handling |
| P70-004 | ✅ | ECS StopCode field |
| P70-005 | ✅ | Lambda DescribeLogStreams ordering |
| P70-006 | ✅ | AWS smoke test — ECS + Lambda integration |

### Milestone 2: GCP Fidelity (P70-007 → P70-012)

| Task | Status | Description |
|---|---|---|
| P70-007 | ✅ | Cloud Logging structured filter parser |
| P70-008 | ✅ | Cloud Run log entry injection |
| P70-009 | ✅ | Cloud Functions log entry injection |
| P70-010 | ✅ | Cloud Functions invoke URL fidelity |
| P70-011 | ✅ | Execution status field completeness |
| P70-012 | ✅ | GCP smoke test — Cloud Run + Cloud Functions |

### Milestone 3: Azure Fidelity (P70-013 → P70-018)

| Task | Status | Description |
|---|---|---|
| P70-013 | ✅ | KQL query parser for backend patterns |
| P70-014 | ✅ | Container Apps log injection |
| P70-015 | ✅ | Functions log injection |
| P70-016 | ✅ | Execution status enum values |
| P70-017 | ✅ | Function App DefaultHostName reachability |
| P70-018 | ✅ | Azure smoke test — ACA + Azure Functions |

### Milestone 4: Cross-Cloud (P70-019 → P70-023)

| Task | Status | Description |
|---|---|---|
| P70-019 | ✅ | Configurable execution timeout — replace hardcoded 3s with cloud-native timeouts |
| P70-020 | ✅ | Shared ProcessRunner engine — `StartProcess()` in shared simulator library |
| P70-021 | ✅ | AWS ECS real execution — RunTask executes container command, real exit codes + CloudWatch logs |
| P70-022 | ✅ | GCP Cloud Run real execution — executions run container command, real exit codes + Cloud Logging |
| P70-023 | ✅ | Azure ACA real execution — executions run container command, real exit codes + Log Analytics |
| P70-024 | ✅ | CI integration for simulator smoke tests |

---

## Phase 72 — Full-Stack E2E Tests (Complete)

**Goal:** Real arithmetic execution through full Docker API stack (Frontend → Backend → Simulator).

### Milestone A: Forward-Agent Backend E2E Tests (P72-001 → P72-004)

| Task | Status | Description |
|---|---|---|
| P72-001 | ✅ | ECS arithmetic integration tests — 6 tests via Docker API |
| P72-002 | ✅ | CloudRun arithmetic integration tests — 6 tests, gRPC logadmin fix |
| P72-003 | ✅ | ACA arithmetic integration tests — 6 tests, soft log assertions |
| P72-004 | ✅ | Forward-agent regression — 147 PASS (was 129), fast-exit fix for CloudRun/ACA |

### Milestone B: FaaS Backend Real Execution (P72-005 → P72-008)

| Task | Status | Description |
|---|---|---|
| P72-005 | ✅ | Lambda real execution via Docker API — invoke function, use FunctionError for exit code |
| P72-006 | ✅ | GCF real execution via Docker API — X-Sim-Command header, base64-encoded JSON |
| P72-007 | ✅ | AZF real execution via Docker API — same X-Sim-Command pattern as GCF |
| P72-008 | ✅ | FaaS helper container regression — all 147 sim-test-all PASS, SDK tests PASS |

### Milestone C: FaaS Backend E2E Tests (P72-009 → P72-012)

| Task | Status | Description |
|---|---|---|
| P72-009 | ✅ | Lambda arithmetic integration tests — 6 tests |
| P72-010 | ✅ | GCF arithmetic integration tests — 6 tests, soft log assertions (gRPC Cloud Logging) |
| P72-011 | ✅ | AZF arithmetic integration tests — 6 tests, soft log assertions (Azure Monitor TLS) |
| P72-012 | ✅ | Full FaaS regression — 75 sim-test-all PASS, ECS hardcoded name fix |

### Milestone D: Central Multi-Backend E2E Tests (P72-013 → P72-015)

| Task | Status | Description |
|---|---|---|
| P72-013 | ✅ | Central arithmetic E2E tests — 4 tests (execution, non-zero exit, exec-in-container, eval binary) |
| P72-014 | ✅ | Verified with memory backend — 65 test-e2e PASS (was 61) |
| P72-015 | ✅ | Final state save |

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

## Future Ideas (Not Scheduled)

- **WASI Preview 2** — component model with async I/O; would enable real subprocesses in WASM sandbox
- **GraphQL subscriptions** — real-time event streaming for live PR/issue updates
- **Full GitHub App permissions** — per-installation permission scoping (read/write per resource type)
- **Webhook delivery UI** — web dashboard for inspecting webhook deliveries
- **Cost controls** — per-pool spending limits, cloud cost tracking, auto-shutdown on budget
