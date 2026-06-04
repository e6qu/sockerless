# bleephub — Pull Requests

Surface: `bleephub/gh_pulls_rest.go`, `bleephub/gh_pr_comments.go`, `bleephub/gh_pr_threads.go`.

Canonical reference: <https://docs.github.com/en/enterprise-server/rest/pulls>

## Status legend

- ✓ — implemented + tested
- ✗ — implemented, no direct test coverage

## Pull request CRUD

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create PR | `POST /api/v3/repos/{owner}/{repo}/pulls` | ✓ `gh_pulls_rest.go::handleCreatePullRequest` | ✓ `gh_pulls_test.go` | |
| List PRs | `GET /api/v3/repos/{owner}/{repo}/pulls` | ✓ `handleListPullRequests` | ✓ same | |
| Get PR | `GET /api/v3/repos/{owner}/{repo}/pulls/{number}` | ✓ `handleGetPullRequest` | ✓ same | |
| Update PR | `PATCH /api/v3/repos/{owner}/{repo}/pulls/{number}` | ✓ `handleUpdatePullRequest` | ✓ same | |
| Merge PR | `PUT /api/v3/repos/{owner}/{repo}/pulls/{number}/merge` | ✓ `handleMergePullRequest` | ✓ same | |

## PR reviews

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create review | `POST /api/v3/repos/{owner}/{repo}/pulls/{number}/reviews` | ✓ `handleCreatePRReview` | ✓ `gh_pulls_test.go` | |
| List reviews | `GET /api/v3/repos/{owner}/{repo}/pulls/{number}/reviews` | ✓ `handleListPRReviews` | ✓ same | |
| Request reviewers | `POST /api/v3/repos/{owner}/{repo}/pulls/{number}/requested_reviewers` | ✓ `handleRequestReviewers` | ✓ same | |

## PR comments & review threads

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create PR comment | `POST /api/v3/repos/{owner}/{repo}/pulls/{number}/comments` | ✓ `gh_pr_comments.go::handleCreatePRComment` | ✓ `gh_pr_comments_test.go` | Inline diff comments. |
| List PR comments | `GET /api/v3/repos/{owner}/{repo}/pulls/{number}/comments` | ✓ `handleListPRComments` | ✓ same | |
| Reply to PR comment | `POST /api/v3/repos/{owner}/{repo}/pulls/{number}/comments/{comment_id}/replies` | ✓ `handleReplyToPRComment` | ✓ same | |
| List review threads | `GET /api/v3/repos/{owner}/{repo}/pulls/{number}/review-threads` | ✓ `handleListReviewThreads` | ✓ same | |
| Get PR comment (dispatch) | `GET /api/v3/repos/{owner}/{repo}/pulls/{p1}/{p2}` | ✓ `handlePRCommentDispatch` | ✓ same | Routes sub-resource GET. |
| Update PR comment (dispatch) | `PATCH /api/v3/repos/{owner}/{repo}/pulls/{p1}/{p2}` | ✓ `handlePRCommentUpdateDispatch` | ✓ same | |
| Delete PR comment (dispatch) | `DELETE /api/v3/repos/{owner}/{repo}/pulls/{p1}/{p2}` | ✓ `handlePRCommentDeleteDispatch` | ✓ same | |
