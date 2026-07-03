# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`fix/open-issues-765-766`. PR #767 will close GitHub issues #765 and #766, and fully resolve #763.

**Scope**
- bleephub `POST /orgs/{org}/teams` now auto-adds the authenticated creator as a team maintainer, matching real GitHub (#763/#765).
- SQS `ReceiveMessage` and `sqsEnqueueBody` debug logging to diagnose empty-receive cases (#766).
- CLI regression test that polls `receive-message` repeatedly after a CloudWatch alarm reaches `ALARM` (#766).

**Validation**
- `go test -run 'TestListAuthUserTeams|TestCreateTeam|TestTeamMembersList' -count=1 ./bleephub` passes.
- `GOWORK=off go test -run 'TestCloudWatchCLI_AlarmSNSActionToSQS_ProcessMode' -count=1 ./` in `simulators/aws/cli-tests` passes.
- `GOWORK=off go test -run 'TestCloudWatch_AlarmSNSActionToSQS' -count=1 ./` in `simulators/aws/sdk-tests` passes.
- `pre-commit run --files <changed files>` passes.

**Next:** create the new PR and update continuity files with its number.

---
### Prior branch (open, PR #767): Team creator auto-maintainer + SQS receive diagnostics

The `fix/open-issues-765-766` branch will close GitHub issues #765 and #766 and fully resolve #763.

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

**Next:** create the new PR.

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
