package bleephub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func autolinksTestServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer()
	s.registerGHRepoRoutes()
	s.registerGHRepoAutolinkRoutes()
	return s
}

func doAutolinkReq(s *Server, token, method, path string, body []byte) *httptest.ResponseRecorder {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	return w
}

func TestAutolinks_CreateListGetDelete(t *testing.T) {
	s := autolinksTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "autolink-repo", "", false)

	w := doAutolinkReq(s, adminPAT, "POST", "/api/v3/repos/"+repo.FullName+"/autolinks", []byte(`{"key_prefix":"TICKET-","url_template":"https://tracker.example/TICKET-<num>","is_alphanumeric":true}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	if created["key_prefix"] != "TICKET-" {
		t.Fatalf("key_prefix = %v, want TICKET-", created["key_prefix"])
	}
	if created["url_template"] != "https://tracker.example/TICKET-<num>" {
		t.Fatalf("url_template = %v", created["url_template"])
	}
	if created["is_alphanumeric"] != true {
		t.Fatalf("is_alphanumeric = %v, want true", created["is_alphanumeric"])
	}
	id := int(created["id"].(float64))

	w = doAutolinkReq(s, adminPAT, "GET", "/api/v3/repos/"+repo.FullName+"/autolinks", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body.String())
	}
	var list []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}

	w = doAutolinkReq(s, adminPAT, "GET", fmt.Sprintf("/api/v3/repos/%s/autolinks/%d", repo.FullName, id), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", w.Code, w.Body.String())
	}
	var got map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["id"] != float64(id) {
		t.Fatalf("get id = %v", got["id"])
	}

	w = doAutolinkReq(s, adminPAT, "DELETE", fmt.Sprintf("/api/v3/repos/%s/autolinks/%d", repo.FullName, id), nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", w.Code, w.Body.String())
	}

	w = doAutolinkReq(s, adminPAT, "GET", fmt.Sprintf("/api/v3/repos/%s/autolinks/%d", repo.FullName, id), nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want 404", w.Code)
	}
}

func TestAutolinks_CreateDefaultsAlphanumeric(t *testing.T) {
	s := autolinksTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "autolink-default", "", false)

	w := doAutolinkReq(s, adminPAT, "POST", "/api/v3/repos/"+repo.FullName+"/autolinks", []byte(`{"key_prefix":"BUG-","url_template":"https://bugs/BUG-<num>"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	if created["is_alphanumeric"] != true {
		t.Fatalf("is_alphanumeric default = %v, want true", created["is_alphanumeric"])
	}
}

func TestAutolinks_CreateMissingFields(t *testing.T) {
	s := autolinksTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "autolink-missing", "", false)

	w := doAutolinkReq(s, adminPAT, "POST", "/api/v3/repos/"+repo.FullName+"/autolinks", []byte(`{"url_template":"https://x"}`))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
}

func TestAutolinks_NonAdminCannotCreate(t *testing.T) {
	s := autolinksTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "autolink-priv", "", true)
	s.store.mu.Lock()
	other := &User{ID: s.store.NextUser, Login: "other", Type: "User", Email: "other@bleephub.local"}
	s.store.NextUser++
	s.store.Users[other.ID] = other
	s.store.UsersByLogin[other.Login] = other
	s.store.mu.Unlock()
	s.store.AddRepoCollaborator(repo.Owner.Login, repo.Name, other.Login, "push")
	otherToken := s.store.CreateToken(other.ID, "repo").Value

	w := doAutolinkReq(s, otherToken, "POST", "/api/v3/repos/"+repo.FullName+"/autolinks", []byte(`{"key_prefix":"X-","url_template":"https://x"}`))
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin create status = %d, want 403", w.Code)
	}
}

func TestAutolinks_PrivateRepoNoRead(t *testing.T) {
	s := autolinksTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "autolink-secret", "", true)
	autolink := s.store.CreateRepoAutolink(repo.FullName, "FOO-", "https://foo/<num>", true)

	w := doAutolinkReq(s, "", "GET", "/api/v3/repos/"+repo.FullName+"/autolinks", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unauthed private list status = %d, want 404", w.Code)
	}

	w = doAutolinkReq(s, "", "GET", fmt.Sprintf("/api/v3/repos/%s/autolinks/%d", repo.FullName, autolink.ID), nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unauthed private get status = %d, want 404", w.Code)
	}
}
