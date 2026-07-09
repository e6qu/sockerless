# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`feat/bleephub-faithful-app-runner-flows` — Bleephub object-store test fidelity, Actions byte-object storage and fail-loud log downloads, GraphQL sub-issues, issue-field values, Projects v2 field values/project connections, issue-comment pin state, issue-type assignment, OAuth fail-loud web-flow authorization, GitHub App Manifest creation/browser installation/settings listing, OAuth App settings creation/listing, removal of legacy internal GitHub App/OAuth App management routes, public user-installation listing, GitHub GraphQL pull request review-thread listing and resolve/unresolve, pull request files and closing-issue references from real git/body state, repository watcher GraphQL totals from the same REST-backed watch subscriptions as `/subscribers`, workflow/runner user-facing path cleanup, GitHub Actions runner/control-plane endpoint fidelity, user-owned issue sidebar routing, Pages build metadata fidelity, Actions rerun workflow-file identity, fail-loud event ref resolution, paginated/searchable organization audit logs, go-github software development kit Actions dispatch coverage, SQLite-only metadata persistence, continuous-integration setup hardening, cloud-backend lint sharding, GCP simulator dependency freshness, and AWS S3 CopyObject list visibility.

**Next**
- Keep tightening Bleephub toward real-service behavior by replacing remaining fake test boundaries, shallow GraphQL resolvers, and shape-only endpoints with real store/object-storage/git-backed behavior. After this branch, BUG-1345 and BUG-1075 remained open unless new boyscout bugs were found.

**Scope**
- **BUG-2341:** Bleephub's S3 filesystem tests no longer use a fake S3 server. They build and start `simulator-aws` in `SIM_RUNTIME=process`, create a real S3 bucket through `aws-sdk-go-v2`, and exercise file open/write/read/seek semantics plus repo-prefix delete and rename through real S3 list/copy/delete APIs.
- **BUG-2341 boyscout:** AWS simulator `CopyObject` now stores the copied object's internal `Key` as `bucket/key`, matching `PutObject`, so `ListObjectsV2` sees copied objects. SDK and CLI regressions prove copied objects are both readable and listable.
- **BUG-2342:** Actions artifacts, dependency caches, and runner-uploaded log files can write byte content to S3-compatible object storage via `BLEEPHUB_OBJECT_S3_BUCKET` plus optional endpoint/prefix overrides. Configured object storage fails startup/write/read loudly, delete paths remove object bytes, and real `simulator-aws` tests assert uploaded artifact/cache/log bytes land in S3.
- **BUG-2355:** Public Actions job, run, and run-attempt log downloads now serve only runner-uploaded timeline log files. Object-store mode reads those bytes back from S3-compatible storage, and missing uploaded logs return 404 instead of substituting in-memory live console capture.
- **BUG-2356:** Event-triggered and scheduled workflow runs no longer substitute `HEAD` or an all-zero SHA when the triggering ref is missing. Non-empty refs must resolve exactly, missing event refs are rejected before run creation, scheduled runs reject unresolved default-branch refs, reusable workflow file reads fail when `HEAD` is unresolved, and the workflow git fixtures create real `main` refs so tests exercise valid git state.
- **BUG-2357:** Organization audit-log listing now uses the shared GitHub-style paginator and emits Link headers instead of dumping every persisted event. Its `phrase` filter token-searches the persisted action, actor, org, and detail payload fields, while `actor_id` still narrows by actor. Regression coverage pins pagination, Link headers, exact-action search, and cross-field persisted-data search.
- **BUG-2343:** GraphQL `Issue.parent`, `Issue.subIssues`, and `Issue.subIssuesSummary` now render the same ordered sub-issue relationships as the REST API. Replacing a sub-issue's parent persists both the old and new parent lists so reload keeps the relationship graph coherent.
- **BUG-2344:** Issues now store per-issue organization issue-type assignments. REST issue create/PATCH accepts `issue_type_id` and validates it against the repository owner's enabled org issue types; GraphQL `Issue.issueType` projects the assigned definition; the issue sidebar reads the assignment through GraphQL, maps it to the org issue-type list, and PATCHes the same validated REST field. Reload coverage proves assignments persist.
- **BUG-2345:** The issue sidebar now gates issue-type queries on the repository owner's real type instead of probing `/orgs/{owner}/issue-types` for every issue. User-owned repositories skip the organization-only REST and GraphQL issue-type calls, so issue detail pages do not emit 404 browser console errors under Playwright.
- **BUG-2354:** GraphQL `Issue.issueFieldValues` now exposes organization issue fields and per-issue values from the same real REST-backed store records. The schema includes the GitHub issue-field value union, typed field-definition union, select-option objects, enum values, pagination, and regression coverage that writes text, number, single-select, multi-select, and date values through REST before querying them through GraphQL.
- **BUG-2359:** Projects v2 GraphQL field values now use the full real Projects v2 store contract instead of the old single-select-only projection. `updateProjectV2ItemFieldValue` validates project/item/field ownership, accepts exactly one value of the field's real type, supports text, number, date, single-select, and iteration values, and `Issue.projectItems.fieldValueByName` renders each stored kind through its matching GraphQL union member.
- **BUG-2361:** Project-level Projects v2 GraphQL now exposes the real store-backed project state instead of scalar metadata only. `ProjectV2.fields`, `ProjectV2.views`, and `ProjectV2.items` return Relay connections from the Projects v2 store, including field options, iteration configuration, view filters/visible fields, item edges, and full shared `PageInfo`; Issue and PullRequest `projectItems` use the same pagination shape.
- **BUG-2360:** OAuth web-flow authorization no longer has a Bleephub-only `?auto=1` compatibility shortcut. Authorization codes require a real session, a rendered consent form, and the CSRF-backed consent POST; the `/user/teams` OAuth regressions drive that flow, and the UI flow simulator no longer advertises auto-approval.
- **BUG-2362 / BUG-2364:** GitHub App creation in the Bleephub user interface and `bleephub/sdk-tests` now uses GitHub's App Manifest flow (`POST /settings/apps/new` followed by `POST /api/v3/app-manifests/{code}/conversions`) instead of `/internal/apps`; GitHub App installation setup now uses the signed-in browser app-slug installation flow (`POST /apps/{slug}/installations/new`) in the Go server tests, `go-github` compatibility tests, and gh parity script; and the Apps page reads GitHub Apps through `/settings/apps`, OAuth Apps through `/settings/oauth-apps`, installations through `GET /api/v3/user/installations`, creates OAuth Apps through `POST /settings/oauth-apps/new`, and manages installation suspend/unsuspend/delete through authenticated settings routes. The legacy operator-only GitHub App, installation, and OAuth App management routes were removed from `/internal/apps*`, `/internal/installations`, and `/internal/oauth-apps`; the route-registry guard fails if they return.
- **BUG-2363:** GitHub Actions workflow dispatch now preserves GitHub's host-mode job shape by sending no default job container when workflow YAML has no `container:` block. The Docker runner integration harness creates the `admin/test` repository through the public repository API, writes workflow files through the contents API, dispatches them through repository-scoped workflow dispatch, polls/cancels through public run/job endpoints, and points both resident and dispatcher-spawned runners at the same repository. The Playwright seed path creates repositories/files and dispatches through the same public endpoints. The Workflows, Overview, and Workflow Detail pages read workflow files, runs, run detail, jobs, and logs through repository-scoped GitHub Actions REST endpoints instead of `/internal/exec`, `/internal/workflow_files`, or `/internal/workflows`.
- **BUG-2366:** The Bleephub runner integration harness now preserves provided workflow-dispatch input JSON exactly. Empty dispatch bodies default to `{}` explicitly, and non-empty JSON no longer picks up an extra brace from Bash default-expansion syntax.
- **BUG-2367:** `BLEEPHUB_TEST_FROM=N` now starts the Bleephub runner integration harness at the requested numbered test. Each test block is guarded independently, so local iteration can rerun test 8 onward without silently replaying tests 1-7, while continuous integration still runs everything by default.
- **BUG-2368:** Amazon EC2 `RunInstances` control-plane state now converges to `running` before optional Firecracker host boot, so the public EC2 lifecycle no longer remains `pending` indefinitely while capable host setup is slow or blocked.
- **BUG-2369:** The required pre-push dependency freshness hook no longer fails on stale Go module pins. `backends/ecs`, `backends/lambda`, `bleephub`, `simulators/aws/sdk-tests`, and `simulators/azure/sdk-tests` were refreshed with `make upgrade-deps`, bringing the stale AWS EC2/ECS SDK clients, Bleephub image/crypto modules, Azure OpenID Connect client, and related indirect dependencies to their current published versions.
- **BUG-2370:** Amazon EC2 simulator instance state and EBS attachment metadata remain authoritative control-plane state even when a capable host has no live Firecracker VM. `DescribeInstances` no longer rewrites `running` instances to `stopped` based on local VM liveness, and EBS attach/detach/modify applies real block-device patches only when a live Firecracker VM exists.
- **BUG-2371:** Pull request review-thread listing and resolve/unresolve now use GitHub GraphQL (`PullRequest.reviewThreads`, `resolveReviewThread`, and `unresolveReviewThread`) instead of operator-only internal routes. The Bleephub UI reads thread state and resolves threads through `/api/graphql`, GraphQL exposes review-comment database identifiers for correlation with REST review comments, and the legacy internal review-thread routes were removed.
- **BUG-2372:** Bleephub container package publishing now uses a GitHub Container Registry-compatible OCI/Docker Registry HTTP API v2 data plane under `/v2/`. Blob uploads verify `sha256:` digests, manifests create `container` package versions, manifest/blob reads serve stored bytes with registry headers, and the Packages user interface no longer exposes an internal `/internal/packages` upload form.
- **BUG-2373:** Package-version creation now rejects duplicate live version names under one package, and package file creation validates storage and file payloads before mutating store indexes. Package bytes require configured package storage instead of falling back to relative working-directory paths.
- **BUG-2374:** The required pre-push dependency freshness hook no longer fails on stale `golang.org/x/net` pins. `bleephub`, `simulators/aws`, and `simulators/aws/sdk-tests` were refreshed with `make upgrade-deps`, and `scripts/check-latest-deps.sh` passed.
- **BUG-2375:** CodeQL database downloads and CodeQL variant-analysis query-pack downloads no longer advertise operator-only `/internal/...` URLs from GitHub REST responses. The byte downloads use public non-internal storage coordinates, the obsolete internal download routes were removed, and the OpenAPI response observer now rejects successful `/api/v3` JSON responses that contain `/internal/` URLs.
- **BUG-2376:** Simulator CI module-download setup now uses `scripts/ci-go-mod-download.sh`, a bounded retry wrapper around required `go mod download` calls. The failed `sim (aws cli compute)` setup path and the related simulator SDK/CLI/AWS SDK prebuild paths no longer fail the job on a single transient Go proxy stream reset before tests start.
- **BUG-2377:** AWS simulator software development kit, AWS Command Line Interface, and Terraform harness health polling now allow a bounded 30-second startup window and report the last observed status or connection error, so process-mode simulator startup under continuous integration load fails with actionable diagnostics instead of a five-second opaque timeout. The AWS Command Line Interface harness also installs a current AWS Command Line Interface when the host binary lacks a recent operation, and unsupported-operation command failures now fail loudly instead of skipping tests.
- **BUG-2378:** The required pre-push dependency freshness hook no longer fails on stale Google Cloud Pub/Sub module pins. `simulators/gcp` and `simulators/gcp/sdk-tests` were refreshed with `make upgrade-deps`, bringing Google Cloud Pub/Sub and related indirect support modules to current published versions.
- **BUG-2379:** The Bleephub user-interface login form now verifies tokens through GitHub's `GET /api/v3/user` identity endpoint instead of `/internal/status`. OAuth/user-to-server tokens accepted by GitHub REST can sign in to the user interface, while operator-only `/internal/*` pages continue to fail loudly when a token lacks operator access.
- **BUG-2380:** The required pre-push dependency freshness hook no longer fails on stale AWS software development kit module pins. `backends/ecs`, `backends/lambda`, `bleephub`, `bleeplab`, and `simulators/aws/sdk-tests` were refreshed with `make upgrade-deps`, bringing `github.com/aws/aws-sdk-go-v2/config`, `github.com/aws/aws-sdk-go-v2/credentials`, and related indirect support modules to current published versions.
- **BUG-2381:** The Bleephub Playwright authentication setup now fills the login token input by its accessible token label instead of the old placeholder text, so GitHub-compatible login-copy changes do not break the end-to-end setup project.
- **BUG-2382:** The Bleephub user-interface shared repository inventory now reads paginated `GET /api/v3/user/repos` instead of `/internal/repos`, so Codespaces, Migrations, and the registered-runner registry use the GitHub REST repository boundary.
- **BUG-2383:** Pull request GraphQL commit rendering no longer re-enters the store lock while resolving git storage. The converter snapshots the repository git-storage handle before taking `Store.mu`, derives real pull-request commit objects outside the lock, and then renders those commits while holding the store read lock only for store-backed metadata such as users and check runs.
- **BUG-2384:** Pull request GraphQL file and closing-issue connections no longer return hardcoded empty data. `PullRequest.files` reuses the git-backed merge-base/head diff from `GET /pulls/{number}/files`, and `PullRequest.closingIssuesReferences` resolves GitHub closing-keyword issue references from the real pull request body to store-backed issue nodes.
- **BUG-2385:** Repository GraphQL watcher counts now use real repository watch subscriptions. `Repository.watchers.totalCount` reads the same `Store.RepoSubscriptions` state as `GET /repos/{owner}/{repo}/subscribers` and `GET /repos/{owner}/{repo}/subscription`, and the GraphQL sweep regression creates a subscription through REST before querying the count.
- **Docs boyscout:** `specs/BLEEPHUB_GITHUB_API_PARITY.md` no longer carries the stale Phase 155 `665 / 1,190` operation inventory; it points at the current route-shape ratchet and records the full vendored REST surface as registered.
- **BUG-2358:** GraphQL `Issue.comments.nodes.isPinned` now reads the persisted REST-backed `Comment.Pinned` field. Regression coverage creates an issue comment, pins it through the REST endpoint, and queries it through GraphQL so the resolver cannot drift back to a default-only value.
- **BUG-2346:** Manual Pages builds no longer store a synthetic all-zero commit SHA, share the audit-log ID allocator, or lose their internal build ID on reload. Build requests require an enabled Pages site plus a real default-branch commit, record that actual commit SHA, allocate IDs from a dedicated persisted Pages-build sequence, and rehydrate the internal ID from the persisted build URL.
- **BUG-2347:** Workflow-run rerun, failed-job rerun, and job rerun resolve cached workflow YAML by the run's originating workflow-file ID/path before accepting an older run's unique workflow-name match. New attempts preserve that workflow-file identity through `submitWorkflow`, and ambiguous same-name older runs fail loudly instead of replaying the wrong file.
- **BUG-2348:** CI no longer runs all cloud-backend module lints in one imbalanced 5-minute shard. The lint matrix now splits cloud backends into AWS, Google Cloud, and Azure provider-family shards, giving each family its own timeout budget and parallel runner.
- **BUG-2349:** The go-github SDK Actions workflow dispatch test no longer skips. It seeds a real `.github/workflows/ci.yml` file through the Git Data API, lists the workflow, dispatches it, reads the workflow run by ID, and lists the queued job through the typed client.
- **BUG-2350:** Actions workflow discovery no longer depends only on git storage `HEAD`. It resolves the repository's recorded default branch when `HEAD` is unset, so Git Data seeded repositories expose default-branch workflow files to list/get/dispatch.
- **BUG-2351:** The GCP simulator CI setup no longer streams the Google Cloud CLI tarball directly into `tar`. It downloads the archive to `$RUNNER_TEMP` with `curl --fail --retry-all-errors` and extracts only the completed file, so transient HTTP stream failures retry instead of producing a truncated gzip.
- **BUG-2365:** CI dependency freshness no longer fails on the GCP simulator module. `simulators/gcp` was refreshed with `make upgrade-deps`, bringing `cloud.google.com/go/longrunning` to v1.2.0 and related indirect dependencies to their current published versions; `scripts/check-latest-deps.sh` passes.
- **BUG-2352:** Bleephub metadata persistence is SQLite-only. The unsupported PostgreSQL dialect, driver dependency, CI sidecar, and skip-if-env PostgreSQL test were removed; `BLEEPHUB_DATABASE_URL` now fails loudly with an explicit unsupported-configuration error; and the SQLite persistence round-trip remains unconditional.
- **BUG-2353:** `TestActionsRun_WorkflowFileReferences` no longer has a rare collision skip. It chooses a non-colliding workflow-file path and always asserts the run's `workflow_id`, `workflow_url`, and path refer to the originating workflow file rather than the per-run ID.
- **Docs/continuity:** Bleephub's README documents that S3 filesystem tests use the AWS simulator object-store slice rather than a local fake; it also documents Actions object-byte storage, no-github.com action resolution, rerun workflow-file identity, the go-github reference adaptor, default-branch workflow discovery, SQLite-only metadata persistence, OAuth's real login/consent web flow, Projects v2 field-value and project-level GraphQL support, and the now-real audit-log, marketplace, app-installation-request, webhook-config, runner-refresh, org people-management, org invitation, and legacy numeric-id team surfaces. Continuity files reflect PR #779 as merged and this branch as active.

**Validation**
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestS3' -count=1` passed.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test -tags noui . -count=1` in `simulators/aws` passed.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test -run TestS3_CopyObject -count=1 ./` in `simulators/aws/sdk-tests` passed.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test -run TestS3API_CopyObjectListed -count=1 ./` in `simulators/aws/cli-tests` passed.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test . -run TestECS_ManagedEBSRunTaskProcessMode -count=1` passed in `simulators/aws/sdk-tests`.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test . -run TestDoesNotExist -count=0` passed in `simulators/aws/cli-tests` and `simulators/aws/terraform-tests`.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test ./internal/tfsim -run TestDoesNotExist -count=0` passed in `simulators/aws/terraform-tests`.
- `bash scripts/check-simulator-tests.sh` passed.
- `make upgrade-deps` passed in `simulators/gcp` and `simulators/gcp/sdk-tests`.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test . -run TestDoesNotExist -count=0` passed in `simulators/gcp` and `simulators/gcp/sdk-tests`.
- `bash scripts/check-latest-deps.sh` passed after refreshing the GCP simulator module dependencies.
- `bun run test src/__tests__/LoginPage.test.tsx` and `bun run typecheck` passed in `ui/packages/bleephub` after the login identity-boundary cleanup.
- `bash scripts/check-latest-deps.sh` passed after refreshing stale AWS software development kit modules in `backends/ecs`, `backends/lambda`, `bleephub`, `bleeplab`, and `simulators/aws/sdk-tests`.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./... -run TestDoesNotExist -count=0` passed in `backends/ecs`, `backends/lambda`, `bleephub`, and `bleeplab` after the AWS software development kit dependency refresh.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test ./... -run TestDoesNotExist -count=0` passed in `simulators/aws/sdk-tests` after the AWS software development kit dependency refresh.
- `SERVER_BIN=/Users/zardoz/projects/sockerless/bleephub/bleephub-server bunx playwright test --project setup --timeout=60000` passed in `ui/packages/bleephub` after the Playwright authentication selector cleanup.
- `bun run test src/__tests__/api.test.ts src/__tests__/CodespacesPage.test.tsx src/__tests__/MigrationsPage.test.tsx src/__tests__/RunnersPage.test.tsx` and `bun run typecheck` passed in `ui/packages/bleephub` after the shared repository inventory moved to `GET /api/v3/user/repos`.
- `go test ./bleephub -run TestRegisteredAPIv3RoutesExistInGitHubSpec -count=1` passed after the Bleephub GitHub API parity spec cleanup.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestGraphQL.*PullRequest|TestPRGraphQL|TestGraphQLPullRequestConverterDoesNotReenterStoreForGitStorage' -count=1` and `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed after the pull request GraphQL store-lock cleanup.
- `GOCACHE=/private/tmp/sockerless-go-cache go test -run TestRepoGraphQL_ViewJSONStaticFields -count=1 ./bleephub` and `GOCACHE=/private/tmp/sockerless-go-cache go test -count=1 ./bleephub` passed after the repository watcher GraphQL cleanup.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPullRequestGraphQLFilesAndClosingIssuesUseRealState|TestPullRequestFilesREST|TestPRGraphQL_ViewDefaultFields' -count=1` and `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed after the pull request GraphQL file/closing-reference cleanup.
- `go test ./bleephub -run 'TestGitHubAppBrowserSettingsListAndManageInstallation|TestOAuthAppBrowserSettingsCreateAndList|TestOAuthAppManagement|TestAppManifestFlowEndToEnd' -count=1` passed after the Apps page moved off internal app-management inventory paths.
- `bun run test src/__tests__/AppsPage.test.tsx`, `bun run typecheck`, `bun run test`, and `bun run build` passed after the Apps page moved to settings/public installation endpoints.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestArtifactUploadWritesObjectStore|TestCacheUploadWritesObjectStore|TestLogfilesUpload_WritesObjectStore|TestArtifactCreateUploadFinalize|TestLogfilesUpload_AppendsBlocks|TestLogfilesUpload_CapsAtFourMiBWithMarker|TestDeleteRunLogs' -count=1` passed with sandbox escalation for loopback/simulator listeners.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./... -run 'TestActionsJobs_Logs|TestLogfilesUpload|TestJobLogs|TestRunLogsZip|TestActionsRunLogs_Delete|TestRunAttemptLogs' -count=1` passed in `bleephub` with sandbox escalation for loopback/simulator listeners.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./... -run 'TestTriggerFiltersEndToEnd|TestWorkflowTriggerRejectsUnresolvedRef|TestRerun|TestActionDownloadInfo|TestReusableWorkflow|TestWorkflows_DiscoverFromGitStorage' -count=1` passed in `bleephub` with sandbox escalation for loopback listeners.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestIssueGraphQL_SubIssueFields|TestSubIssues_AddListReprioritizeRemove|TestSubIssues_ReplaceParentPersistsOldParent|TestIssueDependencies_BlockedBy' -count=1` passed with sandbox escalation for loopback/cache access.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestIssueTypeAssignment|TestIssueGraphQL_IssueTypeAssignment|TestIssueGraphQL_SubIssueFields|TestOrgIssueTypes' -count=1` passed with sandbox escalation for loopback/cache access.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./... -run 'TestIssueGraphQL_IssueFieldValues|TestIssueFieldValues_AddSetListClear|TestIssueGraphQL_IssueTypeAssignment|TestIssueGraphQL_SubIssueFields' -count=1` passed in `bleephub` with sandbox escalation for loopback/cache access.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed after the Actions object-store changes.
- `make ui/packages/bleephub/test TEST_ARGS='IssuesPage.test.tsx --runInBand'` and `make ui/packages/bleephub/lint` passed after the issue-type sidebar changes.
- `SERVER_BIN=/private/tmp/bleephub-server npx playwright test --timeout=60000` passed locally after the user-owned issue sidebar routing fix.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./... -run 'TestPagesBuildsCRUD|TestPagesCreateUpdateShape|TestPagesDeployments_CreateStatusCancel|TestPersistenceReload'` passed in `bleephub` with sandbox escalation for loopback listeners.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./...` passed in `bleephub` with sandbox escalation for loopback listeners.
- `pre-commit run --files BUGS.md DO_NEXT.md STATUS.md WHAT_WE_DID.md bleephub/README.md bleephub/gh_misc_endpoints.go bleephub/gh_misc_surfaces_test.go bleephub/persistence_reload_test.go bleephub/store.go` passed with sandbox escalation after the restricted sandbox blocked loopback listeners and Go build-cache access.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./... -run 'TestActionsRuns_Rerun|TestActionsJobs_Rerun|TestWorkflows_Rerun_ViaCachedYAML|TestRerunKeepsRunIDAndBumpsAttempt|TestRerunFailedJobsCarriesSuccesses|TestRerunWorkflowJob_NewAttemptCarriesOtherJobs'` passed in `bleephub` with sandbox escalation for loopback listeners.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./...` passed in `bleephub` with sandbox escalation for loopback listeners after the Actions rerun workflow-file identity fix.
- `GOWORK=off CGO_ENABLED=0 GOCACHE=/private/tmp/sockerless-go-cache go test -run TestActionsWorkflowDispatch -count=1 ./...` passed in `bleephub/sdk-tests` with sandbox escalation for loopback listeners.
- `GOWORK=off CGO_ENABLED=0 GOCACHE=/private/tmp/sockerless-go-cache go test -count=1 ./...` passed in `bleephub/sdk-tests` with sandbox escalation for loopback listeners.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./... -run 'TestActionsWorkflowDispatch|TestActionsRuns_Rerun|TestActionsJobs_Rerun|TestWorkflows_Rerun_ViaCachedYAML|TestRerunKeepsRunIDAndBumpsAttempt|TestRerunFailedJobsCarriesSuccesses|TestRerunWorkflowJob_NewAttemptCarriesOtherJobs' -count=1` passed in `bleephub` with sandbox escalation for loopback listeners.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./... -count=1` passed in `bleephub` with sandbox escalation for loopback listeners.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./... -run 'TestPersistence_RoundTripAppsInstallationsTokensRepos|TestPersistence_DatabaseURLFailsLoud|TestActionsRun_WorkflowFileReferences' -count=1` passed in `bleephub` with sandbox escalation for loopback listeners.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./... -count=1` passed in `bleephub` with sandbox escalation for loopback listeners after the SQLite-only persistence and Actions test-skip cleanup.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./... -count=1` passed in `bleephub` with sandbox escalation for loopback/cache access after the GraphQL issue-field value projection.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./... -count=1` passed in `bleephub` with sandbox escalation for loopback listeners after the Actions log-download cleanup.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./... -count=1` passed in `bleephub` with sandbox escalation for loopback listeners after the fail-loud workflow event-ref resolution cleanup.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./... -run 'TestAuditLogRecords|TestOrgAuditLogRequiresOwner|TestAuditLogFromRepoCreate' -count=1` passed in `bleephub` with sandbox escalation for loopback listeners after the audit-log pagination/search cleanup.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./... -count=1` passed in `bleephub` with sandbox escalation for loopback listeners after the audit-log pagination/search cleanup.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./... -run 'TestIssueGraphQL_IssueCommentPinned|TestPinIssueCommentREST' -count=1` passed in `bleephub` with sandbox escalation for loopback listeners after the issue-comment pin GraphQL cleanup.
- `GOWORK=off GOCACHE=/private/tmp/sockerless-go-cache go test ./... -count=1` passed in `bleephub` with sandbox escalation for loopback listeners after the issue-comment pin GraphQL cleanup.
- `go test ./bleephub -run 'TestProjectsV2GraphQL_FieldValueKinds|TestIssueGraphQL_IssueCommentPinned|TestOrgInvitationsLifecycle'` passed with sandbox escalation for Go build-cache access after the Projects v2 GraphQL field-value cleanup.
- `go test ./bleephub` passed with sandbox escalation for Go build-cache access after the Projects v2 GraphQL field-value cleanup.
- `go test ./bleephub -run 'TestProjectsV2GraphQL_|TestOAuth_|TestListAuthUserTeams_ViaOAuthWebFlow'` passed with sandbox escalation for Go build-cache access after the OAuth web-flow and Projects v2 project-connection cleanup.
- `go test ./bleephub` passed with sandbox escalation for Go build-cache access after the OAuth web-flow and Projects v2 project-connection cleanup.
- `go test ./bleephub -run 'Test(App|CreateInstallation|ListApp|GetAuthenticatedApp|InstallationToken|GetRepoInstallation|DeleteInstallation|Existing|OAuth_|ProjectsV2GraphQL_|ListAuthUserTeams_ViaOAuthWebFlow|JSONWebToken)'` passed with sandbox escalation for Go build-cache access after the GitHub App Manifest flow, OAuth web-flow, and Projects v2 project-connection cleanup.
- `go test ./bleephub -run 'Test(AppManifest|CreateInstallation|ListAppInstallations|CreateInstallationToken|InstallationToken|GetRepoInstallation|DeleteInstallation|InstallationCreatedFiresAppWebhook|OrgInstallationsList|App)'` and `GOWORK=off go test ./... -run 'TestAppsInstallationTokenFlow|TestAppsGetBySlug'` in `bleephub/sdk-tests` passed with sandbox escalation after GitHub App installation moved to the browser app-slug flow.
- `GOWORK=off go test ./...` passed in `bleephub/sdk-tests` after the software development kit compatibility tests moved GitHub App creation to the manifest flow.
- `bun run test src/__tests__/api.test.ts src/__tests__/OAuthPage.test.tsx src/__tests__/LoginPage.test.tsx src/__tests__/RunnersPage.test.tsx src/__tests__/pageErrorPaths.test.tsx`, `bun run typecheck`, and `bun run build` passed in `ui/packages/bleephub`.
- `go test ./bleephub` passed with sandbox escalation for Go build-cache access after the full branch cleanup.
- `bun run typecheck` and `bun run build` passed in `ui/packages/bleephub`; the production build emitted Vite's existing large-chunk warning only.
- `pre-commit run --files .github/workflows/ci.yml BUGS.md DO_NEXT.md STATUS.md WHAT_WE_DID.md` passed after the cloud-backend lint shard split.
- `bash -n bleephub/test/run-integration.sh` passed after the runner harness JSON/default and per-test-gating cleanup.
- `BLEEPHUB_TEST_FROM=8 make bleephub-runner-docker-test` passed after starting at TEST 8 and completing TEST 8 through TEST 14.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test -run 'TestEC2_InstanceLifecycle|TestEC2_RunInstancesHonorsMaxCount' -count=1 ./` passed in `simulators/aws/sdk-tests`.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test -run 'TestEC2_' -count=1 ./` passed in `simulators/aws/sdk-tests`.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test -run 'TestEC2_InstanceLifecycle|TestEC2_RunInstancesHonorsMaxCount|TestEC2_EBSVolumeSnapshotLifecycleSDK|TestEC2_CreateSnapshotsSDK' -count=1 ./` passed in `simulators/aws/sdk-tests`.
- `bash scripts/check-latest-deps.sh` passed after the pre-push dependency freshness refresh.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestPRGraphQL_ResolveReviewThread|TestPRReviewComments_RootAndReply' -count=1` passed with sandbox escalation for loopback/cache access after pull request review threads moved to GraphQL.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed with sandbox escalation for loopback/cache access after pull request review threads moved to GraphQL.
- `bun run test src/__tests__/PullsPage.test.tsx`, `bun run typecheck`, `bun run test`, and `bun run build` passed in `ui/packages/bleephub`; the production build emitted Vite's existing large-chunk warning only.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestContainerRegistry|TestGitHubPackages|TestPackage' -count=1` passed with sandbox escalation for loopback/cache access after the package registry data-plane cleanup.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -run 'TestCodeQLDatabases_RoundTrip|TestCodeQLVariantAnalyses_CreateAndReadBack|TestRegisteredAPIv3RoutesExistInGitHubSpec' -count=1` passed with sandbox escalation for loopback/cache access after the CodeQL download URL cleanup and OpenAPI response-observer guard.
- `GOCACHE=/private/tmp/sockerless-go-cache go test ./bleephub -count=1` passed with sandbox escalation for loopback/cache access after the package registry data-plane cleanup.
- `bash -n scripts/ci-go-mod-download.sh` and `shellcheck -x -s bash scripts/ci-go-mod-download.sh` passed after the CI module-download retry wrapper was added.
- `bun run typecheck`, `bun run test`, and `bun run build` passed in `ui/packages/bleephub`; the production build emitted Vite's existing large-chunk warning only.
- `bash scripts/check-latest-deps.sh` passed after refreshing `bleephub`, `simulators/aws`, and `simulators/aws/sdk-tests`.

---
### Prior branch (merged, PR #778): Open GitHub issue sweep after #774

`open-issues-776-777` — open GitHub issue sweep after #774.

**Scope**
- **DynamoDB concurrent map panic (#777 / BUG-2330):** `GetItem`, `Query`, `Scan`, `BatchGetItem`, and `TransactGetItems` snapshot each stored item under `ddbItemsMu` before projection, expression matching, response rendering, and consumed-capacity calculation. Read APIs no longer expose maps that `UpdateItem` can mutate in place.
- **ECS CPU/memory limit fidelity (#776 / BUG-2332):** focused regressions pin task-level fallback and container-definition override translation into `MemoryBytes` and `NanoCPU`, the fields the shared AWS simulator container launcher applies to Docker/Podman cgroups as `HostConfig.Memory` and `HostConfig.NanoCPUs`.
- **Shared simulator store snapshotting (BUG-2333):** the AWS, GCP, and Azure `MemoryStore` implementations snapshot values at store boundaries (`Put`, `Get`, `List`, `Filter`, `Update`, and AWS `Upsert`) so maps, slices, and pointers cannot escape as mutable aliases to store-owned state. Shared store tests cover both memory and SQLite stores.
- **Boyscout (BUG-2331):** the DynamoDB Local differential oracle treats Docker as a required dependency and fails loud when the binary is absent instead of silently skipping the test.
- **Skip-if-tool-absent guard (BUG-2334):** pre-commit now runs `scripts/check-no-tool-absent-skips.sh`, which rejects newly-added missing-tool/dependency `t.Skip` paths. Required tools must be installed by the harness or fail loud.
- **Hook-driven dependency refresh:** the pre-push dependency freshness hook was honored by running `make upgrade-deps`, which refreshed stale Go module requirements across the affected repo modules. The badge hook's deterministic README refresh commit was included.

**Validation**
- `GOWORK=off go test -tags noui . -run 'TestDDBItemSnapshotIsIndependentUnderConcurrentMutation|TestECSContainerResourceLimits' -count=1` in `simulators/aws` passed.
- `GOWORK=off go test -tags noui . -count=1` in `simulators/aws` passed.
- `GOWORK=off go test -run 'TestDynamoDB_QueryAndScan|TestDynamoDB_ProjectionExpression|TestECS_RunTask|TestECS_TaskDefinitionFidelitySDK' -count=1 ./` in `simulators/aws/sdk-tests` passed.
- `GOWORK=off go test -race -tags noui . -run TestDDBItemSnapshotIsIndependentUnderConcurrentMutation -count=1` in `simulators/aws` passed.
- `bash scripts/check-latest-deps.sh` passed after the dependency refresh.
- After the dependency refresh, `GOWORK=off go test -tags noui . -count=1` in `simulators/aws` and `GOWORK=off go test -run 'TestDynamoDB_QueryAndScan|TestDynamoDB_ProjectionExpression|TestDynamoDB_BatchAndTransact|TestECS_RunTask|TestECS_TaskDefinitionFidelitySDK' -count=1 ./` in `simulators/aws/sdk-tests` passed.
- `bash scripts/check-no-tool-absent-skips.sh` passed.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test -run 'TestStoreSnapshotsReferenceFields|TestStoreUpdateSnapshotsReferenceFields|TestStore' -count=1 ./...` passed in `simulators/aws/shared`, `simulators/gcp/shared`, and `simulators/azure/shared`.
- `GOCACHE=/private/tmp/sockerless-go-cache GOWORK=off go test -tags noui . -count=1` passed in `simulators/aws`, `simulators/gcp`, and `simulators/azure` when run with sandbox escalation so loopback listeners and Docker/Podman access were available.

---
### Prior branch (merged, PR #767): Team creator auto-maintainer + SQS receive diagnostics

The `fix/open-issues-765-766` branch closed GitHub issues #765 and #766 and fully resolved #763.

**Team creator auto-maintainer (#763/#765).** Real GitHub's `POST /orgs/{org}/teams` makes the authenticated creator a team maintainer automatically. bleephub's `handleCreateTeam` now calls `SetTeamMembership` for the authenticated user with `TeamRoleMaintainer` after creating the team, so downstream OAuth web-flow tests that create a team and then call `/user/teams` see the expected membership. Added `TestListAuthUserTeams_ViaOAuthWebFlow_ReadOrgScope` which creates a team via the API and asserts a `read:org` OAuth token lists it.

**SQS receive diagnostics (#766).** Added Debug-level logging in `sqsEnqueueBody` (queue name and message count after enqueue) and in `handleSQSReceiveMessage` (queue name, total messages, visible messages, picked count, redrived count, visibility timeout). This helps diagnose cases where SNS reports a successful delivery but downstream `ReceiveMessage` calls return an empty `Messages` array.

**Repeated-poll CLI regression test (#766).** `simulators/aws/cli-tests/cloudwatch_alarm_sns_sqs_process_test.go` gained `TestCloudWatchCLI_AlarmSNSActionToSQS_ProcessMode_PollLoop`, which mirrors the downstream probe by polling `receive-message` once per second for up to 15 seconds after the alarm reaches `ALARM` (with no up-front sleep).

**Files changed.** `bleephub/gh_teams_rest.go`, `bleephub/gh_teams_rest_test.go`, `simulators/aws/sqs.go`, `simulators/aws/cli-tests/cloudwatch_alarm_sns_sqs_process_test.go`.

**Validation.**
- `go test -run 'TestListAuthUserTeams|TestCreateTeam|TestTeamMembersList' -count=1 ./bleephub` passes.
- `go test -count=1 ./bleephub` passes.
- `GOWORK=off go test -run 'TestCloudWatchCLI_AlarmSNSActionToSQS_ProcessMode' -count=1 ./` in `simulators/aws/cli-tests` passes.
- `GOWORK=off go test -run 'TestCloudWatch_AlarmSNSActionToSQS' -count=1 ./` in `simulators/aws/sdk-tests` passes.
- `pre-commit run --files bleephub/gh_teams_rest.go bleephub/gh_teams_rest_test.go simulators/aws/sqs.go simulators/aws/cli-tests/cloudwatch_alarm_sns_sqs_process_test.go` passes.

**Next:** PR #767 merged; resume PLAN.md / open issues / BUGS.md work.

---
### Prior branch (merged, PR #759): CloudWatch alarm evaluator dangling-alarm regression test closes #758

The `fix/cloudwatch-alarm-evaluator-758` branch closed GitHub issue #758.

**Alarms whose SNS topics have been deleted no longer hang the background evaluator.** A regression test seeds the simulator with ten dangling alarms pointing at deleted topics, pumps a breaching datapoint, and asserts the evaluator goroutine remains alive and a subsequently-created alarm still dispatches its `AlarmActions` to SQS. The test runs in `SIM_RUNTIME=process` so it exercises the subprocess path independently of the shared TestMain simulator.

**Validation.**
- `GOWORK=off go test -run TestCloudWatch_AlarmSNSActionToSQS_AfterDanglingAlarms -count=1 ./` in `simulators/aws/sdk-tests` passes.

**Next:** PR #761 closed the follow-up issue #760.

---
### Prior branch (merged, PR #756): Open issue fixes — bleephub /user/teams OAuth scope regression and CloudWatch alarm evaluator resilience

The `fix/open-issues-753-754-after-755` branch closed the two open issues that surfaced after merging PR #755.

**bleephub `GET /api/v3/user/teams` no longer requires `read:org`.** Parity tranche #750 had wrapped the route in `requirePerm(scopeMembers, permRead)`, which rejected OAuth tokens that lacked the `read:org` scope. Real GitHub does not require `read:org` for this endpoint because it returns the authenticated user's own team memberships. Fixed by removing the permission gate from `GET /api/v3/user/teams` while keeping the gate on org-scoped team routes. Added `TestListAuthUserTeams_RequiresAuthNotReadOrgScope` to prove a classic OAuth token with only `repo` can list the user's teams, while unauthenticated requests still get 401.

**CloudWatch alarm evaluator is resilient to single-alarm failures.** After #751 reset `cwAlarmLastState` on `PutMetricAlarm`, integrated terraform-sim probes still saw no `SNS.Publish` even though `DescribeAlarms` reported `ALARM`. The evaluator tracked last-dispatched state in a standalone `sync.Map` (`cwAlarmLastState`) that had to be manually reset on every `PutMetricAlarm` path; a panic while evaluating or dispatching one alarm could kill the background evaluator goroutine and silently break AlarmActions for every other alarm. Fixed by moving the last-dispatched state onto each alarm's own `StateValue` field (so `PutMetricAlarm` replacement naturally resets it to `INSUFFICIENT_DATA`) and adding per-alarm panic recovery in `cwEvaluateAlarmsOnce`. Removed the now-redundant `cwAlarmLastState` map and its manual deletions from all three `PutMetricAlarm` handlers and `cwDeleteAlarms`. Added `TestCloudWatch_AlarmSNSActionToSQS_ResilientToOneBadAlarm` in `SIM_RUNTIME=process` to verify that an alarm with an unresolvable action target does not prevent a sibling alarm from delivering its notification.

**Boyscout fix.** `tests/main_test.go` built the `linux/arm64` `sockerless-eval-arithmetic:test` image with `docker build`, but the BuildKit `docker-container` driver leaves the result in the build cache unless `--load` is passed. CI `test (core)` and local runs then failed with `No such image` because the test backend could not resolve the locally-built tag. Fixed by adding `--load` to the `docker build` invocation so the image is loaded into the local store.

**Next:** resume PLAN.md / open issues / BUGS.md work.

---
### Prior branch (merged, PR #751): CloudWatch metric alarm state reset on PutMetricAlarm

The `fix/aws-cloudwatch-alarm-recreate-state-749` branch closed GitHub issue #749: a CloudWatch alarm that had previously reached `ALARM` would not dispatch `AlarmActions` again when it was re-created via `PutMetricAlarm`.

**Fix.** All three CloudWatch alarm wire protocols (awsJson1.0, rpc-v2-cbor, and awsQuery) now call `cwAlarmLastState.Delete(name)` immediately after storing the new alarm configuration, so a re-created alarm is treated as fresh (`INSUFFICIENT_DATA`) and dispatches on the next `ALARM` transition.

**Regression test.** `simulators/aws/sdk-tests/cloudwatch_alarm_sns_sqs_process_test.go` gained `TestCloudWatch_AlarmSNSActionToSQS_RecreatedAlarmResetsState` under `SIM_RUNTIME=process`.

**Next:** PR #752 is the active branch.

---

Closed a large set of remaining GitHub API/UI parity gaps. Coverage moved from 543/1190 to 665/1190 vendored GitHub REST operations (56%). Implemented and tested: Teams, issue management, PR reviews, Git data writes, release assets/reactions, repository settings, org rulesets, Dependabot org/repo, secret scanning org/repo, repository security advisories, Actions permissions/runner labels, gist extras, users extras, notifications. UI: TeamsPage, RepoSettingsPage, SecurityAdvisoriesPage, RulesetsPage, NotificationsPage, GistsPage.

**Next:** PR #751 is the active branch.

---
### Prior branch (merged, PR #747): bleephub API/UI parity continuation

Earlier increments on the merged branch landed Projects classic, secret scanning, code scanning, Dependabot, Migrations, Codespaces, Packages, Discussions GraphQL, organization rulesets, PR reviews, team members, release assets/reactions, and repository settings.

Scope:
- Projects classic (v1) REST API + UI (`bleephub/gh_projects_classic.go`, `ProjectsClassicPage.tsx`).
- Secret scanning REST API + UI (`bleephub/gh_secret_scanning.go`, `SecretScanningPage.tsx`).
- Code scanning REST API + UI (`bleephub/gh_code_scanning.go`, `CodeScanningPage.tsx`).
- Dependabot alerts and secrets REST API + UI (`bleephub/gh_dependabot.go`, `DependabotPage.tsx`).
- Migrations REST API + UI (`bleephub/gh_migrations.go`, `MigrationsPage.tsx`).
- Codespaces REST API + UI with real Docker-backed containers (`bleephub/gh_codespaces.go`, `CodespacesPage.tsx`).
- Packages REST management API + UI with real file storage (`bleephub/gh_packages.go`, `PackagesPage.tsx`).
- Discussions GraphQL API + UI (`bleephub/gh_discussions_graphql.go`, `DiscussionsPage.tsx`).
- **Organization Rulesets REST endpoints (`bleephub/gh_rulesets.go`, `bleephub/store_rulesets.go`).** Done: added org-scoped ruleset CRUD, rule-suites list/get, and versioned history under `/api/v3/orgs/{org}/rulesets`, gated by `requireOrgAdmin(scopeOrgAdministration, permRead/permWrite)`; wired through 2-/3-segment dispatchers to avoid Go 1.22 mux conflicts between `/rulesets/rule-suites/{id}` and `/rulesets/{id}/history`.
- **Pull Request review REST endpoints (`bleephub/gh_pulls_rest.go`, `bleephub/store_pulls.go`).** Done: list/get/create/update/delete/submit/dismiss reviews, request/remove requested reviewers, and update-branch; wired through a 3-segment pull dispatcher to avoid Go 1.22 mux conflicts with PR-comment reactions.
- **Team member/repo REST endpoints (`bleephub/gh_teams_rest.go`, `bleephub/store_orgs.go`).** Done: moved team members, memberships, and repo grants into `gh_teams_rest.go`, wrapped in `requirePerm(scopeMembers, permRead/permWrite)`; reads require active org membership, writes require org admin or team maintainer (only admins can promote to maintainer); team membership response now includes `user`, `team`, and `organization_url`; per-repo permission overrides honored when adding a repo.
- **Repository settings REST endpoints (`bleephub/gh_repos_rest.go`, `bleephub/store_repos.go`).** Done: deploy keys, transfer, branch rename, subscription, vulnerability alerts, automated security fixes, private vulnerability reporting, interaction limits, merge-upstream; persistence and tests wired.
- **Issue management REST endpoints (`bleephub/gh_issues_rest.go`, `bleephub/store_issues.go`).** Done: issue comments (list/get/create/update/delete, repo-level list, pin/unpin), label assignment (add/set/clear), assignee add/remove, issue events (per-issue, repo-level, single event), and timeline; wired through shared issue dispatchers to avoid Go 1.22 mux conflicts; response shapes validated against the vendored OpenAPI description.
- AGENTS.md continuity-only PR rule strengthened.
- Boyscout: bumped Go module deps to latest to satisfy dependency freshness gate.

Validation:
- `go test ./bleephub -count=1` passes; OpenAPI shape ratchet reports no new violations.
- `make bleephub/lint` passes.
- `make ui/packages/bleephub/lint` and `make ui/packages/bleephub/test` pass (104/104).
- `bash scripts/check-latest-deps.sh` reports 0 drifts.

**Next:** PR is open and awaits review/merge. After merge, resume sim/cloud coverage work from PLAN.md / open issues / BUGS.md.

---
### Prior branch (pushed, superseded by PR #744): bleephub API/UI parity continuation

Earlier increments on the same branch landed Projects classic, secret scanning, code scanning, Dependabot, Migrations, Codespaces, Packages, and Discussions. They are now part of PR #744.

---
### Prior branch (merged #743): bleephub branch protection rules API + UI

`feat/bleephub-api-ui-parity-continuation` closed the branch protection surface on bleephub.

Scope:
- Replaced the opaque `BranchProtection map[string]interface{}` blob in `bleephub/gh_misc_endpoints.go` and `bleephub/store.go` with a strongly-typed `BranchProtection` model in new `bleephub/gh_branch_protection.go`.
- Implemented branch protection sub-resource endpoints:
  - `GET/POST/PUT/DELETE /repos/{owner}/{repo}/branches/{branch}/protection/required_status_checks`
  - `GET/POST/PUT/DELETE /repos/{owner}/{repo}/branches/{branch}/protection/required_status_checks/contexts`
  - `GET/PATCH/DELETE /repos/{owner}/{repo}/branches/{branch}/protection/required_pull_request_reviews`
  - `GET/PUT/DELETE /repos/{owner}/{repo}/branches/{branch}/protection/restrictions`
  - `GET/POST/PUT/DELETE /repos/{owner}/{repo}/branches/{branch}/protection/restrictions/users`
  - `GET/POST/PUT/DELETE /repos/{owner}/{repo}/branches/{branch}/protection/restrictions/teams`
  - `GET/POST/DELETE /repos/{owner}/{repo}/branches/{branch}/protection/enforce_admins`
  - `GET/PUT/DELETE /repos/{owner}/{repo}/branches/{branch}/protection/allow_force_pushes`
  - `GET/PUT/DELETE /repos/{owner}/{repo}/branches/{branch}/protection/allow_deletions`
- Updated `GET/PUT/DELETE /repos/{owner}/{repo}/branches/{branch}/protection` to use the typed model and return the canonical GitHub REST shape.
- Enforced required approving review count and requested-changes blocks at PR merge time via `canMergePullRequest` in `bleephub/gh_branch_protection.go`.
- Updated `bleephub/gh_pulls_graphql.go` `baseRef.branchProtectionRule` resolver to return real strict-status-check and required-review-count values.
- Added `BranchProtectionPage.tsx` wired from `RepoSettingsPage.tsx` with route `/ui/repos/:owner/:repo/settings/branch-protection`.
- Added backend HTTP tests in `bleephub/gh_branch_protection_test.go` and updated existing tests to the typed model.
- Added real-but-missing branch-protection paths to `allowedGHESOnly` in `bleephub/gh_api_definition_test.go`.

Validation:
- `go test ./bleephub -count=1` passes; OpenAPI shape ratchet reports no new violations.
- `make bleephub/lint` passes.
- `make ui/packages/bleephub/lint` and `make ui/packages/bleephub/test` pass.

**Next:** PR #743 merged; continue with remaining bleephub API/UI gaps (codespaces, packages, migrations, code scanning, secret scanning, dependabot, projects classic, remaining GraphQL surfaces) or pick from PLAN.md / open issues / BUGS.md.

---
### Prior branch (merged #742): CloudWatch alarm SNS action not delivered to SQS in process mode (#741)

`fix/aws-cloudwatch-sns-sqs-process-mode-741` closed GitHub issue #741 by making the CloudWatch metric and alarm-history stores atomic and adding a subprocess-based `SIM_RUNTIME=process` regression test for the full CloudWatch→SNS→SQS delivery chain.

Scope:
- Replaced racy `Update`-then-`Put` storage in `simulators/aws/cloudwatch_metrics_query.go` (`handleCWQueryPutMetricData`) and `simulators/aws/cloudwatch_alarm_ops.go` (`cwRecordAlarmHistory`) with atomic `Upsert`.
- Added `simulators/aws/sdk-tests/cloudwatch_alarm_sns_sqs_process_test.go`: a subprocess-based `SIM_RUNTIME=process` regression test that starts a fresh simulator process and asserts the CloudWatch→SNS→SQS notification is delivered with valid JSON.

Validation:
- `GOWORK=off go test ./` in `simulators/aws` passes.
- `GOWORK=off go test ./` in `simulators/aws/sdk-tests` passes.
- `./scripts/lint-changed.sh <changed files>` reports 0 issues.
- `./scripts/check-simulator-tests.sh` passes.

**Next:** PR #742 merged; continue with bleephub API/UI parity continuation.

---
### Prior branch (merged #740): bleephub GitHub API/UI parity + internal admin APIs

`feat/bleephub-github-parity-and-admin` closed several commonly-used GitHub API gaps and added the internal admin surface needed to operate a bleephub instance.

Scope:
- Internal admin users/orgs/teams CRUD and audit-log endpoints (`bleephub/handle_mgmt.go`).
- Gists REST API with files, stars, forks, and comments (`bleephub/gh_gists_rest.go`).
- Repository autolinks REST API (`bleephub/gh_repos_autolinks.go`).
- Repository invitations REST API with accept/decline (`bleephub/gh_invitations_rest.go`).
- Commit statuses REST API with combined status (`bleephub/gh_statuses_rest.go`).
- Commit comments REST API (`bleephub/gh_commit_comments_rest.go`).
- UI admin pages (users, orgs, teams, audit log, storage health) and Gists page.
- Store support, persistence wiring, and route registration for all new surfaces.

Validation:
- `go test ./bleephub -count=1` passes.
- `make bleephub/lint` passes.
- `make ui/packages/bleephub/lint` and `make ui/packages/bleephub/test` pass.
- OpenAPI shape ratchet reports no new violations.

**Next:** PR #740 merged; continue with the CloudWatch alarm SNS→SQS process-mode fix (#741).

---
### Prior branch (merged #739): CloudWatch→SNS→SQS malformed JSON body regression tests (#734)

`fix/aws-cloudwatch-sns-sqs-body-734` closed GitHub issue #734 with regression coverage.

Scope:
- Hardened `TestCloudWatch_AlarmActionsDispatchedToSNS` with an adversarial `AlarmDescription` (quotes, newline, control character) and required valid JSON at both the SQS `Body` and embedded SNS `Message` layers.
- Added `TestSNSNotificationEnvelopeQuotesAndBackslashes` for unit-level round-trip coverage of quotes, newlines, and backslashes.
- Updated continuity files for the merged #738 and active #734 branch.

Validation:
- `go test ./simulators/aws -count=1` passes.
- `make -C simulators/aws unit-test` passes.
- AWS SDK regression test `TestCloudWatch_AlarmActionsDispatchedToSNS` passes and fails loudly if either the SQS `Body` or embedded SNS `Message` is invalid JSON.

**Next:** PR #739 merged; pick the next task from PLAN.md / open issues / BUGS.md.

---
### Prior branch (merged #738): bleephub Search, Notifications, and Repository Rulesets APIs

Scope:
- **Search API** (`bleephub/gh_search.go`):
  - `GET /api/v3/notifications` and `GET /api/v3/repos/{owner}/{repo}/notifications` with `all`, `participating`, `since`, `before`.
  - `PUT /api/v3/notifications` and `PUT /api/v3/repos/{owner}/{repo}/notifications` to mark notifications read.
  - `GET /api/v3/notifications/threads/{thread_id}` and `PATCH` (read/done).
  - `GET/PUT/DELETE /api/v3/notifications/threads/{thread_id}/subscription`.
- **Repository Rulesets API** (`bleephub/gh_rulesets.go`, `bleephub/store_rulesets.go`):
  - `GET /api/v3/repos/{owner}/{repo}/rulesets` and `POST`.
  - `GET/PUT/DELETE /api/v3/repos/{owner}/{repo}/rulesets/{ruleset_id}`.
  - `GET /api/v3/repos/{owner}/{repo}/rules/branches/{branch}` evaluating active rulesets against a branch.
  - `GET /api/v3/repos/{owner}/{repo}/rulesets/{ruleset_id}/history` and `GET /api/v3/repos/{owner}/{repo}/rulesets/{ruleset_id}/history/{version_id}`.
- Persistence: notification read/subscription state and rulesets (including history) are stored in new `notifications_state` and `repo_rulesets` buckets.
- Tests: backend HTTP tests for all three surfaces plus live-server shape tests observed by the OpenAPI response-shape validator.

Validation:
- `go test ./bleephub -count=1` passes.
- `make bleephub/lint` clean.
- OpenAPI shape ratchet clean for the new endpoints.

**Next:** PR is open and awaits review/merge. After merge, pick the next task from PLAN.md / open issues / BUGS.md.

---
### Prior branch (merged #737): bleephub repo API finale + AWS sim open-issue fixes

`feat/bleephub-finale-and-open-issues` closed the remaining bleephub repository API gaps and fixed three open AWS simulator fidelity issues.

Scope:
- `GET /api/v3/repos/{owner}/{repo}/languages` with git-tree byte counts, extension-to-language mapping, vendored-path exclusion, and GraphQL `repository.languages` parity.
- `GET /api/v3/repos/{owner}/{repo}/compare/{base}...{head}` with merge-base resolution, ahead/behind/status, commits, and diff file entries.
- `POST /api/v3/repos/{owner}/{repo}/merges` with fast-forward and three-way merge support.
- `POST /api/v3/repos/{owner}/{repo}/forks` and `GET /api/v3/repos/{owner}/{repo}/forks` with git-storage copy and parent/source linkage.
- `PATCH /api/v3/repos/{owner}/{repo}` repository rename with full cascade across stores, collaborators, secrets/variables/hooks, workflow runs, and git-storage prefixes.
- Stargazer endpoints: `GET /repos/{owner}/{repo}/stargazers`, `PUT/DELETE /user/starred/{owner}/{repo}`, `GET /user/starred`, `GET /users/{username}/starred`.
- Collaborator endpoints: `GET /repos/{owner}/{repo}/collaborators`, `GET /repos/{owner}/{repo}/collaborators/{username}/permission`, `PUT/DELETE /repos/{owner}/{repo}/collaborators/{username}`.
- File-editor UI: topics editor in `RepoSettingsPage.tsx`, file delete action in `RepoDetailPage.tsx`.
- AWS sim Route 53 wildcard DNS (#731, BUG-2267).
- AWS sim KMS real encryption and key-policy enforcement (#732, BUG-2268).
- AWS sim CloudWatch→SNS→SQS malformed JSON notification fix (#734, BUG-2269).

Validation:
- `go test ./bleephub -count=1` passes.
- `make bleephub/lint` clean.
- AWS sim `make unit-test` passes.
- UI `bun --bun run typecheck` and `bun --bun run test` pass.
- OpenAPI shape ratchet clean for the new endpoints.

**Next:** PR #738 merged; continue with the CloudWatch→SNS→SQS body fix (#734).

---
### Prior branch (merged #738): bleephub Search, Notifications, and Repository Rulesets APIs

`feat/bleephub-search-notifications-rulesets` closed three public GitHub API surfaces on bleephub.

Scope:
- **Search API** (`bleephub/gh_search.go`): `GET /api/v3/search/issues`, `GET /api/v3/search/repositories`, `GET /api/v3/search/code`, `GET /api/v3/search/users` with qualifier parsing and git-tree code search.
- **Notifications API** (`bleephub/gh_notifications.go`, `bleephub/store_notifications.go`): `GET /api/v3/notifications`, repo-scoped list, mark-read, thread fetch/patch, and thread-subscription CRUD with persistent per-user state.
- **Repository Rulesets API** (`bleephub/gh_rulesets.go`, `bleephub/store_rulesets.go`): ruleset CRUD, branch-rule evaluation, and versioned history.
- Persistence added `notifications_state` and `repo_rulesets` buckets; routes wired in `bleephub/server.go`; `issueToJSON` updated for `active_lock_reason`.
- Backend and live-server OpenAPI shape tests added.

Validation:
- `go test ./bleephub -count=1` passes.
- `make bleephub/lint` clean.
- OpenAPI shape ratchet clean for the new endpoints.

**Next:** PR #738 merged; continue with the next task from PLAN.md / open issues / BUGS.md.

---
### Prior branch (merged #737): bleephub repo API finale + AWS sim open-issue fixes

`feat/bleephub-finale-and-open-issues` closed the remaining bleephub repository API gaps and fixed three open AWS simulator fidelity issues.

Scope:
- `GET /api/v3/repos/{owner}/{repo}/tags` returning lightweight tag objects (name, commit SHA, zipball/tarball URLs).
- `GET /api/v3/repos/{owner}/{repo}/git/refs` listing all refs.
- `GET /api/v3/repos/{owner}/{repo}/git/refs/{namespace}` for `heads`, `tags`, and sub-paths.
- Single-ref lookup vs namespace listing resolution matching GitHub's behavior.
- Backend HTTP tests for tags and refs.

Validation:
- bleephub Go tests pass.
- `make bleephub/lint` clean.
- OpenAPI shape ratchet clean.

**Next:** PR #736 merged; continue with the repo API finale branch.

---
### Prior branch (merged #735): bleephub Phase 2 repo topics and file content deletion

`feat/bleephub-repo-phase2-topics-and-delete-contents` added repository topics and file deletion to the bleephub repo API surface.

Scope:
- `GET /api/v3/repos/{owner}/{repo}/topics` returning the `{names: [...]}` envelope.
- `PUT /api/v3/repos/{owner}/{repo}/topics` with validation (max 20 topics, max 50 chars, no empty or invalid characters).
- `DELETE /api/v3/repos/{owner}/{repo}/contents/{path...}` requiring `message` and `sha`, with optional `branch`, and verifying SHA before deleting.
- Added `deleteFileCommit` helper using go-git worktree `Remove`.
- Added backend HTTP tests in `gh_repos_phase2_test.go`.

Validation:
- bleephub Go tests pass.
- `make bleephub/lint` clean.
- OpenAPI shape ratchet clean.

**Next:** PR #736 is open and awaits review/merge. After merge, continue Phase 4 or pick the next chunk from `PLAN.md`.

---
### Prior branch (merged #733): bleephub Phase 1 org repos, list filters, settings, and org-aware UI

`feat/bleephub-repo-phase1-org-and-settings` closed Phase 1 of the bleephub repo API/UI gap audit.

Scope:
- `GET /api/v3/orgs/{org}/repos` with `type`/`sort`/`direction`/`per_page`/`page`.
- `GET /api/v3/user/repos` extended with `visibility`/`affiliation`/`type`/`sort`/`direction` and 422 conflict validation.
- `GET /api/v3/users/{username}/repos` extended with `type`/`sort`/`direction`.
- `PATCH /api/v3/repos/{owner}/{repo}` extended with description, homepage, default branch, visibility, feature toggles, archived/is_template, web_commit_signoff_required, and merge-method fields; rename rejected with 422.
- Repo response shape gained `homepage`, `license`, `size`, `has_pull_requests`, merge settings, and `organization` for org-owned repos.
- `RepoListOptions.NoPaginate` added so REST list handlers delegate Link-header slicing to `paginateAndLink`.
- Phase 1 UI: `RepoListPage`, `OrgReposPage`, `RepoSettingsPage` General tab, org-targeted repo creation.
- Backend HTTP tests in `gh_repos_phase1_test.go`; UI Vitest 88/88; Playwright e2e 23/23.

**Next:** continue Phase 2 of the repo gap audit.

---
### Prior branch (merged #729): bleephub GitHub-like repository UI

`feat/bleephub-github-like-ui` made the bleephub frontend feel like GitHub's repository surface while adding the real backend endpoints the UI requires.

Scope:
- Global navigation: removed the top-level **Workflows** link (workflows remain reachable per-repository under the Actions tab and via the legacy `/ui/workflows` route).
- Repository creation: new `RepoCreateDialog` with name, description, visibility (public/private/internal), README initialization, .gitignore template, and license template.
- Backend repo-creation extensions: `POST /api/v3/user/repos` and `POST /api/v3/orgs/{org}/repos` now honor `visibility`, `default_branch`, `gitignore_template`, `license_template`, and `auto_init`; `auto_init` writes a real README commit via go-git.
- New backend surfaces: `PUT /api/v3/repos/{owner}/{repo}/contents/{path...}` for file creation/updates; `GET /api/v3/gitignore/templates`, `GET /api/v3/gitignore/templates/{name}`, `GET /api/v3/licenses`, `GET /api/v3/licenses/{license}` with curated real templates.
- Org-owned repos now serialize the organization as the repository owner using the REST `simple-user` shape with `type: "Organization"`.
- Code tab: branch selector, file tree, directory navigation, rendered README via `react-markdown`/`remark-gfm`, and empty-repo clone instructions with HTTPS/SSH/GitHub CLI tabs.

Validation:
- bleephub Go tests pass (`go test ./bleephub -count=1`).
- UI typecheck, Vitest (84/84), and Playwright e2e (23/23) pass.
- `make bleephub/lint` and `make ui/packages/bleephub/lint` clean.

**Next:** pick next task from PLAN.md / open issues / BUGS.md.

---
### Prior branch (merged #728): bleephub local-dev script + AWS sim EC2 revoke-by-rule-id + AWS CLI version drift (BUG-2265/2266)

`feat/bleephub-local-dev-script` delivered three things:

- **`scripts/bleephub-local-dev.sh`** — one-command starter for bleephub API + UI + storage. Supports `start`, `stop`, `restart`, `status`, `logs`, `clean`, plus `--dev` (Vite HMR on :5173), `--tls` (HTTPS :8443 self-signed), `--no-build`, and `--yes`. Default coordinates: API/UI on `http://localhost:5555` (UI at `/ui/`), admin token `bleephub-admin-token-00000000000000000000`, data dir `.local/bleephub/data`, git dir `.local/bleephub/git`.

- **BUG-2265 — EC2 `RevokeSecurityGroupIngress`/`Egress` by `SecurityGroupRuleIds` returned `InvalidPermission.NotFound` for existing rules.** Fixed in `simulators/aws/ec2.go` by parsing `SecurityGroupRuleId.N`, adding `ec2RemoveRuleSource`/`ec2RevokeByRuleIDs`, and materializing the default VPC egress rule as `SecurityGroupRule` rows on group creation. SDK tests in `simulators/aws/sdk-tests/ec2_networking_coverage_test.go` and CLI tests in `simulators/aws/cli-tests/ec2_networking_coverage_test.go` cover ingress/egress revoke-by-id and idempotency.

- **BUG-2266 — AWS CLI test-suite version drift.** `simulators/aws/cli-tests/helpers_test.go` now uses the host AWS CLI when present and working, and installs the latest AWS CLI v2 into a temp dir when it is missing or broken. The suite controls its own reference adaptor version, satisfying the no-skip-if-absent rule.

Full CI green on PR #728.

**Next:** pick next task from PLAN.md / open issues / BUGS.md.

---
### Prior branch (merged #725): AWS sim revoke/filter validation (BUG-2262/2263/2264)

The branch fixes two simulator fidelity gaps filed from open issues #722 and #723:

- **BUG-2262 — EC2 `RevokeSecurityGroupIngress`/`Egress` succeed for non-existent rules.** `simulators/aws/ec2.go` now checks whether the requested permission exists before mutating the security group; a second revoke of the same rule returns `InvalidPermission.NotFound`, matching the AWS SDK for Go v2 documentation for non-default VPCs. A new `ec2PermissionExists` helper compares protocol, ports, and CIDR ranges.

- **BUG-2263 — CloudWatch Logs `PutMetricFilter`/`PutSubscriptionFilter` stored invalid `FilterPattern` values.** `simulators/aws/cloudwatch_logs_ops.go` now calls `cwCompileLogPattern` in both handlers and returns `InvalidParameterException` for malformed patterns, matching the CloudWatch Logs Smithy model. `simulators/aws/cloudwatch_filter_pattern.go` was also corrected so that `{` (an unbalanced brace) is rejected as a malformed structured pattern instead of treated as an unstructured term.

SDK and CLI tests were added for both fixes:
- `simulators/aws/sdk-tests/ec2_networking_coverage_test.go` — `TestEC2_RevokeSecurityGroupRules`
- `simulators/aws/cli-tests/ec2_networking_coverage_test.go` — `TestEC2CLI_RevokeSecurityGroupRules`
- `simulators/aws/sdk-tests/cloudwatch_logs_failloud_test.go` — `TestCloudWatchLogs_PutMetricFilterRejectsInvalidPattern`, `TestCloudWatchLogs_PutSubscriptionFilterRejectsInvalidPattern`
- `simulators/aws/cli-tests/cloudwatch_logs_ops_test.go` — `TestLogs_PutMetricFilterCLIRejectsInvalidPattern`, `TestLogs_PutSubscriptionFilterCLIRejectsInvalidPattern`

**CI-caught follow-up: BUG-2264 — VPC security groups created without the default ALLOW ALL egress rule.** After the revoke-not-found fix landed, the AWS Terraform production-shape test (`TestStackProductionShape`) failed because `terraform-provider-aws` revokes the default egress rule that real AWS creates with every VPC security group. Fixed in `simulators/aws/ec2.go` by initializing `IpPermissionsEgress` with the default rule in `handleCreateSecurityGroup` when `VpcId` is present. Existing SDK tests that assumed empty egress were updated to revoke the default first; `TestStackProductionShape` now passes.

All targeted SDK/CLI tests, the full AWS SDK test suite, AWS sim unit tests, `TestStackProductionShape`, and `make lint` pass.

**Next:** pick next task from PLAN.md / open issues / BUGS.md.

---
### Prior branch (merged #724): bleephub + UI audit (BUG-2261)
`feat/bleephub-comprehensive-audit-2026-06-29` fixed the GraphQL panic on `query($A:){A}` via `graphqlValidateNoPanic` in `bleephub/gh_graphql.go`, fixed the `DataTable` column-merging rendering bug in `ui/packages/core/src/components/DataTable.tsx`, simplified the workflow run detail jobs table in `ui/packages/bleephub/src/pages/WorkflowDetailPage.tsx`, added Playwright console/page-error failure hooks, and spot-checked GitHub API fidelity with curl. bleephub Go tests/race/lint, UI typecheck/test/build, and Playwright e2e (21/21) pass.
`feat/bleephub-ui-audit-2026-06-29` fixed org-aware PR owner rendering: GraphQL `PullRequest.headRepositoryOwner` now resolves the organization from `repo.FullName` for org-owned repos, and REST PR `head.user`/`base.user` now use the snake_case `simple-user` shape via the new `repoOwnerREST` helper. Added `TestPRGraphQL_OrgOwnedHeadRepositoryOwner` and extended `TestCreatePullRequestREST`. Playwright e2e passed with 31 screenshots; extended fuzz targets passed; UI tests/typecheck/build and Go tests/race/lint all pass.

---
### Prior branch (merged #718): continuity rotation after #717
No code change; `STATUS.md`/`DO_NEXT.md` reconciled to the post-#717 state.

---
### Prior branch (merged #717): bleephub fidelity audit (BUG-2256/2257)
`feat/bleephub-fidelity-audit-2026-06-29` implemented runner `AgentRefreshMessage` broker delivery (`sendAgentRefreshMessage` in `broker.go` + site-admin `POST /internal/agents/{agent_id}/refresh-message` in `handle_mgmt.go`), fixed GraphQL `repositoryOwner(login:)` to return real organization data via `orgToGraphQL` instead of a synthetic partial User-shaped payload, and corrected the stale "Artifact + cache stubs" comment in `server.go`. Added `broker_refresh_test.go` and `TestRepoGraphQL_RepositoryOwnerOrg`. All bleephub Go tests, fuzz targets, race tests, UI tests/typecheck/build, and both Docker integration test suites pass; `make lint` clean.

---
### Prior branch (merged #716): continuity rotation after #715
No code change; `STATUS.md`/`DO_NEXT.md` reconciled to the post-#715 state.

---
### Prior branch (merged #715): AWS Budgets Terraform parity (#714, BUG-2255)
`fix/aws-budgets-terraform-parity-714` closed the Terraform lifecycle gaps in the AWS Budgets service slice. `CreateBudget`/`DescribeBudget`/`DeleteBudget`/`UpdateBudget`/`DescribeBudgets` now derive `AccountId` from `awsAccountID()` when the request omits it, matching real AWS behavior when the caller's signing credentials supply the account (the path used by `terraform-provider-aws` with `skip_requesting_account_id = true`). `ListTagsForResource`, `TagResource`, and `UntagResource` are implemented so `aws_budgets_budget` can complete its Create+Read+Delete cycle. SDK tests cover tag round-trips and the implicit-account raw-HTTP path; the terraform-tests production-shape stack gained an `aws_budgets_budget` resource plus endpoint alias and assertions. Boyscout: corrected the SQS missing-queue error `__type` to `AWS.SimpleQueueService.NonExistentQueue`.

---
### Prior branch (merged #713): AWS simulator stored-but-not-enforced sweep + Budgets service slice (#703-#712, BUG-2242 through BUG-2251, plus CI-caught BUG-2253/2254)
Closed all ten open AWS-focused GitHub issues. Each fix ships real side effects and SDK tests: SQS DLQ redrive, ACM real PEM minting, AWS Budgets service slice, Route 53 DNS server, CloudWatch Logs metric-filter→metric publishing, CloudWatch alarm→SNS dispatch, Application Auto Scaling target tracking for ECS, ELBv2 HTTPS/TLS termination, ECS service scheduler, and EC2 security-group host-firewall enforcement. Added `allowedNonSpecTargets` to `spec_conformance_test.go` for the Budgets service, which is real but not in the vendored Smithy corpus. Added deterministic unit tests for the ECS scheduler because the SDK integration test requires a healthy container runtime. Identified and filed BUG-2252: the conformance/coverage gates do not catch behavioral side-effect gaps (background evaluators, protocol listeners, cross-service dispatch); documented in WHAT_WE_DID.md.

The PR #713 CI run surfaced two regressions in the new code and they were fixed in the same branch:
- **BUG-2253:** `TestECS_ServiceScheduler_ReconcilesDesiredCount` flaked because `DescribeServices.RunningCount` lagged behind `DescribeTasks.LastStatus`. Fixed by computing the counts from the live task set in `handleECSDescribeServices`.
- **BUG-2254:** #712's host-firewall enforcement installed a deny-all filter when an awsvpc task/ENI had no explicit security groups, breaking VPC reachability tests. Fixed by clearing the ingress filter instead of applying an empty ruleset when the SG list is empty.

---
### Prior branch (merged #702): second GCP gRPC round (Cloud KMS + Secret Manager) + Compute v1 control-plane tranche #2 (BUG-2240) + AWS ECS ExecuteCommand flake fix (BUG-2241)
`feat/gcp-ratchet-5-grpc` — second GCP gRPC round (Cloud KMS + Secret Manager) + Compute v1 control-plane tranche #2 (BUG-2240), plus boyscout fix for AWS ECS ExecuteCommand flake (BUG-2241). gcp build/lint(0)/vet clean; new SDK tests green; conformance gates green; AWS ECS exec tests updated for the systematic RUNNING-after-start fix. Merged via PR #702.

---
### Prior branch (merged #700): native gRPC data planes for Firestore/Pub/Sub/Spanner + Compute v1 control-plane tranche (BUG-2239)
The high-level Go SDK clients `cloud.google.com/go/{firestore,pubsub,spanner}.NewClient` are gRPC-only and previously could not target the sim at all; this PR extended the Bigtable gRPC transport pattern (#656) to all three, each sharing the existing REST slice's store. Firestore gRPC (~980 lines, full document CRUD + server-streaming BatchGetDocuments/RunQuery reusing the REST evaluator + transactions + BatchWrite; Listen/Write Unimplemented with justification). Pub/Sub gRPC (~1300 lines, Publisher+Subscriber+SchemaService with real at-least-once delivery via a background ack-deadline sweeper; review caught + fixed 4 `projects/projects/...` double-prefix bugs). Spanner gRPC (~520 lines, the defining ExecuteSql/Read need real table storage → per-database in-memory SQLite engine reconciled from `spannerDDLs`, runs REAL parameterized queries — no stubs). Plus a Compute v1 metadata tranche (`compute_more2.go`, 440→864/1994) + IAM triplets on 10 resources; review caught + fixed a nodeTypes catalog fake (hardcoded cpu/memory → real per-name specs). Boyscout: pre-push spanner v1.91.0→v1.92.0 dep bump. gcp build/lint(0)/vet clean; all _GRPC SDK suites green (×2); 0 new spec violations.

---
### Prior branch (merged #698): Bigtable gRPC data-plane coverage close-out (BUG-2237) + CloudBuild test-hang fix (BUG-2238) + "No skip-if-absent tests" rule
PR #656 had already landed the Bigtable Data API v2 gRPC slice; #698 closed its coverage gaps: 18 SDK filter subtests (one per implemented RowFilter, real survival semantics), DeleteCellsInColumn/Family/TimestampRange mutations, AppendValue, ApplyBulk (incl. partial-failure per-entry status), SampleRowKeys, and an unconditional `cbt` CLI test (built in TestMain via `go install cloud.google.com/go/bigtable/cmd/cbt@v1.13.0` — the reference implementation of the new no-skip-if-absent rule). Boyscout: bounded every `docker` call in `TestCloudBuild_FaithfulBuildPush`/setup via `dockerCLIWithTimeout` + a 120s build POST (BUG-2238 — a wedged container runtime now fails in ~3min instead of hanging the sdk-test suite 8–10m); added the "No skip-if-absent tests" section to AGENTS.md; pre-push dep bumps (smithy-go v1.27.2→v1.27.3, configure-aws-credentials v6.2.0→v6.2.1).

---
### Prior branch (merged #697): GCP ratchet round 3 (BUG-2235) + realexec netns robustness (BUG-2236)
Compute Engine v1 174→440/1994 (control plane: images/snapshots, load balancers, instance-group managers, instance actions, catalog reads — metadata-only/host-agnostic); Cloud Run v2 62→89/116 (worker pools, instances, UpdateJob/DeleteExecution/Tasks + a `:getIamPolicy` colon-split fix); GCP 3180→3473/5244 (61%→66%). Plus a CI-caught realexec fix: `CreateNetworkNamespace` reclaims an orphan netns before retrying once (GCP ratchet's Compute SDK test materializes a host-global netns; a killed-process leak made the next sim process fail `ip netns add …: File exists`).

---
### Prior branch (merged #696): fourth Azure service ratchet (BUG-2234)
Logic Apps 100%, App Service/Web Apps 37→161/692, Cosmos DB both versions 100% + PEC + Log Analytics query, API Management + PostgreSQL to 100%, Resources 36/40; Azure 1409→1758/2597 (54%→68%). Plus BUG-2233 (Web FunctionEnvelope name leak).

---
### Prior branch (merged #695): third Azure service ratchet (BUG-2229) + CI-caught fixes (BUG-2230/2231/2232)
Cosmos DB (Mongo/Cassandra/Gremlin families) + Event Grid (both docs 100%, partner family) + API Management (apis 52/91, five docs 100%) + PostgreSQL/Resources/subscriptions/App Insights; Azure 1000→1409/2597. Plus CI-caught fixes: async-op Retry-After (30s→1s polls), CLI timeout budget, Event Grid keyGeneration leak, and a GCP dep-cascade build fix.

---
### Prior branch (merged #694): second Azure service ratchet (BUG-2226) + CI-caught fixes (BUG-2227)
Storage ARM (blob/file/queue/table 100%), DNS/Private DNS/LB/NIC/Public IP/VNet all 100%, Redis/Key Vault/Managed Identity all 100%, Container Instances 100% + RBAC up; Azure 857→1000/2597. Plus two CI-caught test fixes (org-account-ordering flake, stale KeyPermission assertion).

---
### Prior branch (merged #693): first Azure service ratchet (BUG-2224) + EC2 ClientToken idempotency (BUG-2225)
Container Apps / Container Registry / Service Bus + Event Hubs all to 100%, Networking up; Azure 630→857/2597. Plus a CI-caught boyscout fix: EC2 `RunInstances` honors `ClientToken` idempotency.

---
### Prior branch (merged #692): ELBv2 NLB stable DNSName (#691, BUG-2223)
Reverted #683's host:port DNSName hijack — DescribeLoadBalancers returns the stable AWS-shaped hostname again; reachability via listener-port bind + ExtraHosts hostname resolution (per-NLB loopback IP on Linux). Plus the appdata CLI shard split (flakiness).

---
### Prior branch (merged #690): ELBv2 TCP target group HealthCheckPath (#688, BUG-2222)
Same HTTP-only class as #685's Matcher — `HealthCheckPath` was defaulted/emitted for every protocol; now omitted for non-HTTP health checks (`elbv2MatcherApplies` → `elbv2HTTPHealthCheck` + `elbv2DefaultedHealthCheckPath`). SDK/CLI + a TCP `health_check` block in the idempotency TF stack.

---
### Prior branch (merged #689): GCP coverage ratchet round 2 + Azure operation-coverage gate (BUG-2220/2221)
Built `azureMethodFloor` in `simulators/azure/azure_coverage_test.go` (the Swagger-spec analogue of `serviceCoverageFloor`/`gcpMethodFloor`, ratchet over 90 swagger files — all three sims now gated; Azure 630/2597 = 24%); GCP ratcheted 2413→3180/5244 (46%→61%) with ~22 services at 100% (Spanner, Cloud SQL v1, VPC Access, ServiceUsage, IAM Credentials, Dataflow to 100%; CRM v3 11→105, Logging 170→480, Bigtable 65→136, Cloud Run/Functions up); plus a smoke-build proxy-retry resilience fix (BUG-2221).

---
### Prior branch (merged #687): CI flake hardening + ELBv2 #685/#683 + CloudTrail (BUG-2216/2217/2218/2219)
- Flaky-pattern hardening across AWS/GCP/Azure test suites (~20 racy waits → poll-until / widened deadlines; no assertion weakened).
- ELBv2 #685: omit HealthCheck `Matcher` for non-HTTP/HTTPS health checks (terraform idempotency). ELBv2 #683: real NLB raw-TCP data plane, made discoverable via DescribeLoadBalancers (a client `net.Dial`s the reported endpoint). CloudTrail: added the missing ElastiCache `2015-02-02` eventSource mapping (events were being dropped).

---
### Prior branch (merged #686): GCP operation-coverage gate + ratchet (BUG-2214/2215)
Brought the GCP simulator's conformance gate up to AWS parity, all spec-validated against the Discovery schemas (0 new divergences) + real Google Cloud Go SDK.

- **GCP had route-validity + doc-consumption gates but no operation-coverage ratchet.** Built `gcpMethodFloor` in `gcp_coverage_test.go` — per vendored Google Discovery document, it counts how many REST methods the sim implements (a method is covered when a registered route matches its HTTP-method + normalized path under the same `matchSegs` rules the route-validity gate uses) and locks the count with an exact-equality ratchet. `TestServiceConformance_GCPCoverage` logs the per-doc fraction; `TestServiceConformance_GCPCoverageFloor` is the ratchet.
- **12 mid-size services ratcheted** (2 rounds, one service per agent): **Cloud Build 104→130/130, Memorystore Redis 64→90/90, Firestore admin 89→112/112, Cloud Storage JSON 32→84/84 (all 100%)**; Cloud KMS 122→157/166 (real Go-stdlib crypto for mac/raw/asymmetric/generateRandomBytes; honest metadata-only for EKM/HSM/post-quantum decapsulate), IAM admin 204→264/266 (workload/workforce identity pools, OAuth clients, custom roles, SA keys), Artifact Registry 97→144/147 (packages/versions/tags/files/rules/attachments), Eventarc 97→124/132, BigQuery 38→86/95, Cloud DNS 24→74/80 (real DNSSEC DS digests), Pub/Sub 77→86/92 (schemas), Secret Manager 50→60/64 (regional).
- **GCP coverage 1986→2413/5244 (38%→46%); 6 GCP services now at 100%.**
- The consistently-uncovered remainder per service is the `{+name}`/`{+resource}` reserved-expansion *template alternates* the Discovery docs list alongside each flatPath — the flatPath form every real client uses is covered; matching the template form would need an over-broad catch-all the route-validity gate forbids.
- Integration: the whole module built once all agents finished; floor bumps reconciled from a single measured-coverage pass; 2 staticcheck (dns ECDSA embedded-field) + 1 unused func (iam) cleared.
- Tests: gcp sim build/lint(0) green; route-validity + doc-consumption + coverage-floor gates pass; per-service spec-validator 0 new violations.

**Next candidates:** keep ratcheting GCP — the larger mid-size services (**Spanner 186/198, SQL Admin 136/148**, the small-gap batch **API Gateway 54/60 / ServiceUsage 19/20 / VPC Access 15/16 / IAM Credentials 11/14**), then the big surfaces (**Cloud Run 61/152, Logging 154/508, Bigtable Admin 62/162, Cloud Resource Manager 11/124, Cloud Functions 15/42, Dataflow 8/84**; Compute 174/1994 is enormous — ratchet selectively). Then the **Azure simulator** (no coverage gate yet — build the equivalent), and the **live-cloud track (BUG-1075)**. Open GitHub issues: #394 (azuread upstream-blocked).

## Working agreement

The full before/after-task continuity-file workflow, the no-fakes rules, and branch/PR hygiene live in [AGENTS.md](AGENTS.md). In short: read `STATUS.md`/`DO_NEXT.md` first; run the narrowest meaningful tests for the touched area; file bugs before fixing; update the continuity files in the same commit as the code; rebase on `origin/main` before pushing; never merge the PR.

Narrowest-test recipes for the common surfaces:

```bash
# Simulator SDK probe
cd simulators/<cloud>/sdk-tests && GOWORK=off CGO_ENABLED=0 go test -tags noui -run '<pat>' -timeout 15m .
# Simulator module unit tests + lint
cd simulators/<cloud> && make unit-test
# A backend's unit tests
cd backends/<name> && GOWORK=off go test ./...
# bleephub runner topology harness (self-contained)
make bleephub-runner-docker-test
```
