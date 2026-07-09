package bleephub

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func reposTestServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer()
	s.registerGHRepoRoutes()
	return s
}

func doRepoReq(s *Server, method, path string, body string) *httptest.ResponseRecorder {
	return doMiscReq(s, method, path, body)
}

func TestUnitCreateRepo_Basic(t *testing.T) {
	s := reposTestServer(t)
	w := doRepoReq(s, "POST", "/api/v3/user/repos", `{"name":"my-repo","description":"a test repo","private":false}`)
	if w.Code != 201 {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var repo map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &repo)
	if repo["name"] != "my-repo" {
		t.Fatalf("name = %v, want my-repo", repo["name"])
	}
	if repo["full_name"] != "admin/my-repo" {
		t.Fatalf("full_name = %v, want admin/my-repo", repo["full_name"])
	}
	if repo["private"] != false {
		t.Fatalf("private = %v, want false", repo["private"])
	}
	if repo["description"] != "a test repo" {
		t.Fatalf("description = %v, want a test repo", repo["description"])
	}
	owner, _ := repo["owner"].(map[string]interface{})
	if owner["login"] != "admin" {
		t.Fatalf("owner.login = %v, want admin", owner["login"])
	}
	if repo["default_branch"] != "main" {
		t.Fatalf("default_branch = %v, want main", repo["default_branch"])
	}
	if repo["visibility"] != "public" {
		t.Fatalf("visibility = %v, want public", repo["visibility"])
	}
	if repo["fork"] != false {
		t.Fatalf("fork = %v, want false", repo["fork"])
	}
}

func TestUnitCreateRepo_Private(t *testing.T) {
	s := reposTestServer(t)
	w := doRepoReq(s, "POST", "/api/v3/user/repos", `{"name":"priv-repo","private":true}`)
	if w.Code != 201 {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var repo map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &repo)
	if repo["private"] != true {
		t.Fatalf("private = %v, want true", repo["private"])
	}
	if repo["visibility"] != "private" {
		t.Fatalf("visibility = %v, want private", repo["visibility"])
	}
}

func TestUnitCreateRepo_MissingName(t *testing.T) {
	s := reposTestServer(t)
	w := doRepoReq(s, "POST", "/api/v3/user/repos", `{"description":"no name"}`)
	if w.Code != 422 {
		t.Fatalf("status = %d, want 422; body = %s", w.Code, w.Body.String())
	}
	var errResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &errResp)
	if errResp["message"] != "Validation Failed" {
		t.Fatalf("message = %v, want Validation Failed", errResp["message"])
	}
}

func TestUnitCreateRepo_DuplicateName(t *testing.T) {
	s := reposTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	s.store.CreateRepo(admin, "dup-repo", "", false)
	w := doRepoReq(s, "POST", "/api/v3/user/repos", `{"name":"dup-repo"}`)
	if w.Code != 422 {
		t.Fatalf("status = %d, want 422; body = %s", w.Code, w.Body.String())
	}
}

func TestUnitGetRepo_Found(t *testing.T) {
	s := reposTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "get-test", "desc", false)
	w := doRepoReq(s, "GET", "/api/v3/repos/"+repo.FullName, "")
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var got map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["name"] != "get-test" {
		t.Fatalf("name = %v, want get-test", got["name"])
	}
	if got["full_name"] != "admin/get-test" {
		t.Fatalf("full_name = %v, want admin/get-test", got["full_name"])
	}
	if got["description"] != "desc" {
		t.Fatalf("description = %v, want desc", got["description"])
	}
}

func TestUnitGetRepo_NotFound(t *testing.T) {
	s := reposTestServer(t)
	w := doRepoReq(s, "GET", "/api/v3/repos/admin/no-such-repo", "")
	if w.Code != 404 {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
}

func TestUnitGetRepo_PrivateNoAuth(t *testing.T) {
	s := reposTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	s.store.CreateRepo(admin, "secret-repo", "secret", true)
	req := httptest.NewRequest("GET", "/api/v3/repos/admin/secret-repo", nil)
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("unauthed private repo status = %d, want 404", w.Code)
	}
}

func TestUnitListRepos(t *testing.T) {
	s := reposTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	s.store.CreateRepo(admin, "repo-a", "", false)
	s.store.CreateRepo(admin, "repo-b", "", false)
	w := doRepoReq(s, "GET", "/api/v3/user/repos", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var repos []interface{}
	json.Unmarshal(w.Body.Bytes(), &repos)
	if len(repos) != 2 {
		t.Fatalf("count = %d, want 2", len(repos))
	}
}

func TestUnitListRepos_Empty(t *testing.T) {
	s := reposTestServer(t)
	w := doRepoReq(s, "GET", "/api/v3/user/repos", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var repos []interface{}
	json.Unmarshal(w.Body.Bytes(), &repos)
	if len(repos) != 0 {
		t.Fatalf("count = %d, want 0", len(repos))
	}
}

func TestUnitDeleteRepo(t *testing.T) {
	s := reposTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "del-repo", "", false)
	w := doRepoReq(s, "DELETE", "/api/v3/repos/"+repo.FullName, "")
	if w.Code != 204 {
		t.Fatalf("delete status = %d, body = %s", w.Code, w.Body.String())
	}
	w2 := doRepoReq(s, "GET", "/api/v3/repos/"+repo.FullName, "")
	if w2.Code != 404 {
		t.Fatalf("get after delete status = %d, want 404", w2.Code)
	}
}

func TestUnitDeleteRepo_NotFound(t *testing.T) {
	s := reposTestServer(t)
	w := doRepoReq(s, "DELETE", "/api/v3/repos/admin/no-such-repo", "")
	if w.Code != 404 {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
}

func TestUnitUpdateRepo_Description(t *testing.T) {
	s := reposTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "upd-repo", "", false)
	w := doRepoReq(s, "PATCH", "/api/v3/repos/"+repo.FullName, `{"description":"new desc"}`)
	if w.Code != 200 {
		t.Fatalf("patch status = %d, body = %s", w.Code, w.Body.String())
	}
	var got map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["description"] != "new desc" {
		t.Fatalf("description = %v, want new desc", got["description"])
	}
}

func TestUnitUpdateRepo_PrivateFlag(t *testing.T) {
	s := reposTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "vis-repo", "", false)
	w := doRepoReq(s, "PATCH", "/api/v3/repos/"+repo.FullName, `{"private":true}`)
	if w.Code != 200 {
		t.Fatalf("patch status = %d, body = %s", w.Code, w.Body.String())
	}
	var got map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["private"] != true {
		t.Fatalf("private = %v, want true", got["private"])
	}
	if got["visibility"] != "private" {
		t.Fatalf("visibility = %v, want private", got["visibility"])
	}
}

func TestUnitUpdateRepo_NotFound(t *testing.T) {
	s := reposTestServer(t)
	w := doRepoReq(s, "PATCH", "/api/v3/repos/admin/no-such-repo", `{"description":"x"}`)
	if w.Code != 404 {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
}

func TestUnitUpdateRepo_Archived(t *testing.T) {
	s := reposTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "archive-repo", "", false)
	w := doRepoReq(s, "PATCH", "/api/v3/repos/"+repo.FullName, `{"archived":true}`)
	if w.Code != 200 {
		t.Fatalf("patch status = %d, body = %s", w.Code, w.Body.String())
	}
	var got map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["archived"] != true {
		t.Fatalf("archived = %v, want true", got["archived"])
	}
	updated := s.store.GetRepo("admin", "archive-repo")
	if updated == nil || updated.ArchivedAt == nil {
		t.Fatalf("ArchivedAt = %v, want timestamp", updated)
	}
	w = doRepoReq(s, "PATCH", "/api/v3/repos/"+repo.FullName, `{"archived":false}`)
	if w.Code != 200 {
		t.Fatalf("unarchive status = %d, body = %s", w.Code, w.Body.String())
	}
	updated = s.store.GetRepo("admin", "archive-repo")
	if updated == nil {
		t.Fatalf("repository missing after unarchive")
	}
	if updated.ArchivedAt != nil {
		t.Fatalf("ArchivedAt after unarchive = %v, want nil", updated.ArchivedAt)
	}
}

func TestUnitRepoTopics_Empty(t *testing.T) {
	s := reposTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "topics-repo", "", false)
	w := doRepoReq(s, "GET", "/api/v3/repos/"+repo.FullName, "")
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	var got map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &got)
	topics, ok := got["topics"].([]interface{})
	if !ok {
		t.Fatalf("topics not array: %T", got["topics"])
	}
	if len(topics) != 0 {
		t.Fatalf("topics = %v, want empty", topics)
	}
}

func TestUnitRepoTopics_SetViaStore(t *testing.T) {
	s := reposTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "topics-store-repo", "", false)
	s.store.UpdateRepo("admin", "topics-store-repo", func(r *Repo) {
		r.Topics = []string{"go", "testing"}
	})
	w := doRepoReq(s, "GET", "/api/v3/repos/"+repo.FullName, "")
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	var got map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &got)
	topics, _ := got["topics"].([]interface{})
	if len(topics) != 2 {
		t.Fatalf("topics count = %d, want 2", len(topics))
	}
}

func TestUnitListUserRepos(t *testing.T) {
	s := reposTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	s.store.CreateRepo(admin, "user-list-a", "", false)
	s.store.CreateRepo(admin, "user-list-b", "", false)
	w := doRepoReq(s, "GET", "/api/v3/users/admin/repos", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var repos []interface{}
	json.Unmarshal(w.Body.Bytes(), &repos)
	if len(repos) != 2 {
		t.Fatalf("count = %d, want 2", len(repos))
	}
}

func TestUnitListUserRepos_NoAuth(t *testing.T) {
	s := reposTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	s.store.CreateRepo(admin, "pub-repo", "", false)
	req := httptest.NewRequest("GET", "/api/v3/users/admin/repos", nil)
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var repos []interface{}
	json.Unmarshal(w.Body.Bytes(), &repos)
	if len(repos) != 1 {
		t.Fatalf("count = %d, want 1", len(repos))
	}
}

func TestUnitCreateRepo_Fork(t *testing.T) {
	s := reposTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	parent := s.store.CreateRepo(admin, "parent-repo", "original", false)
	s.store.UpdateRepo("admin", "parent-repo", func(r *Repo) {
		r.Fork = false
	})
	child := s.store.CreateRepo(admin, "child-repo", "fork", false)
	s.store.UpdateRepo("admin", "child-repo", func(r *Repo) {
		r.Fork = true
	})
	w := doRepoReq(s, "GET", "/api/v3/repos/"+parent.FullName, "")
	if w.Code != 200 {
		t.Fatalf("parent status = %d", w.Code)
	}
	var parentJSON map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &parentJSON)
	if parentJSON["fork"] != false {
		t.Fatalf("parent fork = %v, want false", parentJSON["fork"])
	}

	w2 := doRepoReq(s, "GET", "/api/v3/repos/"+child.FullName, "")
	if w2.Code != 200 {
		t.Fatalf("child status = %d", w2.Code)
	}
	var childJSON map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &childJSON)
	if childJSON["fork"] != true {
		t.Fatalf("child fork = %v, want true", childJSON["fork"])
	}
}

func TestUnitRepoJSONFields(t *testing.T) {
	s := reposTestServer(t)
	w := doRepoReq(s, "POST", "/api/v3/user/repos", `{"name":"field-check"}`)
	if w.Code != 201 {
		t.Fatalf("status = %d", w.Code)
	}
	var repo map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &repo)

	for _, field := range []string{
		"id", "node_id", "name", "full_name", "owner", "private",
		"html_url", "description", "fork", "url", "clone_url",
		"ssh_url", "default_branch", "visibility", "archived",
		"stargazers_count", "topics", "permissions",
		"created_at", "updated_at", "pushed_at",
	} {
		if _, ok := repo[field]; !ok {
			t.Errorf("missing field %q in repo JSON", field)
		}
	}

	perms, _ := repo["permissions"].(map[string]interface{})
	if perms["admin"] != true || perms["push"] != true || perms["pull"] != true {
		t.Errorf("permissions = %v, want all true", perms)
	}
}

func TestUnitRepoJSONPermissionsFollowViewerAccess(t *testing.T) {
	s := reposTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "viewer-permissions", "", true)
	_, readerToken := makeOtherUser(s, "repo-json-reader")
	if !s.store.AddRepoCollaborator("admin", repo.Name, "repo-json-reader", "pull") {
		t.Fatal("failed to add pull collaborator")
	}

	w := doInvitationReq(s, readerToken, "GET", "/api/v3/repos/"+repo.FullName, nil)
	if w.Code != 200 {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var got map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &got)
	perms, _ := got["permissions"].(map[string]interface{})
	if perms["pull"] != true || perms["push"] != false || perms["admin"] != false {
		t.Fatalf("pull collaborator permissions = %v, want pull only", perms)
	}
}
