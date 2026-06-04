# bleephub — Actions

Surface: `bleephub/gh_actions_rest.go`, `bleephub/gh_actions_extras.go`, `bleephub/gh_workflows_rest.go`.

Canonical reference: <https://docs.github.com/en/enterprise-server/rest/actions>

## Status legend

- ✓ — implemented + tested
- ✗ — implemented, no direct test coverage

## Workflow runs

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List workflow runs | `GET /api/v3/repos/{owner}/{repo}/actions/runs` | ✓ `gh_actions_rest.go::handleListWorkflowRuns` | ✓ `gh_actions_test.go` | |
| Get workflow run | `GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}` | ✓ `handleGetWorkflowRun` | ✓ same | |
| Delete workflow run | `DELETE /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}` | ✓ `handleDeleteWorkflowRun` | ✓ same | |
| Cancel workflow run | `POST /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/cancel` | ✓ `handleCancelWorkflowRun` | ✓ same | |
| Rerun workflow run | `POST /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/rerun` | ✓ `handleRerunWorkflowRun` | ✓ same | |
| Rerun failed jobs | `POST /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/rerun-failed-jobs` | ✓ `gh_actions_extras.go::handleRerunFailedJobs` | ✓ same | |
| Get run logs | `GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/logs` | ✓ `handleGetRunLogs` | ✓ same | |
| Get run timing | `GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/timing` | ✓ `handleGetRunTiming` | ✓ same | |
| Get run approvals | `GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/approvals` | ✓ `handleGetRunApprovals` | ✓ same | |

## Jobs

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List run jobs | `GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/jobs` | ✓ `handleListWorkflowRunJobs` | ✓ `gh_actions_test.go` | |
| Get job | `GET /api/v3/repos/{owner}/{repo}/actions/jobs/{job_id}` | ✓ `handleGetWorkflowJob` | ✓ same | |
| Get job logs | `GET /api/v3/repos/{owner}/{repo}/actions/jobs/{job_id}/logs` | ✓ `handleGetWorkflowJobLogs` | ✓ same | |

## Artifacts

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List run artifacts | `GET /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/artifacts` | ✓ `gh_actions_extras.go::handleListRunArtifacts` | ✓ `gh_actions_test.go` | |
| List repo artifacts | `GET /api/v3/repos/{owner}/{repo}/actions/artifacts` | ✓ `handleListRepoArtifacts` | ✓ same | |

## Runners

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List runners | `GET /api/v3/repos/{owner}/{repo}/actions/runners` | ✓ `handleListRunners` | ✓ `gh_actions_test.go` | |
| Delete runner | `DELETE /api/v3/repos/{owner}/{repo}/actions/runners/{runner_id}` | ✓ `handleDeleteRunner` | ✓ same | |

## Workflow dispatch

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Repository dispatch | `POST /api/v3/repos/{owner}/{repo}/dispatches` | ✓ `gh_actions_extras.go::handleRepositoryDispatch` | ✓ `gh_actions_test.go` | |
| Workflow dispatch | `POST /api/v3/repos/{owner}/{repo}/actions/workflows/{workflow_id}/dispatches` | ✓ `gh_workflows_rest.go::handleDispatchWorkflow` | ✓ `gh_workflows_test.go` | |

## Workflows

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| List workflows | `GET /api/v3/repos/{owner}/{repo}/actions/workflows` | ✓ `handleListGHWorkflows` | ✓ `gh_workflows_test.go` | |
| Get workflow | `GET /api/v3/repos/{owner}/{repo}/actions/workflows/{workflow_id}` | ✓ `handleGetGHWorkflow` | ✓ same | |
| List workflow file runs | `GET /api/v3/repos/{owner}/{repo}/actions/workflows/{workflow_id}/runs` | ✓ `handleListWorkflowFileRuns` | ✓ same | |
