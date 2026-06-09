# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current Branch

Branch: `bleephub-parity-storage`.

This branch is the single working branch for the Bleephub parity, durability,
storage, UI, and docs PR. Keep one PR open for this branch. Make one natural
commit per subtask, with tests and continuity docs included in the same commit.

## Last Completed Subtask

Subtask 1 completed:

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

Verified:

```bash
cd bleephub && GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test -run 'TestCache|TestUnknownRoutesDoNotReturnSuccess' ./...
cd bleephub && GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./...
```

## Current Subtask

Subtask 2: real Actions cache behavior and artifact indexing.

The cache now stores real data, but the next slice should finish the user-facing
Actions cache/artifact behavior that callers see through REST and runner flows:

- Wire artifact list endpoints in
  [bleephub/gh_actions_extras.go](bleephub/gh_actions_extras.go) to
  [bleephub/artifacts.go](bleephub/artifacts.go) instead of returning empty
  lists.
- Preserve run/repo linkage for artifacts created through the Twirp API, then
  make run-level and repo-level artifact REST lists return real artifacts.
- Add download/delete metadata surfaces if the real GitHub REST artifact API path
  already exists in docs but Bleephub lacks it.
- Audit whether cache keys need repo/ref/scope fields from the runner request
  headers or query parameters. Add those fields if the official runner/client
  sends them, and avoid global cache leakage across repos.
- Audit public Bleephub-specific names while touching these paths. Externally
  observable API paths, request fields, response fields, runner parameters,
  workflow environment variables, and UI text must use the GitHub/GHES names
  real clients expect, including `GITHUB_*` variables. Keep `bleephub` names only
  for internal code or explicit operator-only management surfaces.
- Add focused tests that exercise the same mux paths real clients use.

First commands for the next session:

```bash
git status --short --branch
rg -n "handleRunArtifacts|handleRepoArtifacts|ArtifactStore|WorkflowRunBackendID|RunID|artifactcache|actions/artifacts|GITHUB_|bleephub" bleephub ui/packages/bleephub
cd bleephub && GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test -run 'TestArtifact|TestCache|TestActions.*Artifact' ./...
```

Expected first commit shape:

```text
bleephub: return real actions artifacts
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

No Bleephub implementation changes have been made on this branch yet. The last
known repository state was clean on `main` before this branch was created. Run
the Bleephub Go tests at the start of implementation to establish the local
baseline.

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
