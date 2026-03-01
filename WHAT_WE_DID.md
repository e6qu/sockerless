# Sockerless — What We Built

## The Idea

Sockerless presents an HTTP REST API identical to Docker's. CI runners (GitHub Actions via `act`, GitLab Runner, `gitlab-ci-local`) talk to it as if it were Docker, but instead of running containers locally, Sockerless farms work to cloud backends — ECS, Lambda, Cloud Run, Cloud Functions, Azure Container Apps, Azure Functions — or runs everything in-process via a WASM sandbox (the "memory" backend).

For development and testing, cloud simulators stand in for real AWS/GCP/Azure APIs, providing actual execution of tasks the same way a real cloud would. The simulators are validated against official cloud SDKs, CLIs, and Terraform providers.

## Architecture

```
CI Runner (act, gitlab-runner, gitlab-ci-local)
    │
    ▼
Frontend (Docker REST API)
    │
    ▼
Backend (ecs | lambda | cloudrun | gcf | aca | azf | memory | docker)
    │                                                    │
    ▼                                                    ▼
Cloud Simulator (AWS | GCP | Azure)              WASM Sandbox
    │                                           (wazero + mvdan.cc/sh
    ▼                                            + go-busybox)
Agent (inside container or reverse-connected)
```

**8 backends** share a common core (`backends/core/`) with driver interfaces:
- **ExecDriver** — runs commands (WASM shell, forward agent, reverse agent, or synthetic echo)
- **FilesystemDriver** — manages container filesystem (temp dirs, agent bridge, staging)
- **StreamDriver** — attach/logs streaming (pipes, WebSocket relay, log buffer)
- **ProcessLifecycleDriver** — start/stop/kill/cleanup

Each driver chains: Agent → Process → Synthetic, so every handler call falls through to the right implementation.

**3 simulators** (`simulators/{aws,gcp,azure}/`) implement enough cloud API surface for the backends to work. Each is tested against the real SDK, CLI, and Terraform provider for that cloud.

## Completed Phases (1-56) — Summary

| Phase | What | Key Artifacts |
|---|---|---|
| 1-10 | Foundation: simulators (AWS/GCP/Azure), backends, agent, frontend | 3 simulators, 8 backends, Docker REST API frontend |
| 11-34 | WASM sandbox, E2E tests, driver interfaces, Docker build | 217 GitHub + 154 GitLab E2E, 46 sandbox tests |
| 35-42 | bleephub: GitHub API + runner + multi-job engine | 190 unit tests, users, auth, git, orgs, issues, PRs, `gh` CLI |
| 43-52 | CLI, crash safety, pods, service containers, upstream expansion | sockerless CLI, PodContext, resource registry |
| 53-56 | Production Docker API: TLS, auth, logs, DNS, restart, events, filters, export, commit | 16+18+15+14 new tests |

## Phase 57 — Production GitHub Actions: Multi-Job, Scaling, & Validation

Made bleephub's multi-job workflow engine production-ready: output capture, continue-on-error, max-parallel, job timeout, round-robin distribution, pending message queue, concurrent workflow limits, metrics collector, management API, structured logging.

**Tests**: 221 unit tests (+25), 6 integration scenarios (+4)

## Phase 58 — CI Pipeline & Publishing

Established CI/CD: core CI workflow (push/PR), comprehensive test workflow (post-merge), production Dockerfiles, Docker image workflow (6 images on GHCR), version injection, goreleaser (8 binaries), release workflow (v* tags).

## Phase 59 — Production GitHub Actions Runner

Closed critical production gaps: secrets store & API, secrets injection & masking, expression evaluator, job-level `if:` evaluation, matrix fail-fast, workflow cancellation API, concurrency control, event context, persistent artifacts.

**Tests**: 259 unit tests (+38), 9 integration scenarios (+3)

## Phase 60 — Production GitLab CI: gitlabhub

Built **gitlabhub** — GitLab Runner coordinator API server. Runner registration, pipeline YAML parser, job request endpoint (30s long-poll), stage-ordered + DAG engine, job lifecycle, 30+ CI variables, in-memory git repos, artifacts, cache, services, secrets, pipeline management API, metrics.

**Tests**: 62 unit tests, 9 integration scenarios

## Phase 61 — Advanced GitLab CI Pipelines

Expression evaluator (`rules:if:`), `extends:` (deep-merge), `include:local:`, `parallel:matrix:`, timeout/retry, dotenv artifacts, pipeline cancellation, resource groups, DinD support.

**Tests**: 129 unit tests (+67), 17 integration scenarios (+8)

## Phase 62 — Docker API Hardening

HEALTHCHECK parsing, volume auto-creation, container mounts population, network cleanup on remove, restart delay (exponential backoff), error consistency, TTY log mode, atomic prune, compose compat.

**Tests**: 230 core PASS (+53)

## Phase 63 — Docker Compose E2E

Health check race fix (deep-copy `HealthState`), SHELL/STOPSIGNAL/VOLUME directives, image prune.

**Tests**: 249 core PASS (+19)

## Phase 64 — Webhooks for bleephub

Per-repo CRUD, HMAC-SHA256 signing, async delivery with 3-retry backoff, delivery log API, event payloads (push/PR/issues/ping), CI trigger via push/PR events.

**Tests**: 270 bleephub PASS (+11)

## Phase 65 — GitHub Apps for bleephub

App store + RSA keygen, RS256 JWT sign/verify, installation tokens (`ghs_`), auth middleware (JWT + ghs_ + PAT), 9 REST endpoints, manifest code flow.

**Tests**: 293 bleephub PASS (+23)

## Phase 66 — Optional OpenTelemetry Tracing

InitTracer in 4 modules (OTLP HTTP exporter, no-op when env unset), otelhttp middleware on all 4 servers, context propagation through BackendClient, workflow/pipeline engine spans.

**Tests**: 241 core + 298 bleephub + 129 gitlabhub + 7 frontend PASS

## Phase 67 — Network Isolation (Linux)

IPAllocator, SyntheticNetworkDriver (8 methods), Linux NetnsManager (build-tagged), LinuxNetworkDriver wrapper, handler refactoring to driver pattern.

**Tests**: 255 core PASS (+14)

## Phase 69 — ARM64 / Multi-Arch Completion

Goreleaser 8→15 builds (added 6 cloud backends + gitlabhub), gitlabhub Dockerfile.release, docker.yml 6→7 images, CI build-check 8→15 binaries + ARM64 cross-compile job.

## Phase 70 — Simulator Fidelity (Complete)

### Milestones 1-3: Cloud-Specific Fidelity (P70-001 → P70-018)

Brought all three simulators to production quality:
- **AWS** (P70-001→006): Lambda log stream auto-creation, CloudWatch pagination, nil ExitCode handling, StopCode field, DescribeLogStreams ordering, integration smoke test
- **GCP** (P70-007→012): Cloud Logging structured filter parser, Cloud Run/Functions log injection, invoke URL fidelity, execution status completeness, integration smoke test
- **Azure** (P70-013→018): KQL query parser, Container Apps/Functions log injection, execution status enums, DefaultHostName reachability, integration smoke test

### Milestone 4: Cross-Cloud Real Execution (P70-019 → P70-023)

#### P70-019: Configurable Execution Timeout ✅
Replaced hardcoded synthetic timeouts with cloud-native execution timeouts. Each simulator now reads the timeout from the container/job spec (ECS has no timeout, GCP uses `TaskTemplate.Timeout`, Azure uses `ReplicaTimeout`). Fixed a `durationpb.New()` bug that passed 14400 nanoseconds instead of 4 hours.

#### P70-020: Shared ProcessRunner Engine ✅
Created `ProcessConfig`, `LogLine`, `LogSink`, `ProcessResult`, `ProcessHandle` types and `StartProcess()` function in the shared simulator library (`simulators/{aws,gcp,azure}/shared/process.go`). Non-blocking process launch with stdout/stderr streaming via `bufio.Scanner`, context-based cancellation and timeout, real exit codes via `cmd.ProcessState.ExitCode()`. Includes `NoopSink` and `FuncSink` convenience types.

**Tests**: 5 unit tests × 3 clouds = 15 PASS (captures output, exit code, timeout, cancel, env vars)

#### P70-021: AWS ECS Real Execution ✅
Wired ProcessRunner into ECS `RunTask` goroutine. Non-agent tasks with commands execute the real process, producing real stdout/stderr streamed to CloudWatch via `cwLogSink`. Process completion transitions task to STOPPED with real exit code. `StopTask` cancels running processes via `ecsProcessHandles` sync.Map. Tasks with no command stay RUNNING (matches real ECS).

**New tests**: `TestECS_TaskExecutesCommand`, `TestECS_TaskExitCodeNonZero`, `TestECS_TaskLogsToCloudWatch`, `TestECS_TaskNoCommandStaysRunning`

#### P70-022: GCP Cloud Run Real Execution ✅
Wired ProcessRunner into Cloud Run Jobs auto-complete goroutine. Replaces `time.Sleep(timeout)` with real process execution when command is present. Process exit code determines succeeded vs failed count. Cancel handler kills running processes via `crjProcessHandles` sync.Map.

**New tests**: `TestCloudRun_ExecutionRunsCommand`, `TestCloudRun_ExecutionFailedState`, `TestCloudRun_ExecutionLogsRealOutput`

#### P70-023: Azure ACA Real Execution ✅
Wired ProcessRunner into ACA Jobs auto-complete goroutine. Process exit code determines Succeeded vs Failed status. Stop handler kills running processes via `acaProcessHandles` sync.Map.

**New tests**: `TestContainerApps_ExecutionRunsCommand`, `TestContainerApps_ExecutionFailedStatus`, `TestContainerApps_ExecutionLogsRealOutput`

#### P70-024: CI Integration for Simulator Tests ✅
Added simulator tests to PR-level CI (`ci.yml`): `sim-shared` matrix entry runs 15 ProcessRunner unit tests (5 per cloud), new `simulator-sdk-tests` job runs SDK integration tests for all 3 clouds via `make sdk-test`. Added `shared-test` target to each simulator Makefile, included in `make test`.

## Phase 71 — SDK/CLI Verification & Documentation (Complete)

Closed three gaps across all three cloud simulators:

### Milestone A: FaaS Real Execution (P71-001 → P71-006)

Wired `sim.StartProcess()` into each FaaS invoke handler so functions with commands execute real processes and stream output to cloud log systems:

- **Lambda** (P71-001): Uses existing `ImageConfig.Command` field for `PackageType == "Image"`. Added `invokeLambdaProcess` + `lambdaLogSink` (CloudWatch). Injects START/END/REPORT/ERROR entries matching real Lambda format.
- **Cloud Functions** (P71-003): Added `SimCommand []string` to `ServiceConfig`. `cfLogSink` streams to Cloud Logging via `injectCloudFunctionLog`.
- **Azure Functions** (P71-005): Added `SimCommand []string` to `SiteConfig`. `funcLogSink` streams to AppTraces via `injectAppTrace`. Extracts env from AppSettings.

All three fall back to synthetic log injection when no command is present (backward compatible).

**SDK Tests** (P71-002, P71-004, P71-006): 9 new tests across 3 clouds — `InvokeExecutesCommand`, `InvokeNonZeroExit`, `InvokeLogsRealOutput` for each. Azure also added `DefaultHostNameReachability` test.

### Milestone B: CLI Execution & Log Verification (P71-007 → P71-012)

Full lifecycle CLI tests: create → execute/invoke → query cloud logs → verify output → cleanup.

- **AWS** (P71-007, P71-008): ECS `RunTaskAndCheckLogs`/`RunTaskNonZeroExit` (new `ecs_test.go`), Lambda `InvokeAndCheckLogs`
- **GCP** (P71-009, P71-010): Cloud Run `RunJobAndCheckLogs`/`RunJobFailure` (new `run_test.go`), Cloud Functions `InvokeAndCheckLogs`
- **Azure** (P71-011, P71-012): Container Apps `StartAndCheckLogs`/`StartFailure` (new `containerapps_test.go`), Functions `InvokeAndCheckLogs`

### Milestone C: README Quick-Starts (P71-013 → P71-015)

Added Quick Start sections to each simulator's README with CLI commands + expected output + Go SDK snippets for all major services (ECS, Lambda, CloudWatch, ECR, S3, Cloud Run Jobs, Cloud Functions, Cloud Logging, AR, GCS, Container Apps Jobs, Azure Functions, Log Analytics, ACR, Storage).

**Tests**: SDK: AWS 21→35, GCP 23→36, Azure 16→31 | CLI: AWS 21→24, GCP 15→19, Azure 14→17

### Milestone D: Non-Trivial Arithmetic Evaluator Tests (P71-016 → P71-019)

Built a standalone recursive-descent arithmetic expression evaluator (`simulators/testdata/eval-arithmetic/main.go`, ~180 lines) that parses and evaluates expressions with correct operator precedence, parentheses, unary minus, and division-by-zero checking. Logs parsing details (expression, tokens, result) to stderr; prints result to stdout; exits 1 with `ERROR:` on invalid input.

All 6 test suites (`{aws,gcp,azure}/{sdk,cli}-tests`) build the evaluator binary in TestMain alongside the simulator binary.

**SDK tests** (21 new): 7 per cloud — 4 FaaS (Lambda/CloudFunctions/AzureFunctions: basic `3+4*2→11`, parentheses `(3+4)*2→14`, invalid `3+→ERROR`, logs+complex `((2+3)*4-1)/3→6.333`) + 3 container (ECS/CloudRunJobs/ContainerApps: `(10+5)*2→30`, invalid→exit 1, division `10/3→3.333` with log verification).

**CLI tests** (6 new): 2 per cloud — container service tests (`(3+4)*2→14` success, `3+`→exit 1 failure).

**Tests**: SDK: AWS 35→42, GCP 36→43, Azure 31→38 | CLI: AWS 24→26, GCP 19→21, Azure 14→19

## Phase 68 — Multi-Tenant Backend Pools (In Progress)

### P68-001: Pool Configuration ✅
Added `PoolConfig` and `PoolsConfig` types to `backends/core/` for defining named backend pools with concurrency limits and queue sizes. `ValidatePoolsConfig()` checks 8 rules (non-empty pools, unique names, valid backend types, non-negative limits, default pool exists). `LoadPoolsConfig()` loads from `SOCKERLESS_POOLS_CONFIG` env var → `$SOCKERLESS_HOME/pools.json` → default single-pool config. Includes `GetPool()` and `PoolNames()` convenience methods. 18 tests.

## Project Stats

- **71 phases** (1-67, 69-71), 595+ tasks completed
- **16 Go modules** across backends, simulators, sandbox, agent, API, frontend, bleephub, gitlabhub, tests
- **21 Go-implemented builtins** in WASM sandbox
- **18 driver interface methods** across 5 driver types
- **7 external test consumers**: `act`, `gitlab-runner`, `gitlab-ci-local`, upstream act, `actions/runner`, `gh` CLI, gitlabhub gitlab-runner
- **Core tests**: 255 PASS | **Frontend tests**: 7 PASS | **bleephub tests**: 298 PASS | **gitlabhub tests**: 129 PASS | **Shared ProcessRunner**: 15 PASS
- **Cloud SDK tests**: AWS 42, GCP 43, Azure 38 | **Cloud CLI tests**: AWS 26, GCP 21, Azure 19
- **3 cloud simulators** validated against SDKs, CLIs, and Terraform — now with real process execution for all services (container + FaaS)
- **8 backends** sharing a common driver architecture
