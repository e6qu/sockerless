package bleephub

import (
	"net/http"
	"strings"
	"testing"
)

// --- Error format conformance ---

func TestConformance401ErrorFormat(t *testing.T) {
	resp := ghGet(t, "/api/v3/user", "")
	if resp.StatusCode != 401 {
		resp.Body.Close()
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	msg, _ := data["message"].(string)
	if msg != "Bad credentials" {
		t.Fatalf("expected 'Bad credentials', got %q", msg)
	}
	docURL, _ := data["documentation_url"].(string)
	if docURL == "" {
		t.Fatal("missing documentation_url in 401 error")
	}
}

func TestConformance404ErrorFormat(t *testing.T) {
	resp := ghGet(t, "/api/v3/repos/nobody/nothing", "")
	if resp.StatusCode != 404 {
		resp.Body.Close()
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	msg, _ := data["message"].(string)
	if msg != "Not Found" {
		t.Fatalf("expected 'Not Found', got %q", msg)
	}
	docURL, _ := data["documentation_url"].(string)
	if docURL == "" {
		t.Fatal("missing documentation_url in 404 error")
	}
}

func TestConformance422ValidationErrorFormat(t *testing.T) {
	// Try to create repo with empty name
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "",
	})
	if resp.StatusCode != 422 {
		resp.Body.Close()
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	msg, _ := data["message"].(string)
	if msg != "Validation Failed" {
		t.Fatalf("expected 'Validation Failed', got %q", msg)
	}
	docURL, _ := data["documentation_url"].(string)
	if docURL == "" {
		t.Fatal("missing documentation_url in 422 error")
	}

	errors, _ := data["errors"].([]interface{})
	if len(errors) == 0 {
		t.Fatal("expected errors array in 422 response")
	}

	errObj, _ := errors[0].(map[string]interface{})
	if errObj["resource"] == nil {
		t.Fatal("missing 'resource' in error object")
	}
	if errObj["field"] == nil {
		t.Fatal("missing 'field' in error object")
	}
	if errObj["code"] == nil {
		t.Fatal("missing 'code' in error object")
	}
}

func TestConformance422IssueValidation(t *testing.T) {
	createTestIssueRepo(t, "conf-issue-422")

	resp := ghPost(t, "/api/v3/repos/admin/conf-issue-422/issues", defaultToken, map[string]interface{}{
		"title": "",
	})
	if resp.StatusCode != 422 {
		resp.Body.Close()
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	errors, _ := data["errors"].([]interface{})
	if len(errors) == 0 {
		t.Fatal("expected errors array for issue validation")
	}
	errObj, _ := errors[0].(map[string]interface{})
	if errObj["resource"] != "Issue" {
		t.Fatalf("expected resource=Issue, got %v", errObj["resource"])
	}
}

func TestConformance422LabelValidation(t *testing.T) {
	createTestIssueRepo(t, "conf-label-422")

	resp := ghPost(t, "/api/v3/repos/admin/conf-label-422/labels", defaultToken, map[string]interface{}{
		"name": "",
	})
	if resp.StatusCode != 422 {
		resp.Body.Close()
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	errors, _ := data["errors"].([]interface{})
	if len(errors) == 0 {
		t.Fatal("expected errors array for label validation")
	}
	errObj, _ := errors[0].(map[string]interface{})
	if errObj["resource"] != "Label" {
		t.Fatalf("expected resource=Label, got %v", errObj["resource"])
	}
}

func TestConformance422DuplicateLabel(t *testing.T) {
	createTestIssueRepo(t, "conf-label-dup")

	ghPost(t, "/api/v3/repos/admin/conf-label-dup/labels", defaultToken, map[string]interface{}{
		"name": "bug", "color": "d73a4a",
	}).Body.Close()

	resp := ghPost(t, "/api/v3/repos/admin/conf-label-dup/labels", defaultToken, map[string]interface{}{
		"name": "bug", "color": "d73a4a",
	})
	if resp.StatusCode != 422 {
		resp.Body.Close()
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	errors, _ := data["errors"].([]interface{})
	if len(errors) == 0 {
		t.Fatal("expected errors array for duplicate label")
	}
	errObj, _ := errors[0].(map[string]interface{})
	if errObj["code"] != "already_exists" {
		t.Fatalf("expected code=already_exists, got %v", errObj["code"])
	}
}

func TestConformance422PRValidation(t *testing.T) {
	createTestPRRepo(t, "conf-pr-422")

	resp := ghPost(t, "/api/v3/repos/admin/conf-pr-422/pulls", defaultToken, map[string]interface{}{
		"title": "No head",
		"head":  "",
		"base":  "main",
	})
	if resp.StatusCode != 422 {
		resp.Body.Close()
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	errors, _ := data["errors"].([]interface{})
	if len(errors) == 0 {
		t.Fatal("expected errors array for PR validation")
	}
	errObj, _ := errors[0].(map[string]interface{})
	if errObj["resource"] != "PullRequest" {
		t.Fatalf("expected resource=PullRequest, got %v", errObj["resource"])
	}
}

// --- Content negotiation / charset ---

func TestConformanceContentTypeCharset(t *testing.T) {
	resp := ghGet(t, "/api/v3/user", defaultToken)
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "charset=utf-8") {
		t.Fatalf("expected charset=utf-8 in Content-Type, got %s", ct)
	}
}

func TestConformanceContentTypeOnError(t *testing.T) {
	resp := ghGet(t, "/api/v3/user", "")
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "charset=utf-8") {
		t.Fatalf("expected charset=utf-8 in error Content-Type, got %s", ct)
	}
}

func TestConformanceAcceptHeader(t *testing.T) {
	// application/vnd.github+json should be accepted
	req, _ := newGHRequest("GET", testBaseURL+"/api/v3/user", defaultToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := doGHRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 with vnd.github+json Accept, got %d", resp.StatusCode)
	}
}

func TestConformanceApiVersionHeader(t *testing.T) {
	resp := ghGet(t, "/api/v3/user", defaultToken)
	defer resp.Body.Close()

	ver := resp.Header.Get("X-GitHub-Api-Version")
	if ver != "2022-11-28" {
		t.Fatalf("expected X-GitHub-Api-Version=2022-11-28, got %s", ver)
	}
}

// --- Rate limit headers ---

func TestConformanceRateLimitOnREST(t *testing.T) {
	resp := ghGet(t, "/api/v3/user", defaultToken)
	defer resp.Body.Close()

	for _, header := range []string{
		"X-RateLimit-Limit",
		"X-RateLimit-Remaining",
		"X-RateLimit-Used",
		"X-RateLimit-Reset",
		"X-RateLimit-Resource",
	} {
		if resp.Header.Get(header) == "" {
			t.Fatalf("missing %s on REST endpoint", header)
		}
	}
	if resp.Header.Get("X-RateLimit-Resource") != "core" {
		t.Fatalf("expected X-RateLimit-Resource=core, got %s", resp.Header.Get("X-RateLimit-Resource"))
	}
}

func TestConformanceRateLimitOnGraphQL(t *testing.T) {
	resp := ghPost(t, "/api/graphql", defaultToken, map[string]string{
		"query": "{viewer{login}}",
	})
	defer resp.Body.Close()

	if resp.Header.Get("X-RateLimit-Resource") != "graphql" {
		t.Fatalf("expected X-RateLimit-Resource=graphql, got %s", resp.Header.Get("X-RateLimit-Resource"))
	}
}

func TestConformanceRateLimitEndpoint(t *testing.T) {
	resp := ghGet(t, "/api/v3/rate_limit", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	resources, _ := data["resources"].(map[string]interface{})
	if resources == nil {
		t.Fatal("missing resources in rate_limit response")
	}
	core, _ := resources["core"].(map[string]interface{})
	if core == nil {
		t.Fatal("missing core in resources")
	}
	if core["limit"] == nil {
		t.Fatal("missing limit in core resource")
	}
	graphql, _ := resources["graphql"].(map[string]interface{})
	if graphql == nil {
		t.Fatal("missing graphql in resources")
	}
	rate, _ := data["rate"].(map[string]interface{})
	if rate == nil {
		t.Fatal("missing rate in rate_limit response")
	}
}

// --- Cross-endpoint consistency (gh api tests) ---

func TestGHApiRepoCreateThenGraphQL(t *testing.T) {
	// Create via REST
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":        "cross-repo-1",
		"description": "Cross-check",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Query via GraphQL
	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]string{
		"query": `{repository(owner:"admin",name:"cross-repo-1"){name,description}}`,
	})
	data := decodeJSON(t, resp2)
	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	if repo["name"] != "cross-repo-1" {
		t.Fatalf("expected name=cross-repo-1, got %v", repo["name"])
	}
	if repo["description"] != "Cross-check" {
		t.Fatalf("expected description=Cross-check, got %v", repo["description"])
	}
}

func TestGHApiIssueCreateThenGraphQL(t *testing.T) {
	createTestIssueRepo(t, "cross-issue-1")

	// Create via REST
	resp := ghPost(t, "/api/v3/repos/admin/cross-issue-1/issues", defaultToken, map[string]interface{}{
		"title": "Cross issue",
		"body":  "Created via REST",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Query via GraphQL
	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"cross-issue-1"){issue(number:1){title,body,state}}}`,
	})
	data := decodeJSON(t, resp2)
	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	issue, _ := repo["issue"].(map[string]interface{})
	if issue["title"] != "Cross issue" {
		t.Fatalf("expected title='Cross issue', got %v", issue["title"])
	}
	if issue["body"] != "Created via REST" {
		t.Fatalf("expected body='Created via REST', got %v", issue["body"])
	}
	if issue["state"] != "OPEN" {
		t.Fatalf("expected state=OPEN, got %v", issue["state"])
	}
}

func TestGHApiPRCreateThenGraphQL(t *testing.T) {
	createTestPRRepo(t, "cross-pr-1")

	resp := ghPost(t, "/api/v3/repos/admin/cross-pr-1/pulls", defaultToken, map[string]interface{}{
		"title": "Cross PR",
		"body":  "Via REST",
		"head":  "feature",
		"base":  "main",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"cross-pr-1"){pullRequest(number:1){title,body,state,headRefName,baseRefName}}}`,
	})
	data := decodeJSON(t, resp2)
	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	pr, _ := repo["pullRequest"].(map[string]interface{})
	if pr["title"] != "Cross PR" {
		t.Fatalf("expected title='Cross PR', got %v", pr["title"])
	}
	if pr["headRefName"] != "feature" {
		t.Fatalf("expected headRefName=feature, got %v", pr["headRefName"])
	}
}

func TestGHApiLabelCreateThenGraphQL(t *testing.T) {
	createTestIssueRepo(t, "cross-label-1")

	resp := ghPost(t, "/api/v3/repos/admin/cross-label-1/labels", defaultToken, map[string]interface{}{
		"name": "cross-bug", "color": "d73a4a", "description": "A bug label",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp2 := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"cross-label-1"){labels(first:10){nodes{name,color,description}}}}`,
	})
	data := decodeJSON(t, resp2)
	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	labels, _ := repo["labels"].(map[string]interface{})
	nodes, _ := labels["nodes"].([]interface{})
	if len(nodes) == 0 {
		t.Fatal("expected at least 1 label via GraphQL")
	}
	lbl, _ := nodes[0].(map[string]interface{})
	if lbl["name"] != "cross-bug" {
		t.Fatalf("expected name=cross-bug, got %v", lbl["name"])
	}
}

func TestGHApiGraphQLViewerLogin(t *testing.T) {
	resp := ghPost(t, "/api/graphql", defaultToken, map[string]string{
		"query": "{ viewer { login } }",
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	d, _ := data["data"].(map[string]interface{})
	viewer, _ := d["viewer"].(map[string]interface{})
	if viewer["login"] != "admin" {
		t.Fatalf("expected login=admin, got %v", viewer["login"])
	}
}

func TestGHApiGraphQLRepoQuery(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "cross-gql-repo",
	}).Body.Close()

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"cross-gql-repo"){name,owner{login},isPrivate}}`,
	})
	data := decodeJSON(t, resp)
	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	if repo["name"] != "cross-gql-repo" {
		t.Fatalf("expected name=cross-gql-repo, got %v", repo["name"])
	}
	owner, _ := repo["owner"].(map[string]interface{})
	if owner["login"] != "admin" {
		t.Fatalf("expected owner.login=admin, got %v", owner)
	}
}

func TestGHApiGraphQLIssuesQuery(t *testing.T) {
	createTestIssueRepo(t, "cross-gql-issues")
	ghPost(t, "/api/v3/repos/admin/cross-gql-issues/issues", defaultToken, map[string]interface{}{
		"title": "GQL query issue",
	}).Body.Close()

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"cross-gql-issues"){issues(first:5,states:[OPEN]){totalCount,nodes{title,number}}}}`,
	})
	data := decodeJSON(t, resp)
	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	issues, _ := repo["issues"].(map[string]interface{})
	tc, _ := issues["totalCount"].(float64)
	if tc < 1 {
		t.Fatalf("expected totalCount >= 1, got %v", tc)
	}
}

func TestGHApiGraphQLPRsQuery(t *testing.T) {
	createTestPRRepo(t, "cross-gql-prs")
	ghPost(t, "/api/v3/repos/admin/cross-gql-prs/pulls", defaultToken, map[string]interface{}{
		"title": "GQL query PR", "head": "feat", "base": "main",
	}).Body.Close()

	resp := ghPost(t, "/api/graphql", defaultToken, map[string]interface{}{
		"query": `{repository(owner:"admin",name:"cross-gql-prs"){pullRequests(first:5,states:[OPEN]){totalCount,nodes{title,number}}}}`,
	})
	data := decodeJSON(t, resp)
	d, _ := data["data"].(map[string]interface{})
	repo, _ := d["repository"].(map[string]interface{})
	prs, _ := repo["pullRequests"].(map[string]interface{})
	tc, _ := prs["totalCount"].(float64)
	if tc < 1 {
		t.Fatalf("expected totalCount >= 1, got %v", tc)
	}
}

// --- Permissions in repo response ---

func TestConformanceRepoPermissions(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "conf-perms",
	}).Body.Close()

	resp := ghGet(t, "/api/v3/repos/admin/conf-perms", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	perms, _ := data["permissions"].(map[string]interface{})
	if perms == nil {
		t.Fatal("missing permissions in repo response")
	}
	if perms["admin"] != true {
		t.Fatalf("expected admin=true, got %v", perms["admin"])
	}
	if perms["push"] != true {
		t.Fatalf("expected push=true, got %v", perms["push"])
	}
	if perms["pull"] != true {
		t.Fatalf("expected pull=true, got %v", perms["pull"])
	}
}

// --- helpers ---

func newGHRequest(method, url, token string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	return req, nil
}

func doGHRequest(req *http.Request) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

