package bleephub

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// seedCodeQLDatabase uploads a CodeQL database through the internal
// seeding endpoint (real GitHub receives databases from CodeQL analysis
// uploads) and returns the created entity.
func seedCodeQLDatabase(t *testing.T, repoFullName, language, commitOID string, content []byte) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"language":   language,
		"content":    base64.StdEncoding.EncodeToString(content),
		"commit_oid": commitOID,
	})
	resp, err := authedPost("/internal/repos/"+repoFullName+"/code-scanning/codeql/databases", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("seed CodeQL database: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("seed CodeQL database: %d body=%s", resp.StatusCode, b)
	}
	var created map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode seeded database: %v", err)
	}
	return created
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
	dbBytes := []byte("codeql-database-archive-bytes")
	commitOID := "1927de39fefa25a9d0e64e3f540ff824a72f538c"

	created := seedCodeQLDatabase(t, repo.FullName, "go", commitOID, dbBytes)
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
	if db["name"] != "database.zip" || db["language"] != "go" || db["content_type"] != "application/zip" {
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

	dbBytes := []byte("object-backed-codeql-database")
	created := seedCodeQLDatabase(t, repo.FullName, "go", "1927de39fefa25a9d0e64e3f540ff824a72f538c", dbBytes)
	dbID := int(created["id"].(float64))
	if got := string(readS3TestFile(t, objectFS, codeQLDatabaseDataKey(dbID))); got != string(dbBytes) {
		t.Fatalf("CodeQL database object bytes = %q, want %q", got, dbBytes)
	}
	db := testServer.store.GetCodeQLDatabase(repo.FullName, "go")
	if db == nil {
		t.Fatal("CodeQL database missing after seed")
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
	if _, err := objectFS.Open(codeQLDatabaseDataKey(dbID)); err == nil {
		t.Fatalf("CodeQL database object %s survived database deletion", codeQLDatabaseDataKey(dbID))
	}
}

// --- CodeQL variant analyses ---

func TestCodeQLVariantAnalyses_CreateAndReadBack(t *testing.T) {
	controller := seedTestRepo(t, "codeql-va-controller", false)
	withDB := seedTestRepo(t, "codeql-va-with-db", false)
	withoutDB := seedTestRepo(t, "codeql-va-no-db", false)

	seedCodeQLDatabase(t, withDB.FullName, "go", "af5626b4a114abcb82d63db7c8082c3c4756e51b", []byte("db"))

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
	if task["database_commit_sha"] != "af5626b4a114abcb82d63db7c8082c3c4756e51b" {
		t.Fatalf("database_commit_sha = %v", task["database_commit_sha"])
	}

	// A repository that was not scanned is a 404 on the task endpoint.
	mustStatus(t, ghGet(t, fmt.Sprintf("%s/%d/repos/%s", basePath, vaID, withoutDB.FullName), defaultToken), 404, "task for skipped repo")

	// Unknown analysis id.
	mustStatus(t, ghGet(t, basePath+"/99999", defaultToken), 404, "unknown variant analysis")
}

func TestCodeQLVariantAnalyses_QueryPacksUseObjectStore(t *testing.T) {
	controller := seedTestRepo(t, "codeql-va-object-controller", false)
	withDB := seedTestRepo(t, "codeql-va-object-db", false)
	seedCodeQLDatabase(t, withDB.FullName, "go", "23a401530b4b24149f5a03c44f1e622b773e5af7", []byte("db"))

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
