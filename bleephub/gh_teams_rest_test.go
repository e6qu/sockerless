package bleephub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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
	// X-OAuth-Scopes must reflect the OAuth token's scopes (real GitHub emits it).
	if got := w.Header().Get("X-OAuth-Scopes"); got != "repo" {
		t.Fatalf("X-OAuth-Scopes = %q, want repo", got)
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

// TestListAuthUserTeams_ViaOAuthWebFlow verifies that GET /api/v3/user/teams
// returns the authenticated user's teams when the caller authenticated through
// the real OAuth web-flow (authorize → access_token), the path Auth.js's
// GitHub provider uses. Regression for #763.
func TestListAuthUserTeams_ViaOAuthWebFlow(t *testing.T) {
	s := newTestServer()
	s.registerGHTeamRoutes()
	s.registerGHOAuthRoutes()

	admin := s.store.LookupUserByLogin("admin")
	org := s.store.CreateOrg(admin, "oauth-webflow-org", "OAuth WebFlow Org", "")
	team := s.store.CreateTeam(org.Login, "platform-admins", TeamOptions{})
	s.store.SetMembership(org.Login, admin.ID, OrgRoleMember, MembershipStateActive)
	s.store.SetTeamMembership(org.Login, team.Slug, admin.ID, TeamRoleMaintainer)

	oapp := s.store.CreateOAuthApp(admin.ID, "AuthJSReproducer", "", "", "http://localhost:3000/api/auth/callback/github")

	// Step 1: authorize through the real login + consent flow.
	code := authorizeOAuthWebFlow(t, s, "admin", oapp.ClientID, "http://localhost:3000/api/auth/callback/github", "repo", "xyz")

	// Step 2: exchange the code for an access token (JSON Accept, like Auth.js).
	form := url.Values{}
	form.Set("client_id", oapp.ClientID)
	form.Set("client_secret", oapp.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", "http://localhost:3000/api/auth/callback/github")
	form.Set("grant_type", "authorization_code")
	req2 := httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(form.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.Header.Set("Accept", "application/json")
	w2 := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("access_token status = %d, want 200; body=%s", w2.Code, w2.Body.String())
	}
	var tokenResp map[string]interface{}
	if err := json.Unmarshal(w2.Body.Bytes(), &tokenResp); err != nil {
		t.Fatal(err)
	}
	accessToken, _ := tokenResp["access_token"].(string)
	if accessToken == "" {
		t.Fatalf("access_token response missing access_token: %s", w2.Body.String())
	}

	// Step 3: call GET /api/v3/user/teams with the web-flow token.
	req3 := httptest.NewRequest("GET", "/api/v3/user/teams?per_page=100", nil)
	req3.Header.Set("Authorization", "Bearer "+accessToken)
	w3 := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("GET /user/teams status = %d, want 200; body=%s", w3.Code, w3.Body.String())
	}
	var listed []map[string]interface{}
	if err := json.Unmarshal(w3.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed %d teams, want 1; body=%s", len(listed), w3.Body.String())
	}
	if listed[0]["slug"] != "platform-admins" {
		t.Fatalf("expected slug platform-admins, got %v", listed[0]["slug"])
	}
	orgObj, _ := listed[0]["organization"].(map[string]interface{})
	if orgObj == nil || orgObj["login"] != "oauth-webflow-org" {
		t.Fatalf("expected organization.login=oauth-webflow-org, got %v", listed[0]["organization"])
	}
}

// TestListAuthUserTeams_ViaOAuthWebFlow_ReadOrgScope verifies that GET
// /api/v3/user/teams returns the authenticated user's teams when the OAuth
// web flow is authorized with only read:org, and the team was created without
// an explicit add-member call. Real GitHub auto-adds the team creator as a
// maintainer, so /user/teams must reflect that membership. Regression for
// #763/#765.
func TestListAuthUserTeams_ViaOAuthWebFlow_ReadOrgScope(t *testing.T) {
	s := newTestServer()
	s.registerGHTeamRoutes()
	s.registerGHOAuthRoutes()

	admin := s.store.LookupUserByLogin("admin")
	s.store.CreateOrg(admin, "oauth-readorg-org", "OAuth ReadOrg Org", "")
	// Create the team through the API as the admin user; the creator must
	// become a maintainer automatically so /user/teams lists it without an
	// explicit membership call.
	reqCreate := httptest.NewRequest("POST", "/api/v3/orgs/oauth-readorg-org/teams", strings.NewReader(`{"name":"platform-admins"}`))
	reqCreate.Header.Set("Authorization", "Bearer "+s.store.CreateToken(admin.ID, "admin:org").Value)
	reqCreate.Header.Set("Content-Type", "application/json")
	wCreate := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(wCreate, reqCreate)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("create team status = %d, want 201; body=%s", wCreate.Code, wCreate.Body.String())
	}
	var createdTeam map[string]interface{}
	if err := json.Unmarshal(wCreate.Body.Bytes(), &createdTeam); err != nil {
		t.Fatal(err)
	}
	if createdTeam["members_count"] != float64(1) {
		t.Fatalf("expected members_count=1, got %v", createdTeam["members_count"])
	}

	oapp := s.store.CreateOAuthApp(admin.ID, "AuthJSReadOrg", "", "", "http://localhost:3000/api/auth/callback/github")

	// Step 1: authorize with read:org only through the real login + consent flow.
	code := authorizeOAuthWebFlow(t, s, "admin", oapp.ClientID, "http://localhost:3000/api/auth/callback/github", "read:org", "xyz")

	// Step 2: exchange the code for an access token.
	form := url.Values{}
	form.Set("client_id", oapp.ClientID)
	form.Set("client_secret", oapp.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", "http://localhost:3000/api/auth/callback/github")
	form.Set("grant_type", "authorization_code")
	req2 := httptest.NewRequest("POST", "/login/oauth/access_token", strings.NewReader(form.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.Header.Set("Accept", "application/json")
	w2 := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("access_token status = %d, want 200; body=%s", w2.Code, w2.Body.String())
	}
	var tokenResp map[string]interface{}
	if err := json.Unmarshal(w2.Body.Bytes(), &tokenResp); err != nil {
		t.Fatal(err)
	}
	accessToken, _ := tokenResp["access_token"].(string)
	if accessToken == "" {
		t.Fatalf("access_token response missing access_token: %s", w2.Body.String())
	}

	// Step 3: call GET /api/v3/user/teams with the web-flow token.
	req3 := httptest.NewRequest("GET", "/api/v3/user/teams?per_page=100", nil)
	req3.Header.Set("Authorization", "Bearer "+accessToken)
	w3 := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("GET /user/teams status = %d, want 200; body=%s", w3.Code, w3.Body.String())
	}
	var listed []map[string]interface{}
	if err := json.Unmarshal(w3.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed %d teams, want 1; body=%s", len(listed), w3.Body.String())
	}
	if listed[0]["slug"] != "platform-admins" {
		t.Fatalf("expected slug platform-admins, got %v", listed[0]["slug"])
	}
	if listed[0]["members_count"] != float64(1) {
		t.Fatalf("expected members_count=1, got %v", listed[0]["members_count"])
	}
	orgObj, _ := listed[0]["organization"].(map[string]interface{})
	if orgObj == nil || orgObj["login"] != "oauth-readorg-org" {
		t.Fatalf("expected organization.login=oauth-readorg-org, got %v", listed[0]["organization"])
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
