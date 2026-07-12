package bleephub

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// testCodeQLDatabaseBundle creates the same relocatable archive shape emitted
// by `codeql database bundle`: a manifest and non-empty language dataset under
// one database root.
func testCodeQLDatabaseBundle(t *testing.T, language, marker string) []byte {
	t.Helper()
	return testCodeQLDatabaseBundleWithManifest(t, language, "primaryLanguage: "+language+"\n", marker)
}

func testCodeQLDatabaseBundleWithManifest(t *testing.T, language, manifestYAML, marker string) []byte {
	t.Helper()
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	root := language + "-database"
	manifest, err := zw.Create(root + "/codeql-database.yml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.Write([]byte(manifestYAML)); err != nil {
		t.Fatal(err)
	}
	dataset, err := zw.Create(root + "/db-" + language + "/default/cache/pages/0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dataset.Write([]byte(marker)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// uploadCodeQLDatabase sends the raw uploads.github.com request used by the
// official github/codeql-action and returns the public REST representation.
func uploadCodeQLDatabase(t *testing.T, repoFullName, language, commitOID string, content []byte) map[string]any {
	t.Helper()
	resp := postCodeQLDatabase(t, defaultToken, repoFullName, language, language+"-database", commitOID, "application/zip", content)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload CodeQL database: %d", resp.StatusCode)
	}
	get := ghGet(t, "/api/v3/repos/"+repoFullName+"/code-scanning/codeql/databases/"+language, defaultToken)
	if get.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(get.Body)
		get.Body.Close()
		t.Fatalf("get uploaded CodeQL database: %d body=%s", get.StatusCode, b)
	}
	return decodeJSON(t, get)
}

func postCodeQLDatabase(t *testing.T, token, repoFullName, language, name, commitOID, contentType string, content []byte) *http.Response {
	t.Helper()
	path := "/repos/" + repoFullName + "/code-scanning/codeql/databases/" + language +
		"?name=" + name + "&commit_oid=" + commitOID
	req, err := http.NewRequest(http.MethodPost, testBaseURL+path, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("create CodeQL database upload: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(content)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload CodeQL database: %v", err)
	}
	return resp
}

func assertNoInternalURL(t *testing.T, value any) {
	t.Helper()
	var walk func(any, string)
	walk = func(v any, path string) {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				walk(child, path+"."+k)
			}
		case []any:
			for i, child := range x {
				walk(child, fmt.Sprintf("%s[%d]", path, i))
			}
		case string:
			if strings.Contains(x, "/internal/") {
				t.Fatalf("%s contains internal URL %q", path, x)
			}
		}
	}
	walk(value, "$")
}

// putRepoFile creates or updates a file via the contents API, returning
// the commit SHA. This is how the autofix tests give the target branch
// real git content.
func putRepoFile(t *testing.T, repoFullName, path, content, message string) string {
	t.Helper()
	resp := ghPut(t, "/api/v3/repos/"+repoFullName+"/contents/"+path, defaultToken, map[string]interface{}{
		"message": message,
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("put contents %s: %d body=%s", path, resp.StatusCode, b)
	}
	out := decodeJSON(t, resp)
	commit := out["commit"].(map[string]interface{})
	return commit["sha"].(string)
}

// --- organization code scanning alerts ---

func TestCodeScanningOrgAlerts_List(t *testing.T) {
	org := seedTestOrg(t, "cs-org-alerts")
	repo := seedOrgRepo(t, org, "cs-org-repo", false)

	seedCodeScanningAlert(t, org.Login, "cs-org-repo", "org-rule-a", "error", "CodeQL")
	seedCodeScanningAlert(t, org.Login, "cs-org-repo", "org-rule-b", "warning", "Semgrep")

	resp := ghGet(t, "/api/v3/orgs/"+org.Login+"/code-scanning/alerts", defaultToken)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("org alerts list: %d body=%s", resp.StatusCode, b)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode org alerts: %v", err)
	}
	resp.Body.Close()
	if len(list) != 2 {
		t.Fatalf("org alerts = %d, want 2", len(list))
	}
	repoJSON, _ := list[0]["repository"].(map[string]any)
	if repoJSON == nil || repoJSON["full_name"] != repo.FullName {
		t.Fatalf("org alert repository = %v, want %s", list[0]["repository"], repo.FullName)
	}

	// Severity filter.
	resp = ghGet(t, "/api/v3/orgs/"+org.Login+"/code-scanning/alerts?severity=error", defaultToken)
	var filtered []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&filtered); err != nil {
		t.Fatalf("decode filtered org alerts: %v", err)
	}
	resp.Body.Close()
	if len(filtered) != 1 {
		t.Fatalf("severity-filtered org alerts = %d, want 1", len(filtered))
	}

	// Unknown org.
	mustStatus(t, ghGet(t, "/api/v3/orgs/no-such-org/code-scanning/alerts", defaultToken), 404, "unknown org alerts")
}

// --- Copilot Autofix ---

func TestCodeScanningAutofix_GenerateAndCommit(t *testing.T) {
	repo := seedTestRepo(t, "cs-autofix", false)

	// Give the default branch real content at the alert's flagged path.
	fileContent := strings.Repeat("line\n", 11) + "tail"
	putRepoFile(t, repo.FullName, "src/index.js", fileContent, "seed vulnerable file")

	alert := seedCodeScanningAlert(t, "admin", "cs-autofix", "js/sql-injection", "error", "CodeQL")
	number := int(alert["number"].(float64))
	autofixPath := fmt.Sprintf("/api/v3/repos/%s/code-scanning/alerts/%d/autofix", repo.FullName, number)

	// No autofix yet.
	mustStatus(t, ghGet(t, autofixPath, defaultToken), 404, "autofix before generation")

	// Committing before an autofix exists is a 400.
	mustStatus(t, ghPost(t, autofixPath+"/commits", defaultToken, map[string]interface{}{
		"target_ref": "refs/heads/main",
	}), 400, "commit before autofix")

	// Generate: 202 on first trigger, 200 when it already exists.
	resp := ghPost(t, autofixPath, defaultToken, nil)
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create autofix: %d body=%s", resp.StatusCode, b)
	}
	created := decodeJSON(t, resp)
	if created["status"] != "success" {
		t.Fatalf("autofix status = %v, want success", created["status"])
	}
	if desc, _ := created["description"].(string); !strings.Contains(desc, "js/sql-injection") {
		t.Fatalf("autofix description = %v, want rule reference", created["description"])
	}
	mustStatus(t, ghPost(t, autofixPath, defaultToken, nil), 200, "re-create autofix")

	// GET returns the stored autofix.
	resp = ghGet(t, autofixPath, defaultToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get autofix: %d", resp.StatusCode)
	}
	got := decodeJSON(t, resp)
	if got["status"] != "success" || got["started_at"] == nil {
		t.Fatalf("autofix = %v, want success with started_at", got)
	}

	// Commit against a branch that does not exist is a 422.
	mustStatus(t, ghPost(t, autofixPath+"/commits", defaultToken, map[string]interface{}{
		"target_ref": "refs/heads/no-such-branch",
	}), 422, "commit to missing branch")

	// Commit onto main: a real commit lands on the branch.
	resp = ghPost(t, autofixPath+"/commits", defaultToken, map[string]interface{}{
		"target_ref": "refs/heads/main",
		"message":    "Apply Copilot Autofix",
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("commit autofix: %d body=%s", resp.StatusCode, b)
	}
	committed := decodeJSON(t, resp)
	if committed["target_ref"] != "refs/heads/main" {
		t.Fatalf("commit target_ref = %v, want refs/heads/main", committed["target_ref"])
	}
	sha, _ := committed["sha"].(string)
	if len(sha) != 40 {
		t.Fatalf("commit sha = %q, want a 40-char SHA", sha)
	}

	// The branch head is the autofix commit and it changed the flagged file.
	resp = ghGet(t, "/api/v3/repos/"+repo.FullName+"/commits", defaultToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list commits: %d", resp.StatusCode)
	}
	var commits []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		t.Fatalf("decode commits: %v", err)
	}
	resp.Body.Close()
	if len(commits) == 0 || commits[0]["sha"] != sha {
		t.Fatalf("branch head = %v, want autofix commit %s", commits[0]["sha"], sha)
	}

	resp = ghGet(t, "/api/v3/repos/"+repo.FullName+"/contents/src/index.js", defaultToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get fixed file: %d", resp.StatusCode)
	}
	file := decodeJSON(t, resp)
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(file["content"].(string), "\n", ""))
	if err != nil {
		t.Fatalf("decode fixed file: %v", err)
	}
	if !strings.Contains(string(raw), "autofix: ") {
		t.Fatal("fixed file does not contain the applied autofix edit")
	}
}

func TestCodeScanningAutofix_NotEligible(t *testing.T) {
	repo := seedTestRepo(t, "cs-autofix-elig", false)
	alert := seedCodeScanningAlert(t, "admin", "cs-autofix-elig", "js/xss", "warning", "CodeQL")
	number := int(alert["number"].(float64))
	autofixPath := fmt.Sprintf("/api/v3/repos/%s/code-scanning/alerts/%d/autofix", repo.FullName, number)

	// Dismiss the alert; generation must refuse with a 422.
	resp := ghPatch(t, fmt.Sprintf("/api/v3/repos/%s/code-scanning/alerts/%d", repo.FullName, number), defaultToken, map[string]interface{}{
		"state":            "dismissed",
		"dismissed_reason": "won't_fix",
	})
	mustStatus(t, resp, 200, "dismiss alert")
	mustStatus(t, ghPost(t, autofixPath, defaultToken, nil), 422, "autofix for dismissed alert")

	// Unknown alert number.
	mustStatus(t, ghPost(t, "/api/v3/repos/"+repo.FullName+"/code-scanning/alerts/99999/autofix", defaultToken, nil), 404, "autofix for unknown alert")
}

// --- CodeQL databases ---

func TestCodeQLDatabases_RoundTrip(t *testing.T) {
	repo := seedTestRepo(t, "codeql-dbs", false)
	commitOID := putRepoFile(t, repo.FullName, "main.go", "package main\n", "add source")
	dbBytes := testCodeQLDatabaseBundle(t, "go", "codeql-database-archive-bytes")

	created := uploadCodeQLDatabase(t, repo.FullName, "go", commitOID, dbBytes)
	if created["language"] != "go" {
		t.Fatalf("seeded language = %v, want go", created["language"])
	}

	// List.
	resp := ghGet(t, "/api/v3/repos/"+repo.FullName+"/code-scanning/codeql/databases", defaultToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list databases: %d", resp.StatusCode)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode databases: %v", err)
	}
	resp.Body.Close()
	if len(list) != 1 {
		t.Fatalf("databases = %d, want 1", len(list))
	}
	db := list[0]
	if db["name"] != "go-database" || db["language"] != "go" || db["content_type"] != "application/zip" {
		t.Fatalf("database = %v", db)
	}
	if db["size"] != float64(len(dbBytes)) {
		t.Fatalf("size = %v, want %d", db["size"], len(dbBytes))
	}
	if db["commit_oid"] != commitOID {
		t.Fatalf("commit_oid = %v, want %s", db["commit_oid"], commitOID)
	}
	uploader, _ := db["uploader"].(map[string]any)
	if uploader == nil || uploader["login"] != "admin" {
		t.Fatalf("uploader = %v, want admin", db["uploader"])
	}
	assertNoInternalURL(t, db)

	// Get one.
	resp = ghGet(t, "/api/v3/repos/"+repo.FullName+"/code-scanning/codeql/databases/go", defaultToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get database: %d", resp.StatusCode)
	}
	got := decodeJSON(t, resp)
	if got["language"] != "go" {
		t.Fatalf("get database language = %v, want go", got["language"])
	}

	// With Accept set to the archive content type, the redirect resolves
	// to the real database bytes.
	req, err := http.NewRequest("GET", testBaseURL+"/api/v3/repos/"+repo.FullName+"/code-scanning/codeql/databases/go", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "token "+defaultToken)
	req.Header.Set("Accept", "application/zip")
	noFollow := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	redirectResp, err := noFollow.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	redirectResp.Body.Close()
	if redirectResp.StatusCode != http.StatusFound {
		t.Fatalf("database download status = %d, want 302", redirectResp.StatusCode)
	}
	loc := redirectResp.Header.Get("Location")
	if loc == "" || strings.Contains(loc, "/internal/") {
		t.Fatalf("database download redirect Location = %q, want public non-internal URL", loc)
	}

	dlResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(dlResp.Body)
	dlResp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if dlResp.StatusCode != http.StatusOK {
		t.Fatalf("download database: %d body=%s", dlResp.StatusCode, raw)
	}
	if got := dlResp.Header.Get("Content-Disposition"); got != `attachment; filename="go-database.zip"` {
		t.Fatalf("database Content-Disposition = %q", got)
	}
	if got := dlResp.Header.Get("Content-Length"); got != fmt.Sprintf("%d", len(dbBytes)) {
		t.Fatalf("database Content-Length = %q, want %d", got, len(dbBytes))
	}
	if !bytes.Equal(raw, dbBytes) {
		t.Fatalf("downloaded bytes = %q, want %q", raw, dbBytes)
	}

	// Unknown language.
	mustStatus(t, ghGet(t, "/api/v3/repos/"+repo.FullName+"/code-scanning/codeql/databases/ruby", defaultToken), 404, "get unknown database")

	// Delete.
	mustStatus(t, ghDelete(t, "/api/v3/repos/"+repo.FullName+"/code-scanning/codeql/databases/go", defaultToken), 204, "delete database")
	mustStatus(t, ghGet(t, "/api/v3/repos/"+repo.FullName+"/code-scanning/codeql/databases/go", defaultToken), 404, "get deleted database")
	mustStatus(t, ghDelete(t, "/api/v3/repos/"+repo.FullName+"/code-scanning/codeql/databases/go", defaultToken), 404, "delete deleted database")
}

func TestCodeQLDatabases_BytesUseObjectStore(t *testing.T) {
	repo := seedTestRepo(t, "codeql-dbs-object", false)
	objectFS, objectStore := newObjectByteStoreForTest(t)
	oldStore := testServer.store.ObjectByteStore
	testServer.store.ObjectByteStore = objectStore
	t.Cleanup(func() {
		testServer.store.ObjectByteStore = oldStore
	})

	commitOID := putRepoFile(t, repo.FullName, "main.go", "package main\n", "add source")
	dbBytes := testCodeQLDatabaseBundle(t, "go", "object-backed-codeql-database")
	uploadCodeQLDatabase(t, repo.FullName, "go", commitOID, dbBytes)
	db := testServer.store.GetCodeQLDatabase(repo.FullName, "go")
	if db == nil {
		t.Fatal("CodeQL database missing after seed")
	}
	if got := string(readS3TestFile(t, objectFS, db.StoragePath)); got != string(dbBytes) {
		t.Fatalf("CodeQL database object bytes = %q, want %q", got, dbBytes)
	}
	if len(db.Content) != 0 {
		t.Fatalf("CodeQL database metadata retained %d raw bytes; bytes must live in object storage", len(db.Content))
	}

	req, err := http.NewRequest("GET", testBaseURL+"/api/v3/repos/"+repo.FullName+"/code-scanning/codeql/databases/go", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "token "+defaultToken)
	req.Header.Set("Accept", "application/zip")
	dlResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(dlResp.Body)
	dlResp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if dlResp.StatusCode != http.StatusOK {
		t.Fatalf("download object-backed database: %d body=%s", dlResp.StatusCode, raw)
	}
	if !bytes.Equal(raw, dbBytes) {
		t.Fatalf("downloaded object-backed bytes = %q, want %q", raw, dbBytes)
	}

	mustStatus(t, ghDelete(t, "/api/v3/repos/"+repo.FullName+"/code-scanning/codeql/databases/go", defaultToken), http.StatusNoContent, "delete object-backed database")
	if _, err := objectFS.Open(db.StoragePath); err == nil {
		t.Fatalf("CodeQL database object %s survived database deletion", db.StoragePath)
	}
}

func TestCodeQLDatabaseUpload_ValidatesOfficialActionProtocol(t *testing.T) {
	repo := seedTestRepo(t, "codeql-upload-contract", false)
	commitOID := putRepoFile(t, repo.FullName, "main.go", "package main\n", "add source")
	bundle := testCodeQLDatabaseBundle(t, "go", "dataset")

	resp := postCodeQLDatabase(t, "", repo.FullName, "go", "go-database", commitOID, "application/zip", bundle)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated upload = %d, want 401", resp.StatusCode)
	}

	resp = postCodeQLDatabase(t, defaultToken, repo.FullName, "go", "go-database", commitOID, "application/octet-stream", bundle)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("wrong content type = %d, want 415", resp.StatusCode)
	}

	for name, tc := range map[string]struct {
		language string
		commit   string
		bundle   []byte
	}{
		"non-zip":            {language: "go", commit: commitOID, bundle: []byte("not a database")},
		"language mismatch":  {language: "python", commit: commitOID, bundle: bundle},
		"unknown commit":     {language: "go", commit: strings.Repeat("f", 40), bundle: bundle},
		"unfinalized":        {language: "go", commit: commitOID, bundle: testCodeQLDatabaseBundleWithManifest(t, "go", "primaryLanguage: go\ninProgress: false\n", "dataset")},
		"manifest mismatch":  {language: "go", commit: commitOID, bundle: testCodeQLDatabaseBundleWithManifest(t, "go", "primaryLanguage: python\n", "dataset")},
		"oversized manifest": {language: "go", commit: commitOID, bundle: testCodeQLDatabaseBundleWithManifest(t, "go", "primaryLanguage: go\n#"+strings.Repeat("x", 1<<20), "dataset")},
	} {
		t.Run(name, func(t *testing.T) {
			resp := postCodeQLDatabase(t, defaultToken, repo.FullName, tc.language, tc.language+"-database", tc.commit, "application/zip", tc.bundle)
			resp.Body.Close()
			if resp.StatusCode != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422", resp.StatusCode)
			}
		})
	}

	var unsafe bytes.Buffer
	zw := zip.NewWriter(&unsafe)
	w, err := zw.Create("../go-database/codeql-database.yml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("primaryLanguage: go\n"))
	w, err = zw.Create("go-database/db-go/default/cache/pages/0")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("dataset"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	resp = postCodeQLDatabase(t, defaultToken, repo.FullName, "go", "go-database", commitOID, "application/zip", unsafe.Bytes())
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unsafe archive = %d, want 422", resp.StatusCode)
	}

	oldRoute, err := authedPost("/internal/repos/"+repo.FullName+"/code-scanning/codeql/databases", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	oldRoute.Body.Close()
	if oldRoute.StatusCode != http.StatusNotFound {
		t.Fatalf("obsolete internal route = %d, want 404", oldRoute.StatusCode)
	}
	if got := testServer.store.GetCodeQLDatabase(repo.FullName, "go"); got != nil {
		t.Fatalf("invalid uploads created database %+v", got)
	}
}

func TestCodeQLDatabaseProductionHasNoOperatorSeedRoute(t *testing.T) {
	obsoleteRoute := "/internal/repos/" + "{owner}/{repo}/code-scanning/codeql/databases"
	for _, file := range []string{"gh_code_scanning.go", "../specs/BLEEPHUB_GITHUB_API_PARITY.md"} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if strings.Contains(string(source), obsoleteRoute) {
			t.Fatalf("%s reintroduced the obsolete operator CodeQL database seed route", file)
		}
	}
}

func TestCodeQLDatabaseUpload_ActionsInstallationTokenLifecycle(t *testing.T) {
	admin := testServer.store.UsersByLogin["admin"]
	repo := seedTestRepo(t, "codeql-actions-token", true)
	other := seedTestRepo(t, "codeql-actions-token-other", true)
	commitOID := putRepoFile(t, repo.FullName, "main.go", "package main\n", "add source")
	otherCommit := putRepoFile(t, other.FullName, "main.go", "package main\n", "add source")
	bundle := testCodeQLDatabaseBundle(t, "go", "installation dataset")

	permissions := map[string]string{"security_events": "write", "contents": "write"}
	app := testServer.store.CreateApp(admin.ID, "CodeQL Database Producer", "", permissions, nil)
	installation := testServer.store.CreateInstallation(app.ID, "User", admin.ID, admin.Login, permissions, nil)
	token := testServer.store.CreateInstallationToken(installation.ID, app.ID, permissions, []int{repo.ID})

	resp := postCodeQLDatabase(t, token.Token, repo.FullName, "go", "go-database", commitOID, "application/zip", bundle)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("installation upload = %d, want 201", resp.StatusCode)
	}

	get := ghGet(t, "/api/v3/repos/"+repo.FullName+"/code-scanning/codeql/databases/go", token.Token)
	if get.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(get.Body)
		get.Body.Close()
		t.Fatalf("installation get = %d body=%s", get.StatusCode, body)
	}
	database := decodeJSON(t, get)
	uploader := database["uploader"].(map[string]any)
	if uploader["login"] != app.Slug+"[bot]" || uploader["type"] != "Bot" {
		t.Fatalf("installation uploader = %v", uploader)
	}

	resp = postCodeQLDatabase(t, token.Token, other.FullName, "go", "go-database", otherCommit, "application/zip", bundle)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("token upload outside selected repository = %d, want 403", resp.StatusCode)
	}

	contentsOnly := testServer.store.CreateInstallationToken(installation.ID, app.ID, map[string]string{"contents": "write"}, []int{repo.ID})
	resp = postCodeQLDatabase(t, contentsOnly.Token, repo.FullName, "go", "go-database", commitOID, "application/zip", bundle)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("upload without security_events:write = %d, want 403", resp.StatusCode)
	}

	sarif := map[string]any{
		"version": "2.1.0",
		"runs": []map[string]any{{
			"tool": map[string]any{"driver": map[string]any{"name": "CodeQL"}},
			"results": []map[string]any{{
				"ruleId":  "go/installation-producer",
				"message": map[string]any{"text": "installation-token result"},
			}},
		}},
	}
	sarifBytes, err := json.Marshal(sarif)
	if err != nil {
		t.Fatal(err)
	}
	sarifResponse := ghPost(t, "/api/v3/repos/"+repo.FullName+"/code-scanning/sarifs", token.Token, map[string]any{
		"commit_sha": commitOID,
		"ref":        "refs/heads/main",
		"sarif":      base64.StdEncoding.EncodeToString(sarifBytes),
	})
	if sarifResponse.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(sarifResponse.Body)
		sarifResponse.Body.Close()
		t.Fatalf("installation SARIF upload = %d body=%s", sarifResponse.StatusCode, body)
	}
	sarifUpload := decodeJSON(t, sarifResponse)
	status := ghGet(t, "/api/v3/repos/"+repo.FullName+"/code-scanning/sarifs/"+sarifUpload["id"].(string), token.Token)
	if status.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(status.Body)
		status.Body.Close()
		t.Fatalf("installation SARIF status = %d body=%s", status.StatusCode, body)
	}
	statusBody := decodeJSON(t, status)
	if statusBody["processing_status"] != "complete" {
		t.Fatalf("installation SARIF status = %v", statusBody)
	}
	deniedSARIF := ghPost(t, "/api/v3/repos/"+repo.FullName+"/code-scanning/sarifs", contentsOnly.Token, map[string]any{
		"commit_sha": commitOID,
		"ref":        "refs/heads/main",
		"sarif":      base64.StdEncoding.EncodeToString(sarifBytes),
	})
	deniedSARIF.Body.Close()
	if deniedSARIF.StatusCode != http.StatusForbidden {
		t.Fatalf("SARIF upload without security_events:write = %d, want 403", deniedSARIF.StatusCode)
	}

	deleteResp := ghDelete(t, "/api/v3/repos/"+repo.FullName+"/code-scanning/codeql/databases/go", token.Token)
	deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusNoContent {
		t.Fatalf("installation delete = %d, want 204", deleteResp.StatusCode)
	}
}

func TestCodeQLDatabaseUpload_ObjectFailurePreservesPreviousDatabase(t *testing.T) {
	repo := seedTestRepo(t, "codeql-object-atomic", false)
	firstCommit := putRepoFile(t, repo.FullName, "main.go", "package main\n", "add source")
	firstBundle := testCodeQLDatabaseBundle(t, "go", "first dataset")
	objectFS, goodStore := newObjectByteStoreForTest(t)
	oldStore := testServer.store.ObjectByteStore
	testServer.store.ObjectByteStore = goodStore
	t.Cleanup(func() { testServer.store.ObjectByteStore = oldStore })

	created := uploadCodeQLDatabase(t, repo.FullName, "go", firstCommit, firstBundle)
	databaseID := int(created["id"].(float64))
	secondCommit := putRepoFile(t, repo.FullName, "second.go", "package main\n", "add second source")
	secondBundle := testCodeQLDatabaseBundle(t, "go", "replacement dataset")
	testServer.store.ObjectByteStore = &s3ActionsByteStore{fs: &s3FS{client: objectFS.client, bucket: "missing-bucket", prefix: objectFS.prefix}}

	resp := postCodeQLDatabase(t, defaultToken, repo.FullName, "go", "replacement", secondCommit, "application/zip", secondBundle)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("failed replacement = %d body=%s, want 500", resp.StatusCode, body)
	}

	testServer.store.ObjectByteStore = goodStore
	database := testServer.store.GetCodeQLDatabase(repo.FullName, "go")
	if database == nil || database.CommitOID != firstCommit || database.Name != "go-database" {
		t.Fatalf("database changed after object failure: %+v", database)
	}
	if database.ID != databaseID {
		t.Fatalf("database ID changed after object failure: got %d want %d", database.ID, databaseID)
	}
	if got := readS3TestFile(t, objectFS, database.StoragePath); !bytes.Equal(got, firstBundle) {
		t.Fatalf("database bytes changed after object failure")
	}
	previousPath := database.StoragePath
	replaced := uploadCodeQLDatabase(t, repo.FullName, "go", secondCommit, secondBundle)
	database = testServer.store.GetCodeQLDatabase(repo.FullName, "go")
	if database == nil || database.CommitOID != secondCommit || database.StoragePath == previousPath || database.Name != "go-database" {
		t.Fatalf("successful replacement database = %+v response=%v", database, replaced)
	}
	if _, err := objectFS.Open(previousPath); err == nil {
		t.Fatalf("replaced CodeQL database archive %s survived successful replacement", previousPath)
	}
	if got := readS3TestFile(t, objectFS, database.StoragePath); !bytes.Equal(got, secondBundle) {
		t.Fatalf("replacement database bytes differ")
	}
}

func TestCodeQLDatabaseUpload_PersistenceFailurePreservesPreviousDatabase(t *testing.T) {
	t.Setenv("BLEEPHUB_PERSIST", "true")
	t.Setenv("BLEEPHUB_DATA_DIR", t.TempDir())
	persistence, err := NewPersistence()
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	if err := store.SetPersistence(persistence); err != nil {
		t.Fatal(err)
	}
	_, objectStore := newObjectByteStoreForTest(t)
	store.ObjectByteStore = objectStore
	store.SeedDefaultUser()
	user := store.UsersByLogin["admin"]
	first := []byte("first durable CodeQL database")
	database, err := store.UpsertCodeQLDatabase("admin/repo", "go", "go-database", "application/zip", strings.Repeat("1", 40), first, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	previousPath := database.StoragePath
	if err := persistence.Close(); err != nil {
		t.Fatal(err)
	}

	second := []byte("replacement durable CodeQL database")
	if _, err := store.UpsertCodeQLDatabase("admin/repo", "go", "replacement", "application/zip", strings.Repeat("2", 40), second, user.ID); err == nil {
		t.Fatal("replacement with closed SQLite persistence succeeded")
	}
	got := store.GetCodeQLDatabase("admin/repo", "go")
	if got == nil || got.Name != "go-database" || got.CommitOID != strings.Repeat("1", 40) || got.StoragePath != previousPath {
		t.Fatalf("database metadata changed after persistence failure: %+v", got)
	}
	bytesAfterFailure, err := store.ReadCodeQLDatabaseContent(context.Background(), got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytesAfterFailure, first) {
		t.Fatalf("database bytes changed after persistence failure: %q", bytesAfterFailure)
	}
	newPath := codeQLDatabaseDataKey(database.ID, second)
	if _, err := objectStore.Get(context.Background(), newPath); err == nil {
		t.Fatalf("replacement object %s survived persistence rollback", newPath)
	}
}

func TestCodeQLArtifacts_PrivateRepositoryDownloadsRequireAccess(t *testing.T) {
	repo := seedTestRepo(t, "codeql-private-download", true)
	commitOID := putRepoFile(t, repo.FullName, "main.go", "package main\n", "add source")
	bundle := testCodeQLDatabaseBundle(t, "go", "private source dataset")
	uploadCodeQLDatabase(t, repo.FullName, "go", commitOID, bundle)
	databaseURL := testBaseURL + "/code-scanning/repos/" + repo.FullName + "/codeql/databases/go/download"

	unauthenticated, err := http.Get(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	unauthenticated.Body.Close()
	if unauthenticated.StatusCode != http.StatusNotFound {
		t.Fatalf("anonymous private database download = %d, want 404", unauthenticated.StatusCode)
	}
	authenticated, err := http.NewRequest(http.MethodGet, databaseURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	authenticated.Header.Set("Authorization", "token "+defaultToken)
	download, err := http.DefaultClient.Do(authenticated)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(download.Body)
	download.Body.Close()
	if download.StatusCode != http.StatusOK || !bytes.Equal(raw, bundle) {
		t.Fatalf("authorized private database download = %d bytes=%d", download.StatusCode, len(raw))
	}
}

// --- CodeQL variant analyses ---

func TestCodeQLVariantAnalyses_CreateAndReadBack(t *testing.T) {
	controller := seedTestRepo(t, "codeql-va-controller", false)
	withDB := seedTestRepo(t, "codeql-va-with-db", false)
	withoutDB := seedTestRepo(t, "codeql-va-no-db", false)

	databaseCommit := putRepoFile(t, withDB.FullName, "main.go", "package main\n", "add source")
	uploadCodeQLDatabase(t, withDB.FullName, "go", databaseCommit, testCodeQLDatabaseBundle(t, "go", "db"))

	queryPackBytes := testCodeQLQueryPack(t)
	queryPack := base64.StdEncoding.EncodeToString(queryPackBytes)
	basePath := "/api/v3/repos/" + controller.FullName + "/code-scanning/codeql/variant-analyses"

	resp := ghPost(t, basePath, defaultToken, map[string]interface{}{
		"language":   "go",
		"query_pack": queryPack,
		"repositories": []string{
			withDB.FullName,
			withoutDB.FullName,
			"admin/definitely-missing",
		},
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create variant analysis: %d body=%s", resp.StatusCode, b)
	}
	created := decodeJSON(t, resp)
	vaID := int(created["id"].(float64))
	if created["status"] != "succeeded" {
		t.Fatalf("status = %v, want succeeded", created["status"])
	}
	if created["query_language"] != "go" {
		t.Fatalf("query_language = %v, want go", created["query_language"])
	}
	scanned := created["scanned_repositories"].([]interface{})
	if len(scanned) != 1 {
		t.Fatalf("scanned_repositories = %d, want 1", len(scanned))
	}
	scannedRepo := scanned[0].(map[string]interface{})["repository"].(map[string]interface{})
	if scannedRepo["full_name"] != withDB.FullName {
		t.Fatalf("scanned repo = %v, want %s", scannedRepo["full_name"], withDB.FullName)
	}
	skipped := created["skipped_repositories"].(map[string]interface{})
	notFound := skipped["not_found_repos"].(map[string]interface{})
	if notFound["repository_count"] != float64(1) {
		t.Fatalf("not_found_repos = %v, want 1", notFound["repository_count"])
	}
	noDB := skipped["no_codeql_db_repos"].(map[string]interface{})
	if noDB["repository_count"] != float64(1) {
		t.Fatalf("no_codeql_db_repos = %v, want 1", noDB["repository_count"])
	}
	ctrl := created["controller_repo"].(map[string]interface{})
	if ctrl["full_name"] != controller.FullName {
		t.Fatalf("controller_repo = %v, want %s", ctrl["full_name"], controller.FullName)
	}
	assertNoInternalURL(t, created)

	// The advertised query_pack_url resolves to the uploaded pack bytes.
	packURL, _ := created["query_pack_url"].(string)
	if packURL == "" || strings.Contains(packURL, "/internal/") {
		t.Fatalf("query_pack_url = %q, want public non-internal URL", packURL)
	}
	req, err := http.NewRequest("GET", packURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	packResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	packRaw, _ := io.ReadAll(packResp.Body)
	packResp.Body.Close()
	if packResp.StatusCode != http.StatusOK {
		t.Fatalf("download query pack: %d body=%s", packResp.StatusCode, packRaw)
	}
	if !bytes.Equal(packRaw, queryPackBytes) {
		t.Fatalf("query pack bytes = %q, want %q", packRaw, queryPackBytes)
	}

	// Get by id.
	resp = ghGet(t, fmt.Sprintf("%s/%d", basePath, vaID), defaultToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get variant analysis: %d", resp.StatusCode)
	}
	got := decodeJSON(t, resp)
	if got["status"] != "succeeded" || got["completed_at"] == nil {
		t.Fatalf("variant analysis = %v, want succeeded with completed_at", got)
	}
	assertNoInternalURL(t, got)

	// Per-repository task.
	resp = ghGet(t, fmt.Sprintf("%s/%d/repos/%s", basePath, vaID, withDB.FullName), defaultToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get repo task: %d", resp.StatusCode)
	}
	task := decodeJSON(t, resp)
	if task["analysis_status"] != "succeeded" {
		t.Fatalf("repo task status = %v, want succeeded", task["analysis_status"])
	}
	taskRepo := task["repository"].(map[string]interface{})
	if taskRepo["full_name"] != withDB.FullName {
		t.Fatalf("repo task repository = %v, want %s", taskRepo["full_name"], withDB.FullName)
	}
	if task["database_commit_sha"] != databaseCommit {
		t.Fatalf("database_commit_sha = %v", task["database_commit_sha"])
	}

	// A repository that was not scanned is a 404 on the task endpoint.
	mustStatus(t, ghGet(t, fmt.Sprintf("%s/%d/repos/%s", basePath, vaID, withoutDB.FullName), defaultToken), 404, "task for skipped repo")

	// Unknown analysis id.
	mustStatus(t, ghGet(t, basePath+"/99999", defaultToken), 404, "unknown variant analysis")
}

func TestCodeQLVariantAnalyses_QueryPacksUseObjectStore(t *testing.T) {
	controller := seedTestRepo(t, "codeql-va-object-controller", true)
	withDB := seedTestRepo(t, "codeql-va-object-db", false)
	databaseCommit := putRepoFile(t, withDB.FullName, "main.go", "package main\n", "add source")
	uploadCodeQLDatabase(t, withDB.FullName, "go", databaseCommit, testCodeQLDatabaseBundle(t, "go", "db"))

	objectFS, objectStore := newObjectByteStoreForTest(t)
	oldStore := testServer.store.ObjectByteStore
	testServer.store.ObjectByteStore = objectStore
	t.Cleanup(func() {
		testServer.store.ObjectByteStore = oldStore
	})

	queryPackBytes := testCodeQLQueryPack(t)
	resp := ghPost(t, "/api/v3/repos/"+controller.FullName+"/code-scanning/codeql/variant-analyses", defaultToken, map[string]interface{}{
		"language":     "go",
		"query_pack":   base64.StdEncoding.EncodeToString(queryPackBytes),
		"repositories": []string{withDB.FullName},
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create object-backed variant analysis: %d body=%s", resp.StatusCode, b)
	}
	created := decodeJSON(t, resp)
	vaID := int(created["id"].(float64))
	queryPackURL := created["query_pack_url"].(string)
	anonymous, err := http.Get(queryPackURL)
	if err != nil {
		t.Fatal(err)
	}
	anonymous.Body.Close()
	if anonymous.StatusCode != http.StatusNotFound {
		t.Fatalf("anonymous private query-pack download = %d, want 404", anonymous.StatusCode)
	}
	request, err := http.NewRequest(http.MethodGet, queryPackURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "token "+defaultToken)
	authorized, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	authorizedBytes, _ := io.ReadAll(authorized.Body)
	authorized.Body.Close()
	if authorized.StatusCode != http.StatusOK || !bytes.Equal(authorizedBytes, queryPackBytes) {
		t.Fatalf("authorized private query-pack download = %d bytes=%d", authorized.StatusCode, len(authorizedBytes))
	}
	key := codeQLVariantAnalysisQueryPackDataKey(vaID)
	if got := readS3TestFile(t, objectFS, key); !bytes.Equal(got, queryPackBytes) {
		t.Fatalf("CodeQL variant-analysis query-pack object bytes = %q, want %q", got, queryPackBytes)
	}

	va := testServer.store.GetCodeQLVariantAnalysis(controller.FullName, vaID)
	if va == nil {
		t.Fatal("variant analysis missing after create")
	}
	if va.QueryPack != "" {
		t.Fatalf("metadata retained base64 query-pack bytes: %q", va.QueryPack)
	}
	if va.StoragePath != key {
		t.Fatalf("storage path = %q, want %q", va.StoragePath, key)
	}

	packURL, _ := created["query_pack_url"].(string)
	req, err := http.NewRequest("GET", packURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+defaultToken)
	packResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(packResp.Body)
	packResp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if packResp.StatusCode != http.StatusOK {
		t.Fatalf("download object-backed query pack: %d body=%s", packResp.StatusCode, raw)
	}
	if !bytes.Equal(raw, queryPackBytes) {
		t.Fatalf("downloaded object-backed query pack = %q, want %q", raw, queryPackBytes)
	}

	mustStatus(t, ghDelete(t, "/api/v3/repos/"+controller.FullName, defaultToken), http.StatusNoContent, "delete controller repository")
	if _, err := objectFS.Open(key); err == nil {
		t.Fatalf("CodeQL variant-analysis query-pack object %s survived controller repository deletion", key)
	}
}

func testCodeQLQueryPack(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	files := []struct {
		name string
		body string
	}{
		{name: "qlpack.yml", body: "name: bleephub/test-pack\nversion: 1.0.0\nlibraryPathDependencies: []\n"},
		{name: "queries/example.ql", body: "select \"ok\"\n"},
	}
	for _, file := range files {
		content := []byte(file.body)
		if err := tw.WriteHeader(&tar.Header{Name: file.name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatalf("write %s header: %v", file.name, err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("write %s content: %v", file.name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func TestCodeQLVariantAnalyses_Validation(t *testing.T) {
	controller := seedTestRepo(t, "codeql-va-valid", false)
	basePath := "/api/v3/repos/" + controller.FullName + "/code-scanning/codeql/variant-analyses"
	queryPack := base64.StdEncoding.EncodeToString([]byte("pack"))

	// Invalid language.
	mustStatus(t, ghPost(t, basePath, defaultToken, map[string]interface{}{
		"language": "cobol", "query_pack": queryPack, "repositories": []string{"a/b"},
	}), 422, "invalid language")

	// Missing query pack.
	mustStatus(t, ghPost(t, basePath, defaultToken, map[string]interface{}{
		"language": "go", "repositories": []string{"a/b"},
	}), 422, "missing query pack")

	// More than one repository selector.
	mustStatus(t, ghPost(t, basePath, defaultToken, map[string]interface{}{
		"language": "go", "query_pack": queryPack,
		"repositories":      []string{"a/b"},
		"repository_owners": []string{"admin"},
	}), 422, "two repository selectors")

	// No repository selector at all.
	mustStatus(t, ghPost(t, basePath, defaultToken, map[string]interface{}{
		"language": "go", "query_pack": queryPack,
	}), 422, "no repository selector")

	// All targets unresolvable → the analysis fails with no_repos_queried.
	resp := ghPost(t, basePath, defaultToken, map[string]interface{}{
		"language": "go", "query_pack": queryPack,
		"repositories": []string{"admin/never-existed"},
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create failing variant analysis: %d body=%s", resp.StatusCode, b)
	}
	created := decodeJSON(t, resp)
	if created["status"] != "failed" || created["failure_reason"] != "no_repos_queried" {
		t.Fatalf("variant analysis = %v/%v, want failed/no_repos_queried", created["status"], created["failure_reason"])
	}
}
