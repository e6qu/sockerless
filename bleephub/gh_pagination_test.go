package bleephub

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPaginationDefaults(t *testing.T) {
	repoName := "pg-defaults"
	createTestIssueRepo(t, repoName)

	// Create 3 issues
	for i := 0; i < 3; i++ {
		ghPost(t, "/api/v3/repos/admin/"+repoName+"/issues", defaultToken, map[string]interface{}{
			"title": "Issue",
		}).Body.Close()
	}

	resp := ghGet(t, "/api/v3/repos/admin/"+repoName+"/issues", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	issues := decodeJSONArray(t, resp)
	if len(issues) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(issues))
	}

	// No Link header for single page
	if link := resp.Header.Get("Link"); link != "" {
		t.Fatalf("expected no Link header for single page, got %s", link)
	}
}

func TestPaginationCustomPerPage(t *testing.T) {
	repoName := "pg-perpage"
	createTestIssueRepo(t, repoName)

	for i := 0; i < 5; i++ {
		ghPost(t, "/api/v3/repos/admin/"+repoName+"/issues", defaultToken, map[string]interface{}{
			"title": "Issue",
		}).Body.Close()
	}

	// Page 1 with per_page=2
	resp := ghGet(t, "/api/v3/repos/admin/"+repoName+"/issues?per_page=2", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	issues := decodeJSONArray(t, resp)
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}

	link := resp.Header.Get("Link")
	if link == "" {
		t.Fatal("expected Link header for multi-page result")
	}
	if !strings.Contains(link, `rel="next"`) {
		t.Fatalf("expected rel=next in Link, got %s", link)
	}
	if !strings.Contains(link, `rel="last"`) {
		t.Fatalf("expected rel=last in Link, got %s", link)
	}
}

func TestPaginationSecondPage(t *testing.T) {
	repoName := "pg-page2"
	createTestIssueRepo(t, repoName)

	for i := 0; i < 5; i++ {
		ghPost(t, "/api/v3/repos/admin/"+repoName+"/issues", defaultToken, map[string]interface{}{
			"title": "Issue",
		}).Body.Close()
	}

	// Page 2 with per_page=2
	resp := ghGet(t, "/api/v3/repos/admin/"+repoName+"/issues?per_page=2&page=2", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	issues := decodeJSONArray(t, resp)
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues on page 2, got %d", len(issues))
	}

	link := resp.Header.Get("Link")
	if !strings.Contains(link, `rel="first"`) {
		t.Fatalf("expected rel=first on page 2, got %s", link)
	}
	if !strings.Contains(link, `rel="prev"`) {
		t.Fatalf("expected rel=prev on page 2, got %s", link)
	}
}

func TestPaginationLastPage(t *testing.T) {
	repoName := "pg-last"
	createTestIssueRepo(t, repoName)

	for i := 0; i < 5; i++ {
		ghPost(t, "/api/v3/repos/admin/"+repoName+"/issues", defaultToken, map[string]interface{}{
			"title": "Issue",
		}).Body.Close()
	}

	// Last page (page 3, per_page=2): should have 1 item
	resp := ghGet(t, "/api/v3/repos/admin/"+repoName+"/issues?per_page=2&page=3", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	issues := decodeJSONArray(t, resp)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue on last page, got %d", len(issues))
	}

	link := resp.Header.Get("Link")
	// On last page, no next/last, but has first/prev
	if strings.Contains(link, `rel="next"`) {
		t.Fatalf("expected no rel=next on last page, got %s", link)
	}
	if !strings.Contains(link, `rel="first"`) {
		t.Fatalf("expected rel=first on last page, got %s", link)
	}
}

func TestPaginationPerPageMaxClamp(t *testing.T) {
	repoName := "pg-clamp"
	createTestIssueRepo(t, repoName)

	for i := 0; i < 3; i++ {
		ghPost(t, "/api/v3/repos/admin/"+repoName+"/issues", defaultToken, map[string]interface{}{
			"title": "Issue",
		}).Body.Close()
	}

	// per_page=200 should be clamped to 100
	resp := ghGet(t, "/api/v3/repos/admin/"+repoName+"/issues?per_page=200", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	issues := decodeJSONArray(t, resp)
	if len(issues) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(issues))
	}
}

func TestPaginationBeyondRange(t *testing.T) {
	repoName := "pg-beyond"
	createTestIssueRepo(t, repoName)

	ghPost(t, "/api/v3/repos/admin/"+repoName+"/issues", defaultToken, map[string]interface{}{
		"title": "Issue",
	}).Body.Close()

	// Page 99 should return empty
	resp := ghGet(t, "/api/v3/repos/admin/"+repoName+"/issues?page=99", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	var issues []interface{}
	json.NewDecoder(resp.Body).Decode(&issues)
	if len(issues) != 0 {
		t.Fatalf("expected 0 issues beyond range, got %d", len(issues))
	}
}

func TestPaginationRepoList(t *testing.T) {
	// Create several repos
	for i := 0; i < 3; i++ {
		ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
			"name": "pg-repo-" + http.StatusText(200+i),
		}).Body.Close()
	}

	resp := ghGet(t, "/api/v3/user/repos?per_page=1", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	var repos []interface{}
	json.NewDecoder(resp.Body).Decode(&repos)
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo per page, got %d", len(repos))
	}

	link := resp.Header.Get("Link")
	if !strings.Contains(link, `rel="next"`) {
		t.Fatalf("expected Link header with next, got %s", link)
	}
}
