# Sockerless - What We Built

Roadmap [PLAN.md](PLAN.md) - status [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

Detailed historical narrative lives in PR descriptions and `git log`. This file keeps the recent chain plus a compact foundation summary.

## 2026-07-09 - Bleephub Actions Runtime Fidelity (`feat/bleephub-actions-runtime-fidelity`)

This branch continued Bleephub's movement from compatibility-shaped behavior to a real GitHub Enterprise Server-compatible service.

Closed BUG-2391 through BUG-2398 by wiring repository REST/GraphQL metadata to persisted repository, git, Pages, and viewer-access state. Licensed repositories exposed `Repository.licenseInfo`; discussion/issues/wiki settings and merge-method settings flowed through REST and GraphQL; Pages capability, pushed timestamps, archival timestamps, template provenance, and repository permissions stopped using constants or fabricated defaults.

Closed BUG-2399 by rebalancing the AWS Command Line Interface simulator appdata/appdata2 shards while preserving required check names.

Closed BUG-2400 and BUG-2401 by making pull request GraphQL status rollups use both REST commit statuses and check runs, and by adding GitHub's top-level `avatar_url` to REST commit status responses.

Closed BUG-2414 by making release asset upload follow GitHub's raw upload contract. Bleephub now registers the advertised `/api/uploads/repos/{owner}/{repo}/releases/{id}/assets{?name,label}` route, reads metadata from the query string, stores the raw request body with the request content type, and no longer accepts multipart/form fallback bytes.

Closed BUG-2402 through BUG-2405 and BUG-2413 by making Codespaces fail loudly. Codespace records are persisted only after workspace/container creation succeeds; image pull failures do not fall back to `ubuntu:latest`; start/stop/delete return errors on required backend failure; delete preserves state after failed container cleanup; and random-name generation requires cryptographic entropy rather than timestamp fallback.

Closed BUG-2406 by making OAuth device flow require browser approval. Device codes start pending, token polling returns `authorization_pending` until approval, and the final token belongs to the approving logged-in user.

Closed BUG-2407 by snapshotting code-quality setup records at the store boundary so failed validation cannot mutate persisted setup state through escaped slices or timestamps.

Closed BUG-2408 through BUG-2412 by removing fabricated Actions repository/ref/SHA context. Repository-scoped workflow paths resolve refs through real git storage and reject unresolved refs; repo-less internal submissions omit repository context instead of claiming `bleephub/test`; missing repository scope fails job/message construction loudly; webhook test deliveries require a real default-branch commit; and run-control tests seed real repositories before exercising repo-scoped runs.

Closed BUG-2415 by making the Bleephub runner UI use GitHub's repository-scoped Actions runners REST endpoint for its primary inventory. The page no longer fetched `/internal/sessions`, and its coverage asserted the public repository and runner routes while rejecting internal session access.

Closed BUG-2416 by replacing unexplained shorthand in Bleephub UI source comments with descriptive dashboard, user profile, and organization page names.

Closed BUG-2417 by making GitHub Pages deployment creation advertise the GitHub-compatible status URL and making status/cancel lookup resolve the public deployment/build identifier as well as the internal record ID.

Closed BUG-2418 by centralizing checked cryptographic randomness for Bleephub token, secret, invite-code, advisory, gist, and OpenID Connect token identifier generation. Ignored `crypto/rand.Read` calls were removed, timestamp fallback identifiers were removed, and a source guard now rejects unchecked entropy reads.

Closed BUG-2419 by making GitHub Actions artifact finalization and signed-download URL lookup use the workflow run backend identifier when it is supplied. Same-name artifacts from concurrent runs no longer cross-finalize or cross-download, matching the existing list scoping.

Closed BUG-2420 by scoping public GitHub Actions run, attempt, job, log, cancel, rerun, delete, artifact, concurrency, and protection endpoints to the repository named in the GitHub REST path. Global workflow run IDs and stable job IDs no longer resolve across repositories after only checking the requested repository's readability.

Closed BUG-2421 by making Bleephub notification thread identity type-safe. Issue and pull request notification threads now use distinct typed IDs, read/done/subscription state keys no longer collide across resource types, advertised notification thread URLs use `/api/v3/notifications/...`, and old numeric-only notification store helpers were removed.

Closed BUG-2422 through BUG-2425 by moving Bleephub account-management, audit-log, and OAuth UI paths off operator-only management routes. Organization and team management now uses GitHub Enterprise Server/public GitHub REST organization and team routes instead of `/internal/orgs` and `/internal/teams`. User administration now uses GitHub Enterprise Server user list/create/delete/site-admin routes instead of `/internal/users`; Bleephub also persists account suspension state and rejects suspended user tokens with `403`. The audit-log page now reads organization audit logs through `/api/v3/orgs/{org}/audit-log` using GitHub's phrase/order query model, and the server applies ascending audit-log order. The OAuth page now starts web/device flows and polls device tokens through `/login/oauth/authorize`, `/login/device/code`, and `/login/oauth/access_token` instead of rendering pending server-side codes from `/internal/oauth/state`.

Closed BUG-2426 by backing Bleephub browser sessions with real stored credentials. `/login` now requires a stored personal access token for the submitted account, rejects arbitrary password strings and mismatched tokens, refuses suspended accounts, and invalidates existing browser sessions when the account becomes suspended. OAuth web-flow consent and device-flow approval therefore run under a real authenticated Bleephub user instead of a login-name-only session.

Closed BUG-2427 by requiring real registered OAuth clients across Bleephub OAuth flows. Device-code issuance now rejects unknown `client_id` values, device-token polling requires the same client ID that issued the code, authorization-code consent requires a registered OAuth App or GitHub App client, and the token exchange validates the matching client secret before minting a user-to-server token.

Closed BUG-2428 by keeping the Bleephub OAuth UI on the same registered-client contract as the service. The OAuth flow controls no longer rely on a fake default client identifier, and the user-entered registered `client_id` is included in the web authorization URL, device-code request, and device-token polling request.

Closed BUG-2429 by fixing hook-discovered stale coverage and dead UI types. The pending-deployment review flow fixture now creates a real workflow file through the public contents API before submitting a repo-scoped workflow, the GitHub Enterprise Server-only user-administration and Pages deployment status routes are explicitly allowlisted in the route-spec guard, and obsolete runner-session TypeScript exports were removed after the runner UI moved to GitHub Actions public runner endpoints.

Closed BUG-2430 by making the local Bleephub Go pre-commit hook truthful during the temporary local Docker outage. The hook now runs the non-Docker Bleephub suite through `scripts/bleephub-go-test-local.sh`, while Docker-backed Codespaces lifecycle coverage remains fail-loud in CI instead of silently pretending the missing local Docker socket was covered.

Validation in this branch included focused Bleephub Go tests for repository metadata, pull request status rollups, commit statuses, release asset upload, Codespaces name/catalog behavior, OAuth device flow, code-quality setup, Actions secrets/variables, workflow dispatch/internal submission, repository webhook test delivery, and run-control fixtures. The latest combined focused command was:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestReleases_AssetLifecycle|TestGenerateCodespaceNameRequiresRandomBytes|TestCodespacesUserMachines_RealCatalogValues' -count=1
```

It passed with sandbox escalation for loopback listeners. Docker-backed Bleephub/Codespaces validation remained delegated to CI while the local Docker-compatible runtime was temporarily unavailable.

The local Bleephub Go pre-commit test script also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache bash scripts/bleephub-go-test-local.sh
```

The runner UI validation also passed:

```bash
bun run test src/__tests__/RunnersPage.test.tsx
bun run typecheck
```

The GitHub Pages deployment validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPagesDeployments_CreateStatusCancel|TestPagesHealthCheck|TestPagesBuildsCRUD|TestPersistenceReload_PagesBuildIDSequence' -count=1
```

The checked-entropy validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestCryptoRandomReadsAreChecked|TestGitHubApp|TestOAuth|TestPagesDeployments_CreateStatusCancel|TestSecurityAdvisories|TestClassroom' -count=1
```

The Actions artifact run-scoping validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestArtifact(CreateUploadFinalize|FinalizeScopesByWorkflowRunBackendID|ListReturnsFinalized|Download)|TestGetSignedArtifactURL' -count=1
```

The Actions repository-scoped run/job validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestActionsRunAndJobEndpointsScopeIDsToPathRepository|TestActionsRuns_(Get|Delete|Cancel)|TestActionsRunJobs_List|TestActionsJobs_(Get|Logs)|TestActionsArtifacts_ListRunArtifacts' -count=1
```

The notification thread identity validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestNotifications_(ListAndRead|ThreadIDsSeparateIssuesAndPullRequests|ThreadSubscription|RepoScoped|SinceAndBefore|ParticipatingFilter)|TestNotificationThreadMarkDone' -count=1
```

The user/organization/team UI route validation also passed:

```bash
bun run test src/__tests__/api.test.ts
bun run typecheck
```

The GitHub Enterprise Server user administration route changes also passed compile-only Go validation:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -c ./bleephub -o /private/tmp/bleephub.test
```

The focused runtime Go test for the user administration routes did not execute locally because the sandbox denied loopback binds and both escalated attempts timed out in the automatic approval reviewer before execution. Docker-backed Bleephub/Codespaces validation remained delegated to CI while the local Docker-compatible runtime was temporarily unavailable.

The audit-log public-route validation passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestAuditLogRecords' -count=1
```

The OAuth UI endpoint validation also passed:

```bash
bun run test src/__tests__/OAuthPage.test.tsx
```

The browser-authentication validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestOAuth_(LoginPost|Authorize|WebFlow|DeviceFlow|TokenResponse)|TestGHDeviceFlow' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -c ./bleephub -o /private/tmp/bleephub.test
```

The registered OAuth client validation used the same focused command and compile gate after the token endpoint required registered client IDs and web-flow client secrets:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestOAuth_(LoginPost|Authorize|WebFlow|DeviceFlow|TokenResponse)|TestGHDeviceFlow' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -c ./bleephub -o /private/tmp/bleephub.test
```

The OAuth UI registered-client validation also passed:

```bash
bun run test src/__tests__/OAuthPage.test.tsx
```

The hook-discovered fixture/spec/type cleanup also passed focused validation:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestActionsPendingDeploymentReviewFlow|TestRegisteredAPIv3RoutesExistInGitHubSpec' -count=1
npx knip
```

## Recent Merged Context

- **#782 - Persist Bleephub repository metadata and permissions from real state.** Repository license/settings/Pages/pushed/archive/template/permissions fields moved to real persisted state; AWS Command Line Interface simulator shard balance was corrected without changing required contexts.
- **#781 - Bleephub GitHub Apps, Actions, storage, and repository fidelity.** Actions artifacts/caches/logs moved to object storage; S3 filesystem tests used the AWS simulator; GitHub Apps moved to Manifest/browser installation flows; public Actions runner/workflow paths replaced internal paths; metadata persistence became SQLite-only.
- **#779 - Bleephub pull request/release fidelity.** Pull requests, reviews, releases, action downloads, CodeQL fixtures, and repository rename/transfer/delete behavior derived from real git/object storage and public GitHub-compatible paths.
- **#778 - Open issue sweep and class hardening.** Fixed the actionable open issues except upstream-blocked AzureAD and tightened mutable store snapshots across simulators.
- **#774/#773 - Bleephub UI and stress hardening.** The UI became a functional GitHub clone, docs were swept, fuzz/load/concurrency coverage found races and scale bugs, and store/indexing hot paths were hardened.
- **#770/#750/#747 - Bleephub API/UI expansion.** Large REST/UI parity waves added many GitHub surfaces and pages; old operation-count detail lives in those PRs.

## Foundation Summary

- Docker-compatible cloud backends are stateless and map Docker concepts onto cloud primitives.
- AWS, GCP, and Azure simulators are real cloud API slices with conformance/coverage ratchets and official client coverage.
- Bleephub implements GitHub Enterprise Server-shaped REST, GraphQL, Actions, GitHub Apps/OAuth, repositories, issues, pull requests, releases, packages, webhooks, checks/statuses, Pages, and UI surfaces, with more fidelity work still active.
- GitHub Actions runner and GitLab docker-executor topologies are sim-proven across container-capable backends; live-cloud validation remains open under BUG-1075.
