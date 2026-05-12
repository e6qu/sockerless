# Using `gh` CLI against bleephub

bleephub speaks the same REST + GraphQL surface as GitHub Enterprise Server (`/api/v3/` path prefix, `/api/graphql` endpoint, GHES service routing). The `gh` CLI works against it directly — no shims, no `gh api` URL hackery, no flags.

## One-time auth

bleephub serves on `:5555` by default (HTTP). For TLS in production / integration tests, run with `BPH_TLS_CERT` + `BPH_TLS_KEY` and use HTTPS.

```bash
# Plain HTTP (sim mode, dev)
export BLEEPHUB_HOST=localhost:5555

# TLS (integration tests bind to :443)
export BLEEPHUB_HOST=localhost
```

bleephub seeds a default admin user with a static PAT (`bph_0000000000000000000000000000000000000000`). Other tokens can be minted via the OAuth flow or `POST /api/v3/bleephub/apps/{id}/installations/.../access_tokens`.

```bash
# Use the seeded admin PAT
TOKEN="bph_0000000000000000000000000000000000000000"

# Tell gh that bleephub is a GHES-like host:
echo "$TOKEN" | gh auth login --hostname "$BLEEPHUB_HOST" --with-token

# Optional: make it the default host so you don't have to pass --host
export GH_HOST="$BLEEPHUB_HOST"
```

That's it. `gh` is now authenticated against bleephub.

## Supported commands

These work natively (no `gh api` workaround needed):

| Command | Endpoint(s) |
|---|---|
| `gh repo create <name>` | `POST /user/repos` |
| `gh repo view <owner/name>` | `GET /repos/{o}/{r}` + GraphQL `repository` |
| `gh repo list <owner>` | GraphQL `repositoryOwner(login).repositories` |
| `gh repo clone <owner/name>` | smart-HTTP git protocol |
| `gh repo delete <owner/name>` | `DELETE /repos/{o}/{r}` |
| `gh issue create --title --body` | `POST /repos/{o}/{r}/issues` |
| `gh issue view <N>` | GraphQL `repository.issueOrPullRequest` (Issue\|PullRequest union) |
| `gh issue list` | GraphQL `repository.issues` connection |
| `gh issue comment <N> --body` | `POST /repos/{o}/{r}/issues/{n}/comments` |
| `gh issue close / reopen <N>` | `PATCH /repos/{o}/{r}/issues/{n}` |
| `gh pr create` (in a git working dir) | `POST /repos/{o}/{r}/pulls` |
| `gh pr view <N>` | GraphQL `repository.pullRequest` |
| `gh pr list` | GraphQL `repository.pullRequests` connection |
| `gh pr merge <N>` | `PUT /repos/{o}/{r}/pulls/{n}/merge` |
| `gh pr review --approve` / `--request-changes` / `--comment` | `POST /repos/{o}/{r}/pulls/{n}/reviews` |
| `gh pr comment <N>` | issue-comment on PR via `POST /repos/{o}/{r}/issues/{n}/comments` |
| `gh release create <tag>` | `POST /repos/{o}/{r}/releases` |
| `gh release list / view / delete` | GET / PATCH / DELETE on releases |
| `gh release download` | `assets_url` redirect (sim returns empty assets) |
| `gh run list / view / cancel / rerun` | `GET/POST /repos/{o}/{r}/actions/runs*` |
| `gh workflow run <wf> --ref <branch>` | `POST /repos/{o}/{r}/actions/workflows/{id}/dispatches` |
| `gh workflow list / view / enable / disable` | `GET/PUT /actions/workflows/{id}` |
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
| `bph_` | Seeded admin | All scopes | Sim default; bypasses `requirePerm` |
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

For a comprehensive smoke test, run [`make bleephub-gh-docker-test`](../bleephub/test/run-gh-test.sh) which spins up bleephub + the official `gh` binary in Docker with TLS and exercises 50 distinct assertions.

## When things go wrong

- **`gh auth login` keeps asking for credentials.** Make sure you used `--with-token` and the token is non-empty. `GH_TOKEN` env also works as a fallback.
- **`gh repo list` returns empty / 404.** GraphQL queries depend on the `repositoryOwner` resolver — confirm bleephub is current (Phase 154+).
- **`gh issue view` returns "fragment cannot be spread"-style errors.** Should be impossible on Phase 153+ (the `IssueOrPullRequest` union is wired). File a BUG.md entry if seen.
- **`gh api -f` returns 400.** Should not happen on Phase 153+ (`flexBool`/`flexInt` decoders handle string-coerced inputs). File a bug.
- **TLS errors.** When using `BPH_TLS_CERT` with a self-signed cert, either trust the CA system-wide (the Docker harness does this) or pass `--insecure` to `gh api`.

See also: [specs/BLEEPHUB_GITHUB_API_PARITY.md](../specs/BLEEPHUB_GITHUB_API_PARITY.md) for the per-endpoint inventory.
