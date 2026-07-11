package bleephub

import (
	"fmt"
	"strings"
	"testing"
)

// TestListOrgRepos verifies GET /api/v3/orgs/{org}/repos returns org-owned repos.
func TestListOrgRepos(t *testing.T) {
	createOrgViaAdminAPI(t, "list-org", "List Org")

	ghPost(t, "/api/v3/orgs/list-org/repos", defaultToken, map[string]interface{}{
		"name": "alpha",
	})
	ghPost(t, "/api/v3/orgs/list-org/repos", defaultToken, map[string]interface{}{
		"name":    "beta",
		"private": true,
	})

	resp := ghGet(t, "/api/v3/orgs/list-org/repos", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	repos := decodeJSONArray(t, resp)
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	seen := map[string]bool{}
	for _, r := range repos {
		seen[r["name"].(string)] = true
		if r["owner"].(map[string]interface{})["login"] != "list-org" {
			t.Fatalf("expected owner list-org, got %v", r["owner"])
		}
	}
	if !seen["alpha"] || !seen["beta"] {
		t.Fatalf("expected alpha and beta, got %v", seen)
	}
}

// TestListOrgReposNotFound verifies GET for nonexistent org → 404.
func TestListOrgReposNotFound(t *testing.T) {
	resp := ghGet(t, "/api/v3/orgs/no-such-org/repos", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// TestListAuthUserReposFilters verifies GET /api/v3/user/repos filtering.
func TestListAuthUserReposFilters(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":    "public-repo",
		"private": false,
	})
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":    "private-repo",
		"private": true,
	})

	resp := ghGet(t, "/api/v3/user/repos?visibility=public", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	repos := decodeJSONArray(t, resp)
	for _, r := range repos {
		if r["private"].(bool) {
			t.Fatalf("expected only public repos, got %v", r["full_name"])
		}
	}

	resp2 := ghGet(t, "/api/v3/user/repos?visibility=private", defaultToken)
	if resp2.StatusCode != 200 {
		resp2.Body.Close()
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	repos2 := decodeJSONArray(t, resp2)
	for _, r := range repos2 {
		if !r["private"].(bool) {
			t.Fatalf("expected only private repos, got %v", r["full_name"])
		}
	}
}

// TestListAuthUserReposTypeConflict verifies type + visibility/affiliation → 422.
func TestListAuthUserReposTypeConflict(t *testing.T) {
	resp := ghGet(t, "/api/v3/user/repos?type=owner&visibility=public", defaultToken)
	defer resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}

	resp2 := ghGet(t, "/api/v3/user/repos?type=owner&affiliation=owner", defaultToken)
	defer resp2.Body.Close()
	if resp2.StatusCode != 422 {
		t.Fatalf("expected 422, got %d", resp2.StatusCode)
	}
}

// TestListAuthUserReposSort verifies sort and direction query params.
func TestListAuthUserReposSort(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "sort-aaa",
	})
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "sort-zzz",
	})

	// The shared test server accumulates repos across tests; fetch all pages
	// until both target repos are found rather than assuming they fit in one
	// page.
	var idxAaa, idxZzz = -1, -1
	url := "/api/v3/user/repos?sort=full_name&direction=asc&per_page=100"
	seen := 0
	for url != "" {
		resp := ghGet(t, url, defaultToken)
		if resp.StatusCode != 200 {
			resp.Body.Close()
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		repos := decodeJSONArray(t, resp)
		for _, r := range repos {
			name := r["name"].(string)
			if name == "sort-aaa" {
				idxAaa = seen
			}
			if name == "sort-zzz" {
				idxZzz = seen
			}
			seen++
		}
		link := resp.Header.Get("Link")
		url = nextLinkURL(link)
	}

	if idxAaa == -1 || idxZzz == -1 {
		t.Fatalf("expected sort-aaa and sort-zzz in response, found at %d and %d", idxAaa, idxZzz)
	}
	if idxAaa > idxZzz {
		t.Fatalf("expected sort-aaa before sort-zzz, got %d and %d", idxAaa, idxZzz)
	}
}

// nextLinkURL extracts the rel="next" URL from a GitHub-style Link header.
func nextLinkURL(link string) string {
	if link == "" {
		return ""
	}
	for _, part := range strings.Split(link, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start != -1 && end != -1 && end > start {
			return strings.TrimPrefix(part[start+1:end], testBaseURL)
		}
	}
	return ""
}

// TestListUserReposByLogin verifies GET /api/v3/users/{username}/repos filters.
func TestListUserReposByLogin(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":    "user-public",
		"private": false,
	})
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":    "user-private",
		"private": true,
	})

	resp := ghGet(t, "/api/v3/users/admin/repos?type=public", "")
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	repos := decodeJSONArray(t, resp)
	for _, r := range repos {
		if r["private"].(bool) {
			t.Fatalf("expected only public repos for unauthenticated caller, got %v", r["full_name"])
		}
	}
}

// TestRepoResponseShape verifies new fields are emitted in repo responses.
func TestRepoResponseShape(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":               "shape-repo",
		"homepage":           "https://example.test",
		"has_issues":         true,
		"has_projects":       false,
		"has_wiki":           false,
		"has_pull_requests":  true,
		"allow_squash_merge": true,
		"allow_merge_commit": true,
		"allow_rebase_merge": false,
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	assertField := func(key string, want interface{}) {
		if data[key] != want {
			t.Fatalf("expected %s=%v, got %v", key, want, data[key])
		}
	}

	assertField("homepage", "https://example.test")
	assertField("has_issues", true)
	assertField("has_projects", false)
	assertField("has_wiki", false)
	assertField("has_pull_requests", true)
	assertField("allow_squash_merge", true)
	assertField("allow_merge_commit", true)
	assertField("allow_rebase_merge", false)

	if data["size"].(float64) < 0 {
		t.Fatalf("expected size >= 0, got %v", data["size"])
	}
}

// TestUpdateRepoExtended verifies PATCH supports new settings fields.
func TestUpdateRepoExtended(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "update-extended",
	})

	resp := ghPatch(t, "/api/v3/repos/admin/update-extended", defaultToken, map[string]interface{}{
		"description":                    "new description",
		"homepage":                       "https://new.test",
		"has_issues":                     false,
		"has_projects":                   true,
		"has_wiki":                       true,
		"has_pull_requests":              false,
		"allow_squash_merge":             false,
		"allow_merge_commit":             false,
		"allow_rebase_merge":             true,
		"allow_auto_merge":               true,
		"delete_branch_on_merge":         true,
		"use_squash_pr_title_as_default": true,
	})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	assertField := func(key string, want interface{}) {
		if data[key] != want {
			t.Fatalf("expected %s=%v, got %v", key, want, data[key])
		}
	}

	assertField("description", "new description")
	assertField("homepage", "https://new.test")
	assertField("has_issues", false)
	assertField("has_projects", true)
	assertField("has_wiki", true)
	assertField("has_pull_requests", false)
	assertField("allow_squash_merge", false)
	assertField("allow_merge_commit", false)
	assertField("allow_rebase_merge", true)
	assertField("allow_auto_merge", true)
	assertField("delete_branch_on_merge", true)
	assertField("use_squash_pr_title_as_default", true)
}

// TestUpdateRepoRename verifies PATCH with name renames the repo.
func TestUpdateRepoRename(t *testing.T) {
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "no-rename",
	})

	resp := ghPatch(t, "/api/v3/repos/admin/no-rename", defaultToken, map[string]interface{}{
		"name": "renamed",
	})
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	if data["name"] != "renamed" {
		t.Fatalf("expected name=renamed, got %v", data["name"])
	}

	// Old name is gone.
	old := ghGet(t, "/api/v3/repos/admin/no-rename", defaultToken)
	defer old.Body.Close()
	if old.StatusCode != 404 {
		t.Fatalf("expected 404 for old name, got %d", old.StatusCode)
	}
}

// TestCreateRepoWithLicenseTemplate verifies license object is emitted.
func TestCreateRepoWithLicenseTemplate(t *testing.T) {
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name":             "licensed",
		"auto_init":        true,
		"license_template": "mit",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	license, ok := data["license"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected license object, got %v", data["license"])
	}
	if license["key"] != "mit" {
		t.Fatalf("expected license.key=mit, got %v", license["key"])
	}
	if license["spdx_id"] != "MIT" {
		t.Fatalf("expected license.spdx_id=MIT, got %v", license["spdx_id"])
	}
}

// TestRepoOrganizationField verifies org-owned repos include organization object.
func TestRepoOrganizationField(t *testing.T) {
	createOrgViaAdminAPI(t, "field-org", "Field Org")
	ghPost(t, "/api/v3/orgs/field-org/repos", defaultToken, map[string]interface{}{
		"name": "field-repo",
	})

	resp := ghGet(t, "/api/v3/repos/field-org/field-repo", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)

	org, ok := data["organization"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected organization object, got %v", data["organization"])
	}
	if org["login"] != "field-org" {
		t.Fatalf("expected organization.login=field-org, got %v", org["login"])
	}
	if org["type"] != "Organization" {
		t.Fatalf("expected organization.type=Organization, got %v", org["type"])
	}
}

// TestRepoListPaginationLinkHeader verifies Link header for repo list endpoints.
func TestRepoListPaginationLinkHeader(t *testing.T) {
	for i := 0; i < 5; i++ {
		ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
			"name": fmt.Sprintf("page-%d", i),
		})
	}

	resp := ghGet(t, "/api/v3/user/repos?per_page=2&page=1", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	link := resp.Header.Get("Link")
	if link == "" {
		t.Fatal("expected Link header")
	}
	if !strings.Contains(link, `rel="next"`) {
		t.Fatalf("expected next link, got %s", link)
	}
	if !strings.Contains(link, `rel="last"`) {
		t.Fatalf("expected last link, got %s", link)
	}

	repos := decodeJSONArray(t, resp)
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
}
