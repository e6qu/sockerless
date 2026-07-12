# bleephub

Bleephub is a self-contained Go reimplementation of GitHub's server-side surface — enough for the official `actions/runner`, the `gh` command-line interface, Octokit, and Probot to talk to a local process exactly as they would talk to github.com or GitHub Enterprise Server.

The runner-server protocol uses GitHub Enterprise Server-style `/_apis/` paths over five internal services. The Representational State Transfer application programming interface and GraphQL application programming interface use GitHub Enterprise Server-style `/api/v3/` and `/api/graphql`. Both are served from the same binary on the same port.

## Reference adaptors

Bleephub is paired with the external GitHub-compatible tools that drive it. Anything these tools do against `github.com` or a GitHub Enterprise Server instance must work against Bleephub.

| Adaptor | Min version | What it proves |
|---|---|---|
| [`gh` command-line interface](https://cli.github.com/manual/) | 2.50+ | End-to-end command-line interface verbs against `--hostname localhost` — repositories, issues, pull requests, releases, run / view / list. See [`docs/BLEEPHUB_GH_CLI.md`](../docs/BLEEPHUB_GH_CLI.md). |
| [`go-github`](https://github.com/google/go-github) | v88 | Typed Representational State Transfer software development kit coverage against the GitHub Enterprise Server-style application programming interface, including Git Data seeded repositories and Actions workflow dispatch / run / job reads. |
| [`actions/runner`](https://github.com/actions/runner) (official binary) | v2.319+ | The runner-server `/_apis/` protocol — token, agent registration, broker long-poll, run service, timeline/logs upload. |
| [Smart-HTTP git](https://git-scm.com/docs/http-protocol) (`go-git`) | git 2.40+ | `git clone` / `git push` over `https://localhost/{owner}/{repo}.git`. Used by `actions/checkout`. |
| [GitHub Representational State Transfer application programming interface spec](https://docs.github.com/en/rest) | 2022-11-28 | The authoritative reference for paths, request bodies, response envelopes, and `Link`-header pagination. |
| [GitHub GraphQL schema](https://docs.github.com/en/graphql/reference) | 2022-11-28 | The `IssueOrPullRequest` union, connection shapes, enum values. |

The audit artifact mapping Bleephub's coverage to GitHub-real shapes (per-route and per-field) lives at [`specs/BLEEPHUB_GITHUB_API_PARITY.md`](../specs/BLEEPHUB_GITHUB_API_PARITY.md).

## Quick start — Bleephub + `gh` command-line interface in 5 steps

`gh` is HTTPS-only against any non-`github.com` host, and it identifies the target by **hostname** (no base URL flag). The `--hostname` argument on `gh auth login` is what wires it up; once that and `GH_HOST` are set, every `gh` command builds `https://<host>/api/v3/...` automatically and bleephub serves it.

```bash
# 1. Build (user interface first so the Go binary embeds it; skip the user
#    interface step if you only need the application programming interface.
#    make build falls back to a no-user-interface binary when
#    ui/packages/bleephub/dist/ is missing)
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
#    BLEEPHUB_ADMIN_TOKEN is required — there is no default; pick any
#    non-personal-access-token-shaped value.
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

For an end-to-end smoke that wraps all five steps inside Docker (Transport Layer Security, certificate authority trust, gh command-line interface, harness) run [`make bleephub-gh-docker-test`](#integration-tests). For the full walkthrough — supported `gh` commands, endpoints without native verbs, token prefixes, body coercion, troubleshooting — see [`docs/BLEEPHUB_GH_CLI.md`](../docs/BLEEPHUB_GH_CLI.md).

### Bleephub User Interface

The Go binary embeds the React single-page application at `/ui/` via `go embed` (build tag `!noui`, on by default). After step 3 above, open:

- `https://localhost:8443/ui/` (or `https://localhost/ui/` on the `:443` variant) — the Bleephub dashboard, styled to feel like GitHub without copying it verbatim: a top header bar carries the primary navigation and a light/dark toggle (light by default, as on github.com). Pages: **Overview**, **Repos** (GitHub-style repo list → per-repo **Code** / **Issues** / **Pull requests** tabs, plus Commits / Releases / Webhooks / Secrets / Environments), **Workflows** (files + runs, with a per-run detail page showing the job table and the per-job log viewer), **Runners**, **Apps** (GitHub Apps registry + installations + permissions form + Privacy Enhanced Mail key viewer), **OAuth** (OAuth Apps registry + tokens), **Metrics**.
- Auth: the user interface presents a login form on first visit — paste a GitHub-compatible token accepted by this Bleephub instance. The token is verified against `GET /api/v3/user`, kept in browser localStorage, and sent on every user-interface request. The `/internal/*` operator endpoints still require a token accepted by the operator surface, including the admin token; `/health` stays open for liveness probes.

For user-interface hacking without rebuilding the Go binary on every change:

```bash
# In one terminal: keep Bleephub running from step 3 above.
# In another:
cd ui/packages/bleephub
bun install                         # one-time
bun run dev                         # Vite dev server on :5173 with HMR
# Then open http://localhost:5173/ui/ — Vite proxies the application programming interface paths the user interface
# uses (see `server.proxy` in vite.config.ts) to localhost:5555; add new
# paths there if you introduce them.
```

To rebuild the embedded copy (production-style) re-run `bun run build` then `make build` in `bleephub/`.

## One-command local dev

For day-to-day hacking, use the convenience script instead of the manual build steps above. From the repo root:

```bash
./scripts/bleephub-local-dev.sh start          # Hypertext Transfer Protocol :5555 + embedded user interface
./scripts/bleephub-local-dev.sh start --dev    # Hypertext Transfer Protocol :5555 application programming interface + :5173 Vite user interface with hot module replacement
./scripts/bleephub-local-dev.sh start --tls    # Hypertext Transfer Protocol Secure :8443 + embedded user interface (self-signed cert)
./scripts/bleephub-local-dev.sh status
./scripts/bleephub-local-dev.sh logs
./scripts/bleephub-local-dev.sh stop
./scripts/bleephub-local-dev.sh clean          # remove local data, logs, PID files
```

The script compiles the current source, starts the server and user interface, and prints the endpoints, admin token, data directory, and log paths. Data, git storage, logs, and the process identifier file live under `.local/bleephub/` in the repo root by default (override with `BLEEPHUB_DATA_DIR` / `BLEEPHUB_GIT_DIR`). The default admin token is the same non-personal-access-token-shaped value used in the quick start above.

## What it implements

### Runner protocol (`/_apis/`)

| Service | Path prefix | Purpose |
|---|---|---|
| Token service | `/_apis/v1/auth/` | JSON Web Token exchange (`alg: none`, unsigned) |
| Connection data | `/_apis/connectionData` | Service discovery via globally unique identifiers |
| Agent service | `/_apis/v1/Agent/`, `/_apis/v1/AgentPools` | Runner registration, pools, credentials |
| Broker | `/_apis/v1/AgentSession/`, `/_apis/v1/Message/` | Session management, 30 second message long-poll |
| Run service | `/_apis/v1/AgentRequest/`, `/_apis/v1/FinishJob/` | Job acquire/renew/complete |
| Timeline + logs | `/_apis/v1/Timeline/`, `/_apis/v1/Logfiles/` | Step status tracking, log upload |
| Job submission | `/internal/exec/submit` | Operator-only simplified JavaScript Object Notation job input; not part of the GitHub-compatible application programming interface surface (lives under `/internal/`, not `/api/v3/`) |

### GitHub Representational State Transfer Application Programming Interface (`/api/v3/`) — supported surface

**Repositories.** Create / list / get / update / delete; refs (branches, tags); blobs / trees / commits; smart-HTTP git (`go-git`) for `actions/checkout`.

**Issues, pull requests, labels, milestones, comments.** Full create/read/update/delete, paginated lists with `Link` headers, state filters, organization issue-type assignment for issues, GraphQL counterparts.

**Pull request review comments.** Inline / file-line / range / threads. Replies via the dedicated `/replies` endpoint OR `in_reply_to` body field. Reactions on review comments. Review-thread listing and resolve/unresolve use the GitHub GraphQL surface (`PullRequest.reviewThreads`, `resolveReviewThread`, and `unresolveReviewThread`) because real GitHub has no REST equivalent.

**Reactions.** Eight content values (`+1`, `-1`, `laugh`, `confused`, `heart`, `hooray`, `rocket`, `eyes`). Idempotent POST. Surfaces: issues, issue comments, pull request review comments, commit comments, releases. `reactions{url, total_count, +1, ...}` block embedded on parent JavaScript Object Notation.

**Releases.** Create / list / get-by-id / get-by-tag / latest / update / delete + `generate-notes` + release reactions. Full HATEOAS URLs (`html_url`, `tarball_url`, `zipball_url`, `assets_url`, `upload_url`). Webhook event fires on create.

**Packages and GitHub Container Registry.** GitHub REST package management covers user, organization, and repository package listing, version/file reads, delete, and restore where GitHub exposes restore. Container package publication uses the OCI/Docker Registry HTTP API v2 data plane under `/v2/`: blob uploads verify `sha256:` digests, manifest pushes create `container` package versions, and manifest/blob reads serve the stored registry bytes with `Docker-Distribution-Api-Version` and `Docker-Content-Digest` headers. Persisted package files and container-registry blobs are stored in the configured object store, while SQLite stores the package metadata and object keys. The Packages user interface is a management/read surface and does not call operator-only `/internal/packages` seed endpoints.

**Code scanning and CodeQL.** Code scanning alerts, SARIF uploads, default setup, Copilot Autofix, CodeQL databases, and CodeQL variant analyses are GitHub REST-backed surfaces. CodeQL database archives and variant-analysis query-pack tarballs are stored in the configured object store while SQLite keeps metadata and object keys. CodeQL database archive downloads and variant-analysis query-pack downloads are advertised through public non-internal storage URLs; successful `/api/v3` JSON responses are guarded in tests so they cannot leak operator-only `/internal/` routes.

**Artifact attestations.** Artifact attestation Sigstore bundles are uploaded, listed, and deleted through GitHub-compatible repository, organization, and user REST endpoints. Bundle bytes live in the configured object store while SQLite stores repository linkage, subject digests, predicate type, initiator, timestamps, and object keys.

**Deployments + Environments.** Full deployment + status + environment surface, including environment protection rules with required reviewers and wait timers. Workflow runs targeting a protected environment park as `waiting` — `GET`/`POST /actions/runs/{run_id}/pending_deployments` lists and approves/rejects them (approval releases the waiting jobs), and `GET /actions/runs/{run_id}/approvals` returns the review history. `deployment` and `deployment_status` webhook events with `attachInstallationBlock`. Environments lazy-created on first deployment to that env.

**Workflow engine (server-side).** Full `on:` trigger semantics: branch/tag/path filter patterns (`*`, `**`, `?`, `+`, `[...]`, ordered `!` negation; path filters diff real git commits), activity types with per-event defaults (`pull_request` fires opened/synchronize/reopened by default — including `synchronize` on pushes to an open pull request's head branch), `repository_dispatch` types matching the custom event_type, and `on: schedule` crons fired by a minute-aligned dispatcher (POSIX 5-field parser with names/ranges/steps and the day-of-month/day-of-week OR rule). Reusable workflows (`jobs.<id>.uses`, local `./` and same-server `owner/repo/...@ref`): called jobs join the caller's run as "caller / called", inputs are validated/typed/defaulted against the `workflow_call` declarations, `secrets: inherit` or explicit mapping, outputs map back onto `needs.<caller>`, nesting bounded at 4 levels. A real expression engine evaluates job-level `if:` and `${{ }}` templates (concurrency groups, `with:`, `workflow_call` outputs): GitHub's grammar and loose-equality/coercion semantics, `github` (including the full `event` payload), `needs`, `vars`, `inputs`, `matrix` contexts, and `contains`/`startsWith`/`endsWith`/`format`/`join`/`toJSON`/`fromJSON` plus the status functions; invalid expressions fail the job like real GitHub. The runner receives typed `PipelineContextData` (github.event, typed inputs) and the merged secrets/vars with masks.

**Secrets & configuration variables.** Repo / organization / environment scopes for both, with the real sealed-box wire contract (`GET .../secrets/public-key`, libsodium `crypto_box_seal`, `PUT {encrypted_value, key_id}` — plaintext PUTs are rejected), org visibility (`all`/`private`/`selected` + selected-repositories endpoints), name rules (422 on `GITHUB_`-prefixed or invalid), and org→repo→environment precedence merged into runner job messages (every secret value masked in runner logs).

**Checks integration.** Workflow jobs mirror to check runs under a check suite owned by the github-actions app: created at run submission, `in_progress` at runner pickup, completed with the job's conclusion; the suite rolls up at run completion. `workflow_run`, `workflow_job`, `check_run`, and `check_suite` webhook events fire at the same points real GitHub fires them. Pull request `mergeable_state` reflects the head commit's checks (`blocked` on unmet required status checks from base-branch protection, `unstable` on failing/pending non-required ones), and the merge application programming interface rejects with 405 while required checks are not green.

**Actions application programming interface (workflow runs / jobs / steps).** `GET /actions/runs` (status/branch/event filters), `runs/{id}`, `runs/{id}/jobs` (real per-step status/timing from runner timeline records), `runs/{id}/attempts/{n}[/jobs]` (archived attempts), `runs/{id}/logs` (GitHub-layout zip assembled from runner-uploaded timeline log files), `runs/{id}/timing`, `runs/{id}/rerun` + `rerun-failed-jobs` (same run id, run_attempt increments; failed-only rerun carries successful jobs' results over), `jobs/{job_id}/rerun` (archives the prior attempt and reruns the target job plus its dependents), `jobs/{job_id}/logs` (runner-uploaded job log bytes). Public log-download endpoints do not substitute the live console feed for durable uploaded logs: if timeline records have no uploaded log file content, the endpoint returns 404. Reruns preserve the originating workflow-file identifier/path, so repositories with multiple workflow files sharing the same `name:` still replay the correct workflow file; legacy runs without a unique cached workflow file fail loudly. Workflow files are discovered from the repository's recorded default branch, including repos seeded through Git Data refs where git storage `HEAD` is not set: list/get, `PUT .../workflows/{id}/{enable,disable}` (disabled workflows don't trigger and dispatch 403s), `POST .../dispatches` with input validation/defaults/typing against the `workflow_dispatch` declarations. `POST /repos/{o}/{r}/dispatches` for `repository_dispatch`. Runners: repository + organization scope (list/get/delete/registration-token, honest `busy` from running-job association) plus organization runner groups (create, read, update, delete, membership, repository visibility, undeletable Default); the broker routes jobs only to runners whose labels cover `runs-on` (GitHub-hosted aliases like `ubuntu-latest` run on any connected runner — Bleephub has no hosted pool). Cancellation is real: cancel sends `JobCancellation` over the runner's open poll (the runner aborts mid-job), undelivered job messages purge, and `always()`/`cancelled()` jobs still dispatch with the run concluding `cancelled`. Actions referenced with `uses:` resolve from Bleephub-hosted repositories and serve GitHub-layout tarballs from git storage; absent action repositories or refs fail loudly instead of fetching from github.com.

**Checks application programming interface.** `check-runs` create/get/update/list-by-commit/list-by-suite/annotations. `check-suites` get/list-by-commit/preferences. App-owned: writes require `checks:write` on an installation token.

**Webhooks.** Per-repo + org-level (`/orgs/{org}/hooks` CRUD / pings / deliveries / redelivery; repo events on org-owned repos fan out to matching org hooks, and membership changes fire the `organization` event) + app-level. `installation:{id, node_id}` block on every payload when the event flows through an app installation. Full header set: `X-GitHub-Event`, `X-GitHub-Delivery`, `X-GitHub-Hook-ID`, `X-GitHub-Hook-Installation-Target-Type/-Target-ID`, `X-Hub-Signature` (SHA1) + `X-Hub-Signature-256`. Redelivery: `POST /hooks/{id}/deliveries/{delivery_id}/attempts` and `/app/hook/deliveries/{id}/attempts`.

**GitHub Apps.**
- Manifest flow end-to-end: `POST /settings/apps/new` (the browser form-post; 302 with one-time `code`, `state` echoed) → `POST /app-manifests/{code}/conversions` (one-time redemption returning `pem` / `client_secret` / `webhook_secret`).
- Browser installation flow: `POST /apps/{slug}/installations/new` (also accepted under `/settings/apps/{slug}/installations/new`) installs the app for the signed-in user's account or an organization the user owns, using the app's registered default permissions/events and the selected repository mode from the form. Duplicate installations return validation errors, selected-repository installs validate repository identifiers against the target account, and installation webhooks fire from the same store transition as application programming interface changes.
- App lookup: `GET /apps/{slug}` (anonymous), `GET /app` (JSON Web Token).
- Installations: `GET /app/installations[/{id}]`, `GET/DELETE /app/installations/{id}`, suspend / unsuspend (suspension kills every application programming interface request made with the installation's tokens, 403), `GET /repos/{o}/{r}/installation` (repository-aware: 404 for unknown repositories or repositories outside a `selected` installation), `GET /orgs/{org}/installation[s]`, `GET /users/{username}/installation`, `GET /user/installations` (scoped to the caller's account and active organization memberships), `GET /installation/repositories`, `DELETE /installation/token`, repository-selection management (`PUT/DELETE /user/installations/{id}/repositories/{repo_id}`).
- Installation tokens: 1h TTL, permission downscoping validated against the installation grant (escalation, ungranted scopes, and invalid level strings are 422), `repository_ids`/`repositories` scoping validated against the installation's accessible repos (422 on unknown/inaccessible), `repository_selection` reflects the token's effective scope.
- App webhook: `GET/PATCH /app/hook/config`, `GET /app/hook/deliveries[/{id}][/attempts]`.
- Installation events: `installation`, `installation_repositories` fire on store transitions.
- JSON Web Token verification: RS256 only (alg `none`/HMAC rejected), exp at most 10 minutes ahead of the server clock (+60s drift), future-iat rejection — backdated iat (ghinstallation-style) stays valid.

**OAuth Apps.** Distinct entity from GitHub Apps. Browser settings own OAuth App setup: `GET /settings/oauth-apps` lists the signed-in user's OAuth Apps, and `POST /settings/oauth-apps/new` creates one from form data and returns the one-time client secret. OAuth web flow (`/login/oauth/authorize`) requires the real login session plus CSRF consent POST before issuing an authorization code; device flow lives at `/login/device/code`. Token-management family on the real `/api/v3/applications/{client_id}/{token,grant}` (check / reset / revoke / scope).

**Token prefixes.** Match real GitHub exactly: `ghp_` (personal access token), `gho_` (OAuth App user-to-server), `ghu_` (GitHub App user-to-server), `ghs_` (server-to-server installation), `ghr_` (refresh). Middleware distinguishes all five.

**Permission enforcement.** `requirePerm(scope, level)` decorator gates write-class endpoints. Personal access token bypass matches real GitHub personal access tokens being full-scope. `ghs_` tokens are checked against `InstallationToken.Permissions`; `ghu_` tokens are checked against the GitHub App's installation permissions; `gho_` tokens are mapped from classic OAuth scopes.

**Actions OpenID Connect.** `GET /token` issues an RS256-signed JSON Web Token with the canonical claim set (sub, aud, repository, repository_owner, ref, run_id, run_number, sha, actor, environment, jti, exp). `GET /.well-known/jwks` plus `/.well-known/openid-configuration` support cloud identity-provider trust verification.

**Users application programming interface.** Public users, my-user, keys create/read/update/delete, gpg_keys, emails, followers / following, follow / unfollow.

**Meta.** `GET /meta` in GitHub Enterprise Server shape — Bleephub presents as GitHub Enterprise Server (`installed_version: "3.21.0"`). `gh` feature detection requires the member to resolve the host version; without it `gh issue list --label`, `gh pr status`, and `gh workflow run` fail.

**Pages.** Site CRUD, build records, deployments, and DNS-health checks are persisted. Manual legacy build requests validate the configured branch plus `/` or `/docs` source, publish `.nojekyll` static trees directly, and run the pinned `github-pages` 232/Jekyll 3.10.0 toolchain in safe production mode for Markdown, Liquid, layouts, themes, and GitHub-supported plugins. Generated or direct output is archived into S3-compatible object storage; real build errors become terminal Pages build errors and never create deployments.

**Branch protection.** PUT/GET/DELETE per-branch protection rules with typed required-status-checks, review, restriction, admin-enforcement, force-push, and deletion subresources.

**Organizations.** GitHub Enterprise Server admin create plus operator-only create; `GET /organizations` (global list with `since` cursor); organization-full profile (company / blog / location / twitter / billing email / `default_repository_permission` / `members_can_create_repositories` / `web_commit_signoff_required`) readable and PATCHable. Memberships with real invitation semantics: `PUT /orgs/{org}/memberships/{username}` invites (state `pending`), the invitee accepts via `PATCH /user/memberships/orgs/{org}` (`GET /user/memberships/orgs[/{org}]` lists/inspects); member checks (`GET/DELETE /orgs/{org}/members/{username}`), public members (list / check / publicize / conceal — self-only, like real GitHub). Teams: create/read/update/delete plus hierarchy (`parent_team_id`, child-team listing, cycle rejection, delete re-parents children), `notification_setting`, member roles (`member`/`maintainer`) with team-membership state mirroring the organization membership, team repositories (list with `permissions` + `role_name`, check including the `vnd.github.v3.repository+json` media type, add/remove), rename re-keys the slug. Outside collaborators, organization blocks, security-manager teams, member Codespaces administration, member Copilot seat details, and the owner-gated persisted audit log are implemented; audit-log reads support phrase/actor filters and GitHub-style pagination.

**Marketplace.** Listing plans and account purchases are backed by the marketplace plan/purchase store; GitHub's `stubbed` Marketplace endpoints serve the same persisted state as the production variants.

**GraphQL.** Repository / User / Organization queries + the IssueOrPullRequest union + repositoryOwner polymorphic root + repository.issues/pullRequests connections + `search(type: ISSUE)` + check-run/check-suite types + matching enums (RepositoryPrivacy, RepositoryAffiliation, IssueOrderField, OrderDirection, IssueState). Issue nodes expose Representational State Transfer-backed project items, assigned organization issue types (`Issue.issueType`), organization issue-field values (`Issue.issueFieldValues`), and sub-issue relationships (`parent`, ordered `subIssues`, and `subIssuesSummary`). Mutations cover the GraphQL verbs `gh` sends: createIssue / addComment / closeIssue / reopenIssue, createPullRequest / closePullRequest / reopenPullRequest / mergePullRequest / addPullRequestReview, createRepository / deleteRepository, and Projects v2 (createProjectV2, addProjectV2ItemById, createProjectV2Field, updateProjectV2ItemFieldValue). `Issue.projectItems.fieldValueByName` reads the real Projects v2 store and returns typed text, number, date, single-select, and iteration field-value union members.

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

- default — in-memory bytes while metadata persistence is disabled;
- `BLEEPHUB_OBJECT_S3_BUCKET` (+ optional `BLEEPHUB_OBJECT_S3_ENDPOINT`, `BLEEPHUB_OBJECT_S3_PREFIX`) — service byte content in S3-compatible object storage: GitHub Actions artifacts, dependency caches, runner-uploaded log files, release assets, package files, container-registry blobs, CodeQL database archives, CodeQL variant-analysis query packs, artifact attestation bundles, and published GitHub Pages archives. If `BLEEPHUB_OBJECT_S3_BUCKET` is set and the bucket cannot be reached with `HeadBucket`, startup fails loudly.
- `BLEEPHUB_PAGES_JEKYLL_EXECUTABLE` — executable coordinate for the GitHub Pages Jekyll command contract; defaults to the `bleephub-pages-jekyll` wrapper shipped in the release image and is used directly without a static-copy fallback.

Database persistence **requires** `BLEEPHUB_OBJECT_S3_BUCKET`: SQLite stores Bleephub metadata, while GitHub Actions artifact, dependency-cache, runner-log, release-asset, package-file, container-registry, CodeQL database archive, CodeQL variant-analysis query-pack, and artifact attestation bundle bytes must live in object storage. Persisted startup fails loudly when this bucket is absent instead of storing byte content in memory or local files.

The object-byte tests also drive a real `simulator-aws` S3 endpoint: artifact upload, cache upload, runner log upload, release asset upload, package file upload/download, container-registry blob upload/download, CodeQL database archive upload/download, CodeQL variant-analysis query-pack upload/download, artifact attestation bundle upload/list/delete, GitHub Pages publication/serving/replacement/deletion, and public job-log download assert the expected S3 objects are written and read back, so these paths do not rely on fake S3 or memory-only assertions.

### `gh` command-line interface compatibility

Bleephub accepts what real GitHub accepts — including the string-coerced booleans / integers `gh api -f` sends (real GitHub's Rails layer coerces them; Bleephub's `flexBool`/`flexInt`/`flexInt64`/`flexIntSlice` types decode either form). The `gh` command-line interface works against Bleephub directly:

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

Verified end-to-end by [`make bleephub-gh-docker-test`](#integration-tests), which builds a Docker image bundling Bleephub, the official `gh` command-line interface, and a self-signed Transport Layer Security certificate, then runs the harness against the live Bleephub binary inside the container.

## What it does not implement (deferred)

- V2 broker flow (uses legacy V1 pipelines paths).
- Failed-run shells exist for TRIGGERED workflows that can't start (conclusion `startup_failure`, no jobs); explicit dispatches still 422 with the parse error (more useful to the caller).
- SAML SSO + SCIM provisioning.
- Org `plan` member / billing endpoints (bleephub has no billing model).
- Marketplace billing.
- `gh` command-line interface commands that require deep workflow-run state Bleephub does not synthesize (`gh run watch` long-poll, log tail).
- `on: schedule` crons fire from real server time (minute-aligned); there is no time-warp hook for tests beyond calling the dispatcher directly.

## How it works

```
┌──────────────────┐     internal surface  ┌───────────┐     Docker surface ┌────────────┐
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
4. A job is submitted via `POST /internal/exec/submit` (simplified JavaScript Object Notation; operator-only, not a GitHub application programming interface path).
5. bleephub converts to the internal job-message format and delivers it.
6. Runner creates a Docker container through `DOCKER_HOST` (pointing at Sockerless).
7. Runner execs each `run:` step inside the container via `docker exec`.
8. Runner reports step status; bleephub marks the job completed.

For ad-hoc Representational State Transfer / GraphQL workflows (Probot, Octokit, `gh`):
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
- `BLEEPHUB_ADMIN_TOKEN=<token>` — **required.** The seeded admin token. There is no default (a default would be a guessable credential, and the historical `ghp_...` value tripped secret scanners); the binary fails loudly at startup if unset. Set a non-personal-access-token-shaped value.
- `BLEEPHUB_PERSIST=true` — enable SQLite persistence (off by default; see [Persistence](#persistence)).
- `BLEEPHUB_DATA_DIR=<dir>` — directory for the SQLite database (`bleephub.db`) and local non-persistent development metadata (default `.`).
- `BLEEPHUB_GIT_DIR=<dir>` — store git repos on the local filesystem (default: in-memory).
- `BLEEPHUB_S3_BUCKET` / `BLEEPHUB_S3_ENDPOINT` / `BLEEPHUB_S3_PREFIX` — store git repos in S3-compatible object storage (bucket set ⇒ S3 wins over `BLEEPHUB_GIT_DIR`).
- `BLEEPHUB_OBJECT_S3_BUCKET` / `BLEEPHUB_OBJECT_S3_ENDPOINT` / `BLEEPHUB_OBJECT_S3_PREFIX` — store GitHub Actions artifacts, dependency caches, runner logs, release assets, package files, container-registry blobs, CodeQL database archives, CodeQL variant-analysis query packs, and artifact attestation bundles in S3-compatible object storage; required when `BLEEPHUB_PERSIST=true`.
- `BPH_TLS_CERT` + `BPH_TLS_KEY` — serve over TLS.
- `BLEEPHUB_MAX_WORKFLOWS=N` — concurrency cap (default 10).
- `OTEL_EXPORTER_OTLP_ENDPOINT` — when set, emits traces + metrics + logs via OTLP (off by default; preserves the components-decoupled invariant).

## Integration tests

```bash
# Go unit tests
make bleephub/test                  # go test ./bleephub/...

# Official actions/runner end-to-end (Docker; self-contained)
make bleephub-runner-docker-test

# Real gh command-line interface inside Docker (real Bleephub + real gh binary + self-signed TLS)
make bleephub-gh-docker-test
```

`bleephub-runner-docker-test` builds `bleephub/Dockerfile` (bleephub + the
official `actions/runner` binary) and runs
`bleephub/test/run-integration.sh`: the runner registers against bleephub,
polls the broker, executes HOST-MODE jobs (`jobContainer` null — real
GitHub's shape for jobs without `container:`), uploads timeline records and
logs, and completes. Runs in continuous integration as the Bleephub actions/runner job.
Container-mode jobs against the cloud backends are gated on the
bind-mount→EFS translation tracked in
[`docs/GITHUB_RUNNER.md`](../docs/GITHUB_RUNNER.md).

The gh harness builds `bleephub/Dockerfile.gh-test` and runs `bleephub/test/run-gh-test.sh`. It exercises:
- `gh auth login` against Bleephub as a GitHub Enterprise Server host
- Native `gh repo create / view / list`, `gh issue create / view / list` (Representational State Transfer and GraphQL paths)
- `gh secret set` (real sealed-box encryption), `gh variable set/get/list/delete`, `gh workflow run / enable / disable`, check-runs on pushed commits
- The parity probes for endpoints with no native `gh` verb (apps/{slug}, /applications/{cid}/token, suspend, OAuth Apps management)

Runs in continuous integration as the Bleephub gh command-line interface job (must be green to merge).

### OpenAPI fidelity gates (hermetic)

Two unit-test gates validate bleephub against the vendored GitHub OpenAPI description (`testdata/github-openapi.json.gz`, refreshed via `scripts/update-github-openapi.sh`):

- **Route definitions** (`gh_api_definition_test.go`) — every registered `/api/v3` route must exist in the description; paths can't be invented under GitHub's namespace.
- **Response-shape ratchet** (`openapi_shape_validator_test.go`) — an observer on the shared test server validates every 2xx `/api/v3` JavaScript Object Notation response member-by-member against the documented response schema. Violations are gated against [`openapi-violation-allowlist.txt`](openapi-violation-allowlist.txt): each entry is either a real-but-undescribed member (GitHub Enterprise Server-only surface, with a citation — currently only `/meta`'s `installed_version`) or a filed bug on its way to being fixed. The list only shrinks; new violations fail the suite.

## Source layout (~180 Go files)

| Group | Files | Purpose |
|---|---|---|
| Core protocol | `server.go`, `auth.go`, `agents.go`, `broker.go`, `run_service.go`, `timeline.go` | Runner registration, job delivery, lifecycle |
| Jobs & workflows | `jobs.go`, `workflow.go`, `workflows.go`, `workflows_msg.go`, `matrix.go`, `outputs.go`, `secrets.go`, `expressions.go`, `actions.go`, `artifacts.go` | Multi-job, matrix, secrets, expressions, artifacts |
| GitHub Representational State Transfer core | `gh_rest.go`, `gh_repos_*.go`, `gh_orgs_*.go`, `gh_issues_*.go`, `gh_pulls_*.go`, `gh_teams_rest.go`, `gh_labels_rest.go`, `gh_members_rest.go` | Repositories, organizations, issues, pull requests, teams, labels, milestones |
| GitHub Apps + OAuth | `gh_apps_*.go`, `gh_oauth.go`, `gh_app_hooks_rest.go`, `gh_apps_user_tokens.go`, `gh_apps_oauth_mgmt.go`, `gh_apps_perms.go` | JSON Web Token authentication, installations, OAuth Apps, ghs_/ghu_/gho_/ghr_, permission enforcement |
| Reactions + Releases + Deployments | `gh_reactions.go`, `gh_releases.go`, `gh_deployments.go`, `gh_pr_comments.go`, `gh_pr_threads.go` | Reactions, releases, deployments + environments + approvals, pull request review comments/threads |
| Actions extras | `gh_actions_rest.go`, `gh_actions_extras.go`, `gh_workflows_rest.go` | Runs/jobs/steps, repository_dispatch, logs zip, timing |
| Checks application programming interface | `gh_checks_rest.go`, `gh_checks_store.go` | check-runs + check-suites |
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
