# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current Branch

Branch: `bleephub-parity-storage`.

This branch is the single working branch for the Bleephub parity, durability,
storage, UI, and docs PR. Keep one PR open for this branch. Make one natural
commit per subtask, with tests and continuity docs included in the same commit.

## Last Completed Subtask

Subtasks 1, 2, 3, 4, 5, 6, and 7 completed:

- Subtask 1: Unknown GitHub API paths return GitHub-shaped 404s; cache handlers
  replaced with real reserve/upload/finalize/lookup/download behavior.
- Subtask 2: GitHub REST artifact list/get/delete/download endpoints return
  real stored artifacts with pagination, name filtering, digest, and repo/run
  isolation.
- Subtask 3: PostgreSQL persistence support via pgx.
- Subtask 4: Broadened durable state to cover all remaining public API objects.
  Write-through persistence added for hooks, hook_deliveries,
  app_hook_deliveries, check_runs, check_suites, check_suite_prefs,
  repo_secrets, workflow_files, pr_reviews, releases, deployments,
  deployment_statuses, environments, pr_review_comments, reactions,
  projects_v2, project_v2_items, and project_v2_fields. All new buckets
  load correctly from disk on restart.
- Subtask 5: Git storage hardening and permission enforcement.
  - `CreateRepo` returns nil when `openOrInitGitStorage` fails (no more silent
    discard). `loadFromPersistence` returns an error if git storage can't be
    reopened for a persisted repo.
  - `DeleteRepo` removes filesystem git data when `BLEEPHUB_GIT_DIR` is set.
  - Git HTTP handlers (`info/refs`, `git-upload-pack`, `git-receive-pack`) now
    authenticate requests and enforce permissions: read access for fetch, push
    access for push. Supports `token`, `Bearer`, and `Basic` auth headers.
  - New `canPushRepo` function in rbac.go checks ownership or org team push.
  - Extracted `authenticateRequest` from `ghHeadersMiddleware` for reuse by git
    HTTP handlers.
  - 7 new tests: storage init failure, delete cleanup, unauthenticated push
    rejection, public/private repo fetch auth, authenticated push, authenticated
    private repo fetch.
- Subtask 6: S3/MinIO-compatible git content storage.
  - Added `s3fs.go` implementing `billy.Filesystem` and `billy.File` over S3
    using `aws-sdk-go-v2/service/s3`. Key layout: `prefix/{owner}/{repo}/` for
    each repo's git data. Temp files use UUID-prefixed keys. Renames use
    CopyObject+DeleteObject. Directories are emulated via prefix listing.
  - `git_storage.go` now supports three backends: memory (default), filesystem
    (`BLEEPHUB_GIT_DIR`), and S3 (`BLEEPHUB_S3_ENDPOINT` + `BLEEPHUB_S3_BUCKET`
    + optional `BLEEPHUB_S3_PREFIX`). S3 takes priority when configured.
  - `DeleteRepo` cleans up S3 objects by listing and deleting all objects under
    the repo prefix.
  - The S3 filesystem is lazily initialized and cached (singleton connection per
    process). Uses path-style addressing for MinIO compatibility.
- Subtask 7: UI auth and operator storage/status views.
  - Removed hard-coded admin token from `api.ts`. All API calls now use
    `authHeaders()` which reads the token from `localStorage`.
  - Added `LoginPage` with password input that verifies the token against
    `/api/v3/user` before storing it.
  - Added auth guard: unauthenticated users are redirected to `/ui/login`.
    All UI routes require a valid token.
  - Added sign-out button in the AppShell nav.
  - Added `/internal/storage` endpoint reporting persistence backend
    (none/sqlite/postgres), dialect, git storage backend
    (memory/filesystem/s3), and git storage details (dir/bucket/endpoint/prefix).
  - OverviewPage shows storage backend info section with persistence and
    git storage status.
  - Added `BleephubStorageInfo` type to `types.ts`.
  - `BLEEPHUB_DATABASE_URL` activates PostgreSQL (pgx v5, `database/sql`
    interface). `BLEEPHUB_PERSIST=true` continues to activate SQLite.
  - A `dbDialect` struct holds dialect-specific SQL (placeholders, types, DDL)
    so both backends share the same `Persistence` methods.
  - The PostgreSQL test skips unless `BLEEPHUB_TEST_POSTGRES_URL` is set
    (requires a real PostgreSQL instance).
  - All existing SQLite persistence tests pass unchanged.

Verified:

```bash
cd bleephub && GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test -run 'TestGit' ./... -v
cd bleephub && GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./...
gofmt -l bleephub/git_storage.go bleephub/git_http.go bleephub/store_repos.go bleephub/rbac.go bleephub/gh_middleware.go
```

## Current Subtask

Subtask 8: UI/API coverage for cache, artifacts, webhooks, orgs/teams, branch protection, audit events, and repo git refs.

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
