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

Closed BUG-2430 by making the local Bleephub Go pre-commit hook truthful during the temporary local Docker outage. During the outage, the local hook ran the non-Docker Bleephub suite while Docker-backed Codespaces lifecycle coverage remained fail-loud in CI instead of silently pretending the missing local Docker socket was covered.

Closed BUG-2431 by upgrading the stale AWS and Google Cloud Go modules surfaced by pre-push dependency freshness. The affected Amazon EC2 software development kit, Google API client, and Google Cloud Firestore module pins were brought to their latest published versions, and dependency freshness passed again.

Closed BUG-2432 by removing hidden admin-owned identity defaults from GitHub App seed configuration. Seeded GitHub Apps now require an explicit existing owner user, installations require an existing target user or organization with a matching target type, persisted app owners are validated on load, and app JSON no longer fabricates a Simple User when app owner state is corrupt.

Closed BUG-2433 by renaming the Bleephub runner integration harness's Google Cloud service-account credential generation from fake service-account JSON to simulator service-account JSON. The harness still generated a real RSA key and drove the Google client JWT signing and token exchange path, with only the token endpoint coordinate pointed at the simulator.

Closed BUG-2434 by restoring the local Bleephub Go pre-commit hook to the full Bleephub suite after Docker compatibility returned on the host. The temporary non-Docker skip script was removed, so Docker-backed Codespaces coverage ran locally again instead of being deferred to CI.

Closed BUG-2435 by making Docker-backed Make targets load local images correctly across Docker frontends. The shared build helper uses `docker buildx build --load` when Buildx is available and legacy `docker build` otherwise, so smoke, Bleephub runner, Bleeplab runner, and Bleephub `gh` command-line interface harness images are available to the following `docker run` step under Docker Engine and Podman compatibility.

Closed BUG-2436 by correcting the Bleephub `gh` command-line interface documentation to name the actual required `Bleephub GitHub command-line interface` CI job.

Closed BUG-2437 by making GitHub Actions workflow dispatch resolve GitHub `ref` inputs through git storage the way official clients send them. Dispatch now accepts full refs, branch names such as `main`, tag names, and raw commit SHAs, stores the resolved ref/SHA on the workflow run, and still returns a loud `422` for unresolved refs. The real `gh workflow run ci.yml --ref main` path passed in the Docker-backed command-line interface harness.

Closed BUG-2438 by removing the remaining user-facing Bleephub UI dependency on operator-only metrics/status/storage diagnostics. The overview and metrics pages now aggregate workflow runs, jobs, job conclusions, and online runners through public GitHub REST repository Actions routes; tests assert those pages do not call `/internal/metrics`, `/internal/status`, or `/internal/storage`. The storage-coordinate page was removed from the routed UI instead of wrapping non-GitHub persistence details in a user-facing product surface.

Closed BUG-2439 by deleting the dead `formatUptime` helper after process uptime stopped appearing in user-facing Bleephub pages.

Closed BUG-2440 by splitting the Bleephub production UI bundle at real route and dependency boundaries. `App.tsx` lazy-loads page modules through the router, and Vite now emits explicit vendor chunks for React, TanStack, YAML, cryptography, and miscellaneous third-party code without raising Vite's chunk warning threshold. The production build no longer emits large-chunk or circular-chunk warnings.

Closed BUG-2442 by updating Bleephub Playwright end-to-end coverage to the public GitHub Actions metrics contract. The Operations console now expects the `Workflow runs` metrics label exactly, the metrics page checks the `GitHub Actions throughput` heading, and fault-injection coverage fails `/api/v3/user/repos` instead of the removed `/internal/metrics` diagnostic route.

Closed BUG-2443 by making the AWS simulator's Amazon Simple Queue Service `ReceiveMessage` honor long polling. Empty receives now wait up to `WaitTimeSeconds`, available messages still return immediately, and invalid wait times outside the real 0-20 second range fail loudly. The AWS SDK test harness now runs the main simulator at warning level so successful request traffic cannot flood CI logs.

Closed BUG-2444 by adding the missing AWS Budgets CloudTrail event-source mapping. AWS Budgets management calls now record the real `budgets.amazonaws.com` event source instead of emitting fail-loud "no eventSource mapping" warnings, and the mapping unit coverage pins the service prefix.

Closed BUG-2445 by exposing GraphQL `Release.immutable` from the same persisted immutable-release state used by the REST endpoints. Repository release connections, release-by-tag lookup, and latest-release lookup now derive the field from repository-level toggles plus organization all/selected enforcement instead of hiding the field to make official clients fall back.

Closed BUG-2446 by making GraphQL pull request status-rollup connections expose the official GitHub command-line interface count-by-state fields from the same commit-status and check-run stores that back the node list. Actions-created check suites now persist their workflow-run identifiers, workflow name, and workflow file metadata, so `CheckRun.checkSuite.workflowRun.workflow.name` resolves from real Actions state instead of returning null.

Closed BUG-2448 by updating the GraphQL sweep test header to name GitHub command-line interface version 2.96 as the source for the replayed GraphQL shapes used by the current status-rollup coverage.

Closed BUG-2447 by persisting GitHub Actions workflow runs and archived attempts in SQLite. Run creation, dispatch state transitions, cancellation, deployment review, rerun archive/restore, startup-failure runs, repository rename/delete, and run deletion now keep the durable run records coherent; non-terminal runs reload as completed/cancelled because runner dispatch state is process-local and cannot truthfully continue after a service restart.

BUG-2441 stayed open because the current Bleephub UI unused-export toolchain still emitted Node's `DEP0205 module.register()` warning after `knip` was upgraded from 6.15.0 to the current 6.23.0 release. The gate passed and dependency freshness showed no newer `knip` version.

Validation in this branch included focused Bleephub Go tests for repository metadata, pull request status rollups, commit statuses, release asset upload, Codespaces name/catalog behavior, OAuth device flow, code-quality setup, Actions secrets/variables, workflow dispatch/internal submission, repository webhook test delivery, and run-control fixtures. The latest combined focused command was:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestReleases_AssetLifecycle|TestGenerateCodespaceNameRequiresRandomBytes|TestCodespacesUserMachines_RealCatalogValues' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPRGraphQL_ViewDefaultFields|TestPersistenceReload_CheckRunsAndSuites' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_WorkflowRunsAndAttempts|TestWorkflowRunsListNewestFirst|TestActionsRuns_(Get|Delete|Cancel)|TestActionsRunJobs_List|TestRerunWorkflowJob_NewAttemptCarriesOtherJobs|TestApproveWorkflowRun_ReleasesGatedRun' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1
```

They passed with sandbox escalation for loopback listeners.

The full Bleephub Go pre-commit test command passed after Docker compatibility returned:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./... -count=1 -timeout 300s
```

The Bleephub UI validation passed after the public Actions metrics and route-level code-splitting changes:

```bash
bun run test src/__tests__/OverviewPage.test.tsx src/__tests__/MetricsPage.test.tsx
bun run test
bun run typecheck
bun run build
npx knip
bun outdated knip
```

`npx knip` exited successfully but still emitted the Node `DEP0205 module.register()` warning tracked as BUG-2441. `bun run build` completed without Vite large-chunk or circular-chunk warnings.

The Docker-backed Bleephub `gh` command-line interface parity harness passed with the Docker-compatible Podman runtime:

```bash
make bleephub-gh-docker-test
```

The focused Bleephub Playwright coverage for the public Actions metrics UI and error paths passed after rebuilding the embedded UI binary:

```bash
bun run test:e2e -- e2e/bleephub.spec.ts --grep "Operations console|Global navigation|Metrics page"
bun run test:e2e -- e2e/errorPaths.spec.ts
```

The focused AWS simulator validation for the Amazon SQS long-polling and CloudWatch-to-Amazon SQS path passed:

```bash
GOWORK=off CGO_ENABLED=0 go test -v -count=1 -timeout 180s -run 'TestSQS_ReceiveMessageHonorsLongPollingWaitTime|TestSQS_ReceiveMessageRejectsInvalidWaitTimeSeconds|TestCloudWatch_OKActionsDispatchedToSNS' .
```

The full AWS simulator software development kit target passed with the Docker-compatible Podman runtime:

```bash
make sdk-test SDK_TEST_TIMEOUT=600s
```

The AWS simulator CloudTrail event-source mapping unit test passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off CGO_ENABLED=0 go test -v -count=1 -run TestAWSEventSourceCoversAllServiceSlices .
```

The focused AWS simulator software development kit rerun for AWS Budgets, process-mode CloudWatch/SNS/SQS, process-mode Amazon Elastic Container Service managed Amazon Elastic Block Store, and Amazon SQS long polling passed:

```bash
GOWORK=off CGO_ENABLED=0 go test -v -count=1 -timeout 180s -run 'TestBudgetsCRUDSDK|TestECS_ManagedEBSRunTaskProcessMode|TestCloudWatch_AlarmSNSActionToSQS_ProcessMode|TestSQS_ReceiveMessageHonorsLongPollingWaitTime' .
```

The GraphQL release immutable-state validation passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestRepoGraphQL_ReleasesConnection|TestImmutableReleases_OrgSettingsAndRepoEnforcement|TestImmutableReleases_SelectedRepositories' -count=1
```

The full Bleephub Go package test also passed after the GraphQL release schema change:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1
```

The workflow-dispatch `ref` input validation passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestWorkflows_Dispatch' -count=1
```

The Docker-backed `gh` command-line interface parity harness passed:

```bash
make bleephub-gh-docker-test
```

The dependency freshness hook also passed:

```bash
bash scripts/check-latest-deps.sh
```

The GitHub App seed validation also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestSeedPreRegisteredApp|TestSeedAppIdempotentAndBadKey|TestPersistence_RoundTripAppsInstallationsTokensRepos' -count=1
```

The runner harness shell syntax check also passed:

```bash
bash -n bleephub/test/run-integration.sh
```

The full local Bleephub Go hook command also passed:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./... -count=1 -timeout 300s
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

The focused runtime Go test for the user administration routes did not execute locally because the sandbox denied loopback binds and both escalated attempts timed out in the automatic approval reviewer before execution.

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
