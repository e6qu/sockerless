# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current Branch

`feat/bleephub-object-backed-service-bytes` continued the Bleephub GitHub-fidelity work after merged #785. It fixed BUG-2471, BUG-2472, BUG-2473, BUG-2474, BUG-2475, BUG-2476, BUG-2477, BUG-2478, BUG-2479, BUG-2480, BUG-2481, BUG-2482, BUG-2483, and BUG-2484.

#785 made repository deletion clean Codespace runtime/workspace state, hardened the AWS SDK simulator CI shard against hosted-runner disk exhaustion, and made persisted Bleephub require object-backed GitHub Actions artifacts, dependency caches, and runner logs.

This branch extended the same object-backed durable byte contract to release assets and GitHub Packages. Release asset uploads, package version files, and GitHub Container Registry blobs now write through the configured S3-compatible object store when it is present; SQLite stores metadata and object keys. Persisted startup and local development documentation require `BLEEPHUB_OBJECT_S3_BUCKET` for the full durable service-byte set: GitHub Actions artifacts, dependency caches, runner logs, release assets, package files, and container-registry blobs.

The public GitHub Packages file download route now reads package file bytes from object storage when package files were stored there. Object-backed package files therefore work through the same REST download URL that listed package metadata advertises, instead of failing because the downloader looked only for a local filesystem path.

Repository deletion now treats git storage cleanup as a required pre-delete step. Filesystem and S3-backed git storage are purged before repository metadata is deleted, and S3 cleanup failures return an error while preserving the repository record and git storage index instead of logging and orphaning git objects.

The pre-push dependency freshness gate also found stale AWS software development kit service modules in the Amazon Elastic Container Service backend, AWS Lambda backend, and AWS simulator software development kit tests. Those modules were upgraded to the latest published CloudWatch, Amazon Elastic Compute Cloud, and AWS Lambda service module versions, and the freshness gate passed again.

The Bleephub go-github software development kit harness now provisions organizations through GitHub Enterprise Server's public admin organization API instead of the operator-only `/internal/orgs` convenience route. The official-client coverage therefore follows the same public organization provisioning contract as the Docker-backed GitHub command-line interface harness, and source coverage rejects `/internal/orgs` in the go-github harness.

Bleephub's public GitHub REST tests now use the same GitHub Enterprise Server public admin organization API for organization setup. The shared test helper creates prerequisite organizations through `/api/v3/admin/organizations`, and source coverage rejects new direct `/internal/orgs` setup calls outside the explicit operator-management tests.

Bleephub's public code scanning tests now create alert state by uploading SARIF through GitHub's public `/api/v3/repos/{owner}/{repo}/code-scanning/sarifs` route. SARIF rule severity and description metadata now flow into persisted alert state, so severity filters, campaign links, organization alert lists, and Copilot Autofix coverage exercise the public ingestion path instead of an operator-only alert seed route.

The obsolete internal code scanning alert seed route was removed after SARIF upload became the public alert ingestion path. Route coverage no longer includes `/internal/repos/{owner}/{repo}/code-scanning/alerts`, and source guards check both tests and server registration so future code scanning alert setup stays on the public SARIF upload contract.

The Bleephub UI typecheck pre-commit hook now rebuilds `@sockerless/ui-core` declarations after clearing stale incremental build state before running Bleephub `tsc`. A cleaned generated `ui-core/dist` directory no longer leaves the hook dependent on a manual rebuild or stale TypeScript build cache.

Bleephub secret scanning alerts now come from repository content instead of public tests seeding alert rows through an operator-only route. The contents API scans new commits for supported provider patterns, Git Database branch reference creation/update scans commit targets, alert locations contain real commit/blob/path coordinates, and public secret scanning tests use committed secret patterns. The same test run found and fixed the incidental Git Database response-shape drift where `POST /git/blobs` returned an undocumented top-level `node_id`.

Bleephub secret scanning push protection now creates bypass placeholders through protected public write paths instead of through an operator seed route. Public contents writes and Git Database branch reference writes detect protected provider patterns before mutating git state, return a `422` push-protection response with a placeholder, honor active public bypasses for the matched token type, and remove `/internal/repos/{owner}/{repo}/secret-scanning/push-protection-placeholders` from the registered route set.

Bleephub Dependabot alerts now come from public dependency graph snapshots plus published security advisories instead of an operator-only alert seed route. Repository security advisories persist GitHub vulnerability package coordinates, dependency snapshot success on the default branch derives matching Dependabot alerts from the global advisory database, advisory publication derives alerts from already submitted dependency snapshots, and the old `/internal/repos/{owner}/{repo}/dependabot/alerts` seed endpoint was removed.

The AWS simulator software development kit Amazon Elastic Container Service task startup test now polls the real `DescribeTasks` state until a long-running task reaches `RUNNING`. The test no longer assumes Amazon Elastic Container Service startup completed within a fixed sleep, so CI remains sensitive to real task state without failing on legitimate startup timing.

## Continue Here

1. Keep scanning for Bleephub behavior that is still internal-only, shape-only, fake, fallback-based, or not backed by real git/object/store state.
2. Prefer high-value public GitHub surfaces: repository provider behavior, releases/assets, GitHub Actions and runner protocol, Pages, OAuth/GitHub Apps/Auth, packages/container registry, pull requests/reviews/checks/statuses, notifications, repository settings/security/advisories, and the UI paths that consume them.
3. For every found defect, add a `BUGS.md` row first, fix the class of issue where practical, add focused tests, and update continuity in past tense.
4. Use local Docker-backed Bleephub/Codespaces tests again while Docker compatibility remains available; do not restore the temporary non-Docker-only hook path.
5. Keep BUG-2441 visible until the current `knip`/Node `DEP0205 module.register()` warning has an upstream or in-repo fix that does not suppress deprecations.

## Recent Validation

- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestArtifact(CreateUploadFinalize|FinalizeScopesByWorkflowRunBackendID|ListReturnsFinalized|Download)|TestGetSignedArtifactURL' -count=1` passed with sandbox escalation for loopback listeners.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestActionsRunAndJobEndpointsScopeIDsToPathRepository|TestActionsRuns_(Get|Delete|Cancel)|TestActionsRunJobs_List|TestActionsJobs_(Get|Logs)|TestActionsArtifacts_ListRunArtifacts' -count=1` passed with sandbox escalation for loopback listeners.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestNotifications_(ListAndRead|ThreadIDsSeparateIssuesAndPullRequests|ThreadSubscription|RepoScoped|SinceAndBefore|ParticipatingFilter)|TestNotificationThreadMarkDone' -count=1` passed with sandbox escalation for loopback listeners.
- `bun run test src/__tests__/api.test.ts` passed in `ui/packages/bleephub` after the user/organization/team public-route fixes.
- `bun run typecheck` passed in `ui/packages/bleephub` after the user/organization/team public-route fixes.
- `bun run test src/__tests__/OAuthPage.test.tsx` passed in `ui/packages/bleephub` after the OAuth page stopped reading internal state.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -c ./bleephub -o /private/tmp/bleephub.test` passed after the GitHub Enterprise Server user administration route changes.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestAuditLogRecords' -count=1` passed with sandbox escalation after the audit-log public-route/order fix.
- The focused runtime Go test `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestEnterpriseAdminUsersCRUDSiteAdminAndSuspension|TestUserExtras_ListUsers' -count=1` could not run in the sandbox: the unprivileged run failed on loopback bind permission, and both escalated attempts timed out in the automatic approval reviewer before execution.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestOAuth_(LoginPost|Authorize|WebFlow|DeviceFlow|TokenResponse)|TestGHDeviceFlow' -count=1` passed with sandbox escalation after browser login began requiring stored personal access tokens.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -c ./bleephub -o /private/tmp/bleephub.test` passed after the browser authentication change.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestOAuth_(LoginPost|Authorize|WebFlow|DeviceFlow|TokenResponse)|TestGHDeviceFlow' -count=1` passed with sandbox escalation after OAuth web/device flows began requiring registered client IDs and web-flow client secrets.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -c ./bleephub -o /private/tmp/bleephub.test` passed after the OAuth client-validation change.
- `bun run test src/__tests__/OAuthPage.test.tsx` passed in `ui/packages/bleephub` after the OAuth UI began sending explicit registered client identifiers on web, device-code, and device-token requests.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestActionsPendingDeploymentReviewFlow|TestRegisteredAPIv3RoutesExistInGitHubSpec' -count=1` passed with sandbox escalation after the hook-discovered Actions fixture and route-spec gaps were fixed.
- `npx knip` passed in `ui/packages/bleephub` after the dead runner-session TypeScript exports were removed.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./... -count=1 -timeout 300s` passed in `bleephub` with sandbox escalation after Docker compatibility returned and the temporary non-Docker local hook was removed.
- `bash scripts/check-latest-deps.sh` passed after the AWS and Google Cloud Go module upgrades required by pre-push.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestSeedPreRegisteredApp|TestSeedAppIdempotentAndBadKey|TestPersistence_RoundTripAppsInstallationsTokensRepos' -count=1` passed with sandbox escalation after GitHub App seed configuration stopped defaulting or auto-creating identities.
- `bash -n bleephub/test/run-integration.sh` passed after the runner harness renamed simulator Google Cloud service-account credential generation.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestWorkflows_Dispatch' -count=1` passed with sandbox escalation after GitHub Actions workflow dispatch accepted GitHub branch/tag/SHA `ref` inputs.
- `make bleephub-gh-docker-test` passed with the Docker-compatible runtime after the local-image build helper and workflow-dispatch ref-resolution fixes.
- `make -n bleephub-gh-docker-test`, `make -n smoke-test-ecs`, and `make -n bleephub-runner-docker-build` expanded to the shared local-image build helper.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestCryptoRandomReadsAreChecked|TestGitHubApp|TestOAuth|TestPagesDeployments_CreateStatusCancel|TestSecurityAdvisories|TestClassroom' -count=1` passed with sandbox escalation for loopback listeners.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPagesDeployments_CreateStatusCancel|TestPagesHealthCheck|TestPagesBuildsCRUD|TestPersistenceReload_PagesBuildIDSequence' -count=1` passed with sandbox escalation for loopback listeners.
- `bun run test src/__tests__/RunnersPage.test.tsx` passed in `ui/packages/bleephub`.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestReleases_AssetLifecycle|TestGenerateCodespaceNameRequiresRandomBytes|TestCodespacesUserMachines_RealCatalogValues' -count=1` passed with sandbox escalation for loopback listeners.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestBuildJobMessageRepoLessGithubContextHasNoFakeRefSha|TestGithubContextMapRepoLessHasNoFakeRefSha|TestSubmitWorkflowRepoRefResolution|TestSubmitWorkflowRejectsUnresolvedRepoRef|TestConcurrencyGroups_' -count=1` passed with sandbox escalation for loopback listeners.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestBuildJobMessage|TestGithubContextMap|TestSubmitWorkflow|TestConcurrencyGroups|TestCollect|TestWorkflowCall|TestWorkflows_Dispatch|TestWorkflows_AutoRegisterOnSubmit|TestWorkflows_DiscoverFromGitStorage|TestWorkflows_DispatchUsesHostModeWhenWorkflowHasNoContainer|TestApproveWorkflowRun_ReleasesGatedRun|TestRerunWorkflowJob_NewAttemptCarriesOtherJobs|TestPRGraphQL_ViewDefaultFields|TestCommitStatuses_CreateListCombined|TestRepoWebhookTest_' -count=1` passed with sandbox escalation for loopback listeners.
- `git diff --check` passed after the latest fixes.
- `bun run test src/__tests__/OverviewPage.test.tsx src/__tests__/MetricsPage.test.tsx` passed in `ui/packages/bleephub` after the overview and metrics pages stopped using internal diagnostics routes.
- `bun run typecheck` passed in `ui/packages/bleephub` after the public Actions metrics and route code-splitting changes.
- `npx knip` passed in `ui/packages/bleephub`; it still emitted Node's `DEP0205 module.register()` deprecation warning under current `knip` 6.23.0, tracked as BUG-2441.
- `bun run test` passed in `ui/packages/bleephub` with 318 tests after the public Actions metrics and lazy route chunks.
- `bun run build` passed in `ui/packages/bleephub` without Vite large-chunk or circular-chunk warnings after route-level lazy loading and vendor chunking.
- `bun outdated knip` reported no newer `knip` version after the upgrade to 6.23.0.
- `make bleephub-gh-docker-test` passed with the Docker-compatible Podman runtime after the UI/public-route and bundle changes.
- `bun run test:e2e -- e2e/bleephub.spec.ts --grep "Operations console|Global navigation|Metrics page"` passed with the Docker-compatible Podman runtime available and the rebuilt embedded Bleephub UI.
- `bun run test:e2e -- e2e/errorPaths.spec.ts` passed with the rebuilt embedded Bleephub UI, including the public repository-list failure path.
- `GOWORK=off CGO_ENABLED=0 go test -v -count=1 -timeout 180s -run 'TestSQS_ReceiveMessageHonorsLongPollingWaitTime|TestSQS_ReceiveMessageRejectsInvalidWaitTimeSeconds|TestCloudWatch_OKActionsDispatchedToSNS' .` passed in `simulators/aws/sdk-tests` with the Docker-compatible Podman runtime.
- `make sdk-test SDK_TEST_TIMEOUT=600s` passed in `simulators/aws` with the Docker-compatible Podman runtime after the Amazon SQS long-polling fix.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off CGO_ENABLED=0 go test -v -count=1 -run TestAWSEventSourceCoversAllServiceSlices .` passed in `simulators/aws` after the AWS Budgets CloudTrail event-source mapping was added.
- `GOWORK=off CGO_ENABLED=0 go test -v -count=1 -timeout 180s -run 'TestBudgetsCRUDSDK|TestECS_ManagedEBSRunTaskProcessMode|TestCloudWatch_AlarmSNSActionToSQS_ProcessMode|TestSQS_ReceiveMessageHonorsLongPollingWaitTime' .` passed in `simulators/aws/sdk-tests`.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestRepoGraphQL_ReleasesConnection|TestImmutableReleases_OrgSettingsAndRepoEnforcement|TestImmutableReleases_SelectedRepositories' -count=1` passed after GraphQL `Release.immutable` started using persisted immutable-release state.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed after the GraphQL release schema change.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPRGraphQL_ViewDefaultFields|TestPersistenceReload_CheckRunsAndSuites' -count=1` passed after GraphQL status-rollup count fields and check-suite workflow-run links were added.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed after the GraphQL status-rollup and check-suite persistence change.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_WorkflowRunsAndAttempts|TestWorkflowRunsListNewestFirst|TestActionsRuns_(Get|Delete|Cancel)|TestActionsRunJobs_List|TestRerunWorkflowJob_NewAttemptCarriesOtherJobs|TestApproveWorkflowRun_ReleasesGatedRun' -count=1` passed after workflow runs and archived attempts began persisting.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestEntropyHelpersReturnErrors|TestCryptoRandomReadsAreChecked|TestCreateGist|TestGitHubApp|TestOAuth|TestSecurityAdvisories|TestClassroom|TestActionsOIDC' -count=1` passed after public token and identifier entropy failures became returned errors.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed after the entropy failure-handling change.
- `docker version`, `docker ps`, and `docker run --rm alpine:3 true` passed with sandbox escalation against Docker CLI compatibility backed by Podman 6.0.1.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestOAuth(App|Check|Reset|Revoke|Scope)|TestEntropyHelpersReturnErrors|TestCryptoRandomReadsAreChecked' -count=1` passed after OAuth App token management moved to returned entropy errors.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed after the OAuth App token-management entropy fix.
- `make bleephub-gh-docker-test` passed with the Docker-compatible Podman runtime after the OAuth App token-management entropy fix.
- `bash -n bleephub/test/run-gh-test.sh` passed after the Docker-backed `gh` command-line interface harness moved organization creation to the public GitHub Enterprise Server admin organization API.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestGitHubCommandLineInterfaceHarnessUsesAdminOrganizationAPI|TestAdminCreateOrg|TestCreateOrg|TestListAuthUserOrgs' -count=1` passed after the official-client organization provisioning fix.
- `make bleephub-gh-docker-test` passed with 117 passing checks and 0 failures after the official-client organization provisioning fix.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestExistingRoutesUnaffected|TestGHApiRoot' -count=1` passed after `/health` began reporting the configured enterprise slug.
- `bun run test src/__tests__/EnterprisePage.test.tsx src/__tests__/api.test.ts src/__tests__/OverviewPage.test.tsx` passed in `ui/packages/bleephub` after the enterprise UI began using the runtime enterprise slug and the localStorage warning was fixed.
- `bun run typecheck` passed in `ui/packages/bleephub` after the enterprise API helpers became runtime-coordinate driven.
- `make bleephub-gh-docker-test` passed with 117 passing checks and 0 failures after Docker compatibility returned and the runtime enterprise-coordinate fix landed.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestCryptoRandomReadsAreChecked|TestEntropyHelpersReturnErrors|TestOrgPATGrantRequests' -count=1` passed after fine-grained personal access token generation moved onto a full-read entropy helper.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_GistsCommentsStarsAndForks|Test(CreateGist|UpdateGist|DeleteGist|StarUnstarGist|ListStarredGists|ForkGist|GistComments|ListGistsForAuthUser|ListPublicGists|GistCommitsAndRevision)' -count=1` passed after gist/comment/star/fork state began persisting.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed after gist/comment/star/fork state began persisting.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren|TestSubIssues_|TestIssueDependencies_BlockedBy|TestDeleteRepo' -count=1` passed after repository deletion began purging persisted issue and pull request child state.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren|TestSubIssues_|TestIssueDependencies_BlockedBy|TestDeleteRepo' -count=1` passed after repository deletion began purging repository-ID keyed state and selected-repository references.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed after the repository deletion cascade fixes.
- `make bleephub-gh-docker-test` passed with 117 checks and 0 failures against the Docker-compatible Podman runtime after the repository deletion cascade fix.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoLeavesNoResidue|TestPersistenceReload_DeleteDeploymentPurgesStatuses|TestPersistenceReload_DeploymentsStatusesEnvironments|TestDeployments_Lifecycle' -count=1` passed with sandbox escalation after deployment, environment, environment-policy, and Pages deployment records joined the deletion cascade.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoLeavesNoResidue|TestPersistenceReload_RenameRepoMovesRepoScopedMetadata|TestPersistenceReload_TransferRepoMovesRepoScopedMetadata' -count=1` passed with sandbox escalation after team grants and artifact metadata joined repository rename/delete/transfer cascades.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoLeavesNoResidue|TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren' -count=1` passed with sandbox escalation after source import, dependency graph, SBOM export, and enterprise Dependabot access rows joined repository deletion cascades.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoLeavesNoResidue|TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren' -count=1` passed with sandbox escalation after Copilot coding agent tasks, issue field values, and CodeQL variant-analysis target rows joined repository deletion cascades.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed with sandbox escalation after the BUG-2464 repository deletion cascade fix.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren|TestProjectV2' -count=1` passed with sandbox escalation after Projects v2 item rows and indexes joined repository/project deletion cascades.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed with sandbox escalation after the BUG-2465 Projects v2 deletion-index fix.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoPurgesIssueAndPullChildren|TestPersistenceReload_RenameRepoMovesRepoScopedMetadata|TestPersistenceReload_TransferRepoMovesRepoScopedMetadata|TestNotifications_' -count=1` passed with sandbox escalation after notification thread and repo read state joined repository delete/rename cascades.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed with sandbox escalation after the BUG-2466 notification-state cascade fix.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_(DeleteRepo(PurgesIssueAndPullChildren|LeavesNoResidue)|ReactionParentDeletion|Reactions)' -count=1` and `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed with sandbox escalation after labels, milestones, and reaction parent buckets joined the deletion cascade.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistenceReload_DeleteRepoLeavesNoResidue|TestDeleteRepo|TestUnitDeleteRepo|TestCodespaces' -count=1` and `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed with sandbox escalation after repository deletion began cleaning Codespace runtime/workspace state before deleting repository records.
- `cd simulators/aws/sdk-tests && GOWORK=off SOCKERLESS_AWS_SIMULATOR_BINARY=/Users/zardoz/projects/sockerless/simulators/aws/simulator-aws SOCKERLESS_SPEC_VALIDATE=/tmp/aws-spec-violations-direct-full.jsonl SOCKERLESS_SPEC_DIR=/Users/zardoz/projects/sockerless/specs/cloud-api/aws /tmp/aws-sdk-tests.test -test.v -test.count=1 -test.timeout=600s` passed after the AWS SDK simulator CI job moved to the prebuilt test binary path.
- `bash scripts/check-spec-violations.sh aws /tmp/aws-spec-violations-direct-full.jsonl` passed after that direct AWS SDK simulator run.
- `bash -n scripts/bleephub-local-dev.sh` passed after the local development launcher began requiring object-store coordinates for persisted Bleephub.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistentServerStorageRequiresDurableGitAndObjectBytes|TestArtifact(CreateUploadFinalize|FinalizeScopesByWorkflowRunBackendID|ListReturnsFinalized|Download)|TestGetSignedArtifactURL|TestTimelineLogBytesUseObjectStore|TestActionsJobs_Logs' -count=1` passed with sandbox escalation after persisted startup began requiring object-backed Actions bytes.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed with sandbox escalation after the persisted Actions byte-store startup guard.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPersistentServerStorageRequiresDurableGitAndObjectBytes|TestReleases_AssetBytesUseObjectStore|TestPackageAndRegistryBytesUseObjectStore|TestContainerRegistryPublishCreatesPackageVersion|TestReleases_AssetLifecycle|TestDeleteRepo' -count=1` passed with sandbox escalation after release assets and package bytes moved to object storage.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed with sandbox escalation after the release/package object-storage change.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPackageAndRegistryBytesUseObjectStore|TestContainerRegistryPublishCreatesPackageVersion|TestPackages_' -count=1` passed with sandbox escalation after public package file downloads began reading object-backed package files.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed with sandbox escalation after the public package file download fix.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'Test(DeleteRepoS3GitCleanupFailurePreservesRepo|GitDeleteCleanup|UnitDeleteRepo)$' -count=1` passed with sandbox escalation after repository deletion began failing loudly on required S3 git-storage cleanup errors.
- `bash scripts/check-latest-deps.sh` passed after the AWS software development kit module freshness fix required by pre-push.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestGitHub(CommandLineInterface|SoftwareDevelopmentKit)HarnessUsesAdminOrganizationAPI|TestAdminCreateOrg' -count=1` passed with sandbox escalation after the go-github software development kit harness moved organization setup to GitHub Enterprise Server's public admin organization API.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test -run 'Test(Organizations|AppsInstallationTokenFlow|OrgProfileTeamsAndMembershipSurfaces|OrgWebhooksSDK)$' -count=1` passed in `bleephub/sdk-tests` with sandbox escalation after the same change.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test -count=1` passed in `bleephub/sdk-tests`.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestPublicFeatureTestsProvisionOrganizationsThroughAdminAPI|Test(GetOrg|UpdateOrg|DeleteOrg|ListAuthUserOrgs|CreateTeam|ListTeams|GetTeam|DeleteTeam|OrgMembership|RemoveMembership|TeamRepoPermission|ListUserTeams|GraphQLViewerOrgs|GraphQLOrganization|CreateOrgRepo|CreateOrgRepoExtended|ListOrgRepos|RepoOrganizationField|OpenAPIOrg|GetRepoInstallationHTTP|InviteFlow|PublicizeAndConcealMembership|OrgProfileTeamsAndMembershipSurfaces|OrgWebhooks|Codespaces|AppsInstallationTokenFlow|CreateRepositoryInOrganization|Actions.*Org)' -count=1` passed with sandbox escalation after Bleephub public REST tests moved organization setup to GitHub Enterprise Server's public admin organization API.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeScanning(AlertTestsUsePublicSARIFUpload|_ListAndFilter|_GetAndInstances|_PatchDismiss|_InvalidDismissedReason|_SARIFUploadCreatesAlerts|OrgAlerts|Autofix|AutofixEligibility)|LiveCodeScanning_FullFlow|OrgCampaigns)' -count=1` passed with sandbox escalation after public code scanning alert tests moved to SARIF upload.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1` passed with sandbox escalation after the SARIF rule metadata change.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'Test(CodeScanningAlertTestsUsePublicSARIFUpload|RegisteredAPIv3RoutesExistInGitHubSpec|FuzzRoutePatternsMatchRegisteredRoutes|CodeScanning|LiveCodeScanning|OrgCampaigns)' -count=1` passed with sandbox escalation after the obsolete internal code scanning alert seed route was removed.
- `pre-commit run ui-typecheck-bleephub --all-files` passed with sandbox escalation after deleting `ui/packages/core/dist`, `ui/packages/core/tsconfig.build.tsbuildinfo`, and `ui/packages/core/tsconfig.tsbuildinfo`, proving the hook rebuilt required declarations itself.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestSecretScanning|TestLiveSecretScanning_CRUD' -count=1` passed with sandbox escalation after secret scanning alerts moved to committed-content ingestion.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestGitData|TestUpdateRef|TestSecretScanning_GitDatabaseRefCreatesAlert|TestGetBlob|TestCreateBlob|TestListRefs|TestGetRef' -count=1` passed with sandbox escalation after Git Database branch ref updates began feeding secret scanning and blob-create response shape was corrected.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1` passed with sandbox escalation after the secret scanning ingestion and Git Database response-shape fixes.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestSecretScanning|TestGitData|TestUpdateRef|TestCreateRef|TestCreateBlob|TestListRefs|TestGetRef' -count=1` passed with sandbox escalation after secret scanning push protection began minting placeholders from public contents and Git Database writes.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestRegisteredAPIv3RoutesExistInGitHubSpec|TestFuzzRoutePatternsMatchRegisteredRoutes|TestSecretScanning|TestGitData|TestUpdateRef|TestCreateRef|TestCreateBlob|TestListRefs|TestGetRef' -count=1` passed with sandbox escalation after the internal placeholder seed route was removed.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1` passed with sandbox escalation after the public push-protection placeholder change.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -run 'TestDependabot|TestLiveDependabot|TestEnterpriseDependabot|TestDependencyGraph|TestGlobalSecurityAdvisories|TestSecurityAdvisories' -count=1` passed with sandbox escalation after Dependabot alerts moved to public dependency graph and security advisory ingestion.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -tags noui ./bleephub -count=1` passed with sandbox escalation after the Dependabot alert ingestion change.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off CGO_ENABLED=0 go test -v -count=1 -timeout 180s -run TestECS_TaskNoCommandStaysRunning .` passed in `simulators/aws/sdk-tests` after Amazon Elastic Container Service task startup coverage began polling task state.

## Standing Gaps

- BUG-1075: live-cloud validation. No live backend cell was marked green without authenticated real-cloud runs.
- BUG-1345 / GitHub issue #394: `terraform-provider-azuread` still lacks a Microsoft Graph endpoint override, blocking AzureAD Terraform tests against the simulator.
- BUG-2441: the current `knip` unused-export toolchain still emits Node's `DEP0205 module.register()` deprecation warning after upgrading to the current release.
- Issue #363: versioned releases and GitHub Container Registry images remained a standing release/distribution task.

## Work Rules

- Read `STATUS.md` and this file before changing code.
- Keep old history compressed here; use `git log`, PR descriptions, and `WHAT_WE_DID.md` for detail.
- Update `STATUS.md`, `DO_NEXT.md`, `BUGS.md`, `WHAT_WE_DID.md`, and `PLAN.md` together when the branch changes materially.
- Never bypass pre-commit or pre-push. If a hook is wrong, ask the user before changing it.
