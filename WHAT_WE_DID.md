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

**3 simulators** (`simulators/{aws,gcp,azure}/`) implement enough cloud API surface for the backends to work — task scheduling, function invocation, container orchestration, storage, IAM, networking. Each is tested against the real SDK, CLI, and Terraform provider for that cloud.

## What Works

### Fully Passing Tests

| Test Suite | Count | What It Proves |
|---|---|---|
| Cloud SDK tests (AWS+GCP+Azure) | 50 | Simulators match real cloud SDK behavior |
| Cloud CLI tests (AWS+GCP+Azure) | 50 | Simulators match real cloud CLI behavior |
| Terraform integration (6 modules) | 75 | Full apply/destroy against simulators |
| Simulator-backend integration | 129 | Backends correctly orchestrate simulators |
| GitHub E2E (22 workflows × 7 backends) | 154 | Real `act` runner exercises full stack |
| GitLab E2E (17 pipelines × 7 backends) | 119 | Real `gitlab-runner` exercises full stack |
| Upstream gitlab-ci-local (25 tests × 7) | 175 | Unmodified external test suite passes |
| Sandbox unit tests | 46 | WASM shell, builtins (21), filesystem, stdin |
| Lint (13 Go modules) | clean | Code quality across entire codebase |

### Upstream Act Compatibility

Running `act`'s own unmodified test suite (`TestRunEvent`) against Sockerless:

| Backend | PASS | FAIL | Notes |
|---|---|---|---|
| memory | 91 | 24 | WASM sandbox, no real bash/node |
| ecs | 56 | 31 | Real containers via ECS simulator |
| lambda | 57 | 30 | FaaS via Lambda simulator |
| cloudrun | 54 | 33 | Real containers via Cloud Run simulator |
| gcf | 58 | 29 | FaaS via GCF simulator |
| aca | 58 | 29 | Real containers via ACA simulator |
| azf | 69 | 16 | FaaS via AZF simulator (best: real bash + test isolation) |

Remaining failures are mostly: missing `node`/`python` in WASM sandbox (16), service containers/networking (4), `POST /build` (2), and edge cases (2). All bash/shell tests now pass after Phase 31.

## Key Technical Decisions

1. **WASM sandbox** (wazero + mvdan.cc/sh + go-busybox): Pure Go, no external dependencies, runs shell scripts with 21 Go builtins + busybox applets. No fork/exec — everything interpreted.

2. **Agent bridge**: Forward agents (backend dials container) for persistent backends (ECS, Cloud Run, ACA). Reverse agents (container dials backend) for FaaS (Lambda, GCF, AZF) where the function invocation is ephemeral.

3. **Pre-start archive staging**: `docker cp` before `docker start` extracts to a temp staging directory, merged into the process root at start. Required by `gitlab-ci-local` and `gitlab-runner`.

4. **Per-cloud Docker images**: Test images build only the components needed for one cloud (1 simulator + 2 backends) instead of everything, cutting build size ~50%.

5. **Synthetic fallback**: When no real execution is possible (e.g., gitlab-runner helper binaries in WASM), commands are echoed and exit code 0 returned. This is a last resort, not the default.

## Phase 31 — Enhanced WASM Sandbox

Fixed `$(pwd)` returning host paths instead of container-relative paths in the WASM shell. Root cause: mvdan.cc/sh `pwd` is a shell builtin that reads the `PWD` variable, which was set to the host temp directory. Fix: prepend `PWD=<container-path>` shell assignment in `runShellInDir()`.

Added 12 new Go-implemented builtins: `touch`, `base64`, `basename`, `dirname`, `which`, `seq`, `readlink`, `tee`, `ln`, `stat`, `sha256sum`, `md5sum`. Each implemented as a standalone function in `sandbox/builtins.go`. Total builtins now: 21 (9 original + 12 new).

Added 16 unit tests (4 for pwd fix, 12 for new builtins), bringing sandbox test count from 30 to 46.

## Phase 32 — Driver Interface Completion & Code Splitting

Pure refactoring phase with zero behavior changes. Two goals achieved:

**Part A — Eliminated all driver bypasses.** Added 6 new methods across driver interfaces:
- `WaitCh(containerID)`, `Top(containerID)`, `Stats(containerID)`, `IsSynthetic(containerID)` on `ProcessLifecycleDriver`
- `RootPath(containerID)` on `FilesystemDriver`
- `LogBytes(containerID)` on `StreamDriver`

Removed 8 handler bypasses where code checked `s.ProcessFactory != nil` or called `s.Store.Processes.Load()` directly. Handlers now call only through driver interfaces. Moved process wait-and-stop goroutine into `WASMProcessLifecycleDriver.Start()`.

**Part B — Split 4 large files for LLM editability.** Every source file now under 400 lines:
- `handle_containers.go` (829 lines) → 3 files: lifecycle (357), query (159), archive (314)
- `sandbox/shell.go` (602 lines) → 3 files: core (152), exec handler (295), helpers (179)
- `sandbox/builtins.go` (597 lines) → 4 files: dispatch (33), fs (304), text (123), system (167)
- `frontends/docker/containers.go` (419 lines) → 2 files: lifecycle (207), streaming (218)

Extracted `buildContainerFromConfig()` helper from `handleContainerCreate`.

## Phase 33 — Service Container Support

Added Docker API fidelity for service container orchestration — the #2 failure category in upstream act tests (4 tests).

**Health Check Infrastructure.** Created `backends/core/health.go` with periodic exec-based health checking. When a container has `Config.Healthcheck`, `StartHealthCheck()` spawns a goroutine that:
- Initializes `State.Health.Status = "starting"`
- Runs the health check command via the exec driver on a configurable interval
- Transitions to `"healthy"` on success or `"unhealthy"` after N consecutive failures
- Caps health log at 5 entries, cancels on container stop/kill/remove

This fixes the infinite-polling hang where `act` polls `State.Health.Status` for service containers with explicit health checks.

**NetworkingConfig Processing.** Container create now processes `NetworkingConfig.EndpointsConfig`:
- Resolves each network via store for correct IPAM (gateway, IP range)
- Copies aliases from the request endpoint config
- Adds the container to `Network.Containers` map at create time (was only done on explicit connect)
- Fixed hardcoded `172.17.0.1` gateway — now reads from actual network IPAM

**Port Reporting.** Container list and inspect now populate port information from `HostConfig.PortBindings` and `Config.ExposedPorts`.

Added 6 health check unit tests. All existing tests pass with no regression.

## Phase 34 — Docker Build Endpoint

Implemented `POST /build` — the Docker image build endpoint that was previously returning 501. This unblocks Dockerfile-based GitHub Actions (e.g., `local-action-dockerfile` upstream act test).

**Dockerfile Parser.** Created `backends/core/build.go` with a line-by-line parser supporting: FROM (multi-stage), COPY, ADD, ENV (key=value and space forms), CMD, ENTRYPOINT (JSON and shell forms), WORKDIR, ARG (with defaults and build arg overrides), LABEL, EXPOSE, USER. Line continuations and comments handled. RUN instructions are echoed in build output but not executed (sufficient for CI Dockerfile patterns).

**Build Handler.** `handleImageBuild` extracts the tar build context, parses the Dockerfile, resolves base image config from store, merges parsed config (ENV appended, CMD/ENTRYPOINT/WORKDIR overridden), generates an image with correct config, and stages COPY files for injection into containers.

**Pull Guard.** Added early return in `handleImagePull` — if an image already exists in the store (e.g., from a build), return "up to date" without overwriting. This prevents `act`'s post-build pull from destroying the correct ENTRYPOINT/CMD/ENV.

**Build Context Injection.** COPY files from the build context are staged in a temp directory at their destination paths. On container create, if the image has staged files, they're loaded into `StagingDirs` and merged into the container filesystem on start (reusing Phase 25's pre-start archive staging).

**Frontend.** Added `postRawWithQuery` to BackendClient. Replaced `handleNotImplemented` for `POST /build` with `handleImageBuild` that proxies the tar body and query params to the backend.

15 parser unit tests, system test updated from 501→200. All tests pass.

## Phase 35 — Official GitHub Actions Runner (bleephub)

Phase 27 concluded the official `actions/runner` was impractical without a GitHub server. Phase 35 solved this by building `bleephub/`, a Go module that implements enough of the GitHub Actions internal service API (Azure DevOps-derived) for the official C# runner binary to register, receive jobs, execute them through Sockerless, and report completion.

**What bleephub implements:**

1. **Auth service** (`/_services/vstoken/`) — JWT exchange (`alg: "none"`), connection data with service GUIDs
2. **Agent service** (`/_services/distributedtask/`) — Runner registration, agent pools, credential generation
3. **Broker service** — Session management, 30-second message long-poll for job dispatch
4. **Run service** (`/_services/pipelines/`) — Job acquire/renew/complete lifecycle
5. **Timeline + logs** — Step status tracking (pending → in_progress → completed), log upload
6. **Job message builder** — Converts simplified JSON to the runner's PipelineContextData + TemplateToken format

**Key protocol discoveries:**
- TemplateToken serialization: `{"type": 0, "lit": "value"}` for strings, `{"type": 2, "map": [...]}` for mappings
- MappingToken entries: `{"Key": <token>, "Value": <token>}` (Newtonsoft.Json KeyValuePair)
- Step type `"action"` with `reference: {"type": "script"}` for `run:` steps
- PipelineContextData dictionaries: `{"t": 2, "d": [{"k": "key", "v": "value"}]}`
- Runner strips non-standard ports from URLs — bleephub must run on port 80

**Sandbox fixes discovered during integration:**
- `tail -f /dev/null` keepalive: WASI has no inotify, so busybox tail exits immediately. Added `isTailDevNull()` to block on context instead.
- Host path bind mounts: `resolveBindMounts` only handled named volumes. Added host path passthrough.
- Overlapping mount symlinks: Runner creates overlapping mounts (`/__w` and `/__w/_temp`). Sort shortest-first and skip sub-path symlinks.
- `/dev/null` in rootfs: Added empty file so containers can reference it.

**Docker test:** `make bleephub-test` builds a Docker image with bleephub + Sockerless memory backend + Docker frontend + official runner binary (v2.321.0), runs the integration test.

## Phase 36 — bleephub: Users, Auth + GraphQL Engine

Added GitHub API support to bleephub: user accounts, token authentication, OAuth device flow, and a real GraphQL execution engine. This is the foundation for all subsequent GitHub API features (`gh` uses GraphQL for most operations).

**Key components:**
- `store.go` — User, Token, DeviceCode types with seed admin user and `bph_`-prefixed PATs
- `gh_middleware.go` — Injects GitHub-compatible response headers (`X-OAuth-Scopes`, `X-RateLimit-*`, etc.) on `/api/` routes; extracts authenticated user from `Authorization: token {pat}` header
- `gh_rest.go` — REST endpoints: `/api/v3/` (API root), `/api/v3/user`, `/api/v3/users/{username}`, `/api/v3/rate_limit`
- `gh_oauth.go` — Device authorization flow: `/login/device/code`, `/login/oauth/access_token` (auto-approved)
- `gh_graphql.go` — GraphQL engine using `graphql-go/graphql` library; User type, `viewer` resolver, built-in introspection
- `server.go` — TLS support via `BPH_TLS_CERT`/`BPH_TLS_KEY` env vars

**Learnings:**
- `gh` CLI rejects hostnames with ports — GHES instances must run on standard ports (443 for HTTPS)
- `gh auth login --with-token` always uses HTTPS for custom hostnames — no HTTP override
- `gh api` supports full URLs (`http://localhost:5556/api/v3/user`) bypassing hostname restrictions
- `graphql-go/graphql` provides built-in introspection and field selection with zero external dependencies
- GraphQL resolvers receive data as `map[string]interface{}` — camelCase keys must match field names, not Go struct tags

**Test results:** 19 unit tests pass (7 existing + 12 new), 0 lint issues in new code.

## Phase 37 — bleephub: Git Repositories

Added in-memory git repository hosting to bleephub using `go-git/go-git/v5` with memory storage. Repos are the central entity everything else references.

**New files (6):**
- `store_repos.go` — Repo type, CRUD methods, go-git bare repo initialization
- `gh_repos_rest.go` — REST endpoints: create/get/update/delete/list repos
- `git_http.go` — Smart HTTP git protocol: info/refs, upload-pack, receive-pack
- `gh_repos_refs.go` — Branch/ref REST endpoints
- `gh_repos_objects.go` — Commits, trees, blobs, README, contents endpoints
- `gh_repos_graphql.go` — GraphQL types, Relay pagination, mutations

**Modified files (3):**
- `store.go` — Added Repos, ReposByName, GitStorages maps to Store
- `gh_graphql.go` — Wired repo fields/mutations into schema
- `server.go` — Registered new routes, git handling in catch-all

**Key design decisions:**
- Git routes handled in catch-all instead of ServeMux wildcards (avoids conflict with `/api/v3/` pattern)
- HEAD symbolic ref updated after push when target branch doesn't exist (git.Init creates HEAD→master)
- Relay cursor pagination with base64-encoded `cursor:N` format
- GraphQL `repositories` field added to `User` type (not separate `RepositoryOwner`) — matches `gh repo list` queries

**Learnings:**
- Go 1.22+ ServeMux pattern `/{owner}/{repo}/info/refs` conflicts with `/api/v3/` — wildcards and trailing-slash routes overlap
- `go-git` `git.Init(storage, nil)` creates HEAD→refs/heads/master, but pushed branch may be `main` — must check if HEAD target exists and update
- `go-git` `memory.Storage` doesn't have a `SetObjectStorage` method (removed from API)
- `gh api` with full URLs bypasses hostname/port restrictions — ideal for testing

**Test results:** 33 unit tests pass (19 existing + 14 new), 0 lint issues in new code. All `gh api` + `git push`/`git clone` work.

## Phase 38 — bleephub: Organizations + Teams + RBAC

Added organization accounts, team management, memberships, and role-based access control to bleephub.

**New files (7):**
- `store_orgs.go` (~290 lines) — Org, Membership, Team types + all CRUD methods (18 store methods)
- `gh_orgs_rest.go` (~190 lines) — REST: org create/get/update/delete/list, org repo create
- `gh_teams_rest.go` (~180 lines) — REST: team CRUD endpoints
- `gh_members_rest.go` (~250 lines) — REST: membership + team member/repo management
- `rbac.go` (~105 lines) — Permission checking: canReadRepo, canWriteRepo, canAdminRepo, canAdminOrg
- `gh_orgs_graphql.go` (~150 lines) — GraphQL: Organization type, viewer.organizations, organization query
- `gh_orgs_test.go` (~310 lines) — 18 unit tests

**Modified files (4):**
- `store.go` — Added Orgs/OrgsByLogin/Teams/TeamsBySlug/Memberships maps + NextOrg/NextTeam counters
- `server.go` — Wired `registerGHOrgRoutes()` into route registration
- `gh_graphql.go` — Called `addOrgFieldsToSchema()` in schema init
- `gh_repos_rest.go` — Replaced owner-only checks with RBAC-based `canAdminRepo()`

**Endpoints added:**
- `POST/GET /api/v3/user/orgs` — create and list user's orgs
- `GET/PATCH/DELETE /api/v3/orgs/{org}` — org CRUD
- `GET /api/v3/users/{username}/orgs` — list user's orgs
- `POST /api/v3/orgs/{org}/repos` — create org-owned repo
- `POST/GET /api/v3/orgs/{org}/teams` — team create/list
- `GET/PATCH/DELETE /api/v3/orgs/{org}/teams/{slug}` — team CRUD
- `GET/PUT/DELETE /api/v3/orgs/{org}/memberships/{username}` — membership management
- `GET/PUT/DELETE /api/v3/orgs/{org}/teams/{slug}/memberships/{username}` — team members
- `PUT/DELETE /api/v3/orgs/{org}/teams/{slug}/repos/{owner}/{repo}` — team repo access
- GraphQL: `viewer { organizations(first, after) { nodes, totalCount, pageInfo } }`
- GraphQL: `organization(login) { login, name, description, ... }`

**Design decisions:**
- `graphql-go` requires unique type names — used `OrgPageInfo` to avoid conflict with repo's `PageInfo`
- RBAC is permission-level based: pull < push < admin; teams grant minimum permission level on assigned repos
- Org creator is auto-added as admin member
- Team slugs are auto-generated from team name (lowercased, spaces → hyphens)

**Test results:** 51 unit tests pass (33 existing + 18 new), 0 lint issues in new code.

## Documentation Overhaul (Post Phase 38)

Comprehensive restructuring and correction of project documentation:

1. **Removed `DEPLOYMENT.md`** (812 lines). Redistributed all content: state backend bootstrap commands and CI/CD workflow examples moved to `terraform/README.md`; terraform output → env var mapping tables added to each of the 6 cloud backend READMEs; root `README.md` updated with direct pointers to child docs. Fixed dangling references in `docs/GITHUB_RUNNER.md` and `docs/GITLAB_RUNNER_DOCKER.md`.

2. **Terraform/Terragrunt audit.** Fixed stale "LocalStack" references (project uses custom simulators in `simulators/{aws,gcp,azure}/`). Updated environment matrix, prerequisites, and comments in `terraform/README.md` and `terraform/environments/lambda/simulator/terragrunt.hcl`.

3. **ARCHITECTURE.md rewrite.** Fixed backend count (7→8, added docker passthrough). Added bleephub section with sequence diagram. Added production use cases section covering Docker CLI/Compose, TestContainers/SDK, CI runners (GitHub Actions + GitLab CI) with production sequence diagrams. Documented all three `DOCKER_HOST` connection modes (local TCP, remote TCP, SSH tunnel). Updated module structure and test architecture diagrams.

4. **Production milestones.** Added Phases 47-51 to `PLAN.md`: Phase 47 (general Docker API production readiness including `DOCKER_HOST` connectivity), Phase 48 (production GitHub Actions), Phase 49 (production GitLab CI), Phase 50 (Docker API hardening), Phase 51 (production operations).

## Phase 39 — bleephub: Issues + Labels + Milestones

Added full issue tracking to bleephub with per-repo sequential numbering, label/milestone management, comments, and reactions. Both REST and GraphQL APIs implemented — `gh issue create/list/view/close` uses GraphQL exclusively, while labels and milestones use REST.

**New files (5):**
- `store_issues.go` (~300 lines) — IssueLabel, Milestone, Issue, Comment types + 18 CRUD methods
- `gh_labels_rest.go` (~290 lines) — REST: label + milestone CRUD, route registration
- `gh_issues_rest.go` (~540 lines) — REST: issue + comment endpoints, issue-label management
- `gh_issues_graphql.go` (~650 lines) — GraphQL: Issue types, 5 mutations, repository queries, pagination, static reactions
- `gh_issues_test.go` (~580 lines) — 28 unit tests covering REST + GraphQL

**Modified files (5):**
- `store.go` — Added Issues/Labels/Milestones/Comments maps + counters
- `store_repos.go` — Added NextIssueNumber, NextMilestoneNumber to Repo
- `store_orgs.go` — Initialize issue/milestone counters in CreateOrgRepo
- `gh_repos_graphql.go` — Return (repoType, mutationType) tuple
- `gh_graphql.go` — Wire issue schema, pass repoType to addIssueFieldsToSchema

**Key design decisions:**
- Named type `IssueLabel` to avoid collision with existing agent `Label` type in store.go
- GraphQL enum types (IssueState, IssueClosedStateReason, MilestoneState) required for `gh` CLI compatibility — `gh` sends `states:[OPEN]` as enum values, not quoted strings
- Static reaction groups (8 types with zero counts) satisfy `gh issue view` without needing a reaction store
- Careful mutex handling: no nested locks in issueToJSON (counts comments inline) and handleAddIssueLabels (resolves labels before UpdateIssue)

**Test results:** 79 unit tests pass (51 existing + 28 new), go vet clean.

## Phase 40 — bleephub: Pull Requests (Feb 2026)

Added full pull request lifecycle to bleephub — create, list, view, update, close, reopen, merge, reviews.

**New files:**
- `bleephub/store_pulls.go` (~170 lines) — PullRequest + PullRequestReview types + 7 CRUD methods
- `bleephub/gh_pulls_rest.go` (~380 lines) — Route registration + 8 handlers + JSON converters
- `bleephub/gh_pulls_graphql.go` (~570 lines) — GraphQL types, enums, queries, mutations, converters
- `bleephub/gh_pulls_test.go` (~530 lines) — 14 REST + 14 GraphQL unit tests

**Modified files:**
- `bleephub/store.go` (+8 lines) — PullRequests/PRReviews maps + NextPR/NextPRReview counters
- `bleephub/gh_graphql.go` (+2 lines) — Wire addPullRequestFieldsToSchema()
- `bleephub/gh_issues_graphql.go` (+2 lines) — Return issueType from addIssueFieldsToSchema
- `bleephub/server.go` (+1 line) — Register pull routes

**Key design decisions:**
- Shared issue/PR numbering via existing `repo.NextIssueNumber` — no schema changes needed
- PR-prefixed GraphQL types (`PRLabel`, `PRLabelConnection`, `PRAssigneePageInfo`, etc.) to avoid graphql-go duplicate name panics
- REST merged state: internal "MERGED" → REST `state: "closed", merged: true`
- Review decision derived from reviews: APPROVED if any approved + none requesting changes, CHANGES_REQUESTED if any requesting changes
- StatusCheckRollup stub (empty contexts) satisfies `gh pr view` without needing a full CI status system
- Deadlock avoidance: `pullRequestToJSON` and `pullRequestToGQL` resolve all entities + count reviews under a single RLock

**Test results:** 107 unit tests pass (79 existing + 28 new), go vet clean.

## Phase 41 — bleephub: API Conformance + `gh` CLI Test Suite (Feb 2026)

Closed GitHub API protocol gaps and added a comprehensive conformance safety net — REST pagination, error format, content negotiation, OpenAPI schema validation, and a Docker-based `gh` CLI integration test.

**New files (7):**
- `bleephub/gh_pagination.go` (~90 lines) — Generic REST pagination with RFC 5988 Link headers
- `bleephub/gh_pagination_test.go` (~170 lines) — 7 pagination tests
- `bleephub/gh_conformance_test.go` (~300 lines) — 23 tests: error format, charset, rate limits, cross-endpoint REST↔GraphQL consistency
- `bleephub/gh_openapi_test.go` (~250 lines) — 11 OpenAPI schema validation tests for 8 resource types
- `bleephub/testdata/openapi-schemas.json` (~150 lines) — Vendored GitHub OpenAPI response schemas
- `bleephub/test/run-gh-test.sh` (~280 lines) — `gh` CLI integration test (35 assertions via `gh api`)
- `bleephub/Dockerfile.gh-test` (~20 lines) — Docker image for gh CLI test

**Modified files (11):**
- `bleephub/gh_rest.go` — Added `writeGHValidationError` for GitHub-standard 422 errors
- `bleephub/gh_middleware.go` — Content-Type charset upgrade (`application/json; charset=utf-8`)
- `bleephub/gh_repos_rest.go` — Pagination in list handlers, `permissions` object in `repoToJSON`, validation error format
- `bleephub/gh_issues_rest.go` — Pagination, validation error format
- `bleephub/gh_pulls_rest.go` — Pagination, validation error format
- `bleephub/gh_labels_rest.go` — Pagination, validation error format
- `bleephub/gh_orgs_rest.go` — Pagination in list handlers
- `bleephub/gh_teams_rest.go` — Pagination in list handlers
- `bleephub/gh_members_rest.go` — Pagination in list handlers
- `bleephub/gh_repos_refs.go` — Pagination in list branches
- `bleephub/gh_repos_objects.go` — Pagination in list commits
- `Makefile` — Added `bleephub-gh-test` target

**Key design decisions:**
- Go generics (`paginateAndLink[T any]`) for pagination — each of 15 list handlers needed only a 1-line change
- Link header construction handles edge cases: single-page (no header), first/last page, max per_page clamping
- `gh` CLI test uses `gh api` with full URLs + explicit auth headers (avoids GHES hostname/port restrictions)
- OpenAPI schemas are a curated subset (required fields + types only) — enough to catch regressions without maintaining the full spec

**Test results:** 148 unit tests pass (107 existing + 7 pagination + 23 conformance + 11 OpenAPI), `gh` CLI integration test passes (35 assertions).

## Phase 42 — Runner Enhancements (Actions + Multi-Job)

Expanded bleephub from single-job `run:` steps to full workflow support:

1. **Workflow YAML parsing** (`workflow.go`) — `ParseWorkflow()` parses GitHub Actions YAML into typed structs. Handles `needs:` as string or list, `container:` as string or object, matrix reserved keys (`include`/`exclude`). 12 new types: `WorkflowDef`, `JobDef`, `StepDef`, `StrategyDef`, `MatrixDef`, etc.

2. **Action resolution + tarball proxy** (`actions.go`) — `handleActionDownloadInfo` returns proper `ActionDownloadInfoCollection` with `TarballUrl` pointing back to bleephub's proxy endpoint. `handleActionTarball` serves cached tarballs or fetches from `https://api.github.com/repos/{owner}/{repo}/tarball/{ref}`. In-memory `ActionCache` with thread-safe access.

3. **Multi-job workflow engine** (`workflows.go`) — `Workflow` + `WorkflowJob` types. `submitWorkflow` validates no cycles (topological sort DFS), dispatches root jobs. `dispatchReadyJobs` finds pending jobs with all deps satisfied; skips jobs whose deps failed. `onJobCompleted` updates workflow state and cascades to dependents. `buildJobMessageFromDef` supports both `run:` (script reference) and `uses:` (repository reference) step types. `needs` context propagation with outputs.

4. **Matrix expansion** (`matrix.go`) — `ExpandMatrix` computes Cartesian product of matrix values, applies `include` (extends matching combos or adds new), applies `exclude` (removes matching). `MatrixJobName` generates display names like `"test (ubuntu, 3.9)"`. `expandMatrixJobs` on workflow submission creates N jobs per matrix, updating needs references.

5. **Artifact + cache stubs** (`artifacts.go`) — Twirp-style JSON-over-HTTP stubs matching `@actions/artifact` v4: CreateArtifact (returns signed upload URL), upload (blob append), FinalizeArtifact, ListArtifacts (finalized only), GetSignedArtifactURL, download. Cache API stubs: reserve (204 no-op), lookup (204 miss), upload/finalize (200 discard).

6. **Workflow submission endpoint** — `POST /api/v3/bleephub/workflow` accepts `{"workflow":"<yaml>","image":"<default>"}`, parses YAML, expands matrix, submits to workflow engine. `GET /api/v3/bleephub/workflows/{id}` queries status.

7. **Integration test** — Extended `run-integration.sh` with multi-job workflow test: submits 2-job workflow with `needs:` dependency, polls workflow status until completion, validates both jobs succeeded.

**Key decisions:**
- Action tarball proxy avoids requiring runner containers to have direct GitHub access
- Matrix expansion at submit time (not runtime) keeps the runner protocol unchanged
- Workflow env stores `__serverURL` and `__defaultImage` for re-dispatch after job completion
- Matrix values passed via `__matrix_` env prefix from `expandMatrixJobs` to `WorkflowJob.MatrixValues`
- `onJobCompleted` dispatches ready jobs _before_ checking all-done (fixes skip-dependent-then-check-completion ordering)

**Test results:** 190 unit tests pass (148 existing + 42 new: 12 workflow parsing + 5 ParseActionRef + 6 actions + 10 workflow engine + 8 matrix + 7 artifacts). Integration test updated with multi-job workflow.

## Phase 43 — Cloud Resource Tracking (Feb 2026)

Unified cloud resource tagging, tracking, and crash recovery across all 6 cloud backends. Every cloud resource Sockerless creates is now tagged with standard metadata and tracked in a local registry.

**Shared tag builder** (`backends/core/tags.go`) — `TagSet` struct with 3 output formats:
- `AsMap()` — `map[string]string` for AWS and general use
- `AsGCPLabels()` — GCP-safe format (underscore keys, values truncated to 63 chars)
- `AsAzurePtrMap()` — `map[string]*string` for Azure SDK convention

5 standard tags: `sockerless-managed`, `sockerless-container-id`, `sockerless-backend`, `sockerless-instance`, `sockerless-created-at`.

**Backend tagging** — All 6 backends now tag their cloud resources:
- ECS: Tags on `RegisterTaskDefinition` + `RunTask` (converted to `[]ecstypes.Tag`)
- Lambda: Tags on `CreateFunction` (native `map[string]string`)
- Cloud Run: Replaced hardcoded labels with `TagSet.AsGCPLabels()`
- GCF: Labels on `CreateFunction` via `AsGCPLabels()`
- ACA: Replaced hardcoded tags with `TagSet.AsAzurePtrMap()`
- Azure Functions: Tags on `Site` via `AsAzurePtrMap()`

**Resource registry** (`backends/core/resource_registry.go`) — In-memory registry with JSON file persistence. Tracks all cloud resources by resource ID, container ID, backend type, and timestamps. Methods: Register, MarkCleanedUp, ListActive, ListOrphaned, Save, Load. REST endpoints: `GET /internal/v1/resources`, `GET /internal/v1/resources/orphaned`, `POST /internal/v1/resources/cleanup`.

**Crash recovery** (`backends/core/recovery.go`) — `CloudScanner` interface with `ScanOrphanedResources` and `CleanupResource` methods. `RecoverOnStartup` loads the registry from disk, scans cloud for tagged resources not in the registry, and registers them as orphans. Implemented for all 6 backends.

**Simulator tag support** — AWS ECS simulator stores tags on task definitions and tasks, returns them via `ListTagsForResource`. Lambda simulator stores tags on functions, supports `GET /tags/{arn}` and `POST /tags/{arn}`.

**New files (12):** `backends/core/tags.go`, `tags_test.go`, `resource_registry.go`, `resource_registry_test.go`, `recovery.go`; `backends/{ecs,lambda,cloudrun,cloudrun-functions,aca,azure-functions}/recovery.go`; `tests/monitoring_resource_tracking_test.go`.

**Modified files (11):** `backends/core/server.go`, `backends/ecs/{taskdef,containers}.go`, `backends/lambda/containers.go`, `backends/cloudrun/{jobspec,containers}.go`, `backends/cloudrun-functions/containers.go`, `backends/aca/{jobspec,containers}.go`, `backends/azure-functions/containers.go`, `simulators/aws/{ecs,lambda}.go`.

**Test results:** 11 new unit tests (6 tag builder + 5 registry), all sim-test-all tests pass, integration test passes.

## Project Stats

- **43 phases**, 382 tasks completed
- **15 Go modules** across backends, simulators, sandbox, agent, API, frontend, bleephub, tests
- **21 Go-implemented builtins** in WASM sandbox
- **8 driver interface methods** across 4 driver types (was 4 interfaces × ~3 methods, now complete)
- **6 external test consumers**: `act`, `gitlab-runner`, `gitlab-ci-local`, upstream act test suite, official `actions/runner`, `gh` CLI
- **3 cloud simulators** validated against SDKs, CLIs, and Terraform
- **8 backends** sharing a common driver architecture
