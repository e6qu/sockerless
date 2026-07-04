package bleephub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// tokenRequest drives a route through the /api middleware chain with an
// explicit bearer token, so requirePerm sees the token's resolved principal.
func tokenRequest(s *Server, method, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	return w
}

// seedTestUser inserts a fresh user and returns it.
func seedTestUser(s *Server, login string) *User {
	s.store.mu.Lock()
	u := &User{ID: s.store.NextUser, Login: login, Type: "User"}
	s.store.NextUser++
	s.store.Users[u.ID] = u
	s.store.UsersByLogin[u.Login] = u
	s.store.mu.Unlock()
	return u
}

// TestRunnerGroupReadsRequireAdministration guards the read side of the org
// runner-group surface. Real GitHub requires the `administration` permission
// (admin:org) for every runner-group endpoint, GETs included; an installation
// token without it gets 403 "Resource not accessible by integration", and an
// anonymous caller gets 401.
func TestRunnerGroupReadsRequireAdministration(t *testing.T) {
	s := newTestServer()
	s.registerRunnerGroupRoutes()

	admin := s.store.LookupUserByLogin("admin")
	org := s.store.CreateOrg(admin, "rg-org", "RG Org", "")
	if org == nil {
		t.Fatal("CreateOrg returned nil")
	}

	// An installation token WITHOUT the administration permission.
	app := s.store.CreateApp(admin.ID, "RG App", "", map[string]string{"contents": "read"}, nil)
	inst := s.store.CreateInstallation(app.ID, "Organization", org.ID, org.Login, map[string]string{"contents": "read"}, nil)
	noAdminTok := s.store.CreateInstallationToken(inst.ID, app.ID, map[string]string{"contents": "read"}, nil)

	reads := []struct{ name, path string }{
		{"list-groups", "/api/v3/orgs/rg-org/actions/runner-groups"},
		{"get-group", "/api/v3/orgs/rg-org/actions/runner-groups/1"},
		{"list-group-runners", "/api/v3/orgs/rg-org/actions/runner-groups/1/runners"},
		{"list-group-repos", "/api/v3/orgs/rg-org/actions/runner-groups/1/repositories"},
	}
	for _, r := range reads {
		t.Run(r.name+"/anon-401", func(t *testing.T) {
			w := tokenRequest(s, "GET", r.path, "")
			if w.Code != http.StatusUnauthorized {
				t.Errorf("anon GET %s = %d, want 401", r.path, w.Code)
			}
		})
		t.Run(r.name+"/no-admin-403", func(t *testing.T) {
			w := tokenRequest(s, "GET", r.path, noAdminTok.Token)
			if w.Code != http.StatusForbidden {
				t.Errorf("no-administration GET %s = %d, want 403", r.path, w.Code)
			}
		})
	}
}

// TestOrgAuditLogRequiresOwner guards the org audit log: it exposes secret
// and hook changes, so real GitHub restricts it to org owners. Non-owners and
// anonymous callers get 404 (existence is hidden), owners get 200.
func TestOrgAuditLogRequiresOwner(t *testing.T) {
	s := newTestServer()
	s.registerGHMiscEndpoints()

	admin := s.store.LookupUserByLogin("admin")
	org := s.store.CreateOrg(admin, "audit-org", "Audit Org", "")
	if org == nil {
		t.Fatal("CreateOrg nil")
	}
	s.recordAuditEvent("test.action", "admin", "audit-org", nil)

	// A non-owner user with their own token.
	outsider := seedTestUser(s, "audit-outsider")
	outTok := s.store.CreateToken(outsider.ID, "repo")

	// Anonymous → 404.
	if w := tokenRequest(s, "GET", "/api/v3/orgs/audit-org/audit-log", ""); w.Code != http.StatusNotFound {
		t.Errorf("anon audit-log = %d, want 404", w.Code)
	}
	// Non-owner → 404.
	if w := tokenRequest(s, "GET", "/api/v3/orgs/audit-org/audit-log", outTok.Value); w.Code != http.StatusNotFound {
		t.Errorf("non-owner audit-log = %d, want 404", w.Code)
	}
	// Owner (admin PAT) → 200.
	w := tokenRequest(s, "GET", "/api/v3/orgs/audit-org/audit-log", AdminToken())
	if w.Code != http.StatusOK {
		t.Fatalf("owner audit-log = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var entries []map[string]any
	json.Unmarshal(w.Body.Bytes(), &entries)
	if len(entries) != 1 {
		t.Errorf("owner audit-log entries = %d, want 1", len(entries))
	}
}

// TestListOrgMembersNonMemberSeesOnlyPublic guards the member-list scope: a
// non-member (or anonymous caller) of an org sees only publicized members,
// while an active member sees the full roster — matching GitHub's
// GET /orgs/{org}/members vs /public_members behaviour.
func TestListOrgMembersNonMemberSeesOnlyPublic(t *testing.T) {
	s := newTestServer()
	s.registerGHMemberRoutes()

	admin := s.store.LookupUserByLogin("admin")
	org := s.store.CreateOrg(admin, "mem-org", "Mem Org", "")
	if org == nil {
		t.Fatal("CreateOrg nil")
	}
	// admin is an admin member (private). Add a second private member and a
	// publicized member.
	priv := seedTestUser(s, "priv-member")
	s.store.SetMembership("mem-org", priv.ID, OrgRoleMember, MembershipStateActive)
	pub := seedTestUser(s, "pub-member")
	s.store.SetMembership("mem-org", pub.ID, OrgRoleMember, MembershipStateActive)
	if m := s.store.GetMembership("mem-org", pub.ID); m != nil {
		s.store.mu.Lock()
		m.Public = true
		s.store.mu.Unlock()
	}

	outsider := seedTestUser(s, "mem-outsider")
	outTok := s.store.CreateToken(outsider.ID, "read:org")

	// Non-member sees only the publicized member.
	w := tokenRequest(s, "GET", "/api/v3/orgs/mem-org/members", outTok.Value)
	if w.Code != http.StatusOK {
		t.Fatalf("non-member members = %d, want 200", w.Code)
	}
	var got []map[string]any
	json.Unmarshal(w.Body.Bytes(), &got)
	if len(got) != 1 || got[0]["login"] != "pub-member" {
		t.Errorf("non-member view = %v, want only [pub-member]", got)
	}

	// A member (admin) sees everyone.
	w = tokenRequest(s, "GET", "/api/v3/orgs/mem-org/members", AdminToken())
	json.Unmarshal(w.Body.Bytes(), &got)
	if len(got) != 3 {
		t.Errorf("member view count = %d, want 3", len(got))
	}
}

// TestOnJobCompletedIdempotent guards against the runner reporting a job's
// completion twice (DELETE /AgentRequest + POST /FinishJob): the second call
// must not flip the already-recorded conclusion or re-stamp CompletedAt.
func TestOnJobCompletedIdempotent(t *testing.T) {
	s := newTestServer()
	s.metrics = NewMetrics()
	wf, job := seedRun(t, s, "octo/repo", "running", "")

	// Put the job into a non-terminal state so the first completion is the
	// real terminal transition.
	s.store.mu.Lock()
	job.Status = JobStatusRunning
	jobID := job.JobID
	s.store.mu.Unlock()

	s.onJobCompleted(context.Background(), jobID, "Succeeded")
	s.store.mu.RLock()
	firstResult := job.Result
	firstCompleted := job.CompletedAt
	s.store.mu.RUnlock()
	if firstResult == "" {
		t.Fatal("first completion did not set result")
	}

	// Duplicate completion with a DIFFERENT result must be ignored.
	s.onJobCompleted(context.Background(), jobID, "Failed")
	s.store.mu.RLock()
	secondResult := job.Result
	secondCompleted := job.CompletedAt
	s.store.mu.RUnlock()
	if secondResult != firstResult {
		t.Errorf("duplicate completion flipped result %q -> %q", firstResult, secondResult)
	}
	if !secondCompleted.Equal(firstCompleted) {
		t.Errorf("duplicate completion changed CompletedAt %v -> %v", firstCompleted, secondCompleted)
	}
	_ = wf
}

// TestGraphQLRepositoryPrivateHidden guards the GraphQL repository(owner,name)
// resolver: a private repo the viewer can't read must surface as a NOT_FOUND
// error with null data, exactly like the REST handler, rather than leaking the
// repo's contents.
func TestGraphQLRepositoryPrivateHidden(t *testing.T) {
	owner := seedTestUser(testServer, "gqlpriv-owner")
	testServer.store.CreateRepo(owner, "secret-repo", "", true)

	outsider := seedTestUser(testServer, "gqlpriv-outsider")
	outTok := testServer.store.CreateToken(outsider.ID, "repo")

	query := fmt.Sprintf(`{repository(owner:"%s", name:"secret-repo"){name}}`, owner.Login)
	resp := ghPost(t, "/api/graphql", outTok.Value, map[string]string{"query": query})
	data := decodeJSON(t, resp)
	d, _ := data["data"].(map[string]interface{})
	if d != nil && d["repository"] != nil {
		t.Fatalf("private repo leaked to non-member: %v", d["repository"])
	}
	if data["errors"] == nil {
		t.Errorf("expected NOT_FOUND error, got none: %v", data)
	}

	// The owner can still read it.
	ownerTok := testServer.store.CreateToken(owner.ID, "repo")
	resp2 := ghPost(t, "/api/graphql", ownerTok.Value, map[string]string{"query": query})
	data2 := decodeJSON(t, resp2)
	d2, _ := data2["data"].(map[string]interface{})
	if d2 == nil || d2["repository"] == nil {
		t.Fatalf("owner could not read own private repo: %v", data2)
	}
}
