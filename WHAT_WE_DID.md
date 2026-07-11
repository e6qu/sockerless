# Sockerless - What We Built

Roadmap [PLAN.md](PLAN.md) - status [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

Detailed historical narrative lives in PR descriptions and `git log`. This file keeps the recent chain plus a compact foundation summary.

## 2026-07-11 - Bleephub Public State Fidelity (`feat/bleephub-public-state-fidelity`)

This branch continued from merged #787, which moved CodeQL variant-analysis query-pack tarballs to object storage and made runner-log object-store failures preserve live process state.

Closed BUG-2491 by making persisted repository ownership strict. Repository reload now requires valid `owner_type` and `owner_id`, loads organizations before repositories so organization-owned repositories validate against real organization state, and fails loudly when persisted owner data is missing or inconsistent. Public repository listing and event paths no longer treat empty owner types as user repositories.

Closed BUG-2492 by removing the internal runner-submission image fallback. `/internal/exec/submit` and `/internal/exec/workflow` now require either an explicit `image` or `hostMode`, and tests that intend container execution pass `alpine:latest` explicitly instead of relying on hidden server-side defaulting.

Validation in this branch included:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(PersistenceReload_(OwnerAndCountersAndState|OrganizationRepositoryOwnerIsValidated|RepositoryMissingOwner(Type|ID)FailsLoud)|InternalSubmit(Job|Workflow)RequiresExplicitImageOrHostMode)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(ConcurrencyGroups_(RepoAndRunEndpoints|CompletedRunReleasesLease)|SubmitWorkflow(RepoRefResolution|RejectsUnresolvedRepoRef)|Workflows_Dispatch|InternalSubmit(Job|Workflow)RequiresExplicitImageOrHostMode)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
```

## 2026-07-11 - Bleephub CodeQL Variant-Analysis Query Pack Objects (`feat/bleephub-codeql-variant-query-pack-objects`)

This branch continued from merged #786, which moved more Bleephub service bytes to object storage and hardened public GitHub-compatible ingestion, deletion, and official-client coverage.

Closed BUG-2489 by moving CodeQL variant-analysis query-pack tarballs out of SQLite metadata and into the configured object byte store. Variant-analysis rows now persist controller, actor, language, target, status, query-pack size, and object-key metadata; public query-pack downloads read the object store; persistent stores fail loudly without `BLEEPHUB_OBJECT_S3_BUCKET`; and controller-repository deletion purges query-pack objects before deleting repository metadata.

Closed BUG-2490 by making GitHub Actions runner-log upload and run-log deletion complete required object-store writes/deletes before mutating in-memory log, console, or timeline state. Fail-loud object-store errors now preserve the previously visible process state instead of leaving live state diverged from durable object storage.

Validation in this branch included:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(LogfilesUpload_(WritesObjectStore|ObjectStoreFailurePreservesState|AppendsBlocks|CapsAtFourMiBWithMarker)|JobLogs_ReadsUploadedLogFilesFromObjectStore|RunLogsDelete_ObjectStoreFailurePreservesState|ActionsRuns_DeleteLogs)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeQLVariantAnalyses_|PersistentServerStorageRequiresDurableGitAndObjectBytes|AgentsCodeScanPersistenceReload|PersistenceReload_(DeleteRepoLeavesNoResidue|RenameRepoMovesRepoScopedMetadata|TransferRepoMovesRepoScopedMetadata))' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
bash -n scripts/bleephub-local-dev.sh
git diff --check
pre-commit run --all-files
```

## 2026-07-10 - Bleephub Object-Backed Service Bytes (`feat/bleephub-object-backed-service-bytes`)

This branch continued from merged #785, which cleaned Codespace runtime/workspace state during repository deletion, hardened the AWS SDK simulator CI shard, and made persisted Bleephub require object-backed GitHub Actions artifacts, dependency caches, and runner logs.

Closed BUG-2471 by extending the object-backed byte-storage contract to release assets and GitHub Packages. Release asset uploads, package version files, and GitHub Container Registry blobs now write through the configured S3-compatible object store when it is present; SQLite stores metadata and object keys. Persisted startup and local development documentation now describe `BLEEPHUB_OBJECT_S3_BUCKET` as the required store for all durable service bytes, and release asset object-delete failures surface through the API and repository deletion path instead of being ignored.

Closed BUG-2472 by making public GitHub Packages file downloads read object-backed package file bytes. The metadata/listing path and the byte-serving path now use the same object storage source, so advertised package file REST URLs work for object-backed service bytes instead of looking only for local filesystem paths.

Closed BUG-2473 by making repository deletion fail loudly on required git-storage cleanup failures. Bleephub now purges filesystem or S3-backed git storage before deleting repository metadata; if S3 git-prefix cleanup cannot be resolved or completed, the delete returns an error and preserves the repository record and git storage index instead of logging and orphaning git objects.

Closed BUG-2486 by making repository deletion purge repository-owned GitHub Packages file bytes before deleting repository metadata. Object-backed and local package file bytes now go through the required cleanup path, so package byte cleanup failures surface as repository-delete errors instead of leaving durable package objects behind after package metadata is gone.

Closed BUG-2487 by moving GitHub CodeQL database archive bytes out of SQLite metadata and into the configured object byte store. CodeQL database rows now persist metadata, size, and object keys; public archive downloads read the object store; database deletion removes the object before deleting metadata; and repository deletion purges repository-owned CodeQL database archive objects before deleting repository metadata.

Closed BUG-2488 by moving artifact attestation Sigstore bundle bytes out of SQLite metadata and into the configured object byte store. Attestation rows now persist repository linkage, subject digests, predicate type, initiator, timestamps, and object keys; repository, organization, and user attestation list endpoints read bundle JSON from object storage; public attestation deletion removes object bytes before metadata; and repository deletion purges repository-owned attestation bundle objects before deleting repository metadata.

Closed BUG-2474 by upgrading stale AWS software development kit service modules found by the pre-push dependency freshness gate. The Amazon Elastic Container Service backend, AWS Lambda backend, and AWS simulator software development kit tests now use the latest published CloudWatch, Amazon Elastic Compute Cloud, and AWS Lambda service module versions required by the gate.

Closed BUG-2475 by moving the Bleephub go-github software development kit harness's organization provisioning onto GitHub Enterprise Server's public admin organization API. The SDK tests no longer create organizations through `/internal/orgs`, and source coverage rejects that operator-only route in the official-client harness.

Closed BUG-2476 by moving Bleephub public GitHub REST test organization setup onto GitHub Enterprise Server's public admin organization API. Public feature tests now use a shared `/api/v3/admin/organizations` helper for prerequisite organizations, while the only remaining direct `/internal/orgs` organization-creation calls are explicit operator-management coverage; a source guard rejects new direct public-test setup calls to the operator route.

Closed BUG-2477 by moving Bleephub public code scanning alert setup onto GitHub's public SARIF upload route. The shared code scanning alert helper now uploads SARIF to `/api/v3/repos/{owner}/{repo}/code-scanning/sarifs`, live-shape and campaign coverage use that public ingestion path, and SARIF rule severity/description metadata now flows into persisted alert state for filtering and downstream features.

Closed BUG-2478 by making the Bleephub UI typecheck pre-commit hook rebuild `@sockerless/ui-core` declarations before checking Bleephub. The hook clears stale ignored incremental build state, emits the required declarations, and then runs Bleephub `tsc`, so cleaning generated `dist` output no longer leaves the hook dependent on manual repair.

Closed BUG-2479 by making Bleephub secret scanning derive alerts from real repository content. Contents API writes now scan the new commit for supported provider secret patterns, Git Database branch reference creation/update scans commit targets, alert locations persist real commit/blob/path coordinates, and public secret scanning tests use committed secret patterns instead of an internal operator alert seed route.

Closed BUG-2480 by removing the undocumented `node_id` field from Git Database blob-create responses. `POST /api/v3/repos/{owner}/{repo}/git/blobs` now matches the OpenAPI response-shape ratchet.

Closed BUG-2481 by making Bleephub Dependabot alerts derive from public dependency graph snapshots and published security advisories. Repository security advisories now persist GitHub vulnerability package coordinates; successful default-branch dependency snapshots create matching Dependabot alerts from the global advisory database; publishing an advisory creates alerts from already submitted dependency snapshots; and the old operator-only Dependabot alert seed endpoint was removed.

Closed BUG-2482 by making the AWS simulator software development kit Amazon Elastic Container Service long-running task test poll real task state through `DescribeTasks`. `TestECS_TaskNoCommandStaysRunning` no longer assumes task startup completed after a fixed sleep, while still asserting that the no-command task reaches and remains `RUNNING` without container exit codes.

Closed BUG-2483 by making Bleephub secret scanning push protection mint bypass placeholders from protected public writes. Public contents writes and Git Database branch reference creation/update now detect enabled provider patterns before mutating git state, return a `422` push-protection response with a placeholder, honor active public bypasses for the matched token type, and no longer expose the internal operator placeholder seed route.

Closed BUG-2484 by removing the obsolete internal code scanning alert seed endpoint. Code scanning alert tests and downstream campaign/autofix coverage already created alert state through GitHub's public SARIF upload route, so `/internal/repos/{owner}/{repo}/code-scanning/alerts` no longer existed in the route table, and source guards rejected reintroducing that operator shortcut in either tests or server registration.

Closed BUG-2485 by removing the obsolete internal secret scanning alert seed endpoint. Secret scanning alert tests already created alert state from committed repository content and Git Database branch reference writes, so `/internal/repos/{owner}/{repo}/secret-scanning/alerts` no longer existed in the route table, and source guards rejected reintroducing that operator shortcut in either tests or server registration.

Validation in this branch included:

```bash
bash -n scripts/bleephub-local-dev.sh
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistentServerStorageRequiresDurableGitAndObjectBytes|TestReleases_AssetBytesUseObjectStore|TestPackageAndRegistryBytesUseObjectStore|TestContainerRegistryPublishCreatesPackageVersion|TestReleases_AssetLifecycle|TestDeleteRepo' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPackageAndRegistryBytesUseObjectStore|TestContainerRegistryPublishCreatesPackageVersion|TestPackages_' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'Test(DeleteRepoS3GitCleanupFailurePreservesRepo|GitDeleteCleanup|UnitDeleteRepo)$' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(DeleteRepoPurgesRepositoryPackageObjectBytes|PackageAndRegistryBytesUseObjectStore|PersistenceReload_DeleteRepoLeavesNoResidue)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(Packages_|ContainerRegistry|PackageAndRegistryBytesUseObjectStore|DeleteRepoPurgesRepositoryPackageObjectBytes|PersistenceReload_DeleteRepoLeavesNoResidue|DeleteRepo)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeQLDatabases_(RoundTrip|BytesUseObjectStore)|AgentsCodeScanPersistenceReload|PersistenceReload_(DeleteRepoLeavesNoResidue|RenameRepoMovesRepoScopedMetadata))' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeQL|CodeScanning|RegisteredAPIv3RoutesExistInGitHubSpec|FuzzRoutePatternsMatchRegisteredRoutes|PersistenceReload_DeleteRepoLeavesNoResidue|DeleteRepo)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(RepoAttestations_|OrgAttestations_|UserAttestations_|Attestations_CursorPagination|ArtifactMetadataAndAttestationPersistenceReload|PersistenceReload_DeleteRepoPurgesIssueAndPullChildren|PersistentServerStorageRequiresDurableGitAndObjectBytes)' -count=1
bash -n scripts/bleephub-local-dev.sh
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1
bash scripts/check-latest-deps.sh
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestGitHub(CommandLineInterface|SoftwareDevelopmentKit)HarnessUsesAdminOrganizationAPI|TestAdminCreateOrg' -count=1
(cd bleephub/sdk-tests && GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test -run 'Test(Organizations|AppsInstallationTokenFlow|OrgProfileTeamsAndMembershipSurfaces|OrgWebhooksSDK)$' -count=1)
(cd bleephub/sdk-tests && GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test -count=1)
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestPublicFeatureTestsProvisionOrganizationsThroughAdminAPI|Test(GetOrg|UpdateOrg|DeleteOrg|ListAuthUserOrgs|CreateTeam|ListTeams|GetTeam|DeleteTeam|OrgMembership|RemoveMembership|TeamRepoPermission|ListUserTeams|GraphQLViewerOrgs|GraphQLOrganization|CreateOrgRepo|CreateOrgRepoExtended|ListOrgRepos|RepoOrganizationField|OpenAPIOrg|GetRepoInstallationHTTP|InviteFlow|PublicizeAndConcealMembership|OrgProfileTeamsAndMembershipSurfaces|OrgWebhooks|Codespaces|AppsInstallationTokenFlow|CreateRepositoryInOrganization|Actions.*Org)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeScanning(AlertTestsUsePublicSARIFUpload|_ListAndFilter|_GetAndInstances|_PatchDismiss|_InvalidDismissedReason|_SARIFUploadCreatesAlerts|OrgAlerts|Autofix|AutofixEligibility)|LiveCodeScanning_FullFlow|OrgCampaigns)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
pre-commit run ui-typecheck-bleephub --all-files
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestSecretScanning|TestLiveSecretScanning_CRUD' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestGitData|TestUpdateRef|TestSecretScanning_GitDatabaseRefCreatesAlert|TestGetBlob|TestCreateBlob|TestListRefs|TestGetRef' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestDependabot|TestLiveDependabot|TestEnterpriseDependabot|TestDependencyGraph|TestGlobalSecurityAdvisories|TestSecurityAdvisories' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
(cd simulators/aws/sdk-tests && GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off CGO_ENABLED=0 go test -v -count=1 -timeout 180s -run TestECS_TaskNoCommandStaysRunning .)
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestSecretScanning|TestGitData|TestUpdateRef|TestCreateRef|TestCreateBlob|TestListRefs|TestGetRef' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestRegisteredAPIv3RoutesExistInGitHubSpec|TestFuzzRoutePatternsMatchRegisteredRoutes|TestSecretScanning|TestGitData|TestUpdateRef|TestCreateRef|TestCreateBlob|TestListRefs|TestGetRef' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeScanningAlertTestsUsePublicSARIFUpload|RegisteredAPIv3RoutesExistInGitHubSpec|FuzzRoutePatternsMatchRegisteredRoutes|CodeScanning|LiveCodeScanning|OrgCampaigns)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(SecretScanningAlertTestsUseCommittedContent|RegisteredAPIv3RoutesExistInGitHubSpec|FuzzRoutePatternsMatchRegisteredRoutes|SecretScanning|LiveSecretScanning|GitData|UpdateRef|CreateRef|CreateBlob|ListRefs|GetRef)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1
pre-commit run --all-files
```

## 2026-07-10 - Bleephub Repository Codespace Cleanup (`feat/bleephub-repository-codespace-cleanup`)

This branch continued from merged #784, which closed the broad Bleephub repository-deletion durable-state cascade for issue/pull request child state, repository-ID keyed rows, selected-repository references, deployment state, environment state, GitHub Pages deployment records, team grants, artifact metadata, source import records, dependency snapshots, SBOM exports, enterprise Dependabot repository-access IDs, labels, milestones, and reaction parent buckets.

Closed BUG-2468 by making repository deletion clean Codespace runtime state before deleting repository records. Repository deletion now removes backing Codespace containers and workspace directories through the same fail-loud path as direct Codespace deletion, and REST/GraphQL callers surface cleanup failures instead of deleting only the SQLite row.

Closed BUG-2469 by hardening the `sim (aws sdk)` CI job against GitHub-hosted runner disk exhaustion. The job now frees regenerable Go/Docker/apt caches before the large AWS SDK simulator shard, runs the prebuilt SDK test binary directly, and passes the prebuilt simulator binary into the SDK test harness instead of rebuilding the simulator during execution.

Closed BUG-2470 by making persisted Bleephub require object-backed GitHub Actions byte storage. Startup now refuses SQLite persistence unless the Actions artifact/cache/log byte store has been initialized from `BLEEPHUB_OBJECT_S3_BUCKET`, so a restarted service cannot advertise durable CI/CD records whose bytes lived only in memory or local files. The local development launcher fails loudly until object-store coordinates are supplied, and the persistence documentation now names the same requirement.

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

Closed BUG-2449 by returning fail-loud GitHub API errors for public secure-random generation paths that had still panicked. GitHub App manifest conversion, seeded GitHub App secrets, OAuth App creation, OAuth web/device token issuance, installation tokens, gist create/update/fork identifiers, security advisory and CVE identifiers, Classroom invite codes, OpenID Connect signing keys/token IDs, hosted-compute network settings/configuration IDs, GitHub Actions runner registration/removal tokens, and Actions cache download tokens now propagate entropy failures to their HTTP handlers; cache reservation avoids creating partial cache records when token generation fails.

Closed BUG-2450 by moving OAuth App token reset and scoped-token creation onto the error-returning user-to-server token path. Reset now mints the replacement before revoking the original token, so entropy or persistence failure returns a fail-loud GitHub API error without destroying the existing credential.

Closed BUG-2451 by moving the Docker-backed `gh` command-line interface parity harness's organization provisioning onto GitHub Enterprise Server's public admin organization API. The harness no longer calls `/internal/orgs`, and Go coverage now rejects that operator-only route in the official-client harness.

Closed BUG-2452 by making the Bleephub enterprise UI consume the configured enterprise slug at runtime. `/health` now reports the service's `BLEEPHUB_ENTERPRISE_SLUG`, Enterprise page copy displays that slug, and all enterprise UI REST helpers build `/api/v3/enterprises/{enterprise}/...` paths from that runtime coordinate instead of hardcoding the default `bleephub` slug.

Closed BUG-2453 by removing the Bleephub UI test setup's localStorage warning source. The setup now installs jsdom localStorage without first touching Node's warning-producing localStorage getter, so localStorage-backed auth paths still run and Vitest output stays clean.

Closed BUG-2454 by moving fine-grained personal access token generation onto an injectable full-read entropy helper. The helper now returns a normal error when secure randomness is unavailable, and entropy failure is covered directly alongside the other credential helpers.

Closed BUG-2455 by persisting Bleephub gist state in SQLite-backed service storage. Gists, comments, stars, forks, histories, comment counters, and sequence counters now reload as durable service state instead of disappearing on process restart.

Closed BUG-2456 by replacing the stale Bleephub persistence bucket inventory comment with a pointer to the actual `loadBucket` registrations. The code no longer carried a duplicate manual list that drifted when durable state buckets changed.

Closed BUG-2457 by making repository deletion purge the persisted child state attached to the deleted repository's issues and pull requests. Issue comments, issue events, sub-issue links, issue dependency links, pull request reviews, and pull request review comments no longer survived a SQLite reload or attached to a later repository that reused issue or pull request IDs.

Closed BUG-2458 by extending repository deletion to repository-ID keyed rows and selected-repository references. Artifact attestations, repository activity, clone traffic, watch subscriptions, GitHub App selected repositories, installation token repository scopes, organization Actions settings, runner groups, Actions secrets/variables, agent secrets/variables, Dependabot access and org secrets, Codespaces org secrets, Copilot coding-agent permissions, private registries, immutable-release enforcement, and code-security attachments no longer survived deletion with the old repository ID.

Closed BUG-2459 by extending repository deletion to deployments, deployment statuses, environments, environment branch policies, environment protection rules, and GitHub Pages deployment records. Those repository-ID keyed rows no longer survived SQLite reload or attached to a later repository that reused the old ID.

Closed BUG-2460 by making deployment deletion purge the deployment's status rows from memory and SQLite with the deployment record.

Closed BUG-2461 by moving team repository access lists and permission overrides during repository rename and transfer, and by removing those team grants when the repository is deleted.

Closed BUG-2462 by moving organization artifact storage/deployment metadata `github_repository` references during repository rename and transfer, and by deleting artifact metadata rows for a deleted repository.

Closed BUG-2463 by adding source imports, dependency graph snapshots, generated SBOM exports, and enterprise Dependabot repository-access IDs to the repository deletion cascade.

Closed BUG-2464 by adding Copilot coding agent tasks, issue field values, and CodeQL variant-analysis target rows to the repository deletion cascade. Deleted repositories and their issues no longer left those durable rows behind for a reloaded or recreated repository to inherit.

Closed BUG-2465 by adding Projects v2 content items to the repository deletion cascade and by making project deletion clear its in-memory content index. Deleted repository issues and pull requests no longer left project items behind after reload or ID reuse.

Closed BUG-2466 by adding notification state to repository deletion and rename cascades. Deleted issue and pull request threads no longer left read, done, or subscription state behind for later ID reuse, and repository rename/transfer moved repo-scoped notification read markers to the new full name.

BUG-2441 stayed open because the current Bleephub UI unused-export toolchain still emitted Node's `DEP0205 module.register()` warning after `knip` was upgraded from 6.15.0 to the current 6.23.0 release. The gate passed and dependency freshness showed no newer `knip` version.

Validation in this branch included focused Bleephub Go tests for repository metadata, pull request status rollups, commit statuses, release asset upload, Codespaces name/catalog behavior, OAuth device flow, code-quality setup, Actions secrets/variables, workflow dispatch/internal submission, repository webhook test delivery, and run-control fixtures. The latest combined focused command was:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestReleases_AssetLifecycle|TestGenerateCodespaceNameRequiresRandomBytes|TestCodespacesUserMachines_RealCatalogValues' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPRGraphQL_ViewDefaultFields|TestPersistenceReload_CheckRunsAndSuites' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_WorkflowRunsAndAttempts|TestWorkflowRunsListNewestFirst|TestActionsRuns_(Get|Delete|Cancel)|TestActionsRunJobs_List|TestRerunWorkflowJob_NewAttemptCarriesOtherJobs|TestApproveWorkflowRun_ReleasesGatedRun' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestEntropyHelpersReturnErrors|TestCryptoRandomReadsAreChecked|TestCreateGist|TestGitHubApp|TestOAuth|TestSecurityAdvisories|TestClassroom|TestActionsOIDC' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestOAuth(App|Check|Reset|Revoke|Scope)|TestEntropyHelpersReturnErrors|TestCryptoRandomReadsAreChecked' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestGitHubCommandLineInterfaceHarnessUsesAdminOrganizationAPI|TestAdminCreateOrg|TestCreateOrg|TestListAuthUserOrgs' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestExistingRoutesUnaffected|TestGHApiRoot' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestCryptoRandomReadsAreChecked|TestEntropyHelpersReturnErrors|TestOrgPATGrantRequests' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_GistsCommentsStarsAndForks|Test(CreateGist|UpdateGist|DeleteGist|StarUnstarGist|ListStarredGists|ForkGist|GistComments|ListGistsForAuthUser|ListPublicGists|GistCommitsAndRevision)' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren|TestSubIssues_|TestIssueDependencies_BlockedBy|TestDeleteRepo' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoLeavesNoResidue|TestPersistenceReload_DeleteDeploymentPurgesStatuses|TestPersistenceReload_DeploymentsStatusesEnvironments|TestDeployments_Lifecycle' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoLeavesNoResidue|TestPersistenceReload_RenameRepoMovesRepoScopedMetadata|TestPersistenceReload_TransferRepoMovesRepoScopedMetadata' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoLeavesNoResidue|TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren|TestProjectV2' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren|TestPersistenceReload_RenameRepoMovesRepoScopedMetadata|TestPersistenceReload_TransferRepoMovesRepoScopedMetadata|TestNotifications_' -count=1
GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1
```

They passed with sandbox escalation for loopback listeners.

Docker compatibility was available again through Podman 6.0.1, container listing worked, and a minimal container run passed:

```bash
docker version
docker ps
docker run --rm alpine:3 true
```

The full Bleephub Go pre-commit test command passed after Docker compatibility returned:

```bash
GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./... -count=1 -timeout 300s
```

The Bleephub UI validation passed after the public Actions metrics and route-level code-splitting changes:

```bash
bun run test src/__tests__/EnterprisePage.test.tsx src/__tests__/api.test.ts src/__tests__/OverviewPage.test.tsx
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

It passed again after the OAuth App token-management entropy fix, after the official-client organization provisioning fix, and after the runtime enterprise-coordinate fix, each time with 117 checks passing and 0 failing.

It also passed after gist state became durable and after repository deletion began purging persisted issue and pull request child state, with 117 checks passing and 0 failing.

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
