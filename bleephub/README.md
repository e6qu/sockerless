# bleephub

bleephub is a self-contained Go reimplementation of GitHub's server-side surface — enough for the official `actions/runner`, the `gh` CLI, octokit, and probot to talk to a local process exactly as they would talk to github.com or GitHub Enterprise Server (GHES).

The runner-server protocol uses GHES-style `/_apis/` paths over five internal services. The REST + GraphQL API uses GHES-style `/api/v3/` (REST) and `/api/graphql`. Both are served from the same binary on the same port.

## Reference adaptors

bleephub is paired with the external GitHub-compatible tools that drive it. Anything these tools do against `github.com` (or a GHES instance) must work against bleephub.

| Adaptor | Min version | What it proves |
|---|---|---|
| [`gh` CLI](https://cli.github.com/manual/) | 2.50+ | End-to-end CLI verbs against `--hostname localhost` — repos, issues, PRs, releases, run / view / list. See [`docs/BLEEPHUB_GH_CLI.md`](../docs/BLEEPHUB_GH_CLI.md). |
| [`actions/runner`](https://github.com/actions/runner) (official binary) | v2.319+ | The runner-server `/_apis/` protocol — token, agent registration, broker long-poll, run service, timeline/logs upload. |
| [Smart-HTTP git](https://git-scm.com/docs/http-protocol) (`go-git`) | git 2.40+ | `git clone` / `git push` over `https://localhost/{owner}/{repo}.git`. Used by `actions/checkout`. |
| [GitHub REST API spec](https://docs.github.com/en/rest) | 2022-11-28 | The authoritative reference for paths, request bodies, response envelopes, and `Link`-header pagination. |
| [GitHub GraphQL schema](https://docs.github.com/en/graphql/reference) | 2022-11-28 | The `IssueOrPullRequest` union, connection shapes, enum values. |

The audit artifact mapping bleephub's coverage to GitHub-real shapes (per-route + per-field) lives at [`specs/BLEEPHUB_GITHUB_API_PARITY.md`](../specs/BLEEPHUB_GITHUB_API_PARITY.md).

## Quick start — bleephub + `gh` CLI in 5 steps

`gh` is HTTPS-only against any non-`github.com` host, and it identifies the target by **hostname** (no base URL flag). The `--hostname` argument on `gh auth login` is what wires it up; once that and `GH_HOST` are set, every `gh` command builds `https://<host>/api/v3/...` automatically and bleephub serves it.

```bash
# 1. Build (UI first so the Go binary embeds it; both steps optional if you only need the API)
cd ui/packages/bleephub && bun install && bun run build      # → ui/packages/bleephub/dist/
cd ../../../bleephub && make build                           # → ./sockerless-bleephub (embeds dist/)

# 2. Generate + trust a localhost TLS cert (gh requires HTTPS)
openssl req -x509 -newkey rsa:2048 -days 1 -nodes \
  -keyout /tmp/bph.key -out /tmp/bph.crt \
  -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
# macOS — trust the cert system-wide:
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain /tmp/bph.crt
# Linux (Debian/Ubuntu):
# sudo cp /tmp/bph.crt /usr/local/share/ca-certificates/bleephub.crt && sudo update-ca-certificates

# 3. Start bleephub on :443 with TLS  (use :8443 + --hostname localhost:8443 below if you prefer no sudo)
sudo BPH_TLS_CERT=/tmp/bph.crt BPH_TLS_KEY=/tmp/bph.key \
  ./sockerless-bleephub --addr :443 &

# 4. Point gh at bleephub — --hostname is the key flag
echo "ghp_0000000000000000000000000000000000000000" \
  | gh auth login --hostname localhost --with-token
export GH_HOST=localhost                                     # make it the default host

# 5. Use real gh verbs against bleephub
gh repo create demo --public
gh issue create --repo admin/demo --title "first" --body "hi"
gh issue list --repo admin/demo
gh release create v1.0.0 --repo admin/demo --title "v1"
```

For an end-to-end smoke that wraps all five steps inside Docker (TLS, CA trust, gh CLI, harness) run [`make bleephub-gh-docker-test`](#integration-tests). For the full walkthrough — supported `gh` commands, endpoints without native verbs, token prefixes, body coercion, troubleshooting — see [`docs/BLEEPHUB_GH_CLI.md`](../docs/BLEEPHUB_GH_CLI.md).

### bleephub UI

The Go binary embeds the React SPA at `/ui/` via `go embed` (build tag `!noui`, on by default). After step 3 above, open:

- `https://localhost/ui/` — bleephub dashboard. Routes (left nav): **Overview**, **Workflows** (+ per-run detail), **Runners**, **Repos**, **Apps** (GitHub Apps registry + installations + permissions form + PEM viewer), **OAuth** (OAuth Apps registry + tokens), **Metrics**.
- Auth: the UI hard-codes the seeded admin PAT (`bph_0000…`) on its API calls. There's no login form — anyone who can reach `/ui/` has admin in the current implementation. If you front bleephub with auth, do it at the reverse proxy.

For UI hacking without rebuilding the Go binary on every change:

```bash
# In one terminal: keep bleephub running from step 3 above.
# In another:
cd ui/packages/bleephub
bun install                         # one-time
bun run dev                         # Vite dev server on :5173 with HMR
# Then open http://localhost:5173/ui/ — Vite proxies /internal + /health
# to localhost:5555. For other API paths during dev, add them to
# `server.proxy` in vite.config.ts (or load the UI from bleephub's own
# /ui/ once you've rebuilt with `bun run build` && `make build`).
```

To rebuild the embedded copy (production-style) re-run `bun run build` then `make build` in `bleephub/`.

## What it implements

### Runner protocol (`/_apis/`)

| Service | Path prefix | Purpose |
|---|---|---|
| Token service | `/_apis/v1/auth/` | JWT exchange (`alg: none`, unsigned) |
| Connection data | `/_apis/connectionData` | Service discovery via GUIDs |
| Agent service | `/_apis/v1/Agent/`, `/_apis/v1/AgentPools` | Runner registration, pools, credentials |
| Broker | `/_apis/v1/AgentSession/`, `/_apis/v1/Message/` | Session management, 30s message long-poll |
| Run service | `/_apis/v1/AgentRequest/`, `/_apis/v1/FinishJob/` | Job acquire/renew/complete |
| Timeline + logs | `/_apis/v1/Timeline/`, `/_apis/v1/Logfiles/` | Step status tracking, log upload |
| Job submission | `/api/v3/bleephub/submit` | Simplified JSON job input (not part of runner protocol) |

### GitHub REST API (`/api/v3/`) — supported surface

**Repositories.** Create / list / get / update / delete; refs (branches, tags); blobs / trees / commits; smart-HTTP git (`go-git`) for `actions/checkout`.

**Issues, PRs, labels, milestones, comments.** Full CRUD, paginated lists with `Link` headers, state filters, GraphQL counterparts.

**PR review comments.** Inline / file-line / range / threads. Replies via the dedicated `/replies` endpoint OR `in_reply_to` body field. `GET /pulls/{n}/review-threads` returns threads with `isResolved`. REST helpers for resolve/unresolve (`/pulls/{n}/review-threads/{tid}/{resolve|unresolve}`). Reactions on review comments.

**Reactions.** Eight content values (`+1`, `-1`, `laugh`, `confused`, `heart`, `hooray`, `rocket`, `eyes`). Idempotent POST. Surfaces: issues, issue comments, PR review comments, commit comments, releases. `reactions{url, total_count, +1, ...}` block embedded on parent JSON.

**Releases.** Create / list / get-by-id / get-by-tag / latest / update / delete + `generate-notes` + release reactions. Full HATEOAS URLs (`html_url`, `tarball_url`, `zipball_url`, `assets_url`, `upload_url`). Webhook event fires on create.

**Deployments + Environments.** Full deployment + status + environment surface. `deployment` and `deployment_status` webhook events with `attachInstallationBlock`. Environments lazy-created on first deployment to that env.

**Actions API (workflow runs / jobs / steps).** `GET /actions/runs`, `runs/{id}`, `runs/{id}/jobs`, `runs/{id}/logs` (zip), `runs/{id}/timing`, `runs/{id}/rerun`, `runs/{id}/rerun-failed-jobs`, `runs/{id}/cancel`. `POST /repos/{o}/{r}/dispatches` for `repository_dispatch`. `workflow_dispatch` via `POST /actions/workflows/{id}/dispatches`.

**Checks API.** `check-runs` create/get/update/list-by-commit/list-by-suite/annotations. `check-suites` get/list-by-commit/preferences. App-owned: writes require `checks:write` on an installation token.

**Webhooks.** Per-repo + app-level. `installation:{id, node_id}` block on every payload when the event flows through an app installation. Full header set: `X-GitHub-Event`, `X-GitHub-Delivery`, `X-GitHub-Hook-ID`, `X-GitHub-Hook-Installation-Target-Type/-Target-ID`, `X-Hub-Signature` (SHA1) + `X-Hub-Signature-256`. Redelivery: `POST /hooks/{id}/deliveries/{delivery_id}/attempts` and `/app/hook/deliveries/{id}/attempts`.

**GitHub Apps.**
- Manifest flow: `POST /app-manifests/{code}/conversions`.
- App lookup: `GET /apps/{slug}` (anon), `GET /app` (JWT).
- Installations: `GET/DELETE /app/installations/{id}`, suspend / unsuspend, installation tokens with `repository_ids` subset, repo-selection management (`PUT/DELETE /user/installations/{id}/repositories/{repo_id}`).
- App webhook: `GET/PATCH /app/hook/config`, `GET /app/hook/deliveries[/{id}][/attempts]`.
- Installation events: `installation`, `installation_repositories` fire on store transitions.
- JWT verification: RS256, 600s max lifetime, iat/exp validation.

**OAuth Apps.** Distinct entity from GitHub Apps. `POST /api/v3/bleephub/oauth-apps` (sim management). OAuth web flow (`/login/oauth/authorize`) + device flow (`/login/device/code`). Token-management family on `/api/v3/applications/{client_id}/{token,grant}` (check / reset / revoke / scope).

**Token prefixes.** Match real GH exactly: `ghp_` (PAT), `gho_` (OAuth App user-to-server), `ghu_` (GitHub App user-to-server), `ghs_` (server-to-server installation), `ghr_` (refresh). Middleware distinguishes all five.

**Permission enforcement.** `requirePerm(scope, level)` decorator gates write-class endpoints. PAT bypass (matches real GH PATs being full-scope). `ghs_` tokens checked against `InstallationToken.Permissions`; `ghu_` against the App's installation perms; `gho_` mapped from classic OAuth scopes.

**Actions OIDC.** `GET /token` issues an RS256-signed JWT with the canonical claim set (sub, aud, repository, repository_owner, ref, run_id, run_number, sha, actor, environment, jti, exp). `GET /.well-known/jwks` + `/.well-known/openid-configuration` for cloud-IdP trust verification.

**Users API.** Public users, my-user, keys CRUD, gpg_keys (stub), emails, followers / following (stub), follow / unfollow.

**Pages.** Site CRUD + builds shape.

**Branch protection.** PUT/GET/DELETE per-branch protection rules; JSON pass-through.

**Orgs.** Create, list memberships, members, audit log (empty stub), teams, IdP-group sync (stub).

**Marketplace.** Listing plans + accounts (stub).

**GraphQL.** Repository / User / Organization queries + the IssueOrPullRequest union + repositoryOwner polymorphic root + repository.issues/pullRequests connections + check-run/check-suite types + Issue.projectItems (empty ProjectV2 stub for `gh issue view` compatibility) + matching enums (RepositoryPrivacy, RepositoryAffiliation, IssueOrderField, OrderDirection, IssueState).

### Persistence

`BLEEPHUB_PERSIST=true` enables SQLite write-through for users / tokens / apps / oauth_apps / installations / installation_tokens / user_to_server_tokens / refresh_tokens / repos. `BLEEPHUB_DATA_DIR` selects the on-disk location (default `./bleephub.db`). Open failure → `log.Fatalf` (BUG-985/986 pattern; never silent in-memory fallback). Git storage (go-git) stays in-memory; switching to filesystem.Storage is a separate phase.

### `gh` CLI compatibility

bleephub accepts what real GitHub accepts — including the string-coerced booleans / integers `gh api -f` sends (real GH's Rails layer coerces them; bleephub's `flexBool`/`flexInt`/`flexInt64`/`flexIntSlice` types decode either form). `gh` CLI works against bleephub directly:

```bash
echo "$TOKEN" | gh auth login --hostname localhost --with-token
gh repo create my-repo --public
gh issue create --repo admin/my-repo --title "test"
gh issue view 1 --repo admin/my-repo
gh issue list --repo admin/my-repo
gh repo view admin/my-repo
gh repo list admin
gh release create v1.0.0 --repo admin/my-repo
gh pr create / view / list (in a git working dir)
gh run list / view (when workflow runs exist)
```

Verified end-to-end by [`make bleephub-gh-docker-test`](#integration-tests), which builds a Docker image bundling bleephub + the official `gh` CLI + a self-signed TLS cert and runs the harness against the live bleephub binary inside the container.

## What it does not implement (deferred)

- Runner auto-update (`AgentRefreshMessage`).
- V2 broker flow (uses legacy V1 pipelines paths).
- Reusable workflows (`uses: ./.github/workflows/`).
- Composite actions.
- Full Projects v2 (only an empty stub for `gh issue view` compatibility).
- SAML SSO + SCIM provisioning.
- Per-installation audit log content (shape-only empty endpoint).
- Marketplace billing.
- gh CLI commands that require deep workflow-run state bleephub doesn't synthesise (`gh run watch` long-poll, log tail).

## How it works

```
┌──────────────────┐     internal API      ┌───────────┐     Docker API     ┌────────────┐
│  actions/runner  │ ◄──────────────────► │  bleephub │                    │            │
│  (C# binary)     │                      │  (Go)     │                    │ Sockerless │
│                  │     docker exec       │           │                    │            │
│                  │ ─────────────────────►│           │───────────────────►│            │
└──────────────────┘                      └───────────┘                    └────────────┘
```

For local end-to-end workflow runs:
1. Runner calls `config.sh --url http://bleephub/owner/repo --token ...`
2. bleephub returns registration data, agent pool, credentials.
3. Runner starts `run.sh`, creates a session, long-polls `/_apis/v1/Message/`.
4. A job is submitted via `POST /api/v3/bleephub/submit` (simplified JSON).
5. bleephub converts to the internal job-message format and delivers it.
6. Runner creates a Docker container through `DOCKER_HOST` (pointing at Sockerless).
7. Runner execs each `run:` step inside the container via `docker exec`.
8. Runner reports step status; bleephub marks the job completed.

For ad-hoc REST / GraphQL workflows (probot, octokit, `gh`):
- Point `GH_HOST=localhost` (or set the host in `gh auth login`).
- Use a token recognised by bleephub's middleware (seeded `bph_0000...` PAT works; mint your own via the OAuth flow for stricter testing).

## Usage

```bash
bleephub --addr :80 --log-level info
```

Flags:
- `--addr` — listen address (default `:5555`). Runner strips non-standard ports from URLs, so use port 80/443 for integration tests with the runner.
- `--log-level` — `debug` | `info` | `warn` | `error` (default `info`).

Env vars:
- `BLEEPHUB_PERSIST=true` — enable SQLite persistence (off by default).
- `BLEEPHUB_DATA_DIR=<dir>` — persistence + artifact directory.
- `BPH_TLS_CERT` + `BPH_TLS_KEY` — serve over TLS.
- `BLEEPHUB_MAX_WORKFLOWS=N` — concurrency cap (default 10).
- `OTEL_EXPORTER_OTLP_ENDPOINT` — when set, emits traces + metrics + logs via OTLP (off by default; preserves the components-decoupled invariant).

## Integration tests

```bash
# Go unit tests
make bleephub/test                  # go test ./bleephub/...

# Official actions/runner harness (Docker)
make bleephub/test-integration

# Real gh CLI inside Docker (real bleephub + real gh binary + self-signed TLS)
make bleephub-gh-docker-test
```

The Docker harness builds `bleephub/Dockerfile.gh-test` and runs `bleephub/test/run-gh-test.sh`. It exercises:
- `gh auth login` against bleephub as a GHES host
- Native `gh repo create / view / list`, `gh issue create / view / list` (REST + GraphQL paths)
- The Phase 153 parity probes for endpoints with no native `gh` verb (apps/{slug}, /applications/{cid}/token, suspend, OAuth Apps mgmt)

Last green run: 50/50 PASS.

## Source layout (~60 Go files)

| Group | Files | Purpose |
|---|---|---|
| Core protocol | `server.go`, `auth.go`, `agents.go`, `broker.go`, `run_service.go`, `timeline.go` | Runner registration, job delivery, lifecycle |
| Jobs & workflows | `jobs.go`, `workflow.go`, `workflows.go`, `workflows_msg.go`, `matrix.go`, `outputs.go`, `secrets.go`, `expressions.go`, `actions.go`, `artifacts.go` | Multi-job, matrix, secrets, expressions, artifacts |
| GitHub REST core | `gh_rest.go`, `gh_repos_*.go`, `gh_orgs_*.go`, `gh_issues_*.go`, `gh_pulls_*.go`, `gh_teams_rest.go`, `gh_labels_rest.go`, `gh_members_rest.go` | Repos, orgs, issues, PRs, teams, labels, milestones |
| GitHub Apps + OAuth | `gh_apps_*.go`, `gh_oauth.go`, `gh_app_hooks_rest.go`, `gh_apps_user_tokens.go`, `gh_apps_oauth_mgmt.go`, `gh_apps_perms.go` | JWT, installations, OAuth Apps, ghs_/ghu_/gho_/ghr_, permission enforcement |
| Reactions + Releases + Deployments | `gh_reactions.go`, `gh_releases.go`, `gh_deployments.go`, `gh_pr_comments.go`, `gh_pr_threads.go` | Phase 154 |
| Actions extras | `gh_actions_rest.go`, `gh_actions_extras.go`, `gh_workflows_rest.go` | Runs/jobs/steps, repository_dispatch, logs zip, timing |
| Checks API | `gh_checks_rest.go`, `gh_checks_store.go` | check-runs + check-suites |
| Misc long-tail | `gh_misc_endpoints.go` | Users keys/follow, Actions OIDC + JWKS, Pages, Branch protection, Marketplace |
| GraphQL | `gh_graphql.go`, `gh_*_graphql.go`, `gh_request_decode.go` | Schema + flex decoders |
| Webhooks | `webhooks.go`, `webhooks_store.go`, `webhooks_payloads.go`, `gh_hooks_rest.go` | HMAC-SHA256/SHA1 delivery with retry |
| Git | `git_http.go` | Smart HTTP protocol (go-git) |
| Persistence | `persistence.go` | SQLite write-through layer |
| Infrastructure | `store.go`, `store_*.go`, `rbac.go`, `metrics.go`, `otel.go`, `handle_mgmt.go`, `ui_embed.go` | State, RBAC, metrics, OTel, dashboard |

## See also

- [docs/BLEEPHUB_GH_CLI.md](../docs/BLEEPHUB_GH_CLI.md) — operator-facing `gh` setup walkthrough.
- [specs/BLEEPHUB_GITHUB_API_PARITY.md](../specs/BLEEPHUB_GITHUB_API_PARITY.md) — audit + acceptance criteria from Phase 153.
- [ARCHITECTURE.md](../ARCHITECTURE.md), [docs/GITHUB_RUNNER.md](../docs/GITHUB_RUNNER.md).

## Prior art

[ChristopherHX/runner.server](https://github.com/ChristopherHX/runner.server) (C#, 25 controllers) proved this approach works. bleephub is a from-scratch Go implementation informed by studying the runner source + runner.server's protocol handling, but shares no code with either.
