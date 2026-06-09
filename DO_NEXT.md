# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current Branch

Branch: `bleephub-parity-storage`.

This branch is the single working branch for the Bleephub parity, durability,
storage, UI, and docs PR. Keep one PR open for this branch. Make one natural
commit per subtask, with tests and continuity docs included in the same commit.

## Last Completed Subtask

Subtasks 1, 2, and 3 completed:

- Subtask 1: Unknown GitHub API paths return GitHub-shaped 404s; cache handlers
  replaced with real reserve/upload/finalize/lookup/download behavior.
- Subtask 2: GitHub REST artifact list/get/delete/download endpoints return
  real stored artifacts with pagination, name filtering, digest, and repo/run
  isolation.
- Subtask 3: PostgreSQL persistence support via pgx.
  - `BLEEPHUB_DATABASE_URL` activates PostgreSQL (pgx v5, `database/sql`
    interface). `BLEEPHUB_PERSIST=true` continues to activate SQLite.
  - A `dbDialect` struct holds dialect-specific SQL (placeholders, types, DDL)
    so both backends share the same `Persistence` methods.
  - The PostgreSQL test skips unless `BLEEPHUB_TEST_POSTGRES_URL` is set
    (requires a real PostgreSQL instance).
  - All existing SQLite persistence tests pass unchanged.

Verified:

```bash
cd bleephub && GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test -run 'TestPersistence' ./... -v
cd bleephub && GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./...
gofmt -l bleephub/persistence.go bleephub/persistence_test.go bleephub/server.go
```

## Current Subtask

Subtask 4: Broaden durable state for public Bleephub API objects.

The persistence layer currently covers users, tokens, apps, oauth_apps,
installations, installation_tokens, user_to_server_tokens, refresh_tokens,
repos, orgs, teams, memberships, labels, milestones, issues, comments, and
pull_requests. Other exposed API state (workflows, hooks, hook_deliveries,
check_runs, check_suites, releases, deployments, reactions, PR review
comments, projects_v2, secrets, and artifacts/cache) stays in-memory only and
is lost on restart.

This subtask should extend write-through persistence to the remaining public
API surfaces that the GitHub API promises as durable.

Likely files: `bleephub/persistence.go`, `bleephub/store.go`,
`bleephub/store_*.go`, `bleephub/webhooks_store.go`,
`bleephub/gh_releases.go`, `bleephub/gh_deployments.go`,
`bleephub/gh_pr_comments.go`, `bleephub/gh_projects_v2_graphql.go`,
`bleephub/gh_reactions.go`, `bleephub/gh_checks_store.go`,
`bleephub/artifacts.go`, `bleephub/secrets.go`.

First commands:

```bash
rg -n "persist\." bleephub/store.go bleephub/store_*.go bleephub/webhooks_store.go bleephub/artifacts.go
```

## Ordered Subtasks For This PR

1. Baseline audit tests and error handling.
2. Real Actions cache behavior and artifact indexing.
3. SQLite/PostgreSQL persistence abstraction and configuration.
4. Broaden durable state for public Bleephub API objects.
5. Git storage hardening and git HTTP permission enforcement.
6. S3/MinIO-compatible git content storage.
7. UI auth and operator storage/status views.
8. UI/API coverage for cache, artifacts, webhooks, orgs/teams, branch protection,
   audit events, and repo git refs.
9. Remove or implement long-tail shape-only endpoints, then refresh all Bleephub
   docs and parity specs.
10. Final verification, rebase, push, PR creation, and local `main` sync after
    the user merges.

## Handoff Protocol

Before starting a subtask:

1. Read [STATUS.md](STATUS.md) and this file.
2. Confirm the active branch is `bleephub-parity-storage`.
3. Check `git status --short --branch`.
4. If the previous session stopped mid-subtask, inspect the modified files before
   editing anything.
5. Update this file if the next command list is no longer accurate.

After finishing a subtask:

1. Run focused tests for the touched area.
2. Run broader Bleephub tests when public API behavior changed:

```bash
cd bleephub && GOWORK=off go test ./...
cd ui/packages/bleephub && bun test
```

3. Update [STATUS.md](STATUS.md) with what changed, what was verified, and any
   remaining risk.
4. Update this file so the next session has an exact next subtask, likely files,
   and first verification command.
5. Add a short [WHAT_WE_DID.md](WHAT_WE_DID.md) entry for meaningful completed
   chunks.
6. Commit the code, tests, and continuity docs together.

## Verification State

PR #534 is open and was green after subtask 1. Subtask 2 and 3 are committed
locally with all tests passing; push is pending.

## Branch And PR Hygiene

- Keep one PR open for this branch.
- Before pushing the PR branch:

```bash
git fetch origin main
git rebase origin/main
```

- After pushing the branch and opening/updating the PR, return local `main` to
  the remote state after the user merges:

```bash
git checkout main
git pull origin main
```

Do not merge the PR from the agent session; the user handles merges.

## Rules That Matter Most

- No stubs, fakes, mocks, synthetic responses, discarded uploads, or silent
  fallback to memory when durable storage was requested.
- Bleephub should match real GitHub/GHES behavior for every API it exposes.
- Use official GitHub REST/OpenAPI, GraphQL, Actions cache/artifact, official
  runner behavior, and Git smart-HTTP behavior as references.
- For object storage, use a real S3-compatible client against S3/MinIO-shaped
  APIs. Do not create an in-memory object-store stand-in.
