package bleephub

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
)

func issueTestServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer()
	s.registerGHIssueRoutes()
	return s
}

func TestUnitCreateIssue_Basic(t *testing.T) {
	s := issueTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "testrepo", "", false)

	body, _ := json.Marshal(map[string]string{"title": "Bug fix", "body": "Something broke"})
	w := doMiscReq(s, "POST", "/api/v3/repos/"+repo.FullName+"/issues", string(body))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}

	var issue map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &issue); err != nil {
		t.Fatal(err)
	}
	if title, _ := issue["title"].(string); title != "Bug fix" {
		t.Errorf("title = %q, want %q", title, "Bug fix")
	}
	if state, _ := issue["state"].(string); state != "open" {
		t.Errorf("state = %q, want %q", state, "open")
	}
	if num, ok := issue["number"]; !ok || num == nil {
		t.Error("missing number field")
	}
}

func TestUnitCreateIssue_MissingTitle(t *testing.T) {
	s := issueTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "testrepo", "", false)

	body, _ := json.Marshal(map[string]string{"body": "no title"})
	w := doMiscReq(s, "POST", "/api/v3/repos/"+repo.FullName+"/issues", string(body))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", w.Code, w.Body.String())
	}
}

func TestUnitListIssues(t *testing.T) {
	s := issueTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "testrepo", "", false)
	s.store.CreateIssue(repo.ID, admin.ID, "First", "body1", nil, nil, 0)
	s.store.CreateIssue(repo.ID, admin.ID, "Second", "body2", nil, nil, 0)

	w := doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/issues", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var list []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
}

func TestUnitGetIssue(t *testing.T) {
	s := issueTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "testrepo", "", false)
	issue := s.store.CreateIssue(repo.ID, admin.ID, "Found me", "body", nil, nil, 0)

	w := doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/issues/"+strconv.Itoa(issue.Number), "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if title, _ := got["title"].(string); title != "Found me" {
		t.Errorf("title = %q, want %q", title, "Found me")
	}
}

func TestUnitGetIssue_NotFound(t *testing.T) {
	s := issueTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "testrepo", "", false)

	w := doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/issues/999", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
}

func TestUnitUpdateIssue(t *testing.T) {
	s := issueTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "testrepo", "", false)
	issue := s.store.CreateIssue(repo.ID, admin.ID, "Old title", "body", nil, nil, 0)

	patchBody, _ := json.Marshal(map[string]string{"title": "New title"})
	w := doMiscReq(s, "PATCH", "/api/v3/repos/"+repo.FullName+"/issues/"+strconv.Itoa(issue.Number), string(patchBody))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	w2 := doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/issues/"+strconv.Itoa(issue.Number), "")
	var got map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if title, _ := got["title"].(string); title != "New title" {
		t.Errorf("title = %q, want %q", title, "New title")
	}
}

func TestUnitCloseIssue(t *testing.T) {
	s := issueTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "testrepo", "", false)
	issue := s.store.CreateIssue(repo.ID, admin.ID, "To close", "body", nil, nil, 0)

	patchBody, _ := json.Marshal(map[string]string{"state": "closed"})
	w := doMiscReq(s, "PATCH", "/api/v3/repos/"+repo.FullName+"/issues/"+strconv.Itoa(issue.Number), string(patchBody))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if state, _ := got["state"].(string); state != "closed" {
		t.Errorf("state = %q, want %q", state, "closed")
	}
}

func TestUnitReopenIssue(t *testing.T) {
	s := issueTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "testrepo", "", false)
	issue := s.store.CreateIssue(repo.ID, admin.ID, "To reopen", "body", nil, nil, 0)

	closeBody, _ := json.Marshal(map[string]string{"state": "closed"})
	doMiscReq(s, "PATCH", "/api/v3/repos/"+repo.FullName+"/issues/"+strconv.Itoa(issue.Number), string(closeBody))

	openBody, _ := json.Marshal(map[string]string{"state": "open"})
	w := doMiscReq(s, "PATCH", "/api/v3/repos/"+repo.FullName+"/issues/"+strconv.Itoa(issue.Number), string(openBody))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if state, _ := got["state"].(string); state != "open" {
		t.Errorf("state = %q, want %q", state, "open")
	}
}

func TestUnitLockUnlockIssue(t *testing.T) {
	s := issueTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "testrepo", "", false)
	issue := s.store.CreateIssue(repo.ID, admin.ID, "Lock me", "body", nil, nil, 0)

	w := doMiscReq(s, "PUT", "/api/v3/repos/"+repo.FullName+"/issues/"+strconv.Itoa(issue.Number)+"/lock", "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("lock status = %d, want 204; body = %s", w.Code, w.Body.String())
	}

	w2 := doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/issues/"+strconv.Itoa(issue.Number), "")
	var locked map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &locked); err != nil {
		t.Fatal(err)
	}
	if v, _ := locked["locked"].(bool); !v {
		t.Errorf("locked = %v, want true", v)
	}

	w3 := doMiscReq(s, "DELETE", "/api/v3/repos/"+repo.FullName+"/issues/"+strconv.Itoa(issue.Number)+"/lock", "")
	if w3.Code != http.StatusNoContent {
		t.Fatalf("unlock status = %d, want 204; body = %s", w3.Code, w3.Body.String())
	}

	w4 := doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/issues/"+strconv.Itoa(issue.Number), "")
	var unlocked map[string]any
	if err := json.Unmarshal(w4.Body.Bytes(), &unlocked); err != nil {
		t.Fatal(err)
	}
	if v, _ := unlocked["locked"].(bool); v {
		t.Errorf("locked = %v, want false", v)
	}
}

func TestUnitDeleteIssueComment(t *testing.T) {
	s := issueTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "testrepo", "", false)
	issue := s.store.CreateIssue(repo.ID, admin.ID, "With comment", "body", nil, nil, 0)
	comment := s.store.CreateCommentFor("issue", issue.ID, admin.ID, "nice issue")

	w := doMiscReq(s, "DELETE", "/api/v3/repos/"+repo.FullName+"/issues/comments/"+strconv.Itoa(comment.ID), "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204; body = %s", w.Code, w.Body.String())
	}

	w2 := doMiscReq(s, "GET", "/api/v3/repos/"+repo.FullName+"/issues/"+strconv.Itoa(issue.Number)+"/comments", "")
	var list []map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		if id, ok := c["id"]; ok && id == float64(comment.ID) {
			t.Errorf("comment %d still in list after delete", comment.ID)
		}
	}
}

func TestUnitCreateLabel(t *testing.T) {
	s := issueTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "testrepo", "", false)

	body, _ := json.Marshal(map[string]string{"name": "bug", "color": "ff0000"})
	w := doMiscReq(s, "POST", "/api/v3/repos/"+repo.FullName+"/labels", string(body))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}

	var label map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &label); err != nil {
		t.Fatal(err)
	}
	if name, _ := label["name"].(string); name != "bug" {
		t.Errorf("name = %q, want %q", name, "bug")
	}
}

func TestUnitDeleteLabel(t *testing.T) {
	s := issueTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "testrepo", "", false)
	lbl := s.store.CreateLabel(repo.ID, "bug", "", "ff0000")

	w := doMiscReq(s, "DELETE", "/api/v3/repos/"+repo.FullName+"/labels/"+lbl.Name, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body = %s", w.Code, w.Body.String())
	}
}

func TestUnitCreateMilestone(t *testing.T) {
	s := issueTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "testrepo", "", false)

	body, _ := json.Marshal(map[string]string{"title": "v1.0"})
	w := doMiscReq(s, "POST", "/api/v3/repos/"+repo.FullName+"/milestones", string(body))
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}

	var ms map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &ms); err != nil {
		t.Fatal(err)
	}
	if title, _ := ms["title"].(string); title != "v1.0" {
		t.Errorf("title = %q, want %q", title, "v1.0")
	}
}

func TestUnitDeleteMilestone(t *testing.T) {
	s := issueTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "testrepo", "", false)
	ms := s.store.CreateMilestone(repo.ID, admin.ID, "v1.0", "", "open", nil)

	w := doMiscReq(s, "DELETE", "/api/v3/repos/"+repo.FullName+"/milestones/"+strconv.Itoa(ms.Number), "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body = %s", w.Code, w.Body.String())
	}
}
