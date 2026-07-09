# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current Branch

`feat/bleephub-actions-runtime-fidelity` continued the Bleephub GitHub-fidelity work after #782. It fixed BUG-2391 through BUG-2440.

The branch converted a broad set of Bleephub surfaces from shape-only or internal shortcuts to real GitHub Enterprise Server-compatible behavior: repository metadata/permissions, pull request status rollups, commit status payloads, release asset uploads, GitHub Pages deployments, Codespaces lifecycle failures, OAuth device approval and UI flows, OAuth client validation, browser session authentication, credential entropy, code-quality setup state, Actions repository/ref/SHA context, Actions artifact/run/job scoping, notification identity/URLs, runner inventory routing, user/organization/team/audit-log UI management, and repository webhook/run-control fixtures.

The newest fixes moved user administration, audit-log viewing, OAuth flow controls, and the operations dashboard off operator-only routes, then tied browser login and OAuth client exchange to real stored credential sources. Bleephub now exposes GitHub Enterprise Server user administration routes for list/create/delete/site-admin/suspension flows, persists account suspension state, rejects suspended user tokens with `403`, and the UI helpers call the public GitHub routes. The audit-log page now reads organization audit logs from `/api/v3/orgs/{org}/audit-log` with GitHub's phrase/order query model, and the server honors ascending audit-log order. The OAuth page now starts web/device flows and polls device tokens through GitHub OAuth endpoints instead of reading `/internal/oauth/state`. Browser sessions now require a stored personal access token for the submitted account, suspended accounts cannot sign in through `/login`, OAuth web/device flows require registered OAuth App or GitHub App clients with web-flow client-secret validation, and the UI sends the user-entered registered client identifier on each OAuth flow request. The overview and metrics pages now derive workflow-run, job, and runner counts from public repository Actions REST routes instead of `/internal/metrics` or `/internal/status`, and the storage-coordinate route was removed from the user-facing UI. Hook-discovered stale coverage was also fixed: a repo-scoped Actions fixture now creates a real git ref, GitHub Enterprise Server-only route coverage is allowlisted explicitly, old runner-session UI types were removed, the local Bleephub Go pre-commit hook again ran the full Bleephub suite after Docker compatibility returned, pre-push dependency freshness passed after stale AWS and Google Cloud Go module pins were upgraded, GitHub App seed configuration required explicit real owner and installation accounts instead of defaulting or creating admin-owned identities, the runner harness described generated Google Cloud service-account credentials as simulator endpoint coordinates for the real JWT exchange flow, Docker-backed Make targets loaded local images correctly with either Buildx or legacy Docker builders, the `gh` command-line interface docs named the real required CI job, GitHub Actions workflow dispatch accepted GitHub `ref` inputs such as `main` by resolving branches, tags, full refs, and raw commit SHAs through git storage, and the UI production bundle now lazy-loads route pages with explicit vendor chunks.

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
