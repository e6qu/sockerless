package bleephub

import (
	"testing"
)

func createTestPRRepo(t *testing.T, name string) {
	t.Helper()
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": name,
	})
	resp.Body.Close()
}

// --- REST tests ---

func TestCreatePullRequestREST(t *testing.T) {
	createTestPRRepo(t, "pr-create")

	resp := ghPost(t, "/api/v3/repos/admin/pr-create/pulls", defaultToken, map[string]interface{}{
		"title": "First PR",
		"body":  "PR body",
		"head":  "feature",
		"base":  "main",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	if data["title"] != "First PR" {
		t.Fatalf("expected title='First PR', got %v", data["title"])
	}
	if data["number"] != 1.0 {
		t.Fatalf("expected number=1, got %v", data["number"])
	}
	if data["state"] != "open" {
		t.Fatalf("expected state=open, got %v", data["state"])
	}
	head, _ := data["head"].(map[string]interface{})
	if head == nil || head["ref"] != "feature" {
		t.Fatalf("expected head.ref=feature, got %v", head)
	}
	base, _ := data["base"].(map[string]interface{})
	if base == nil || base["ref"] != "main" {
		t.Fatalf("expected base.ref=main, got %v", base)
	}
	if data["user"] == nil {
		t.Fatal("missing user")
	}
}

func TestListPullRequestsREST(t *testing.T) {
	createTestPRRepo(t, "pr-list")
	ghPost(t, "/api/v3/repos/admin/pr-list/pulls", defaultToken, map[string]interface{}{
		"title": "List PR", "head": "feat", "base": "main",
	}).Body.Close()

	resp := ghGet(t, "/api/v3/repos/admin/pr-list/pulls", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	prs := decodeJSONArray(t, resp)
	if len(prs) == 0 {
		t.Fatal("expected at least 1 PR")
	}
}

func TestListPullRequestsFilterState(t *testing.T) {
	createTestPRRepo(t, "pr-filter")
	ghPost(t, "/api/v3/repos/admin/pr-filter/pulls", defaultToken, map[string]interface{}{
		"title": "Open PR", "head": "feat", "base": "main",
	}).Body.Close()

	resp := ghGet(t, "/api/v3/repos/admin/pr-filter/pulls?state=closed", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	prs := decodeJSONArray(t, resp)
	if len(prs) != 0 {
		t.Fatalf("expected 0 closed PRs, got %d", len(prs))
	}
}

func TestGetPullRequestREST(t *testing.T) {
	createTestPRRepo(t, "pr-get")
	ghPost(t, "/api/v3/repos/admin/pr-get/pulls", defaultToken, map[string]interface{}{
		"title": "Get PR", "head": "feat", "base": "main",
	}).Body.Close()

	resp := ghGet(t, "/api/v3/repos/admin/pr-get/pulls/1", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["title"] != "Get PR" {
		t.Fatalf("expected title='Get PR', got %v", data["title"])
	}
}

func TestGetPullRequestNotFound(t *testing.T) {
	createTestPRRepo(t, "pr-notfound")

	resp := ghGet(t, "/api/v3/repos/admin/pr-notfound/pulls/999", "")
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestUpdatePullRequestREST(t *testing.T) {
	createTestPRRepo(t, "pr-update")
	ghPost(t, "/api/v3/repos/admin/pr-update/pulls", defaultToken, map[string]interface{}{
		"title": "Before", "head": "feat", "base": "main",
	}).Body.Close()

	resp := ghPatch(t, "/api/v3/repos/admin/pr-update/pulls/1", defaultToken, map[string]interface{}{
		"title": "After",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["title"] != "After" {
		t.Fatalf("expected title='After', got %v", data["title"])
	}
}

func TestClosePullRequestREST(t *testing.T) {
	createTestPRRepo(t, "pr-close")
	ghPost(t, "/api/v3/repos/admin/pr-close/pulls", defaultToken, map[string]interface{}{
		"title": "To close", "head": "feat", "base": "main",
	}).Body.Close()

	resp := ghPatch(t, "/api/v3/repos/admin/pr-close/pulls/1", defaultToken, map[string]interface{}{
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
	if data["closed_at"] == nil {
		t.Fatal("expected closed_at to be set")
	}
}

func TestReopenPullRequestREST(t *testing.T) {
	createTestPRRepo(t, "pr-reopen")
	ghPost(t, "/api/v3/repos/admin/pr-reopen/pulls", defaultToken, map[string]interface{}{
		"title": "To reopen", "head": "feat", "base": "main",
	}).Body.Close()

	ghPatch(t, "/api/v3/repos/admin/pr-reopen/pulls/1", defaultToken, map[string]interface{}{
		"state": "closed",
	}).Body.Close()

	resp := ghPatch(t, "/api/v3/repos/admin/pr-reopen/pulls/1", defaultToken, map[string]interface{}{
		"state": "open",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["state"] != "open" {
		t.Fatalf("expected state=open, got %v", data["state"])
	}
}

func TestMergePullRequestREST(t *testing.T) {
	createTestPRRepo(t, "pr-merge")
	ghPost(t, "/api/v3/repos/admin/pr-merge/pulls", defaultToken, map[string]interface{}{
		"title": "To merge", "head": "feat", "base": "main",
	}).Body.Close()

	resp := ghPut(t, "/api/v3/repos/admin/pr-merge/pulls/1/merge", defaultToken, map[string]interface{}{
		"merge_method": "merge",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["merged"] != true {
		t.Fatalf("expected merged=true, got %v", data["merged"])
	}
	if data["sha"] == nil || data["sha"] == "" {
		t.Fatal("expected sha to be present")
	}
}

func TestMergeAlreadyMerged(t *testing.T) {
	createTestPRRepo(t, "pr-double-merge")
	ghPost(t, "/api/v3/repos/admin/pr-double-merge/pulls", defaultToken, map[string]interface{}{
		"title": "Double merge", "head": "feat", "base": "main",
	}).Body.Close()

	ghPut(t, "/api/v3/repos/admin/pr-double-merge/pulls/1/merge", defaultToken, map[string]interface{}{}).Body.Close()

	resp := ghPut(t, "/api/v3/repos/admin/pr-double-merge/pulls/1/merge", defaultToken, map[string]interface{}{})
	defer resp.Body.Close()
	if resp.StatusCode != 405 {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func TestCreatePRReviewREST(t *testing.T) {
	createTestPRRepo(t, "pr-review")
	ghPost(t, "/api/v3/repos/admin/pr-review/pulls", defaultToken, map[string]interface{}{
		"title": "Review PR", "head": "feat", "base": "main",
	}).Body.Close()

	resp := ghPost(t, "/api/v3/repos/admin/pr-review/pulls/1/reviews", defaultToken, map[string]interface{}{
		"body":  "LGTM",
		"event": "APPROVE",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["state"] != "APPROVED" {
		t.Fatalf("expected state=APPROVED, got %v", data["state"])
	}
	if data["body"] != "LGTM" {
		t.Fatalf("expected body='LGTM', got %v", data["body"])
	}
}

func TestListPRReviewsREST(t *testing.T) {
	createTestPRRepo(t, "pr-reviews-list")
	ghPost(t, "/api/v3/repos/admin/pr-reviews-list/pulls", defaultToken, map[string]interface{}{
		"title": "Reviews list", "head": "feat", "base": "main",
	}).Body.Close()
	ghPost(t, "/api/v3/repos/admin/pr-reviews-list/pulls/1/reviews", defaultToken, map[string]interface{}{
		"body": "OK", "event": "APPROVE",
	}).Body.Close()

	resp := ghGet(t, "/api/v3/repos/admin/pr-reviews-list/pulls/1/reviews", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	reviews := decodeJSONArray(t, resp)
	if len(reviews) == 0 {
		t.Fatal("expected at least 1 review")
	}
}

func TestSharedNumbering(t *testing.T) {
	createTestPRRepo(t, "shared-num")

	// Issue #1
	r1 := ghPost(t, "/api/v3/repos/admin/shared-num/issues", defaultToken, map[string]interface{}{
		"title": "Issue 1",
	})
	d1 := decodeJSON(t, r1)
	if d1["number"] != 1.0 {
		t.Fatalf("expected issue number=1, got %v", d1["number"])
	}

	// PR #2
	r2 := ghPost(t, "/api/v3/repos/admin/shared-num/pulls", defaultToken, map[string]interface{}{
		"title": "PR 2", "head": "feat", "base": "main",
	})
	d2 := decodeJSON(t, r2)
	if d2["number"] != 2.0 {
		t.Fatalf("expected PR number=2, got %v", d2["number"])
	}

	// Issue #3
	r3 := ghPost(t, "/api/v3/repos/admin/shared-num/issues", defaultToken, map[string]interface{}{
		"title": "Issue 3",
	})
	d3 := decodeJSON(t, r3)
	if d3["number"] != 3.0 {
		t.Fatalf("expected issue number=3, got %v", d3["number"])
	}
}

func TestDeleteRefREST(t *testing.T) {
	createTestPRRepo(t, "pr-delref")

	// Non-existent ref returns 422 (matching real GitHub behavior)
	resp := ghDelete(t, "/api/v3/repos/admin/pr-delref/git/refs/heads/feature", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("expected 422 for non-existent ref, got %d", resp.StatusCode)
	}
}

// --- GraphQL tests ---

func TestGraphQLCreatePullRequest(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "gql-pr-create",
	})
	repoData := decodeJSON(t, resp)
	repoNodeID := repoData["node_id"].(string)

	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: CreatePullRequestInput!) { createPullRequest(input: $input) { pullRequest { number title headRefName baseRefName state isDraft } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"repositoryId": repoNodeID,
				"title":        "GQL PR",
				"headRefName":  "feature",
				"baseRefName":  "main",
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
	payload, _ := d["createPullRequest"].(map[string]interface{})
	pr, _ := payload["pullRequest"].(map[string]interface{})
	if pr == nil {
		t.Fatalf("expected pullRequest in payload: %v", data)
	}
	if pr["title"] != "GQL PR" {
		t.Fatalf("expected title='GQL PR', got %v", pr["title"])
	}
	if pr["number"] != 1.0 {
		t.Fatalf("expected number=1, got %v", pr["number"])
	}
	if pr["headRefName"] != "feature" {
		t.Fatalf("expected headRefName=feature, got %v", pr["headRefName"])
	}
	if pr["state"] != "OPEN" {
		t.Fatalf("expected state=OPEN, got %v", pr["state"])
	}
}

func TestGraphQLListPullRequests(t *testing.T) {
	createTestPRRepo(t, "gql-pr-list")
	ghPost(t, "/api/v3/repos/admin/gql-pr-list/pulls", defaultToken, map[string]interface{}{
		"title": "GQL list PR 1", "head": "feat1", "base": "main",
	}).Body.Close()
	ghPost(t, "/api/v3/repos/admin/gql-pr-list/pulls", defaultToken, map[string]interface{}{
		"title": "GQL list PR 2", "head": "feat2", "base": "main",
	}).Body.Close()

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"gql-pr-list"){pullRequests(first:10,states:[OPEN]){totalCount,nodes{number,title,state}}}}`,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	prs, _ := repo["pullRequests"].(map[string]interface{})
	if prs == nil {
		t.Fatalf("expected pullRequests: %v", data)
	}
	tc, _ := prs["totalCount"].(float64)
	if tc < 2 {
		t.Fatalf("expected totalCount >= 2, got %v", tc)
	}
}

func TestGraphQLGetPullRequest(t *testing.T) {
	createTestPRRepo(t, "gql-pr-get")
	ghPost(t, "/api/v3/repos/admin/gql-pr-get/pulls", defaultToken, map[string]interface{}{
		"title": "GQL get PR", "body": "PR body", "head": "feat", "base": "main",
	}).Body.Close()

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"gql-pr-get"){pullRequest(number:1){title,body,state,headRefName,baseRefName,isDraft,merged,author{login},labels(first:10){nodes{name}},reviews(first:10){nodes{state}}}}}`,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	pr, _ := repo["pullRequest"].(map[string]interface{})
	if pr == nil {
		t.Fatalf("expected pullRequest: %v", data)
	}
	if pr["title"] != "GQL get PR" {
		t.Fatalf("expected title='GQL get PR', got %v", pr["title"])
	}
	if pr["body"] != "PR body" {
		t.Fatalf("expected body='PR body', got %v", pr["body"])
	}
	if pr["headRefName"] != "feat" {
		t.Fatalf("expected headRefName=feat, got %v", pr["headRefName"])
	}
	author, _ := pr["author"].(map[string]interface{})
	if author == nil || author["login"] != "admin" {
		t.Fatalf("expected author.login=admin, got %v", author)
	}
}

func TestGraphQLClosePullRequest(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "gql-pr-close",
	})
	repoData := decodeJSON(t, resp)
	repoNodeID := repoData["node_id"].(string)

	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: CreatePullRequestInput!) { createPullRequest(input: $input) { pullRequest { id } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"repositoryId": repoNodeID,
				"title":        "To close",
				"headRefName":  "feat",
				"baseRefName":  "main",
			},
		},
	})
	d2 := decodeJSON(t, resp2)
	dd2, _ := d2["data"].(map[string]interface{})
	cp, _ := dd2["createPullRequest"].(map[string]interface{})
	prData, _ := cp["pullRequest"].(map[string]interface{})
	prID := prData["id"].(string)

	resp3 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: ClosePullRequestInput!) { closePullRequest(input: $input) { pullRequest { state } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{"pullRequestId": prID},
		},
	})
	data := decodeJSON(t, resp3)
	d, _ := data["data"].(map[string]interface{})
	cl, _ := d["closePullRequest"].(map[string]interface{})
	pr, _ := cl["pullRequest"].(map[string]interface{})
	if pr["state"] != "CLOSED" {
		t.Fatalf("expected state=CLOSED, got %v", pr["state"])
	}
}

func TestGraphQLReopenPullRequest(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "gql-pr-reopen",
	})
	repoData := decodeJSON(t, resp)
	repoNodeID := repoData["node_id"].(string)

	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: CreatePullRequestInput!) { createPullRequest(input: $input) { pullRequest { id } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"repositoryId": repoNodeID,
				"title":        "To reopen",
				"headRefName":  "feat",
				"baseRefName":  "main",
			},
		},
	})
	d2 := decodeJSON(t, resp2)
	dd2, _ := d2["data"].(map[string]interface{})
	cp, _ := dd2["createPullRequest"].(map[string]interface{})
	prData, _ := cp["pullRequest"].(map[string]interface{})
	prID := prData["id"].(string)

	// Close
	ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: ClosePullRequestInput!) { closePullRequest(input: $input) { pullRequest { state } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{"pullRequestId": prID},
		},
	}).Body.Close()

	// Reopen
	resp3 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: ReopenPullRequestInput!) { reopenPullRequest(input: $input) { pullRequest { state } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{"pullRequestId": prID},
		},
	})
	data := decodeJSON(t, resp3)
	d, _ := data["data"].(map[string]interface{})
	ro, _ := d["reopenPullRequest"].(map[string]interface{})
	pr, _ := ro["pullRequest"].(map[string]interface{})
	if pr["state"] != "OPEN" {
		t.Fatalf("expected state=OPEN, got %v", pr["state"])
	}
}

func TestGraphQLMergePullRequest(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "gql-pr-merge",
	})
	repoData := decodeJSON(t, resp)
	repoNodeID := repoData["node_id"].(string)

	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: CreatePullRequestInput!) { createPullRequest(input: $input) { pullRequest { id } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"repositoryId": repoNodeID,
				"title":        "To merge",
				"headRefName":  "feat",
				"baseRefName":  "main",
			},
		},
	})
	d2 := decodeJSON(t, resp2)
	dd2, _ := d2["data"].(map[string]interface{})
	cp, _ := dd2["createPullRequest"].(map[string]interface{})
	prData, _ := cp["pullRequest"].(map[string]interface{})
	prID := prData["id"].(string)

	resp3 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: MergePullRequestInput!) { mergePullRequest(input: $input) { pullRequest { state merged mergedAt } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{"pullRequestId": prID},
		},
	})
	data := decodeJSON(t, resp3)
	d, _ := data["data"].(map[string]interface{})
	mg, _ := d["mergePullRequest"].(map[string]interface{})
	pr, _ := mg["pullRequest"].(map[string]interface{})
	if pr["state"] != "MERGED" {
		t.Fatalf("expected state=MERGED, got %v", pr["state"])
	}
	if pr["merged"] != true {
		t.Fatalf("expected merged=true, got %v", pr["merged"])
	}
	if pr["mergedAt"] == nil {
		t.Fatal("expected mergedAt to be set")
	}
}

func TestGraphQLMergeWithMethod(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "gql-pr-squash",
	})
	repoData := decodeJSON(t, resp)
	repoNodeID := repoData["node_id"].(string)

	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: CreatePullRequestInput!) { createPullRequest(input: $input) { pullRequest { id } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"repositoryId": repoNodeID,
				"title":        "Squash merge",
				"headRefName":  "feat",
				"baseRefName":  "main",
			},
		},
	})
	d2 := decodeJSON(t, resp2)
	dd2, _ := d2["data"].(map[string]interface{})
	cp, _ := dd2["createPullRequest"].(map[string]interface{})
	prData, _ := cp["pullRequest"].(map[string]interface{})
	prID := prData["id"].(string)

	resp3 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: MergePullRequestInput!) { mergePullRequest(input: $input) { pullRequest { state merged } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"pullRequestId": prID,
				"mergeMethod":   "SQUASH",
			},
		},
	})
	data := decodeJSON(t, resp3)
	d, _ := data["data"].(map[string]interface{})
	mg, _ := d["mergePullRequest"].(map[string]interface{})
	pr, _ := mg["pullRequest"].(map[string]interface{})
	if pr["state"] != "MERGED" {
		t.Fatalf("expected state=MERGED, got %v", pr["state"])
	}
}

func TestGraphQLUpdatePullRequest(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "gql-pr-update",
	})
	repoData := decodeJSON(t, resp)
	repoNodeID := repoData["node_id"].(string)

	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: CreatePullRequestInput!) { createPullRequest(input: $input) { pullRequest { id } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"repositoryId": repoNodeID,
				"title":        "Before update",
				"headRefName":  "feat",
				"baseRefName":  "main",
			},
		},
	})
	d2 := decodeJSON(t, resp2)
	dd2, _ := d2["data"].(map[string]interface{})
	cp, _ := dd2["createPullRequest"].(map[string]interface{})
	prData, _ := cp["pullRequest"].(map[string]interface{})
	prID := prData["id"].(string)

	resp3 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: UpdatePullRequestInput!) { updatePullRequest(input: $input) { pullRequest { title } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"pullRequestId": prID,
				"title":         "After update",
			},
		},
	})
	data := decodeJSON(t, resp3)
	d, _ := data["data"].(map[string]interface{})
	if d == nil {
		t.Fatalf("expected data, got errors: %v", data)
	}
	up, _ := d["updatePullRequest"].(map[string]interface{})
	pr, _ := up["pullRequest"].(map[string]interface{})
	if pr["title"] != "After update" {
		t.Fatalf("expected title='After update', got %v", pr["title"])
	}
}

func TestGraphQLPullRequestWithLabels(t *testing.T) {
	createTestPRRepo(t, "gql-pr-labels")
	ghPost(t, "/api/v3/repos/admin/gql-pr-labels/labels", defaultToken, map[string]interface{}{
		"name": "bug", "color": "d73a4a",
	}).Body.Close()

	// Create PR and add label via REST
	r1 := ghPost(t, "/api/v3/repos/admin/gql-pr-labels/pulls", defaultToken, map[string]interface{}{
		"title": "Labeled PR", "head": "feat", "base": "main",
	})
	prData := decodeJSON(t, r1)
	prNodeID := prData["node_id"].(string)

	// Get label node ID
	r2 := ghGet(t, "/api/v3/repos/admin/gql-pr-labels/labels/bug", "")
	labelData := decodeJSON(t, r2)
	labelNodeID := labelData["node_id"].(string)

	// Update PR with labels via GraphQL
	ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: UpdatePullRequestInput!) { updatePullRequest(input: $input) { pullRequest { title } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"pullRequestId": prNodeID,
				"labelIds":      []string{labelNodeID},
			},
		},
	}).Body.Close()

	// Query labels
	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"gql-pr-labels"){pullRequest(number:1){labels(first:10){nodes{name},totalCount}}}}`,
	})
	data := decodeJSON(t, resp)
	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	pr, _ := repo["pullRequest"].(map[string]interface{})
	labels, _ := pr["labels"].(map[string]interface{})
	tc, _ := labels["totalCount"].(float64)
	if tc != 1 {
		t.Fatalf("expected 1 label, got %v", tc)
	}
	nodes, _ := labels["nodes"].([]interface{})
	lbl, _ := nodes[0].(map[string]interface{})
	if lbl["name"] != "bug" {
		t.Fatalf("expected label name=bug, got %v", lbl["name"])
	}
}

func TestGraphQLPullRequestReviews(t *testing.T) {
	createTestPRRepo(t, "gql-pr-reviews")
	ghPost(t, "/api/v3/repos/admin/gql-pr-reviews/pulls", defaultToken, map[string]interface{}{
		"title": "Review PR", "head": "feat", "base": "main",
	}).Body.Close()

	ghPost(t, "/api/v3/repos/admin/gql-pr-reviews/pulls/1/reviews", defaultToken, map[string]interface{}{
		"body": "Looks good", "event": "APPROVE",
	}).Body.Close()

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"gql-pr-reviews"){pullRequest(number:1){reviews(first:10){totalCount,nodes{state,body}}}}}`,
	})
	data := decodeJSON(t, resp)
	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	pr, _ := repo["pullRequest"].(map[string]interface{})
	reviews, _ := pr["reviews"].(map[string]interface{})
	tc, _ := reviews["totalCount"].(float64)
	if tc != 1 {
		t.Fatalf("expected 1 review, got %v", tc)
	}
	nodes, _ := reviews["nodes"].([]interface{})
	review, _ := nodes[0].(map[string]interface{})
	if review["state"] != "APPROVED" {
		t.Fatalf("expected state=APPROVED, got %v", review["state"])
	}
}

func TestGraphQLReviewDecision(t *testing.T) {
	createTestPRRepo(t, "gql-pr-decision")
	ghPost(t, "/api/v3/repos/admin/gql-pr-decision/pulls", defaultToken, map[string]interface{}{
		"title": "Decision PR", "head": "feat", "base": "main",
	}).Body.Close()

	ghPost(t, "/api/v3/repos/admin/gql-pr-decision/pulls/1/reviews", defaultToken, map[string]interface{}{
		"body": "LGTM", "event": "APPROVE",
	}).Body.Close()

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"gql-pr-decision"){pullRequest(number:1){reviewDecision}}}`,
	})
	data := decodeJSON(t, resp)
	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	pr, _ := repo["pullRequest"].(map[string]interface{})
	if pr["reviewDecision"] != "APPROVED" {
		t.Fatalf("expected reviewDecision=APPROVED, got %v", pr["reviewDecision"])
	}
}

func TestGraphQLFilterByState(t *testing.T) {
	createTestPRRepo(t, "gql-pr-statefilter")

	// Create and merge a PR
	ghPost(t, "/api/v3/repos/admin/gql-pr-statefilter/pulls", defaultToken, map[string]interface{}{
		"title": "Merged PR", "head": "feat", "base": "main",
	}).Body.Close()
	ghPut(t, "/api/v3/repos/admin/gql-pr-statefilter/pulls/1/merge", defaultToken, map[string]interface{}{}).Body.Close()

	// Create an open PR
	ghPost(t, "/api/v3/repos/admin/gql-pr-statefilter/pulls", defaultToken, map[string]interface{}{
		"title": "Open PR", "head": "feat2", "base": "main",
	}).Body.Close()

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"gql-pr-statefilter"){pullRequests(first:10,states:[MERGED]){totalCount,nodes{title,state}}}}`,
	})
	data := decodeJSON(t, resp)
	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	prs, _ := repo["pullRequests"].(map[string]interface{})
	tc, _ := prs["totalCount"].(float64)
	if tc != 1 {
		t.Fatalf("expected 1 merged PR, got %v", tc)
	}
	nodes, _ := prs["nodes"].([]interface{})
	pr, _ := nodes[0].(map[string]interface{})
	if pr["title"] != "Merged PR" {
		t.Fatalf("expected title='Merged PR', got %v", pr["title"])
	}
}

func TestGraphQLDraftPullRequest(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "gql-pr-draft",
	})
	repoData := decodeJSON(t, resp)
	repoNodeID := repoData["node_id"].(string)

	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: CreatePullRequestInput!) { createPullRequest(input: $input) { pullRequest { isDraft } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"repositoryId": repoNodeID,
				"title":        "Draft PR",
				"headRefName":  "draft-feat",
				"baseRefName":  "main",
				"draft":        true,
			},
		},
	})
	data := decodeJSON(t, resp2)
	d, _ := data["data"].(map[string]interface{})
	cp, _ := d["createPullRequest"].(map[string]interface{})
	pr, _ := cp["pullRequest"].(map[string]interface{})
	if pr["isDraft"] != true {
		t.Fatalf("expected isDraft=true, got %v", pr["isDraft"])
	}
}

func TestGraphQLCannotMergeClosed(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "gql-pr-merge-closed",
	})
	repoData := decodeJSON(t, resp)
	repoNodeID := repoData["node_id"].(string)

	// Create and close
	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: CreatePullRequestInput!) { createPullRequest(input: $input) { pullRequest { id } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{
				"repositoryId": repoNodeID,
				"title":        "To close then merge",
				"headRefName":  "feat",
				"baseRefName":  "main",
			},
		},
	})
	d2 := decodeJSON(t, resp2)
	dd2, _ := d2["data"].(map[string]interface{})
	cp, _ := dd2["createPullRequest"].(map[string]interface{})
	prData, _ := cp["pullRequest"].(map[string]interface{})
	prID := prData["id"].(string)

	// Close
	ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: ClosePullRequestInput!) { closePullRequest(input: $input) { pullRequest { state } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{"pullRequestId": prID},
		},
	}).Body.Close()

	// Try to merge
	resp3 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `mutation($input: MergePullRequestInput!) { mergePullRequest(input: $input) { pullRequest { state } } }`,
		"variables": map[string]interface{}{
			"input": map[string]interface{}{"pullRequestId": prID},
		},
	})
	data := decodeJSON(t, resp3)
	errors, _ := data["errors"].([]interface{})
	if len(errors) == 0 {
		t.Fatalf("expected errors when merging closed PR, got none: %v", data)
	}
}
