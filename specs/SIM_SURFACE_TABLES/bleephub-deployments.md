# bleephub — Deployments & Environments

Surface: `bleephub/gh_deployments.go`.

Canonical reference: <https://docs.github.com/en/enterprise-server/rest/deployments>

## Status legend

- ✓ — implemented + tested
- ✗ — implemented, no direct test coverage

## Deployments

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create deployment | `POST /api/v3/repos/{owner}/{repo}/deployments` | ✓ `gh_deployments.go::handleCreateDeployment` | ✓ `gh_deployments_test.go` | |
| List deployments | `GET /api/v3/repos/{owner}/{repo}/deployments` | ✓ `handleListDeployments` | ✓ same | |
| Get deployment | `GET /api/v3/repos/{owner}/{repo}/deployments/{deployment_id}` | ✓ `handleGetDeployment` | ✓ same | |
| Delete deployment | `DELETE /api/v3/repos/{owner}/{repo}/deployments/{deployment_id}` | ✓ `handleDeleteDeployment` | ✓ same | |

## Deployment statuses

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create status | `POST /api/v3/repos/{owner}/{repo}/deployments/{deployment_id}/statuses` | ✓ `handleCreateDeploymentStatus` | ✓ `gh_deployments_test.go` | |
| List statuses | `GET /api/v3/repos/{owner}/{repo}/deployments/{deployment_id}/statuses` | ✓ `handleListDeploymentStatuses` | ✓ same | |
| Get status | `GET /api/v3/repos/{owner}/{repo}/deployments/{deployment_id}/statuses/{status_id}` | ✓ `handleGetDeploymentStatus` | ✓ same | |

## Environments

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List environments | `GET /api/v3/repos/{owner}/{repo}/environments` | ✓ `handleListEnvironments` | ✓ `gh_deployments_test.go` | |
| Get environment | `GET /api/v3/repos/{owner}/{repo}/environments/{env_name}` | ✓ `handleGetEnvironment` | ✓ same | |
| Create / update environment | `PUT /api/v3/repos/{owner}/{repo}/environments/{env_name}` | ✓ `handleUpsertEnvironment` | ✓ same | |
| Delete environment | `DELETE /api/v3/repos/{owner}/{repo}/environments/{env_name}` | ✓ `handleDeleteEnvironment` | ✓ same | |
