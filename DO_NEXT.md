# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current Branch

Branch: `bleephub-parity-storage`.

This branch is the single working branch for the Bleephub parity, durability,
storage, UI, and docs PR. Keep one PR open for this branch. Make one natural
commit per subtask, with tests and continuity docs included in the same commit.

## Last Completed Subtask

Subtasks 1 and 2 completed:

- [bleephub/artifacts.go](bleephub/artifacts.go) replaced the Actions cache
  no-op handlers with real cache reserve/upload/finalize/lookup/download state.
- Cache lookup now returns 204 for misses, 200 with `archiveLocation` and
  `cacheKey` for hits, and supports exact keys before restore-key prefix
  matching.
- [bleephub/server.go](bleephub/server.go) and
  [bleephub/gh_rest.go](bleephub/gh_rest.go) stopped returning successful or
  plain responses for unknown GitHub API paths.
- Tests cover cache round-trip, cache misses, restore-key prefix matching, and
  unknown-route status/body behavior.
- [bleephub/artifacts.go](bleephub/artifacts.go) now records artifact
  `repoFullName`, GitHub `run_id`, and `workflow_run_backend_id` metadata when
  the runner creates artifacts through the Twirp results API.
- [bleephub/gh_actions_extras.go](bleephub/gh_actions_extras.go) now serves real
  finalized artifacts through the documented GitHub REST paths:
  `/actions/runs/{run_id}/artifacts`, `/actions/artifacts`,
  `/actions/artifacts/{artifact_id}`, DELETE, and `/zip` download redirect.
- Artifact REST responses include GitHub-shaped artifact fields, workflow-run
  linkage, digest, pagination, name filtering, and repo/run isolation.
- [BUGS.md](BUGS.md) now records the separate environment approvals fidelity gap
  instead of letting the empty approvals endpoint look like proof of modeled
  state.

Verified:

```bash
cd bleephub && GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test -run 'TestCache|TestUnknownRoutesDoNotReturnSuccess' ./...
cd bleephub && GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./...
cd bleephub && GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test -c ./...
git diff --check
```

The focused artifact/cache test run was attempted with
`go test -run 'TestActionsArtifacts|TestArtifact|TestCache' ./...`, but this
sandbox could not bind `127.0.0.1:0`, and escalation was unavailable because the
session hit its usage limit. Re-run that command before pushing the next commit.

## Current Subtask

Subtask 3: SQLite/PostgreSQL persistence abstraction and configuration.

The next slice should make persistence an explicit backend choice instead of a
SQLite-only switch:

- Keep SQLite support and its existing durability behavior.
- Add PostgreSQL support with real migrations/schema creation and no in-memory
  fallback if the configured database cannot open or migrate.
- Define natural configuration names and preserve GitHub/GHES-facing external
  API names. Bleephub-specific env vars are acceptable for operator-only server
  configuration, but runner/API/workflow-visible names must stay GitHub-shaped.
- Update [bleephub/persistence.go](bleephub/persistence.go), server startup
  wiring, docs, and tests together.
- Before changing database code, remove the generated local
  `bleephub/bleephub.test` file if it is still present. It was produced by
  compile-only validation and was not staged.

First commands for the next session:

```bash
git status --short --branch
cd bleephub && GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test -run 'TestActionsArtifacts|TestArtifact|TestCache' ./...
rg -n "NewPersistence|BLEEPHUB_PERSIST|BLEEPHUB_DATA_DIR|sqlite|postgres|database|persistence" bleephub docs README.md
```

Expected first commit shape:

```text
bleephub: add postgresql persistence
```

Adjust the message to match the actual completed work. Do not mention internal
task numbering in the commit message.

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

PR #534 existed for this branch and was green after subtask 1. Subtask 2 has
compile-only validation and whitespace validation locally; the focused runtime
tests still need to be rerun once loopback bind permission is available.

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
