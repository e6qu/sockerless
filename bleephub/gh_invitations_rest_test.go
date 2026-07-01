package bleephub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func invitationsTestServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer()
	s.registerGHRepoRoutes()
	s.registerGHRepoInvitationRoutes()
	return s
}

func doInvitationReq(s *Server, token, method, path string, body []byte) *httptest.ResponseRecorder {
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

func makeOtherUser(s *Server, login string) (*User, string) {
	s.store.mu.Lock()
	u := &User{ID: s.store.NextUser, Login: login, Type: "User", Email: login + "@bleephub.local"}
	s.store.NextUser++
	s.store.Users[u.ID] = u
	s.store.UsersByLogin[u.Login] = u
	s.store.mu.Unlock()
	tok := s.store.CreateToken(u.ID, "repo")
	return u, tok.Value
}

func TestInvitations_RepoListUpdateCancel(t *testing.T) {
	s := invitationsTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "invite-repo", "", false)
	other, _ := makeOtherUser(s, "colleague")
	inv := s.store.CreateRepoInvitation(repo.FullName, other.Login, "", admin.ID, "pull")

	w := doInvitationReq(s, adminPAT, "GET", "/api/v3/repos/"+repo.FullName+"/invitations", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body.String())
	}
	var list []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
	if list[0]["permissions"] != "read" {
		t.Fatalf("permissions = %v, want read", list[0]["permissions"])
	}

	w = doInvitationReq(s, adminPAT, "PATCH", fmt.Sprintf("/api/v3/repos/%s/invitations/%d", repo.FullName, inv.ID), []byte(`{"permissions":"push"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
	}
	var updated map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &updated)
	if updated["permissions"] != "write" {
		t.Fatalf("permissions after update = %v, want write", updated["permissions"])
	}

	w = doInvitationReq(s, adminPAT, "DELETE", fmt.Sprintf("/api/v3/repos/%s/invitations/%d", repo.FullName, inv.ID), nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("cancel status = %d, body = %s", w.Code, w.Body.String())
	}
	if s.store.GetRepoInvitation(repo.FullName, inv.ID) != nil {
		t.Fatal("invitation still exists after cancel")
	}
}

func TestInvitations_UserAcceptAndDecline(t *testing.T) {
	s := invitationsTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "invite-user-repo", "", false)
	other, otherToken := makeOtherUser(s, "invited")
	inv := s.store.CreateRepoInvitation(repo.FullName, other.Login, "", admin.ID, "push")

	w := doInvitationReq(s, otherToken, "GET", "/api/v3/user/repository_invitations", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("user list status = %d, body = %s", w.Code, w.Body.String())
	}
	var list []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("user list len = %d, want 1", len(list))
	}

	w = doInvitationReq(s, otherToken, "PATCH", fmt.Sprintf("/api/v3/user/repository_invitations/%d", inv.ID), nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("accept status = %d, body = %s", w.Code, w.Body.String())
	}
	perm := s.store.GetRepoCollaboratorPermission(repo.Owner.Login, repo.Name, other.Login)
	if perm != "push" {
		t.Fatalf("collaborator permission = %v, want push", perm)
	}
	if s.store.GetRepoInvitation(repo.FullName, inv.ID) != nil {
		t.Fatal("invitation still pending after accept")
	}

	// Decline: create a second invitation and decline it.
	inv2 := s.store.CreateRepoInvitation(repo.FullName, other.Login, "", admin.ID, "pull")
	w = doInvitationReq(s, otherToken, "DELETE", fmt.Sprintf("/api/v3/user/repository_invitations/%d", inv2.ID), nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("decline status = %d, body = %s", w.Code, w.Body.String())
	}
	if s.store.GetRepoInvitation(repo.FullName, inv2.ID) != nil {
		t.Fatal("invitation still exists after decline")
	}
}

func TestInvitations_NonAdminCannotManage(t *testing.T) {
	s := invitationsTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "invite-priv", "", true)
	other, otherToken := makeOtherUser(s, "outsider")
	inv := s.store.CreateRepoInvitation(repo.FullName, other.Login, "", admin.ID, "pull")
	s.store.AddRepoCollaborator(repo.Owner.Login, repo.Name, other.Login, "push")

	w := doInvitationReq(s, otherToken, "GET", "/api/v3/repos/"+repo.FullName+"/invitations", nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin list status = %d, want 403", w.Code)
	}

	w = doInvitationReq(s, otherToken, "PATCH", fmt.Sprintf("/api/v3/repos/%s/invitations/%d", repo.FullName, inv.ID), []byte(`{"permissions":"admin"}`))
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin update status = %d, want 403", w.Code)
	}

	w = doInvitationReq(s, otherToken, "DELETE", fmt.Sprintf("/api/v3/repos/%s/invitations/%d", repo.FullName, inv.ID), nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin cancel status = %d, want 403", w.Code)
	}
}

func TestInvitations_CannotAcceptOtherUsersInvitation(t *testing.T) {
	s := invitationsTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "invite-mismatch", "", false)
	_, aliceToken := makeOtherUser(s, "alice")
	bob, _ := makeOtherUser(s, "bob")
	inv := s.store.CreateRepoInvitation(repo.FullName, bob.Login, "", admin.ID, "pull")

	w := doInvitationReq(s, aliceToken, "PATCH", fmt.Sprintf("/api/v3/user/repository_invitations/%d", inv.ID), nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("mismatched accept status = %d, want 404", w.Code)
	}

	w = doInvitationReq(s, aliceToken, "DELETE", fmt.Sprintf("/api/v3/user/repository_invitations/%d", inv.ID), nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("mismatched decline status = %d, want 404", w.Code)
	}
}

func TestInvitations_UserEndpointsRequireAuth(t *testing.T) {
	s := invitationsTestServer(t)
	w := doInvitationReq(s, "", "GET", "/api/v3/user/repository_invitations", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthed user list status = %d, want 401", w.Code)
	}
}
