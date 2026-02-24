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

## Completed Phases (1-56)

| Phase | What | Key Artifacts |
|---|---|---|
| 1-10 | Foundation: simulators (AWS/GCP/Azure), backends, agent, frontend | 3 simulators, 8 backends, Docker REST API frontend |
| 11-20 | WASM sandbox, process execution, archive ops, build contexts | `sandbox/` module, pre-start staging, bind mounts |
| 21-30 | E2E tests, driver interfaces, GitLab runner support | 217 GitHub + 154 GitLab E2E tests, DriverSet |
| 31 | Enhanced WASM sandbox: 12 new builtins, pwd fix | 46 sandbox tests |
| 32 | Driver interface completion + code splitting (<400 lines/file) | 6 new driver methods, 0 handler bypasses |
| 33 | Service container support: health checks, NetworkingConfig | 6 health check tests |
| 34 | Docker build endpoint: Dockerfile parser (FROM/COPY/ENV/CMD/...) | 15 parser tests |
| 35 | bleephub: official GitHub Actions runner integration | Azure DevOps-derived internal API, Docker integration test |
| 36 | bleephub: users, auth, OAuth device flow, GraphQL engine | 19 unit tests |
| 37 | bleephub: in-memory git hosting (go-git, smart HTTP) | 33 unit tests |
| 38 | bleephub: organizations, teams, RBAC | 51 unit tests |
| 39 | bleephub: issues, labels, milestones, comments, reactions | 79 unit tests |
| 40 | bleephub: pull requests, reviews, merge | 107 unit tests |
| 41 | bleephub: API conformance, REST pagination, OpenAPI, `gh` CLI test | 148 unit tests + gh CLI integration |
| 42 | bleephub: `uses:` actions, multi-job engine, matrix, artifacts/cache | 190 unit tests |
| 43 | Cloud resource tracking: unified tagging, resource registry, CloudScanner | 11 new tests |
| 44 | Crash-only software: auto-save, atomic writes, startup recovery | 18 new tests |
| 45 | Podman pod API + PodContext/PodRegistry, implicit grouping | 21 pod tests |
| 46 | Cloud multi-container specs: deferred start, ECS/CR/ACA multi-container | 8 new tests |
| 47 | sockerless CLI + context management (`~/.sockerless/`) | 6 new tests |
| 48 | Private management API: healthz, status, container summary | 4 new tests |
| 49 | Server lifecycle + metrics: latency percentiles, PID management | 4 new tests |
| 50 | Resource management, health checker, live config reload | 3 new tests |
| 51 | CI service containers: bleephub services parsing, health wait, E2E | 9 new tests |
| 52 | Upstream test expansion: gitlab-ci-local 36, GitHub 31, GitLab 22 | 23 new E2E test cases |
| 53 | Production Docker API: TLS, Docker auth, registry creds, tmpfs | 16 new tests |
| 54 | Production Docker API: log filter/follow, ExtraHosts, DNS, restart | 18 new tests |
| 55 | Production Docker API: EventBus, disconnect, update, volume filters | 15 new tests |
| 56 | Docker API polish: list limit/sort, ancestor/network/health/before/since filters, export, commit, push stub, flushingCopy | 14 new tests |

## Phase 57 — Production GitHub Actions: Multi-Job, Scaling, & Validation

Made bleephub's multi-job workflow engine production-ready:

- **Output capture**: Runner's `outputVariables` from FinishJob are resolved against `JobDef.Outputs` (`${{ steps.<id>.outputs.<name> }}` expressions) and stored on WorkflowJob for propagation via `needs` context
- **continue-on-error**: Jobs with `continue-on-error: true` don't block dependent dispatch — dependents still run, `needs.<job>.result` still reports "failure"
- **max-parallel**: Matrix groups respect `strategy.max-parallel` — dispatches count queued/running jobs in same group, skip if at limit
- **Job timeout**: `timeout-minutes` per job (default 360), background watcher goroutine checks every 30s, marks timed-out jobs as cancelled
- **Round-robin distribution**: `sendMessageToAgent` now sorts sessions deterministically and iterates from `lastSessionIdx` for fair load balancing
- **Pending message queue**: Failed sends are requeued to `Store.PendingMessages`; drained when new sessions connect
- **Concurrent workflow limits**: `BLEEPHUB_MAX_WORKFLOWS` env var (default 10), rejects with 429 when at limit
- **Metrics collector**: Tracks workflow submissions, job dispatches, completions by result, active workflows/sessions, uptime, heap/goroutines
- **Management API**: `GET /internal/metrics` (JSON snapshot), `GET /internal/status` (active workflows, jobs by status, connected runners, uptime)
- **Structured logging**: Standardized fields (`workflow_id`, `workflow_name`, `job_key`, `job_id`, `result`, `duration_ms`) on all workflow lifecycle events
- **Code split**: `workflows.go` split into `workflows.go` (engine) + `workflows_msg.go` (message building + helpers)
- **Integration tests**: Expanded from 2 to 6 scenarios (single-job, multi-job, 3-stage pipeline, 2x2 matrix, output propagation, service containers)

**Files**: 6 new (`workflows_msg.go`, `outputs.go`, `outputs_test.go`, `metrics.go`, `metrics_test.go`, `workflows_complex_test.go`), ~10 modified
**Tests**: 221 unit tests (+25), 6 integration scenarios (+4)

## Phase 58 — CI Pipeline & Publishing

Established proper CI/CD for the project:

- **Core CI workflow** (`.github/workflows/ci.yml`): triggers on push to `main`/`implementation` and PRs to `main`. Three parallel job groups: lint, 5-suite unit test matrix (sandbox, core, bleephub, frontend, integration), and build check (8 binaries with `GOWORK=off`). Uses `concurrency` with `cancel-in-progress: true`
- **Comprehensive test workflow** (`.github/workflows/test-comprehensive.yml`): post-merge and manual trigger. Simulator tests (3 clouds), bleephub integration, GitHub/GitLab E2E (memory), plus manual-only terraform and upstream act
- **Production Dockerfiles**: multi-stage builds for bleephub, frontend, memory backend. `golang:1.24-alpine` builder, `alpine:3.20` runtime, `CGO_ENABLED=0`, `GOWORK=off`. Bumped simulator Dockerfiles from Go 1.23 → 1.24
- **Docker image workflow** (`.github/workflows/docker.yml`): builds and pushes 6 images to GHCR. Multi-arch (`linux/amd64,linux/arm64`), semver tags, GHA layer cache
- **Version injection**: added `version`/`commit` vars to bleephub, frontend, CLI main entry points. Logged at startup, CLI `version` command uses injected value
- **goreleaser config** (`.goreleaser.yml`): 8 binaries (bleephub, sockerless, frontend, backend-memory, 3 simulators, agent) × 4 platforms. `CGO_ENABLED=0`, `GOWORK=off`, ldflags version injection
- **Release workflow** (`.github/workflows/release.yml`): triggered on `v*` tags, gates on CI + test-gate, runs goreleaser
- **GitLab CI update**: added `merge_request_event` rule to lint job

**Files**: 4 new workflows, 3 new Dockerfiles, `.goreleaser.yml`, 3 modified main.go, 3 modified Dockerfiles, `.gitlab-ci.yml`

## Phase 59 — Production GitHub Actions Runner

Closed critical production gaps in bleephub's GitHub Actions runner support:

- **Secrets store & API** (`bleephub/secrets.go`): Per-repo secret storage with full CRUD via REST endpoints matching GitHub Actions Secrets API (`GET/PUT/DELETE /api/v3/repos/{owner}/{repo}/actions/secrets[/{name}]`). Values never exposed via GET. 8 tests
- **Secrets injection & masking** (`bleephub/workflows_msg.go`): Repository secrets injected into runner job messages as `"secrets"` context data. `GITHUB_TOKEN` always included. Mask array built with regex entries for each secret value to prevent log leakage
- **Expression evaluator** (`bleephub/expressions.go`): Minimal recursive-descent parser for GitHub Actions `if:` conditions. Supports `success()`, `failure()`, `always()`, `cancelled()` status functions, string comparison (`==`, `!=`), boolean operators (`&&`, `||`, `!`), context access (dot-notation), and parentheses. 14 tests
- **Job-level `if:` evaluation** (`bleephub/workflows.go`): Wired expression evaluator into `dispatchReadyJobs`. Expressions containing `always()` or `failure()` override default dep-failure skip behavior. 3 tests
- **Matrix fail-fast** (`bleephub/workflows.go`): In `onJobCompleted`, if a matrix job fails and `fail-fast` is true (default per spec), cancel pending/queued siblings in the same matrix group. Running siblings continue (matches real GitHub Actions). 4 tests
- **Workflow cancellation API** (`bleephub/workflows.go`, `bleephub/jobs.go`): `cancelWorkflow` helper cancels all pending/queued jobs, sets workflow result to cancelled. HTTP endpoint `POST /api/v3/bleephub/workflows/{id}/cancel` with 200/404/409. 4 tests
- **Concurrency control** (`bleephub/workflow.go`, `bleephub/workflows.go`): Parse `concurrency:` workflow key (string or object). `cancel-in-progress: true` cancels active workflow in same group. `false` queues as `"pending_concurrency"`, started when active workflow completes. 6 tests
- **Event context & dispatch inputs** (`bleephub/jobs.go`, `bleephub/workflows_msg.go`): Extended `WorkflowSubmitRequest` with `event_name`, `ref`, `sha`, `repo`, `inputs`. All fields flow through to runner message context data. Default values for backward compatibility
- **Persistent artifacts** (`bleephub/artifacts.go`): Disk-based artifact storage when `BLEEPHUB_DATA_DIR` is set. Layout: `$DIR/artifacts/{id}/meta.json + data`. Upload appends to data file. Recovery on startup scans artifacts dir. In-memory mode preserved for tests
- **Integration test expansion** (`bleephub/test/run-integration.sh`): 3 new scenarios — secrets injection (TEST 7), workflow dispatch with inputs (TEST 8), matrix fail-fast (TEST 9)

**Files**: 3 new (secrets.go, expressions.go, concurrency_test.go + test files), 7 modified (store.go, server.go, workflow.go, workflows.go, workflows_msg.go, jobs.go, artifacts.go, run-integration.sh)

**Tests**: 259 unit tests (+38), 9 integration scenarios (+3)

## Phase 60 — Production GitLab CI: gitlabhub Server + Single-Job Pipelines

Built **gitlabhub** — a lightweight Go server implementing the GitLab Runner coordinator API, the GitLab equivalent of bleephub. Enables unmodified `gitlab-runner` to register, poll for jobs, execute them through Sockerless, and report results, without a real GitLab CE instance.

- **Server skeleton** (`gitlabhub/server.go`): HTTP server with logging middleware, health check, catch-all git handler. Same crash-only pattern as bleephub
- **Runner registration** (`gitlabhub/runners.go`): `POST /api/v4/runners` (register, returns glrt-xxx token), `POST /api/v4/runners/verify`, `DELETE /api/v4/runners`
- **Pipeline YAML parser** (`gitlabhub/pipeline.go`): Full `.gitlab-ci.yml` parsing — stages, job definitions, image, script/before_script/after_script, variables (global+job merge), artifacts, services, needs (DAG), rules, allow_failure, when, cache. Handles reserved keys, dot-prefix templates
- **Job request endpoint** (`gitlabhub/jobs_request.go`): `POST /api/v4/jobs/request` with 30s long-poll. Builds complete `JobResponse` with steps, variables, image, services, artifacts, cache, dependencies
- **Pipeline engine** (`gitlabhub/engine.go`): Stage-ordered dispatch (all stage N complete before N+1), DAG `needs:` override, dependency failure cascading, cycle detection
- **Job lifecycle** (`gitlabhub/jobs_update.go`): `PUT /api/v4/jobs/:id` (status update), `PATCH /api/v4/jobs/:id/trace` (incremental log upload with Content-Range)
- **CI variable injection** (`gitlabhub/variables.go`): 30+ CI variables (CI, GITLAB_CI, CI_JOB_ID, CI_PIPELINE_ID, CI_REPOSITORY_URL, etc.), user-defined from YAML, project-level with masked support
- **Git repo serving** (`gitlabhub/git.go`): In-memory bare repos via go-git, smart HTTP protocol (info/refs + git-upload-pack), createProjectRepo with initial commit
- **Artifacts** (`gitlabhub/artifacts.go`): Upload (multipart/form-data) and download endpoints, dependency resolution for prior-stage artifacts
- **Cache** (`gitlabhub/cache.go`): PUT/GET/HEAD key-value cache endpoints
- **Services** (`gitlabhub/services.go`): Service container handling with automatic alias generation
- **Secrets/Variables API** (`gitlabhub/secrets.go`): Project variable CRUD (POST/GET/DELETE), masked and protected support
- **Pipeline management API** (`gitlabhub/pipeline_api.go`): `POST /api/v3/gitlabhub/pipeline` (submit YAML, creates project + git repo + pipeline), `GET /api/v3/gitlabhub/pipelines/:id` (status)
- **Metrics** (`gitlabhub/metrics.go`): Pipeline submissions, job dispatches/completions, runner registrations, uptime, heap/goroutines
- **Integration test** (`gitlabhub/test/run-integration.sh`): 9 scenarios with real gitlab-runner through Sockerless memory backend + Docker frontend
- **Dockerfile** (`gitlabhub/Dockerfile`): Multi-stage build with gitlab/gitlab-runner:latest runtime

**Files**: 27 new source files in `gitlabhub/` module, ~4000 lines. Modified: go.work (+1 line), Makefile (+target)

**Tests**: 62 unit tests, 9 integration scenarios

### Phase 61 — Advanced GitLab CI Pipelines (10 tasks)

Closed all major feature gaps in gitlabhub so it can run production `.gitlab-ci.yml` pipelines:

- **Expression evaluator** (`gitlabhub/expressions.go`): Recursive-descent parser for `rules:if:` — `$VAR`/`${VAR}` expansion, `==`/`!=` comparison, `=~`/`!~` regex matching with `/pattern/` syntax, `&&`/`||` boolean ops, parentheses, null checks for undefined variables
- **Extends keyword** (`gitlabhub/extends.go`): Job inheritance via `extends:` — deep-merge following GitLab rules (script/artifacts/rules replaced, variables merged), multi-level chains (A extends B extends C), `extends: [.a, .b]` list form, circular detection
- **Include keyword** (`gitlabhub/include.go`): `include:local:` YAML composition — reads files from go-git storage, merges stages (union), variables (main overrides), jobs (main precedence). Works with extends across included files
- **Parallel/matrix** (`gitlabhub/parallel.go`): `parallel: N` → N copies (`job 1/N`..`job N/N`), `parallel:matrix:` → cartesian product expansion with matrix variables injected
- **Timeout/retry** (`gitlabhub/timeout.go`): Duration parsing (`10s`, `2m`, `1h 30m`), `RetryDef` (max 0-2), retry logic re-enqueues failed jobs
- **Dotenv artifacts** (`gitlabhub/dotenv.go`): `artifacts:reports:dotenv` — parses dotenv from uploaded zip, injects vars into downstream job variables
- **Pipeline cancellation** (`gitlabhub/engine.go`): `POST /api/v3/gitlabhub/pipelines/:id/cancel` — cancels all non-terminal jobs, removes from queue
- **Resource groups** (`gitlabhub/engine.go`): `resource_group:` — only one job per group runs at a time, next dispatched on completion
- **DinD support** (`gitlabhub/jobs_request.go`): Auto-detect `docker:*-dind` services, inject `DOCKER_HOST`, `DOCKER_TLS_CERTDIR`, `DOCKER_DRIVER`. Service-level `variables:` passthrough
- **Integration tests** (`gitlabhub/test/run-integration.sh`): 8 new scenarios covering all Phase 61 features

**Files**: 8 new source files, 9 modified files in `gitlabhub/`, ~2200 lines added

**Tests**: 129 unit tests (62 prior + 67 new), 17 integration scenarios (9 prior + 8 new)

### Phase 62 — Docker API Hardening for Production (9 tasks)

Closed Docker API gaps discovered during production validation with CI runners.

- **HEALTHCHECK parsing** (`backends/core/build.go`): Parse `HEALTHCHECK CMD`, `HEALTHCHECK CMD [...]`, `HEALTHCHECK NONE`, with options `--interval`, `--timeout`, `--retries`, `--start-period`. Multi-stage resets healthcheck
- **Volume auto-creation** (`backends/core/handle_containers_archive.go`): Named volumes from `docker run -v mydata:/data` auto-created in store + VolumeDirs. In-use check on `handleVolumeRemove` returns 409 Conflict unless `force=true`
- **Container mounts population** (`backends/core/handle_containers_archive.go`): `buildMounts()` parses Binds, HostConfig.Mounts, Tmpfs into `Container.Mounts` for docker inspect
- **Network cleanup on remove** (`backends/core/handle_containers.go`): Container removal now cleans up `Network.Containers` map entries
- **Restart delay** (`backends/core/restart_policy.go`): Exponential backoff `100ms × 2^restartCount`, capped at 60s
- **Error consistency** (`backends/core/build.go`, `handle_exec.go`, `handle_containers_query.go`, `agent_registry.go`): All `http.Error()` → `WriteError()` for Docker-format JSON `{"message":"..."}`
- **TTY log mode** (`backends/core/handle_containers_query.go`): Raw stream for `Config.Tty: true`, multiplexed stream for non-TTY
- **Atomic prune** (`backends/core/store.go`): `PruneIf()` on `StateStore[T]` holds write lock for entire operation. Used in container, volume, network prune handlers
- **Compose compat** (`frontends/docker/system.go`): `Containerd` stub in Info, `containerd` component in Version

**Files**: 7 new test files, 13 modified source files, ~1400 lines added

**Tests**: 230 core PASS (177 prior + 53 new), 4 frontend PASS

## Phase 63 — Docker Compose E2E: Race Fix, Dockerfile Directives, Image Prune (10 tasks)

Fixed a pre-existing data race in health check and closed remaining gaps for full `docker compose` support.

- **Health check race fix** (`backends/core/health.go`): Deep-copy `HealthState` struct on every `Update()` to avoid sharing the `*HealthState` pointer between the health check goroutine and concurrent `Get()` callers
- **SHELL directive** (`backends/core/build.go`, `api/types.go`): Parse `SHELL ["/bin/bash", "-c"]` in Dockerfiles, add `Shell []string` field to `ContainerConfig`, propagate through multi-stage builds
- **STOPSIGNAL directive** (`backends/core/build.go`): Parse `STOPSIGNAL SIGTERM` and set `ContainerConfig.StopSignal`
- **VOLUME directive** (`backends/core/build.go`): Parse `VOLUME ["/data"]` (JSON array) and `VOLUME /data /logs` (space-separated), populate `ContainerConfig.Volumes` map
- **Image prune** (`backends/core/handle_images.go`): Real implementation replacing no-op stub. Removes unreferenced images, skips in-use images, supports `dangling` filter
- **Integration tests**: Health check race validation (concurrent reads during health transitions), service discovery (peer DNS resolution with pods, hostnames, aliases), compose lifecycle (create-start-stop-remove, volume persistence, network cleanup, name reuse)
- **Ignored list cleanup** (`backends/core/build.go`): Removed `SHELL`, `STOPSIGNAL`, `VOLUME` from ignored list — now only `RUN` and `ONBUILD` ignored

**Files**: 4 new test files, 5 modified source files, ~550 lines added

**Tests**: 249 core PASS (230 prior + 19 new), 4 frontend PASS

## Phase 64 — Webhooks for bleephub

Added GitHub-compatible webhook infrastructure to bleephub: per-repo CRUD registration, HMAC-SHA256 signing, async delivery engine with 3-retry exponential backoff (1s/5s), delivery log API, event payloads (push/pull_request/issues/ping), and CI trigger integration (push/PR events fire matching `on:` workflows from git storage).

**New files**: `webhooks_store.go` (types + store CRUD), `gh_hooks_rest.go` (7 REST endpoints), `webhooks.go` (HMAC signing + delivery engine + CI trigger), `webhooks_payloads.go` (event payload builders), `webhooks_test.go` (11 tests)

**Modified**: `store.go` (Hooks/HookDeliveries maps), `server.go` (route registration), `git_http.go` (push event emission), `gh_pulls_rest.go` (PR events), `gh_issues_rest.go` (issue events)

**Tests**: 270 bleephub PASS (259 prior + 11 new)

## Phase 65 — GitHub Apps for bleephub

Added GitHub App authentication and installation token support to bleephub, enabling app-based workflows alongside existing PAT auth.

- **App store + RSA keygen** (`bleephub/gh_apps_store.go`): App/Installation/InstallationToken types, CRUD methods, 2048-bit RSA key pair generation per app, `ghs_`-prefixed installation tokens with 1h expiry, manifest code one-time-use flow
- **RS256 JWT sign/verify** (`bleephub/gh_apps_jwt.go`): Pure Go stdlib JWT implementation — parse header/payload/signature, verify algorithm, validate lifetime (≤600s), check expiry/iat with 60s clock skew, lookup app by `iss` claim, verify RSA signature. Signing function for tests
- **Auth middleware extension** (`bleephub/gh_middleware.go`): `looksLikeJWT()` detection (starts with `eyJ`, 2 dots), `ghs_` prefix for installation tokens, bot user synthesis (`slug[bot]`) for backward compat with handlers checking `ghUserFromContext`
- **REST API endpoints** (`bleephub/gh_apps_rest.go`): 9 endpoints — manifest conversion, authenticated app info, installation CRUD, installation token creation, repo installation lookup, plus management shortcuts for testing
- **Tests** (`bleephub/gh_apps_test.go`): 10 unit tests (store CRUD, JWT sign/verify/expired/long-lifetime/wrong-ID/bad-signature, token expiry, manifest codes) + 12 integration tests (HTTP endpoints for app creation, JWT auth, installation management, token auth, cross-app rejection, PAT backward compat)

**New files**: `gh_apps_store.go`, `gh_apps_jwt.go`, `gh_apps_rest.go`, `gh_apps_test.go`

**Modified**: `store.go` (5 new maps + 2 counters), `gh_middleware.go` (JWT/ghs_ detection + context keys), `server.go` (route registration)

**Tests**: 293 bleephub PASS (270 prior + 23 new)

## Phase 66 — Optional OpenTelemetry Tracing

Added opt-in distributed tracing to all 4 HTTP servers (Docker frontend, backend core, bleephub, gitlabhub). When `OTEL_EXPORTER_OTLP_ENDPOINT` is set, traces flow end-to-end via W3C Trace Context; when unset, the default no-op TracerProvider adds zero overhead.

- **InitTracer pattern** (4 new `otel.go` files): Identical function in each module — checks env var, creates OTLP HTTP exporter + `sdktrace.NewTracerProvider` with batching, sets global provider. Returns shutdown func. No-op when disabled.
- **HTTP middleware** (4 servers): `otelhttp.NewHandler` wraps each server's handler chain — automatic per-request spans with method, path, status code attributes.
- **Context propagation** (`frontends/docker/backend_client.go`): All 11 HTTP methods gained `ctx context.Context` as first parameter. Transport wrapped with `otelhttp.NewTransport` for outbound W3C trace header injection. 56 frontend handler call sites updated with `r.Context()`.
- **Workflow spans** (`bleephub/workflows.go`): 4 functions (submitWorkflow, dispatchReadyJobs, dispatchWorkflowJob, onJobCompleted) create spans with workflow/job attributes. ~67 test call sites updated.
- **Pipeline spans** (`gitlabhub/engine.go`): 3 functions (submitPipeline, dispatchReadyJobs, onJobCompleted) create spans with pipeline/job attributes. ~13 test call sites updated.
- **Tests**: 8 new tests — InitTracer no-op/active, HTTP middleware span creation, workflow dispatch span, no-crash when disabled.

**New files**: `backends/core/otel.go`, `frontends/docker/otel.go`, `bleephub/otel.go`, `gitlabhub/otel.go`, `backends/core/otel_test.go`, `bleephub/otel_test.go`

**Modified**: 4 go.mod (OTel deps), 4 cmd/main.go (InitTracer), 4 server.go (middleware), `backend_client.go` (context + transport), 8 frontend handlers (r.Context()), `workflows.go` + 3 callers (context), `engine.go` + 2 callers (context), 6 test files (context.Background())

**Tests**: 241 core PASS (249→241 renumbered), 298 bleephub PASS (293 + 5 new), 129 gitlabhub PASS, 7 frontend PASS

## Phase 67 — Network Isolation (Linux)

Extracted network logic from handlers into a proper driver abstraction with IPAM allocator, and added Linux network namespace infrastructure as a best-effort layer.

- **IPAllocator** (`ipam.go`): Standalone allocator replacing fragile inline IP math. Auto-assigns subnets from `172.18.0.0/16` pool, sequential host IPs from `.2`, MAC generation from IP octets (`02:42:ac:XX:YY:ZZ`), IP release/reuse, custom subnet support.
- **SyntheticNetworkDriver** (`drivers_network.go`): Implements `api.NetworkDriver` (8 methods) using in-memory store + IPAllocator. Default driver on all platforms.
- **Linux NetnsManager** (`netns_linux.go`/`netns_other.go`): Build-tagged manager for Linux network namespaces. Creates netns + bridge interfaces, veth pairs for container attachment. Graceful capability check (`Available()`) — no-op on macOS/non-root.
- **LinuxNetworkDriver** (`drivers_network_linux.go`/`drivers_network_other.go`): Wraps SyntheticNetworkDriver, adds best-effort real netns operations. Logs warning and continues on failure.
- **Handler refactoring** (`handle_networks.go`): All 7 network handlers became thin HTTP→driver adapters. Events and pod grouping remain in handlers; IPAM and store ops delegated to driver.
- **Container integration**: `buildEndpointForNetwork` uses IPAllocator for endpoint creation. Container remove uses `Drivers.Network.Disconnect()` for cleanup (enables Linux veth pair teardown).
- **Tests**: 14 new tests — 6 IPAM (subnet uniqueness, sequential IPs, release/reuse, custom subnet, MAC generation, subnet release), 8 driver (create, duplicate, connect, disconnect, prune, list, remove-builtin, integration).

**New files**: `ipam.go`, `drivers_network.go`, `netns_linux.go`, `netns_other.go`, `drivers_network_linux.go`, `drivers_network_other.go`, `ipam_test.go`, `drivers_network_test.go`, `netns_linux_test.go`

**Modified**: `drivers.go` (Network field in DriverSet), `store.go` (IPAlloc field), `server.go` (InitDrivers network setup), `handle_networks.go` (7 handlers refactored), `handle_containers.go` (remove uses driver disconnect), `handle_containers_archive.go` (buildEndpointForNetwork uses IPAllocator)

**Tests**: 255 core PASS (241 + 14 new), 7 frontend PASS, 298 bleephub PASS, 129 gitlabhub PASS

## Phase 69 — ARM64 / Multi-Arch Completion

Filled remaining multi-arch gaps so all 15 binaries are covered by goreleaser, Docker images, and CI ARM64 verification.

- **Goreleaser expansion** (`.goreleaser.yml`): Added 7 missing build entries — 6 cloud backends (ECS, Lambda, Cloud Run, Cloud Functions, ACA, Azure Functions) and gitlabhub. Total: 15 builds × 2 OS × 2 arch = 60 release binaries
- **Gitlabhub Dockerfile** (`gitlabhub/Dockerfile.release`): Multi-stage build matching bleephub pattern — `golang:1.24-alpine` builder, `alpine:3.20` runtime
- **Docker workflow** (`.github/workflows/docker.yml`): Added gitlabhub to matrix. Total: 7 multi-arch Docker images on GHCR
- **CI build-check expansion** (`.github/workflows/ci.yml`): Existing `build-check` job now builds all 15 binaries (was 8). New `build-check-arm64` job cross-compiles all 15 binaries with `GOOS=linux GOARCH=arm64 CGO_ENABLED=0`

**New files**: `gitlabhub/Dockerfile.release`

**Modified**: `.goreleaser.yml` (+7 builds), `.github/workflows/docker.yml` (+1 matrix entry), `.github/workflows/ci.yml` (+7 build lines, +new ARM64 job)

## Project Stats

- **68 phases** (1-67, 69), 578 tasks completed
- **16 Go modules** across backends, simulators, sandbox, agent, API, frontend, bleephub, gitlabhub, tests
- **21 Go-implemented builtins** in WASM sandbox
- **18 driver interface methods** across 5 driver types (ExecDriver 1, FilesystemDriver 4, StreamDriver 4, ProcessLifecycleDriver 8, NetworkDriver 8)
- **7 external test consumers**: `act` (31 workflows), `gitlab-runner` (22 pipelines), `gitlab-ci-local` (36 tests), upstream act test suite, official `actions/runner`, `gh` CLI, gitlabhub gitlab-runner
- **Core tests**: 255 PASS | **Frontend tests**: 7 PASS (TLS + mux) | **bleephub tests**: 298 PASS | **gitlabhub tests**: 129 PASS
- **3 cloud simulators** validated against SDKs, CLIs, and Terraform
- **8 backends** sharing a common driver architecture
