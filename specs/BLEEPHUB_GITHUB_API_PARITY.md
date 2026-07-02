# bleephub ↔ GitHub API signature parity

Status: **Phase 155 in progress on branch `feat/bleephub-full-api-ui-parity`**. Original audit date: 2026-05-12; refreshed 2026-07-02.

> **Goal:** every bleephub HTTP endpoint matches real GitHub's path + request shape + response shape exactly, modulo base domain. A client built against GitHub or GitHub Enterprise Server (GHES) should round-trip against bleephub by swapping the base URL only.

This doc is the audit artifact + acceptance criteria. Current coverage against the vendored GitHub OpenAPI description: **665 / 1,190 operations registered (56%)**; **595 operations remain unimplemented**.

## Base-URL convention

bleephub follows **GHES path shapes**, not `api.github.com` shapes:

- REST: `http(s)://<bleephub-host>/api/v3/...` (matches GHES).
- GraphQL: `http(s)://<bleephub-host>/api/graphql` (matches GHES; on api.github.com it's `/graphql`).
- OAuth: `http(s)://<bleephub-host>/login/{device,oauth}/...` (matches both).
- Runner protocol: `http(s)://<bleephub-host>/_apis/v1/...` (GHES Actions service).
- Git smart HTTP: `http(s)://<bleephub-host>/<owner>/<repo>.git` (matches both).

Rationale: the official `actions/runner` is GHES-aware (`/_apis/` is a GHES path), and switching bleephub to `api.github.com` shapes would break the runner. Clients targeting `api.github.com` paths point their base URL at `http://localhost:5555/api/v3` (same swap GHES users do). The parity acceptance criterion is "GHES path shapes match GHES exactly", not "api.github.com path shapes match api.github.com exactly".

## Coverage summary

| Phase | Status | Surfaces | Ops added (approx) |
|---|---|---|---|
| 153 | shipped | GitHub Apps, OAuth apps, webhooks, checks, auth, repos, issues, pulls, gists, users, orgs, GraphQL compatibility | ~300 |
| 154 | shipped | Reactions, releases, deployments, environments, PR review comments, PR threads, users extras, OIDC, Pages, branch protection, org audit log, marketplace | +~120 |
| 155 | in progress | Teams, issue management, PR reviews, Git data writes, release assets, repo settings, org rulesets, Dependabot org/repo, secret scanning org/repo, security advisories, Actions permissions/runners, gist extras, users extras, notifications | +~122 net |

## Phase 155 surfaces shipped

| Surface | Files |
|---|---|
| Teams (members, memberships, repo grants) | `bleephub/gh_teams_rest.go`, `bleephub/store_orgs.go` |
| Issue management (comments, labels, assignees, lock, events, timeline) | `bleephub/gh_issues_rest.go`, `bleephub/store_issues.go`, `bleephub/gh_labels_rest.go`, `bleephub/gh_issue_moderation.go` |
| Pull Request reviews (create/submit/dismiss/requested reviewers/update-branch) | `bleephub/gh_pulls_rest.go`, `bleephub/store_pulls.go` |
| Git data REST writes (blobs, commits, refs, tags, trees) | `bleephub/gh_repos_git.go` |
| Release assets and release reactions | `bleephub/gh_releases.go` |
| Repository settings (deploy keys, transfer, rename, subscription, alerts, interaction limits) | `bleephub/gh_repos_rest.go`, `bleephub/store_repos.go` |
| Organization rulesets | `bleephub/gh_rulesets.go`, `bleephub/store_rulesets.go` |
| Dependabot org/repo alerts and secrets | `bleephub/gh_dependabot.go`, `bleephub/store_dependabot.go` |
| Secret scanning org/repo alerts and pattern configurations | `bleephub/gh_secret_scanning.go`, `bleephub/store_secret_scanning.go` |
| Repository security advisories | `bleephub/gh_security_advisories.go`, `bleephub/store_security_advisories.go` |
| Actions permissions and runner labels | `bleephub/gh_actions_permissions.go` |
| User/gist extras (events, blocks, social accounts, SSH signing keys, starred, gist forks/commits) | `bleephub/gh_misc_endpoints_users.go`, `bleephub/gh_gists_rest.go` |
| Notification thread subscriptions | `bleephub/gh_notifications.go`, `bleephub/store_notifications.go` |
| UI pages for teams, repo settings, security advisories, rulesets, notifications, gists | `ui/packages/bleephub/src/pages/*` |

## Removed from /api/v3

The following were implemented during Phase 155 but removed before final validation because they are not documented public GitHub `/api/v3` paths:

- **Projects v2 REST endpoints** — GitHub Projects v2 is GraphQL-only in the public API. The GraphQL surface remains at `/api/graphql`.
- **User-scoped Dependabot secrets** — Real GitHub has repo/org Dependabot secrets, not user-scoped.
- **User-scoped secret-scanning alerts** — Real GitHub has repo/org alerts, not user-scoped.

The `allowedBleephubExtensions` escape hatch in `gh_api_definition_test.go` was also removed; the API-definition ratchet now enforces only real GitHub paths plus the documented GHES-only allowlist and dispatch routes.

## Remaining gap inventory (595 operations)

Grouped by surface. These are tracked for future phases.

### Enterprise / admin

- Enterprise teams, enterprise code security, enterprise copilot metrics/policies, enterprise dependabot alerts.
- Organization roles / custom repository roles.
- Billing / budgets / network configurations / private registries.
- Attestations, agent tasks.

### Classroom / education

- Classrooms, assignments, grades.

### Copilot

- Copilot billing, seats, metrics, content exclusion, coding agent permissions, spaces.

### Marketplace

- Marketplace listing accounts and stubbed plans.

### Misc novelty

- `/events`, `/feeds`, `/emojis`, `/octocat`, `/zen`, `/versions`, `/codes_of_conduct`.

### Already substantial but not exhaustive

- Teams: invitations, project assignments, team discussions (legacy).
- Actions: hosted runners, agent secrets/variables, concurrency groups.
- Security: code security configurations (shape-only), org-level Dependabot repository-access, security managers.
- Users: full user profile management, email visibility.
- Gists: more exhaustive history/revision handling.

## Semantic gaps

### G1 — Permission enforcement on installation tokens

**Status: partially fixed.** `requirePerm(scope, level)` is now used across many endpoints, but coverage is not yet universal.

### G2 — `repository_selection: "selected"` with allow-list

**Status: not fixed.** `repository_selection` is still hard-coded to `"all"` for installations.

### G3 — Webhook payload `installation` field + headers

**Status: not fixed.** Webhook payloads still omit the `installation` block and the four `X-GitHub-Hook-*` headers.

### G4 — App-targeted webhook events not fired

**Status: not fixed.** Installation lifecycle events are still silent.

### G5 — OAuth token prefixes + refresh tokens

**Status: fixed.** bleephub mints `ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_` prefixes.

### G6 — App-level webhook config

**Status: partially fixed.** `/app/hook` routes exist; config shape and delivery surface need audit.

### G7 — JSON shape (HATEOAS + missing fields)

**Status: partially fixed.** Some serializers emit full URLs; Apps/Installations and older surfaces still omit several HATEOAS fields.

## Acceptance criteria

1. Every `/api/v3` route added in Phase 155 is a real public GitHub path (or a documented GHES-only endpoint in `allowedGHESOnly`).
2. Every new endpoint responds with documented status codes, headers, and JSON shapes; passes the vendored OpenAPI response-shape validator.
3. New write-class endpoints carry `requirePerm` with the correct GitHub Apps permission scope.
4. Tests cover every new route, including at least one positive path and one 404/403 path.
5. UI pages exist for all new admin-facing surfaces.
6. `go test ./bleephub -count=1` and `make bleephub/lint` remain green.
7. OpenAPI violation allowlist grows only with documented, real-GHES-only divergences.

## Implementation order — Phase 155

1. Teams API completion.
2. Issue comments, assignees, labels, lock, pin, timeline.
3. PR reviews + requested reviewers + update-branch.
4. Git data REST writes.
5. Release assets + release reactions.
6. Repository settings.
7. Org rulesets.
8. Dependabot org/repo surfaces.
9. Secret scanning org/repo surfaces.
10. Security advisories.
11. Actions permissions + runner labels.
12. User/gist extras.
13. Notification thread subscriptions.
14. UI pages for all new surfaces.
15. Validation and continuity-file update.

## Non-goals

- Fine-grained PATs (`github_pat_`).
- Multiple GitHub Apps per `client_id` (1:1 mapping in bleephub).
- SAML / SSO enforcement on installation tokens (Enterprise-only behavior).
- Copilot billing / usage / Marketplace transactions.
- GitHub Classroom / Assignments (education domain).
- GitHub AE / EMU enterprise-only endpoints.
- Projects v2 REST (GraphQL-only in real GitHub).
- User-scoped Dependabot secrets / secret-scanning alerts (not public GitHub endpoints).

## Notes

- The active branch is `feat/bleephub-full-api-ui-parity`.
- Coverage numbers are measured against `bleephub/testdata/github-openapi.json.gz` via `TestRegisteredAPIv3RoutesExistInGitHubSpec`.
- When a real consumer reports a 404 or shape mismatch, open an issue and update this doc.
