package bleephub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// Commit Comments REST API — create, list, get, update, delete.

func TestCommitComments_CRUD(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHCommitCommentsRoutes()

	user := s.store.UsersByLogin["admin"]
	_ = s.store.CreateRepo(user, "cc-repo", "", false)
	sha := "deadbeef"

	create := func(body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/v3/repos/admin/cc-repo/commits/"+sha+"/comments", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
		return w
	}

	// Create a commit comment.
	b1, _ := json.Marshal(map[string]any{
		"body":     "nice commit",
		"path":     "main.go",
		"position": 7,
		"line":     7,
	})
	w := create(b1)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d body=%s", w.Code, w.Body.String())
	}
	var c1 map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &c1)
	id1 := int(c1["id"].(float64))
	if c1["body"] != "nice commit" || c1["commit_id"] != sha {
		t.Errorf("created shape: %v", c1)
	}

	// Create another comment on a different commit.
	sha2 := "cafebabe"
	b2, _ := json.Marshal(map[string]string{"body": "another comment"})
	req := httptest.NewRequest("POST", "/api/v3/repos/admin/cc-repo/commits/"+sha2+"/comments", bytes.NewReader(b2))
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w = httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create2: %d body=%s", w.Code, w.Body.String())
	}
	var c2 map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &c2)
	id2 := int(c2["id"].(float64))

	// List comments for the first commit.
	req = httptest.NewRequest("GET", "/api/v3/repos/admin/cc-repo/commits/"+sha+"/comments", nil)
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w = httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list commit comments: %d body=%s", w.Code, w.Body.String())
	}
	var commitList []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &commitList)
	if len(commitList) != 1 {
		t.Errorf("commit list len = %d, want 1", len(commitList))
	}

	// List all repo commit comments.
	req = httptest.NewRequest("GET", "/api/v3/repos/admin/cc-repo/comments", nil)
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w = httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list repo comments: %d body=%s", w.Code, w.Body.String())
	}
	var repoList []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &repoList)
	if len(repoList) != 2 {
		t.Errorf("repo list len = %d, want 2", len(repoList))
	}

	// Get single comment.
	req = httptest.NewRequest("GET", "/api/v3/repos/admin/cc-repo/comments/"+strconv.Itoa(id1), nil)
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w = httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get comment: %d body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["body"] != "nice commit" {
		t.Errorf("get body = %v", got["body"])
	}

	// Update comment.
	patch, _ := json.Marshal(map[string]string{"body": "updated comment"})
	req = httptest.NewRequest("PATCH", "/api/v3/repos/admin/cc-repo/comments/"+strconv.Itoa(id1), bytes.NewReader(patch))
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w = httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch comment: %d body=%s", w.Code, w.Body.String())
	}
	var patched map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &patched)
	if patched["body"] != "updated comment" {
		t.Errorf("patched body = %v", patched["body"])
	}

	// Delete the second comment.
	req = httptest.NewRequest("DELETE", "/api/v3/repos/admin/cc-repo/comments/"+strconv.Itoa(id2), nil)
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w = httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete comment: %d body=%s", w.Code, w.Body.String())
	}

	// Repo list now has one.
	req = httptest.NewRequest("GET", "/api/v3/repos/admin/cc-repo/comments", nil)
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w = httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	_ = json.Unmarshal(w.Body.Bytes(), &repoList)
	if len(repoList) != 1 {
		t.Errorf("repo list after delete len = %d, want 1", len(repoList))
	}
}

func TestCommitComments_MissingBody422(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHCommitCommentsRoutes()

	user := s.store.UsersByLogin["admin"]
	_ = s.store.CreateRepo(user, "cc-repo2", "", false)

	body, _ := json.Marshal(map[string]string{"path": "x.go"})
	req := httptest.NewRequest("POST", "/api/v3/repos/admin/cc-repo2/commits/abc/comments", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("missing body: %d", w.Code)
	}
}
