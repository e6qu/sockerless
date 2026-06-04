# bleephub — Repositories

Surface: `bleephub/gh_repos_rest.go`, `bleephub/gh_repos_refs.go`, `bleephub/gh_repos_objects.go`.

Canonical reference: <https://docs.github.com/en/enterprise-server/rest/repos>

## Status legend

- ✓ — implemented + tested
- ✗ — implemented, no direct test coverage

## Repo CRUD

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create user repo | `POST /api/v3/user/repos` | ✓ `gh_repos_rest.go::handleCreateRepo` | ✓ `gh_repos_test.go` | |
| List authenticated user repos | `GET /api/v3/user/repos` | ✓ `handleListAuthUserRepos` | ✓ same | |
| Get repo | `GET /api/v3/repos/{owner}/{repo}` | ✓ `handleGetRepo` | ✓ same | |
| Update repo | `PATCH /api/v3/repos/{owner}/{repo}` | ✓ `handleUpdateRepo` | ✓ same | |
| Delete repo | `DELETE /api/v3/repos/{owner}/{repo}` | ✓ `handleDeleteRepo` | ✓ same | |
| List user repos | `GET /api/v3/users/{username}/repos` | ✓ `handleListUserRepos` | ✓ same | |

## Branches & refs

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List branches | `GET /api/v3/repos/{owner}/{repo}/branches` | ✓ `gh_repos_refs.go::handleListBranches` | ✓ `gh_repos_test.go` | |
| Get branch | `GET /api/v3/repos/{owner}/{repo}/branches/{branch}` | ✓ `handleGetBranch` | ✓ same | |
| Delete ref | `DELETE /api/v3/repos/{owner}/{repo}/git/refs/{ref...}` | ✓ `handleDeleteRef` | ✓ same | Supports multi-segment ref paths. |

## Commits & git objects

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List commits | `GET /api/v3/repos/{owner}/{repo}/commits` | ✓ `gh_repos_objects.go::handleListCommits` | ✓ `gh_repos_test.go` | |
| Get tree | `GET /api/v3/repos/{owner}/{repo}/git/trees/{sha}` | ✓ `handleGetTree` | ✓ same | |
| Get blob | `GET /api/v3/repos/{owner}/{repo}/git/blobs/{sha}` | ✓ `handleGetBlob` | ✓ same | |
| Get readme | `GET /api/v3/repos/{owner}/{repo}/readme` | ✓ `handleGetReadme` | ✓ same | |
| Get contents | `GET /api/v3/repos/{owner}/{repo}/contents/{path...}` | ✓ `handleGetContents` | ✓ same | Supports nested paths. |
