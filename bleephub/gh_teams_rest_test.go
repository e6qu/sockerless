package bleephub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func setupTeamTestServer(t *testing.T) (*Server, *User, *User, *User, *User, *Org, *Team) {
	t.Helper()
	s := newTestServer()
	s.registerGHTeamRoutes()

	admin := s.store.LookupUserByLogin("admin")
	org := s.store.CreateOrg(admin, "team-test-org", "Team Test Org", "")
	if org == nil {
		t.Fatal("CreateOrg returned nil")
	}
	team := s.store.CreateTeam(org.Login, "engineers", TeamOptions{Permission: TeamPermissionPush})
	if team == nil {
		t.Fatal("CreateTeam returned nil")
	}
	maintainer := seedTestUser(s, "team-maintainer")
	member := seedTestUser(s, "team-member")
	outsider := seedTestUser(s, "team-outsider")

	s.store.SetMembership(org.Login, maintainer.ID, OrgRoleMember, MembershipStateActive)
	s.store.SetMembership(org.Login, member.ID, OrgRoleMember, MembershipStateActive)
	s.store.SetTeamMembership(org.Login, team.Slug, admin.ID, TeamRoleMaintainer)
	s.store.SetTeamMembership(org.Login, team.Slug, maintainer.ID, TeamRoleMaintainer)
	s.store.SetTeamMembership(org.Login, team.Slug, member.ID, TeamRoleMember)

	return s, admin, maintainer, member, outsider, org, team
}

func teamTestToken(s *Server, u *User, scopes string) string {
	tok := s.store.CreateToken(u.ID, scopes)
	return tok.Value
}

func TestListAuthUserTeams_RequiresAuthNotReadOrgScope(t *testing.T) {
	s := newTestServer()
	s.registerGHTeamRoutes()

	admin := s.store.LookupUserByLogin("admin")
	org := s.store.CreateOrg(admin, "auth-user-teams-org", "Auth User Teams Org", "")
	team := s.store.CreateTeam(org.Login, "engineers", TeamOptions{})
	s.store.SetMembership(org.Login, admin.ID, OrgRoleMember, MembershipStateActive)
	s.store.SetTeamMembership(org.Login, team.Slug, admin.ID, TeamRoleMaintainer)

	// A classic OAuth token with only "repo" (no read:org) must still be
	// able to list the authenticated user's own teams. Regression for #754.
	oapp := s.store.CreateOAuthApp(admin.ID, "ScopeRegressor", "", "", "")
	tokRepoOnly, _ := s.store.CreateUserToServerToken(admin.ID, 0, oapp.ClientID, "repo", 8*time.Hour, false)

	req := httptest.NewRequest("GET", "/api/v3/user/teams", nil)
	req.Header.Set("Authorization", "Bearer "+tokRepoOnly.Token)
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /user/teams with repo-only OAuth token = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var listed []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed %d teams, want 1", len(listed))
	}

	// Unauthenticated requests still get 401.
	req2 := httptest.NewRequest("GET", "/api/v3/user/teams", nil)
	w2 := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("GET /user/teams without token = %d, want 401", w2.Code)
	}
}

func TestTeamMembersList(t *testing.T) {
	s, _, _, member, outsider, _, team := setupTeamTestServer(t)

	// Outsider (authenticated but not org member) gets 404.
	w := tokenRequest(s, "GET", "/api/v3/orgs/team-test-org/teams/"+team.Slug+"/members", teamTestToken(s, outsider, "read:org"))
	if w.Code != http.StatusNotFound {
		t.Errorf("outsider list members = %d, want 404", w.Code)
	}

	// Org member can list.
	w = tokenRequest(s, "GET", "/api/v3/orgs/team-test-org/teams/"+team.Slug+"/members", teamTestToken(s, member, "read:org"))
	if w.Code != http.StatusOK {
		t.Fatalf("member list members = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var listed []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 3 {
		t.Errorf("listed %d members, want 3", len(listed))
	}
}

func TestTeamMembershipGet(t *testing.T) {
	s, _, maintainer, member, outsider, _, team := setupTeamTestServer(t)

	path := "/api/v3/orgs/team-test-org/teams/" + team.Slug + "/memberships/" + member.Login

	// Outsider cannot read.
	w := tokenRequest(s, "GET", path, teamTestToken(s, outsider, "read:org"))
	if w.Code != http.StatusNotFound {
		t.Errorf("outsider get membership = %d, want 404", w.Code)
	}

	// Org member can read.
	w = tokenRequest(s, "GET", path, teamTestToken(s, maintainer, "read:org"))
	if w.Code != http.StatusOK {
		t.Fatalf("get membership = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var m map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["role"] != "member" || m["state"] != "active" {
		t.Errorf("membership = %v, want member/active", m)
	}
	if _, ok := m["user"].(map[string]interface{}); !ok {
		t.Errorf("membership missing user object")
	}
	if _, ok := m["team"].(map[string]interface{}); !ok {
		t.Errorf("membership missing team object")
	}
	if m["organization_url"] == "" {
		t.Errorf("membership missing organization_url")
	}
}

func TestTeamMembershipAddUpdateRemove(t *testing.T) {
	s, admin, maintainer, _, outsider, org, team := setupTeamTestServer(t)
	newUser := seedTestUser(s, "team-newuser")
	s.store.SetMembership(org.Login, newUser.ID, OrgRoleMember, MembershipStateActive)

	path := "/api/v3/orgs/team-test-org/teams/" + team.Slug + "/memberships/" + newUser.Login

	// Outsider cannot add.
	w := tokenRequest(s, "PUT", path, teamTestToken(s, outsider, "read:org"))
	if w.Code != http.StatusForbidden && w.Code != http.StatusNotFound {
		t.Errorf("outsider add membership = %d, want 403/404", w.Code)
	}

	// Maintainer can add a member.
	body, _ := json.Marshal(map[string]string{"role": "member"})
	w = httptestPost(s, path, teamTestToken(s, maintainer, "admin:org"), body)
	if w.Code != http.StatusOK {
		t.Fatalf("maintainer add member = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	// Maintainer cannot promote to maintainer.
	body, _ = json.Marshal(map[string]string{"role": "maintainer"})
	w = httptestPost(s, path, teamTestToken(s, maintainer, "admin:org"), body)
	if w.Code != http.StatusForbidden {
		t.Errorf("maintainer promote = %d, want 403", w.Code)
	}

	// Owner can promote.
	w = httptestPost(s, path, teamTestToken(s, admin, "admin:org"), body)
	if w.Code != http.StatusOK {
		t.Fatalf("owner promote = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	// Maintainer can remove.
	w = tokenRequest(s, "DELETE", path, teamTestToken(s, maintainer, "admin:org"))
	if w.Code != http.StatusNoContent {
		t.Errorf("maintainer remove = %d, want 204; body=%s", w.Code, w.Body.String())
	}
}

func TestTeamMembershipRequiresAuth(t *testing.T) {
	s, _, _, member, _, _, team := setupTeamTestServer(t)
	path := "/api/v3/orgs/team-test-org/teams/" + team.Slug + "/members"

	// Unauthenticated request is rejected.
	w := tokenRequest(s, "GET", path, "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated list = %d, want 401", w.Code)
	}

	// Authenticated org member can read.
	w = tokenRequest(s, "GET", path, teamTestToken(s, member, "read:org"))
	if w.Code != http.StatusOK {
		t.Errorf("authenticated list = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

func TestTeamReposCRUD(t *testing.T) {
	s, admin, maintainer, member, _, org, team := setupTeamTestServer(t)
	repo := s.store.CreateOrgRepo(org, admin, "team-repo", "", false)
	if repo == nil {
		t.Fatal("CreateOrgRepo returned nil")
	}

	listPath := "/api/v3/orgs/team-test-org/teams/" + team.Slug + "/repos"
	repoPath := "/api/v3/orgs/team-test-org/teams/" + team.Slug + "/repos/team-test-org/team-repo"

	// Org member can list (empty initially).
	w := tokenRequest(s, "GET", listPath, teamTestToken(s, member, "read:org"))
	if w.Code != http.StatusOK {
		t.Fatalf("list repos = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	// Maintainer adds repo.
	w = httptestPost(s, repoPath, teamTestToken(s, maintainer, "admin:org"), nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("add repo = %d, want 204; body=%s", w.Code, w.Body.String())
	}

	// List now contains the repo with role_name derived from team default permission.
	w = tokenRequest(s, "GET", listPath, teamTestToken(s, member, "read:org"))
	if w.Code != http.StatusOK {
		t.Fatalf("list repos after add = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var repos []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &repos); err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0]["full_name"] != "team-test-org/team-repo" {
		t.Errorf("repos = %v, want [team-test-org/team-repo]", repos)
	}
	if repos[0]["role_name"] != "write" {
		t.Errorf("role_name = %v, want write", repos[0]["role_name"])
	}

	// Check repo returns 204.
	w = tokenRequest(s, "GET", repoPath, teamTestToken(s, member, "read:org"))
	if w.Code != http.StatusNoContent {
		t.Errorf("check repo = %d, want 204; body=%s", w.Code, w.Body.String())
	}

	// Maintainer removes repo.
	w = tokenRequest(s, "DELETE", repoPath, teamTestToken(s, maintainer, "admin:org"))
	if w.Code != http.StatusNoContent {
		t.Errorf("remove repo = %d, want 204; body=%s", w.Code, w.Body.String())
	}
}

func TestTeamRepoPermissionOverride(t *testing.T) {
	s, admin, _, _, _, org, team := setupTeamTestServer(t)
	repo := s.store.CreateOrgRepo(org, admin, "perm-repo", "", false)
	if repo == nil {
		t.Fatal("CreateOrgRepo returned nil")
	}

	repoPath := "/api/v3/orgs/team-test-org/teams/" + team.Slug + "/repos/team-test-org/perm-repo"

	// Add with explicit admin permission.
	body, _ := json.Marshal(map[string]string{"permission": "admin"})
	w := httptestPost(s, repoPath, teamTestToken(s, admin, "admin:org"), body)
	if w.Code != http.StatusNoContent {
		t.Fatalf("add repo admin = %d, want 204; body=%s", w.Code, w.Body.String())
	}

	perm, linked := s.store.GetTeamRepoPermission(org.Login, team.Slug, repo.FullName)
	if !linked || perm != TeamPermissionAdmin {
		t.Errorf("repo perm = %v, linked=%v, want admin/true", perm, linked)
	}

	// Invalid permission is rejected.
	body, _ = json.Marshal(map[string]string{"permission": "superuser"})
	w = httptestPost(s, repoPath, teamTestToken(s, admin, "admin:org"), body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("invalid permission = %d, want 422", w.Code)
	}
}

func httptestPost(s *Server, path, token string, body []byte) *httptest.ResponseRecorder {
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest("PUT", path, nil)
	} else {
		r = httptest.NewRequest("PUT", path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, r)
	return w
}
