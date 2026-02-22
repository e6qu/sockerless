# Sockerless — Current Status

**Phase 43 complete. 382 tasks done across 43 phases. Next: Phase 44 (crash-only software).**

## Test Results (Latest)

### All-Pass Suites

| Suite | Count | Command |
|---|---|---|
| Sandbox unit tests | 46 PASS | `cd sandbox && go test -v ./...` |
| Sim-backend integration | 129 PASS / 0 FAIL | `make sim-test-all` |
| Lint (14 modules) | 0 issues | `make lint` |
| Unit + integration | ALL PASS | `make test` |
| GitHub E2E | 22 workflows × 7 backends = 154 PASS | `make e2e-github-{memory,aws,gcp,azure}` |
| GitLab E2E | 17 pipelines × 7 backends = 119 PASS | `make e2e-gitlab-{memory,aws,gcp,azure}` |
| Upstream gitlab-ci-local | 25 tests × 7 backends = 175 PASS | `make upstream-test-gitlab-ci-local-{backend}` |
| Terraform integration | 75 PASS (ECS 21, Lambda 5, CR 13, GCF 7, ACA 18, AZF 11) | `make tf-int-test-all` |
| Cloud SDK tests | AWS 17, GCP 20, Azure 13 | `make docker-test` per cloud |
| Cloud CLI tests | AWS 21, GCP 15, Azure 14 | `make docker-test` per cloud |
| bleephub integration | 1 PASS (full runner lifecycle) | `make bleephub-test` |
| bleephub unit tests | 190 PASS (148 prior + 12 workflow parsing + 6 actions + 10 workflow engine + 8 matrix + 6 artifacts) | `cd bleephub && go test -v ./...` |
| bleephub gh CLI test | 1 PASS (35 assertions, Docker-based) | `make bleephub-gh-test` |
| Core tag builder | 6 PASS (AsMap, AsGCPLabels, AsAzurePtrMap, truncation, DefaultInstanceID) | `cd backends/core && go test -v -run TestTag ./...` |
| Core resource registry | 5 PASS (register, cleanup, orphaned, save/load, non-existent) | `cd backends/core && go test -v -run TestRegistry ./...` |

### Upstream Act (Informational — not all expected to pass)

| Backend | PASS | FAIL | Mode |
|---|---|---|---|
| memory | 91 | 24 | individual |
| ecs | 56 | 31 | monolithic |
| lambda | 57 | 30 | monolithic |
| cloudrun | 54 | 33 | monolithic |
| gcf | 58 | 29 | monolithic |
| aca | 58 | 29 | monolithic |
| azf | 69 | 16 | individual |

Remaining failures: missing `node` runtime (16), service containers/networking (4, health poll no longer hangs), edge cases (2-4). All bash/shell tests now pass (Phase 31). Health check infrastructure added (Phase 33). `POST /build` implemented (Phase 34) — `local-action-dockerfile` should now pass.

### bleephub (Official GitHub Actions Runner)

The official `actions/runner` binary (C#) registers, receives jobs, executes them through Sockerless, and reports completion. Tested with a container workflow (`run:` steps only, no `uses:` actions). Runner communicates via Azure DevOps-derived internal API (5 services on GHES-style path prefixes). Test runs in Docker: bleephub + memory backend + Docker frontend + official runner binary.

### bleephub GitHub API (Phases 36-38)

bleephub now also serves GitHub REST API and GraphQL endpoints, validated against the `gh` CLI. Features:
- **User accounts** with token authentication (`Authorization: token {pat}`)
- **OAuth device flow** (`/login/device/code`, `/login/oauth/access_token`)
- **GraphQL engine** (`graphql-go/graphql`) with introspection, `viewer` query, mutations
- **Git repositories** — in-memory bare repos via `go-git/go-git/v5` with smart HTTP protocol (`git clone`, `git push`)
- **Repository CRUD** — REST + GraphQL create/read/update/delete, Relay cursor pagination
- **Git objects** — commits, trees, blobs, README, branches, refs
- **Organizations** — org CRUD, team management, membership, RBAC enforcement on repos
- **Teams** — team CRUD, team membership, team-repo permissions (pull/push/admin)
- **RBAC** — org admins, team permissions, public/private repo visibility
- **Issues** — issue CRUD with per-repo sequential numbering, state (OPEN/CLOSED), stateReason (COMPLETED/NOT_PLANNED)
- **Labels** — per-repo label CRUD, issue-label association
- **Milestones** — per-repo milestone CRUD with sequential numbering
- **Comments** — issue comment CRUD
- **Reactions** — static reaction groups (8 types) on issues and PRs
- **Pull Requests** — PR CRUD with shared issue/PR numbering, state (OPEN/CLOSED/MERGED), draft support, head/base ref tracking
- **PR Merge** — merge via REST and GraphQL with merge method support (MERGE/SQUASH/REBASE)
- **PR Reviews** — review CRUD (APPROVED/CHANGES_REQUESTED/COMMENTED), review decision derivation
- **GraphQL mutations** — createIssue, closeIssue, reopenIssue, addComment, updateIssue, createPullRequest, closePullRequest, reopenPullRequest, mergePullRequest, updatePullRequest
- **GraphQL queries** — repository.issues (with state/label/assignee filtering), repository.issue, repository.labels, repository.milestones, repository.assignableUsers, repository.pullRequests (with state/label/head/base filtering), repository.pullRequest
- **Response headers**: `X-OAuth-Scopes`, `X-RateLimit-*`, `X-GitHub-Request-Id`, `X-GitHub-Api-Version`
- **TLS support** via `BPH_TLS_CERT`/`BPH_TLS_KEY` env vars
- **REST pagination** — `Link` headers (RFC 5988) on all list endpoints, configurable `page`/`per_page`
- **Error conformance** — 422 responses include `errors` array with `resource`/`field`/`code`
- **Content negotiation** — `Content-Type: application/json; charset=utf-8`, `X-GitHub-Api-Version` header
- **OpenAPI schema validation** — vendored schemas for 8 resource types, validated in unit tests
- **`gh` CLI integration test** — Docker-based end-to-end test using `gh api` with TLS

## Architecture

### Backends (8)

| Backend | Cloud | Execution | Agent |
|---|---|---|---|
| memory | none | WASM sandbox (in-process) | none |
| docker | none | Docker daemon passthrough | none |
| ecs | AWS | ECS tasks | forward |
| lambda | AWS | Lambda invocation | reverse |
| cloudrun | GCP | Cloud Run jobs | forward |
| gcf | GCP | Cloud Functions invocation | reverse |
| aca | Azure | Container Apps jobs | forward |
| azf | Azure | Azure Functions invocation | reverse |

### Driver Chain (Phase 30)

Handler code dispatches through driver interfaces instead of if/else chains:

```
Agent Driver → Process Driver → Synthetic Driver
```

8 interface methods: `ExecDriver` (1), `FilesystemDriver` (4), `StreamDriver` (2), `ProcessLifecycleDriver` (8).

`DriverSet` on `BaseServer` auto-constructs the chain via `InitDrivers()`. Handlers call only through driver methods — zero `ProcessFactory`/`Store.Processes` references in handler files (Phase 32).

### Simulators (3)

Each simulator implements enough cloud API for its backends. Validated against official SDKs, CLIs, and Terraform providers.

| Simulator | Backends | SDK Tests | CLI Tests | TF Tests |
|---|---|---|---|---|
| AWS | ecs, lambda | 17 PASS | 21 PASS | 26 PASS |
| GCP | cloudrun, gcf | 20 PASS | 15 PASS | 20 PASS |
| Azure | aca, azf | 13 PASS | 14 PASS | 29 PASS |

### Cloud Resource Tracking (Phase 43)

All cloud resources tagged with 5 standard tags. Local resource registry tracks creates/deletes. CloudScanner interface enables crash recovery. REST endpoints for listing and cleanup.

## Known Limitations

1. **GitLab + memory = synthetic**: gitlab-runner requires helper binaries (`gitlab-runner-helper`, `gitlab-runner-build`) that can't run in WASM. Memory backend uses `SOCKERLESS_SYNTHETIC=1` for GitLab E2E. Not fixable without native execution.

2. **WASM sandbox scope**: No `bash`, `node`, `python`, `git`, `apt-get`. Only busybox applets + Go-implemented builtins (21) + POSIX shell via mvdan.cc/sh. This limits upstream act pass rate for memory backend.

3. **FaaS transient failures**: ~1 transient failure per sequential E2E run on FaaS backends (lambda, gcf, azf) due to reverse agent cleanup timing. All pass individually.

4. **Upstream act individual mode**: Memory and azf backends require `--individual` flag (hang in monolithic mode). Cloud backends work in monolithic mode.

5. **Azure terraform tests**: Docker-only (Linux). On macOS, Go uses Security.framework for TLS and ignores `SSL_CERT_FILE`, so terraform can't trust the self-signed CA.
