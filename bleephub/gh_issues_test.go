package bleephub

import (
	"testing"
)

// helper: create a repo for issue tests, returns owner/name.
func createTestIssueRepo(t *testing.T, name string) {
	t.Helper()
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": name,
	})
	resp.Body.Close()
}

// --- Label tests ---

func TestCreateLabel(t *testing.T) {
	createTestIssueRepo(t, "label-test")

	resp := ghPost(t, "/api/v3/repos/admin/label-test/labels", defaultToken, map[string]interface{}{
		"name":        "bug",
		"color":       "d73a4a",
		"description": "Something is broken",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	if data["name"] != "bug" {
		t.Fatalf("expected name=bug, got %v", data["name"])
	}
	if data["color"] != "d73a4a" {
		t.Fatalf("expected color=d73a4a, got %v", data["color"])
	}
	if data["description"] != "Something is broken" {
		t.Fatalf("expected description='Something is broken', got %v", data["description"])
	}
}

func TestListLabels(t *testing.T) {
	createTestIssueRepo(t, "label-list")
	ghPost(t, "/api/v3/repos/admin/label-list/labels", defaultToken, map[string]interface{}{
		"name": "enhancement", "color": "a2eeef",
	}).Body.Close()

	resp := ghGet(t, "/api/v3/repos/admin/label-list/labels", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	labels := decodeJSONArray(t, resp)
	if len(labels) == 0 {
		t.Fatal("expected at least 1 label")
	}
}

func TestGetLabel(t *testing.T) {
	createTestIssueRepo(t, "label-get")
	ghPost(t, "/api/v3/repos/admin/label-get/labels", defaultToken, map[string]interface{}{
		"name": "docs", "color": "0075ca",
	}).Body.Close()

	resp := ghGet(t, "/api/v3/repos/admin/label-get/labels/docs", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["name"] != "docs" {
		t.Fatalf("expected name=docs, got %v", data["name"])
	}
}

func TestUpdateLabel(t *testing.T) {
	createTestIssueRepo(t, "label-update")
	ghPost(t, "/api/v3/repos/admin/label-update/labels", defaultToken, map[string]interface{}{
		"name": "wontfix", "color": "ffffff",
	}).Body.Close()

	resp := ghPatch(t, "/api/v3/repos/admin/label-update/labels/wontfix", defaultToken, map[string]interface{}{
		"color": "000000",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["color"] != "000000" {
		t.Fatalf("expected color=000000, got %v", data["color"])
	}
}

func TestDeleteLabel(t *testing.T) {
	createTestIssueRepo(t, "label-delete")
	ghPost(t, "/api/v3/repos/admin/label-delete/labels", defaultToken, map[string]interface{}{
		"name": "temp", "color": "aaaaaa",
	}).Body.Close()

	resp := ghDelete(t, "/api/v3/repos/admin/label-delete/labels/temp", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	resp2 := ghGet(t, "/api/v3/repos/admin/label-delete/labels/temp", "")
	defer resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Fatalf("expected 404 after delete, got %d", resp2.StatusCode)
	}
}

// --- Milestone tests ---

func TestCreateMilestone(t *testing.T) {
	createTestIssueRepo(t, "ms-test")

	resp := ghPost(t, "/api/v3/repos/admin/ms-test/milestones", defaultToken, map[string]interface{}{
		"title":       "v1.0",
		"description": "First release",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	if data["title"] != "v1.0" {
		t.Fatalf("expected title=v1.0, got %v", data["title"])
	}
	if data["number"] != 1.0 {
		t.Fatalf("expected number=1, got %v", data["number"])
	}
	if data["state"] != "open" {
		t.Fatalf("expected state=open, got %v", data["state"])
	}
}

func TestListMilestones(t *testing.T) {
	createTestIssueRepo(t, "ms-list")
	ghPost(t, "/api/v3/repos/admin/ms-list/milestones", defaultToken, map[string]interface{}{
		"title": "v2.0",
	}).Body.Close()

	resp := ghGet(t, "/api/v3/repos/admin/ms-list/milestones", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	milestones := decodeJSONArray(t, resp)
	if len(milestones) == 0 {
		t.Fatal("expected at least 1 milestone")
	}
}

func TestGetMilestone(t *testing.T) {
	createTestIssueRepo(t, "ms-get")
	ghPost(t, "/api/v3/repos/admin/ms-get/milestones", defaultToken, map[string]interface{}{
		"title": "v3.0",
	}).Body.Close()

	resp := ghGet(t, "/api/v3/repos/admin/ms-get/milestones/1", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["title"] != "v3.0" {
		t.Fatalf("expected title=v3.0, got %v", data["title"])
	}
}

func TestUpdateMilestone(t *testing.T) {
	createTestIssueRepo(t, "ms-update")
	ghPost(t, "/api/v3/repos/admin/ms-update/milestones", defaultToken, map[string]interface{}{
		"title": "v4.0",
	}).Body.Close()

	resp := ghPatch(t, "/api/v3/repos/admin/ms-update/milestones/1", defaultToken, map[string]interface{}{
		"state": "closed",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["state"] != "closed" {
		t.Fatalf("expected state=closed, got %v", data["state"])
	}
}

func TestDeleteMilestone(t *testing.T) {
	createTestIssueRepo(t, "ms-delete")
	ghPost(t, "/api/v3/repos/admin/ms-delete/milestones", defaultToken, map[string]interface{}{
		"title": "temp-ms",
	}).Body.Close()

	resp := ghDelete(t, "/api/v3/repos/admin/ms-delete/milestones/1", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	resp2 := ghGet(t, "/api/v3/repos/admin/ms-delete/milestones/1", "")
	defer resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Fatalf("expected 404 after delete, got %d", resp2.StatusCode)
	}
}

// --- Issue REST tests ---

func TestCreateIssueREST(t *testing.T) {
	createTestIssueRepo(t, "issue-create")

	resp := ghPost(t, "/api/v3/repos/admin/issue-create/issues", defaultToken, map[string]interface{}{
		"title": "First issue",
		"body":  "This is a test",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	if data["title"] != "First issue" {
		t.Fatalf("expected title='First issue', got %v", data["title"])
	}
	if data["number"] != 1.0 {
		t.Fatalf("expected number=1, got %v", data["number"])
	}
	if data["state"] != "open" {
		t.Fatalf("expected state=open, got %v", data["state"])
	}
	if data["user"] == nil {
		t.Fatal("missing user")
	}
}

func TestListIssuesREST(t *testing.T) {
	createTestIssueRepo(t, "issue-list")
	ghPost(t, "/api/v3/repos/admin/issue-list/issues", defaultToken, map[string]interface{}{
		"title": "List test issue",
	}).Body.Close()

	resp := ghGet(t, "/api/v3/repos/admin/issue-list/issues", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	issues := decodeJSONArray(t, resp)
	if len(issues) == 0 {
		t.Fatal("expected at least 1 issue")
	}
}

func TestGetIssueREST(t *testing.T) {
	createTestIssueRepo(t, "issue-get")
	ghPost(t, "/api/v3/repos/admin/issue-get/issues", defaultToken, map[string]interface{}{
		"title": "Get test",
	}).Body.Close()

	resp := ghGet(t, "/api/v3/repos/admin/issue-get/issues/1", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["title"] != "Get test" {
		t.Fatalf("expected title='Get test', got %v", data["title"])
	}
}

func TestUpdateIssueREST(t *testing.T) {
	createTestIssueRepo(t, "issue-update")
	ghPost(t, "/api/v3/repos/admin/issue-update/issues", defaultToken, map[string]interface{}{
		"title": "Update test",
	}).Body.Close()

	resp := ghPatch(t, "/api/v3/repos/admin/issue-update/issues/1", defaultToken, map[string]interface{}{
		"state": "closed",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["state"] != "closed" {
		t.Fatalf("expected state=closed, got %v", data["state"])
	}
}

// --- Comment REST tests ---

func TestCreateCommentREST(t *testing.T) {
	createTestIssueRepo(t, "comment-create")
	ghPost(t, "/api/v3/repos/admin/comment-create/issues", defaultToken, map[string]interface{}{
		"title": "Comment test",
	}).Body.Close()

	resp := ghPost(t, "/api/v3/repos/admin/comment-create/issues/1/comments", defaultToken, map[string]interface{}{
		"body": "A comment",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["body"] != "A comment" {
		t.Fatalf("expected body='A comment', got %v", data["body"])
	}
	if data["user"] == nil {
		t.Fatal("missing user in comment")
	}
}

func TestListCommentsREST(t *testing.T) {
	createTestIssueRepo(t, "comment-list")
	ghPost(t, "/api/v3/repos/admin/comment-list/issues", defaultToken, map[string]interface{}{
		"title": "Comment list test",
	}).Body.Close()
	ghPost(t, "/api/v3/repos/admin/comment-list/issues/1/comments", defaultToken, map[string]interface{}{
		"body": "Comment 1",
	}).Body.Close()

	resp := ghGet(t, "/api/v3/repos/admin/comment-list/issues/1/comments", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	comments := decodeJSONArray(t, resp)
	if len(comments) == 0 {
		t.Fatal("expected at least 1 comment")
	}
}

// --- Issue label management tests ---

func TestAddIssueLabelsREST(t *testing.T) {
	createTestIssueRepo(t, "issue-addlabels")
	ghPost(t, "/api/v3/repos/admin/issue-addlabels/labels", defaultToken, map[string]interface{}{
		"name": "bug", "color": "d73a4a",
	}).Body.Close()
	ghPost(t, "/api/v3/repos/admin/issue-addlabels/issues", defaultToken, map[string]interface{}{
		"title": "Label test",
	}).Body.Close()

	resp := ghPost(t, "/api/v3/repos/admin/issue-addlabels/issues/1/labels", defaultToken, map[string]interface{}{
		"labels": []string{"bug"},
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	labels := decodeJSONArray(t, resp)
	if len(labels) == 0 {
		t.Fatal("expected at least 1 label")
	}
	if labels[0]["name"] != "bug" {
		t.Fatalf("expected label name=bug, got %v", labels[0]["name"])
	}
}

func TestRemoveIssueLabelREST(t *testing.T) {
	createTestIssueRepo(t, "issue-rmlabel")
	ghPost(t, "/api/v3/repos/admin/issue-rmlabel/labels", defaultToken, map[string]interface{}{
		"name": "remove-me", "color": "ffffff",
	}).Body.Close()
	ghPost(t, "/api/v3/repos/admin/issue-rmlabel/issues", defaultToken, map[string]interface{}{
		"title": "Remove label test",
	}).Body.Close()
	ghPost(t, "/api/v3/repos/admin/issue-rmlabel/issues/1/labels", defaultToken, map[string]interface{}{
		"labels": []string{"remove-me"},
	}).Body.Close()

	resp := ghDelete(t, "/api/v3/repos/admin/issue-rmlabel/issues/1/labels/remove-me", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

// --- GraphQL tests ---

func TestGraphQLCreateIssue(t *testing.T) {
	// Create repo and get its node ID
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "gql-issue-create",
	})
	repoData := decodeJSON(t, resp)
	repoNodeID := repoData["node_id"].(string)

	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: CreateIssueInput!) { createIssue(input: $input) { issue { number title state url } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"repositoryId": repoNodeID,
				"title":        "GQL Issue",
				"body":         "Created via GraphQL",
			},
		},
	})
	if resp2.StatusCode != 200 {
		resp2.Body.Close()
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	data := decodeJSON(t, resp2)

	d, _ := data["data"].(map[string]interface{})
	if d == nil {
		t.Fatalf("expected data, got errors: %v", data)
	}
	payload, _ := d["createIssue"].(map[string]interface{})
	issue, _ := payload["issue"].(map[string]interface{})
	if issue == nil {
		t.Fatalf("expected issue in payload: %v", data)
	}
	if issue["title"] != "GQL Issue" {
		t.Fatalf("expected title='GQL Issue', got %v", issue["title"])
	}
	if issue["number"] != 1.0 {
		t.Fatalf("expected number=1, got %v", issue["number"])
	}
	if issue["state"] != "OPEN" {
		t.Fatalf("expected state=OPEN, got %v", issue["state"])
	}
}

func TestGraphQLCloseIssue(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "gql-issue-close",
	})
	repoData := decodeJSON(t, resp)
	repoNodeID := repoData["node_id"].(string)

	// Create issue
	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: CreateIssueInput!) { createIssue(input: $input) { issue { id } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"repositoryId": repoNodeID,
				"title":        "To close",
			},
		},
	})
	data2 := decodeJSON(t, resp2)
	d2, _ := data2["data"].(map[string]interface{})
	ci, _ := d2["createIssue"].(map[string]interface{})
	iss, _ := ci["issue"].(map[string]interface{})
	issueID := iss["id"].(string)

	// Close
	resp3 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: CloseIssueInput!) { closeIssue(input: $input) { issue { state stateReason } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"issueId": issueID,
			},
		},
	})
	if resp3.StatusCode != 200 {
		resp3.Body.Close()
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}
	data := decodeJSON(t, resp3)

	d, _ := data["data"].(map[string]interface{})
	if d == nil {
		t.Fatalf("expected data, got errors: %v", data)
	}
	cl, _ := d["closeIssue"].(map[string]interface{})
	issue, _ := cl["issue"].(map[string]interface{})
	if issue["state"] != "CLOSED" {
		t.Fatalf("expected state=CLOSED, got %v", issue["state"])
	}
	if issue["stateReason"] != "COMPLETED" {
		t.Fatalf("expected stateReason=COMPLETED, got %v", issue["stateReason"])
	}
}

func TestGraphQLReopenIssue(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "gql-issue-reopen",
	})
	repoData := decodeJSON(t, resp)
	repoNodeID := repoData["node_id"].(string)

	// Create and close
	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: CreateIssueInput!) { createIssue(input: $input) { issue { id } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"repositoryId": repoNodeID,
				"title":        "To reopen",
			},
		},
	})
	data2 := decodeJSON(t, resp2)
	d2, _ := data2["data"].(map[string]interface{})
	ci, _ := d2["createIssue"].(map[string]interface{})
	iss, _ := ci["issue"].(map[string]interface{})
	issueID := iss["id"].(string)

	ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: CloseIssueInput!) { closeIssue(input: $input) { issue { state } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{"issueId": issueID},
		},
	}).Body.Close()

	// Reopen
	resp3 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: ReopenIssueInput!) { reopenIssue(input: $input) { issue { state } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{"issueId": issueID},
		},
	})
	data := decodeJSON(t, resp3)
	d, _ := data["data"].(map[string]interface{})
	ro, _ := d["reopenIssue"].(map[string]interface{})
	issue, _ := ro["issue"].(map[string]interface{})
	if issue["state"] != "OPEN" {
		t.Fatalf("expected state=OPEN, got %v", issue["state"])
	}
}

func TestGraphQLAddComment(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "gql-comment",
	})
	repoData := decodeJSON(t, resp)
	repoNodeID := repoData["node_id"].(string)

	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: CreateIssueInput!) { createIssue(input: $input) { issue { id } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"repositoryId": repoNodeID,
				"title":        "Comment target",
			},
		},
	})
	data2 := decodeJSON(t, resp2)
	d2, _ := data2["data"].(map[string]interface{})
	ci, _ := d2["createIssue"].(map[string]interface{})
	iss, _ := ci["issue"].(map[string]interface{})
	issueID := iss["id"].(string)

	resp3 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: AddCommentInput!) { addComment(input: $input) { commentEdge { node { body } } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"subjectId": issueID,
				"body":      "GQL comment",
			},
		},
	})
	if resp3.StatusCode != 200 {
		resp3.Body.Close()
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}
	data := decodeJSON(t, resp3)
	d, _ := data["data"].(map[string]interface{})
	if d == nil {
		t.Fatalf("expected data, got errors: %v", data)
	}
	ac, _ := d["addComment"].(map[string]interface{})
	edge, _ := ac["commentEdge"].(map[string]interface{})
	node, _ := edge["node"].(map[string]interface{})
	if node["body"] != "GQL comment" {
		t.Fatalf("expected body='GQL comment', got %v", node["body"])
	}
}

func TestGraphQLListIssues(t *testing.T) {
	createTestIssueRepo(t, "gql-issue-list")

	ghPost(t, "/api/v3/repos/admin/gql-issue-list/issues", defaultToken, map[string]interface{}{
		"title": "GQL list issue 1",
	}).Body.Close()
	ghPost(t, "/api/v3/repos/admin/gql-issue-list/issues", defaultToken, map[string]interface{}{
		"title": "GQL list issue 2",
	}).Body.Close()

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"gql-issue-list"){issues(first:10,states:[OPEN]){totalCount,nodes{number,title,state}}}}`,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	issues, _ := repo["issues"].(map[string]interface{})
	if issues == nil {
		t.Fatalf("expected issues in repository: %v", data)
	}
	tc, _ := issues["totalCount"].(float64)
	if tc < 2 {
		t.Fatalf("expected totalCount >= 2, got %v", tc)
	}
	nodes, _ := issues["nodes"].([]interface{})
	if len(nodes) < 2 {
		t.Fatalf("expected >= 2 nodes, got %d", len(nodes))
	}
}

func TestGraphQLGetIssue(t *testing.T) {
	createTestIssueRepo(t, "gql-issue-get")

	ghPost(t, "/api/v3/repos/admin/gql-issue-get/issues", defaultToken, map[string]interface{}{
		"title": "GQL get issue",
		"body":  "Get by number",
	}).Body.Close()

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"gql-issue-get"){issue(number:1){title,body,state,author{login},labels(first:10){nodes{name}},comments(first:10){nodes{body}}}}}`,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	issue, _ := repo["issue"].(map[string]interface{})
	if issue == nil {
		t.Fatalf("expected issue: %v", data)
	}
	if issue["title"] != "GQL get issue" {
		t.Fatalf("expected title='GQL get issue', got %v", issue["title"])
	}
	if issue["body"] != "Get by number" {
		t.Fatalf("expected body='Get by number', got %v", issue["body"])
	}
	author, _ := issue["author"].(map[string]interface{})
	if author == nil || author["login"] != "admin" {
		t.Fatalf("expected author.login=admin, got %v", author)
	}
}

func TestGraphQLRepoLabels(t *testing.T) {
	createTestIssueRepo(t, "gql-labels")
	ghPost(t, "/api/v3/repos/admin/gql-labels/labels", defaultToken, map[string]interface{}{
		"name": "gql-bug", "color": "d73a4a",
	}).Body.Close()

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"gql-labels"){labels(first:10){totalCount,nodes{name,color}}}}`,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	labels, _ := repo["labels"].(map[string]interface{})
	if labels == nil {
		t.Fatalf("expected labels: %v", data)
	}
	tc, _ := labels["totalCount"].(float64)
	if tc < 1 {
		t.Fatalf("expected totalCount >= 1, got %v", tc)
	}
}

func TestGraphQLRepoMilestones(t *testing.T) {
	createTestIssueRepo(t, "gql-milestones")
	ghPost(t, "/api/v3/repos/admin/gql-milestones/milestones", defaultToken, map[string]interface{}{
		"title": "gql-v1",
	}).Body.Close()

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"gql-milestones"){milestones(first:10){totalCount,nodes{title,number,state}}}}`,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	milestones, _ := repo["milestones"].(map[string]interface{})
	if milestones == nil {
		t.Fatalf("expected milestones: %v", data)
	}
	tc, _ := milestones["totalCount"].(float64)
	if tc < 1 {
		t.Fatalf("expected totalCount >= 1, got %v", tc)
	}
}

func TestGraphQLUpdateIssue(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "gql-issue-update",
	})
	repoData := decodeJSON(t, resp)
	repoNodeID := repoData["node_id"].(string)

	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: CreateIssueInput!) { createIssue(input: $input) { issue { id } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"repositoryId": repoNodeID,
				"title":        "Before update",
			},
		},
	})
	data2 := decodeJSON(t, resp2)
	d2, _ := data2["data"].(map[string]interface{})
	ci, _ := d2["createIssue"].(map[string]interface{})
	iss, _ := ci["issue"].(map[string]interface{})
	issueID := iss["id"].(string)

	resp3 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: UpdateIssueInput!) { updateIssue(input: $input) { issue { title } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"id":    issueID,
				"title": "After update",
			},
		},
	})
	if resp3.StatusCode != 200 {
		resp3.Body.Close()
		t.Fatalf("expected 200, got %d", resp3.StatusCode)
	}
	data := decodeJSON(t, resp3)
	d, _ := data["data"].(map[string]interface{})
	if d == nil {
		t.Fatalf("expected data, got errors: %v", data)
	}
	ui, _ := d["updateIssue"].(map[string]interface{})
	issue, _ := ui["issue"].(map[string]interface{})
	if issue["title"] != "After update" {
		t.Fatalf("expected title='After update', got %v", issue["title"])
	}
}

func TestGraphQLHasIssuesEnabled(t *testing.T) {
	createTestIssueRepo(t, "gql-has-issues")

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"gql-has-issues"){hasIssuesEnabled,viewerPermission}}`,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	if repo["hasIssuesEnabled"] != true {
		t.Fatalf("expected hasIssuesEnabled=true, got %v", repo["hasIssuesEnabled"])
	}
	if repo["viewerPermission"] != "ADMIN" {
		t.Fatalf("expected viewerPermission=ADMIN, got %v", repo["viewerPermission"])
	}
}
