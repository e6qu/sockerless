package bleephub

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
)

// createReadsBranch creates a branch pointing at the current main head
// through the git refs API and returns the main head SHA.
func createReadsBranch(t *testing.T, repo, branch string) string {
	t.Helper()
	resp := ghGet(t, "/api/v3/repos/admin/"+repo+"/git/ref/heads/main", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("resolve main head: %d", resp.StatusCode)
	}
	ref := decodeJSON(t, resp)
	obj, _ := ref["object"].(map[string]interface{})
	sha, _ := obj["sha"].(string)
	resp = ghPost(t, "/api/v3/repos/admin/"+repo+"/git/refs", defaultToken, map[string]interface{}{
		"ref": "refs/heads/" + branch, "sha": sha,
	})
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create branch %s: %d", branch, resp.StatusCode)
	}
	return sha
}

func TestGetSingleCommit(t *testing.T) {
	createReadsRepo(t, "reads-commit", map[string]interface{}{"auto_init": true})
	sha := putReadsFile(t, "reads-commit", "hello.txt", "hello world\nsecond line\n", "add hello", "")

	resp := ghGet(t, "/api/v3/repos/admin/reads-commit/commits/main", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("get commit by branch: %d", resp.StatusCode)
	}
	out := decodeJSON(t, resp)
	if out["sha"] != sha {
		t.Fatalf("expected head sha %s, got %v", sha, out["sha"])
	}
	commit, _ := out["commit"].(map[string]interface{})
	if commit["message"] != "add hello" {
		t.Fatalf("expected message 'add hello', got %v", commit["message"])
	}
	parents, _ := out["parents"].([]interface{})
	if len(parents) != 1 {
		t.Fatalf("expected 1 parent, got %v", parents)
	}
	stats, _ := out["stats"].(map[string]interface{})
	if adds, _ := stats["additions"].(float64); int(adds) != 2 {
		t.Fatalf("expected 2 additions, got %v", stats)
	}
	if total, _ := stats["total"].(float64); int(total) != 2 {
		t.Fatalf("expected stats.total 2, got %v", stats)
	}
	files, _ := out["files"].([]interface{})
	if len(files) != 1 {
		t.Fatalf("expected 1 changed file, got %v", files)
	}
	file, _ := files[0].(map[string]interface{})
	if file["filename"] != "hello.txt" || file["status"] != "added" {
		t.Fatalf("expected added hello.txt, got %v", file)
	}
	patch, _ := file["patch"].(string)
	if !strings.HasPrefix(patch, "@@") || !strings.Contains(patch, "+hello world") {
		t.Fatalf("expected real unified diff hunks, got %q", patch)
	}
	// The author resolves to the real admin account.
	author, _ := out["author"].(map[string]interface{})
	if author["login"] != "admin" {
		t.Fatalf("expected commit author resolved to admin, got %v", out["author"])
	}

	// Lookup by full SHA works identically.
	resp = ghGet(t, "/api/v3/repos/admin/reads-commit/commits/"+sha, defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("get commit by sha: %d", resp.StatusCode)
	}
	out = decodeJSON(t, resp)
	if out["sha"] != sha {
		t.Fatalf("sha lookup mismatch: %v", out["sha"])
	}

	resp = ghGet(t, "/api/v3/repos/admin/reads-commit/commits/no-such-ref", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("unknown ref: expected 404, got %d", resp.StatusCode)
	}
}

func TestListCommitsEmptyRepositoryFailsLoud(t *testing.T) {
	createReadsRepo(t, "reads-empty-commits", nil)

	resp := ghGet(t, "/api/v3/repos/admin/reads-empty-commits/commits", defaultToken)
	if resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("list commits on empty repository: %d body=%s", resp.StatusCode, body)
	}
	out := decodeJSON(t, resp)
	if out["message"] != "Git Repository is empty." {
		t.Fatalf("empty repository message = %v", out["message"])
	}
}

func TestUIListCommitsEmptyRepositoryReturnsEmptyHistory(t *testing.T) {
	createReadsRepo(t, "reads-ui-empty-commits", nil)

	resp := ghGet(t, "/ui-data/repos/admin/reads-ui-empty-commits/commits", defaultToken)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("UI list commits on empty repository: %d body=%s", resp.StatusCode, body)
	}
	out := decodeJSONArray(t, resp)
	if len(out) != 0 {
		t.Fatalf("UI empty repository commits = %v, want []", out)
	}
}

func TestCommitBranchesWhereHead(t *testing.T) {
	createReadsRepo(t, "reads-headbranch", map[string]interface{}{"auto_init": true})
	sha := putReadsFile(t, "reads-headbranch", "a.txt", "a\n", "add a", "")

	resp := ghGet(t, "/api/v3/repos/admin/reads-headbranch/commits/"+sha+"/branches-where-head", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("branches-where-head: %d", resp.StatusCode)
	}
	branches := decodeJSONArray(t, resp)
	if len(branches) != 1 || branches[0]["name"] != "main" {
		t.Fatalf("expected [main], got %v", branches)
	}
	commit, _ := branches[0]["commit"].(map[string]interface{})
	if commit["sha"] != sha {
		t.Fatalf("expected head sha %s, got %v", sha, commit["sha"])
	}
}

func TestCommitPulls(t *testing.T) {
	createReadsRepo(t, "reads-commitpulls", map[string]interface{}{"auto_init": true})
	createReadsBranch(t, "reads-commitpulls", "feature-cp")
	featureSHA := putReadsFile(t, "reads-commitpulls", "feature.txt", "feature\n", "feature commit", "feature-cp")

	resp := ghPost(t, "/api/v3/repos/admin/reads-commitpulls/pulls", defaultToken, map[string]interface{}{
		"title": "feature PR", "head": "feature-cp", "base": "main",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("create PR: %d", resp.StatusCode)
	}
	pr := decodeJSON(t, resp)
	prNumber := int(pr["number"].(float64))

	resp = ghGet(t, "/api/v3/repos/admin/reads-commitpulls/commits/"+featureSHA+"/pulls", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("commit pulls: %d", resp.StatusCode)
	}
	pulls := decodeJSONArray(t, resp)
	if len(pulls) != 1 || int(pulls[0]["number"].(float64)) != prNumber {
		t.Fatalf("expected PR #%d containing the commit, got %v", prNumber, pulls)
	}

	// The base branch head is not part of the PR.
	resp = ghGet(t, "/api/v3/repos/admin/reads-commitpulls/commits/main/pulls", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("commit pulls for base head: %d", resp.StatusCode)
	}
	if pulls := decodeJSONArray(t, resp); len(pulls) != 0 {
		t.Fatalf("base head commit must not be in any PR, got %v", pulls)
	}
}

func TestRepoContributors(t *testing.T) {
	createReadsRepo(t, "reads-contrib", map[string]interface{}{"auto_init": true})
	putReadsFile(t, "reads-contrib", "one.txt", "1\n", "commit one", "")
	putReadsFile(t, "reads-contrib", "two.txt", "2\n", "commit two", "")

	resp := ghGet(t, "/api/v3/repos/admin/reads-contrib/contributors", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("contributors: %d", resp.StatusCode)
	}
	contribs := decodeJSONArray(t, resp)
	if len(contribs) != 1 {
		t.Fatalf("expected exactly the admin contributor, got %v", contribs)
	}
	if contribs[0]["login"] != "admin" || contribs[0]["type"] != "User" {
		t.Fatalf("expected admin User contributor, got %v", contribs[0])
	}
	// auto_init + two contents commits, all authored by admin identities.
	if n, _ := contribs[0]["contributions"].(float64); int(n) != 3 {
		t.Fatalf("expected 3 contributions, got %v", contribs[0]["contributions"])
	}

	// A repository without commits is a 204.
	createReadsRepo(t, "reads-contrib-empty", nil)
	resp = ghGet(t, "/api/v3/repos/admin/reads-contrib-empty/contributors", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("contributors on empty repo: expected 204, got %d", resp.StatusCode)
	}
}

func TestRepoStatistics(t *testing.T) {
	createReadsRepo(t, "reads-stats", map[string]interface{}{"auto_init": true})
	putReadsFile(t, "reads-stats", "s1.txt", "line1\nline2\n", "stats one", "")
	putReadsFile(t, "reads-stats", "s2.txt", "line1\n", "stats two", "")
	const commitCount = 3 // auto_init + two contents commits

	// stats/contributors: one bucket for admin, weekly a/d/c cells.
	resp := ghGet(t, "/api/v3/repos/admin/reads-stats/stats/contributors", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("stats/contributors: %d", resp.StatusCode)
	}
	sc := decodeJSONArray(t, resp)
	if len(sc) != 1 {
		t.Fatalf("expected one contributor activity entry, got %v", sc)
	}
	author, _ := sc[0]["author"].(map[string]interface{})
	if author["login"] != "admin" {
		t.Fatalf("expected admin author, got %v", author)
	}
	if total, _ := sc[0]["total"].(float64); int(total) != commitCount {
		t.Fatalf("expected total %d, got %v", commitCount, sc[0]["total"])
	}
	weeks, _ := sc[0]["weeks"].([]interface{})
	weekCommits, weekAdds := 0, 0
	for _, wk := range weeks {
		cell, _ := wk.(map[string]interface{})
		c, _ := cell["c"].(float64)
		a, _ := cell["a"].(float64)
		weekCommits += int(c)
		weekAdds += int(a)
	}
	if weekCommits != commitCount || weekAdds < 3 {
		t.Fatalf("expected weekly cells summing to %d commits and >=3 additions, got c=%d a=%d", commitCount, weekCommits, weekAdds)
	}

	// stats/code_frequency: [week, additions, -deletions] rows.
	resp = ghGet(t, "/api/v3/repos/admin/reads-stats/stats/code_frequency", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("stats/code_frequency: %d", resp.StatusCode)
	}
	var freq [][]int64
	if err := json.NewDecoder(resp.Body).Decode(&freq); err != nil {
		resp.Body.Close()
		t.Fatalf("decode code_frequency: %v", err)
	}
	resp.Body.Close()
	if len(freq) == 0 || len(freq[0]) != 3 {
		t.Fatalf("expected [w,a,d] rows, got %v", freq)
	}
	totalAdds := int64(0)
	for _, row := range freq {
		totalAdds += row[1]
		if row[2] > 0 {
			t.Fatalf("deletions must be non-positive, got %v", row)
		}
	}
	if totalAdds < 3 {
		t.Fatalf("expected >=3 total additions, got %d", totalAdds)
	}

	// stats/commit_activity: 52 weekly buckets summing to the commit count.
	resp = ghGet(t, "/api/v3/repos/admin/reads-stats/stats/commit_activity", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("stats/commit_activity: %d", resp.StatusCode)
	}
	activity := decodeJSONArray(t, resp)
	if len(activity) != 52 {
		t.Fatalf("expected 52 weeks, got %d", len(activity))
	}
	sum := 0
	for _, wk := range activity {
		total, _ := wk["total"].(float64)
		sum += int(total)
		days, _ := wk["days"].([]interface{})
		if len(days) != 7 {
			t.Fatalf("expected 7 day cells, got %v", wk["days"])
		}
	}
	if sum != commitCount {
		t.Fatalf("expected weekly totals summing to %d, got %d", commitCount, sum)
	}

	// stats/participation: all/owner 52-week vectors; every commit is the
	// owner's here.
	resp = ghGet(t, "/api/v3/repos/admin/reads-stats/stats/participation", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("stats/participation: %d", resp.StatusCode)
	}
	part := decodeJSON(t, resp)
	all, _ := part["all"].([]interface{})
	owner, _ := part["owner"].([]interface{})
	if len(all) != 52 || len(owner) != 52 {
		t.Fatalf("expected 52-week vectors, got %d/%d", len(all), len(owner))
	}
	allSum, ownerSum := 0, 0
	for i := range all {
		a, _ := all[i].(float64)
		o, _ := owner[i].(float64)
		allSum += int(a)
		ownerSum += int(o)
	}
	if allSum != commitCount || ownerSum != commitCount {
		t.Fatalf("expected all=owner=%d, got all=%d owner=%d", commitCount, allSum, ownerSum)
	}

	// stats/punch_card: 168 [day,hour,count] cells summing to the commits.
	resp = ghGet(t, "/api/v3/repos/admin/reads-stats/stats/punch_card", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("stats/punch_card: %d", resp.StatusCode)
	}
	var punch [][]int
	if err := json.NewDecoder(resp.Body).Decode(&punch); err != nil {
		resp.Body.Close()
		t.Fatalf("decode punch_card: %v", err)
	}
	resp.Body.Close()
	if len(punch) != 7*24 {
		t.Fatalf("expected 168 punch card cells, got %d", len(punch))
	}
	punchSum := 0
	for _, cell := range punch {
		punchSum += cell[2]
	}
	if punchSum != commitCount {
		t.Fatalf("expected punch card total %d, got %d", commitCount, punchSum)
	}
}

// noRedirectGet issues a GET that does not follow redirects.
func noRedirectGet(t *testing.T, path string) *http.Response {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	req, err := http.NewRequest("GET", testBaseURL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "token "+defaultToken)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestRepoTarballAndZipball(t *testing.T) {
	createReadsRepo(t, "reads-archive", map[string]interface{}{"auto_init": true})
	putReadsFile(t, "reads-archive", "src/app.txt", "archive me\n", "add src", "")

	headResp := ghGet(t, "/api/v3/repos/admin/reads-archive/commits/main", defaultToken)
	head := decodeJSON(t, headResp)
	shortSHA := head["sha"].(string)[:7]
	topDir := "admin-reads-archive-" + shortSHA + "/"

	// tarball: 302 to the codeload-style legacy URL, which streams a real
	// tar.gz with GitHub's top-level directory convention.
	resp := noRedirectGet(t, "/api/v3/repos/admin/reads-archive/tarball/main")
	resp.Body.Close()
	if resp.StatusCode != 302 {
		t.Fatalf("tarball: expected 302, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "/admin/reads-archive/legacy.tar.gz/main") {
		t.Fatalf("unexpected tarball Location %q", loc)
	}
	resp = ghGet(t, strings.TrimPrefix(loc, testBaseURL), defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("legacy tarball download: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read tarball: %v", err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("tarball is not gzip: %v", err)
	}
	tarFiles := map[string]string{}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar: %v", err)
		}
		content, _ := io.ReadAll(tr)
		tarFiles[hdr.Name] = string(content)
	}
	if tarFiles[topDir+"src/app.txt"] != "archive me\n" {
		t.Fatalf("expected %ssrc/app.txt in tarball, got entries %v", topDir, mapKeys(tarFiles))
	}
	if _, ok := tarFiles[topDir+"README.md"]; !ok {
		t.Fatalf("expected README.md in tarball, got %v", mapKeys(tarFiles))
	}

	// zipball, same contract.
	resp = noRedirectGet(t, "/api/v3/repos/admin/reads-archive/zipball/main")
	resp.Body.Close()
	if resp.StatusCode != 302 {
		t.Fatalf("zipball: expected 302, got %d", resp.StatusCode)
	}
	loc = resp.Header.Get("Location")
	if !strings.Contains(loc, "/admin/reads-archive/legacy.zip/main") {
		t.Fatalf("unexpected zipball Location %q", loc)
	}
	resp = ghGet(t, strings.TrimPrefix(loc, testBaseURL), defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("legacy zipball download: %d", resp.StatusCode)
	}
	body, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read zipball: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("zipball is not a zip: %v", err)
	}
	zipFound := false
	for _, f := range zr.File {
		if f.Name == topDir+"src/app.txt" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open zip entry: %v", err)
			}
			content, _ := io.ReadAll(rc)
			rc.Close()
			if string(content) != "archive me\n" {
				t.Fatalf("wrong zip entry content %q", content)
			}
			zipFound = true
		}
	}
	if !zipFound {
		t.Fatalf("expected %ssrc/app.txt in zipball", topDir)
	}

	// Unknown ref: 404 at the API endpoint, no redirect.
	resp = noRedirectGet(t, "/api/v3/repos/admin/reads-archive/tarball/no-such-ref")
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("tarball of unknown ref: expected 404, got %d", resp.StatusCode)
	}
}

func mapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// pushReadsCommit pushes one root commit to the repo over git smart HTTP with
// the real go-git client and returns the commit hash.
func pushReadsCommit(t *testing.T, repoName string) plumbing.Hash {
	t.Helper()
	storage := memory.NewStorage()
	repo, err := git.Init(storage, nil)
	if err != nil {
		t.Fatalf("git init: %v", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{testBaseURL + "/admin/" + repoName + ".git"},
	}); err != nil {
		t.Fatalf("create remote: %v", err)
	}
	blobHash, err := storeBlob(storage, []byte("pushed content\n"))
	if err != nil {
		t.Fatalf("store blob: %v", err)
	}
	treeHash, err := storeTree(storage, []object.TreeEntry{
		{Name: "pushed.txt", Mode: 0o100644, Hash: blobHash},
	})
	if err != nil {
		t.Fatalf("store tree: %v", err)
	}
	commitHash, err := storeCommit(storage, treeHash, plumbing.ZeroHash, "pushed commit")
	if err != nil {
		t.Fatalf("store commit: %v", err)
	}
	if err := storage.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), commitHash)); err != nil {
		t.Fatalf("set ref: %v", err)
	}
	if err := repo.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{"refs/heads/main:refs/heads/main"},
		Auth:       &githttp.BasicAuth{Username: "x", Password: defaultToken},
	}); err != nil {
		t.Fatalf("git push: %v", err)
	}
	return commitHash
}

func TestRepoActivityAndEvents(t *testing.T) {
	createReadsRepo(t, "reads-activity", nil)
	commitHash := pushReadsCommit(t, "reads-activity")

	resp := ghGet(t, "/api/v3/repos/admin/reads-activity/activity", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("activity: %d", resp.StatusCode)
	}
	acts := decodeJSONArray(t, resp)
	if len(acts) != 1 {
		t.Fatalf("expected 1 recorded activity, got %v", acts)
	}
	act := acts[0]
	if act["activity_type"] != "branch_creation" || act["ref"] != "refs/heads/main" {
		t.Fatalf("expected branch_creation of refs/heads/main, got %v", act)
	}
	if act["after"] != commitHash.String() {
		t.Fatalf("expected after=%s, got %v", commitHash, act["after"])
	}
	actor, _ := act["actor"].(map[string]interface{})
	if actor["login"] != "admin" {
		t.Fatalf("expected pusher admin, got %v", act["actor"])
	}

	// Filters narrow the real records.
	for query, want := range map[string]int{
		"?activity_type=branch_creation": 1,
		"?activity_type=force_push":      0,
		"?actor=admin":                   1,
		"?actor=nobody":                  0,
		"?ref=main":                      1,
	} {
		resp := ghGet(t, "/api/v3/repos/admin/reads-activity/activity"+query, defaultToken)
		if resp.StatusCode != 200 {
			resp.Body.Close()
			t.Fatalf("activity%s: %d", query, resp.StatusCode)
		}
		if got := len(decodeJSONArray(t, resp)); got != want {
			t.Fatalf("activity%s: expected %d rows, got %d", query, want, got)
		}
	}
	resp = ghGet(t, "/api/v3/repos/admin/reads-activity/activity?activity_type=bogus", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("invalid activity_type: expected 422, got %d", resp.StatusCode)
	}

	// The events feed derives from the same records plus issue state.
	resp = ghPost(t, "/api/v3/repos/admin/reads-activity/issues", defaultToken,
		map[string]interface{}{"title": "activity issue"})
	resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create issue: %d", resp.StatusCode)
	}

	resp = ghGet(t, "/api/v3/repos/admin/reads-activity/events", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("events: %d", resp.StatusCode)
	}
	types := map[string]bool{}
	for _, ev := range decodeJSONArray(t, resp) {
		typ, _ := ev["type"].(string)
		types[typ] = true
	}
	if !types["CreateEvent"] || !types["IssuesEvent"] {
		t.Fatalf("expected CreateEvent + IssuesEvent in repo events, got %v", types)
	}

	// The network feed covers the repository itself (it is its own root).
	resp = ghGet(t, "/api/v3/networks/admin/reads-activity/events", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("network events: %d", resp.StatusCode)
	}
	if evs := decodeJSONArray(t, resp); len(evs) == 0 {
		t.Fatalf("expected network events, got none")
	}
}

func TestRepoTraffic(t *testing.T) {
	createReadsRepo(t, "reads-traffic", nil)
	pushReadsCommit(t, "reads-traffic")

	// A real clone through git smart HTTP is counted.
	if _, err := git.Clone(memory.NewStorage(), nil, &git.CloneOptions{
		URL:  testBaseURL + "/admin/reads-traffic.git",
		Auth: &githttp.BasicAuth{Username: "x", Password: defaultToken},
	}); err != nil {
		t.Fatalf("git clone: %v", err)
	}

	resp := ghGet(t, "/api/v3/repos/admin/reads-traffic/traffic/clones", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("traffic/clones: %d", resp.StatusCode)
	}
	out := decodeJSON(t, resp)
	if count, _ := out["count"].(float64); int(count) != 1 {
		t.Fatalf("expected 1 clone, got %v", out["count"])
	}
	if uniques, _ := out["uniques"].(float64); int(uniques) != 1 {
		t.Fatalf("expected 1 unique cloner, got %v", out["uniques"])
	}
	clones, _ := out["clones"].([]interface{})
	if len(clones) != 1 {
		t.Fatalf("expected one clone bucket, got %v", clones)
	}

	// Views: bleephub serves no repository HTML pages, so the counters are
	// real zeros.
	resp = ghGet(t, "/api/v3/repos/admin/reads-traffic/traffic/views", defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("traffic/views: %d", resp.StatusCode)
	}
	views := decodeJSON(t, resp)
	if count, _ := views["count"].(float64); int(count) != 0 {
		t.Fatalf("expected 0 views, got %v", views["count"])
	}

	for _, path := range []string{"paths", "referrers"} {
		resp := ghGet(t, "/api/v3/repos/admin/reads-traffic/traffic/popular/"+path, defaultToken)
		if resp.StatusCode != 200 {
			resp.Body.Close()
			t.Fatalf("traffic/popular/%s: %d", path, resp.StatusCode)
		}
		if rows := decodeJSONArray(t, resp); len(rows) != 0 {
			t.Fatalf("expected empty popular %s, got %v", path, rows)
		}
	}

	// Traffic requires push access: anonymous callers get 403.
	resp = ghGet(t, "/api/v3/repos/admin/reads-traffic/traffic/clones", "")
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("anonymous traffic read: expected 403, got %d", resp.StatusCode)
	}
}

func TestCheckRunAndSuiteRerequest(t *testing.T) {
	createReadsRepo(t, "reads-checks", map[string]interface{}{"auto_init": true})
	headResp := ghGet(t, "/api/v3/repos/admin/reads-checks/commits/main", defaultToken)
	head := decodeJSON(t, headResp)
	sha, _ := head["sha"].(string)

	resp := ghPost(t, "/api/v3/repos/admin/reads-checks/check-runs", defaultToken, map[string]interface{}{
		"name": "build", "head_sha": sha, "status": "completed", "conclusion": "failure",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("create check run: %d", resp.StatusCode)
	}
	cr := decodeJSON(t, resp)
	runID := int64(cr["id"].(float64))
	suite, _ := cr["check_suite"].(map[string]interface{})
	suiteID := int64(suite["id"].(float64))

	resp = ghPost(t, fmt.Sprintf("/api/v3/repos/admin/reads-checks/check-runs/%d/rerequest", runID), defaultToken, nil)
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("rerequest check run: %d", resp.StatusCode)
	}
	if body := decodeJSON(t, resp); len(body) != 0 {
		t.Fatalf("rerequest must return an empty object, got %v", body)
	}
	resp = ghGet(t, fmt.Sprintf("/api/v3/repos/admin/reads-checks/check-runs/%d", runID), defaultToken)
	rerun := decodeJSON(t, resp)
	if rerun["status"] != "queued" {
		t.Fatalf("expected rerequested run queued, got %v", rerun["status"])
	}

	// A run that is not completed cannot be rerequested.
	resp = ghPost(t, fmt.Sprintf("/api/v3/repos/admin/reads-checks/check-runs/%d/rerequest", runID), defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != 422 {
		t.Fatalf("rerequest of queued run: expected 422, got %d", resp.StatusCode)
	}

	resp = ghPost(t, fmt.Sprintf("/api/v3/repos/admin/reads-checks/check-suites/%d/rerequest", suiteID), defaultToken, nil)
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("rerequest check suite: %d", resp.StatusCode)
	}
	if body := decodeJSON(t, resp); len(body) != 0 {
		t.Fatalf("suite rerequest must return an empty object, got %v", body)
	}
	resp = ghGet(t, fmt.Sprintf("/api/v3/repos/admin/reads-checks/check-suites/%d", suiteID), defaultToken)
	suiteOut := decodeJSON(t, resp)
	if suiteOut["status"] != "queued" {
		t.Fatalf("expected rerequested suite queued, got %v", suiteOut["status"])
	}

	resp = ghPost(t, "/api/v3/repos/admin/reads-checks/check-runs/999999/rerequest", defaultToken, nil)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("rerequest of unknown run: expected 404, got %d", resp.StatusCode)
	}
}

func TestRepoRuleSuitesAndRulesetHistoryDispatch(t *testing.T) {
	createReadsRepo(t, "reads-rulesuites", nil)

	// bleephub records no ruleset evaluations, so any rule-suite lookup is a
	// real 404.
	resp := ghGet(t, "/api/v3/repos/admin/reads-rulesuites/rulesets/rule-suites/123", defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("rule suite lookup: expected 404, got %d", resp.StatusCode)
	}

	// The ruleset history listing shares the dispatch and still works.
	resp = ghPost(t, "/api/v3/repos/admin/reads-rulesuites/rulesets", defaultToken, map[string]interface{}{
		"name": "rs", "enforcement": "active",
	})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("create ruleset: %d", resp.StatusCode)
	}
	rs := decodeJSON(t, resp)
	rsID := int(rs["id"].(float64))
	resp = ghDo(t, "PUT", fmt.Sprintf("/api/v3/repos/admin/reads-rulesuites/rulesets/%d", rsID), defaultToken,
		map[string]interface{}{"name": "rs-renamed"})
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("update ruleset: %d", resp.StatusCode)
	}
	resp = ghGet(t, fmt.Sprintf("/api/v3/repos/admin/reads-rulesuites/rulesets/%d/history", rsID), defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("ruleset history: %d", resp.StatusCode)
	}
	if versions := decodeJSONArray(t, resp); len(versions) == 0 {
		t.Fatalf("expected ruleset history versions, got none")
	}
}

func TestIssueCommentReactionDelete(t *testing.T) {
	createReadsRepo(t, "reads-icreact", nil)
	resp := ghPost(t, "/api/v3/repos/admin/reads-icreact/issues", defaultToken,
		map[string]interface{}{"title": "react here"})
	issue := decodeJSON(t, resp)
	issueNumber := int(issue["number"].(float64))

	resp = ghPost(t, fmt.Sprintf("/api/v3/repos/admin/reads-icreact/issues/%d/comments", issueNumber), defaultToken,
		map[string]interface{}{"body": "a comment"})
	comment := decodeJSON(t, resp)
	commentID := int(comment["id"].(float64))

	resp = ghPost(t, fmt.Sprintf("/api/v3/repos/admin/reads-icreact/issues/comments/%d/reactions", commentID), defaultToken,
		map[string]interface{}{"content": "heart"})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("create reaction: %d", resp.StatusCode)
	}
	reaction := decodeJSON(t, resp)
	reactionID := int(reaction["id"].(float64))

	resp = ghDelete(t, fmt.Sprintf("/api/v3/repos/admin/reads-icreact/issues/comments/%d/reactions/%d", commentID, reactionID), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete issue comment reaction: expected 204, got %d", resp.StatusCode)
	}

	resp = ghGet(t, fmt.Sprintf("/api/v3/repos/admin/reads-icreact/issues/comments/%d/reactions", commentID), defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list reactions: %d", resp.StatusCode)
	}
	if rows := decodeJSONArray(t, resp); len(rows) != 0 {
		t.Fatalf("expected reaction gone, got %v", rows)
	}

	// Deleting it again is a 404.
	resp = ghDelete(t, fmt.Sprintf("/api/v3/repos/admin/reads-icreact/issues/comments/%d/reactions/%d", commentID, reactionID), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("double delete: expected 404, got %d", resp.StatusCode)
	}
}

func TestPullReviewCommentsAndReactionDelete(t *testing.T) {
	createReadsRepo(t, "reads-prreview", map[string]interface{}{"auto_init": true})
	createReadsBranch(t, "reads-prreview", "review-branch")
	putReadsFile(t, "reads-prreview", "code.txt", "reviewed line\n", "feature commit", "review-branch")

	resp := ghPost(t, "/api/v3/repos/admin/reads-prreview/pulls", defaultToken, map[string]interface{}{
		"title": "review PR", "head": "review-branch", "base": "main",
	})
	pr := decodeJSON(t, resp)
	prNumber := int(pr["number"].(float64))

	// Create a review carrying a draft comment (the create-review API's
	// comments array).
	resp = ghPost(t, fmt.Sprintf("/api/v3/repos/admin/reads-prreview/pulls/%d/reviews", prNumber), defaultToken,
		map[string]interface{}{
			"body":  "needs work",
			"event": "COMMENT",
			"comments": []map[string]interface{}{
				{"path": "code.txt", "body": "inline note", "line": 1},
			},
		})
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("create review: %d", resp.StatusCode)
	}
	review := decodeJSON(t, resp)
	reviewID := int(review["id"].(float64))

	resp = ghGet(t, fmt.Sprintf("/api/v3/repos/admin/reads-prreview/pulls/%d/reviews/%d/comments", prNumber, reviewID), defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list review comments: %d", resp.StatusCode)
	}
	comments := decodeJSONArray(t, resp)
	if len(comments) != 1 || comments[0]["body"] != "inline note" {
		t.Fatalf("expected the review's inline comment, got %v", comments)
	}
	if int(comments[0]["pull_request_review_id"].(float64)) != reviewID {
		t.Fatalf("comment not linked to review: %v", comments[0])
	}
	commentID := int(comments[0]["id"].(float64))

	// An unknown review is a 404.
	resp = ghGet(t, fmt.Sprintf("/api/v3/repos/admin/reads-prreview/pulls/%d/reviews/999999/comments", prNumber), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("comments of unknown review: expected 404, got %d", resp.StatusCode)
	}

	// React to the review comment, then delete the reaction through the
	// four-segment path.
	resp = ghPost(t, fmt.Sprintf("/api/v3/repos/admin/reads-prreview/pulls/comments/%d/reactions", commentID), defaultToken,
		map[string]interface{}{"content": "+1"})
	if resp.StatusCode != 201 {
		resp.Body.Close()
		t.Fatalf("create review comment reaction: %d", resp.StatusCode)
	}
	reaction := decodeJSON(t, resp)
	reactionID := int(reaction["id"].(float64))

	resp = ghDelete(t, fmt.Sprintf("/api/v3/repos/admin/reads-prreview/pulls/comments/%d/reactions/%d", commentID, reactionID), defaultToken)
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete review comment reaction: expected 204, got %d", resp.StatusCode)
	}
	resp = ghGet(t, fmt.Sprintf("/api/v3/repos/admin/reads-prreview/pulls/comments/%d/reactions", commentID), defaultToken)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		t.Fatalf("list review comment reactions: %d", resp.StatusCode)
	}
	if rows := decodeJSONArray(t, resp); len(rows) != 0 {
		t.Fatalf("expected reaction gone, got %v", rows)
	}
}
