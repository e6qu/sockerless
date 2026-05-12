# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

`main` is clean. PR #151 (Phase 87d + 92) and PR #152 (`docs/POD_MATERIALIZATION.md`) merged 2026-05-11/12. No phase currently in flight.

Three resumable tracks below. Pick one, branch from `origin/main`, single-branch rule applies. Never auto-merge.

## Track A — Phase 153: bleephub ↔ GitHub API signature parity ⭐ next planned

Goal: every bleephub HTTP endpoint matches real GitHub's path + request shape + response shape exactly, modulo base domain. Audit details + acceptance criteria in [specs/BLEEPHUB_GITHUB_API_PARITY.md](specs/BLEEPHUB_GITHUB_API_PARITY.md).

Branch: `phase-153-bleephub-github-api-parity` off `origin/main`.

Order of operations (lands as one PR per single-branch rule, even if large):

1. **Read [specs/BLEEPHUB_GITHUB_API_PARITY.md](specs/BLEEPHUB_GITHUB_API_PARITY.md) end-to-end.** Audit groups the gaps into 7 buckets; spec lists them. Don't redo the audit.
2. **Store + types first.** Extend `bleephub/gh_apps_store.go`: `Suspended*` + `SelectedRepoIDs` + `OAuthApp` (client_id-keyed) + per-installation app webhook config. Wire to `Store.NewStore`.
3. **Token prefixes.** `gho_` for OAuth user-to-server (web flow), `ghu_` for App user-to-server (device + OAuth-with-app), `ghr_` for refresh, `ghs_` (existing) for server-to-server installation. Update `gh_oauth.go::createTokenLocked` + middleware.
4. **Missing endpoints.** Add `GET /apps/{slug}`, `GET /orgs/{org}/installation`, `GET /users/{username}/installation`, `PUT|DELETE /app/installations/{id}/suspended`, `GET /installation/repositories`, `PUT|DELETE /user/installations/{id}/repositories/{repo_id}`, `GET|POST /repos/{o}/{r}/hooks/{id}/deliveries/{delivery_id}[/attempts]`, app-level webhook surface `GET|PATCH /app/hook/config` + `GET /app/hook/deliveries[/{id}][/attempts]`, `POST|PATCH|DELETE /applications/{client_id}/token`, `DELETE /applications/{client_id}/grant`.
5. **Checks API.** `POST|GET|PATCH /repos/{o}/{r}/check-runs[/{id}]`, `GET /repos/{o}/{r}/commits/{sha}/check-runs|check-suites`, suite endpoints.
6. **Webhook payload fields + headers.** All payloads carry `installation: {id, node_id}` when associated; add `X-GitHub-Hook-ID`, `X-GitHub-Hook-Installation-Target-Type`, `X-GitHub-Hook-Installation-Target-ID`, `X-Hub-Signature` (SHA1). Emit `installation`, `installation_repositories`, `installation_target`, `github_app_authorization` events.
7. **Permission enforcement.** Decorator that reads `ctxInstallation`'s token permissions and gates write endpoints (repo, issues, pulls, checks, secrets). 403 on insufficient scope.
8. **JSON shape.** Add `*_url` fields (`hooks_url`, `html_url`, `repositories_url`, `access_tokens_url`, `events_url`, `installations_count`, `suspended_at`, `suspended_by`, `single_file_name`, etc.) to `appToJSON` + `installationToJSON`.
9. **UI.** Per-installation CRUD on AppsPage; permissions/events form on app create; PEM viewer after create; OAuthPage gains token revoke/check; webhook deliveries page.
10. **Tests.** Every new endpoint, header, payload field, redelivery, suspend cycle, OAuth token revoke + check, check-run lifecycle. `go test ./...` green in `bleephub/`.
11. **State save.** Update STATUS.md / DO_NEXT.md / WHAT_WE_DID.md / MEMORY.md / `_tasks/done/`.

Acceptance: `bleephub` answers every endpoint listed in `specs/BLEEPHUB_GITHUB_API_PARITY.md § Endpoint inventory` with the documented status codes, headers, and JSON shapes. The probot reference test suite (or octokit-app) round-trips without modification against `http://localhost:5555` modulo base URL.

## Track B — Live-cloud validation track

Outstanding live-cloud sweeps separate from sim CI. Each is one branch, one PR.

- **Lambda live** (deferred from Phase 86) — runs `make test-integration-cloud` for `lambda` with real AWS account.
- **Cloud Run Services + ACA Apps live** — `UseService` / `UseApp` paths closed in code 2026-04-21; needs cloud validation.
- **AZF + cloud-dns on Azure live** (new in #136).
- **Lambda + service-mesh on AWS live** (new in #136).
- **ACA / AZF + Azure AD access on Azure live** (new in #136).

Common shape: `make stack-{sim,backend}-up` → `make test-integration-cloud TARGET=<backend>` → file bugs as they surface (BUGS.md before any fix attempt) → cross-cloud sweep on every find. Teardown self-sufficient per `feedback_teardown_aggressive.md`.

## Track C — Phase 91d (bookmarked)

Real `pd-ephemeral` lifecycle on cloudrun + gcf. **Bookmarked indefinitely.** Cloud Run lacks the protobuf field; implementation requires either a future GCE-style sockerless backend or a Cloud Run feature change. Reject-with-pointers shape (Phase 91c, PR #149) stays in place. Don't reopen until one of those preconditions changes.

## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub.
- **No fallbacks.** Unknown config values fail-loud.
- **CI green per commit.** Each commit independently testable.
- **Test target gating.** All backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud`.
- **No docs-only PRs.** Pair docs updates with implementation work on the same branch / PR.
- **Never auto-merge.** Push the PR, wait for user.
- **Single-branch rule.** All in-flight work lands on one branch per phase.

## Session-resume checklist

1. `git status` + `git log --oneline -5 origin/main` to confirm where main is.
2. Read STATUS.md (snapshot) + this file (concrete actions).
3. For the chosen track, branch from `origin/main` with the prescribed name.
4. Run `go test ./...` from the relevant module to establish a green baseline before changes.
5. File BUGS.md entries for anything that surfaces, fix in the same session.
6. State-save before pushing: STATUS.md, this file, WHAT_WE_DID.md, MEMORY.md, BUGS.md headers.
