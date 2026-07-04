package bleephub

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// createReadsRepo creates a repository owned by admin through the API.
func createReadsRepo(t *testing.T, name string, extra map[string]interface{}) {
	t.Helper()
	body := map[string]interface{}{"name": name}
	for k, v := range extra {
		body[k] = v
	}
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, body)
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create repo %s: %d", name, resp.StatusCode)
	}
}

// putReadsFile commits one file through the contents API and returns the
// commit SHA.
func putReadsFile(t *testing.T, repo, path, content, message, branch string) string {
	t.Helper()
	body := map[string]interface{}{
		"message": message,
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
	}
	if branch != "" {
		body["branch"] = branch
	}
	resp := ghDo(t, "PUT", "/api/v3/repos/admin/"+repo+"/contents/"+path, defaultToken, body)
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("put contents %s/%s: %d", repo, path, resp.StatusCode)
	}
	var out struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode put contents response: %v", err)
	}
	return out.Commit.SHA
}

func TestRepoSubscribersAndSubscription(t *testing.T) {
	createReadsRepo(t, "reads-watch", nil)

	// Not watching yet: GET subscription is 404.
	resp := ghGet(t, "/api/v3/repos/admin/reads-watch/subscription", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("subscription before watching: expected 404, got %d", resp.StatusCode)
	}

	resp = ghDo(t, "PUT", "/api/v3/repos/admin/reads-watch/subscription", defaultToken,
		map[string]interface{}{"subscribed": true})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("set subscription: %d", resp.StatusCode)
	}

	resp = ghGet(t, "/api/v3/repos/admin/reads-watch/subscription", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("get subscription: %d", resp.StatusCode)
	}
	sub := decodeJSON(t, resp)
	if sub["subscribed"] != true {
		t.Fatalf("expected subscribed true, got %v", sub["subscribed"])
	}

	resp = ghGet(t, "/api/v3/repos/admin/reads-watch/subscribers", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list subscribers: %d", resp.StatusCode)
	}
	subs := decodeJSONArray(t, resp)
	found := false
	for _, u := range subs {
		if u["login"] == "admin" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected admin among subscribers, got %v", subs)
	}

	resp = ghDelete(t, "/api/v3/repos/admin/reads-watch/subscription", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete subscription: %d", resp.StatusCode)
	}
	resp = ghGet(t, "/api/v3/repos/admin/reads-watch/subscription", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("subscription after unwatch: expected 404, got %d", resp.StatusCode)
	}
}

func TestRepoTeamsList(t *testing.T) {
	resp := ghPost(t, "/api/v3/admin/organizations", defaultToken, map[string]interface{}{
		"login": "reads-teams-org", "admin": "admin",
	})
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create org: %d", resp.StatusCode)
	}
	resp = ghPost(t, "/api/v3/orgs/reads-teams-org/repos", defaultToken, map[string]interface{}{"name": "teamed"})
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create org repo: %d", resp.StatusCode)
	}
	resp = ghPost(t, "/api/v3/orgs/reads-teams-org/teams", defaultToken, map[string]interface{}{"name": "readers"})
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create team: %d", resp.StatusCode)
	}
	resp = ghPut(t, "/api/v3/orgs/reads-teams-org/teams/readers/repos/reads-teams-org/teamed", defaultToken,
		map[string]interface{}{"permission": "push"})
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("add team repo: %d", resp.StatusCode)
	}

	resp = ghGet(t, "/api/v3/repos/reads-teams-org/teamed/teams", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list repo teams: %d", resp.StatusCode)
	}
	teams := decodeJSONArray(t, resp)
	if len(teams) != 1 || teams[0]["slug"] != "readers" {
		t.Fatalf("expected team readers, got %v", teams)
	}

	// A user-owned repo has no teams.
	createReadsRepo(t, "reads-teamless", nil)
	resp = ghGet(t, "/api/v3/repos/admin/reads-teamless/teams", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list teams on user repo: %d", resp.StatusCode)
	}
	if teams := decodeJSONArray(t, resp); len(teams) != 0 {
		t.Fatalf("expected no teams on user repo, got %v", teams)
	}
}

func TestRepoAssigneesAndCollaboratorCheck(t *testing.T) {
	createReadsRepo(t, "reads-assign", nil)
	createTestUser(t, "reads-collab")
	createTestUser(t, "reads-stranger")

	// Inviting a new user answers 201 with a pending repository invitation;
	// the invitee becomes a collaborator once the invitation is accepted.
	resp := ghPut(t, "/api/v3/repos/admin/reads-assign/collaborators/reads-collab", defaultToken,
		map[string]interface{}{"permission": "push"})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("add collaborator: %d", resp.StatusCode)
	}
	inv := decodeJSON(t, resp)
	invID, _ := inv["id"].(float64)
	if invID <= 0 {
		t.Fatalf("expected real invitation id, got %v", inv["id"])
	}
	if !testServer.store.AcceptRepoInvitation(int(invID), testServer.store.UsersByLogin["reads-collab"]) {
		t.Fatalf("accept invitation %d failed", int(invID))
	}

	resp = ghGet(t, "/api/v3/repos/admin/reads-assign/assignees", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list assignees: %d", resp.StatusCode)
	}
	logins := map[string]bool{}
	for _, u := range decodeJSONArray(t, resp) {
		login, _ := u["login"].(string)
		logins[login] = true
	}
	if !logins["admin"] || !logins["reads-collab"] {
		t.Fatalf("expected admin + reads-collab among assignees, got %v", logins)
	}
	if logins["reads-stranger"] {
		t.Fatalf("reads-stranger must not be assignable")
	}

	for path, want := range map[string]int{
		"/api/v3/repos/admin/reads-assign/assignees/reads-collab":       204,
		"/api/v3/repos/admin/reads-assign/assignees/admin":              204,
		"/api/v3/repos/admin/reads-assign/assignees/reads-stranger":     404,
		"/api/v3/repos/admin/reads-assign/collaborators/reads-collab":   204,
		"/api/v3/repos/admin/reads-assign/collaborators/admin":          204,
		"/api/v3/repos/admin/reads-assign/collaborators/reads-stranger": 404,
	} {
		resp := ghGet(t, path, defaultToken)
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("GET %s: expected %d, got %d", path, want, resp.StatusCode)
		}
	}
}

func TestRepoHashAlgorithm(t *testing.T) {
	createReadsRepo(t, "reads-hash", nil)
	resp := ghGet(t, "/api/v3/repos/admin/reads-hash/hash-algorithm", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("hash-algorithm: %d", resp.StatusCode)
	}
	out := decodeJSON(t, resp)
	if out["hash_algorithm"] != "sha1" {
		t.Fatalf("expected sha1, got %v", out["hash_algorithm"])
	}
	resp = ghGet(t, "/api/v3/repos/admin/no-such-repo-hash/hash-algorithm", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("hash-algorithm on missing repo: expected 404, got %d", resp.StatusCode)
	}
}

func TestRepoLicenseDetection(t *testing.T) {
	createReadsRepo(t, "reads-license", map[string]interface{}{
		"auto_init": true, "license_template": "mit",
	})
	resp := ghGet(t, "/api/v3/repos/admin/reads-license/license", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("get license: %d", resp.StatusCode)
	}
	out := decodeJSON(t, resp)
	if out["name"] != "LICENSE" || out["type"] != "file" {
		t.Fatalf("expected LICENSE content file, got name=%v type=%v", out["name"], out["type"])
	}
	lic, _ := out["license"].(map[string]interface{})
	if lic["key"] != "mit" || lic["spdx_id"] != "MIT" {
		t.Fatalf("expected detected MIT License, got %v", lic)
	}
	content, _ := out["content"].(string)
	decoded, err := base64.StdEncoding.DecodeString(content)
	if err != nil || !strings.Contains(string(decoded), "MIT License") {
		t.Fatalf("expected real base64 LICENSE content, got %q (err %v)", content, err)
	}

	// A license the catalog cannot identify is "other".
	createReadsRepo(t, "reads-license-other", nil)
	putReadsFile(t, "reads-license-other", "LICENSE", "You may use this software however you like.\n", "add license", "")
	resp = ghGet(t, "/api/v3/repos/admin/reads-license-other/license", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("get unidentified license: %d", resp.StatusCode)
	}
	out = decodeJSON(t, resp)
	lic, _ = out["license"].(map[string]interface{})
	if lic["key"] != "other" || lic["spdx_id"] != "NOASSERTION" {
		t.Fatalf("expected license key other, got %v", lic)
	}

	// No license file at all: 404.
	createReadsRepo(t, "reads-license-none", map[string]interface{}{"auto_init": true})
	resp = ghGet(t, "/api/v3/repos/admin/reads-license-none/license", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("license on repo without one: expected 404, got %d", resp.StatusCode)
	}
}

func TestRepoReadmeInDirectory(t *testing.T) {
	createReadsRepo(t, "reads-dirreadme", map[string]interface{}{"auto_init": true})
	putReadsFile(t, "reads-dirreadme", "docs/README.md", "# Docs readme\n", "add docs readme", "")

	resp := ghGet(t, "/api/v3/repos/admin/reads-dirreadme/readme/docs", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("readme/docs: %d", resp.StatusCode)
	}
	out := decodeJSON(t, resp)
	if out["path"] != "docs/README.md" || out["name"] != "README.md" {
		t.Fatalf("expected docs/README.md, got %v", out["path"])
	}
	content, _ := out["content"].(string)
	if decoded, err := base64.StdEncoding.DecodeString(content); err != nil || string(decoded) != "# Docs readme\n" {
		t.Fatalf("wrong readme content: %q (err %v)", content, err)
	}

	resp = ghGet(t, "/api/v3/repos/admin/reads-dirreadme/readme/no-such-dir", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("readme in missing dir: expected 404, got %d", resp.StatusCode)
	}
}

func TestRepoCommunityProfile(t *testing.T) {
	createReadsRepo(t, "reads-community", map[string]interface{}{
		"auto_init": true, "license_template": "mit", "description": "a community repo",
	})
	resp := ghGet(t, "/api/v3/repos/admin/reads-community/community/profile", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("community profile: %d", resp.StatusCode)
	}
	out := decodeJSON(t, resp)
	// Present: description, readme, license. Absent: contributing, code of
	// conduct, issue template, PR template. 3 of 7 checks → 42%.
	if got, _ := out["health_percentage"].(float64); int(got) != 42 {
		t.Fatalf("expected health 42, got %v", out["health_percentage"])
	}
	files, _ := out["files"].(map[string]interface{})
	if files["readme"] == nil {
		t.Fatalf("expected readme file entry, got nil")
	}
	lic, _ := files["license"].(map[string]interface{})
	if lic["key"] != "mit" {
		t.Fatalf("expected detected mit license, got %v", files["license"])
	}
	if files["contributing"] != nil || files["code_of_conduct"] != nil {
		t.Fatalf("expected absent community files to be null, got %v", files)
	}

	// Adding CONTRIBUTING.md raises the health percentage.
	putReadsFile(t, "reads-community", "CONTRIBUTING.md", "# How to contribute\n", "add contributing", "")
	resp = ghGet(t, "/api/v3/repos/admin/reads-community/community/profile", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("community profile after contributing: %d", resp.StatusCode)
	}
	out = decodeJSON(t, resp)
	if got, _ := out["health_percentage"].(float64); int(got) != 57 {
		t.Fatalf("expected health 57 after CONTRIBUTING.md, got %v", out["health_percentage"])
	}
}

func TestRepoCodeownersErrors(t *testing.T) {
	createReadsRepo(t, "reads-codeowners", map[string]interface{}{"auto_init": true})

	// No CODEOWNERS file: empty error list.
	resp := ghGet(t, "/api/v3/repos/admin/reads-codeowners/codeowners/errors", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("codeowners errors (absent): %d", resp.StatusCode)
	}
	out := decodeJSON(t, resp)
	if errs, _ := out["errors"].([]interface{}); len(errs) != 0 {
		t.Fatalf("expected no errors without CODEOWNERS, got %v", errs)
	}

	content := "# owners\n* @admin\ndocs/ @no-such-owner plainword\n"
	putReadsFile(t, "reads-codeowners", ".github/CODEOWNERS", content, "add CODEOWNERS", "")

	resp = ghGet(t, "/api/v3/repos/admin/reads-codeowners/codeowners/errors", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("codeowners errors: %d", resp.StatusCode)
	}
	out = decodeJSON(t, resp)
	errs, _ := out["errors"].([]interface{})
	if len(errs) != 2 {
		t.Fatalf("expected 2 CODEOWNERS errors, got %v", errs)
	}
	kinds := map[string]bool{}
	for _, e := range errs {
		entry, _ := e.(map[string]interface{})
		kinds[entry["kind"].(string)] = true
		if entry["path"] != ".github/CODEOWNERS" {
			t.Fatalf("wrong error path: %v", entry["path"])
		}
		if line, _ := entry["line"].(float64); int(line) != 3 {
			t.Fatalf("expected errors on line 3, got %v", entry["line"])
		}
	}
	if !kinds["Unknown owner"] || !kinds["Invalid owner"] {
		t.Fatalf("expected Unknown owner + Invalid owner kinds, got %v", kinds)
	}
}

func TestListPublicRepositories(t *testing.T) {
	createReadsRepo(t, "reads-public-list", nil)
	repo := testServer.store.GetRepo("admin", "reads-public-list")
	if repo == nil {
		t.Fatal("repo not created")
	}

	// Walk the since-paginated listing the way a real client does until the
	// repository appears.
	found := false
	since := 0
	for !found {
		resp := ghGet(t, fmt.Sprintf("/api/v3/repositories?since=%d", since), defaultToken)
		if resp.StatusCode != 200 {
			resp.Body.Close()
			t.Fatalf("list public repositories: %d", resp.StatusCode)
		}
		page := decodeJSONArray(t, resp)
		if len(page) == 0 {
			break
		}
		for _, r := range page {
			if r["full_name"] == "admin/reads-public-list" {
				found = true
			}
			since = int(r["id"].(float64))
		}
	}
	if !found {
		t.Fatalf("expected admin/reads-public-list in /repositories")
	}

	// since= pagination excludes repositories up to that ID.
	resp := ghGet(t, fmt.Sprintf("/api/v3/repositories?since=%d", repo.ID), defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list public repositories since: %d", resp.StatusCode)
	}
	for _, r := range decodeJSONArray(t, resp) {
		if r["full_name"] == "admin/reads-public-list" {
			t.Fatalf("since= must exclude the repo itself")
		}
	}
}

func TestGitSingleRefAndMatchingRefs(t *testing.T) {
	createReadsRepo(t, "reads-refs", map[string]interface{}{"auto_init": true})

	resp := ghGet(t, "/api/v3/repos/admin/reads-refs/git/ref/heads/main", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("git/ref/heads/main: %d", resp.StatusCode)
	}
	out := decodeJSON(t, resp)
	if out["ref"] != "refs/heads/main" {
		t.Fatalf("expected refs/heads/main, got %v", out["ref"])
	}
	obj, _ := out["object"].(map[string]interface{})
	if sha, _ := obj["sha"].(string); len(sha) != 40 {
		t.Fatalf("expected 40-char sha, got %v", obj["sha"])
	}

	resp = ghGet(t, "/api/v3/repos/admin/reads-refs/git/ref/heads/definitely-missing", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("missing single ref: expected 404, got %d", resp.StatusCode)
	}

	resp = ghGet(t, "/api/v3/repos/admin/reads-refs/git/matching-refs/heads", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("matching-refs/heads: %d", resp.StatusCode)
	}
	refs := decodeJSONArray(t, resp)
	if len(refs) != 1 || refs[0]["ref"] != "refs/heads/main" {
		t.Fatalf("expected exactly refs/heads/main, got %v", refs)
	}

	// A prefix matching nothing is an empty 200 list, not a 404.
	resp = ghGet(t, "/api/v3/repos/admin/reads-refs/git/matching-refs/tags", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("matching-refs/tags: %d", resp.StatusCode)
	}
	if refs := decodeJSONArray(t, resp); len(refs) != 0 {
		t.Fatalf("expected no tag refs, got %v", refs)
	}
}
