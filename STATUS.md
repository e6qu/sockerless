# Sockerless — Current Status

**Phase 56 complete. 461 tasks done across 56 phases. Next: Phase 57.**

## Test Results (Latest)

### All-Pass Suites

| Suite | Count | Command |
|---|---|---|
| Sandbox unit tests | 46 PASS | `cd sandbox && go test -v ./...` |
| Sim-backend integration | 129 PASS / 0 FAIL | `make sim-test-all` |
| Lint (14 modules) | 0 issues | `make lint` |
| Unit + integration | ALL PASS | `make test` |
| GitHub E2E | 31 workflows × 7 backends = 217 PASS | `make e2e-github-{memory,aws,gcp,azure}` |
| GitLab E2E | 22 pipelines × 7 backends = 154 PASS | `make e2e-gitlab-{memory,aws,gcp,azure}` |
| Upstream gitlab-ci-local | 36 tests × 7 backends = 252 PASS | `make upstream-test-gitlab-ci-local-{backend}` |
| Terraform integration | 75 PASS (ECS 21, Lambda 5, CR 13, GCF 7, ACA 18, AZF 11) | `make tf-int-test-all` |
| Cloud SDK tests | AWS 17, GCP 20, Azure 13 | `make docker-test` per cloud |
| Cloud CLI tests | AWS 21, GCP 15, Azure 14 | `make docker-test` per cloud |
| bleephub integration | 1 PASS (full runner lifecycle) | `make bleephub-test` |
| bleephub unit tests | 196 PASS (190 prior + 4 service parsing + 2 job message services) | `cd bleephub && go test -v ./...` |
| bleephub gh CLI test | 1 PASS (35 assertions, Docker-based) | `make bleephub-gh-test` |
| Core tag builder | 6 PASS (AsMap, AsGCPLabels, AsAzurePtrMap, truncation, DefaultInstanceID) | `cd backends/core && go test -v -run TestTag ./...` |
| Core resource registry | 11 PASS (register, cleanup, orphaned, save/load, non-existent, auto-save, atomic, status, backward-compat, metadata) | `cd backends/core && go test -v -run TestRegistry ./...` |
| Core crash recovery | 7 PASS (load-from-disk, merge-orphans, scan-error, reconstruct, skip-existing, skip-cleaned, default-name) | `cd backends/core && go test -v -run "TestRecover\|TestReconstruct" ./...` |
| Core pod registry + API | 21 PASS (8 registry, 7 API handler, 6 container-pod association) | `cd backends/core && go test -v -run "TestPod\|TestHandle.*Pod\|TestContainer.*Pod\|TestImplicit\|TestBuiltin\|TestNetwork.*Joins" ./...` |
| Core pod deferred start | 8 PASS (5 deferred start, 1 idempotency, 1 rejection, 1 compat) | `cd backends/core && go test -v -run "TestPodDeferred\|TestMarkStarted\|TestMemory.*Reject\|TestSingleContainer.*Pod" ./...` |
| Core context loader | 6 PASS (no-context no-op, sets vars, no override, missing file, invalid JSON, active file) | `cd backends/core && go test -v -run "TestLoadContext\|TestActiveContext" ./...` |
| Core management API | 7 PASS (healthz, status, container summary, summary empty, check, check-no-checker, reload) | `cd backends/core && go test -v -run "TestHandleHealthz\|TestHandleMgmt\|TestHandleContainerSummary\|TestHandleCheck\|TestHandleReload" ./...` |
| Core metrics | 4 PASS (record, percentiles, middleware, handler) | `cd backends/core && go test -v -run "TestMetrics\|TestHandleMetrics" ./...` |
| Core pod health wait | 3 PASS (all-healthy, timeout, no-healthcheck) | `cd backends/core && go test -v -run "TestWaitForServiceHealth" ./...` |
| Core docker config | 6 PASS (multi-registry, empty-auths, missing-file, invalid-json, hub-alias, auth-decode) | `cd backends/core && go test -v -run "TestLoadDockerConfig\|TestDockerConfig" ./...` |
| Core auth decode | 3 PASS (valid, invalid, empty) | `cd backends/core && go test -v -run "TestDecodeRegistryAuth" ./...` |
| Core tmpfs mounts | 3 PASS (creates-temp-dirs, empty-map, merges-with-binds) | `cd backends/core && go test -v -run "TestResolveTmpfsMounts" ./...` |
| Core log filter | 6 PASS (tail-lastN, tail-all, tail-zero, since, until, parse-timestamp) | `cd backends/core && go test -v -run "TestFilterLog\|TestParseDocker" ./...` |
| Core log follow | 3 PASS (buffered-then-close, empty-logs, tail-and-follow) | `cd backends/core && go test -v -run "TestSyntheticLogSubscribe\|TestLogFollow" ./...` |
| Core extra hosts | 3 PASS (format-env, format-empty, build-hosts-file) | `cd backends/core && go test -v -run "TestFormatExtraHosts\|TestBuildHostsFile" ./...` |
| Core DNS hosts | 3 PASS (same-pod, no-pod, includes-hostname) | `cd backends/core && go test -v -run "TestResolvePeerHosts" ./...` |
| Core restart policy | 3 PASS (on-failure, max-retry, no-policy) | `cd backends/core && go test -v -run "TestShouldRestart" ./...` |
| Core network disconnect | 4 PASS (basic, not-found-network, not-found-container, force) | `cd backends/core && go test -v -run "TestNetworkDisconnect" ./...` |
| Core event bus | 3 PASS (publish-subscribe, multiple-subscribers, unsubscribe) | `cd backends/core && go test -v -run "TestEventBus" ./...` |
| Core container update | 3 PASS (restart-policy, not-found, empty-body) | `cd backends/core && go test -v -run "TestContainerUpdate" ./...` |
| Core volume filters | 2 PASS (filter-by-name, filter-by-label) | `cd backends/core && go test -v -run "TestVolumeList_Filter" ./...` |
| Core container changes | 1 PASS (empty) | `cd backends/core && go test -v -run "TestContainerChanges" ./...` |
| Core event emission | 2 PASS (container-create, network-remove) | `cd backends/core && go test -v -run "TestEventEmission" ./...` |
| Core container list | 4 PASS (limit, limit-zero, sort-by-created, all-false-excludes-stopped) | `cd backends/core && go test -v -run "TestContainerList_" ./...` |
| Core container filters | 5 PASS (ancestor, ancestor-no-match, network, health, before-since) | `cd backends/core && go test -v -run "TestContainerFilter_" ./...` |
| Core container export | 2 PASS (not-found, synthetic-empty) | `cd backends/core && go test -v -run "TestContainerExport_" ./...` |
| Core commit | 3 PASS (basic, not-found, config-override) | `cd backends/core && go test -v -run "TestCommit_" ./...` |
| Frontend TLS | 4 PASS (tls-https, non-tls-fallback, unix-ignores-tls, mgmt-tls) | `cd frontends/docker && go test -v -run "TestTLS\|TestNonTLS\|TestUnixSocket\|TestMgmt" ./...` |

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

Remaining failure categories:
- **Node.js 16 required** (16 tests): `uses: actions/*` steps need `node16` binary — not available in WASM sandbox or FaaS containers
- **Networking/services** (4 tests): Service container health polling and inter-container networking
- **Docker build** (2 tests): Multi-stage builds with RUN instructions that execute real commands
- **Shell edge cases** (2 tests): POSIX shell differences between busybox ash and bash

All bash/shell tests now pass (Phase 31). Health check infrastructure added (Phase 33). `POST /build` implemented (Phase 34).

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

10 interface methods: `ExecDriver` (1), `FilesystemDriver` (4), `StreamDriver` (4), `ProcessLifecycleDriver` (8).

`DriverSet` on `BaseServer` auto-constructs the chain via `InitDrivers()`. Handlers call only through driver methods — zero `ProcessFactory`/`Store.Processes` references in handler files (Phase 32).

### Simulators (3)

Each simulator implements enough cloud API for its backends. Validated against official SDKs, CLIs, and Terraform providers.

| Simulator | Backends | SDK Tests | CLI Tests | TF Tests |
|---|---|---|---|---|
| AWS | ecs, lambda | 17 PASS | 21 PASS | 26 PASS |
| GCP | cloudrun, gcf | 20 PASS | 15 PASS | 20 PASS |
| Azure | aca, azf | 13 PASS | 14 PASS | 29 PASS |

### Cloud Resource Tracking + Crash Recovery (Phases 43-44)

All cloud resources tagged with 5 standard tags. Persistent resource registry auto-saves on every mutation (atomic writes). Metadata (image, name, backend-specific IDs) stored per entry. Status lifecycle: pending → active → cleanedUp. RecoverOnStartup called in all 6 cloud backends. Container state reconstructed from registry on startup. bleephub made crash-only (no graceful shutdown). REST endpoints for listing and cleanup.

## Known Limitations

1. **GitLab + memory = synthetic**: gitlab-runner requires helper binaries (`gitlab-runner-helper`, `gitlab-runner-build`) that can't run in WASM. Memory backend uses `SOCKERLESS_SYNTHETIC=1` for GitLab E2E. Not fixable without native execution.

2. **WASM sandbox scope**: No `bash`, `node`, `python`, `git`, `apt-get`. Only busybox applets + Go-implemented builtins (21) + POSIX shell via mvdan.cc/sh. This limits upstream act pass rate for memory backend.

3. **FaaS transient failures**: ~1 transient failure per sequential E2E run on FaaS backends (lambda, gcf, azf) due to reverse agent cleanup timing. All pass individually.

4. **Upstream act individual mode**: Memory and azf backends require `--individual` flag (hang in monolithic mode). Cloud backends work in monolithic mode.

5. **Azure terraform tests**: Docker-only (Linux). On macOS, Go uses Security.framework for TLS and ignores `SSL_CERT_FILE`, so terraform can't trust the self-signed CA.
