# bleephub ↔ GitHub API signature parity

Status: **shipped through Phase 154**. Original audit date: 2026-05-12.

History:
- Phase 153 — initial parity sweep (PR #153, merged 2026-05-12 as `fadf851f`).
- Phase 154 — broad API sweep covering Reactions, Releases, Deployments, Environments, PR review comments, OIDC, Pages, Branch protection, Org audit log, Marketplace (PR #154, merged 2026-05-12 as `9e03621c`).

> **Goal:** every bleephub HTTP endpoint matches real GitHub's path + request shape + response shape exactly, modulo base domain. A client built against GitHub or GHES should round-trip against bleephub by swapping the base URL only.

This doc is the audit artifact + acceptance criteria. Don't re-audit; pick up from the gap list.

## Base-URL convention

bleephub follows **GHES path shapes**, not `api.github.com` shapes:

- REST: `http(s)://<bleephub-host>/api/v3/...` (matches GHES).
- GraphQL: `http(s)://<bleephub-host>/api/graphql` (matches GHES; on api.github.com it's `/graphql`).
- OAuth: `http(s)://<bleephub-host>/login/{device,oauth}/...` (matches both).
- Runner protocol: `http(s)://<bleephub-host>/_apis/v1/...` (GHES Actions service).
- Git smart HTTP: `http(s)://<bleephub-host>/<owner>/<repo>.git` (matches both).

Rationale: the official `actions/runner` is GHES-aware (`/_apis/` is a GHES path), and switching bleephub to `api.github.com` shapes would break the runner. Clients targeting `api.github.com` paths point their base URL at `http://localhost:5555/api/v3` (same swap GHES users do). The parity acceptance criterion is "GHES path shapes match GHES exactly", not "api.github.com path shapes match api.github.com exactly".

## Endpoint inventory (target state)

Each row: path · method · auth (P=PAT, J=App JWT, S=ghs_ installation token, U=ghu_ user-to-server, O=gho_ OAuth user-to-server, A=anonymous) · status today.

### GitHub Apps — all shipped Phase 153

Every Apps endpoint in the original audit is now implemented + tested. See `bleephub/gh_apps_rest.go`, `gh_apps_user_tokens.go`, `gh_app_hooks_rest.go`, `gh_apps_oauth_mgmt.go`, `gh_apps_perms.go`.

### OAuth applications — all shipped Phase 153 (P153.5)

`/api/v3/applications/{client_id}/*` family (check / reset / revoke / scope / grant). See `bleephub/gh_apps_oauth_mgmt.go`.

### Webhooks + Checks API — all shipped Phase 153

Per-repo + app-level webhooks with full delivery view + redelivery (P153.4 + P153.7). Full Checks API (P153.8). See `bleephub/gh_hooks_rest.go`, `gh_app_hooks_rest.go`, `gh_checks_rest.go`.

### Phase 154 — additional surfaces shipped

| Surface | Commit | Files |
|---|---|---|
| Reactions (8 content types × 5 parent types) | P154.1 | `gh_reactions.go` |
| Releases (CRUD + latest + by-tag + generate-notes + reactions) | P154.2 | `gh_releases.go` |
| Actions extras (repository_dispatch, run logs zip, rerun-failed-jobs, timing) | P154.3 | `gh_actions_extras.go` |
| Deployments + Environments | P154.4 | `gh_deployments.go` |
| PR review comments (inline / file-line / threads / replies) | P154.5 | `gh_pr_comments.go` |
| PR thread resolve/unresolve REST | P154.6 | `gh_pr_threads.go` |
| Users API extras (keys, gpg, emails, follow) | P154.7 | `gh_misc_endpoints.go` |
| Actions OIDC + JWKS + discovery | P154.8 | `gh_misc_endpoints.go` |
| GitHub Pages | P154.9 | `gh_misc_endpoints.go` |
| Branch protection | P154.10 | `gh_misc_endpoints.go` |
| Org audit log | P154.11 | `gh_misc_endpoints.go` |
| Marketplace listing | P154.12 | `gh_misc_endpoints.go` |

## Semantic gaps

### G1 — Permission enforcement on installation tokens

Today: `Installation.Permissions` and `InstallationToken.Permissions` are stored + echoed but never read at request time. A `ghs_` token treated as user-equivalent (bot identity); a token with `contents: read` can mutate issues, secrets, etc. (`gh_apps_rest.go:113-123`; `gh_middleware.go:59-67`).

Fix: decorator helper `requirePerm(scope, level)` that reads `ctxInstallation` (or `ctxToken` when `ghu_`/`gho_`) and 403s on insufficient grant. Apply to all write-class endpoints. Permission scope map mirrors GitHub's: `contents`, `issues`, `pull_requests`, `metadata`, `actions`, `checks`, `secrets`, `administration`, `members`, `organization_administration`, etc., each `read` / `write` / `admin`.

### G2 — `repository_selection: "selected"` with allow-list

Today: `repository_selection` hard-coded to `"all"` (`gh_apps_store.go:133`). `GET /user/installations/{id}/repositories` enumerates every repo owned by `TargetLogin`.

Fix: add `SelectedRepoIDs []int` to `Installation`; `PUT|DELETE /user/installations/{id}/repositories/{repo_id}` toggles entries; selection respected in repos lookup and installation-token-scoped operations.

### G3 — Webhook payload `installation` field + headers

Today: push / pull_request / issues / ping payloads carry `repository` + `sender` only. No `installation: {id, node_id}` block. Headers set: `X-GitHub-Event`, `X-GitHub-Delivery`, `X-Hub-Signature-256`, `User-Agent: GitHub-Hookshot/bleephub`. Missing: `X-GitHub-Hook-ID`, `X-GitHub-Hook-Installation-Target-Type`, `X-GitHub-Hook-Installation-Target-ID`, `X-Hub-Signature` (SHA1, legacy).

Fix: when a webhook fires through an app installation, inject `installation: {id: <inst.ID>, node_id: <inst.NodeID>}` at the top level of every event payload. Add the four missing headers (compute installation target from the hook's owning installation if any).

### G4 — App-targeted webhook events not fired

Real GH emits: `installation` (created / deleted / suspend / unsuspend / new_permissions_accepted), `installation_repositories` (added / removed), `installation_target` (renamed), `github_app_authorization` (revoked). bleephub's `CreateInstallation` / `DeleteInstallation` / `Suspend*` / `Add|RemoveInstallationRepo` are silent.

Fix: each store method emits the matching event through `emitAppWebhookEvent(eventType, action, payload)` against the app's webhook config (G6's app-level webhook surface).

### G5 — OAuth token prefixes + refresh tokens

Real GH prefixes: `ghp_` (classic PAT), `gho_` (OAuth user-to-server), `ghu_` (App user-to-server), `ghs_` (server-to-server installation), `ghr_` (refresh), `github_pat_` (fine-grained). bleephub today: `bph_` for everything except `ghs_`; both device flow and web flow mint `bph_…` (`gh_oauth.go:285-295`).

Fix:
- Web flow `/login/oauth/access_token` mints `gho_…` (OAuth app context) or `ghu_…` (when client_id maps to a GitHub App's client_id).
- Add `ghr_` refresh tokens with longer expiry (default: 6 months); `PATCH /applications/{client_id}/token` rotates user-to-server token + returns new `gho_`/`ghu_` + new `ghr_`.
- Middleware recognises every prefix; `ghp_` still maps to PAT semantics for compat with the seeded admin user (token currently `bph_…` — leave a backwards-compat alias).

### G6 — App-level webhook config

Real GH apps have a webhook configured at app-creation time (separate from per-repo hooks). Operators retrieve / update it via `GET|PATCH /app/hook/config`; deliveries land at `/app/hook/deliveries`.

Fix: add `App.WebhookURL` + `App.WebhookSecret` + `App.WebhookActive` (struct already has `WebhookSecret` but no URL / active field). Per-app `HookDeliveries` indexed by app ID alongside the existing per-hook map. Wire G4 events through this surface.

### G7 — JSON shape (HATEOAS + missing fields)

`appToJSON` and `installationToJSON` omit fields real GH always includes:

- App: `html_url`, `external_url` (already present), `events_url`, `hooks_url`, `repositories_url`, `installations_count`, `slug` (present), `permissions` (present), `events` (present).
- Installation: `html_url`, `repositories_url`, `access_tokens_url`, `events_url`, `single_file_name`, `has_multiple_single_files`, `single_file_paths`, `suspended_at`, `suspended_by`, `app_id` (present), `target_id` (present), `target_type` (present), `repository_selection` (present), `permissions` (present), `events` (present), `created_at` / `updated_at` (present), `account` block (present), `app_slug` (present).

Fix: extend serialisers to construct full URLs using `s.baseURL(r)` plus the canonical path templates real GH uses (the same shapes the repos serializer at `gh_repos_rest.go:176-215` already emits).

## Acceptance criteria

1. Every row in **Endpoint inventory** answers with the documented status codes, headers, and JSON shapes. Anonymous endpoints accept no token; PAT endpoints reject JWT and vice versa.
2. **Permission gate** active on every write-class endpoint listed in real GH's [endpoints by permission](https://docs.github.com/rest/overview/permissions-required-for-github-apps) table; a `ghs_` token with `contents: read` cannot mutate issues.
3. **Webhook delivery** carries `installation: {id, node_id}` when associated; all four `X-GitHub-Hook-*` headers + `X-Hub-Signature` (SHA1) plus the existing four.
4. **Installation events** (`installation`, `installation_repositories`, `installation_target`, `github_app_authorization`) fire on the corresponding store-side transitions.
5. **Token prefixes**: `gho_` (OAuth user-to-server), `ghu_` (App user-to-server), `ghr_` (refresh), `ghs_` (existing). Middleware recognises each, sets the matching ctx value.
6. **JSON shape**: every URL field in §G7 present and resolvable.
7. **Tests**: `go test ./...` green in `bleephub/`. New tests cover every new endpoint, every permission check, every header, every payload field, every event emission, redelivery cycle, suspend cycle, token revoke + check + reset, check-run lifecycle.
8. **UI**: AppsPage gains per-installation CRUD + suspend / unsuspend + repo selection editor + permissions/events form on app create + PEM viewer + installation token mint dialog. OAuthPage gains token check + revoke. New WebhookDeliveriesPage.
9. **External reference clients**: probot's reference setup (or octokit-app) round-trips against `http://localhost:5555/api/v3/` modulo base URL. Concretely: app create → installation create → installation token mint → mutate an issue with that token (permission gate verifies) → webhook fires with `installation:{id}` → delivery visible at `/app/hook/deliveries`. Recorded as an integration test under `bleephub/test/`.

## Implementation order — shipped

Phase 153 (PR #153, merged 2026-05-12 as `fadf851f`):
1. Store + types (Installation suspend / repo-selection / OAuthApp / per-app webhook).
2. Token prefixes + middleware updates.
3. Missing Apps + OAuth endpoints.
4. App-level webhook config + deliveries.
5. Permission enforcement decorator.
6. Webhook payload `installation` field + headers + installation event emission.
7. Checks API.
8. Full JSON shape (URL fields).
9. UI updates.
10. Tests, gh CLI Docker harness end-to-end.
11. State save.
12. SQLite persistence (BLEEPHUB_PERSIST + write-through buckets).
13. Real `gh` CLI compatibility (flex decoders + GraphQL enums + union + Issue.projectItems compatibility connection).

Phase 154 (PR #154, merged 2026-05-12 as `9e03621c`):
1. Reactions API.
2. Releases API.
3. Actions extras (repository_dispatch, logs zip, timing).
4. Deployments + Environments.
5. PR review comments (inline / threads / replies).
6. Thread resolve/unresolve REST.
7-12. Users / OIDC / Pages / Branch protection / Org audit / Marketplace.

## Non-goals (carried forward through P155)

- Fine-grained PATs (`github_pat_`).
- Multiple GitHub Apps per `client_id` (1:1 mapping in bleephub).
- SAML / SSO enforcement on installation tokens (Enterprise-only behavior).
- Codespaces / Copilot / Dependabot endpoints (separate domains, not in scope).
- Per-installation audit log content (shape-only empty endpoint).
- Full Projects v2 (only the empty `Issue.projectItems` compatibility connection needed by `gh issue view`).
- Real GitHub Marketplace billing (sim returns a single Free plan).

## Future audits

When a real consumer reports a 404 or a shape mismatch against bleephub, open an issue + spec the gap here under a new "Phase 1xx" header. Don't preemptively expand without a real signal — the surface is large; depth-where-needed beats breadth-everywhere.
