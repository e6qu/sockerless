# Using `gh` CLI against bleephub

bleephub speaks the same REST + GraphQL surface as GitHub Enterprise Server (`/api/v3/` path prefix, `/api/graphql` endpoint, GHES service routing). The `gh` CLI works against it directly — no shims, no `gh api` URL hackery, no flags.

## The mental model — `--hostname`, not a base URL

`gh` does **not** take a base-URL argument. It identifies a target by **hostname** and derives the URLs from it using a fixed rule:

| Host | API base | GraphQL |
|---|---|---|
| `github.com` | `https://api.github.com/` | `https://api.github.com/graphql` |
| anything else | `https://<host>/api/v3/` | `https://<host>/api/graphql` |

So when you run `gh auth login --hostname localhost --with-token`, `gh` writes a record to `~/.config/gh/hosts.yml` under the key `localhost` and from that point on builds every API call as `https://localhost/api/v3/...`. bleephub serves both `/api/v3/` and `/api/graphql` — that's the entire wiring story.

Three consequences:

- **`gh` is HTTPS-only against any non-`github.com` host.** Plain HTTP on `:5555` will not work. Run bleephub with `BPH_TLS_CERT` + `BPH_TLS_KEY` (the Docker harness does this; bare-metal recipe in [`bleephub/README.md`](../bleephub/README.md#quick-start--bleephub--gh-cli-in-5-steps)).
- **`gh auth login --hostname` accepts a bare hostname only.** Current `gh` (verified on 2.92.0) rejects `host:port` with `error parsing hostname: invalid hostname`, so the login flow requires bleephub on `:443`. If you can't (or don't want to) bind 443, skip `gh auth login` entirely: `export GH_HOST=localhost:8443` + `export GH_ENTERPRISE_TOKEN=<token>` — the runtime accepts a port in `GH_HOST` and the env token replaces the hosts.yml entry. The variable must be `GH_ENTERPRISE_TOKEN`: gh reads `GH_TOKEN` only for `github.com`, and sends nothing to other hosts when only `GH_TOKEN` is set (bleephub answers `401 Bad credentials`).
- **macOS trust comes only from the keychain.** `gh` is a Go binary, and Go on darwin ignores `SSL_CERT_FILE`/`SSL_CERT_DIR` — the self-signed cert MUST be added to the system keychain (`sudo security add-trusted-cert …`, see the quick start). On Linux the usual CA-store mechanisms work.

## One-time auth

```bash
# The admin user's token is whatever BLEEPHUB_ADMIN_TOKEN was set to when
# bleephub started (required, no default — see bleephub/README.md § Usage).
# The Docker harnesses use this value:
TOKEN="bleephub-admin-token-00000000000000000000"

# Option A — bleephub on any port (e.g. :8443), no gh auth login needed.
# GH_HOST accepts host:port at runtime; GH_ENTERPRISE_TOKEN is the env
# credential gh uses for every non-github.com host (GH_TOKEN is
# github.com-only and is silently ignored here).
export GH_HOST=localhost:8443
export GH_ENTERPRISE_TOKEN="$TOKEN"

# Option B — bleephub on :443: the bare hostname passes gh auth login's
# validator, giving you a persistent ~/.config/gh/hosts.yml entry.
echo "$TOKEN" | gh auth login --hostname localhost --with-token
export GH_HOST=localhost
```

Other tokens (OAuth user, installation server-to-server) can be minted via the OAuth flow or the real GitHub endpoint `POST /api/v3/app/installations/{installation_id}/access_tokens` (JWT-authenticated) — use the resulting token in place of `$TOKEN` on the `gh auth login` line.

That's it. `gh` is now authenticated against bleephub.

Setup and full teardown (server, cert material, keychain trust, gh wiring) are
idempotent shell blocks in the quick start — see
[`bleephub/README.md`](../bleephub/README.md#quick-start--bleephub--gh-cli-in-5-steps)
and its **Teardown** section.

## Supported commands

These work natively (no `gh api` workaround needed):

| Command | Endpoint(s) |
|---|---|
| `gh repo create <name>` | `POST /user/repos` |
| `gh repo view <owner/name>` | `GET /repos/{o}/{r}` + GraphQL `repository` |
| `gh repo list <owner>` | GraphQL `repositoryOwner(login).repositories` |
| `gh repo clone <owner/name>` | GraphQL `RepositoryInfo` (`hasWikiEnabled`, `parent`) + smart-HTTP git protocol |
| `gh repo delete <owner/name>` | `DELETE /repos/{o}/{r}` |
| `gh issue create --title --body` | GraphQL `createIssue` mutation |
| `gh issue view <N>` | GraphQL `repository.issueOrPullRequest` (Issue\|PullRequest union) |
| `gh issue list` | GraphQL `repository.issues` connection; `--label`/`--author`/`--search` route through GraphQL `search(type: ISSUE)` gated on `GET /meta` feature detection |
| `gh issue comment <N> --body` | GraphQL `addComment` mutation |
| `gh issue close / reopen <N>` | GraphQL `closeIssue` / `reopenIssue` mutations |
| `gh pr create` (in a git working dir) | GraphQL `RepositoryInfo` + `createPullRequest` mutation |
| `gh pr view <N>` | GraphQL `repository.pullRequest` (incl. `statusCheckRollup` via `commits(last:1)`, backed by the checks store) |
| `gh pr list` | GraphQL `repository.pullRequests` connection (enum `orderBy`) |
| `gh pr status` | GraphQL `search(type: ISSUE)` + `repository.pullRequests`; needs `GET /meta` |
| `gh pr merge <N>` | GraphQL `mergePullRequest` mutation (finder reads `mergeStateStatus` + `commits(last:1)`) |
| `gh pr review --approve` / `--request-changes` / `--comment` | GraphQL `addPullRequestReview` mutation |
| `gh pr comment <N>` | GraphQL `addComment` mutation |
| `gh release create <tag>` | `POST /repos/{o}/{r}/releases` |
| `gh release list` | GraphQL `repository.releases` connection |
| `gh release view / delete` | `GET`/`DELETE /repos/{o}/{r}/releases*` + GraphQL `repository.release(tagName:)` draft lookup |
| `gh release download` | `assets_url` redirect (sim returns empty assets) |
| `gh run list / view / cancel / rerun` | `GET/POST /repos/{o}/{r}/actions/runs*` (push-triggered runs resolve their `workflow_id`) |
| `gh workflow run <wf> --ref <branch>` | `POST /repos/{o}/{r}/actions/workflows/{id}/dispatches`; version-gated on `GET /meta` |
| `gh workflow list / view` | `GET /actions/workflows[/{id}]` |
| `gh workflow enable / disable` | `PUT /actions/workflows/{id}/{enable,disable}`; disabled workflows don't trigger and dispatch returns 403 |
| `gh secret set / list / delete` | `GET /actions/secrets/public-key` + libsodium sealed-box `PUT {encrypted_value, key_id}` / `GET /actions/secrets` / `DELETE /actions/secrets/{name}`; org + environment scopes too |
| `gh variable set / get / list / delete` | `POST`/`PATCH`/`GET`/`DELETE /actions/variables[/{name}]` (gh's POST→409→PATCH update fallback works); org + environment scopes too |
| `gh org list` | GraphQL `user(login:).organizations` connection |
| `gh api /repos/{o}/{r}/...` | direct REST passthrough |

## Endpoints with no native `gh` verb

Use `gh api` for these (real GH also doesn't expose them in `gh`'s top-level commands):

```bash
gh api /apps/<slug>                                           # public app lookup (anon-allowed)
gh api -X PUT /app/installations/{id}/suspended               # suspend
gh api -X DELETE /app/installations/{id}/suspended            # unsuspend
gh api /installation/repositories                              # ghs_-token-scoped repos
gh api /repos/{o}/{r}/environments                             # env list
gh api -X POST /repos/{o}/{r}/dispatches -f event_type=deploy  # repository_dispatch
gh api /repos/{o}/{r}/branches/main/protection                 # branch protection
gh api /token                                                  # Actions OIDC token
gh api /.well-known/jwks                                       # JWKS for cloud-IdP verification
```

## Tokens at a glance

| Prefix | Issued by | Scope model | Use case |
|---|---|---|---|
| (admin) | `BLEEPHUB_ADMIN_TOKEN` env var at startup | All scopes | Operator/admin token; bypasses `requirePerm` |
| `ghp_` | `POST /login/oauth/access_token` (legacy) | All scopes | Classic PAT |
| `gho_` | OAuth web/device flow (OAuth App) | Classic OAuth scopes (`repo`, `read:org`, …) | OAuth App user tokens |
| `ghu_` | OAuth flow against a GitHub App | App installation perms | GitHub App user-to-server |
| `ghs_` | `POST /app/installations/{id}/access_tokens` | Installation-scoped perms | Server-to-server |
| `ghr_` | Paired with `gho_` / `ghu_` | — | Refresh token (6 month TTL) |

`requirePerm(scope, level)` enforces permissions on write-class endpoints. PATs bypass; `ghs_` / `ghu_` / `gho_` get checked against their respective scope tables. See [specs/BLEEPHUB_GITHUB_API_PARITY.md](../specs/BLEEPHUB_GITHUB_API_PARITY.md) § "Permission enforcement on installation tokens" for the exact mapping.

## Body coercion

bleephub accepts both typed and string-coerced JSON booleans/integers — what `gh api -f` sends (string `"false"`) gets coerced to bool `false` server-side, exactly as Rails does on real GH. `gh api -F` (typed) also works. Don't substitute one form for the other; bleephub accepts what real GH accepts.

## Testing your gh setup end-to-end

```bash
# Round-trip: create repo → issue → react → comment → close
gh repo create bleephub-test --public --description "smoke"
ISSUE=$(gh issue create --repo admin/bleephub-test --title "first" --body "hello")
gh issue view 1 --repo admin/bleephub-test
gh api -X POST /repos/admin/bleephub-test/issues/1/reactions -f content="rocket"
gh issue comment 1 --repo admin/bleephub-test --body "great work"
gh issue close 1 --repo admin/bleephub-test
gh issue list --repo admin/bleephub-test --state closed
```

For a comprehensive smoke test, run [`make bleephub-gh-docker-test`](../bleephub/test/run-gh-test.sh) which spins up bleephub + the official `gh` binary in Docker with TLS and exercises the full gh-CLI assertion suite (repos, issues, PRs, reactions, releases, runs, apps, OAuth). It runs in CI as the `sim (bleephub gh CLI)` job.

## When things go wrong

- **`gh auth login` keeps asking for credentials.** Make sure you used `--with-token` and the token is non-empty. `GH_ENTERPRISE_TOKEN` also works as an env fallback (not `GH_TOKEN` — that's read for `github.com` only).
- **`gh` is hitting `github.com` instead of bleephub.** You forgot `--hostname <bleephub-host>` on `gh auth login`, or `GH_HOST` isn't exported. `gh` only routes to bleephub if the hostname is in `~/.config/gh/hosts.yml` AND either `GH_HOST` matches it or every command passes `--hostname` explicitly.
- **`gh auth login` fails with `dial tcp [::1]:443: connection refused` / `x509: cannot validate ...`.** bleephub is on a plain-HTTP port, or its cert isn't trusted. `gh` is HTTPS-only — run bleephub with `BPH_TLS_CERT` + `BPH_TLS_KEY` and trust the CA system-wide. If you can't bind to `:443`, skip `gh auth login` (it rejects `host:port`) and use `GH_HOST=localhost:8443` + `GH_ENTERPRISE_TOKEN` instead.
- **`gh repo list` returns empty / 404.** GraphQL queries depend on the `repositoryOwner` resolver — confirm your bleephub binary is current.
- **`gh issue view` returns "fragment cannot be spread"-style errors.** Should be impossible (the `IssueOrPullRequest` union is wired). File a BUGS.md entry if seen.
- **`gh api -f` returns 400.** Should not happen (`flexBool`/`flexInt` decoders handle string-coerced inputs). File a bug.
- **TLS errors.** When using `BPH_TLS_CERT` with a self-signed cert, either trust the CA system-wide (the Docker harness does this) or pass `--insecure` to `gh api`.

See also: [specs/BLEEPHUB_GITHUB_API_PARITY.md](../specs/BLEEPHUB_GITHUB_API_PARITY.md) for the per-endpoint inventory.
