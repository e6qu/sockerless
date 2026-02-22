# Sockerless — Roadmap

> Phases 1-38 complete (342 tasks). This document covers future work.
>
> **Production target:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners (GitHub Actions from github.com, GitLab CI from gitlab.com), and custom SDK clients — backed by real cloud infrastructure (AWS, GCP, Azure). Phases 39-46 build out the testing infrastructure and bleephub. Phase 47 achieves general Docker API production readiness. Phases 48-49 achieve production CI. Phases 50-51 harden for production operations.

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

## Phase 39 — bleephub: Issues + Labels + Milestones

**Goal:** Full issue tracking. `gh` uses GraphQL exclusively for issue CRUD and listing. Issues are also the foundation for PR metadata (labels, assignees, milestones are shared concepts).

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P39-NNN.md`.

| Task | Description |
|---|---|
| P39-001 | Issue store: per-repo sequential numbering. Fields: `id`, `nodeID`, `number`, `title`, `body`, `state` (OPEN/CLOSED), `stateReason` (COMPLETED/NOT_PLANNED), `author`, `assignees`, `labels`, `milestone`, `createdAt`, `updatedAt`, `closedAt`. Save state |
| P39-002 | Label store: per-repo. `GET/POST /api/v3/repos/{owner}/{repo}/labels`, `GET/PATCH/DELETE /api/v3/repos/{owner}/{repo}/labels/{name}`. Issue ↔ label association. Save state |
| P39-003 | Milestone store: per-repo. `GET/POST /api/v3/repos/{owner}/{repo}/milestones`, `GET/PATCH/DELETE /api/v3/repos/{owner}/{repo}/milestones/{number}`. Save state |
| P39-004 | REST issue endpoints: `GET/POST /api/v3/repos/{owner}/{repo}/issues`, `GET/PATCH /api/v3/repos/{owner}/{repo}/issues/{number}`. Filtering by state, assignee, label, milestone. Save state |
| P39-005 | Issue comments: `GET/POST /api/v3/repos/{owner}/{repo}/issues/{number}/comments`, `PATCH/DELETE /api/v3/repos/{owner}/{repo}/issues/comments/{id}`. Save state |
| P39-006 | GraphQL: `createIssue` mutation (repositoryId, title, body, assigneeIds, labelIds, milestoneId). Save state |
| P39-007 | GraphQL: `closeIssue` mutation with `stateReason` (COMPLETED/NOT_PLANNED). Feature detection query. Save state |
| P39-008 | GraphQL: issue listing — `repository.issues` connection with state/assignee/author filters + `search(type: ISSUE)` query. Save state |
| P39-009 | GraphQL: issue view — full field selection (number, url, state, title, body, author, assignees, labels, milestone, comments, reactionGroups, stateReason). Save state |
| P39-010 | Reactions: basic emoji reaction support on issues and comments (thumbsUp, thumbsDown, laugh, hooray, confused, heart, rocket, eyes). Save state |
| P39-011 | Unit tests: issue CRUD, comments, labels, milestones, GraphQL queries. Save state |
| P39-012 | `gh` CLI validation: `gh issue create`, `gh issue list`, `gh issue view`, `gh issue close` work against bleephub. Save state |
| P39-013 | Save final state |

**Verification:** Full `gh issue` lifecycle works. Filtering and search return correct results.

---

## Phase 40 — bleephub: Pull Requests

**Goal:** Pull request lifecycle — create, review, merge, close. PRs are the most complex entity, depending on git (branches, diffs, merge), issues (labels, assignees), and users (reviewers). `gh` uses GraphQL for all PR operations with REST for branch deletion and reviewer assignment.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P40-NNN.md`.

| Task | Description |
|---|---|
| P40-001 | PR store: per-repo, sequential numbering (shared with issues). Fields: `id`, `nodeID`, `number`, `title`, `body`, `state` (OPEN/CLOSED/MERGED), `draft`, `baseRefName`, `headRefName`, `headRepositoryOwner`, `isCrossRepository`, `mergeable`, `additions`, `deletions`, `commits`, `author`, `assignees`, `labels`, `milestone`, `createdAt`, `mergedAt`, `closedAt`. Save state |
| P40-002 | PR creation: validate head/base branches exist in git storage, compute diff stats (additions/deletions), create merge-check ref. Save state |
| P40-003 | PR merging: implement merge/squash/rebase strategies using `go-git`. Update default branch ref. Optionally delete head branch. Save state |
| P40-004 | GraphQL mutations: `createPullRequest`, `closePullRequest`, `mergePullRequest` (with mergeMethod MERGE/SQUASH/REBASE), `enablePullRequestAutoMerge`, `disablePullRequestAutoMerge`. Save state |
| P40-005 | GraphQL queries: PR listing via `repository.pullRequests` connection + `search(type: ISSUE)`. PR view with full field selection (state, body, author, isDraft, mergeable, additions, deletions, commits, baseRefName, headRefName, reviews, statusCheckRollup). Save state |
| P40-006 | GraphQL: merge text queries — `viewerMergeHeadlineText`, `viewerMergeBodyText`. Save state |
| P40-007 | PR reviews: `requestReviews` mutation, `pullRequest.reviews` connection, `pullRequest.reviewRequests` connection. Save state |
| P40-008 | REST supporting endpoints: `DELETE /api/v3/repos/{owner}/{repo}/git/refs/heads/{branch}` (post-merge branch delete), `POST/DELETE /api/v3/repos/{owner}/{repo}/pulls/{number}/requested_reviewers`. Save state |
| P40-009 | PR files/commits: `GET /api/v3/repos/{owner}/{repo}/pulls/{number}/files`, `GET /api/v3/repos/{owner}/{repo}/pulls/{number}/commits`. Save state |
| P40-010 | Checks/status: basic status check framework for PR mergeability (`statusCheckRollup` in GraphQL). Save state |
| P40-011 | Unit tests: PR lifecycle, merge strategies, diff stats, GraphQL queries. Save state |
| P40-012 | `gh` CLI validation: `gh pr create`, `gh pr list`, `gh pr view`, `gh pr merge`, `gh pr close` work against bleephub. Save state |
| P40-013 | Save final state |

**Verification:** Full `gh pr` lifecycle works — create from branch, review, merge (all 3 strategies), branch cleanup.

---

## Phase 41 — bleephub: API Conformance + `gh` CLI Test Suite

**Goal:** Validate bleephub against GitHub's published OpenAPI spec and build a comprehensive `gh` CLI test suite. Fix any protocol gaps discovered.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P41-NNN.md`.

| Task | Description |
|---|---|
| P41-001 | Download and parse GitHub's OpenAPI spec (`github/rest-api-description`). Build a conformance checker that validates bleephub's responses against the spec schemas. Save state |
| P41-002 | Pagination conformance: `Link` header format for REST, Relay cursor spec for GraphQL. Verify with multi-page result sets. Save state |
| P41-003 | Error format conformance: `{"message": "...", "documentation_url": "..."}` for REST, `{"errors": [...]}` for GraphQL. HTTP status codes match spec. Save state |
| P41-004 | Rate limit headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Used`, `X-RateLimit-Reset`, `X-RateLimit-Resource` on all responses. Save state |
| P41-005 | Content negotiation: `Accept: application/vnd.github+json`, `X-GitHub-Api-Version: 2022-11-28` header handling. Save state |
| P41-006 | `gh` CLI integration test suite: Docker-based, runs `gh auth login`, `gh repo create/list/view/clone`, `gh issue create/list/view/close`, `gh pr create/list/view/merge/close`, `gh org list` against bleephub. Save state |
| P41-007 | `gh api` raw endpoint testing: verify key REST endpoints return spec-compliant JSON. Save state |
| P41-008 | Fix protocol gaps discovered during conformance testing (expect iteration). Save state |
| P41-009 | Save final state |

**Verification:** `gh` CLI test suite passes. OpenAPI conformance checker reports no schema violations on exercised endpoints.

---

## Phase 42 — bleephub: Runner Enhancements (Actions + Multi-Job)

**Goal:** Expand the runner protocol to support `uses:` actions and multi-job workflows, building on the GitHub API foundation from phases 36-41.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P42-NNN.md`.

| Task | Description |
|---|---|
| P42-001 | Action resolution: serve action tarballs from upstream GitHub (cache locally). Implement `ActionDownloadInfo` with real download URLs. Save state |
| P42-002 | Multi-job workflows: `needs:` dependencies, job graph execution, result propagation. Save state |
| P42-003 | Matrix strategies: expand matrix configurations into parallel job requests. Save state |
| P42-004 | Workflow YAML parsing: accept raw workflow YAML (not just simplified JSON) as job input. Save state |
| P42-005 | Artifacts + cache API stubs: `actions/upload-artifact`, `actions/download-artifact`, `actions/cache`. Save state |
| P42-006 | Integration test: multi-step workflow with `uses:` actions, matrix, and artifacts. Save state |
| P42-007 | Save final state |

**Verification:** Workflow with `uses: actions/checkout@v4` + matrix + artifact upload/download runs through bleephub.

---

## Phase 43 — Cloud Resource Tracking

**Goal:** Identify, tag, and track all cloud resources created, modified, or used by Sockerless. Enables resource accounting, leak detection, cleanup after crashes, and operational visibility. Uses cloud-native tagging (AWS tags, GCP labels, Azure tags) plus local metadata for cross-cloud consistency.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P43-NNN.md`.

| Task | Description |
|---|---|
| P43-001 | **Research cloud tagging systems.** Document AWS resource tags, GCP labels, Azure tags — capabilities, limits, propagation rules, which resource types support them. Identify cross-cloud common denominator. Save state |
| P43-002 | **Design tagging schema.** Define standard tag set: `sockerless:instance-id`, `sockerless:backend`, `sockerless:container-id`, `sockerless:job-id`, `sockerless:created-at`. Document in `docs/resource-tracking.md`. Save state |
| P43-003 | **Local resource registry.** Create `backends/core/resources.go`: in-memory registry tracking all cloud resources by type (task, function, storage, log stream, etc.), cloud ID, tags, state, timestamps. Serializable to JSON for crash recovery. Save state |
| P43-004 | **AWS backend tagging.** Add tags to ECS tasks, Lambda functions, CloudWatch log groups/streams, S3 objects, ECR repos created by the ecs/lambda backends. Propagate through simulator API calls. Save state |
| P43-005 | **GCP backend labeling.** Add labels to Cloud Run jobs, Cloud Functions, Cloud Logging, GCS objects created by cloudrun/gcf backends. Save state |
| P43-006 | **Azure backend tagging.** Add tags to Container Apps jobs, Azure Functions, Log Analytics, Blob Storage created by aca/azf backends. Save state |
| P43-007 | **Resource listing endpoint.** `GET /api/v3/sockerless/resources` — list all tracked resources with filters (backend, type, state, age). Save state |
| P43-008 | **Resource cleanup command.** `DELETE /api/v3/sockerless/resources?older_than=1h` — clean up orphaned/stale resources. Also usable as CLI: `sockerless cleanup --older-than 1h`. Save state |
| P43-009 | **Crash recovery.** On startup, scan cloud APIs for resources matching Sockerless tags that aren't in local registry (orphans from previous crashes). Reconcile or offer cleanup. Save state |
| P43-010 | **Simulator support.** Simulators store and return tags/labels on created resources (ECS tasks, Cloud Run jobs, etc.) so tagging can be tested without real clouds. Save state |
| P43-011 | **Unit + integration tests.** Tag propagation tests per cloud, resource registry tests, cleanup tests, orphan detection tests. Save state |
| P43-012 | **Save final state** |

**Verification:** Resources created by Sockerless are identifiable via cloud-native tags/labels. `sockerless cleanup` finds and removes orphaned resources. Crash → restart → orphans detected.

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

## Phase 46 — Capability Negotiation

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

## Phase 47 — Production Docker API: CLI, Compose, TestContainers, SDK Clients

**Goal:** Make `docker run`, `docker compose`, TestContainers, and arbitrary Docker SDK clients work against Sockerless on real cloud infrastructure. This is the foundational production milestone — all other production use cases (CI runners, custom tooling) build on this.

**State save:** Every task ends with state save.

| Task | Description |
|---|---|
| P47-001 | **Host provisioning.** Terraform module for a VM (AWS EC2 / GCP GCE / Azure VM) that runs the Sockerless frontend and backend as systemd services. Cloud credentials and backend env vars configured via instance metadata or secrets manager. Save state |
| P47-002 | **Image pull from real registries.** Ensure all cloud backends can pull images from Docker Hub, GHCR, ECR, GCR, ACR, Quay. Implement registry auth forwarding (`docker login` credentials passed through to cloud image pulls). Test with common images (`alpine`, `ubuntu`, `node`, `python`, `postgres`, `redis`). Save state |
| P47-003 | **`DOCKER_HOST` connectivity modes.** Validate all three connection modes: (a) local TCP — `DOCKER_HOST=tcp://localhost:2375 docker run alpine echo hello`, (b) remote TCP — `DOCKER_HOST=tcp://remote-host:2375 docker run alpine echo hello` from a separate machine, (c) SSH — `DOCKER_HOST=ssh://user@remote-host docker run alpine echo hello` where the Docker CLI opens an SSH tunnel to the remote Sockerless frontend. Frontend must support unix socket (`/var/run/docker.sock` symlink or listen path) for the SSH case (SSH tunnels to the remote unix socket). Test all three modes on at least one cloud backend. Save state |
| P47-004 | **`docker run` end-to-end.** Validate full lifecycle on each of the 6 cloud backends: `docker run --rm alpine echo hello`, `docker run -d nginx` + `docker exec` + `docker stop`, `docker run -p 8080:80 nginx` with port access. Test via all `DOCKER_HOST` modes from P47-003. Save state |
| P47-005 | **Environment and volume mapping.** `-e` env vars, `--env-file`, `-v` bind mounts, named volumes, `--tmpfs`. Verify data persists across exec calls within a container, and that bind mounts map to real cloud storage (EFS, GCS FUSE, Azure Files). Save state |
| P47-006 | **Docker Compose.** Validate `docker compose up/down/ps/logs/exec` for a multi-service stack (e.g., web app + database + cache). Inter-container networking via Compose service names. `depends_on` with health checks. Save state |
| P47-007 | **TestContainers integration.** Validate the TestContainers library (Go, Java, Python, Node) works against Sockerless. Test with standard modules: PostgreSQL, Redis, Kafka, LocalStack. Fix any API gaps (container wait strategies, log consumers, exposed port detection). Save state |
| P47-008 | **Docker SDK clients.** Validate programmatic usage via Go (`docker/docker` client), Python (`docker-py`), Java (`docker-java`). Run a non-trivial integration test suite (create, start, exec, logs, stop, remove) against each cloud backend. Save state |
| P47-009 | **Docker build on real cloud.** `docker build` produces an image that can be used in subsequent `docker run` commands. Multi-stage builds, COPY from build context, ARG/ENV propagation. Images stored in-memory or pushed to a real registry. Save state |
| P47-010 | **Networking between containers.** User-defined bridge networks, `docker network create/connect/disconnect`, DNS resolution by container name, exposed ports. Critical for Compose and TestContainers. Save state |
| P47-011 | **Streaming and logging.** `docker logs -f`, `docker attach`, `docker exec -it` (interactive TTY). Verify output is complete, ordered, and low-latency on each cloud backend. Save state |
| P47-012 | **Error handling and edge cases.** Container OOM, cloud-side timeout, image not found, permission denied, quota exceeded — all must return proper Docker API error codes so clients can handle failures gracefully. Save state |
| P47-013 | **Validation matrix.** Run the full test suite (`docker run`, Compose stack, TestContainers, SDK tests) across all 6 cloud backends, via all three `DOCKER_HOST` modes (local TCP, remote TCP, SSH). Document pass/fail/gap matrix. Save state |
| P47-014 | **Save final state** |

**Verification:** `DOCKER_HOST=tcp://localhost:2375 docker run --rm alpine echo hello` works on all 6 cloud backends. `DOCKER_HOST=ssh://user@remote docker run --rm alpine echo hello` works over SSH. `docker compose up` with a 3-service stack (app + postgres + redis) works on ECS, Cloud Run, and ACA. TestContainers Go and Java test suites pass on at least 3 cloud backends.

---

## Phase 48 — Production GitHub Actions: Self-Hosted Runner on Real Cloud

**Goal:** Run real GitHub Actions workflows from github.com through a self-hosted runner backed by Sockerless on real cloud infrastructure.

**State save:** Every task ends with state save.

| Task | Description |
|---|---|
| P48-001 | **Runner host provisioning.** Extend the Phase 47 VM module to also run the `actions/runner` binary alongside Sockerless. Systemd unit for the runner process. Save state |
| P48-002 | **Runner registration automation.** Script that registers the `actions/runner` with a GitHub repo as a self-hosted runner, configures `DOCKER_HOST` to point at the local Sockerless frontend, and validates connectivity. Save state |
| P48-003 | **`actions/checkout` end-to-end.** Validate that `uses: actions/checkout@v4` works: runner downloads the action tarball from github.com, executes it, clones the repo via git. Save state |
| P48-004 | **Marketplace action support.** Ensure the runner can download and execute JavaScript and Docker actions from github.com. Fix any Docker API gaps (e.g., `docker build` for Dockerfile-based actions, volume mounts for action workspaces). Save state |
| P48-005 | **Service containers.** Validate `services:` in workflow files work on real cloud backends — health checks, networking between job container and service containers, port mapping. Save state |
| P48-006 | **Artifact upload/download.** Ensure `actions/upload-artifact` and `actions/download-artifact` work. The runner talks to the GitHub artifact API (hosted by github.com) — verify artifacts survive the full upload/download round-trip. Save state |
| P48-007 | **Caching.** Validate `actions/cache` works through github.com's cache API. Save state |
| P48-008 | **Secrets and encrypted variables.** Ensure GitHub-encrypted secrets are correctly decrypted and injected into the runner environment. Validate with real repo secrets from github.com. Save state |
| P48-009 | **Multi-job workflows.** Validate `needs:` dependencies, matrix strategies, and `outputs:` passing between jobs when running through a self-hosted runner pool. May require multiple runner instances. Save state |
| P48-010 | **Concurrency and queueing.** Multiple runners sharing a backend. Job queue management, resource limits, graceful scaling. Save state |
| P48-011 | **Logging and observability.** Runner logs, backend logs, and cloud workload logs unified. Structured logging with job/step correlation IDs. Save state |
| P48-012 | **Validation matrix.** Run a representative set of real-world GitHub Actions workflows (build + test for Go, Node, Python, Rust projects) across all 6 cloud backends. Document pass/fail/gap matrix. Save state |
| P48-013 | **Save final state** |

**Verification:** A public GitHub repo with standard CI workflows (build, test, lint, deploy) runs all jobs through a self-hosted runner backed by Sockerless on ECS. Workflows pass identically to GitHub-hosted runners.

---

## Phase 49 — Production GitLab CI: Runner on Real Cloud

**Goal:** Run real GitLab CI pipelines from gitlab.com through a GitLab Runner (docker executor) backed by Sockerless on real cloud infrastructure.

**State save:** Every task ends with state save.

| Task | Description |
|---|---|
| P49-001 | **Runner registration automation.** Script that registers `gitlab-runner` with a gitlab.com project, configures `host = "tcp://localhost:2375"` in `config.toml` to point at Sockerless, and validates connectivity. Save state |
| P49-002 | **Helper image compatibility.** GitLab Runner uses `gitlab/gitlab-runner-helper` for git clone, artifacts, and cache. Ensure this image works correctly through Sockerless on real cloud backends (the helper uses `docker cp`, `docker exec`, and stdin injection). Save state |
| P49-003 | **Git clone via helper.** Validate the full clone flow: runner creates helper container, starts it, uses `docker exec` to inject git credentials and clone the repo. This is the most complex Docker API interaction in the GitLab runner. Save state |
| P49-004 | **Artifact upload/download.** Validate `artifacts:paths`, `artifacts:reports`, and inter-job artifact passing. The helper container handles artifact collection via `docker cp`. Save state |
| P49-005 | **Cache support.** Validate `cache:key` and `cache:paths`. GitLab Runner uses a cache container or S3/GCS backend for caching. Ensure both paths work through Sockerless. Save state |
| P49-006 | **Service containers.** Validate `services:` in `.gitlab-ci.yml` — health checks, networking, aliases. GitLab creates service containers as separate Docker containers linked via network. Save state |
| P49-007 | **Multi-stage pipelines.** Validate complex pipeline topologies: stages, `needs:` DAG dependencies, `rules:`, `only:/except:`, `when: manual`, `allow_failure:`. Save state |
| P49-008 | **Docker-in-Docker (DinD).** Some GitLab CI jobs use `docker:dind` as a service. Determine how this interacts with Sockerless (the inner Docker daemon would need to be real or also Sockerless). Document supported patterns. Save state |
| P49-009 | **Secrets and variables.** Ensure CI/CD variables (protected, masked, file-type) are correctly passed through to containers. Validate with real gitlab.com project variables. Save state |
| P49-010 | **Runner autoscaling.** GitLab Runner supports autoscaling via Docker Machine or Kubernetes. Design a Sockerless-compatible autoscaling approach (multiple runner instances sharing a backend, or a runner per backend instance). Save state |
| P49-011 | **Logging and observability.** Job logs flow from Sockerless backend → cloud workload → runner → gitlab.com. Ensure log streaming works without gaps or delays. Save state |
| P49-012 | **Validation matrix.** Run a representative set of real-world GitLab CI pipelines (build + test for Go, Node, Python, Rust projects) across all 6 cloud backends. Document pass/fail/gap matrix. Save state |
| P49-013 | **Save final state** |

**Verification:** A gitlab.com project with standard CI pipelines (build, test, lint, deploy) runs all jobs through a self-hosted runner backed by Sockerless on Cloud Run. Pipelines pass identically to shared runners.

---

## Phase 50 — Docker API Hardening for Production

**Goal:** Fix Docker API gaps and edge cases discovered during production validation (Phases 47-49). Real-world clients will inevitably exercise API paths that simulators and local tests miss.

**State save:** Every task ends with state save.

| Task | Description |
|---|---|
| P50-001 | **Docker build production paths.** Expand `POST /build` to support multi-stage builds, build args, `.dockerignore`, layer caching, and BuildKit-style output. Many marketplace actions, GitLab CI jobs, and TestContainers modules build images. Save state |
| P50-002 | **Volume lifecycle.** Named volumes, volume mounts between containers (data sharing between Compose services, job and service containers), tmpfs mounts, bind mount permissions. Save state |
| P50-003 | **Network fidelity.** Container-to-container DNS resolution, user-defined bridge networks, network aliases, exposed ports. Critical for Compose, TestContainers, and CI service containers. Save state |
| P50-004 | **Container restart and retry.** Handle container restart policies, OOM kills, timeout-based cleanup. Cloud backends need graceful handling of cloud-side failures (ECS task failures, Lambda timeouts, etc.). Save state |
| P50-005 | **Streaming fidelity.** `docker logs --follow`, `docker attach`, `docker exec` interactive mode, TTY allocation. Ensure output is never lost or reordered. Save state |
| P50-006 | **Large file handling.** `docker cp` with large tarballs (artifacts, build contexts), image layers >1GB, long-running log streams. Test with realistic payloads. Save state |
| P50-007 | **Concurrent operations.** Multiple containers running simultaneously (Compose stacks, parallel CI jobs, service containers + job container + helper containers). Backend must handle concurrent cloud API calls without races or deadlocks. Save state |
| P50-008 | **Error propagation.** Cloud API errors (rate limits, quota exceeded, permission denied, transient failures) must be translated to meaningful Docker API error responses so clients can retry or fail gracefully. Save state |
| P50-009 | **Save final state** |

**Verification:** All production validation matrices from P47-013, P48-012, and P49-012 pass at 100%. No Docker API gaps remain for the tested client/workflow set.

---

## Phase 51 — Production Hardening and Operations

**Goal:** Make Sockerless production-ready for continuous operation: monitoring, alerting, upgrade procedures, security, and documentation.

**State save:** Every task ends with state save.

| Task | Description |
|---|---|
| P51-001 | **Health checks and readiness probes.** Frontend and backend expose `/healthz` and `/readyz` endpoints. Cloud load balancers and systemd use these for routing and restart decisions. Save state |
| P51-002 | **Metrics and monitoring.** Prometheus metrics: request latency, container lifecycle durations, cloud API call counts/errors, active containers, queue depth. Grafana dashboard template. Save state |
| P51-003 | **Alerting.** Alert rules: cloud API error rate spike, container start latency degradation, orphaned resource accumulation, backend unreachable. Save state |
| P51-004 | **Security audit.** Agent token authentication, frontend access control, secrets handling, network exposure review. Document the threat model and security boundaries. Save state |
| P51-005 | **TLS everywhere.** Frontend ↔ backend, backend ↔ agent, all cloud API calls. Certificate management (Let's Encrypt or cloud-native cert managers). Save state |
| P51-006 | **Upgrade procedures.** Zero-downtime backend upgrades (drain running jobs, switch, resume). Frontend is stateless so trivially replaceable. Document the upgrade runbook. Save state |
| P51-007 | **Cost controls.** Per-container and per-runner resource limits. Automatic cleanup of idle cloud resources. Cost alerting thresholds. Save state |
| P51-008 | **Operations guide.** Complete operations documentation: deployment, configuration, monitoring, troubleshooting, upgrade, backup/restore. Save state |
| P51-009 | **Save final state** |

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
