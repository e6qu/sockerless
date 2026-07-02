package bleephub

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Releases API parity — release CRUD + asset upload/download + tag-based
// lookup against /repos/{}/releases, matching the GitHub-compatible shape.

func TestReleases_FullLifecycle(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHReleasesRoutes()
	s.registerGHReactionsRoutes()

	user := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(user, "rel-repo", "", false)
	_ = repo

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

func TestReleases_GenerateNotes(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHReleasesRoutes()

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

func TestReleases_AssetLifecycle(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHReleasesRoutes()

	user := s.store.UsersByLogin["admin"]
	_ = s.store.CreateRepo(user, "asset-repo", "", false)

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
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		_ = mw.WriteField("name", "foo.tar.gz")
		_ = mw.WriteField("label", "archive")
		_ = mw.WriteField("content_type", "application/gzip")
		fw, _ := mw.CreateFormFile("file", "foo.tar.gz")
		_, _ = fw.Write([]byte("hello world"))
		_ = mw.Close()

		req := httptest.NewRequest("POST", "/api/v3/repos/admin/asset-repo/releases/"+itoa(relID)+"/assets", &body)
		req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
		req.Header.Set("Content-Type", mw.FormDataContentType())
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

func TestReleases_ReleaseReactionsLifecycle(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHReleasesRoutes()
	s.registerGHReactionsRoutes()

	user := s.store.UsersByLogin["admin"]
	_ = s.store.CreateRepo(user, "relrxn-repo", "", false)

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
