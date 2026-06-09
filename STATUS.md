# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `bleephub-parity-storage` |
| In-flight | Subtask 1 completed on `bleephub-parity-storage`: Bleephub now has real in-process Actions cache reserve/upload/finalize/lookup/download behavior instead of no-op cache handlers, and unknown GitHub API paths return GitHub-shaped 404s instead of silent success/plain 404s. Next work is deeper Actions artifact/cache indexing and durability. |
| Last merged | The Bigtable Terraform coverage + AWS real-execution semantics branch was merged before this branch started. |
| Open GitHub issues | #394 remained upstream-blocked from the previous issue sweep. Re-check GitHub before doing any non-Bleephub issue work. |
| Bugs | 1589 filed - 1544 fixed - 6 open - 5 false positives. |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1345 azuread upstream; BUG-1584 AzureStack provider deprecation warning despite `metadata_host`. |
| Live infra | None up. |

## Current Bleephub Findings

- [bleephub/artifacts.go](bleephub/artifacts.go) now keeps real Actions cache
  records and downloadable saved entries for reserve/upload/finalize/lookup.
  Next cache work should index caches by run/repo/scope where the runner/API
  surfaces require it and wire the model into the later durable storage work.
- [bleephub/server.go](bleephub/server.go) and
  [bleephub/gh_rest.go](bleephub/gh_rest.go) no longer return success/plain
  responses for unknown GitHub API paths. Unknown REST paths return
  GitHub-shaped 404s; non-API unmatched paths return normal HTTP 404s.
- [bleephub/persistence.go](bleephub/persistence.go) is SQLite-only and only
  persists selected buckets. The branch must add PostgreSQL and broaden durable
  state for public API objects.
- [bleephub/git_storage.go](bleephub/git_storage.go) supports memory/filesystem
  git storage only; [bleephub/store_repos.go](bleephub/store_repos.go) ignores
  git-storage initialization errors. The branch must fail loudly and add real
  S3/MinIO-compatible git content storage.
- [bleephub/git_http.go](bleephub/git_http.go) serves clone/fetch/push without a
  visible repo permission gate. The branch must enforce real visibility and token
  permissions.
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
  shape-only or empty responses for Pages builds, audit log, run artifact lists,
  approvals, and status rollups.

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
