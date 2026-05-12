# bleephub ↔ GitHub API signature parity (Phase 153)

Status: planned. Branch: `phase-153-bleephub-github-api-parity` (off `origin/main`). Audit date: 2026-05-12.

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

### GitHub Apps

| Path | Method | Auth | Today |
|---|---|---|---|
| `/api/v3/app-manifests/{code}/conversions` | POST | A | ✓ |
| `/api/v3/app` | GET | J | ✓ |
| `/api/v3/apps/{app_slug}` | GET | A | ❌ missing |
| `/api/v3/app/installations` | GET | J | ✓ |
| `/api/v3/app/installations/{id}` | GET | J | ✓ |
| `/api/v3/app/installations/{id}` | DELETE | J | ✓ |
| `/api/v3/app/installations/{id}/access_tokens` | POST | J | ✓ |
| `/api/v3/app/installations/{id}/suspended` | PUT | J | ❌ missing |
| `/api/v3/app/installations/{id}/suspended` | DELETE | J | ❌ missing |
| `/api/v3/app/hook/config` | GET | J | ❌ missing |
| `/api/v3/app/hook/config` | PATCH | J | ❌ missing |
| `/api/v3/app/hook/deliveries` | GET | J | ❌ missing |
| `/api/v3/app/hook/deliveries/{id}` | GET | J | ❌ missing |
| `/api/v3/app/hook/deliveries/{id}/attempts` | POST | J | ❌ missing |
| `/api/v3/orgs/{org}/installation` | GET | P/U | ❌ missing |
| `/api/v3/users/{username}/installation` | GET | P/U | ❌ missing |
| `/api/v3/repos/{owner}/{repo}/installation` | GET | P/U | ✓ |
| `/api/v3/installation/repositories` | GET | S | ❌ missing |
| `/api/v3/installation/token` | DELETE | S | ✓ |
| `/api/v3/user/installations` | GET | P/U | ✓ |
| `/api/v3/user/installations/{id}/repositories` | GET | P/U | ✓ |
| `/api/v3/user/installations/{id}/repositories/{repo_id}` | PUT | U | ❌ missing |
| `/api/v3/user/installations/{id}/repositories/{repo_id}` | DELETE | U | ❌ missing |
| `/api/v3/bleephub/apps` | POST | P | ✓ (sim-only management) |
| `/api/v3/bleephub/apps/{app_id}/installations` | POST | P | ✓ (sim-only management) |

### OAuth applications

| Path | Method | Auth | Today |
|---|---|---|---|
| `/login/oauth/authorize` | GET | A | ✓ |
| `/login/oauth/authorize` | POST | A | ✓ |
| `/login/oauth/access_token` | POST | A | ✓ |
| `/login/device/code` | POST | A | ✓ |
| `/login/device` | GET | A | ✓ |
| `/api/v3/applications/{client_id}/token` | POST | Basic(cid:cs) | ❌ missing (check token) |
| `/api/v3/applications/{client_id}/token` | PATCH | Basic(cid:cs) | ❌ missing (reset token) |
| `/api/v3/applications/{client_id}/token` | DELETE | Basic(cid:cs) | ❌ missing (revoke token) |
| `/api/v3/applications/{client_id}/token/scoped` | POST | Basic(cid:cs) | ❌ missing (scope user-to-server) |
| `/api/v3/applications/{client_id}/grant` | DELETE | Basic(cid:cs) | ❌ missing (revoke grant) |

### Webhooks (repo + app level)

| Path | Method | Auth | Today |
|---|---|---|---|
| `/api/v3/repos/{o}/{r}/hooks` | POST/GET | P | ✓ |
| `/api/v3/repos/{o}/{r}/hooks/{id}` | GET/PATCH/DELETE | P | ✓ |
| `/api/v3/repos/{o}/{r}/hooks/{id}/pings` | POST | P | ✓ |
| `/api/v3/repos/{o}/{r}/hooks/{id}/deliveries` | GET | P | ✓ (summary fields only) |
| `/api/v3/repos/{o}/{r}/hooks/{id}/deliveries/{delivery_id}` | GET | P | ❌ missing (full request/response payload) |
| `/api/v3/repos/{o}/{r}/hooks/{id}/deliveries/{delivery_id}/attempts` | POST | P | ❌ missing (redelivery) |

### Checks API (App-owned)

| Path | Method | Auth | Today |
|---|---|---|---|
| `/api/v3/repos/{o}/{r}/check-runs` | POST | S | ❌ missing |
| `/api/v3/repos/{o}/{r}/check-runs/{id}` | GET/PATCH | S | ❌ missing |
| `/api/v3/repos/{o}/{r}/check-runs/{id}/annotations` | GET | S | ❌ missing |
| `/api/v3/repos/{o}/{r}/commits/{sha}/check-runs` | GET | S/P | ❌ missing |
| `/api/v3/repos/{o}/{r}/commits/{sha}/check-suites` | GET | S/P | ❌ missing |
| `/api/v3/repos/{o}/{r}/check-suites` | POST | S | ❌ missing |
| `/api/v3/repos/{o}/{r}/check-suites/preferences` | PATCH | S | ❌ missing |
| `/api/v3/repos/{o}/{r}/check-suites/{id}` | GET | S/P | ❌ missing |

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

## Implementation order (single branch, multiple commits)

1. Store + types (Installation suspend / repo-selection / OAuthApp / per-app webhook).
2. Token prefixes + middleware updates.
3. Missing Apps + OAuth endpoints (paths from inventory above).
4. App-level webhook config + deliveries.
5. Permission enforcement decorator.
6. Webhook payload `installation` field + headers + installation event emission.
7. Checks API.
8. Full JSON shape (URL fields).
9. UI updates.
10. Tests, including the probot round-trip integration test.
11. State save (STATUS.md / DO_NEXT.md / WHAT_WE_DID.md / MEMORY.md).

## Non-goals

- GitHub Marketplace / billing / plans.
- Fine-grained PATs (`github_pat_`).
- Multiple GitHub Apps per `client_id` (1:1 mapping in bleephub).
- SAML / SSO enforcement on installation tokens (Enterprise-only behavior).
- Codespaces / Copilot / Dependabot endpoints (separate domains, not in scope).
