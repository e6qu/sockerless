package bleephub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func pullsTestServer(t *testing.T) (*Server, *User, *Repo) {
	t.Helper()
	s := newTestServer()
	s.registerGHPullRoutes()
	s.registerGHIssueRoutes()
	s.registerGHMiscEndpoints()
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "pr-unit-test", "", false)
	seedPullRequestBranches(t, s, repo, "feature", "feat", "fix", "branch", "r", "f", "a", "b", "feat-a", "feat-b", "draft-branch")
	return s, admin, repo
}

func doPullsReq(s *Server, method, path string, body string) *httptest.ResponseRecorder {
	return doMiscReq(s, method, path, body)
}

func assertJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("response not JSON: %v; body=%s", err, w.Body.String())
	}
	return m
}

func assertJSONArray(t *testing.T, w *httptest.ResponseRecorder) []map[string]interface{} {
	t.Helper()
	var arr []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &arr); err != nil {
		t.Fatalf("response not JSON array: %v; body=%s", err, w.Body.String())
	}
	return arr
}

func TestUnitCreatePR_Basic(t *testing.T) {
	s, admin, repo := pullsTestServer(t)
	issue := s.store.CreateIssue(repo.ID, admin.ID, "issue-for-pr", "", nil, nil, 0)
	if issue == nil {
		t.Fatal("failed to create seed issue")
	}

	body := `{"title":"Fix typo","body":"nit fix","head":"feature","base":"main","draft":false}`
	w := doPullsReq(s, "POST", "/api/v3/repos/admin/pr-unit-test/pulls", body)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	data := assertJSON(t, w)

	if data["number"] != float64(2) {
		t.Fatalf("number = %v, want 2 (issue consumed #1)", data["number"])
	}
	if data["title"] != "Fix typo" {
		t.Fatalf("title = %v, want 'Fix typo'", data["title"])
	}
	if data["state"] != "open" {
		t.Fatalf("state = %v, want 'open'", data["state"])
	}
	if data["draft"] != false {
		t.Fatalf("draft = %v, want false", data["draft"])
	}
	head, _ := data["head"].(map[string]interface{})
	if head == nil || head["ref"] != "feature" {
		t.Fatalf("head.ref = %v, want 'feature'", head)
	}
	base, _ := data["base"].(map[string]interface{})
	if base == nil || base["ref"] != "main" {
		t.Fatalf("base.ref = %v, want 'main'", base)
	}
	user, _ := data["user"].(map[string]interface{})
	if user == nil || user["login"] != "admin" {
		t.Fatalf("user.login = %v, want 'admin'", user)
	}
}

func TestUnitCreatePR_NoBody(t *testing.T) {
	s, _, _ := pullsTestServer(t)

	w := doPullsReq(s, "POST", "/api/v3/repos/admin/pr-unit-test/pulls", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	data := assertJSON(t, w)
	msg, _ := data["message"].(string)
	if msg != "Problems parsing JSON" {
		t.Fatalf("message = %q, want 'Problems parsing JSON'", msg)
	}
}

func TestUnitCreatePR_MissingHead(t *testing.T) {
	s, _, _ := pullsTestServer(t)

	w := doPullsReq(s, "POST", "/api/v3/repos/admin/pr-unit-test/pulls", `{"title":"no head","base":"main"}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", w.Code, w.Body.String())
	}
	data := assertJSON(t, w)
	msg, _ := data["message"].(string)
	if msg != "Validation Failed" {
		t.Fatalf("message = %q, want 'Validation Failed'", msg)
	}
}

func TestUnitListPRs(t *testing.T) {
	s, admin, repo := pullsTestServer(t)

	pr1 := s.store.CreatePullRequest(repo.ID, admin.ID, "PR one", "", "feat-a", "main", false, nil, nil, 0)
	pr2 := s.store.CreatePullRequest(repo.ID, admin.ID, "PR two", "", "feat-b", "main", false, nil, nil, 0)
	if pr1 == nil || pr2 == nil {
		t.Fatal("failed to create PRs")
	}

	w := doPullsReq(s, "GET", "/api/v3/repos/admin/pr-unit-test/pulls", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	arr := assertJSONArray(t, w)
	if len(arr) != 2 {
		t.Fatalf("len = %d, want 2", len(arr))
	}
}

func TestUnitListPRs_Empty(t *testing.T) {
	s, _, _ := pullsTestServer(t)

	w := doPullsReq(s, "GET", "/api/v3/repos/admin/pr-unit-test/pulls", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	arr := assertJSONArray(t, w)
	if len(arr) != 0 {
		t.Fatalf("len = %d, want 0", len(arr))
	}
}

func TestUnitGetPR(t *testing.T) {
	s, admin, repo := pullsTestServer(t)
	pr := s.store.CreatePullRequest(repo.ID, admin.ID, "Get me", "", "branch", "main", false, nil, nil, 0)
	if pr == nil {
		t.Fatal("failed to create PR")
	}

	w := doPullsReq(s, "GET", "/api/v3/repos/admin/pr-unit-test/pulls/"+strconv.Itoa(pr.Number), "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := assertJSON(t, w)
	if data["title"] != "Get me" {
		t.Fatalf("title = %v, want 'Get me'", data["title"])
	}
	if data["number"] != float64(pr.Number) {
		t.Fatalf("number = %v, want %d", data["number"], pr.Number)
	}
}

func TestUnitGetPR_NotFound(t *testing.T) {
	s, _, _ := pullsTestServer(t)

	w := doPullsReq(s, "GET", "/api/v3/repos/admin/pr-unit-test/pulls/9999", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
}

func TestUnitGetPR_RepoNotFound(t *testing.T) {
	s, _, _ := pullsTestServer(t)

	w := doPullsReq(s, "GET", "/api/v3/repos/admin/no-such-repo/pulls/1", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
}

func TestUnitUpdatePR_Merge(t *testing.T) {
	s, admin, repo := pullsTestServer(t)
	pr := s.store.CreatePullRequest(repo.ID, admin.ID, "To close", "", "fix", "main", false, nil, nil, 0)
	if pr == nil {
		t.Fatal("failed to create PR")
	}

	w := doPullsReq(s, "PATCH", "/api/v3/repos/admin/pr-unit-test/pulls/"+strconv.Itoa(pr.Number),
		`{"state":"closed"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := assertJSON(t, w)
	if data["state"] != "closed" {
		t.Fatalf("state = %v, want 'closed'", data["state"])
	}
	if data["closed_at"] == nil {
		t.Fatal("closed_at should be set")
	}
}

func TestUnitUpdatePR_Reopen(t *testing.T) {
	s, admin, repo := pullsTestServer(t)
	pr := s.store.CreatePullRequest(repo.ID, admin.ID, "Reopen me", "", "fix", "main", false, nil, nil, 0)
	if pr == nil {
		t.Fatal("failed to create PR")
	}

	s.store.UpdatePullRequest(pr.ID, func(p *PullRequest) {
		p.State = "CLOSED"
	})

	w := doPullsReq(s, "PATCH", "/api/v3/repos/admin/pr-unit-test/pulls/"+strconv.Itoa(pr.Number),
		`{"state":"open"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := assertJSON(t, w)
	if data["state"] != "open" {
		t.Fatalf("state = %v, want 'open'", data["state"])
	}
}

func TestUnitUpdatePR_NotFound(t *testing.T) {
	s, _, _ := pullsTestServer(t)

	w := doPullsReq(s, "PATCH", "/api/v3/repos/admin/pr-unit-test/pulls/9999",
		`{"title":"won't work"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestUnitMergePR_Success(t *testing.T) {
	s, admin, repo := pullsTestServer(t)
	pr := s.store.CreatePullRequest(repo.ID, admin.ID, "Merge me", "", "feat", "main", false, nil, nil, 0)
	if pr == nil {
		t.Fatal("failed to create PR")
	}

	w := doPullsReq(s, "PUT", "/api/v3/repos/admin/pr-unit-test/pulls/"+strconv.Itoa(pr.Number)+"/merge", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := assertJSON(t, w)
	if data["merged"] != true {
		t.Fatalf("merged = %v, want true", data["merged"])
	}
	sha, _ := data["sha"].(string)
	if sha == "" || len(sha) != 40 {
		t.Fatalf("sha = %q, want 40-char hex", sha)
	}

	updated := s.store.GetPullRequest(pr.ID)
	if updated.State != "MERGED" {
		t.Fatalf("store state = %q, want 'MERGED'", updated.State)
	}
}

func TestUnitMergePR_AlreadyMerged(t *testing.T) {
	s, admin, repo := pullsTestServer(t)
	pr := s.store.CreatePullRequest(repo.ID, admin.ID, "Merge me twice", "", "feat", "main", false, nil, nil, 0)
	if pr == nil {
		t.Fatal("failed to create PR")
	}

	s.store.UpdatePullRequest(pr.ID, func(p *PullRequest) {
		p.State = "MERGED"
	})

	w := doPullsReq(s, "PUT", "/api/v3/repos/admin/pr-unit-test/pulls/"+strconv.Itoa(pr.Number)+"/merge", "")
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405; body = %s", w.Code, w.Body.String())
	}
}

func TestUnitMergePR_Closed(t *testing.T) {
	s, admin, repo := pullsTestServer(t)
	pr := s.store.CreatePullRequest(repo.ID, admin.ID, "Closed PR", "", "feat", "main", false, nil, nil, 0)
	if pr == nil {
		t.Fatal("failed to create PR")
	}

	s.store.UpdatePullRequest(pr.ID, func(p *PullRequest) {
		p.State = "CLOSED"
	})

	w := doPullsReq(s, "PUT", "/api/v3/repos/admin/pr-unit-test/pulls/"+strconv.Itoa(pr.Number)+"/merge", "")
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", w.Code, w.Body.String())
	}
}

func TestUnitCreatePR_RepoNotFound(t *testing.T) {
	s, _, _ := pullsTestServer(t)

	w := doPullsReq(s, "POST", "/api/v3/repos/admin/no-such-repo/pulls",
		`{"title":"X","head":"f","base":"main"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
}

func TestUnitCreatePR_Draft(t *testing.T) {
	s, _, _ := pullsTestServer(t)

	w := doPullsReq(s, "POST", "/api/v3/repos/admin/pr-unit-test/pulls",
		`{"title":"Draft PR","head":"draft-branch","base":"main","draft":true}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	data := assertJSON(t, w)
	if data["draft"] != true {
		t.Fatalf("draft = %v, want true", data["draft"])
	}
}

func TestUnitListPRs_StateFilter(t *testing.T) {
	s, admin, repo := pullsTestServer(t)
	s.store.CreatePullRequest(repo.ID, admin.ID, "Open PR", "", "a", "main", false, nil, nil, 0)
	closedPR := s.store.CreatePullRequest(repo.ID, admin.ID, "Closed PR", "", "b", "main", false, nil, nil, 0)
	if closedPR == nil {
		t.Fatal("failed to create closed PR")
	}
	s.store.UpdatePullRequest(closedPR.ID, func(p *PullRequest) {
		p.State = "CLOSED"
	})

	w := doPullsReq(s, "GET", "/api/v3/repos/admin/pr-unit-test/pulls?state=open", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	arr := assertJSONArray(t, w)
	if len(arr) != 1 {
		t.Fatalf("open PRs len = %d, want 1", len(arr))
	}

	w2 := doPullsReq(s, "GET", "/api/v3/repos/admin/pr-unit-test/pulls?state=closed", "")
	arr2 := assertJSONArray(t, w2)
	if len(arr2) != 1 {
		t.Fatalf("closed PRs len = %d, want 1", len(arr2))
	}

	w3 := doPullsReq(s, "GET", "/api/v3/repos/admin/pr-unit-test/pulls?state=all", "")
	arr3 := assertJSONArray(t, w3)
	if len(arr3) != 2 {
		t.Fatalf("all PRs len = %d, want 2", len(arr3))
	}
}

func TestUnitCreatePRReview(t *testing.T) {
	s, admin, repo := pullsTestServer(t)
	pr := s.store.CreatePullRequest(repo.ID, admin.ID, "Review me", "", "r", "main", false, nil, nil, 0)
	if pr == nil {
		t.Fatal("failed to create PR")
	}

	w := doPullsReq(s, "POST", "/api/v3/repos/admin/pr-unit-test/pulls/"+strconv.Itoa(pr.Number)+"/reviews",
		`{"body":"LGTM","event":"APPROVE"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := assertJSON(t, w)
	if data["state"] != "APPROVED" {
		t.Fatalf("state = %v, want 'APPROVED'", data["state"])
	}
	if data["body"] != "LGTM" {
		t.Fatalf("body = %v, want 'LGTM'", data["body"])
	}
}

func TestUnitListPRReviews(t *testing.T) {
	s, admin, repo := pullsTestServer(t)
	pr := s.store.CreatePullRequest(repo.ID, admin.ID, "Reviewed", "", "r", "main", false, nil, nil, 0)
	if pr == nil {
		t.Fatal("failed to create PR")
	}
	s.store.CreatePRReview(pr.ID, admin.ID, "APPROVED", "ok")
	s.store.CreatePRReview(pr.ID, admin.ID, "COMMENTED", "nit")

	w := doPullsReq(s, "GET", "/api/v3/repos/admin/pr-unit-test/pulls/"+strconv.Itoa(pr.Number)+"/reviews", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	arr := assertJSONArray(t, w)
	if len(arr) != 2 {
		t.Fatalf("len = %d, want 2", len(arr))
	}
}

func TestUnitRequestReviewers(t *testing.T) {
	s, admin, repo := pullsTestServer(t)
	pr := s.store.CreatePullRequest(repo.ID, admin.ID, "Need review", "", "f", "main", false, nil, nil, 0)
	if pr == nil {
		t.Fatal("failed to create PR")
	}

	w := doPullsReq(s, "POST", "/api/v3/repos/admin/pr-unit-test/pulls/"+strconv.Itoa(pr.Number)+"/requested_reviewers",
		`{"reviewers":["admin"]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	data := assertJSON(t, w)
	if data["number"] != float64(pr.Number) {
		t.Fatalf("number = %v, want %d", data["number"], pr.Number)
	}
}
