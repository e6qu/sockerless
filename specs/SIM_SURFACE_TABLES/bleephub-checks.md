# bleephub — Checks

Surface: `bleephub/gh_checks_rest.go`.

Canonical reference: <https://docs.github.com/en/enterprise-server/rest/checks>

## Status legend

- ✓ — implemented + tested
- ✗ — implemented, no direct test coverage

## Check runs

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create check run | `POST /api/v3/repos/{owner}/{repo}/check-runs` | ✓ `gh_checks_rest.go::handleCreateCheckRun` | ✓ `gh_checks_test.go` | |
| Get check run | `GET /api/v3/repos/{owner}/{repo}/check-runs/{id}` | ✓ `handleGetCheckRun` | ✓ same | |
| Update check run | `PATCH /api/v3/repos/{owner}/{repo}/check-runs/{id}` | ✓ `handleUpdateCheckRun` | ✓ same | |
| List annotations | `GET /api/v3/repos/{owner}/{repo}/check-runs/{id}/annotations` | ✓ `handleListCheckRunAnnotations` | ✓ same | |
| List check runs for commit | `GET /api/v3/repos/{owner}/{repo}/commits/{sha}/check-runs` | ✓ `handleListCheckRunsForCommit` | ✓ same | |

## Check suites

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create check suite | `POST /api/v3/repos/{owner}/{repo}/check-suites` | ✓ `handleCreateCheckSuite` | ✓ `gh_checks_test.go` | |
| Get check suite | `GET /api/v3/repos/{owner}/{repo}/check-suites/{id}` | ✓ `handleGetCheckSuite` | ✓ same | |
| List check suites for commit | `GET /api/v3/repos/{owner}/{repo}/commits/{sha}/check-suites` | ✓ `handleListCheckSuitesForCommit` | ✓ same | |
| List check runs for suite | `GET /api/v3/repos/{owner}/{repo}/check-suites/{id}/check-runs` | ✓ `handleListCheckRunsForSuite` | ✓ same | |
| Update check suite preferences | `PATCH /api/v3/repos/{owner}/{repo}/check-suites/preferences` | ✓ `handleUpdateCheckSuitePrefs` | ✓ same | |
