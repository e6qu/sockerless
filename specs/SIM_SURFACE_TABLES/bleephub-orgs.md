# bleephub — Organizations

Surface: `bleephub/gh_orgs_rest.go`.

Canonical reference: <https://docs.github.com/en/enterprise-server/rest/orgs>

## Status legend

- ✓ — implemented + tested
- ✗ — implemented, no direct test coverage

## Org CRUD

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Admin create org | `POST /api/v3/admin/organizations` | ✓ `gh_orgs_rest.go::handleAdminCreateOrg` | ✓ `gh_orgs_test.go` | Site-admin only; returns 403 for non-admin callers. |
| Create org (user-context) | `POST /api/v3/user/orgs` | ✓ `handleCreateOrg` | ✓ same | |
| List authenticated user orgs | `GET /api/v3/user/orgs` | ✓ `handleListAuthUserOrgs` | ✓ same | |
| Get org | `GET /api/v3/orgs/{org}` | ✓ `handleGetOrg` | ✓ same | |
| Update org | `PATCH /api/v3/orgs/{org}` | ✓ `handleUpdateOrg` | ✓ same | |
| Delete org | `DELETE /api/v3/orgs/{org}` | ✓ `handleDeleteOrg` | ✓ same | |
| List user orgs | `GET /api/v3/users/{username}/orgs` | ✓ `handleListUserOrgs` | ✓ same | |

## Org repos

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create org repo | `POST /api/v3/orgs/{org}/repos` | ✓ `handleCreateOrgRepo` | ✓ `gh_repos_test.go` | |
