# bleephub

bleephub is a self-contained Go reimplementation of GitHub's server-side surface — enough for the official `actions/runner`, the `gh` CLI, octokit, and probot to talk to a local process exactly as they would talk to github.com or GitHub Enterprise Server (GHES).

The runner-server protocol uses GHES-style `/_apis/` paths over five internal services. The REST + GraphQL API uses GHES-style `/api/v3/` (REST) and `/api/graphql`. Both are served from the same binary on the same port.

## Reference adaptors

bleephub is paired with the external GitHub-compatible tools that drive it. Anything these tools do against `github.com` (or a GHES instance) must work against bleephub.

| Adaptor | Min version | What it proves |
|---|---|---|
| [`gh` CLI](https://cli.github.com/manual/) | 2.50+ | End-to-end CLI verbs against `--hostname localhost` — repos, issues, PRs, releases, run / view / list. See [`docs/BLEEPHUB_GH_CLI.md`](../docs/BLEEPHUB_GH_CLI.md). |
| [`go-github`](https://github.com/google/go-github) | v88 | Typed REST SDK coverage against the GHES-style API, including Git Data seeded repositories and Actions workflow dispatch / run / job reads. |
| [`actions/runner`](https://github.com/actions/runner) (official binary) | v2.319+ | The runner-server `/_apis/` protocol — token, agent registration, broker long-poll, run service, timeline/logs upload. |
| [Smart-HTTP git](https://git-scm.com/docs/http-protocol) (`go-git`) | git 2.40+ | `git clone` / `git push` over `https://localhost/{owner}/{repo}.git`. Used by `actions/checkout`. |
| [GitHub REST API spec](https://docs.github.com/en/rest) | 2022-11-28 | The authoritative reference for paths, request bodies, response envelopes, and `Link`-header pagination. |
| [GitHub GraphQL schema](https://docs.github.com/en/graphql/reference) | 2022-11-28 | The `IssueOrPullRequest` union, connection shapes, enum values. |

The audit artifact mapping bleephub's coverage to GitHub-real shapes (per-route + per-field) lives at [`specs/BLEEPHUB_GITHUB_API_PARITY.md`](../specs/BLEEPHUB_GITHUB_API_PARITY.md).

## Quick start — bleephub + `gh` CLI in 5 steps

`gh` is HTTPS-only against any non-`github.com` host, and it identifies the target by **hostname** (no base URL flag). The `--hostname` argument on `gh auth login` is what wires it up; once that and `GH_HOST` are set, every `gh` command builds `https://<host>/api/v3/...` automatically and bleephub serves it.

```bash
# 1. Build (UI first so the Go binary embeds it; skip the UI step if you only need the API —
#    make build falls back to a no-UI binary when ui/packages/bleephub/dist/ is missing)
cd ui/packages/bleephub && bun install && bun run build      # → ui/packages/bleephub/dist/
cd ../../../bleephub && make build                           # → ./bleephub-server (embeds dist/)

# 2. Generate + trust a localhost TLS cert (gh requires HTTPS). Idempotent —
#    safe to re-run any time; it only mints a new cert when none exists or the
#    current one is within a day of expiry, and only touches the keychain when
#    the cert isn't already trusted. Certs live under ~/.sockerless (durable),
#    NOT /tmp — /tmp is purged on reboot, and a purged cert leaves orphaned
#    trust in the keychain while the server can no longer start.
BPH_TLS_DIR="$HOME/.sockerless/bleephub-tls"
mkdir -p "$BPH_TLS_DIR"
if ! openssl x509 -checkend 86400 -noout -in "$BPH_TLS_DIR/bph.crt" 2>/dev/null; then
  openssl req -x509 -newkey rsa:2048 -days 825 -nodes \
    -keyout "$BPH_TLS_DIR/bph.key" -out "$BPH_TLS_DIR/bph.crt" \
    -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
fi
# macOS — trust the cert in the system keychain. REQUIRED on macOS: gh is a
# Go binary and Go on darwin reads trust ONLY from the keychain — the
# SSL_CERT_FILE / SSL_CERT_DIR env vars are ignored there.
if ! security verify-cert -c "$BPH_TLS_DIR/bph.crt" -p ssl -s localhost >/dev/null 2>&1; then
  sudo security add-trusted-cert -d -r trustRoot \
    -k /Library/Keychains/System.keychain "$BPH_TLS_DIR/bph.crt"
fi
# Linux (Debian/Ubuntu) — instead of the security commands:
# sudo cp "$BPH_TLS_DIR/bph.crt" /usr/local/share/ca-certificates/bleephub.crt && sudo update-ca-certificates

# 3. Start bleephub on :8443 with TLS (no sudo needed — :8443 doesn't require root).
#    BLEEPHUB_ADMIN_TOKEN is required — there is no default; pick any non-PAT-shaped value.
BLEEPHUB_ADMIN_TOKEN="bleephub-admin-token-00000000000000000000" \
  BPH_TLS_CERT="$BPH_TLS_DIR/bph.crt" BPH_TLS_KEY="$BPH_TLS_DIR/bph.key" \
  ./bleephub-server --addr :8443 &

# 4. Point gh at bleephub via environment. Current gh rejects host:port in
#    `gh auth login --hostname` ("error parsing hostname"), but GH_HOST
#    accepts a port at runtime — pair it with GH_ENTERPRISE_TOKEN and the
#    login step disappears entirely. (GH_ENTERPRISE_TOKEN, not GH_TOKEN:
#    gh reads GH_TOKEN only for github.com; every other host reads
#    GH_ENTERPRISE_TOKEN.)
export GH_HOST=localhost:8443
export GH_ENTERPRISE_TOKEN="bleephub-admin-token-00000000000000000000"

# 5. Use real gh verbs against bleephub
gh repo create demo --public
gh issue create --repo admin/demo --title "first" --body "hi"
gh issue list --repo admin/demo
gh release create v1.0.0 --repo admin/demo --title "v1"
```

To bind the real `:443` instead (lets you use `gh auth login --hostname localhost`
and a persistent `~/.config/gh/hosts.yml` entry, since the no-port hostname is
the only shape `gh auth login` accepts): run step 3 with `--addr :443` under
`sudo`, in its own foreground terminal — `sudo … &` backgrounds the process
before the password prompt, so the server never actually starts and the next
`gh` call fails with `connection refused` on 443.

### Teardown

Removes everything the quick start created, including the keychain trust.
Safe to run in any state — each step tolerates the artifact already being
gone (so a half-cleaned setup, e.g. after a /tmp-era cert purge, still
tears down fully):

```bash
BPH_TLS_DIR="$HOME/.sockerless/bleephub-tls"

# 1. Stop the server.
pkill -f 'bleephub-server --addr' 2>/dev/null

# 2. Remove the keychain trust (macOS). Deleting by SHA-1 fingerprint removes
#    exactly this cert; if the cert file is already gone, list any leftover
#    localhost entries and delete by the hash shown.
if [ -f "$BPH_TLS_DIR/bph.crt" ]; then
  sudo security delete-certificate \
    -Z "$(openssl x509 -in "$BPH_TLS_DIR/bph.crt" -noout -fingerprint -sha1 | cut -d= -f2 | tr -d :)" \
    /Library/Keychains/System.keychain
else
  security find-certificate -a -c localhost -Z /Library/Keychains/System.keychain | grep "SHA-1"
  # → sudo security delete-certificate -Z <HASH> /Library/Keychains/System.keychain
fi
# Linux: sudo rm -f /usr/local/share/ca-certificates/bleephub.crt && sudo update-ca-certificates --fresh

# 3. Remove the cert material and the gh wiring.
rm -rf "$BPH_TLS_DIR"
unset GH_HOST GH_ENTERPRISE_TOKEN
gh auth logout --hostname localhost 2>/dev/null   # only if you used the :443 login flow
```

For an end-to-end smoke that wraps all five steps inside Docker (TLS, CA trust, gh CLI, harness) run [`make bleephub-gh-docker-test`](#integration-tests). For the full walkthrough — supported `gh` commands, endpoints without native verbs, token prefixes, body coercion, troubleshooting — see [`docs/BLEEPHUB_GH_CLI.md`](../docs/BLEEPHUB_GH_CLI.md).

### bleephub UI

The Go binary embeds the React SPA at `/ui/` via `go embed` (build tag `!noui`, on by default). After step 3 above, open:

- `https://localhost:8443/ui/` (or `https://localhost/ui/` on the `:443` variant) — the bleephub dashboard, styled to feel like GitHub without copying it verbatim: a top header bar carries the primary nav and a light/dark toggle (light by default, as on github.com). Pages: **Overview**, **Repos** (GitHub-style repo list → per-repo **Code** / **Issues** / **Pull requests** tabs, plus Commits / Releases / Webhooks / Secrets / Environments), **Workflows** (files + runs, with a per-run detail page showing the job table and the per-job log viewer), **Runners**, **Apps** (GitHub Apps registry + installations + permissions form + PEM viewer), **OAuth** (OAuth Apps registry + tokens), **Metrics**.
- Auth: the UI presents a login form on first visit — paste the `BLEEPHUB_ADMIN_TOKEN` value. The token is verified against `GET /api/v3/user`, kept in browser localStorage, and sent on every UI request. The `/internal/*` dashboard endpoints enforce it server-side (any valid PAT, including the admin token); `/health` stays open for liveness probes. It is a single-token operator surface — for multi-user access control, front bleephub with a reverse proxy.

For UI hacking without rebuilding the Go binary on every change:

```bash
# In one terminal: keep bleephub running from step 3 above.
# In another:
cd ui/packages/bleephub
bun install                         # one-time
bun run dev                         # Vite dev server on :5173 with HMR
# Then open http://localhost:5173/ui/ — Vite proxies the API paths the UI
# uses (see `server.proxy` in vite.config.ts) to localhost:5555; add new
# paths there if you introduce them.
```

To rebuild the embedded copy (production-style) re-run `bun run build` then `make build` in `bleephub/`.

## One-command local dev

For day-to-day hacking, use the convenience script instead of the manual build steps above. From the repo root:

```bash
./scripts/bleephub-local-dev.sh start          # HTTP :5555 + embedded UI
./scripts/bleephub-local-dev.sh start --dev    # HTTP :5555 API + :5173 Vite UI with HMR
./scripts/bleephub-local-dev.sh start --tls    # HTTPS :8443 + embedded UI (self-signed cert)
./scripts/bleephub-local-dev.sh status
./scripts/bleephub-local-dev.sh logs
./scripts/bleephub-local-dev.sh stop
./scripts/bleephub-local-dev.sh clean          # remove local data, logs, PID files
```

The script compiles the current source, starts the server and UI, and prints the endpoints, admin token, data directory, and log paths. Data, git storage, logs, and the PID file live under `.local/bleephub/` in the repo root by default (override with `BLEEPHUB_DATA_DIR` / `BLEEPHUB_GIT_DIR`). The default admin token is the same non-PAT-shaped value used in the quick start above.

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
| Job submission | `/internal/exec/submit` | Simplified JSON job input — sim-control, NOT a GitHub API (lives under `/internal/`, not `/api/v3/`) |

### GitHub REST API (`/api/v3/`) — supported surface

**Repositories.** Create / list / get / update / delete; refs (branches, tags); blobs / trees / commits; smart-HTTP git (`go-git`) for `actions/checkout`.

**Issues, PRs, labels, milestones, comments.** Full CRUD, paginated lists with `Link` headers, state filters, organization issue-type assignment for issues, GraphQL counterparts.

**PR review comments.** Inline / file-line / range / threads. Replies via the dedicated `/replies` endpoint OR `in_reply_to` body field. Reactions on review comments. Review-thread listing + resolve/unresolve have no GitHub REST equivalent (GraphQL-only on real GitHub: `resolveReviewThread`/`unresolveReviewThread`), so bleephub exposes them as sim-control helpers under `/internal/repos/{o}/{r}/pulls/{n}/review-threads[/{tid}/{resolve|unresolve}]`, not the GitHub namespace.

**Reactions.** Eight content values (`+1`, `-1`, `laugh`, `confused`, `heart`, `hooray`, `rocket`, `eyes`). Idempotent POST. Surfaces: issues, issue comments, PR review comments, commit comments, releases. `reactions{url, total_count, +1, ...}` block embedded on parent JSON.

**Releases.** Create / list / get-by-id / get-by-tag / latest / update / delete + `generate-notes` + release reactions. Full HATEOAS URLs (`html_url`, `tarball_url`, `zipball_url`, `assets_url`, `upload_url`). Webhook event fires on create.

**Deployments + Environments.** Full deployment + status + environment surface, including environment protection rules with required reviewers and wait timers. Workflow runs targeting a protected environment park as `waiting` — `GET`/`POST /actions/runs/{run_id}/pending_deployments` lists and approves/rejects them (approval releases the waiting jobs), and `GET /actions/runs/{run_id}/approvals` returns the review history. `deployment` and `deployment_status` webhook events with `attachInstallationBlock`. Environments lazy-created on first deployment to that env.

**Workflow engine (server-side).** Full `on:` trigger semantics: branch/tag/path filter patterns (`*`, `**`, `?`, `+`, `[...]`, ordered `!` negation; path filters diff real git commits), activity types with per-event defaults (`pull_request` fires opened/synchronize/reopened by default — including `synchronize` on pushes to an open PR's head branch), `repository_dispatch` types matching the custom event_type, and `on: schedule` crons fired by a minute-aligned dispatcher (POSIX 5-field parser with names/ranges/steps and the dom/dow OR rule). Reusable workflows (`jobs.<id>.uses`, local `./` and same-server `owner/repo/...@ref`): called jobs join the caller's run as "caller / called", inputs are validated/typed/defaulted against the `workflow_call` declarations, `secrets: inherit` or explicit mapping, outputs map back onto `needs.<caller>`, nesting bounded at 4 levels. A real expression engine evaluates job-level `if:` and `${{ }}` templates (concurrency groups, `with:`, `workflow_call` outputs): GitHub's grammar and loose-equality/coercion semantics, `github` (incl. the full `event` payload), `needs`, `vars`, `inputs`, `matrix` contexts, and `contains`/`startsWith`/`endsWith`/`format`/`join`/`toJSON`/`fromJSON` plus the status functions; invalid expressions fail the job like real GitHub. The runner receives typed `PipelineContextData` (github.event, typed inputs) and the merged secrets/vars with masks.

**Secrets & configuration variables.** Repo / organization / environment scopes for both, with the real sealed-box wire contract (`GET .../secrets/public-key`, libsodium `crypto_box_seal`, `PUT {encrypted_value, key_id}` — plaintext PUTs are rejected), org visibility (`all`/`private`/`selected` + selected-repositories endpoints), name rules (422 on `GITHUB_`-prefixed or invalid), and org→repo→environment precedence merged into runner job messages (every secret value masked in runner logs).

**Checks integration.** Workflow jobs mirror to check runs under a check suite owned by the github-actions app: created at run submission, `in_progress` at runner pickup, completed with the job's conclusion; the suite rolls up at run completion. `workflow_run`, `workflow_job`, `check_run`, and `check_suite` webhook events fire at the same points real GitHub fires them. PR `mergeable_state` reflects the head commit's checks (`blocked` on unmet required status checks from base-branch protection, `unstable` on failing/pending non-required ones), and the merge API rejects with 405 while required checks aren't green.

**Actions API (workflow runs / jobs / steps).** `GET /actions/runs` (status/branch/event filters), `runs/{id}`, `runs/{id}/jobs` (real per-step status/timing from runner timeline records), `runs/{id}/attempts/{n}[/jobs]` (archived attempts), `runs/{id}/logs` (GitHub-layout zip assembled from runner-uploaded timeline log files), `runs/{id}/timing`, `runs/{id}/rerun` + `rerun-failed-jobs` (same run id, run_attempt increments; failed-only rerun carries successful jobs' results over), `jobs/{job_id}/rerun` (archives the prior attempt and reruns the target job plus its dependents), `jobs/{job_id}/logs` (runner-uploaded job log bytes). Public log-download endpoints do not substitute the live console feed for durable uploaded logs: if timeline records have no uploaded log file content, the endpoint returns 404. Reruns preserve the originating workflow-file ID/path, so repositories with multiple workflow files sharing the same `name:` still replay the correct YAML; legacy runs without a unique cached workflow file fail loudly. Workflow files are discovered from the repository's recorded default branch, including repos seeded through Git Data refs where git storage `HEAD` is not set: list/get, `PUT .../workflows/{id}/{enable,disable}` (disabled workflows don't trigger and dispatch 403s), `POST .../dispatches` with input validation/defaults/typing against the `workflow_dispatch` declarations. `POST /repos/{o}/{r}/dispatches` for `repository_dispatch`. Runners: repo + org scope (list/get/delete/registration-token, honest `busy` from running-job association) plus org runner groups (CRUD, membership, repo visibility, undeletable Default); the broker routes jobs only to runners whose labels cover `runs-on` (GitHub-hosted aliases like `ubuntu-latest` run on any connected runner — bleephub has no hosted pool). Cancellation is real: cancel sends `JobCancellation` over the runner's open poll (the runner aborts mid-job), undelivered job messages purge, and `always()`/`cancelled()` jobs still dispatch with the run concluding `cancelled`. Actions referenced with `uses:` resolve from bleephub-hosted repos and serve GitHub-layout tarballs from git storage; absent action repositories or refs fail loudly instead of fetching from github.com.

**Checks API.** `check-runs` create/get/update/list-by-commit/list-by-suite/annotations. `check-suites` get/list-by-commit/preferences. App-owned: writes require `checks:write` on an installation token.

**Webhooks.** Per-repo + org-level (`/orgs/{org}/hooks` CRUD / pings / deliveries / redelivery; repo events on org-owned repos fan out to matching org hooks, and membership changes fire the `organization` event) + app-level. `installation:{id, node_id}` block on every payload when the event flows through an app installation. Full header set: `X-GitHub-Event`, `X-GitHub-Delivery`, `X-GitHub-Hook-ID`, `X-GitHub-Hook-Installation-Target-Type/-Target-ID`, `X-Hub-Signature` (SHA1) + `X-Hub-Signature-256`. Redelivery: `POST /hooks/{id}/deliveries/{delivery_id}/attempts` and `/app/hook/deliveries/{id}/attempts`.

**GitHub Apps.**
- Manifest flow end-to-end: `POST /settings/apps/new` (the browser form-post; 302 with one-time `code`, `state` echoed) → `POST /app-manifests/{code}/conversions` (one-time redemption returning `pem` / `client_secret` / `webhook_secret`).
- App lookup: `GET /apps/{slug}` (anon), `GET /app` (JWT).
- Installations: `GET /app/installations[/{id}]`, `GET/DELETE /app/installations/{id}`, suspend / unsuspend (suspension kills every API request made with the installation's tokens, 403), `GET /repos/{o}/{r}/installation` (repo-aware: 404 for unknown repos or repos outside a `selected` installation), `GET /orgs/{org}/installation[s]`, `GET /users/{username}/installation`, `GET /user/installations` (scoped to the caller's account + active org memberships), `GET /installation/repositories`, `DELETE /installation/token`, repo-selection management (`PUT/DELETE /user/installations/{id}/repositories/{repo_id}`).
- Installation tokens: 1h TTL, permission downscoping validated against the installation grant (escalation, ungranted scopes, and invalid level strings are 422), `repository_ids`/`repositories` scoping validated against the installation's accessible repos (422 on unknown/inaccessible), `repository_selection` reflects the token's effective scope.
- App webhook: `GET/PATCH /app/hook/config`, `GET /app/hook/deliveries[/{id}][/attempts]`.
- Installation events: `installation`, `installation_repositories` fire on store transitions.
- JWT verification: RS256 only (alg `none`/HMAC rejected), exp at most 10 minutes ahead of the server clock (+60s drift), future-iat rejection — backdated iat (ghinstallation-style) stays valid.

**OAuth Apps.** Distinct entity from GitHub Apps. Created/listed via the sim-control surface `POST/GET /internal/oauth-apps` (GitHub has no REST API to create OAuth Apps, so this is NOT under `/api/v3/`). OAuth web flow (`/login/oauth/authorize`) + device flow (`/login/device/code`). Token-management family on the real `/api/v3/applications/{client_id}/{token,grant}` (check / reset / revoke / scope).

**Token prefixes.** Match real GH exactly: `ghp_` (PAT), `gho_` (OAuth App user-to-server), `ghu_` (GitHub App user-to-server), `ghs_` (server-to-server installation), `ghr_` (refresh). Middleware distinguishes all five.

**Permission enforcement.** `requirePerm(scope, level)` decorator gates write-class endpoints. PAT bypass (matches real GH PATs being full-scope). `ghs_` tokens checked against `InstallationToken.Permissions`; `ghu_` against the App's installation perms; `gho_` mapped from classic OAuth scopes.

**Actions OIDC.** `GET /token` issues an RS256-signed JWT with the canonical claim set (sub, aud, repository, repository_owner, ref, run_id, run_number, sha, actor, environment, jti, exp). `GET /.well-known/jwks` + `/.well-known/openid-configuration` for cloud-IdP trust verification.

**Users API.** Public users, my-user, keys CRUD, gpg_keys compatibility surface, emails, followers / following compatibility surface, follow / unfollow.

**Meta.** `GET /meta` in GHES shape — bleephub presents as GHES (`installed_version: "3.21.0"`). `gh`'s feature detection requires the member to resolve the host version; without it `gh issue list --label`, `gh pr status`, and `gh workflow run` fail.

**Pages.** Site CRUD, build records, deployments, and DNS-health checks are persisted. Manual build requests require a configured Pages site and record the actual latest default-branch commit SHA.

**Branch protection.** PUT/GET/DELETE per-branch protection rules; JSON pass-through.

**Orgs.** GHES admin create + sim-control create; `GET /organizations` (global list with `since` cursor); organization-full profile (company / blog / location / twitter / billing email / `default_repository_permission` / `members_can_create_repositories` / `web_commit_signoff_required`) readable + PATCHable. Memberships with real invitation semantics: `PUT /orgs/{org}/memberships/{username}` invites (state `pending`), the invitee accepts via `PATCH /user/memberships/orgs/{org}` (`GET /user/memberships/orgs[/{org}]` lists/inspects); member checks (`GET/DELETE /orgs/{org}/members/{username}`), public members (list / check / publicize / conceal — self-only, like real GitHub). Teams: CRUD + hierarchy (`parent_team_id`, child-team listing, cycle rejection, delete re-parents children), `notification_setting`, member roles (`member`/`maintainer`) with team-membership state mirroring the org membership, team repos (list with `permissions` + `role_name`, check incl. the `vnd.github.v3.repository+json` media type, add/remove), rename re-keys the slug. Audit log shape-only endpoint, IdP-group sync compatibility surface.

**Marketplace.** Listing plans + accounts compatibility surface.

**GraphQL.** Repository / User / Organization queries + the IssueOrPullRequest union + repositoryOwner polymorphic root + repository.issues/pullRequests connections + `search(type: ISSUE)` + check-run/check-suite types + matching enums (RepositoryPrivacy, RepositoryAffiliation, IssueOrderField, OrderDirection, IssueState). Issue nodes expose REST-backed project items, assigned organization issue types (`Issue.issueType`), organization issue-field values (`Issue.issueFieldValues`), and sub-issue relationships (`parent`, ordered `subIssues`, and `subIssuesSummary`). Mutations cover the GraphQL verbs `gh` sends: createIssue / addComment / closeIssue / reopenIssue, createPullRequest / closePullRequest / reopenPullRequest / mergePullRequest / addPullRequestReview, createRepository / deleteRepository, and Projects v2 (createProjectV2, addProjectV2ItemById, createProjectV2Field, updateProjectV2ItemFieldValue) with Issue.projectItems backed by the store.

### Persistence

Bleephub stores its own metadata state in SQLite. `BLEEPHUB_PERSIST=true` enables the write-through database, and the DB file is `<BLEEPHUB_DATA_DIR>/bleephub.db` (default `./bleephub.db`). SQLite open/schema failures fail startup loudly; there is no silent in-memory fallback once persistence is requested.

`persistence_test.go` always exercises the SQLite round-trip. The obsolete `BLEEPHUB_DATABASE_URL` PostgreSQL path fails loudly so operators do not accidentally deploy a state backend outside the supported service model.

The full metadata surface is persisted: users, tokens, apps (incl. credentials + webhook config), OAuth apps, installations (incl. selected repos) + installation / user-to-server / refresh tokens, repos, orgs, teams, memberships, issues, labels, milestones, comments, pull requests + reviews + review comments, hooks (incl. secrets) + org hooks + deliveries, app hook deliveries, repo secrets, check suites/runs/preferences, workflow files, releases, deployments + statuses + environments (incl. reviewers/wait timer), reactions, Projects v2, user SSH/GPG keys, Pages, branch protection, the audit log, and marketplace plans. ID numbering is re-derived on load so it resumes where it left off.

Intentionally NOT persisted: runner/workflow runtime state (workflows, sessions, agents — a restart abandons in-flight runs) and the Actions OIDC signing key, which rotates on restart; consumers must re-fetch the JWKS, exactly as against real GitHub key rotation.

Git repository storage (go-git) is selected by its own env vars:

- default — in-memory (lost on restart);
- `BLEEPHUB_GIT_DIR=<dir>` — bare repos on the local filesystem;
- `BLEEPHUB_S3_BUCKET` (+ optional `BLEEPHUB_S3_ENDPOINT`, `BLEEPHUB_S3_PREFIX`) — repos in S3-compatible object storage (takes priority over `BLEEPHUB_GIT_DIR`).

Database persistence **requires** durable git storage (`BLEEPHUB_GIT_DIR` or `BLEEPHUB_S3_BUCKET`): reloading repo metadata against in-memory git storage would resurrect every repo empty, so that combination is a startup error — never a silent degraded mode.

The S3 filesystem test suite drives this path through a real `simulator-aws` S3 endpoint and `aws-sdk-go-v2`; it does not use a local fake S3 server. The tests cover object reads/writes/open modes, paginated listings, and repository-prefix rename/delete through the same list/copy/delete APIs that S3-backed git storage uses.

Actions byte storage is selected separately from git storage:

- default — in-memory bytes, or local files under `BLEEPHUB_DATA_DIR` for local development;
- `BLEEPHUB_OBJECT_S3_BUCKET` (+ optional `BLEEPHUB_OBJECT_S3_ENDPOINT`, `BLEEPHUB_OBJECT_S3_PREFIX`) — Actions artifacts, dependency caches, and runner-uploaded log files in S3-compatible object storage. If `BLEEPHUB_OBJECT_S3_BUCKET` is set and the bucket cannot be reached with `HeadBucket`, startup fails loudly.

The object-byte tests also drive a real `simulator-aws` S3 endpoint: artifact upload, cache upload, runner log upload, and public job-log download assert the expected S3 objects are written and read back, so these paths do not rely on fake S3 or memory-only assertions.

### `gh` CLI compatibility

bleephub accepts what real GitHub accepts — including the string-coerced booleans / integers `gh api -f` sends (real GH's Rails layer coerces them; bleephub's `flexBool`/`flexInt`/`flexInt64`/`flexIntSlice` types decode either form). `gh` CLI works against bleephub directly:

```bash
echo "$TOKEN" | gh auth login --hostname localhost --with-token
gh repo create my-repo --public
gh issue create --repo admin/my-repo --title "test"
gh issue view / list / comment / close / reopen
gh repo view admin/my-repo
gh repo list admin
gh release create v1.0.0 --repo admin/my-repo
gh pr create / view / list / merge / review / comment (in a git working dir)
gh run list / view / cancel / rerun (when workflow runs exist)
gh workflow run / list / view
```

The full command ↔ endpoint table lives in [`docs/BLEEPHUB_GH_CLI.md` § Supported commands](../docs/BLEEPHUB_GH_CLI.md#supported-commands).

Verified end-to-end by [`make bleephub-gh-docker-test`](#integration-tests), which builds a Docker image bundling bleephub + the official `gh` CLI + a self-signed TLS cert and runs the harness against the live bleephub binary inside the container.

## What it does not implement (deferred)

- Runner auto-update (`AgentRefreshMessage`).
- V2 broker flow (uses legacy V1 pipelines paths).
- Failed-run shells exist for TRIGGERED workflows that can't start (conclusion `startup_failure`, no jobs); explicit dispatches still 422 with the parse error (more useful to the caller).
- Full Projects v2 (boards / views / iteration fields; bleephub implements the createProjectV2 / addProjectV2ItemById / createProjectV2Field / updateProjectV2ItemFieldValue mutations and the `Issue.projectItems` connection).
- SAML SSO + SCIM provisioning.
- Org invitation entities (`/orgs/{org}/invitations`, `failed_invitations`, team invitations) — bleephub has no email model; the invite flow is modeled as `pending` memberships (`PUT /orgs/{org}/memberships/{username}` → `PATCH /user/memberships/orgs/{org}`), which is what the membership APIs expose.
- Org people-management extras: `outside_collaborators`, `blocks`, `security-managers`, member codespaces/copilot endpoints.
- Legacy numeric-id team routes (`/teams/{team_id}/…`) — deprecated upstream; the `/orgs/{org}/teams/{team_slug}/…` family is the supported path.
- Webhook `config` subresources (`/repos/{o}/{r}/hooks/{id}/config`, `/orgs/{org}/hooks/{id}/config`) — config rides the hook CRUD bodies, which is what gh / terraform / go-github use.
- `GET /app/installation-requests` and the marketplace `stubbed` endpoints.
- Org `plan` member / billing endpoints (bleephub has no billing model).
- Per-installation audit log content (shape-only empty endpoint).
- Marketplace billing.
- gh CLI commands that require deep workflow-run state bleephub doesn't synthesise (`gh run watch` long-poll, log tail).
- `on: schedule` crons fire from real server time (minute-aligned); there is no time-warp hook for tests beyond calling the dispatcher directly.

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
4. A job is submitted via `POST /internal/exec/submit` (simplified JSON; sim-control, not a GitHub API path).
5. bleephub converts to the internal job-message format and delivers it.
6. Runner creates a Docker container through `DOCKER_HOST` (pointing at Sockerless).
7. Runner execs each `run:` step inside the container via `docker exec`.
8. Runner reports step status; bleephub marks the job completed.

For ad-hoc REST / GraphQL workflows (probot, octokit, `gh`):
- Point `GH_HOST=localhost` (or set the host in `gh auth login`).
- Use a token recognised by bleephub's middleware (the `BLEEPHUB_ADMIN_TOKEN` value works everywhere; mint your own via the OAuth flow for stricter testing — see the token table in [`docs/BLEEPHUB_GH_CLI.md`](../docs/BLEEPHUB_GH_CLI.md#tokens-at-a-glance)).

## Usage

```bash
make build                                            # → ./bleephub-server
BLEEPHUB_ADMIN_TOKEN=<token> ./bleephub-server --addr :80 --log-level info
# or: make run   (builds + runs on :5555; still requires BLEEPHUB_ADMIN_TOKEN in the env)
```

Flags:
- `--addr` — listen address (default `:5555`). Runner strips non-standard ports from URLs, so use port 80/443 for integration tests with the runner.
- `--log-level` — `debug` | `info` | `warn` | `error` (default `info`).

Env vars:
- `BLEEPHUB_ADMIN_TOKEN=<token>` — **required.** The seeded admin token. There is no default (a default would be a guessable credential, and the historical `ghp_…` value tripped secret scanners); the binary fails loudly at startup if unset. Set a non-PAT-shaped value.
- `BLEEPHUB_PERSIST=true` — enable SQLite persistence (off by default; see [Persistence](#persistence)).
- `BLEEPHUB_DATA_DIR=<dir>` — directory for the SQLite DB (`bleephub.db`) + artifact store (default `.`).
- `BLEEPHUB_GIT_DIR=<dir>` — store git repos on the local filesystem (default: in-memory).
- `BLEEPHUB_S3_BUCKET` / `BLEEPHUB_S3_ENDPOINT` / `BLEEPHUB_S3_PREFIX` — store git repos in S3-compatible object storage (bucket set ⇒ S3 wins over `BLEEPHUB_GIT_DIR`).
- `BPH_TLS_CERT` + `BPH_TLS_KEY` — serve over TLS.
- `BLEEPHUB_MAX_WORKFLOWS=N` — concurrency cap (default 10).
- `OTEL_EXPORTER_OTLP_ENDPOINT` — when set, emits traces + metrics + logs via OTLP (off by default; preserves the components-decoupled invariant).

## Integration tests

```bash
# Go unit tests
make bleephub/test                  # go test ./bleephub/...

# Official actions/runner end-to-end (Docker; self-contained)
make bleephub-runner-docker-test

# Real gh CLI inside Docker (real bleephub + real gh binary + self-signed TLS)
make bleephub-gh-docker-test
```

`bleephub-runner-docker-test` builds `bleephub/Dockerfile` (bleephub + the
official `actions/runner` binary) and runs
`bleephub/test/run-integration.sh`: the runner registers against bleephub,
polls the broker, executes HOST-MODE jobs (`jobContainer` null — real
GitHub's shape for jobs without `container:`), uploads timeline records and
logs, and completes. Runs in CI as the `sim (bleephub actions/runner)` job.
Container-mode jobs against the cloud backends are gated on the
bind-mount→EFS translation tracked in
[`docs/GITHUB_RUNNER.md`](../docs/GITHUB_RUNNER.md).

The gh harness builds `bleephub/Dockerfile.gh-test` and runs `bleephub/test/run-gh-test.sh`. It exercises:
- `gh auth login` against bleephub as a GHES host
- Native `gh repo create / view / list`, `gh issue create / view / list` (REST + GraphQL paths)
- `gh secret set` (real sealed-box encryption), `gh variable set/get/list/delete`, `gh workflow run / enable / disable`, check-runs on pushed commits
- The parity probes for endpoints with no native `gh` verb (apps/{slug}, /applications/{cid}/token, suspend, OAuth Apps mgmt)

Runs in CI as the `sim (bleephub gh CLI)` job (must be green to merge).

### OpenAPI fidelity gates (hermetic)

Two unit-test gates validate bleephub against the vendored GitHub OpenAPI description (`testdata/github-openapi.json.gz`, refreshed via `scripts/update-github-openapi.sh`):

- **Route definitions** (`gh_api_definition_test.go`) — every registered `/api/v3` route must exist in the description; paths can't be invented under GitHub's namespace.
- **Response-shape ratchet** (`openapi_shape_validator_test.go`) — an observer on the shared test server validates every 2xx `/api/v3` JSON response member-by-member against the documented response schema. Violations are gated against [`openapi-violation-allowlist.txt`](openapi-violation-allowlist.txt): each entry is either a real-but-undescribed member (GHES-only surface, with a citation — currently only `/meta`'s `installed_version`) or a filed BUG on its way to being fixed. The list only shrinks; new violations fail the suite.

## Source layout (~180 Go files)

| Group | Files | Purpose |
|---|---|---|
| Core protocol | `server.go`, `auth.go`, `agents.go`, `broker.go`, `run_service.go`, `timeline.go` | Runner registration, job delivery, lifecycle |
| Jobs & workflows | `jobs.go`, `workflow.go`, `workflows.go`, `workflows_msg.go`, `matrix.go`, `outputs.go`, `secrets.go`, `expressions.go`, `actions.go`, `artifacts.go` | Multi-job, matrix, secrets, expressions, artifacts |
| GitHub REST core | `gh_rest.go`, `gh_repos_*.go`, `gh_orgs_*.go`, `gh_issues_*.go`, `gh_pulls_*.go`, `gh_teams_rest.go`, `gh_labels_rest.go`, `gh_members_rest.go` | Repos, orgs, issues, PRs, teams, labels, milestones |
| GitHub Apps + OAuth | `gh_apps_*.go`, `gh_oauth.go`, `gh_app_hooks_rest.go`, `gh_apps_user_tokens.go`, `gh_apps_oauth_mgmt.go`, `gh_apps_perms.go` | JWT, installations, OAuth Apps, ghs_/ghu_/gho_/ghr_, permission enforcement |
| Reactions + Releases + Deployments | `gh_reactions.go`, `gh_releases.go`, `gh_deployments.go`, `gh_pr_comments.go`, `gh_pr_threads.go` | Reactions, releases, deployments + environments + approvals, PR review comments/threads |
| Actions extras | `gh_actions_rest.go`, `gh_actions_extras.go`, `gh_workflows_rest.go` | Runs/jobs/steps, repository_dispatch, logs zip, timing |
| Checks API | `gh_checks_rest.go`, `gh_checks_store.go` | check-runs + check-suites |
| Misc long-tail | `gh_misc_endpoints.go` | Users keys/follow, Actions OIDC + JWKS, Pages, Branch protection, Marketplace |
| GraphQL | `gh_graphql.go`, `gh_*_graphql.go`, `gh_request_decode.go` | Schema + flex decoders |
| Webhooks | `webhooks.go`, `webhooks_store.go`, `webhooks_payloads.go`, `gh_hooks_rest.go` | HMAC-SHA256/SHA1 delivery with retry |
| Git | `git_http.go`, `git_storage.go`, `s3fs.go` | Smart HTTP protocol (go-git); in-memory / on-disk / S3 repo storage |
| Persistence | `persistence.go` | SQLite write-through layer |
| Infrastructure | `store.go`, `store_*.go`, `rbac.go`, `metrics.go`, `otel.go`, `handle_mgmt.go`, `ui_embed.go` | State, RBAC, metrics, OTel, dashboard |

## See also

- [docs/BLEEPHUB_GH_CLI.md](../docs/BLEEPHUB_GH_CLI.md) — operator-facing `gh` setup walkthrough.
- [specs/BLEEPHUB_GITHUB_API_PARITY.md](../specs/BLEEPHUB_GITHUB_API_PARITY.md) — per-endpoint parity audit + acceptance criteria.
- [ARCHITECTURE.md](../ARCHITECTURE.md), [docs/GITHUB_RUNNER.md](../docs/GITHUB_RUNNER.md).

## Prior art

[ChristopherHX/runner.server](https://github.com/ChristopherHX/runner.server) (C#, 25 controllers) proved this approach works. bleephub is a from-scratch Go implementation informed by studying the runner source + runner.server's protocol handling, but shares no code with either.
