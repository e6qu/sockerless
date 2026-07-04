package bleephub

import (
	"strconv"
	"strings"
	"testing"
)

func TestRepositoryImport_GitSourceLifecycle(t *testing.T) {
	source := createRepoWriteRepo(t, true)
	target := createRepoWriteRepo(t, false)
	base := "/api/v3/repos/admin/" + target + "/import"

	// No import yet.
	resp := ghGet(t, base, defaultToken)
	requireStatus(t, resp, 404)

	// vcs_url is required.
	resp = ghPut(t, base, defaultToken, map[string]interface{}{})
	requireStatus(t, resp, 422)

	// Start a real git import from the source repo's smart HTTP endpoint.
	resp = ghPut(t, base, defaultToken, map[string]interface{}{
		"vcs":     "git",
		"vcs_url": testBaseURL + "/admin/" + source + ".git",
	})
	imp := decodeJSONWithStatus(t, resp, 201)
	if imp["status"] != "complete" {
		t.Fatalf("import status = %v (error=%v), want complete", imp["status"], imp["error_message"])
	}
	if imp["vcs"] != "git" {
		t.Fatalf("vcs = %v, want git", imp["vcs"])
	}
	if imp["commit_count"] != float64(1) {
		t.Fatalf("commit_count = %v, want 1", imp["commit_count"])
	}
	if imp["authors_count"] != float64(1) {
		t.Fatalf("authors_count = %v, want 1", imp["authors_count"])
	}
	if imp["import_percent"] != float64(100) {
		t.Fatalf("import_percent = %v, want 100", imp["import_percent"])
	}
	if !strings.HasSuffix(imp["authors_url"].(string), "/import/authors") {
		t.Fatalf("authors_url = %v", imp["authors_url"])
	}

	// The import really landed: the source's README is in the target's git.
	resp = ghGet(t, "/api/v3/repos/admin/"+target+"/contents/README.md", defaultToken)
	requireStatus(t, resp, 200)

	// Status reads back.
	resp = ghGet(t, base, defaultToken)
	got := decodeJSONWithStatus(t, resp, 200)
	if got["status"] != "complete" {
		t.Fatalf("GET status = %v, want complete", got["status"])
	}

	// Authors were discovered from the imported commits and can be remapped.
	resp = ghGet(t, base+"/authors", defaultToken)
	authors := decodeJSONWithStatus2xxArray(t, resp, 200)
	if len(authors) != 1 {
		t.Fatalf("authors = %v, want 1 entry", authors)
	}
	authorID := strconv.Itoa(int(authors[0]["id"].(float64)))
	if authors[0]["remote_id"] == "" || authors[0]["email"] == nil {
		t.Fatalf("author entry missing members: %v", authors[0])
	}
	resp = ghPatch(t, base+"/authors/"+authorID, defaultToken, map[string]interface{}{
		"name":  "Mapped Author",
		"email": "mapped@example.com",
	})
	updated := decodeJSONWithStatus(t, resp, 200)
	if updated["name"] != "Mapped Author" || updated["email"] != "mapped@example.com" {
		t.Fatalf("updated author = %v", updated)
	}
	// Unknown author → 404.
	resp = ghPatch(t, base+"/authors/424242", defaultToken, map[string]interface{}{"name": "x"})
	requireStatus(t, resp, 404)

	// Git LFS preference round-trips.
	resp = ghPatch(t, base+"/lfs", defaultToken, map[string]interface{}{"use_lfs": "opt_in"})
	lfs := decodeJSONWithStatus(t, resp, 200)
	if lfs["use_lfs"] != true {
		t.Fatalf("use_lfs = %v, want true", lfs["use_lfs"])
	}
	resp = ghPatch(t, base+"/lfs", defaultToken, map[string]interface{}{"use_lfs": "bogus"})
	requireStatus(t, resp, 422)

	// No files over the 100 MB threshold.
	resp = ghGet(t, base+"/large_files", defaultToken)
	largeFiles := decodeJSONWithStatus2xxArray(t, resp, 200)
	if len(largeFiles) != 0 {
		t.Fatalf("large_files = %v, want empty", largeFiles)
	}

	// PATCH restarts the import with updated parameters.
	resp = ghPatch(t, base, defaultToken, map[string]interface{}{"vcs": "git"})
	restarted := decodeJSONWithStatus(t, resp, 200)
	if restarted["status"] != "complete" {
		t.Fatalf("restarted status = %v, want complete", restarted["status"])
	}

	// Cancel removes the import record.
	delResp := ghDelete(t, base, defaultToken)
	requireStatus(t, delResp, 204)
	resp = ghGet(t, base, defaultToken)
	requireStatus(t, resp, 404)
	delResp = ghDelete(t, base, defaultToken)
	requireStatus(t, delResp, 404)
}

func TestRepositoryImport_HonestFailures(t *testing.T) {
	target := createRepoWriteRepo(t, false)
	base := "/api/v3/repos/admin/" + target + "/import"

	// A VCS type bleephub cannot really import ends in an honest error.
	resp := ghPut(t, base, defaultToken, map[string]interface{}{
		"vcs":     "subversion",
		"vcs_url": "https://svn.example.invalid/repo",
	})
	imp := decodeJSONWithStatus(t, resp, 201)
	if imp["status"] != "error" {
		t.Fatalf("subversion import status = %v, want error", imp["status"])
	}
	msg, _ := imp["error_message"].(string)
	if !strings.Contains(msg, "subversion") {
		t.Fatalf("error_message = %q, want mention of subversion", msg)
	}

	// An unreachable git remote ends in an honest failure state too.
	resp = ghPut(t, base, defaultToken, map[string]interface{}{
		"vcs":     "git",
		"vcs_url": "http://127.0.0.1:1/unreachable.git",
	})
	imp = decodeJSONWithStatus(t, resp, 201)
	status, _ := imp["status"].(string)
	if status == "complete" {
		t.Fatalf("unreachable remote import status = %v, want a failure state", status)
	}
	if imp["error_message"] == nil {
		t.Fatal("unreachable remote import has no error_message")
	}
}
