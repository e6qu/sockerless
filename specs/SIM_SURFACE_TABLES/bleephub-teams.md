# bleephub — Teams

Surface: `bleephub/gh_teams_rest.go`, `bleephub/gh_members_rest.go`.

Canonical reference: <https://docs.github.com/en/enterprise-server/rest/teams>

## Status legend

- ✓ — implemented + tested
- ✗ — implemented, no direct test coverage

## Team CRUD

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List user teams | `GET /api/v3/user/teams` | ✓ `gh_teams_rest.go::handleListAuthUserTeams` | ✓ `gh_orgs_test.go` | |
| Create team | `POST /api/v3/orgs/{org}/teams` | ✓ `handleCreateTeam` | ✓ same | |
| List org teams | `GET /api/v3/orgs/{org}/teams` | ✓ `handleListTeams` | ✓ same | |
| Get team | `GET /api/v3/orgs/{org}/teams/{team_slug}` | ✓ `handleGetTeam` | ✓ same | |
| Update team | `PATCH /api/v3/orgs/{org}/teams/{team_slug}` | ✓ `handleUpdateTeam` | ✓ same | |
| Delete team | `DELETE /api/v3/orgs/{org}/teams/{team_slug}` | ✓ `handleDeleteTeam` | ✓ same | |

## Org membership

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List org members | `GET /api/v3/orgs/{org}/members` | ✓ `gh_members_rest.go::handleListOrgMembers` | ✓ `gh_orgs_test.go` | |
| Get org membership | `GET /api/v3/orgs/{org}/memberships/{username}` | ✓ `handleGetOrgMembership` | ✓ same | |
| Set org membership | `PUT /api/v3/orgs/{org}/memberships/{username}` | ✓ `handleSetOrgMembership` | ✓ same | |
| Remove org membership | `DELETE /api/v3/orgs/{org}/memberships/{username}` | ✓ `handleRemoveOrgMembership` | ✓ same | |

## Team membership & repo access

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List team members | `GET /api/v3/orgs/{org}/teams/{team_slug}/members` | ✓ `handleListTeamMembers` | ✓ `gh_orgs_test.go` | |
| Add team member | `PUT /api/v3/orgs/{org}/teams/{team_slug}/memberships/{username}` | ✓ `handleAddTeamMember` | ✓ same | |
| Remove team member | `DELETE /api/v3/orgs/{org}/teams/{team_slug}/memberships/{username}` | ✓ `handleRemoveTeamMember` | ✓ same | |
| Add team repo | `PUT /api/v3/orgs/{org}/teams/{team_slug}/repos/{owner}/{repo}` | ✓ `handleAddTeamRepo` | ✓ same | |
| Remove team repo | `DELETE /api/v3/orgs/{org}/teams/{team_slug}/repos/{owner}/{repo}` | ✓ `handleRemoveTeamRepo` | ✓ same | |
