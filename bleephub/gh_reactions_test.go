package bleephub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Reactions API parity — issue / PR / commit / comment reaction CRUD against
// the GitHub-compatible /repos/{}/issues/{}/reactions surface.

func TestReactions_IssueLifecycle(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHRepoRoutes()
	s.registerGHIssueRoutes()
	s.registerGHReactionsRoutes()

	user := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(user, "rxn-repo", "", false)
	_ = repo
	// Create issue via store directly (skip route auth churn).
	issue := s.store.CreateIssue(repo.ID, user.ID, "test issue", "", nil, nil, 0)

	addPath := func(content string) []byte {
		body, _ := json.Marshal(map[string]string{"content": content})
		return body
	}
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

	// POST +1 → 201
	w := do("POST", "/api/v3/repos/admin/rxn-repo/issues/"+itoa(issue.ID)+"/reactions", addPath("+1"))
	if w.Code != http.StatusCreated {
		t.Fatalf("first POST +1: status %d body %s", w.Code, w.Body.String())
	}
	var first map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &first)
	if first["content"] != "+1" {
		t.Errorf("content = %v, want +1", first["content"])
	}

	// POST same again → 200 (idempotent, same id)
	w = do("POST", "/api/v3/repos/admin/rxn-repo/issues/"+itoa(issue.ID)+"/reactions", addPath("+1"))
	if w.Code != http.StatusOK {
		t.Fatalf("second POST +1: status %d body %s", w.Code, w.Body.String())
	}
	var second map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &second)
	if second["id"] != first["id"] {
		t.Errorf("second POST returned different id: %v vs %v", second["id"], first["id"])
	}

	// POST heart → another reaction, new id
	w = do("POST", "/api/v3/repos/admin/rxn-repo/issues/"+itoa(issue.ID)+"/reactions", addPath("heart"))
	if w.Code != http.StatusCreated {
		t.Fatalf("POST heart: status %d", w.Code)
	}

	// POST invalid → 422
	w = do("POST", "/api/v3/repos/admin/rxn-repo/issues/"+itoa(issue.ID)+"/reactions", addPath("smile"))
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("invalid content: status %d, want 422", w.Code)
	}

	// GET list → 2 reactions
	w = do("GET", "/api/v3/repos/admin/rxn-repo/issues/"+itoa(issue.ID)+"/reactions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET list: status %d", w.Code)
	}
	var list []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 2 {
		t.Errorf("list len = %d, want 2", len(list))
	}

	// GET filter by content=heart → 1
	w = do("GET", "/api/v3/repos/admin/rxn-repo/issues/"+itoa(issue.ID)+"/reactions?content=heart", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET filter: %d", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 || list[0]["content"] != "heart" {
		t.Errorf("filtered list = %v", list)
	}

	// DELETE the +1 reaction → 204
	firstID := int(first["id"].(float64))
	w = do("DELETE", "/api/v3/repos/admin/rxn-repo/issues/"+itoa(issue.ID)+"/reactions/"+itoa(firstID), nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("DELETE: status %d", w.Code)
	}

	// DELETE again → 404
	w = do("DELETE", "/api/v3/repos/admin/rxn-repo/issues/"+itoa(issue.ID)+"/reactions/"+itoa(firstID), nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("DELETE missing: status %d, want 404", w.Code)
	}
}

func TestReactions_AllParentTypes(t *testing.T) {
	s := newTestServer()
	s.store.SeedDefaultUser()
	s.registerGHReactionsRoutes()
	s.registerGHReleasesRoutes() // release-reactions live under the release dispatcher

	// Issue reactions resolve through the repository to a real issue; the
	// other parent types are keyed by globally-unique comment/release ids.
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "r", "", false)
	issue := s.store.CreateIssue(repo.ID, admin.ID, "reaction target", "", nil, nil, 0)

	parents := []string{
		"/api/v3/repos/admin/r/issues/" + itoa(issue.Number) + "/reactions",
		"/api/v3/repos/admin/r/issues/comments/1/reactions",
		"/api/v3/repos/admin/r/pulls/comments/1/reactions",
		"/api/v3/repos/admin/r/comments/1/reactions",
		"/api/v3/repos/admin/r/releases/1/reactions",
	}
	body, _ := json.Marshal(map[string]string{"content": "rocket"})
	for _, p := range parents {
		req := httptest.NewRequest("POST", p, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
		w := httptest.NewRecorder()
		s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Errorf("%s: status %d body %s", p, w.Code, w.Body.String())
		}
	}

	// A same-numbered issue in another repository shares no reactions with
	// the one above, and reacting to a nonexistent issue is a 404.
	other := s.store.CreateRepo(admin, "r2", "", false)
	otherIssue := s.store.CreateIssue(other.ID, admin.ID, "other target", "", nil, nil, 0)
	if otherIssue.Number != issue.Number {
		t.Fatalf("test premise: issue numbers differ (%d vs %d)", otherIssue.Number, issue.Number)
	}
	// The GET route lives on the issues two-segment dispatcher, which this
	// minimal server does not mount; assert against the store directly.
	if leaked := s.store.Reactions.ListReactions("issue", otherIssue.ID, ""); len(leaked) != 0 {
		t.Errorf("cross-repo issue reactions leaked: %d reactions", len(leaked))
	}
	if own := s.store.Reactions.ListReactions("issue", issue.ID, ""); len(own) != 1 {
		t.Errorf("reaction not keyed to the issue's global id: %d reactions", len(own))
	}
	missReq := httptest.NewRequest("POST", "/api/v3/repos/admin/r/issues/9999/reactions", bytes.NewReader(body))
	missReq.Header.Set("Authorization", "Bearer bleephub-admin-token-00000000000000000000")
	mw := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(mw, missReq)
	if mw.Code != http.StatusNotFound {
		t.Errorf("reaction on nonexistent issue: status %d, want 404", mw.Code)
	}
}

// itoa avoids strconv import noise in tests.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		buf[n] = '-'
	}
	return string(buf[n:])
}
