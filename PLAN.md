# Sockerless — Roadmap

> Phases 1-43 complete (382 tasks). This document covers future work.
>
> **Production target:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners (GitHub Actions from github.com, GitLab CI from gitlab.com), and custom SDK clients — backed by real cloud infrastructure (AWS, GCP, Azure). Phases 39-46 build out the testing infrastructure and bleephub. Phases 47-49 achieve general Docker API production readiness. Phases 50-53 achieve production CI. Phases 54-55 harden for production operations.

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

## Phase 31 — Enhanced WASM Sandbox ✓

**Completed.** Fixed pwd to return container-relative paths. Added 12 builtins (touch, base64, basename, dirname, which, seq, readlink, tee, ln, stat, sha256sum, md5sum). Refactored all builtins into `sandbox/builtins.go`. 46 unit tests pass, 129 sim-test-all pass, 0 lint issues. Upstream act memory: 91 PASS / 24 FAIL (up from 57/28). All 4 bash/shell failures fixed. Dockerfile.memory fixed (missing `COPY agent/`).

---

## Phase 32 — Driver Interface Completion & Code Splitting ✓

**Completed.** Eliminated all 8 driver bypasses in handler code. Added 6 new methods across driver interfaces: `WaitCh`, `Top`, `Stats`, `IsSynthetic` on ProcessLifecycleDriver; `RootPath` on FilesystemDriver; `LogBytes` on StreamDriver. Zero `ProcessFactory`/`Store.Processes` references remain in handler files. Moved wait-and-stop goroutine into `WASMProcessLifecycleDriver.Start()`. Split 4 large files: `handle_containers.go` (829→3 files), `shell.go` (602→3 files), `builtins.go` (597→4 files), `containers.go` frontend (419→2 files). Extracted `buildContainerFromConfig` helper. All files under 400 lines. All tests pass, 0 lint issues. 279 tasks done across 32 phases.

---

## Phase 33 — Service Container Support ✓

**Completed.** Added Docker API fidelity for service container orchestration. Health check infrastructure with periodic exec-based health checking (`starting` → `healthy` / `unhealthy`). `NetworkingConfig` processing at container create (aliases, IPAM resolution, Network.Containers map). Port reporting in container list and inspect. 6 health check unit tests. All tests pass, 0 lint issues. 129 sim-test-all, 46 sandbox tests, no regression.

---

## Phase 34 — Docker Build Endpoint ✓

**Completed.** Implemented `POST /build` with a minimal Dockerfile parser (FROM, COPY, ADD, ENV, CMD, ENTRYPOINT, WORKDIR, ARG, LABEL, EXPOSE, USER). Build context files (COPY) injected via staging dirs on container create. Pull guard prevents `POST /images/create` from overwriting built images. Streaming Docker build JSON progress output. Frontend route wired. 15 parser unit tests, system test updated. All tests pass, 0 lint issues. 129 sim-test-all, 46 sandbox tests, no regression. RUN not executed (no-op) — sufficient for CI Dockerfile-based actions.

---

## Phase 35 — Official GitHub Actions Runner + `bleephub` GitHub API Simulator ✓

**Completed.** Built `bleephub/`, a Go module implementing the Azure DevOps-derived internal API that the official GitHub Actions runner uses. The runner registers, polls for jobs, executes container workflows through Sockerless's Docker API, and reports completion. Implemented 5 service groups: auth/tokens (JWT exchange, connection data with GUIDs), agent registration (pools, agents, credentials), broker (sessions, 30s long-poll), run service (acquire/renew/complete), timeline + logs (CRUD, log upload). Job message builder converts simplified JSON to the runner's PipelineContextData + TemplateToken format. Also fixed 3 sandbox issues discovered during integration: `tail -f /dev/null` keepalive detection (WASI has no inotify), host path bind mount resolution, and overlapping volume mount symlink deduplication. Docker integration test with official runner binary passes. 11 tasks, ~1300 lines of Go.

---

## Phase 36 — bleephub: Users, Auth + GraphQL Engine ✓

**Completed.** Added user accounts, authentication, OAuth device flow, and GraphQL execution engine to bleephub using `graphql-go/graphql`. 6 new files, 19 unit tests pass (7 existing + 12 new), 0 lint issues. All `gh api` commands work with explicit URLs. TLS support added via `BPH_TLS_CERT`/`BPH_TLS_KEY` env vars. Note: `gh auth login` requires HTTPS + port 443 (GHES assumption), so full `gh auth` integration requires TLS deployment.

---

## Phase 37 — bleephub: Git Repositories ✓

**Completed.** In-memory git repository hosting with smart HTTP protocol using `go-git/go-git/v5`. Repo CRUD via REST and GraphQL, smart HTTP for `git clone`/`git push`, branch/commit/tree/blob/README endpoints, Relay pagination on `viewer.repositories`. 33 unit tests pass (19 existing + 14 new), 0 lint issues in new code. All `gh api` commands work. `git push` + `git clone` validated against real git CLI.

---

## Phase 38 — bleephub: Organizations + Teams + RBAC ✓

**Completed.** Organization accounts, teams, memberships, and RBAC for bleephub. 7 new files (~1200 lines), 18 new unit tests (51 total). Org CRUD, team CRUD, membership management, RBAC enforcement on repos (public/private visibility, org admin, team permission levels), org-owned repos, GraphQL `viewer.organizations` + `organization(login)` query. All `gh api` commands work for org/team operations.

---

## Phase 39 — bleephub: Issues + Labels + Milestones ✓

**Completed.** Full issue tracking with labels, milestones, comments, and reactions. 6 new files (~1,700 lines), 28 new unit tests (79 total). REST endpoints for labels, milestones, issues, comments, and issue-label management. GraphQL mutations (createIssue, closeIssue, reopenIssue, addComment, updateIssue) and queries (repository.issues, repository.issue, repository.labels, repository.milestones, repository.assignableUsers). IssueState/MilestoneState/IssueClosedStateReason enums for `gh` compatibility. Static reactionGroups (8 types). hasIssuesEnabled, viewerPermission, merge*Allowed fields on Repository type. All `gh api` commands work for issue lifecycle.

---

## Phase 40 — bleephub: Pull Requests ✓

**Completed.** Full pull request lifecycle with reviews, merge, and shared issue/PR numbering. 4 new files (~1,650 lines), 28 new unit tests (107 total). PullRequest + PullRequestReview store types with 7 CRUD methods. REST endpoints for PR CRUD, merge, reviews, and reviewer requests. GraphQL mutations (createPullRequest, closePullRequest, reopenPullRequest, mergePullRequest, updatePullRequest) and queries (repository.pullRequests connection with state/label/head/base filters, repository.pullRequest by number). PR-prefixed GraphQL types to avoid name collisions. Review decision derived from reviews (APPROVED/CHANGES_REQUESTED). StatusCheckRollup stub. All `gh pr` operations work against bleephub.

---

## Phase 41 — bleephub: API Conformance + `gh` CLI Test Suite ✓

**Completed.** Added REST pagination with RFC 5988 Link headers (15 list handlers). GitHub-standard 422 validation error format with `errors` array (`resource`, `field`, `code`). Content-Type charset upgrade (`application/json; charset=utf-8`) in middleware. Repo `permissions` object in REST responses. OpenAPI schema validation test suite (vendored schemas for 8 resource types). Comprehensive conformance tests: error formats (401/404/422), content negotiation, rate limit headers, cross-endpoint REST↔GraphQL consistency. Docker-based `gh` CLI integration test (`make bleephub-gh-test`) exercising full lifecycle: repos, labels, issues, PRs, reviews, merges, orgs, pagination, GraphQL queries. 148 unit tests pass (107→148: +7 pagination, +23 conformance, +11 OpenAPI).

---

## Phase 42 — bleephub: Runner Enhancements (Actions + Multi-Job) ✓

**Completed.** Expanded bleephub to support `uses:` actions (tarball proxy with in-memory cache), multi-job workflows with `needs:` dependency graph execution, matrix strategy expansion (Cartesian product + include/exclude), workflow YAML parsing (`gopkg.in/yaml.v3`), artifact upload/download (Twirp-style stubs for `@actions/artifact` v4), and cache API stubs (miss/discard). 7 new files (~1,300 lines), 42 new unit tests (190 total). Integration test updated with multi-job workflow test.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P42-NNN.md`.

| Task | Description |
|---|---|
| P42-001 ✓ | Workflow YAML parsing: `ParseWorkflow()`, types for WorkflowDef/JobDef/StepDef/MatrixDef/StrategyDef |
| P42-002 ✓ | Action resolution: `ActionDownloadInfo` with tarball URLs, tarball proxy with in-memory cache |
| P42-003 ✓ | Multi-job workflow engine: `submitWorkflow`, `dispatchReadyJobs`, `onJobCompleted`, `buildJobMessageFromDef` |
| P42-004 ✓ | Workflow submission endpoint + matrix expansion: `ExpandMatrix`, `expandMatrixJobs`, `POST /api/v3/bleephub/workflow` |
| P42-005 ✓ | Artifacts + cache API stubs: Twirp artifact service (create/upload/finalize/list/download), cache stubs (204 miss) |
| P42-006 ✓ | Integration test: multi-job workflow with `needs:` dependency via `POST /api/v3/bleephub/workflow` |
| P42-007 ✓ | Save final state |

**Verification:** 190 unit tests pass. Integration test covers single-job + multi-job workflow with `needs:` dependencies.

---

## Phase 43 — Cloud Resource Tracking ✓

**Completed.** Unified cloud resource tagging, tracking, and crash recovery across all 6 backends. Shared tag builder in `backends/core/tags.go` with 3 output formats (AWS map, GCP labels, Azure pointer map). 5 standard tags: `sockerless-managed`, `sockerless-container-id`, `sockerless-backend`, `sockerless-instance`, `sockerless-created-at`. Resource registry with JSON persistence and REST endpoints. CloudScanner interface for crash recovery in all 6 backends. AWS ECS + Lambda simulators updated to store and return tags.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P43-NNN.md`.

| Task | Description |
|---|---|
| P43-001 ✓ | Shared tag builder (`backends/core/tags.go`): TagSet with AsMap, AsGCPLabels, AsAzurePtrMap + 6 unit tests |
| P43-002 ✓ | Tag ECS resources: tags on RegisterTaskDefinition + RunTask, registry register/cleanup |
| P43-003 ✓ | Tag Lambda resources: tags on CreateFunction, registry register/cleanup |
| P43-004 ✓ | Tag GCF resources: labels on CreateFunction via AsGCPLabels, registry register/cleanup |
| P43-005 ✓ | Tag Azure Functions resources: tags on Site via AsAzurePtrMap, registry register/cleanup |
| P43-006 ✓ | Normalize Cloud Run + ACA to shared tag builder (replace hardcoded labels/tags) |
| P43-007 ✓ | AWS simulator tag support: ECSTag struct, Tags on ECSTaskDefinition/ECSTask, Lambda Tags + ListTags/TagResource |
| P43-008 ✓ | Resource registry (`backends/core/resource_registry.go`): register/cleanup/save/load + REST endpoints + 5 unit tests |
| P43-009 ✓ | Crash recovery: CloudScanner interface + RecoverOnStartup + 6 backend implementations |
| P43-010 ✓ | Integration tests: TestResourceTaggingIntegration + sim-test-all pass |
| P43-011 ✓ | Save final state |

**Verification:** 11 new unit tests (6 tag + 5 registry), all sim-test-all tests pass. Tags applied to every cloud resource across all 6 backends.

---

## Phase 44 — Crash-Only Software

**Goal:** Make Sockerless a "crash-only" system — one that is always safe to crash and restart, with no distinction between clean shutdown and crash recovery. The system should be correct after any interruption at any point.

**Research note:** First task is to research the crash-only software philosophy (Candea & Fox, 2003 — "Crash-Only Software") and related work (recovery-oriented computing, fail-fast systems). Key principles: no clean shutdown path (crash is the only shutdown), all state is recoverable, startup always runs recovery, components are independently restartable.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P44-NNN.md`.

| Task | Description |
|---|---|
| P44-001 | **Research crash-only software.** Read Candea & Fox (2003), document principles and how they apply to Sockerless. Audit current shutdown paths, identify state that would be lost on crash. Write `docs/crash-only.md` with findings and design. Save state |
| P44-002 | **Persistent resource registry.** Evolve the in-memory resource registry (Phase 43) to write-ahead-log or append-only file. Every resource create/delete is durably recorded before the cloud API call. Save state |
| P44-003 | **Idempotent operations.** Audit all backend operations (container create, exec, archive, etc.) for idempotency. Ensure replaying any operation after crash is safe. Add operation IDs where needed. Save state |
| P44-004 | **Startup recovery.** On startup: (1) replay resource registry to rebuild in-memory state, (2) scan cloud for tagged resources, (3) reconcile — adopt still-running resources, clean up stale ones. No separate "clean start" vs "recovery" path. Save state |
| P44-005 | **Session recovery.** Frontend reconnection: if a CI runner reconnects after Sockerless restart, match it to its previous containers via resource tags. Restore attach/log streams. Save state |
| P44-006 | **Remove clean shutdown paths.** Eliminate graceful shutdown code that differs from crash behavior. The only way to stop is crash (SIGKILL). SIGTERM = immediate exit, no drain. Startup always assumes previous instance crashed. Save state |
| P44-007 | **Chaos testing.** Build a test harness that kills Sockerless at random points during E2E test runs, restarts it, and verifies the test still passes or fails cleanly (no hangs, no leaked resources, no data corruption). Save state |
| P44-008 | **Unit + integration tests.** Recovery tests: create resources, simulate crash (kill process), restart, verify state is recovered. Orphan cleanup tests. Idempotency tests. Save state |
| P44-009 | **Save final state** |

**Verification:** Kill Sockerless with SIGKILL at any point during an E2E test. Restart. Either the test resumes correctly or resources are cleaned up. No leaked cloud resources. No data corruption.

---

## Phase 45 — Upstream Test Expansion

**Goal:** Maximize external test coverage to validate Docker API correctness.

**State save:** Every task ends with state save.

| Task | Description |
|---|---|
| P45-001 | Add more gitlab-ci-local test cases (target: 35+) — artifacts, caches, includes, rules. Save state |
| P45-002 | Add more E2E GitHub workflows (target: 30+) — composite actions, reusable workflows, job containers. Save state |
| P45-003 | Add more E2E GitLab pipelines (target: 25+) — multi-stage, DAG, trigger. Save state |
| P45-004 | Investigate other Docker API consumers (Drone CI, Buildkite, Woodpecker). Save state |
| P45-005 | Run full upstream act suite per cloud backend, document per-backend delta and failure categories. Save state |
| P45-006 | Save final state |

**Verification:** All new tests pass across all 7 backends.

---

## **CANCELLED** Phase 46 — Capability Negotiation

Note. this phase will be skipped because we do not want capabilities. All backends should support all tests.
Skip to next non-cancelled phase.

**Goal:** Backends advertise what they support so runners/tests can adapt.

**State save:** Every task ends with state save.

| Task | Description |
|---|---|
| P46-001 | Define capability flags: `exec.real`, `exec.interactive`, `fs.writable`, `fs.archive`, `network.real`, `volume.persistent`. Save state |
| P46-002 | Each backend reports capabilities via driver introspection. Save state |
| P46-003 | Frontend `/info` endpoint returns capability flags. Save state |
| P46-004 | E2E tests conditionally skip tests requiring missing capabilities. Save state |
| P46-005 | Save final state |

---

## Phase 47 — Production Docker API: Core Infrastructure & `docker run`

**Goal:** Establish the production foundation — host provisioning, real registry image pulls, all `DOCKER_HOST` connectivity modes, and validated `docker run` + env/volume support across all 6 cloud backends. This phase proves that a single container can be created, run, and managed on real cloud infrastructure.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P47-NNN.md`.

| Task | Description |
|---|---|
| P47-001 | **Host provisioning.** Terraform module for a VM (AWS EC2 / GCP GCE / Azure VM) that runs the Sockerless frontend and backend as systemd services. Cloud credentials and backend env vars configured via instance metadata or secrets manager. Save state |
| P47-002 | **Image pull from real registries.** Ensure all cloud backends can pull images from Docker Hub, GHCR, ECR, GCR, ACR, Quay. Implement registry auth forwarding (`docker login` credentials passed through to cloud image pulls). Test with common images (`alpine`, `ubuntu`, `node`, `python`, `postgres`, `redis`). Save state |
| P47-003 | **`DOCKER_HOST` connectivity modes.** Validate all three connection modes: (a) local TCP — `DOCKER_HOST=tcp://localhost:2375 docker run alpine echo hello`, (b) remote TCP — `DOCKER_HOST=tcp://remote-host:2375 docker run alpine echo hello` from a separate machine, (c) SSH — `DOCKER_HOST=ssh://user@remote-host docker run alpine echo hello` where the Docker CLI opens an SSH tunnel to the remote Sockerless frontend. Frontend must support unix socket (`/var/run/docker.sock` symlink or listen path) for the SSH case (SSH tunnels to the remote unix socket). Test all three modes on at least one cloud backend. Save state |
| P47-004 | **`docker run` end-to-end.** Validate full lifecycle on each of the 6 cloud backends: `docker run --rm alpine echo hello`, `docker run -d nginx` + `docker exec` + `docker stop`, `docker run -p 8080:80 nginx` with port access. Test via all `DOCKER_HOST` modes from P47-003. Save state |
| P47-005 | **Environment and volume mapping.** `-e` env vars, `--env-file`, `-v` bind mounts, named volumes, `--tmpfs`. Verify data persists across exec calls within a container, and that bind mounts map to real cloud storage (EFS, GCS FUSE, Azure Files). Save state |
| P47-006 | **Save final state** |

**Verification:** `DOCKER_HOST=tcp://localhost:2375 docker run --rm alpine echo hello` works on all 6 cloud backends. `DOCKER_HOST=ssh://user@remote docker run --rm alpine echo hello` works over SSH. `docker run -e FOO=bar -v /data:/data alpine` works with real cloud storage on at least 3 backends.

---

## Phase 48 — Production Docker API: Networking, Build, & Streaming

**Goal:** Add the Docker engine features that multi-service scenarios (Compose, TestContainers) depend on — container networking, image builds, log streaming, and robust error handling. Builds on the single-container foundation from Phase 47.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P48-NNN.md`.

| Task | Description |
|---|---|
| P48-001 | **Networking between containers.** User-defined bridge networks, `docker network create/connect/disconnect`, DNS resolution by container name, exposed ports. Critical for Compose and TestContainers. Save state |
| P48-002 | **Docker build on real cloud.** `docker build` produces an image that can be used in subsequent `docker run` commands. Multi-stage builds, COPY from build context, ARG/ENV propagation. Images stored in-memory or pushed to a real registry. Save state |
| P48-003 | **Streaming and logging.** `docker logs -f`, `docker attach`, `docker exec -it` (interactive TTY). Verify output is complete, ordered, and low-latency on each cloud backend. Save state |
| P48-004 | **Error handling and edge cases.** Container OOM, cloud-side timeout, image not found, permission denied, quota exceeded — all must return proper Docker API error codes so clients can handle failures gracefully. Save state |
| P48-005 | **Save final state** |

**Verification:** Two containers on the same user-defined network can resolve each other by name. `docker build -t myapp . && docker run myapp` works on at least 3 backends. `docker logs -f` streams output in real-time. Cloud-side failures return meaningful Docker API errors.

---

## Phase 49 — Production Docker API: Compose, TestContainers, & SDK Clients

**Goal:** Validate that higher-level Docker clients — Compose, TestContainers, and SDK libraries — work against Sockerless on real cloud infrastructure. Run the full validation matrix across all backends and connectivity modes.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P49-NNN.md`.

| Task | Description |
|---|---|
| P49-001 | **Docker Compose.** Validate `docker compose up/down/ps/logs/exec` for a multi-service stack (e.g., web app + database + cache). Inter-container networking via Compose service names. `depends_on` with health checks. Save state |
| P49-002 | **TestContainers integration.** Validate the TestContainers library (Go, Java, Python, Node) works against Sockerless. Test with standard modules: PostgreSQL, Redis, Kafka, LocalStack. Fix any API gaps (container wait strategies, log consumers, exposed port detection). Save state |
| P49-003 | **Docker SDK clients.** Validate programmatic usage via Go (`docker/docker` client), Python (`docker-py`), Java (`docker-java`). Run a non-trivial integration test suite (create, start, exec, logs, stop, remove) against each cloud backend. Save state |
| P49-004 | **Validation matrix.** Run the full test suite (`docker run`, Compose stack, TestContainers, SDK tests) across all 6 cloud backends, via all three `DOCKER_HOST` modes (local TCP, remote TCP, SSH). Document pass/fail/gap matrix. Save state |
| P49-005 | **Save final state** |

**Verification:** `docker compose up` with a 3-service stack (app + postgres + redis) works on ECS, Cloud Run, and ACA. TestContainers Go and Java test suites pass on at least 3 cloud backends. SDK integration tests pass across all 6 backends.

---

## Phase 50 — Production GitHub Actions: Runner Setup & Single-Job Workflows

**Goal:** Get a self-hosted GitHub Actions runner running on real cloud infrastructure via Sockerless. Validate single-job workflows end-to-end: checkout, marketplace actions, service containers, artifacts, caching, and secrets.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P50-NNN.md`.

| Task | Description |
|---|---|
| P50-001 | **Runner host provisioning.** Extend the Phase 47 VM module to also run the `actions/runner` binary alongside Sockerless. Systemd unit for the runner process. Save state |
| P50-002 | **Runner registration automation.** Script that registers the `actions/runner` with a GitHub repo as a self-hosted runner, configures `DOCKER_HOST` to point at the local Sockerless frontend, and validates connectivity. Save state |
| P50-003 | **`actions/checkout` end-to-end.** Validate that `uses: actions/checkout@v4` works: runner downloads the action tarball from github.com, executes it, clones the repo via git. Save state |
| P50-004 | **Marketplace action support.** Ensure the runner can download and execute JavaScript and Docker actions from github.com. Fix any Docker API gaps (e.g., `docker build` for Dockerfile-based actions, volume mounts for action workspaces). Save state |
| P50-005 | **Service containers.** Validate `services:` in workflow files work on real cloud backends — health checks, networking between job container and service containers, port mapping. Save state |
| P50-006 | **Artifact upload/download.** Ensure `actions/upload-artifact` and `actions/download-artifact` work. The runner talks to the GitHub artifact API (hosted by github.com) — verify artifacts survive the full upload/download round-trip. Save state |
| P50-007 | **Caching.** Validate `actions/cache` works through github.com's cache API. Save state |
| P50-008 | **Secrets and encrypted variables.** Ensure GitHub-encrypted secrets are correctly decrypted and injected into the runner environment. Validate with real repo secrets from github.com. Save state |
| P50-009 | **Save final state** |

**Verification:** A single-job GitHub Actions workflow with `actions/checkout`, a marketplace action, service containers, artifact upload/download, caching, and secrets runs successfully through a self-hosted runner backed by Sockerless on at least 3 cloud backends.

---

## Phase 51 — Production GitHub Actions: Multi-Job, Scaling, & Validation

**Goal:** Extend the self-hosted runner to support multi-job workflows, concurrent execution, and full cross-backend validation. Builds on the single-job foundation from Phase 50.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P51-NNN.md`.

| Task | Description |
|---|---|
| P51-001 | **Multi-job workflows.** Validate `needs:` dependencies, matrix strategies, and `outputs:` passing between jobs when running through a self-hosted runner pool. May require multiple runner instances. Save state |
| P51-002 | **Concurrency and queueing.** Multiple runners sharing a backend. Job queue management, resource limits, graceful scaling. Save state |
| P51-003 | **Logging and observability.** Runner logs, backend logs, and cloud workload logs unified. Structured logging with job/step correlation IDs. Save state |
| P51-004 | **Validation matrix.** Run a representative set of real-world GitHub Actions workflows (build + test for Go, Node, Python, Rust projects) across all 6 cloud backends. Document pass/fail/gap matrix. Save state |
| P51-005 | **Save final state** |

**Verification:** A public GitHub repo with standard CI workflows (build, test, lint, deploy) including multi-job `needs:` dependencies and matrix strategies runs all jobs through self-hosted runners backed by Sockerless on ECS. Workflows pass identically to GitHub-hosted runners.

---

## Phase 52 — Production GitLab CI: Runner Setup & Single-Job Pipelines

**Goal:** Get a GitLab Runner (docker executor) running on real cloud infrastructure via Sockerless. Validate single-job pipelines end-to-end: helper image, git clone, artifacts, caching, service containers, and secrets.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P52-NNN.md`.

| Task | Description |
|---|---|
| P52-001 | **Runner registration automation.** Script that registers `gitlab-runner` with a gitlab.com project, configures `host = "tcp://localhost:2375"` in `config.toml` to point at Sockerless, and validates connectivity. Save state |
| P52-002 | **Helper image compatibility.** GitLab Runner uses `gitlab/gitlab-runner-helper` for git clone, artifacts, and cache. Ensure this image works correctly through Sockerless on real cloud backends (the helper uses `docker cp`, `docker exec`, and stdin injection). Save state |
| P52-003 | **Git clone via helper.** Validate the full clone flow: runner creates helper container, starts it, uses `docker exec` to inject git credentials and clone the repo. This is the most complex Docker API interaction in the GitLab runner. Save state |
| P52-004 | **Artifact upload/download.** Validate `artifacts:paths`, `artifacts:reports`, and inter-job artifact passing. The helper container handles artifact collection via `docker cp`. Save state |
| P52-005 | **Cache support.** Validate `cache:key` and `cache:paths`. GitLab Runner uses a cache container or S3/GCS backend for caching. Ensure both paths work through Sockerless. Save state |
| P52-006 | **Service containers.** Validate `services:` in `.gitlab-ci.yml` — health checks, networking, aliases. GitLab creates service containers as separate Docker containers linked via network. Save state |
| P52-007 | **Secrets and variables.** Ensure CI/CD variables (protected, masked, file-type) are correctly passed through to containers. Validate with real gitlab.com project variables. Save state |
| P52-008 | **Save final state** |

**Verification:** A single-job GitLab CI pipeline with git clone, build, test, artifact upload, cache, service containers, and secrets runs successfully through a GitLab Runner backed by Sockerless on at least 3 cloud backends.

---

## Phase 53 — Production GitLab CI: Advanced Pipelines, Scaling, & Validation

**Goal:** Extend GitLab CI support to multi-stage pipelines, DinD, autoscaling, and full cross-backend validation. Builds on the single-job foundation from Phase 52.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P53-NNN.md`.

| Task | Description |
|---|---|
| P53-001 | **Multi-stage pipelines.** Validate complex pipeline topologies: stages, `needs:` DAG dependencies, `rules:`, `only:/except:`, `when: manual`, `allow_failure:`. Save state |
| P53-002 | **Docker-in-Docker (DinD).** Some GitLab CI jobs use `docker:dind` as a service. Determine how this interacts with Sockerless (the inner Docker daemon would need to be real or also Sockerless). Document supported patterns. Save state |
| P53-003 | **Runner autoscaling.** GitLab Runner supports autoscaling via Docker Machine or Kubernetes. Design a Sockerless-compatible autoscaling approach (multiple runner instances sharing a backend, or a runner per backend instance). Save state |
| P53-004 | **Logging and observability.** Job logs flow from Sockerless backend → cloud workload → runner → gitlab.com. Ensure log streaming works without gaps or delays. Save state |
| P53-005 | **Validation matrix.** Run a representative set of real-world GitLab CI pipelines (build + test for Go, Node, Python, Rust projects) across all 6 cloud backends. Document pass/fail/gap matrix. Save state |
| P53-006 | **Save final state** |

**Verification:** A gitlab.com project with standard CI pipelines (build, test, lint, deploy) including multi-stage `needs:` DAG dependencies runs all jobs through a self-hosted runner backed by Sockerless on Cloud Run. Pipelines pass identically to shared runners.

---

## Phase 54 — Docker API Hardening for Production

**Goal:** Fix Docker API gaps and edge cases discovered during production validation (Phases 47-53). Real-world clients will inevitably exercise API paths that simulators and local tests miss.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P54-NNN.md`.

| Task | Description |
|---|---|
| P54-001 | **Docker build production paths.** Expand `POST /build` to support multi-stage builds, build args, `.dockerignore`, layer caching, and BuildKit-style output. Many marketplace actions, GitLab CI jobs, and TestContainers modules build images. Save state |
| P54-002 | **Volume lifecycle.** Named volumes, volume mounts between containers (data sharing between Compose services, job and service containers), tmpfs mounts, bind mount permissions. Save state |
| P54-003 | **Network fidelity.** Container-to-container DNS resolution, user-defined bridge networks, network aliases, exposed ports. Critical for Compose, TestContainers, and CI service containers. Save state |
| P54-004 | **Container restart and retry.** Handle container restart policies, OOM kills, timeout-based cleanup. Cloud backends need graceful handling of cloud-side failures (ECS task failures, Lambda timeouts, etc.). Save state |
| P54-005 | **Streaming fidelity.** `docker logs --follow`, `docker attach`, `docker exec` interactive mode, TTY allocation. Ensure output is never lost or reordered. Save state |
| P54-006 | **Large file handling.** `docker cp` with large tarballs (artifacts, build contexts), image layers >1GB, long-running log streams. Test with realistic payloads. Save state |
| P54-007 | **Concurrent operations.** Multiple containers running simultaneously (Compose stacks, parallel CI jobs, service containers + job container + helper containers). Backend must handle concurrent cloud API calls without races or deadlocks. Save state |
| P54-008 | **Error propagation.** Cloud API errors (rate limits, quota exceeded, permission denied, transient failures) must be translated to meaningful Docker API error responses so clients can retry or fail gracefully. Save state |
| P54-009 | **Save final state** |

**Verification:** All production validation matrices from P49-004, P51-004, and P53-005 pass at 100%. No Docker API gaps remain for the tested client/workflow set.

---

## Phase 55 — Production Hardening and Operations

**Goal:** Make Sockerless production-ready for continuous operation: monitoring, alerting, upgrade procedures, security, and documentation.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P55-NNN.md`.

| Task | Description |
|---|---|
| P55-001 | **Health checks and readiness probes.** Frontend and backend expose `/healthz` and `/readyz` endpoints. Cloud load balancers and systemd use these for routing and restart decisions. Save state |
| P55-002 | **Metrics and monitoring.** Prometheus metrics: request latency, container lifecycle durations, cloud API call counts/errors, active containers, queue depth. Grafana dashboard template. Save state |
| P55-003 | **Alerting.** Alert rules: cloud API error rate spike, container start latency degradation, orphaned resource accumulation, backend unreachable. Save state |
| P55-004 | **Security audit.** Agent token authentication, frontend access control, secrets handling, network exposure review. Document the threat model and security boundaries. Save state |
| P55-005 | **TLS everywhere.** Frontend ↔ backend, backend ↔ agent, all cloud API calls. Certificate management (Let's Encrypt or cloud-native cert managers). Save state |
| P55-006 | **Upgrade procedures.** Zero-downtime backend upgrades (drain running jobs, switch, resume). Frontend is stateless so trivially replaceable. Document the upgrade runbook. Save state |
| P55-007 | **Cost controls.** Per-container and per-runner resource limits. Automatic cleanup of idle cloud resources. Cost alerting thresholds. Save state |
| P55-008 | **Operations guide.** Complete operations documentation: deployment, configuration, monitoring, troubleshooting, upgrade, backup/restore. Save state |
| P55-009 | **Save final state** |

**Verification:** Sockerless runs continuously for 7 days under mixed load (Docker Compose stacks, TestContainers suites, CI jobs). No leaked resources, no crashes, no silent failures. Monitoring dashboard shows all key metrics. Alert rules fire correctly on injected failures.

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
