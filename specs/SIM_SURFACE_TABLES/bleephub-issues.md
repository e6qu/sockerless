# bleephub — Issues

Surface: `bleephub/gh_issues_rest.go`, `bleephub/gh_labels_rest.go`, `bleephub/gh_reactions.go`, `bleephub/gh_issue_moderation.go`.

Canonical reference: <https://docs.github.com/en/enterprise-server/rest/issues>

## Status legend

- ✓ — implemented + tested
- ✗ — implemented, no direct test coverage

## Issues

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create issue | `POST /api/v3/repos/{owner}/{repo}/issues` | ✓ `gh_labels_rest.go::handleCreateIssue` | ✓ `gh_issues_test.go` | |
| List issues | `GET /api/v3/repos/{owner}/{repo}/issues` | ✓ `handleListIssues` | ✓ same | |
| Get issue | `GET /api/v3/repos/{owner}/{repo}/issues/{number}` | ✓ `handleGetIssue` | ✓ same | |
| Update issue | `PATCH /api/v3/repos/{owner}/{repo}/issues/{number}` | ✓ `handleUpdateIssue` | ✓ same | |
| Lock issue | `PUT /api/v3/repos/{owner}/{repo}/issues/{number}/lock` | ✓ `handleLockIssue` | ✓ same | |
| Issue delete dispatch | `DELETE /api/v3/repos/{owner}/{repo}/issues/{p1}/{p2}` | ✓ `handleIssuesDeleteDispatch` | ✓ same | Routes label/reaction deletes. |

## Issue comments

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create comment | `POST /api/v3/repos/{owner}/{repo}/issues/{number}/comments` | ✓ `handleCreateIssueComment` | ✓ `gh_issues_test.go` | |
| List comments | `GET /api/v3/repos/{owner}/{repo}/issues/{number}/comments` | ✓ `handleListIssueComments` | ✓ same | |
| Update comment | `PATCH /api/v3/repos/{owner}/{repo}/issues/comments/{comment_id}` | ✓ `handleUpdateIssueComment` | ✓ same | |

## Labels

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create label | `POST /api/v3/repos/{owner}/{repo}/labels` | ✓ `handleCreateLabel` | ✓ `gh_issues_test.go` | |
| List labels | `GET /api/v3/repos/{owner}/{repo}/labels` | ✓ `handleListLabels` | ✓ same | |
| Get label | `GET /api/v3/repos/{owner}/{repo}/labels/{name}` | ✓ `handleGetLabel` | ✓ same | |
| Update label | `PATCH /api/v3/repos/{owner}/{repo}/labels/{name}` | ✓ `handleUpdateLabel` | ✓ same | |
| Delete label | `DELETE /api/v3/repos/{owner}/{repo}/labels/{name}` | ✓ `handleDeleteLabel` | ✓ same | |
| Add issue labels | `POST /api/v3/repos/{owner}/{repo}/issues/{number}/labels` | ✓ `handleAddIssueLabels` | ✓ same | |
| Remove issue label | `DELETE /api/v3/repos/{owner}/{repo}/issues/{number}/labels/{name}` | ✓ `handleRemoveIssueLabel` | ✓ same | |

## Milestones

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create milestone | `POST /api/v3/repos/{owner}/{repo}/milestones` | ✓ `handleCreateMilestone` | ✓ `gh_issues_test.go` | |
| List milestones | `GET /api/v3/repos/{owner}/{repo}/milestones` | ✓ `handleListMilestones` | ✓ same | |
| Get milestone | `GET /api/v3/repos/{owner}/{repo}/milestones/{number}` | ✓ `handleGetMilestone` | ✓ same | |
| Update milestone | `PATCH /api/v3/repos/{owner}/{repo}/milestones/{number}` | ✓ `handleUpdateMilestone` | ✓ same | |
| Delete milestone | `DELETE /api/v3/repos/{owner}/{repo}/milestones/{number}` | ✓ `handleDeleteMilestone` | ✓ same | |

## Reactions

| Operation | Verb + path | sim handler | test | notes |
|---|---|---|---|---|
| Create issue reaction | `POST /api/v3/repos/{owner}/{repo}/issues/{number}/reactions` | ✓ `gh_reactions.go::handleCreateIssueReaction` | ✓ `gh_reactions_test.go` | |
| List issue reactions | `GET /api/v3/repos/{owner}/{repo}/issues/{number}/reactions` | ✓ `handleListIssueReactions` | ✓ same | |
| Delete issue reaction | `DELETE /api/v3/repos/{owner}/{repo}/issues/{number}/reactions/{reaction_id}` | ✓ `handleDeleteIssueReaction` | ✓ same | |
| Create comment reaction | `POST /api/v3/repos/{owner}/{repo}/issues/comments/{comment_id}/reactions` | ✓ `handleCreateIssueCommentReaction` | ✓ same | |
| List comment reactions | `GET /api/v3/repos/{owner}/{repo}/issues/comments/{comment_id}/reactions` | ✓ `handleListIssueCommentReactions` | ✓ same | |
| Delete comment reaction | `DELETE /api/v3/repos/{owner}/{repo}/issues/comments/{comment_id}/reactions/{reaction_id}` | ✓ `handleDeleteIssueCommentReaction` | ✓ same | |
