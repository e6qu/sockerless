# Sockerless — Roadmap

> Phases 1-37 complete (332 tasks). This document covers future work.

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

## Phase 38 — bleephub: Organizations + Teams + RBAC

**Goal:** Organization accounts, team management, and role-based access control. `gh org list` uses GraphQL. Org membership determines repo visibility and write access.

**State save:** Every task ends with: update `PLAN.md` (mark done), `STATUS.md`, `WHAT_WE_DID.md` (append), `MEMORY.md` (learnings), `_tasks/done/P38-NNN.md`.

| Task | Description |
|---|---|
| P38-001 | Organization store: in-memory org CRUD. Fields: `id`, `nodeID`, `login`, `name`, `description`, `email`, `members`, `teams`. Org is an account type (like user) that owns repos. Save state |
| P38-002 | Membership: org owners, members, outside collaborators. `GET /api/v3/orgs/{org}/members`, `PUT/DELETE /api/v3/orgs/{org}/memberships/{username}`. Save state |
| P38-003 | Teams: `GET/POST /api/v3/orgs/{org}/teams`, `GET/PATCH/DELETE /api/v3/orgs/{org}/teams/{slug}`. Team membership endpoints. Team repo permissions. Save state |
| P38-004 | RBAC enforcement: repo visibility (public/private/internal), org-level roles, team-level permissions (pull/push/admin). Check permissions on repo operations. Save state |
| P38-005 | REST endpoints: `GET /api/v3/orgs/{org}`, `GET /api/v3/user/orgs`, `GET /api/v3/users/{username}/orgs`. Save state |
| P38-006 | GraphQL: `user.organizations` connection (for `gh org list`), `organization(login)` query, `organization.teams` connection. Save state |
| P38-007 | Collaborator endpoints: `GET/PUT/DELETE /api/v3/repos/{owner}/{repo}/collaborators/{username}`, permission levels. Save state |
| P38-008 | Unit tests: org/team CRUD, membership, RBAC enforcement. Save state |
| P38-009 | `gh` CLI validation: `gh org list` works. Repo operations respect org permissions. Save state |
| P38-010 | Save final state |

**Verification:** `gh org list` returns orgs. Creating repos under orgs works. RBAC prevents unauthorized access.

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

## Future Ideas (Not Scheduled)

- **WASI Preview 2** — component model with async I/O; would enable real subprocesses in WASM sandbox
- **Real network isolation** — network namespaces on Linux for true container networking
- **OpenTelemetry tracing** — distributed tracing across frontend → backend → simulator → agent
- **Multi-tenant mode** — backend pools with scheduling and resource limits
- **ARM/multi-arch support** — WASM is arch-independent; agent and simulators may need cross-compilation
- **Webhooks** — bleephub sends webhook events on push, PR, issue changes (enables CI trigger testing)
- **GitHub Apps** — app installation, JWT auth, installation tokens (enables testing GitHub App-based workflows)
- **GraphQL subscriptions** — real-time event streaming for live PR/issue updates
