package bleephub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
)

// Releases API parity — release CRUD + asset upload/download + tag-based
// lookup against /repos/{}/releases, matching the GitHub-compatible shape.

func initializeReleaseTestRepo(t *testing.T, s *Server, repo *Repo, user *User) {
	t.Helper()
	stor, _ := s.store.GitStorageForRepoID(repo.ID)
	if _, err := initRepoWithFiles(stor, repo.DefaultBranch, "Initial commit", map[string]string{
		"README.md": "# " + repo.Name,
	}, repoSignature(user.Login, "bleephub@local")); err != nil {
		t.Fatalf("initialize release repository: %v", err)
	}
}

func TestReleases_FullLifecycle(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHReleasesRoutes()
	s.registerGHReactionsRoutes()

	user := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(user, "rel-repo", "", false)
	initializeReleaseTestRepo(t, s, repo, user)

	do := func(method, path string, body []byte) *httptest.ResponseRecorder {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, bytes.NewReader(body))
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
		return w
	}

	// Create release (gh release create equivalent).
	create, _ := json.Marshal(map[string]any{
		"tag_name":   "v1.0.0",
		"name":       "Release 1.0",
		"body":       "first release",
		"draft":      false,
		"prerelease": false,
	})
	w := do("POST", "/api/v3/repos/admin/rel-repo/releases", create)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d body=%s", w.Code, w.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	relID := int(created["id"].(float64))
	if created["tag_name"] != "v1.0.0" {
		t.Errorf("tag = %v", created["tag_name"])
	}
	if created["html_url"] == nil || created["tarball_url"] == nil {
		t.Errorf("missing HATEOAS urls")
	}

	// Missing tag_name → 422
	bad, _ := json.Marshal(map[string]any{"name": "x"})
	w = do("POST", "/api/v3/repos/admin/rel-repo/releases", bad)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("missing tag → %d", w.Code)
	}

	// Get by id
	w = do("GET", "/api/v3/repos/admin/rel-repo/releases/"+itoa(relID), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get by id: %d", w.Code)
	}

	// Get by tag
	w = do("GET", "/api/v3/repos/admin/rel-repo/releases/tags/v1.0.0", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get by tag: %d body=%s", w.Code, w.Body.String())
	}

	// Latest
	w = do("GET", "/api/v3/repos/admin/rel-repo/releases/latest", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("latest: %d", w.Code)
	}

	// Update — flip to draft
	patch, _ := json.Marshal(map[string]any{"draft": true, "body": "rewritten"})
	w = do("PATCH", "/api/v3/repos/admin/rel-repo/releases/"+itoa(relID), patch)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: %d", w.Code)
	}

	// /releases/latest should now return 404 (only non-draft is non-existent).
	w = do("GET", "/api/v3/repos/admin/rel-repo/releases/latest", nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("latest after draft: %d", w.Code)
	}

	// React to the release.
	reactBody, _ := json.Marshal(map[string]string{"content": "rocket"})
	w = do("POST", "/api/v3/repos/admin/rel-repo/releases/"+itoa(relID)+"/reactions", reactBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("release reaction: %d body=%s", w.Code, w.Body.String())
	}

	// Delete release
	w = do("DELETE", "/api/v3/repos/admin/rel-repo/releases/"+itoa(relID), nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", w.Code)
	}

	// Subsequent GET → 404
	w = do("GET", "/api/v3/repos/admin/rel-repo/releases/"+itoa(relID), nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("get after delete: %d", w.Code)
	}
}

func TestReleases_CreateUsesRealGitTagAndRejectsUnresolvedTarget(t *testing.T) {
	repoPath := "/api/v3/repos/admin/release-git-source"
	resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "release-git-source", "auto_init": true,
	})
	resp.Body.Close()
	repo := testServer.store.GetRepo("admin", "release-git-source")
	if repo == nil {
		t.Fatal("repository not created")
	}
	stor, _ := testServer.store.GitStorageForRepoID(repo.ID)
	mainRef, err := stor.Reference(plumbing.NewBranchReferenceName("main"))
	if err != nil {
		t.Fatalf("resolve main: %v", err)
	}

	create := ghPost(t, repoPath+"/releases", defaultToken, map[string]interface{}{
		"tag_name": "v1.0.0", "target_commitish": "main", "name": "Real source release",
	})
	if create.StatusCode != http.StatusCreated {
		create.Body.Close()
		t.Fatalf("create status = %d", create.StatusCode)
	}
	create.Body.Close()
	tagRef, err := stor.Reference(plumbing.NewTagReferenceName("v1.0.0"))
	if err != nil {
		t.Fatalf("release did not create git tag: %v", err)
	}
	if tagRef.Hash() != mainRef.Hash() {
		t.Fatalf("tag hash = %s, want main %s", tagRef.Hash(), mainRef.Hash())
	}

	bad := ghPost(t, repoPath+"/releases", defaultToken, map[string]interface{}{
		"tag_name": "v2.0.0", "target_commitish": "missing-branch",
	})
	if bad.StatusCode != http.StatusUnprocessableEntity {
		bad.Body.Close()
		t.Fatalf("unresolved target status = %d", bad.StatusCode)
	}
	bad.Body.Close()
	if _, err := stor.Reference(plumbing.NewTagReferenceName("v2.0.0")); err == nil {
		t.Fatal("unresolved release target created a tag")
	}
}

func TestReleases_UpdateIsRepositoryScoped(t *testing.T) {
	for _, name := range []string{"release-scope-a", "release-scope-b"} {
		resp := ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
			"name": name, "auto_init": true,
		})
		resp.Body.Close()
	}
	created := ghPost(t, "/api/v3/repos/admin/release-scope-a/releases", defaultToken, map[string]interface{}{
		"tag_name": "v1.0.0", "name": "Repository A",
	})
	if created.StatusCode != http.StatusCreated {
		created.Body.Close()
		t.Fatalf("create status = %d", created.StatusCode)
	}
	release := decodeJSON(t, created)
	id := int(release["id"].(float64))

	patch, _ := json.Marshal(map[string]interface{}{"name": "Cross-repository mutation"})
	wrongRepo := doAuthReq(testServer, http.MethodPatch, "/api/v3/repos/admin/release-scope-b/releases/"+itoa(id), patch)
	if wrongRepo.Code != http.StatusNotFound {
		t.Fatalf("cross-repository update status = %d body=%s", wrongRepo.Code, wrongRepo.Body.String())
	}
	stored := testServer.store.Releases.Get(id)
	if stored.Name != "Repository A" {
		t.Fatalf("cross-repository update mutated release: %q", stored.Name)
	}
}

func TestReleases_GenerateNotes(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHReleasesRoutes()
	user := s.store.UsersByLogin["admin"]
	_ = s.store.CreateRepo(user, "r", "", false)

	body, _ := json.Marshal(map[string]string{
		"tag_name":          "v2.0.0",
		"previous_tag_name": "v1.0.0",
	})
	req := httptest.NewRequest("POST", "/api/v3/repos/admin/r/releases/generate-notes", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("generate-notes: %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["name"] != "v2.0.0" {
		t.Errorf("name = %v", resp["name"])
	}
	if resp["body"] == nil {
		t.Errorf("body missing")
	}
}

func TestReleases_GenerateNotesMergedPullRequests(t *testing.T) {
	repoPath := "/api/v3/repos/admin/rel-notes-prs"
	ghPost(t, "/api/v3/user/repos", defaultToken, map[string]interface{}{
		"name": "rel-notes-prs", "auto_init": true,
	}).Body.Close()
	repo := testServer.store.GetRepo("admin", "rel-notes-prs")
	if repo == nil {
		t.Fatal("repo not created")
	}
	seedPullRequestBranches(t, testServer, repo, "feature")

	mainRef := ghGet(t, repoPath+"/git/refs/heads/main", defaultToken)
	mainData := decodeJSON(t, mainRef)
	mainObj, _ := mainData["object"].(map[string]interface{})
	baseSHA, _ := mainObj["sha"].(string)
	if baseSHA == "" {
		t.Fatalf("main ref missing sha: %v", mainData)
	}
	ghPost(t, repoPath+"/git/refs", defaultToken, map[string]interface{}{
		"ref": "refs/tags/v1.0.0", "sha": baseSHA,
	}).Body.Close()

	ghPost(t, repoPath+"/pulls", defaultToken, map[string]interface{}{
		"title": "Ship release notes", "head": "feature", "base": "main",
	}).Body.Close()
	mergeResp := ghPut(t, repoPath+"/pulls/1/merge", defaultToken, map[string]interface{}{})
	if mergeResp.StatusCode != http.StatusOK {
		mergeResp.Body.Close()
		t.Fatalf("merge status = %d", mergeResp.StatusCode)
	}
	mergeData := decodeJSON(t, mergeResp)
	mergeSHA, _ := mergeData["sha"].(string)
	if mergeSHA == "" {
		t.Fatalf("merge returned no sha: %v", mergeData)
	}
	ghPost(t, repoPath+"/git/refs", defaultToken, map[string]interface{}{
		"ref": "refs/tags/v2.0.0", "sha": mergeSHA,
	}).Body.Close()

	body, _ := json.Marshal(map[string]string{
		"tag_name":          "v2.0.0",
		"previous_tag_name": "v1.0.0",
	})
	resp := ghPost(t, repoPath+"/releases/generate-notes", defaultToken, json.RawMessage(body))
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("generate-notes status = %d", resp.StatusCode)
	}
	data := decodeJSON(t, resp)
	notes, _ := data["body"].(string)
	if !strings.Contains(notes, "* Ship release notes by @admin in admin/rel-notes-prs/pull/1") {
		t.Fatalf("notes missing merged PR bullet:\n%s", notes)
	}
	if !strings.Contains(notes, "**Full Changelog**: v1.0.0...v2.0.0") {
		t.Fatalf("notes missing full changelog:\n%s", notes)
	}
}

func TestReleases_AssetLifecycle(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHReleasesRoutes()

	user := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(user, "asset-repo", "", false)
	initializeReleaseTestRepo(t, s, repo, user)

	// Create a release to attach assets to.
	createBody, _ := json.Marshal(map[string]any{
		"tag_name": "v1.0.0",
		"name":     "Release 1.0",
		"body":     "first release",
	})
	w := doAuthReq(s, "POST", "/api/v3/repos/admin/asset-repo/releases", createBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create release: %d body=%s", w.Code, w.Body.String())
	}
	var rel map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &rel)
	relID := int(rel["id"].(float64))

	upload := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/uploads/repos/admin/asset-repo/releases/"+itoa(relID)+"/assets?name=foo.tar.gz&label=archive", strings.NewReader("hello world"))
		req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
		req.Header.Set("Content-Type", "application/gzip")
		rec := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(rec, req)
		return rec
	}

	w = upload()
	if w.Code != http.StatusCreated {
		t.Fatalf("upload asset: %d body=%s", w.Code, w.Body.String())
	}
	var asset map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &asset)
	assetID := int(asset["id"].(float64))
	if asset["name"] != "foo.tar.gz" {
		t.Errorf("asset name = %v", asset["name"])
	}
	if asset["content_type"] != "application/gzip" {
		t.Errorf("asset content_type = %v", asset["content_type"])
	}
	if asset["size"] != float64(len("hello world")) {
		t.Errorf("asset size = %v", asset["size"])
	}

	missingName := httptest.NewRequest("POST", "/api/uploads/repos/admin/asset-repo/releases/"+itoa(relID)+"/assets", strings.NewReader("ignored"))
	missingName.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	missingNameRec := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(missingNameRec, missingName)
	if missingNameRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing-name upload: %d body=%s", missingNameRec.Code, missingNameRec.Body.String())
	}

	// List assets for the release.
	w = doAuthReq(s, "GET", "/api/v3/repos/admin/asset-repo/releases/"+itoa(relID)+"/assets", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list assets: %d body=%s", w.Code, w.Body.String())
	}
	var list []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("asset list len = %d", len(list))
	}

	// Get asset metadata.
	w = doAuthReq(s, "GET", "/api/v3/repos/admin/asset-repo/releases/assets/"+itoa(assetID), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get asset metadata: %d body=%s", w.Code, w.Body.String())
	}

	// Download asset bytes.
	req := httptest.NewRequest("GET", "/api/v3/repos/admin/asset-repo/releases/assets/"+itoa(assetID), nil)
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	req.Header.Set("Accept", "application/octet-stream")
	rec := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("download asset: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "gzip") {
		t.Errorf("download content-type = %v", rec.Header().Get("Content-Type"))
	}
	if rec.Body.String() != "hello world" {
		t.Errorf("download body = %q", rec.Body.String())
	}

	// Update label/name.
	patch, _ := json.Marshal(map[string]any{"name": "bar.tar.gz", "label": "updated"})
	w = doAuthReq(s, "PATCH", "/api/v3/repos/admin/asset-repo/releases/assets/"+itoa(assetID), patch)
	if w.Code != http.StatusOK {
		t.Fatalf("patch asset: %d body=%s", w.Code, w.Body.String())
	}
	var updated map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &updated)
	if updated["name"] != "bar.tar.gz" || updated["label"] != "updated" {
		t.Errorf("updated asset = %v", updated)
	}

	// Delete asset.
	w = doAuthReq(s, "DELETE", "/api/v3/repos/admin/asset-repo/releases/assets/"+itoa(assetID), nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete asset: %d", w.Code)
	}
	w = doAuthReq(s, "GET", "/api/v3/repos/admin/asset-repo/releases/assets/"+itoa(assetID), nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("get after delete: %d", w.Code)
	}
}

func TestReleases_AssetBytesUseObjectStore(t *testing.T) {
	fs := newS3FSForTest(t)
	objectFS := &s3FS{client: fs.client, bucket: fs.bucket, prefix: "objects"}
	s := newTestServer()
	s.store.ObjectByteStore = &s3ActionsByteStore{fs: objectFS}
	s.store.Releases.byteStore = s.store.ObjectByteStore
	s.registerGHReleasesRoutes()

	user := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(user, "object-release-repo", "", false)
	initializeReleaseTestRepo(t, s, repo, user)
	createBody, _ := json.Marshal(map[string]any{
		"tag_name": "v1.0.0",
		"name":     "Release 1.0",
	})
	w := doAuthReq(s, "POST", "/api/v3/repos/admin/object-release-repo/releases", createBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create release: %d body=%s", w.Code, w.Body.String())
	}
	var rel map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &rel)
	relID := int(rel["id"].(float64))

	req := httptest.NewRequest("POST", "/api/uploads/repos/admin/object-release-repo/releases/"+itoa(relID)+"/assets?name=object.tar.gz", strings.NewReader("release object bytes"))
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	req.Header.Set("Content-Type", "application/gzip")
	rec := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload asset: %d body=%s", rec.Code, rec.Body.String())
	}
	var asset map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &asset)
	assetID := int(asset["id"].(float64))

	got := readS3TestFile(t, objectFS, releaseAssetDataKey(assetID))
	if string(got) != "release object bytes" {
		t.Fatalf("release asset object bytes = %q", string(got))
	}

	download := httptest.NewRequest("GET", "/api/v3/repos/admin/object-release-repo/releases/assets/"+itoa(assetID), nil)
	download.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	download.Header.Set("Accept", "application/octet-stream")
	out := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(out, download)
	if out.Code != http.StatusOK || out.Body.String() != "release object bytes" {
		t.Fatalf("download asset: status=%d body=%q", out.Code, out.Body.String())
	}
}

func TestReleases_ReleaseReactionsLifecycle(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHReleasesRoutes()
	s.registerGHReactionsRoutes()

	user := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(user, "relrxn-repo", "", false)
	initializeReleaseTestRepo(t, s, repo, user)

	createBody, _ := json.Marshal(map[string]any{"tag_name": "v1.0.0", "name": "R"})
	w := doAuthReq(s, "POST", "/api/v3/repos/admin/relrxn-repo/releases", createBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create release: %d body=%s", w.Code, w.Body.String())
	}
	var rel map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &rel)
	relID := int(rel["id"].(float64))

	body, _ := json.Marshal(map[string]string{"content": "heart"})
	w = doAuthReq(s, "POST", "/api/v3/repos/admin/relrxn-repo/releases/"+itoa(relID)+"/reactions", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create reaction: %d body=%s", w.Code, w.Body.String())
	}
	var first map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &first)
	if first["content"] != "heart" {
		t.Errorf("content = %v", first["content"])
	}

	// Idempotent repeat.
	w = doAuthReq(s, "POST", "/api/v3/repos/admin/relrxn-repo/releases/"+itoa(relID)+"/reactions", body)
	if w.Code != http.StatusOK {
		t.Fatalf("repeat reaction: %d body=%s", w.Code, w.Body.String())
	}
	var second map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &second)
	if second["id"] != first["id"] {
		t.Errorf("repeat returned different id: %v vs %v", second["id"], first["id"])
	}

	// List.
	w = doAuthReq(s, "GET", "/api/v3/repos/admin/relrxn-repo/releases/"+itoa(relID)+"/reactions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list reactions: %d", w.Code)
	}
	var list []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Errorf("list len = %d", len(list))
	}

	// Delete.
	rxnID := int(first["id"].(float64))
	w = doAuthReq(s, "DELETE", "/api/v3/repos/admin/relrxn-repo/releases/"+itoa(relID)+"/reactions/"+itoa(rxnID), nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete reaction: %d", w.Code)
	}
	w = doAuthReq(s, "DELETE", "/api/v3/repos/admin/relrxn-repo/releases/"+itoa(relID)+"/reactions/"+itoa(rxnID), nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("delete missing reaction: %d", w.Code)
	}
}

func doAuthReq(s *Server, method, path string, body []byte) *httptest.ResponseRecorder {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	return w
}
