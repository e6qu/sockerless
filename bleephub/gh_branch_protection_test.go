package bleephub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func bpTestServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer()
	s.registerRoutes()
	admin := s.store.UsersByLogin["admin"]
	s.store.Tokens[adminPAT] = &Token{Value: adminPAT, UserID: admin.ID, Scopes: "repo,admin:org"}
	return s
}

func doBPReq(s *Server, token, method, path, body string) *httptest.ResponseRecorder {
	var bodyR *bytes.Reader
	if body != "" {
		bodyR = bytes.NewReader([]byte(body))
	} else {
		bodyR = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, bodyR)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.ghHeadersMiddleware(s.mux).ServeHTTP(w, req)
	return w
}

func TestBranchProtection_CRUD(t *testing.T) {
	s := bpTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "bp-crud", "", false)

	w := doBPReq(s, adminPAT, "GET", "/api/v3/repos/"+repo.FullName+"/branches/main/protection", "")
	require.Equal(t, http.StatusNotFound, w.Code)

	body := `{
		"required_status_checks": {"strict": true, "contexts": ["ci"]},
		"required_pull_request_reviews": {"required_approving_review_count": 2},
		"enforce_admins": true,
		"allow_force_pushes": true,
		"allow_deletions": false
	}`
	w = doBPReq(s, adminPAT, "PUT", "/api/v3/repos/"+repo.FullName+"/branches/main/protection", body)
	require.Equal(t, http.StatusOK, w.Code)

	var bp BranchProtection
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &bp))
	require.NotNil(t, bp.RequiredStatusChecks)
	require.True(t, bp.RequiredStatusChecks.Strict)
	require.Equal(t, []string{"ci"}, bp.RequiredStatusChecks.Contexts)
	require.NotNil(t, bp.RequiredPullRequestReviews)
	require.Equal(t, 2, bp.RequiredPullRequestReviews.RequiredApprovingReviewCount)
	require.NotNil(t, bp.EnforceAdmins)
	require.True(t, bp.EnforceAdmins.Enabled)
	require.NotNil(t, bp.AllowForcePushes)
	require.True(t, bp.AllowForcePushes.Enabled)
	require.Nil(t, bp.AllowDeletions)

	w = doBPReq(s, adminPAT, "GET", "/api/v3/repos/"+repo.FullName+"/branches/main/protection", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &bp))
	require.Equal(t, []string{"ci"}, bp.RequiredStatusChecks.Contexts)

	w = doBPReq(s, adminPAT, "DELETE", "/api/v3/repos/"+repo.FullName+"/branches/main/protection", "")
	require.Equal(t, http.StatusNoContent, w.Code)
	w = doBPReq(s, adminPAT, "GET", "/api/v3/repos/"+repo.FullName+"/branches/main/protection", "")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestBranchProtection_RequiredStatusChecksSubresource(t *testing.T) {
	s := bpTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "bp-rsc", "", false)

	doBPReq(s, adminPAT, "PUT", "/api/v3/repos/"+repo.FullName+"/branches/main/protection", `{}`)

	w := doBPReq(s, adminPAT, "PUT", "/api/v3/repos/"+repo.FullName+"/branches/main/protection/required_status_checks", `{"strict": true, "contexts": ["ci", "lint"]}`)
	require.Equal(t, http.StatusOK, w.Code)
	var sc BPStatusChecks
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sc))
	require.True(t, sc.Strict)
	require.Equal(t, []string{"ci", "lint"}, sc.Contexts)

	w = doBPReq(s, adminPAT, "GET", "/api/v3/repos/"+repo.FullName+"/branches/main/protection/required_status_checks", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sc))
	require.Equal(t, []string{"ci", "lint"}, sc.Contexts)

	w = doBPReq(s, adminPAT, "GET", "/api/v3/repos/"+repo.FullName+"/branches/main/protection/required_status_checks/contexts", "")
	require.Equal(t, http.StatusOK, w.Code)
	var contexts []string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &contexts))
	require.Equal(t, []string{"ci", "lint"}, contexts)

	w = doBPReq(s, adminPAT, "POST", "/api/v3/repos/"+repo.FullName+"/branches/main/protection/required_status_checks/contexts", `["build"]`)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &contexts))
	require.Equal(t, []string{"build"}, contexts)

	w = doBPReq(s, adminPAT, "DELETE", "/api/v3/repos/"+repo.FullName+"/branches/main/protection/required_status_checks", "")
	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestBranchProtection_RequiredReviewsSubresource(t *testing.T) {
	s := bpTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "bp-reviews", "", false)

	doBPReq(s, adminPAT, "PUT", "/api/v3/repos/"+repo.FullName+"/branches/main/protection", `{}`)

	w := doBPReq(s, adminPAT, "PATCH", "/api/v3/repos/"+repo.FullName+"/branches/main/protection/required_pull_request_reviews", `{"required_approving_review_count": 1, "dismiss_stale_reviews": true}`)
	require.Equal(t, http.StatusOK, w.Code)
	var rev BPPullRequestReviews
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rev))
	require.Equal(t, 1, rev.RequiredApprovingReviewCount)
	require.True(t, rev.DismissStaleReviews)

	w = doBPReq(s, adminPAT, "GET", "/api/v3/repos/"+repo.FullName+"/branches/main/protection/required_pull_request_reviews", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rev))
	require.Equal(t, 1, rev.RequiredApprovingReviewCount)
}

func TestBranchProtection_EnforceAdminsSubresource(t *testing.T) {
	s := bpTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "bp-admins", "", false)

	doBPReq(s, adminPAT, "PUT", "/api/v3/repos/"+repo.FullName+"/branches/main/protection", `{}`)

	w := doBPReq(s, adminPAT, "POST", "/api/v3/repos/"+repo.FullName+"/branches/main/protection/enforce_admins", ``)
	require.Equal(t, http.StatusOK, w.Code)
	var ea BPEnforceAdmins
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ea))
	require.True(t, ea.Enabled)

	w = doBPReq(s, adminPAT, "GET", "/api/v3/repos/"+repo.FullName+"/branches/main/protection/enforce_admins", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ea))
	require.True(t, ea.Enabled)

	w = doBPReq(s, adminPAT, "DELETE", "/api/v3/repos/"+repo.FullName+"/branches/main/protection/enforce_admins", "")
	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestBranchProtection_RestrictionsSubresource(t *testing.T) {
	s := bpTestServer(t)
	admin := s.store.UsersByLogin["admin"]
	repo := s.store.CreateRepo(admin, "bp-restrict", "", false)

	doBPReq(s, adminPAT, "PUT", "/api/v3/repos/"+repo.FullName+"/branches/main/protection", `{}`)

	body := `{"users":[{"login":"admin","id":1,"type":"User"}]}`
	w := doBPReq(s, adminPAT, "PUT", "/api/v3/repos/"+repo.FullName+"/branches/main/protection/restrictions", body)
	require.Equal(t, http.StatusOK, w.Code)
	var res BPRestrictions
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	require.Len(t, res.Users, 1)
	require.Equal(t, "admin", res.Users[0].Login)

	w = doBPReq(s, adminPAT, "GET", "/api/v3/repos/"+repo.FullName+"/branches/main/protection/restrictions/users", "")
	require.Equal(t, http.StatusOK, w.Code)
	var users []BPActor
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &users))
	require.Len(t, users, 1)

	w = doBPReq(s, adminPAT, "DELETE", "/api/v3/repos/"+repo.FullName+"/branches/main/protection/restrictions", "")
	require.Equal(t, http.StatusNoContent, w.Code)
}

func TestBranchProtection_MergeEnforcesRequiredReviews(t *testing.T) {
	createTestPRRepo(t, "bp-merge-reviews")
	defer func() { ghDelete(t, "/api/v3/repos/admin/bp-merge-reviews", defaultToken).Body.Close() }()

	ghPut(t, "/api/v3/repos/admin/bp-merge-reviews/branches/main/protection", defaultToken, map[string]interface{}{
		"required_pull_request_reviews": map[string]interface{}{"required_approving_review_count": 1},
		"enforce_admins":                true,
	}).Body.Close()

	ghPost(t, "/api/v3/repos/admin/bp-merge-reviews/pulls", defaultToken, map[string]interface{}{
		"title": "To merge", "head": "feat", "base": "main",
	}).Body.Close()

	resp := ghPut(t, "/api/v3/repos/admin/bp-merge-reviews/pulls/1/merge", defaultToken, map[string]interface{}{})
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	resp.Body.Close()

	resp = ghPost(t, "/api/v3/repos/admin/bp-merge-reviews/pulls/1/reviews", defaultToken, map[string]interface{}{
		"body": "LGTM", "event": "APPROVE",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp = ghPut(t, "/api/v3/repos/admin/bp-merge-reviews/pulls/1/merge", defaultToken, map[string]interface{}{})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

func TestBranchProtection_MergeEnforcesRequestedChanges(t *testing.T) {
	createTestPRRepo(t, "bp-merge-changes")
	defer func() { ghDelete(t, "/api/v3/repos/admin/bp-merge-changes", defaultToken).Body.Close() }()

	ghPut(t, "/api/v3/repos/admin/bp-merge-changes/branches/main/protection", defaultToken, map[string]interface{}{
		"enforce_admins": true,
	}).Body.Close()

	ghPost(t, "/api/v3/repos/admin/bp-merge-changes/pulls", defaultToken, map[string]interface{}{
		"title": "To merge", "head": "feat", "base": "main",
	}).Body.Close()

	reviewer := &User{ID: 1000, Login: "reviewer", Type: "User", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	testServer.store.Users[reviewer.ID] = reviewer
	testServer.store.UsersByLogin[reviewer.Login] = reviewer
	tok := testServer.store.CreateToken(reviewer.ID, "repo")

	resp := ghPost(t, "/api/v3/repos/admin/bp-merge-changes/pulls/1/reviews", tok.Value, map[string]interface{}{
		"body": "nope", "event": "REQUEST_CHANGES",
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp = ghPut(t, "/api/v3/repos/admin/bp-merge-changes/pulls/1/merge", defaultToken, map[string]interface{}{})
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	resp.Body.Close()
}
