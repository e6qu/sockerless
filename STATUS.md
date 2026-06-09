# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `bleephub-parity-storage` |
| In-flight | Subtasks 1-7 completed on `bleephub-parity-storage`. Next work is UI/API coverage for cache, artifacts, webhooks, orgs/teams, branch protection, audit events, and repo git refs. |
| Last merged | The Bigtable Terraform coverage + AWS real-execution semantics branch was merged before this branch started. |
| Open GitHub issues | #394 remained upstream-blocked from the previous issue sweep. Re-check GitHub before doing any non-Bleephub issue work. |
| Bugs | 1590 filed - 1544 fixed - 7 open - 5 false positives. |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1345 azuread upstream; BUG-1584 AzureStack provider deprecation warning despite `metadata_host`; BUG-1590 Bleephub run approvals empty-success gap. |
| Live infra | None up. |

## Current Bleephub Findings

- [bleephub/artifacts.go](bleephub/artifacts.go) now keeps real Actions cache
  records and downloadable saved entries for reserve/upload/finalize/lookup.
  Next cache work should index caches by run/repo/scope where the runner/API
  surfaces require it and wire the model into the later durable storage work.
- [bleephub/artifacts.go](bleephub/artifacts.go) and
  [bleephub/gh_actions_extras.go](bleephub/gh_actions_extras.go) now join
  runner-created artifacts to repositories and GitHub run IDs, then expose real
  finalized artifacts through GitHub REST list/get/delete/download paths with
  pagination, name filtering, digest fields, and repo/run isolation.
- [bleephub/server.go](bleephub/server.go) and
  [bleephub/gh_rest.go](bleephub/gh_rest.go) no longer return success/plain
  responses for unknown GitHub API paths. Unknown REST paths return
  GitHub-shaped 404s; non-API unmatched paths return normal HTTP 404s.
- [bleephub/persistence.go](bleephub/persistence.go) supports SQLite
  (`BLEEPHUB_PERSIST=true`) and PostgreSQL (`BLEEPHUB_DATABASE_URL`) via
  a `dbDialect` struct that holds dialect-specific SQL. Both backends share
  the same `Persistence` methods. Operator-requested persistence that fails to
  open will `log.Fatalf`. The next persistence work should extend write-through
  coverage to remaining public API state (workflows, hooks, releases,
  deployments, reactions, check runs, secrets, artifacts/cache).
- [bleephub/git_storage.go](bleephub/git_storage.go) now fails loudly when git storage
  init fails. `CreateRepo` returns `nil` if `openOrInitGitStorage` errors.
  `loadFromPersistence` returns an error if git storage can't be reopened for a
  persisted repo.
- [bleephub/store_repos.go](bleephub/store_repos.go) `DeleteRepo` now removes the
  filesystem git data directory when `BLEEPHUB_GIT_DIR` is set.
- [bleephub/git_http.go](bleephub/git_http.go) enforces authentication and
  permissions on all git HTTP operations. `info/refs` with `git-upload-pack`
  requires read access; `info/refs` with `git-receive-pack` and `git-receive-pack`
  itself require push access. Supports `token`, `Bearer`, and `Basic` auth headers.
- [bleephub/rbac.go](bleephub/rbac.go) has a new `canPushRepo` function that checks
  ownership or org team-level "push" permission.
- [bleephub/gh_middleware.go](bleephub/gh_middleware.go) extracted
  `authenticateRequest` as a shared function used by both `/api/` middleware and git
  HTTP handlers. Basic auth (`username:password` where password is the token) is now
  recognized alongside `token` and `Bearer` prefixes.
- [bleephub/git_storage.go](bleephub/git_storage.go) supports memory/filesystem
  git storage only; [bleephub/store_repos.go](bleephub/store_repos.go) no longer
  ignores git-storage initialization errors. The next storage work should add
  S3/MinIO-compatible git content storage.
- [bleephub/git_http.go](bleephub/git_http.go) now serves clone/fetch/push with
  proper permission gates.
- [ui/packages/bleephub/src/api.ts](ui/packages/bleephub/src/api.ts) hard-codes
  an admin token while [bleephub/store.go](bleephub/store.go) requires
  `BLEEPHUB_ADMIN_TOKEN`. The UI needs real configured auth/session handling.
- Bleephub must preserve GitHub/GHES external identity. Public API paths,
  runner/workflow variables, request parameters, response fields, and
  client-facing UI text must use the GitHub names real clients expect, including
  `GITHUB_*` variables. Bleephub-specific names are acceptable only for internal
  code or clearly operator-only management surfaces.
- [bleephub/gh_misc_endpoints.go](bleephub/gh_misc_endpoints.go),
  [bleephub/gh_actions_extras.go](bleephub/gh_actions_extras.go), and
  [bleephub/gh_pulls_graphql.go](bleephub/gh_pulls_graphql.go) still contain
  shape-only or empty responses for Pages builds, audit log, approvals, and
  status rollups. BUG-1590 tracks the approvals gap explicitly.

## Bleephub Branch Rules

- Keep one PR open for `bleephub-parity-storage`.
- Use one natural commit per subtask from [PLAN.md](PLAN.md); target 8-10
  commits total for the implementation work.
- Update `STATUS.md` and [DO_NEXT.md](DO_NEXT.md) before and after each subtask.
- Do not add fake compatibility responses. Implement the real behavior or remove
  the claim from docs/API coverage until real behavior exists.
- Do not rename GitHub's observable external API. Endpoints, request fields,
  response fields, runner parameters, workflow variables, and UI identity should
  match GitHub/GHES rather than using Bleephub-branded substitutes.
- Use official GitHub REST/OpenAPI, GraphQL, Actions runner/cache/artifact, and
  Git smart-HTTP behavior as the reference surface.

## Invariants

- Never auto-merge PRs; the user handles merges.
- Rebase PR branches on `origin/main` before pushing.
- File concrete BUG entries before fixing discovered defects.
- No stubs, fakes, mocks, synthetic responses, silent fallbacks, or degraded modes.
- Simulators implement real cloud API slices in one binary per cloud.
- Every simulator public endpoint ships with official SDK, vendor CLI, and Terraform coverage where those surfaces exist.
- Coverage authorities are [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md) and [specs/SIM_SURFACE_TABLES](specs/SIM_SURFACE_TABLES).

## Environment Notes

- AzureRM custom metadata discovery still needs HTTPS through the local Caddy gateway: `make stack-https-{up,status,ca,down}`.
- AWS and GCP Terraform providers accept localhost custom endpoints directly.
- Simulator ports: AWS 4566, GCP 4567, Azure 4568.
- Linux network-fabric tests require `CAP_NET_ADMIN`, `iproute2`, and nftables; off-Linux tests skip through the realexec capability gate.
