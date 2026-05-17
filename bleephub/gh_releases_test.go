package bleephub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		req.Header.Set("Authorization", "Bearer ghp_0000000000000000000000000000000000000000")
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
	req.Header.Set("Authorization", "Bearer ghp_0000000000000000000000000000000000000000")
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
