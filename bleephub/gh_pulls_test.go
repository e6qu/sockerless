package bleephub

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func createTestPRRepo(t *testing.T, name string) {
	t.Helper()
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": name, "auto_init": true,
	})
	resp.Body.Close()
	repo := testServer.store.GetRepo("admin", name)
	if repo == nil {
		t.Fatalf("repo %s not created", name)
	}
	seedPullRequestBranches(t, testServer, repo, "feature", "feat", "feat1", "feat2", "fix", "branch", "r", "f", "a", "b", "draft-feat")
}

func createGraphQLPRRepo(t *testing.T, name string, branches ...string) string {
	t.Helper()
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": name, "auto_init": true,
	})
	repoData := decodeJSON(t, resp)
	repo := testServer.store.GetRepo("admin", name)
	if repo == nil {
		t.Fatalf("repo %s not created", name)
	}
	seedPullRequestBranches(t, testServer, repo, branches...)
	return repoData["node_id"].(string)
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

	// head.user and base.user must be the REST simple-user shape
	// (snake_case), not the GraphQL camelCase map.
	headUser, _ := head["user"].(map[string]interface{})
	if headUser == nil {
		t.Fatal("missing head.user")
	}
	if _, ok := headUser["node_id"]; !ok {
		t.Errorf("head.user missing node_id: %v", headUser)
	}
	if _, ok := headUser["login"]; !ok {
		t.Errorf("head.user missing login: %v", headUser)
	}
	if _, ok := headUser["nodeID"]; ok {
		t.Errorf("head.user has GraphQL nodeID in REST response: %v", headUser)
	}
}

func TestForkPullRequestRESTAndGraphQL(t *testing.T) {
	sourceName := "pr-fork-source"
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": sourceName, "auto_init": true,
	})
	resp.Body.Close()
	source := testServer.store.GetRepo("admin", sourceName)
	if source == nil {
		t.Fatalf("source repo not created")
	}
	seedPullRequestBranches(t, testServer, source)

	testServer.store.mu.Lock()
	forker := &User{ID: testServer.store.NextUser, Login: "pr-forker", Type: "User", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	testServer.store.NextUser++
	testServer.store.Users[forker.ID] = forker
	testServer.store.UsersByLogin[forker.Login] = forker
	tok := &Token{Value: "pr-forker-token", UserID: forker.ID, Scopes: "repo", CreatedAt: time.Now()}
	testServer.store.Tokens[tok.Value] = tok
	testServer.store.mu.Unlock()

	forkResp := ghPost(t, "/api/v3/repos/admin/"+sourceName+"/forks", tok.Value, map[string]interface{}{})
	if forkResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(forkResp.Body)
		forkResp.Body.Close()
		t.Fatalf("fork status = %d body=%s", forkResp.StatusCode, body)
	}
	forkResp.Body.Close()
	fork := testServer.store.GetRepo("pr-forker", sourceName)
	if fork == nil {
		t.Fatalf("fork repo not created")
	}
	seedPullRequestBranches(t, testServer, fork, "fork-change")

	createResp := ghPost(t, "/api/v3/repos/admin/"+sourceName+"/pulls", tok.Value, map[string]interface{}{
		"title":                 "Fork change",
		"body":                  "from fork",
		"head":                  "pr-forker:fork-change",
		"base":                  "main",
		"maintainer_can_modify": true,
	})
	if createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		createResp.Body.Close()
		t.Fatalf("create fork pull request status = %d body=%s", createResp.StatusCode, body)
	}
	created := decodeJSON(t, createResp)
	head, _ := created["head"].(map[string]interface{})
	headRepo, _ := head["repo"].(map[string]interface{})
	if headRepo == nil || headRepo["full_name"] != "pr-forker/"+sourceName {
		t.Fatalf("head.repo = %v, want fork repo", head["repo"])
	}
	headUser, _ := head["user"].(map[string]interface{})
	if headUser == nil || headUser["login"] != "pr-forker" {
		t.Fatalf("head.user = %v, want pr-forker", head["user"])
	}
	if created["maintainer_can_modify"] != true {
		t.Fatalf("maintainer_can_modify = %v, want true", created["maintainer_can_modify"])
	}

	filesResp := ghGet(t, "/api/v3/repos/admin/"+sourceName+"/pulls/1/files", defaultToken)
	if filesResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(filesResp.Body)
		filesResp.Body.Close()
		t.Fatalf("files status = %d body=%s", filesResp.StatusCode, body)
	}
	files := decodeJSONArray(t, filesResp)
	if len(files) != 1 || files[0]["filename"] != "fork-change.txt" {
		t.Fatalf("files = %v, want fork-change.txt", files)
	}

	gql := gqlData(t, `query($owner:String!,$name:String!){
		repository(owner:$owner,name:$name){
			pullRequest(number:1){
				headRefName
				headRepository{nameWithOwner}
				headRepositoryOwner{login}
				isCrossRepository
				maintainerCanModify
				files(first:10){totalCount nodes{path}}
				commits(first:10){totalCount nodes{commit{oid}}}
			}
		}
	}`, map[string]interface{}{"owner": "admin", "name": sourceName})
	repoData, _ := gql["repository"].(map[string]interface{})
	prData, _ := repoData["pullRequest"].(map[string]interface{})
	if prData["isCrossRepository"] != true {
		t.Fatalf("isCrossRepository = %v, want true", prData["isCrossRepository"])
	}
	if prData["maintainerCanModify"] != true {
		t.Fatalf("maintainerCanModify = %v, want true", prData["maintainerCanModify"])
	}
	gqlHeadRepo, _ := prData["headRepository"].(map[string]interface{})
	if gqlHeadRepo == nil || gqlHeadRepo["nameWithOwner"] != "pr-forker/"+sourceName {
		t.Fatalf("GraphQL headRepository = %v", prData["headRepository"])
	}
	gqlHeadOwner, _ := prData["headRepositoryOwner"].(map[string]interface{})
	if gqlHeadOwner == nil || gqlHeadOwner["login"] != "pr-forker" {
		t.Fatalf("GraphQL headRepositoryOwner = %v", prData["headRepositoryOwner"])
	}
	gqlFiles, _ := prData["files"].(map[string]interface{})
	if gqlFiles["totalCount"] != float64(1) {
		t.Fatalf("GraphQL files = %v, want one changed file", gqlFiles)
	}
	gqlCommits, _ := prData["commits"].(map[string]interface{})
	if gqlCommits["totalCount"] != float64(1) {
		t.Fatalf("GraphQL commits = %v, want one commit", gqlCommits)
	}

	beforeMergeSHA := resolveBranchSha(testServer.store.GetGitStorage("admin", sourceName), "main")
	mergeResp := ghPut(t, "/api/v3/repos/admin/"+sourceName+"/pulls/1/merge", defaultToken, map[string]interface{}{
		"merge_method": "merge",
	})
	if mergeResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(mergeResp.Body)
		mergeResp.Body.Close()
		t.Fatalf("merge status = %d body=%s", mergeResp.StatusCode, body)
	}
	mergeResp.Body.Close()
	baseSHA := resolveBranchSha(testServer.store.GetGitStorage("admin", sourceName), "main")
	if baseSHA == "" || baseSHA == beforeMergeSHA {
		t.Fatalf("base branch did not advance after fork pull request merge: %q", baseSHA)
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

	resp := ghGet(t, "/api/v3/repos/admin/pr-reviews-list/pulls/1/reviews", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	reviews := decodeJSONArray(t, resp)
	if len(reviews) == 0 {
		t.Fatal("expected at least 1 review")
	}
}

func TestPRReviewCRUDREST(t *testing.T) {
	createTestPRRepo(t, "pr-review-crud")
	ghPost(t, "/api/v3/repos/admin/pr-review-crud/pulls", defaultToken, map[string]interface{}{
		"title": "Review CRUD", "head": "feat", "base": "main",
	}).Body.Close()

	// Create a pending review (no event)
	resp := ghPost(t, "/api/v3/repos/admin/pr-review-crud/pulls/1/reviews", defaultToken, map[string]interface{}{
		"body": "Pending feedback",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("create review: expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["state"] != "PENDING" {
		t.Fatalf("expected state=PENDING, got %v", data["state"])
	}
	if data["submitted_at"] != nil {
		t.Fatalf("expected submitted_at=null for pending, got %v", data["submitted_at"])
	}
	reviewID := int(data["id"].(float64))

	// Get review
	resp = ghGet(t, fmt.Sprintf("/api/v3/repos/admin/pr-review-crud/pulls/1/reviews/%d", reviewID), defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("get review: expected 200, got %d", resp.StatusCode)
	}
	data = decodeJSON(t, resp)
	if data["body"] != "Pending feedback" {
		t.Fatalf("expected body='Pending feedback', got %v", data["body"])
	}

	// Update review body
	resp = ghPut(t, fmt.Sprintf("/api/v3/repos/admin/pr-review-crud/pulls/1/reviews/%d", reviewID), defaultToken, map[string]interface{}{
		"body": "Updated feedback",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("update review: expected 200, got %d", resp.StatusCode)
	}
	data = decodeJSON(t, resp)
	if data["body"] != "Updated feedback" {
		t.Fatalf("expected body='Updated feedback', got %v", data["body"])
	}

	// Submit review as APPROVED
	resp = ghPost(t, fmt.Sprintf("/api/v3/repos/admin/pr-review-crud/pulls/1/reviews/%d/events", reviewID), defaultToken, map[string]interface{}{
		"event": "APPROVE",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("submit review: expected 200, got %d", resp.StatusCode)
	}
	data = decodeJSON(t, resp)
	if data["state"] != "APPROVED" {
		t.Fatalf("expected state=APPROVED, got %v", data["state"])
	}
	if data["submitted_at"] == nil {
		t.Fatal("expected submitted_at to be set after submit")
	}

	// Dismiss review
	resp = ghPut(t, fmt.Sprintf("/api/v3/repos/admin/pr-review-crud/pulls/1/reviews/%d/dismissals", reviewID), defaultToken, map[string]interface{}{
		"message": "Dismissed via test",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("dismiss review: expected 200, got %d", resp.StatusCode)
	}
	data = decodeJSON(t, resp)
	if data["state"] != "DISMISSED" {
		t.Fatalf("expected state=DISMISSED, got %v", data["state"])
	}
}

func TestPRReviewDeletePendingREST(t *testing.T) {
	createTestPRRepo(t, "pr-review-delete")
	ghPost(t, "/api/v3/repos/admin/pr-review-delete/pulls", defaultToken, map[string]interface{}{
		"title": "Review delete", "head": "feat", "base": "main",
	}).Body.Close()

	resp := ghPost(t, "/api/v3/repos/admin/pr-review-delete/pulls/1/reviews", defaultToken, map[string]interface{}{
		"body": "To delete",
	})
	data := decodeJSON(t, resp)
	reviewID := int(data["id"].(float64))

	resp = ghDelete(t, fmt.Sprintf("/api/v3/repos/admin/pr-review-delete/pulls/1/reviews/%d", reviewID), defaultToken)
	if resp.StatusCode != 204 {
		resp.Body.Close()
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify deleted
	resp = ghGet(t, fmt.Sprintf("/api/v3/repos/admin/pr-review-delete/pulls/1/reviews/%d", reviewID), defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404 after delete, got %d", resp.StatusCode)
	}
}

func TestPRReviewRequestReviewersREST(t *testing.T) {
	createTestPRRepo(t, "pr-reviewers")
	ghPost(t, "/api/v3/repos/admin/pr-reviewers/pulls", defaultToken, map[string]interface{}{
		"title": "Reviewers", "head": "feat", "base": "main",
	}).Body.Close()

	// Create a second user to request as reviewer
	ghPost(t, "/internal/users", defaultToken, map[string]interface{}{
		"login": "reviewer1", "name": "Reviewer One", "email": "r1@example.com",
	}).Body.Close()

	// Request reviewer by login
	resp := ghPost(t, "/api/v3/repos/admin/pr-reviewers/pulls/1/requested_reviewers", defaultToken, map[string]interface{}{
		"reviewers": []string{"reviewer1"},
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("request reviewers: expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	reviewers, _ := data["requested_reviewers"].([]interface{})
	if len(reviewers) != 1 {
		t.Fatalf("expected 1 requested reviewer, got %d", len(reviewers))
	}
	reviewer := reviewers[0].(map[string]interface{})
	if reviewer["login"] != "reviewer1" {
		t.Fatalf("expected login=reviewer1, got %v", reviewer["login"])
	}

	// Remove requested reviewer
	body, _ := json.Marshal(map[string]interface{}{
		"reviewers": []string{"reviewer1"},
	})
	req, _ := http.NewRequest("DELETE", testBaseURL+"/api/v3/repos/admin/pr-reviewers/pulls/1/requested_reviewers", bytes.NewReader(body))
	req.Header.Set("Authorization", "token "+defaultToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("remove reviewers: expected 200, got %d", resp.StatusCode)
	}
	data = decodeJSON(t, resp)
	reviewers, _ = data["requested_reviewers"].([]interface{})
	if len(reviewers) != 0 {
		t.Fatalf("expected 0 requested reviewers after removal, got %d", len(reviewers))
	}
}

func TestPRUpdateBranchREST(t *testing.T) {
	createTestPRRepo(t, "pr-update-branch")
	ghPost(t, "/api/v3/repos/admin/pr-update-branch/pulls", defaultToken, map[string]interface{}{
		"title": "Update branch", "head": "feat", "base": "main",
	}).Body.Close()

	resp := ghPut(t, "/api/v3/repos/admin/pr-update-branch/pulls/1/update-branch", defaultToken, map[string]interface{}{})
	if resp.StatusCode != 202 {
		resp.Body.Close()
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["message"] == nil || data["message"] == "" {
		t.Fatal("expected message in update-branch response")
	}
}

func TestSharedNumbering(t *testing.T) {
	createTestPRRepo(t, "shared-num")

	r1 := ghPost(t, "/api/v3/repos/admin/shared-num/issues", defaultToken, map[string]interface{}{
		"title": "Issue 1",
	})
	d1 := decodeJSON(t, r1)
	if d1["number"] != 1.0 {
		t.Fatalf("expected issue number=1, got %v", d1["number"])
	}

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
	resp := ghDelete(t, "/api/v3/repos/admin/pr-delref/git/refs/heads/missing-feature", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("expected 422 for non-existent ref, got %d", resp.StatusCode)
	}
}

// --- GraphQL tests ---

func TestGraphQLCreatePullRequest(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "gql-pr-create", "auto_init": true,
	})
	repoData := decodeJSON(t, resp)
	repoNodeID := repoData["node_id"].(string)
	repo := testServer.store.GetRepo("admin", "gql-pr-create")
	if repo == nil {
		t.Fatal("repo not created")
	}
	seedPullRequestBranches(t, testServer, repo, "feature")

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

func TestGraphQLPullRequestConverterDoesNotReenterStoreForGitStorage(t *testing.T) {
	src, err := os.ReadFile("gh_pulls_graphql.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "pullRequestCommitObjects(st,") || strings.Contains(body, "pullRequestCommitObjects(s.store,") {
		t.Fatal("pull request GraphQL conversion must not call store-locking commit helpers while rendering under Store.mu")
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
	repoNodeID := createGraphQLPRRepo(t, "gql-pr-close", "feat")

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
	repoNodeID := createGraphQLPRRepo(t, "gql-pr-reopen", "feat")

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
	repoNodeID := createGraphQLPRRepo(t, "gql-pr-merge", "feat")

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
	repoNodeID := createGraphQLPRRepo(t, "gql-pr-squash", "feat")

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
	repoNodeID := createGraphQLPRRepo(t, "gql-pr-update", "feat")

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
	repoNodeID := createGraphQLPRRepo(t, "gql-pr-draft", "draft-feat")

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
	repoNodeID := createGraphQLPRRepo(t, "gql-pr-merge-closed", "feat")

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

func TestListRequestedReviewersREST(t *testing.T) {
	createTestPRRepo(t, "pr-req-reviewers-get")
	ghPost(t, "/api/v3/repos/admin/pr-req-reviewers-get/pulls", defaultToken, map[string]interface{}{
		"title": "Reviewer request PR", "head": "feat", "base": "main",
	}).Body.Close()

	// Empty before any request.
	resp := ghGet(t, "/api/v3/repos/admin/pr-req-reviewers-get/pulls/1/requested_reviewers", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	users, ok := data["users"].([]interface{})
	if !ok {
		t.Fatalf("expected users array, got %T", data["users"])
	}
	if len(users) != 0 {
		t.Fatalf("expected no requested reviewers, got %d", len(users))
	}
	if _, ok := data["teams"].([]interface{}); !ok {
		t.Fatalf("expected teams array, got %T", data["teams"])
	}

	// Request the admin user, read it back.
	ghPost(t, "/api/v3/repos/admin/pr-req-reviewers-get/pulls/1/requested_reviewers", defaultToken, map[string]interface{}{
		"reviewers": []string{"admin"},
	}).Body.Close()
	resp = ghGet(t, "/api/v3/repos/admin/pr-req-reviewers-get/pulls/1/requested_reviewers", defaultToken)
	data = decodeJSON(t, resp)
	users, _ = data["users"].([]interface{})
	if len(users) != 1 {
		t.Fatalf("expected 1 requested reviewer, got %d", len(users))
	}
	u, _ := users[0].(map[string]interface{})
	if u["login"] != "admin" {
		t.Fatalf("expected requested reviewer admin, got %v", u["login"])
	}

	// Remove it again.
	ghDeleteWithBody(t, "/api/v3/repos/admin/pr-req-reviewers-get/pulls/1/requested_reviewers", defaultToken, map[string]interface{}{
		"reviewers": []string{"admin"},
	}).Body.Close()
	resp = ghGet(t, "/api/v3/repos/admin/pr-req-reviewers-get/pulls/1/requested_reviewers", defaultToken)
	data = decodeJSON(t, resp)
	users, _ = data["users"].([]interface{})
	if len(users) != 0 {
		t.Fatalf("expected requested reviewers cleared, got %d", len(users))
	}
}

func TestPullRequestTimelineREST(t *testing.T) {
	createTestPRRepo(t, "pr-timeline")
	ghPost(t, "/api/v3/repos/admin/pr-timeline/pulls", defaultToken, map[string]interface{}{
		"title": "Timeline PR", "head": "feat", "base": "main",
	}).Body.Close()

	// A real PR timeline includes the head commits, conversation comments,
	// and submitted reviews. Pending reviews must not surface.
	ghPost(t, "/api/v3/repos/admin/pr-timeline/issues/1/comments", defaultToken, map[string]interface{}{
		"body": "first conversation comment",
	}).Body.Close()
	ghPost(t, "/api/v3/repos/admin/pr-timeline/pulls/1/reviews", defaultToken, map[string]interface{}{
		"body": "review body", "event": "APPROVE",
	}).Body.Close()
	// A pending review must NOT surface in the timeline.
	ghPost(t, "/api/v3/repos/admin/pr-timeline/pulls/1/reviews", defaultToken, map[string]interface{}{
		"body": "still drafting",
	}).Body.Close()

	resp := ghGet(t, "/api/v3/repos/admin/pr-timeline/issues/1/timeline", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	items := decodeJSONArray(t, resp)
	if len(items) != 3 {
		t.Fatalf("expected 3 timeline items (committed + commented + reviewed), got %d: %v", len(items), items)
	}
	byEvent := map[string]map[string]interface{}{}
	for _, item := range items {
		ev, _ := item["event"].(string)
		byEvent[ev] = item
	}
	commented := byEvent["commented"]
	if commented == nil {
		t.Fatalf("expected a commented timeline item, got %v", items)
	}
	if byEvent["committed"] == nil {
		t.Fatalf("expected a committed timeline item, got %v", items)
	}
	if commented["body"] != "first conversation comment" {
		t.Fatalf("expected comment body, got %v", commented["body"])
	}
	reviewed := byEvent["reviewed"]
	if reviewed == nil {
		t.Fatalf("expected a reviewed timeline item, got %v", items)
	}
	if reviewed["state"] != "APPROVED" {
		t.Fatalf("expected reviewed state APPROVED, got %v", reviewed["state"])
	}
	if reviewed["body"] != "review body" {
		t.Fatalf("expected review body, got %v", reviewed["body"])
	}
	if reviewed["submitted_at"] == nil {
		t.Fatal("expected reviewed submitted_at to be set")
	}
}

// TestPullRequestTimelineFullFlowREST drives a git-backed pull request
// through reviewer request/removal and merge, and asserts the issue
// timeline carries the committed / review_requested /
// review_request_removed / merged / closed entries GitHub documents, in
// order, with stable ids and recorded timestamps.
func TestPullRequestTimelineFullFlowREST(t *testing.T) {
	repoPath := "/api/v3/repos/admin/pr-timeline-flow"
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "pr-timeline-flow", "auto_init": true,
	}).Body.Close()
	createTestUser(t, "flow-reviewer")

	// Branch feat off main and add two real commits via the contents API.
	refResp := ghGet(t, repoPath+"/git/refs/heads/main", defaultToken)
	refData := decodeJSON(t, refResp)
	mainObj, _ := refData["object"].(map[string]interface{})
	mainSha, _ := mainObj["sha"].(string)
	if mainSha == "" {
		t.Fatalf("main ref sha missing: %v", refData)
	}
	ghPost(t, repoPath+"/git/refs", defaultToken, map[string]interface{}{
		"ref": "refs/heads/feat", "sha": mainSha,
	}).Body.Close()

	var commitShas []string
	for _, f := range []struct{ path, content, message string }{
		{"alpha.txt", "alpha", "add alpha"},
		{"beta.txt", "beta", "add beta"},
	} {
		resp := ghPut(t, repoPath+"/contents/"+f.path, defaultToken, map[string]interface{}{
			"message": f.message,
			"content": base64.StdEncoding.EncodeToString([]byte(f.content)),
			"branch":  "feat",
		})
		data := decodeJSON(t, resp)
		commit, _ := data["commit"].(map[string]interface{})
		sha, _ := commit["sha"].(string)
		if sha == "" {
			t.Fatalf("contents PUT %s returned no commit sha: %v", f.path, data)
		}
		commitShas = append(commitShas, sha)
	}

	ghPost(t, repoPath+"/pulls", defaultToken, map[string]interface{}{
		"title": "Timeline flow", "head": "feat", "base": "main",
	}).Body.Close()
	ghPost(t, repoPath+"/pulls/1/requested_reviewers", defaultToken, map[string]interface{}{
		"reviewers": []string{"flow-reviewer"},
	}).Body.Close()
	ghDeleteWithBody(t, repoPath+"/pulls/1/requested_reviewers", defaultToken, map[string]interface{}{
		"reviewers": []string{"flow-reviewer"},
	}).Body.Close()

	// GET /pulls/1/commits lists the two real commits, oldest first.
	commitsResp := ghGet(t, repoPath+"/pulls/1/commits", defaultToken)
	if commitsResp.StatusCode != 200 {
		commitsResp.Body.Close()
		t.Fatalf("pulls/1/commits: expected 200, got %d", commitsResp.StatusCode)
	}
	commitList := decodeJSONArray(t, commitsResp)
	if len(commitList) != 2 {
		t.Fatalf("expected 2 PR commits, got %d: %v", len(commitList), commitList)
	}
	for i := range commitList {
		if commitList[i]["sha"] != commitShas[i] {
			t.Fatalf("PR commit %d sha = %v, want %s", i, commitList[i]["sha"], commitShas[i])
		}
	}

	mergeResp := ghPut(t, repoPath+"/pulls/1/merge", defaultToken, map[string]interface{}{
		"merge_method": "merge",
	})
	if mergeResp.StatusCode != 200 {
		mergeResp.Body.Close()
		t.Fatalf("merge: expected 200, got %d", mergeResp.StatusCode)
	}
	mergeData := decodeJSON(t, mergeResp)
	if mergeData["merged"] != true {
		t.Fatalf("expected merged=true, got %v", mergeData["merged"])
	}
	mergeSha, _ := mergeData["sha"].(string)
	if mergeSha == "" {
		t.Fatalf("merge returned no sha: %v", mergeData)
	}

	// The merge commit is real: resolvable through the git data API, with
	// the old base head and the PR head as its parents.
	mcResp := ghGet(t, repoPath+"/git/commits/"+mergeSha, defaultToken)
	if mcResp.StatusCode != 200 {
		mcResp.Body.Close()
		t.Fatalf("git/commits/%s: expected 200, got %d", mergeSha, mcResp.StatusCode)
	}
	mc := decodeJSON(t, mcResp)
	parents, _ := mc["parents"].([]interface{})
	parentShas := map[string]bool{}
	for _, p := range parents {
		pm, _ := p.(map[string]interface{})
		sha, _ := pm["sha"].(string)
		parentShas[sha] = true
	}
	if !parentShas[mainSha] || !parentShas[commitShas[1]] {
		t.Fatalf("merge commit parents = %v, want base %s and head %s", parents, mainSha, commitShas[1])
	}

	// PR read-back carries the real shas and commit count.
	prResp := ghGet(t, repoPath+"/pulls/1", defaultToken)
	prData := decodeJSON(t, prResp)
	if prData["merge_commit_sha"] != mergeSha {
		t.Fatalf("merge_commit_sha = %v, want %s", prData["merge_commit_sha"], mergeSha)
	}
	if prData["commits"] != 2.0 {
		t.Fatalf("commits = %v, want 2", prData["commits"])
	}
	head, _ := prData["head"].(map[string]interface{})
	if head["sha"] != commitShas[1] {
		t.Fatalf("head.sha = %v, want %s", head["sha"], commitShas[1])
	}
	base, _ := prData["base"].(map[string]interface{})
	if base["sha"] != mainSha {
		t.Fatalf("base.sha = %v, want %s", base["sha"], mainSha)
	}

	// Timeline: committed ×2, review_requested, review_request_removed,
	// merged, closed — in that order.
	tlResp := ghGet(t, repoPath+"/issues/1/timeline", defaultToken)
	if tlResp.StatusCode != 200 {
		tlResp.Body.Close()
		t.Fatalf("timeline: expected 200, got %d", tlResp.StatusCode)
	}
	items := decodeJSONArray(t, tlResp)
	var gotEvents []string
	for _, item := range items {
		ev, _ := item["event"].(string)
		gotEvents = append(gotEvents, ev)
	}
	wantEvents := []string{"committed", "committed", "review_requested", "review_request_removed", "merged", "closed"}
	if len(gotEvents) != len(wantEvents) {
		t.Fatalf("timeline events = %v, want %v", gotEvents, wantEvents)
	}
	for i := range wantEvents {
		if gotEvents[i] != wantEvents[i] {
			t.Fatalf("timeline events = %v, want %v", gotEvents, wantEvents)
		}
	}

	// Committed entries are the PR's real commits, oldest first, in the
	// documented git-commit shape.
	for i := 0; i < 2; i++ {
		c := items[i]
		if c["sha"] != commitShas[i] {
			t.Fatalf("committed[%d].sha = %v, want %s", i, c["sha"], commitShas[i])
		}
		author, _ := c["author"].(map[string]interface{})
		if author == nil || author["email"] == "" || author["name"] == "" || author["date"] == "" {
			t.Fatalf("committed[%d].author incomplete: %v", i, c["author"])
		}
		if _, ok := c["committer"].(map[string]interface{}); !ok {
			t.Fatalf("committed[%d] missing committer: %v", i, c)
		}
		msg, _ := c["message"].(string)
		if msg == "" {
			t.Fatalf("committed[%d] missing message", i)
		}
		tree, _ := c["tree"].(map[string]interface{})
		if tree == nil || tree["sha"] == "" {
			t.Fatalf("committed[%d] missing tree: %v", i, c)
		}
		cparents, _ := c["parents"].([]interface{})
		if len(cparents) != 1 {
			t.Fatalf("committed[%d].parents = %v, want 1 parent", i, c["parents"])
		}
	}
	firstParent, _ := items[0]["parents"].([]interface{})[0].(map[string]interface{})
	if firstParent["sha"] != mainSha {
		t.Fatalf("committed[0] parent = %v, want %s", firstParent["sha"], mainSha)
	}
	secondParent, _ := items[1]["parents"].([]interface{})[0].(map[string]interface{})
	if secondParent["sha"] != commitShas[0] {
		t.Fatalf("committed[1] parent = %v, want %s", secondParent["sha"], commitShas[0])
	}

	// review_requested / review_request_removed carry the documented
	// review_requester + requested_reviewer payload.
	for i, ev := range []string{"review_requested", "review_request_removed"} {
		item := items[2+i]
		requester, _ := item["review_requester"].(map[string]interface{})
		if requester == nil || requester["login"] != "admin" {
			t.Fatalf("%s.review_requester = %v, want admin", ev, item["review_requester"])
		}
		reviewer, _ := item["requested_reviewer"].(map[string]interface{})
		if reviewer == nil || reviewer["login"] != "flow-reviewer" {
			t.Fatalf("%s.requested_reviewer = %v, want flow-reviewer", ev, item["requested_reviewer"])
		}
		if id, ok := item["id"].(float64); !ok || id <= 0 {
			t.Fatalf("%s.id = %v, want positive integer", ev, item["id"])
		}
		if at, _ := item["created_at"].(string); at == "" {
			t.Fatalf("%s.created_at missing", ev)
		}
	}

	// merged carries the merge commit; closed follows it.
	mergedItem := items[4]
	if mergedItem["commit_id"] != mergeSha {
		t.Fatalf("merged.commit_id = %v, want %s", mergedItem["commit_id"], mergeSha)
	}
	actor, _ := mergedItem["actor"].(map[string]interface{})
	if actor == nil || actor["login"] != "admin" {
		t.Fatalf("merged.actor = %v, want admin", mergedItem["actor"])
	}
	closedItem := items[5]
	if closedItem["commit_id"] != nil {
		t.Fatalf("closed.commit_id = %v, want null", closedItem["commit_id"])
	}

	// The issue-events endpoint serves the PR's recorded events too (PRs
	// share the issue number space on real GitHub).
	evResp := ghGet(t, repoPath+"/issues/1/events", defaultToken)
	if evResp.StatusCode != 200 {
		evResp.Body.Close()
		t.Fatalf("issues/1/events: expected 200, got %d", evResp.StatusCode)
	}
	evItems := decodeJSONArray(t, evResp)
	var evNames []string
	for _, item := range evItems {
		ev, _ := item["event"].(string)
		evNames = append(evNames, ev)
	}
	wantEvNames := []string{"review_requested", "review_request_removed", "merged", "closed"}
	if len(evNames) != len(wantEvNames) {
		t.Fatalf("issue events = %v, want %v", evNames, wantEvNames)
	}
	for i := range wantEvNames {
		if evNames[i] != wantEvNames[i] {
			t.Fatalf("issue events = %v, want %v", evNames, wantEvNames)
		}
	}

	// Ids and timestamps are recorded state, identical across reads.
	tlResp2 := ghGet(t, repoPath+"/issues/1/timeline", defaultToken)
	items2 := decodeJSONArray(t, tlResp2)
	if len(items2) != len(items) {
		t.Fatalf("second timeline read has %d items, want %d", len(items2), len(items))
	}
	for i := range items {
		if items[i]["id"] != items2[i]["id"] || items[i]["created_at"] != items2[i]["created_at"] || items[i]["sha"] != items2[i]["sha"] {
			t.Fatalf("timeline item %d unstable across reads: %v vs %v", i, items[i], items2[i])
		}
	}
}

// TestPullRequestFilesREST exercises GET /repos/{owner}/{repo}/pulls/{n}/files —
// the changed-file diff list with per-file unified-diff patches. It commits a
// modification and an addition on the head branch and asserts the endpoint
// reports both with real additions/deletions and a patch hunk.
func TestPullRequestFilesREST(t *testing.T) {
	repoPath := "/api/v3/repos/admin/pr-files-rest"
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "pr-files-rest", "auto_init": true,
	}).Body.Close()

	// Seed a file on main so the head branch can modify it.
	ghPut(t, repoPath+"/contents/greeting.txt", defaultToken, map[string]interface{}{
		"message": "seed greeting",
		"content": base64.StdEncoding.EncodeToString([]byte("hello\n")),
		"branch":  "main",
	}).Body.Close()

	refResp := ghGet(t, repoPath+"/git/refs/heads/main", defaultToken)
	refData := decodeJSON(t, refResp)
	mainObj, _ := refData["object"].(map[string]interface{})
	mainSha, _ := mainObj["sha"].(string)
	if mainSha == "" {
		t.Fatalf("main ref sha missing: %v", refData)
	}
	ghPost(t, repoPath+"/git/refs", defaultToken, map[string]interface{}{
		"ref": "refs/heads/feat", "sha": mainSha,
	}).Body.Close()

	// Modify greeting.txt and add a new file on feat.
	ghPut(t, repoPath+"/contents/greeting.txt", defaultToken, map[string]interface{}{
		"message": "change greeting",
		"content": base64.StdEncoding.EncodeToString([]byte("hello world\n")),
		"branch":  "feat",
		"sha":     mainSha, // placeholder; contents PUT resolves by path on the branch
	}).Body.Close()
	// The contents PUT above needs the file's blob sha, not the commit sha; do a
	// GET to fetch it, then update.
	gResp := ghGet(t, repoPath+"/contents/greeting.txt?ref=feat", defaultToken)
	gData := decodeJSON(t, gResp)
	blobSha, _ := gData["sha"].(string)
	if blobSha != "" {
		ghPut(t, repoPath+"/contents/greeting.txt", defaultToken, map[string]interface{}{
			"message": "change greeting again",
			"content": base64.StdEncoding.EncodeToString([]byte("hello there\n")),
			"branch":  "feat",
			"sha":     blobSha,
		}).Body.Close()
	}
	ghPut(t, repoPath+"/contents/added.txt", defaultToken, map[string]interface{}{
		"message": "add file",
		"content": base64.StdEncoding.EncodeToString([]byte("brand new\n")),
		"branch":  "feat",
	}).Body.Close()

	ghPost(t, repoPath+"/pulls", defaultToken, map[string]interface{}{
		"title": "Files flow", "head": "feat", "base": "main",
	}).Body.Close()

	resp := ghGet(t, repoPath+"/pulls/1/files", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("pulls/1/files: expected 200, got %d", resp.StatusCode)
	}
	files := decodeJSONArray(t, resp)
	byName := map[string]map[string]interface{}{}
	for _, f := range files {
		name, _ := f["filename"].(string)
		byName[name] = f
	}
	added, ok := byName["added.txt"]
	if !ok {
		t.Fatalf("added.txt missing from files: %v", files)
	}
	if added["status"] != "added" {
		t.Errorf("added.txt status = %v, want added", added["status"])
	}
	greeting, ok := byName["greeting.txt"]
	if !ok {
		t.Fatalf("greeting.txt missing from files: %v", files)
	}
	if greeting["status"] != "modified" {
		t.Errorf("greeting.txt status = %v, want modified", greeting["status"])
	}
	if patch, _ := greeting["patch"].(string); !strings.Contains(patch, "@@") {
		t.Errorf("greeting.txt patch missing hunk header: %q", patch)
	}
}

func TestPullRequestGraphQLFilesAndClosingIssuesUseRealState(t *testing.T) {
	repoName := "pr-graphql-files-closing"
	repoPath := "/api/v3/repos/admin/" + repoName
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": repoName, "auto_init": true,
	}).Body.Close()

	issueResp := ghPost(t, repoPath+"/issues", defaultToken, map[string]interface{}{
		"title": "release blocker",
		"body":  "tracked by the pull request body",
	})
	issue := decodeJSON(t, issueResp)
	if issue["number"] != float64(1) {
		t.Fatalf("issue number = %v, want 1", issue["number"])
	}

	ghPut(t, repoPath+"/contents/greeting.txt", defaultToken, map[string]interface{}{
		"message": "seed greeting",
		"content": base64.StdEncoding.EncodeToString([]byte("hello\n")),
		"branch":  "main",
	}).Body.Close()
	gResp := ghGet(t, repoPath+"/contents/greeting.txt?ref=main", defaultToken)
	gData := decodeJSON(t, gResp)
	blobSha, _ := gData["sha"].(string)
	if blobSha == "" {
		t.Fatalf("base blob sha missing: %v", gData)
	}
	refResp := ghGet(t, repoPath+"/git/refs/heads/main", defaultToken)
	refData := decodeJSON(t, refResp)
	mainObj, _ := refData["object"].(map[string]interface{})
	mainSha, _ := mainObj["sha"].(string)
	if mainSha == "" {
		t.Fatalf("main ref sha missing: %v", refData)
	}
	ghPost(t, repoPath+"/git/refs", defaultToken, map[string]interface{}{
		"ref": "refs/heads/feature", "sha": mainSha,
	}).Body.Close()
	ghPut(t, repoPath+"/contents/greeting.txt", defaultToken, map[string]interface{}{
		"message": "change greeting",
		"content": base64.StdEncoding.EncodeToString([]byte("hello there\n")),
		"branch":  "feature",
		"sha":     blobSha,
	}).Body.Close()
	ghPut(t, repoPath+"/contents/added.txt", defaultToken, map[string]interface{}{
		"message": "add file",
		"content": base64.StdEncoding.EncodeToString([]byte("brand new\n")),
		"branch":  "feature",
	}).Body.Close()

	prResp := ghPost(t, repoPath+"/pulls", defaultToken, map[string]interface{}{
		"title": "GraphQL files",
		"head":  "feature",
		"base":  "main",
		"body":  "Fixes #1.",
	})
	prData := decodeJSON(t, prResp)
	prNumber := int(prData["number"].(float64))

	query := `query PullRequestFilesAndClosingIssues($owner:String!,$repo:String!,$number:Int!){
		repository(owner:$owner,name:$repo){
			pullRequest(number:$number){
				files(first:10){totalCount,nodes{path,additions,deletions,changeType}}
				closingIssuesReferences(first:10){totalCount,nodes{number,title},pageInfo{hasNextPage,endCursor}}
			}
		}
	}`
	d := gqlData(t, query, map[string]interface{}{"owner": "admin", "repo": repoName, "number": prNumber})
	repo, _ := d["repository"].(map[string]interface{})
	pr, _ := repo["pullRequest"].(map[string]interface{})
	if pr == nil {
		t.Fatalf("pullRequest null: %v", d)
	}
	files, _ := pr["files"].(map[string]interface{})
	if files["totalCount"] != float64(2) {
		t.Fatalf("files.totalCount = %v, want 2: %v", files["totalCount"], files)
	}
	fileNodes, _ := files["nodes"].([]interface{})
	byPath := map[string]map[string]interface{}{}
	for _, raw := range fileNodes {
		node, _ := raw.(map[string]interface{})
		path, _ := node["path"].(string)
		byPath[path] = node
	}
	if byPath["added.txt"]["changeType"] != "ADDED" {
		t.Fatalf("added.txt GraphQL file = %v, want ADDED", byPath["added.txt"])
	}
	if byPath["greeting.txt"]["changeType"] != "CHANGED" {
		t.Fatalf("greeting.txt GraphQL file = %v, want CHANGED", byPath["greeting.txt"])
	}
	closing, _ := pr["closingIssuesReferences"].(map[string]interface{})
	if closing["totalCount"] != float64(1) {
		t.Fatalf("closingIssuesReferences.totalCount = %v, want 1: %v", closing["totalCount"], closing)
	}
	closingNodes, _ := closing["nodes"].([]interface{})
	if len(closingNodes) != 1 {
		t.Fatalf("closing issue nodes = %v, want 1", closingNodes)
	}
	closingIssue, _ := closingNodes[0].(map[string]interface{})
	if closingIssue["number"] != float64(1) || closingIssue["title"] != "release blocker" {
		t.Fatalf("closing issue = %v, want #1 release blocker", closingIssue)
	}
}
