package bleephub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// PR review comments — root + reply CRUD against
// /repos/{}/pulls/{}/comments + thread navigation (in_reply_to_id).

func TestPRReviewComments_RootAndReply(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHPRCommentsRoutes()

	user := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(user, "rc-repo", "", false)
	seedPullRequestBranches(t, s, repo, "feat")
	pr := s.store.CreatePullRequest(repo.ID, user.ID, "title", "body", "feat", "main", false, nil, nil, 0)

	create := func(path string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
		return w
	}

	// Root inline comment on a specific line.
	rootBody, _ := json.Marshal(map[string]any{
		"body":      "consider refactoring this",
		"path":      "src/foo.go",
		"line":      42,
		"side":      "RIGHT",
		"commit_id": "abc123",
	})
	w := create("/api/v3/repos/admin/rc-repo/pulls/"+itoa(pr.Number)+"/comments", rootBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("create root: %d body=%s", w.Code, w.Body.String())
	}
	var root map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &root)
	rootID := int(root["id"].(float64))
	if root["path"] != "src/foo.go" || root["line"].(float64) != 42 {
		t.Errorf("root shape: %v", root)
	}

	// Reply via /comments/{id}/replies endpoint.
	replyBody, _ := json.Marshal(map[string]string{"body": "I agree"})
	w = create("/api/v3/repos/admin/rc-repo/pulls/"+itoa(pr.Number)+"/comments/"+itoa(rootID)+"/replies", replyBody)
	if w.Code != http.StatusCreated {
		t.Fatalf("reply: %d body=%s", w.Code, w.Body.String())
	}
	var reply map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &reply)
	if reply["in_reply_to_id"].(float64) != float64(rootID) {
		t.Errorf("in_reply_to: %v", reply["in_reply_to_id"])
	}
	if reply["path"] != "src/foo.go" {
		t.Errorf("reply path: %v", reply["path"])
	}

	// Reply via POST /pulls/{n}/comments with in_reply_to.
	inlineReply, _ := json.Marshal(map[string]any{"body": "another reply", "in_reply_to": rootID})
	w = create("/api/v3/repos/admin/rc-repo/pulls/"+itoa(pr.Number)+"/comments", inlineReply)
	if w.Code != http.StatusCreated {
		t.Fatalf("inline reply: %d body=%s", w.Code, w.Body.String())
	}

	// List comments → 3 (root + 2 replies).
	req := httptest.NewRequest("GET", "/api/v3/repos/admin/rc-repo/pulls/"+itoa(pr.Number)+"/comments", nil)
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w = httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	var list []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 3 {
		t.Errorf("list len = %d, want 3", len(list))
	}

	// Get single comment by id via /pulls/comments/{id}.
	req = httptest.NewRequest("GET", "/api/v3/repos/admin/rc-repo/pulls/comments/"+itoa(rootID), nil)
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w = httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get by id: %d body=%s", w.Code, w.Body.String())
	}

	// PATCH body
	patch, _ := json.Marshal(map[string]string{"body": "EDITED"})
	req = httptest.NewRequest("PATCH", "/api/v3/repos/admin/rc-repo/pulls/comments/"+itoa(rootID), bytes.NewReader(patch))
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w = httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: %d body=%s", w.Code, w.Body.String())
	}
	var patched map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &patched)
	if patched["body"] != "EDITED" {
		t.Errorf("body after patch: %v", patched["body"])
	}

	// Review threads — 1 thread with 3 comments (root + 2 replies share a thread).
	// Public clients read and mutate this state through GitHub GraphQL.
	threads := s.store.PRReviewComments.ListThreads(pr.ID)
	if len(threads) != 1 {
		t.Fatalf("threads len = %d", len(threads))
	}
	if threads[0].IsResolved {
		t.Errorf("thread resolved = %v", threads[0].IsResolved)
	}
}

func TestPRReviewComments_MissingBody422(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHPRCommentsRoutes()

	user := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(user, "rc2", "", false)
	seedPullRequestBranches(t, s, repo, "f", "m")
	pr := s.store.CreatePullRequest(repo.ID, user.ID, "t", "b", "f", "m", false, nil, nil, 0)

	bad, _ := json.Marshal(map[string]any{"path": "x.go"})
	req := httptest.NewRequest("POST", "/api/v3/repos/admin/rc2/pulls/"+itoa(pr.Number)+"/comments", bytes.NewReader(bad))
	req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("missing body: %d", w.Code)
	}
}
