package bleephub

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerGHCodespacesRoutes() {
	// User-scoped codespaces.
	s.route("GET /api/v3/user/codespaces", s.requirePerm(scopeCodespaces, permRead, s.handleListUserCodespaces))
	s.route("POST /api/v3/user/codespaces", s.requirePerm(scopeCodespaces, permWrite, s.handleCreateUserCodespace))
	s.route("GET /api/v3/user/codespaces/{codespace_name}", s.requirePerm(scopeCodespaces, permRead, s.handleGetUserCodespace))
	s.route("PATCH /api/v3/user/codespaces/{codespace_name}", s.requirePerm(scopeCodespaces, permWrite, s.handlePatchUserCodespace))
	s.route("DELETE /api/v3/user/codespaces/{codespace_name}", s.requirePerm(scopeCodespaces, permWrite, s.handleDeleteUserCodespace))
	s.route("POST /api/v3/user/codespaces/{codespace_name}/start", s.requirePerm(scopeCodespaces, permWrite, s.handleStartUserCodespace))
	s.route("POST /api/v3/user/codespaces/{codespace_name}/stop", s.requirePerm(scopeCodespaces, permWrite, s.handleStopUserCodespace))

	// Public-ish user-scoped list (matches real GitHub path shape).
	s.route("GET /api/v3/users/{username}/codespaces", s.handleListUserCodespacesByLogin)

	// Repository-scoped codespaces.
	s.route("GET /api/v3/repos/{owner}/{repo}/codespaces", s.requirePerm(scopeCodespaces, permRead, s.handleListRepoCodespaces))
	s.route("POST /api/v3/repos/{owner}/{repo}/codespaces", s.requirePerm(scopeCodespaces, permWrite, s.handleCreateRepoCodespace))
	s.route("GET /api/v3/repos/{owner}/{repo}/codespaces/{codespace_name}", s.requirePerm(scopeCodespaces, permRead, s.handleGetRepoCodespace))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/codespaces/{codespace_name}", s.requirePerm(scopeCodespaces, permWrite, s.handleDeleteRepoCodespace))
	s.route("POST /api/v3/repos/{owner}/{repo}/codespaces/{codespace_name}/start", s.requirePerm(scopeCodespaces, permWrite, s.handleStartRepoCodespace))
	s.route("POST /api/v3/repos/{owner}/{repo}/codespaces/{codespace_name}/stop", s.requirePerm(scopeCodespaces, permWrite, s.handleStopRepoCodespace))

	// Machine types.
	s.route("GET /api/v3/repos/{owner}/{repo}/codespaces/machines", s.requirePerm(scopeCodespaces, permRead, s.handleListCodespaceMachines))

	// Organization-member codespace administration: an org owner
	// operating on a member's codespaces on the org's repositories.
	s.route("GET /api/v3/orgs/{org}/members/{username}/codespaces", s.requireOrgAdminOrCodespaceScope(s.handleListOrgMemberCodespaces))
	s.route("DELETE /api/v3/orgs/{org}/members/{username}/codespaces/{codespace_name}", s.requireOrgAdminOrCodespaceScope(s.handleDeleteOrgMemberCodespace))
	s.route("POST /api/v3/orgs/{org}/members/{username}/codespaces/{codespace_name}/stop", s.requireOrgAdminOrCodespaceScope(s.handleStopOrgMemberCodespace))

	// User-scoped secrets.
	s.route("GET /api/v3/user/codespaces/secrets", s.requirePerm(scopeCodespaces, permRead, s.handleListUserCodespaceSecrets))
	s.route("GET /api/v3/user/codespaces/secrets/public-key", s.requirePerm(scopeCodespaces, permRead, s.handleGetCodespacePublicKey))
	s.route("GET /api/v3/user/codespaces/secrets/{secret_name}", s.requirePerm(scopeCodespaces, permRead, s.handleGetUserCodespaceSecret))
	s.route("PUT /api/v3/user/codespaces/secrets/{secret_name}", s.requirePerm(scopeCodespaces, permWrite, s.handlePutUserCodespaceSecret))
	s.route("DELETE /api/v3/user/codespaces/secrets/{secret_name}", s.requirePerm(scopeCodespaces, permWrite, s.handleDeleteUserCodespaceSecret))

	// User-secret selected repositories.
	s.route("GET /api/v3/user/codespaces/secrets/{secret_name}/repositories", s.requirePerm(scopeCodespaces, permRead, s.handleListUserCodespaceSecretRepos))
	s.route("PUT /api/v3/user/codespaces/secrets/{secret_name}/repositories", s.requirePerm(scopeCodespaces, permWrite, s.handleSetUserCodespaceSecretRepos))
	s.route("PUT /api/v3/user/codespaces/secrets/{secret_name}/repositories/{repository_id}", s.requirePerm(scopeCodespaces, permWrite, s.handleAddUserCodespaceSecretRepo))
	s.route("DELETE /api/v3/user/codespaces/secrets/{secret_name}/repositories/{repository_id}", s.requirePerm(scopeCodespaces, permWrite, s.handleRemoveUserCodespaceSecretRepo))

	// Per-codespace machines + export details. Go 1.22's ServeMux rejects
	// registering GET /user/codespaces/{codespace_name}/machines alongside
	// the literal GET /user/codespaces/secrets/{secret_name} (they overlap
	// at secrets/machines with neither more specific), so both GET shapes
	// dispatch through wildcards; the more-specific secrets routes above
	// still win for secrets/* paths.
	s.route("GET /api/v3/user/codespaces/{codespace_name}/{sub}", s.requirePerm(scopeCodespaces, permRead, s.handleUserCodespaceTwoSegGetDispatch))
	s.route("GET /api/v3/user/codespaces/{codespace_name}/{sub}/{export_id}", s.requirePerm(scopeCodespaces, permRead, s.handleUserCodespaceThreeSegGetDispatch))

	// Codespace exports + publish.
	s.route("POST /api/v3/user/codespaces/{codespace_name}/exports", s.requirePerm(scopeCodespaces, permWrite, s.handleExportUserCodespace))
	s.route("POST /api/v3/user/codespaces/{codespace_name}/publish", s.requirePerm(scopeCodespaces, permWrite, s.handlePublishUserCodespace))

	// Pull-request codespaces.
	s.route("POST /api/v3/repos/{owner}/{repo}/pulls/{pull_number}/codespaces", s.requirePerm(scopeCodespaces, permWrite, s.handleCreatePullRequestCodespace))

	// Repository-scoped secrets.
	s.route("GET /api/v3/repos/{owner}/{repo}/codespaces/secrets", s.requirePerm(scopeCodespaces, permRead, s.handleListRepoCodespaceSecrets))
	s.route("GET /api/v3/repos/{owner}/{repo}/codespaces/secrets/public-key", s.requirePerm(scopeCodespaces, permRead, s.handleGetCodespacePublicKey))
	s.route("GET /api/v3/repos/{owner}/{repo}/codespaces/secrets/{secret_name}", s.requirePerm(scopeCodespaces, permRead, s.handleGetRepoCodespaceSecret))
	s.route("PUT /api/v3/repos/{owner}/{repo}/codespaces/secrets/{secret_name}", s.requirePerm(scopeCodespaces, permWrite, s.handlePutRepoCodespaceSecret))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/codespaces/secrets/{secret_name}", s.requirePerm(scopeCodespaces, permWrite, s.handleDeleteRepoCodespaceSecret))

	// Organization-scoped codespaces + access controls.
	s.route("GET /api/v3/orgs/{org}/codespaces", s.requireOrgAdminOrCodespaceScope(s.handleListOrgCodespaces))
	s.route("PUT /api/v3/orgs/{org}/codespaces/access", s.requireOrgAdminOrCodespaceScope(s.handleSetOrgCodespacesAccess))
	s.route("POST /api/v3/orgs/{org}/codespaces/access/selected_users", s.requireOrgAdminOrCodespaceScope(s.handleAddOrgCodespacesAccessUsers))
	s.route("DELETE /api/v3/orgs/{org}/codespaces/access/selected_users", s.requireOrgAdminOrCodespaceScope(s.handleRemoveOrgCodespacesAccessUsers))

	// Organization-scoped secrets.
	s.route("GET /api/v3/orgs/{org}/codespaces/secrets", s.requireOrgAdminOrCodespaceScope(s.handleListOrgCodespaceSecrets))
	s.route("GET /api/v3/orgs/{org}/codespaces/secrets/public-key", s.requireOrgAdminOrCodespaceScope(s.handleGetCodespacePublicKey))
	s.route("GET /api/v3/orgs/{org}/codespaces/secrets/{secret_name}", s.requireOrgAdminOrCodespaceScope(s.handleGetOrgCodespaceSecret))
	s.route("PUT /api/v3/orgs/{org}/codespaces/secrets/{secret_name}", s.requireOrgAdminOrCodespaceScope(s.handlePutOrgCodespaceSecret))
	s.route("DELETE /api/v3/orgs/{org}/codespaces/secrets/{secret_name}", s.requireOrgAdminOrCodespaceScope(s.handleDeleteOrgCodespaceSecret))
	s.route("GET /api/v3/orgs/{org}/codespaces/secrets/{secret_name}/repositories", s.requireOrgAdminOrCodespaceScope(s.handleListOrgCodespaceSecretRepos))
	s.route("PUT /api/v3/orgs/{org}/codespaces/secrets/{secret_name}/repositories", s.requireOrgAdminOrCodespaceScope(s.handleSetOrgCodespaceSecretRepos))
	s.route("PUT /api/v3/orgs/{org}/codespaces/secrets/{secret_name}/repositories/{repository_id}", s.requireOrgAdminOrCodespaceScope(s.handleAddOrgCodespaceSecretRepo))
	s.route("DELETE /api/v3/orgs/{org}/codespaces/secrets/{secret_name}/repositories/{repository_id}", s.requireOrgAdminOrCodespaceScope(s.handleRemoveOrgCodespaceSecretRepo))
}

// --- auth helpers ---

func (s *Server) requireOrgAdminOrCodespaceScope(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := ghUserFromContext(r.Context())
		if user == nil {
			writeGHError(w, http.StatusUnauthorized, "Requires authentication")
			return
		}
		org := s.store.GetOrg(r.PathValue("org"))
		if org == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		if !canAdminOrg(s.store, user, org) {
			writeGHError(w, http.StatusForbidden, "Must have admin rights to Organization.")
			return
		}
		next(w, r)
	}
}

func (s *Server) resolveCodespace(w http.ResponseWriter, r *http.Request, ownerLogin, repoKey string) *Codespace {
	name := r.PathValue("codespace_name")
	cs := s.store.GetCodespaceByName(name)
	if cs == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	if ownerLogin != "" && cs.OwnerLogin != ownerLogin {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	if repoKey != "" && cs.RepoKey != repoKey {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return cs
}

// --- user codespace handlers ---

func (s *Server) handleListUserCodespaces(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	list := s.store.ListCodespacesByOwner(user.Login)
	out := make([]map[string]interface{}, len(list))
	for i, cs := range list {
		out[i] = s.codespaceToJSON(cs, s.baseURL(r))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"codespaces": out, "total_count": len(out)})
}

func (s *Server) handleListUserCodespacesByLogin(w http.ResponseWriter, r *http.Request) {
	login := r.PathValue("username")
	list := s.store.ListCodespacesByOwner(login)
	out := make([]map[string]interface{}, len(list))
	for i, cs := range list {
		out[i] = s.codespaceToJSON(cs, s.baseURL(r))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"codespaces": out, "total_count": len(out)})
}

func (s *Server) handleCreateUserCodespace(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	var req codespaceCreateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	repoKey := ""
	if req.RepositoryID > 0 {
		repo := s.store.GetRepoByID(req.RepositoryID)
		if repo == nil {
			writeGHValidationError(w, "Codespace", "repository_id", "invalid")
			return
		}
		repoKey = repo.FullName
	}
	cs, err := s.store.CreateCodespace(user.Login, repoKey, req.Ref, req.Machine, req.DisplayName)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "codespace create failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, s.codespaceToJSON(cs, s.baseURL(r)))
}

func (s *Server) handleGetUserCodespace(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	cs := s.resolveCodespace(w, r, user.Login, "")
	if cs == nil {
		return
	}
	_ = s.store.RefreshCodespaceState(cs.ID)
	writeJSON(w, http.StatusOK, s.codespaceToJSON(cs, s.baseURL(r)))
}

func (s *Server) handlePatchUserCodespace(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	cs := s.resolveCodespace(w, r, user.Login, "")
	if cs == nil {
		return
	}
	var req codespacePatchRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	cs, ok := s.store.UpdateCodespace(cs.ID, req.DisplayName, req.Machine, req.RetentionPeriodMinutes)
	if !ok {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	_ = s.store.RefreshCodespaceState(cs.ID)
	writeJSON(w, http.StatusOK, s.codespaceToJSON(cs, s.baseURL(r)))
}

func (s *Server) handleDeleteUserCodespace(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	cs := s.resolveCodespace(w, r, user.Login, "")
	if cs == nil {
		return
	}
	ok, err := s.store.DeleteCodespace(cs.ID)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "codespace delete failed: "+err.Error())
		return
	}
	if !ok {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleStartUserCodespace(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	cs := s.resolveCodespace(w, r, user.Login, "")
	if cs == nil {
		return
	}
	if err := s.startCodespace(cs); err != nil {
		writeGHError(w, http.StatusInternalServerError, "codespace start failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, s.codespaceToJSON(cs, s.baseURL(r)))
}

func (s *Server) handleStopUserCodespace(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	cs := s.resolveCodespace(w, r, user.Login, "")
	if cs == nil {
		return
	}
	if err := s.stopCodespace(cs); err != nil {
		writeGHError(w, http.StatusInternalServerError, "codespace stop failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, s.codespaceToJSON(cs, s.baseURL(r)))
}

// --- repo codespace handlers ---

func (s *Server) handleListRepoCodespaces(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	list := s.store.ListCodespacesByRepo(repo.FullName)
	out := make([]map[string]interface{}, len(list))
	for i, cs := range list {
		out[i] = s.codespaceToJSON(cs, s.baseURL(r))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"codespaces": out, "total_count": len(out)})
}

func (s *Server) handleCreateRepoCodespace(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req codespaceCreateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.RepositoryID == 0 {
		req.RepositoryID = repo.ID
	} else if req.RepositoryID != repo.ID {
		writeGHValidationError(w, "Codespace", "repository_id", "invalid")
		return
	}
	cs, err := s.store.CreateCodespace(user.Login, repo.FullName, req.Ref, req.Machine, req.DisplayName)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "codespace create failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, s.codespaceToJSON(cs, s.baseURL(r)))
}

func (s *Server) handleGetRepoCodespace(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	cs := s.resolveCodespace(w, r, "", repo.FullName)
	if cs == nil {
		return
	}
	_ = s.store.RefreshCodespaceState(cs.ID)
	writeJSON(w, http.StatusOK, s.codespaceToJSON(cs, s.baseURL(r)))
}

func (s *Server) handleDeleteRepoCodespace(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	cs := s.resolveCodespace(w, r, "", repo.FullName)
	if cs == nil {
		return
	}
	ok, err := s.store.DeleteCodespace(cs.ID)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "codespace delete failed: "+err.Error())
		return
	}
	if !ok {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleStartRepoCodespace(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	cs := s.resolveCodespace(w, r, "", repo.FullName)
	if cs == nil {
		return
	}
	if err := s.startCodespace(cs); err != nil {
		writeGHError(w, http.StatusInternalServerError, "codespace start failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, s.codespaceToJSON(cs, s.baseURL(r)))
}

func (s *Server) handleStopRepoCodespace(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	cs := s.resolveCodespace(w, r, "", repo.FullName)
	if cs == nil {
		return
	}
	if err := s.stopCodespace(cs); err != nil {
		writeGHError(w, http.StatusInternalServerError, "codespace stop failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, s.codespaceToJSON(cs, s.baseURL(r)))
}

// --- machines ---

// codespaceMachineJSON renders one catalog machine in GitHub's
// codespace-machine schema. bleephub has no prebuild pipeline, so
// prebuild availability is "none".
func codespaceMachineJSON(m codespaceMachine) map[string]interface{} {
	return map[string]interface{}{
		"name":                  m.Name,
		"display_name":          m.DisplayName,
		"operating_system":      "linux",
		"storage_in_bytes":      m.StorageBytes,
		"memory_in_bytes":       m.MemoryBytes,
		"cpus":                  m.CPUs,
		"prebuild_availability": "none",
	}
}

func codespaceMachinesListJSON() map[string]interface{} {
	machines := make([]map[string]interface{}, len(codespaceMachines))
	for i, m := range codespaceMachines {
		machines[i] = codespaceMachineJSON(m)
	}
	return map[string]interface{}{"machines": machines, "total_count": len(machines)}
}

func (s *Server) handleListCodespaceMachines(w http.ResponseWriter, r *http.Request) {
	if s.lookupRepoFromPath(r) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, codespaceMachinesListJSON())
}

// handleUserCodespaceTwoSegGetDispatch fans out
// GET /user/codespaces/{codespace_name}/{sub} to the real GitHub
// sub-resource: machines.
func (s *Server) handleUserCodespaceTwoSegGetDispatch(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("sub") != "machines" {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	user := ghUserFromContext(r.Context())
	cs := s.resolveCodespace(w, r, user.Login, "")
	if cs == nil {
		return
	}
	writeJSON(w, http.StatusOK, codespaceMachinesListJSON())
}

// handleUserCodespaceThreeSegGetDispatch fans out
// GET /user/codespaces/{codespace_name}/{sub}/{export_id} to the real
// GitHub sub-resource: exports/{export_id}.
func (s *Server) handleUserCodespaceThreeSegGetDispatch(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("sub") != "exports" {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	user := ghUserFromContext(r.Context())
	cs := s.resolveCodespace(w, r, user.Login, "")
	if cs == nil {
		return
	}
	if cs.LatestExport == nil || r.PathValue("export_id") != cs.LatestExport.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.codespaceExportJSON(cs, cs.LatestExport, s.baseURL(r)))
}

// --- exports + publish ---

func (s *Server) codespaceExportJSON(cs *Codespace, export *CodespaceExport, baseURL string) map[string]interface{} {
	out := map[string]interface{}{
		"state":        export.State,
		"completed_at": export.CompletedAt.UTC().Format(time.RFC3339),
		"branch":       export.Branch,
		"sha":          export.SHA,
		"id":           export.ID,
		"export_url":   fmt.Sprintf("%s/api/v3/user/codespaces/%s/exports/%s", baseURL, cs.Name, export.ID),
	}
	if cs.RepoKey != "" {
		out["html_url"] = fmt.Sprintf("%s/%s/tree/%s", baseURL, cs.RepoKey, export.Branch)
	} else {
		out["html_url"] = nil
	}
	return out
}

func (s *Server) handleExportUserCodespace(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	cs := s.resolveCodespace(w, r, user.Login, "")
	if cs == nil {
		return
	}
	export, err := s.store.ExportCodespace(cs.ID)
	switch {
	case err == errCodespaceNoRepository:
		writeGHValidationError(w, "CodespaceExport", "repository", "missing")
		return
	case err != nil:
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, s.codespaceExportJSON(cs, export, s.baseURL(r)))
}

func (s *Server) handlePublishUserCodespace(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	cs := s.resolveCodespace(w, r, user.Login, "")
	if cs == nil {
		return
	}
	var req struct {
		Name    string `json:"name"`
		Private bool   `json:"private"`
	}
	if !decodeJSONBodyOptional(w, r, &req) {
		return
	}
	published, err := s.store.PublishCodespace(cs.ID, user, req.Name, req.Private)
	switch err {
	case nil:
	case errCodespacePublished:
		writeGHValidationError(w, "Codespace", "codespace", "already_exists")
		return
	case errRepoNameTaken:
		writeGHValidationError(w, "Repository", "name", "already_exists")
		return
	default:
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	baseURL := s.baseURL(r)
	out := s.codespaceToJSON(published, baseURL)
	// codespace-with-full-repository embeds the full-repository shape.
	if repo := s.store.GetRepoByFullName(published.RepoKey); repo != nil {
		out["repository"] = fullRepoJSON(repo, s.store, baseURL)
	}
	writeJSON(w, http.StatusCreated, out)
}

// --- pull-request codespaces ---

func (s *Server) handleCreatePullRequestCodespace(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	num, err := strconv.Atoi(r.PathValue("pull_number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	pr := s.store.GetPullRequestByNumber(repo.ID, num)
	if pr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req codespaceCreateRequest
	if !decodeJSONBodyOptional(w, r, &req) {
		return
	}
	cs, err := s.store.CreateCodespace(user.Login, repo.FullName, pr.HeadRefName, req.Machine, req.DisplayName)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "codespace create failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, s.codespaceToJSON(cs, s.baseURL(r)))
}

// --- user-secret selected repositories ---

func (s *Server) handleListUserCodespaceSecretRepos(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	sec := s.getCodespaceSecret(r, "user", user.Login)
	if sec == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	baseURL := s.baseURL(r)
	repos := make([]map[string]interface{}, 0, len(sec.SelectedRepoIDs))
	for _, id := range sec.SelectedRepoIDs {
		if repo := s.store.GetRepoByID(id); repo != nil {
			repos = append(repos, minimalRepoJSON(repo, s.store, baseURL))
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"repositories": repos, "total_count": len(repos)})
}

func (s *Server) handleSetUserCodespaceSecretRepos(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	var req struct {
		SelectedRepositoryIDs []int `json:"selected_repository_ids"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	for _, id := range req.SelectedRepositoryIDs {
		if s.store.GetRepoByID(id) == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
	}
	scope := codespaceSecretScopeKey("user", user.Login)
	if !s.store.SetCodespaceSecretSelectedRepos(scope, r.PathValue("secret_name"), req.SelectedRepositoryIDs) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddUserCodespaceSecretRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	repoID, err := strconv.Atoi(r.PathValue("repository_id"))
	if err != nil || s.store.GetRepoByID(repoID) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	scope := codespaceSecretScopeKey("user", user.Login)
	if !s.store.AddCodespaceSecretSelectedRepo(scope, r.PathValue("secret_name"), repoID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveUserCodespaceSecretRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	repoID, err := strconv.Atoi(r.PathValue("repository_id"))
	if err != nil || s.store.GetRepoByID(repoID) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	scope := codespaceSecretScopeKey("user", user.Login)
	if !s.store.RemoveCodespaceSecretSelectedRepo(scope, r.PathValue("secret_name"), repoID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- organization-member codespace handlers ---

// resolveOrgMemberForCodespaces resolves the {org}/{username} pair for
// the org-member codespace endpoints: the user must exist and hold an
// active membership in the org. The caller was already vetted as an org
// owner by requireOrgAdminOrCodespaceScope.
func (s *Server) resolveOrgMemberForCodespaces(w http.ResponseWriter, r *http.Request) (*Org, *User) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	member := s.store.LookupUserByLogin(r.PathValue("username"))
	if member == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	m := s.store.GetMembership(org.Login, member.ID)
	if m == nil || m.State != MembershipStateActive {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	return org, member
}

// orgScopedCodespaceJSON renders a codespace with exactly the members
// the documented codespace schema carries — the org-member operations
// have no allowlisted extras.
func (s *Server) orgScopedCodespaceJSON(cs *Codespace, baseURL string) map[string]interface{} {
	j := s.codespaceToJSON(cs, baseURL)
	delete(j, "html_url")
	delete(j, "billing_url")
	return j
}

// handleListOrgMemberCodespaces — GET /api/v3/orgs/{org}/members/{username}/codespaces:
// the member's codespaces on the organization's repositories.
func (s *Server) handleListOrgMemberCodespaces(w http.ResponseWriter, r *http.Request) {
	org, member := s.resolveOrgMemberForCodespaces(w, r)
	if org == nil {
		return
	}
	prefix := org.Login + "/"
	base := s.baseURL(r)
	out := []map[string]interface{}{}
	for _, cs := range s.store.ListCodespacesByOwner(member.Login) {
		if !strings.HasPrefix(cs.RepoKey, prefix) {
			continue
		}
		out = append(out, s.orgScopedCodespaceJSON(cs, base))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"codespaces": out, "total_count": len(out)})
}

// resolveOrgMemberCodespace resolves {codespace_name} to a codespace the
// member owns on one of the organization's repositories.
func (s *Server) resolveOrgMemberCodespace(w http.ResponseWriter, r *http.Request, org *Org, member *User) *Codespace {
	cs := s.store.GetCodespaceByName(r.PathValue("codespace_name"))
	if cs == nil || cs.OwnerLogin != member.Login || !strings.HasPrefix(cs.RepoKey, org.Login+"/") {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return cs
}

// handleDeleteOrgMemberCodespace — DELETE /api/v3/orgs/{org}/members/{username}/codespaces/{codespace_name}.
func (s *Server) handleDeleteOrgMemberCodespace(w http.ResponseWriter, r *http.Request) {
	org, member := s.resolveOrgMemberForCodespaces(w, r)
	if org == nil {
		return
	}
	cs := s.resolveOrgMemberCodespace(w, r, org, member)
	if cs == nil {
		return
	}
	ok, err := s.store.DeleteCodespace(cs.ID)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "codespace delete failed: "+err.Error())
		return
	}
	if !ok {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// handleStopOrgMemberCodespace — POST /api/v3/orgs/{org}/members/{username}/codespaces/{codespace_name}/stop.
// Unlike the user-scoped stop (202), the org-member stop answers 200
// with the codespace, per the documented operation.
func (s *Server) handleStopOrgMemberCodespace(w http.ResponseWriter, r *http.Request) {
	org, member := s.resolveOrgMemberForCodespaces(w, r)
	if org == nil {
		return
	}
	cs := s.resolveOrgMemberCodespace(w, r, org, member)
	if cs == nil {
		return
	}
	if err := s.stopCodespace(cs); err != nil {
		writeGHError(w, http.StatusInternalServerError, "codespace stop failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.orgScopedCodespaceJSON(cs, s.baseURL(r)))
}

// --- secrets handlers ---

func (s *Server) handleListUserCodespaceSecrets(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	scope := codespaceSecretScopeKey("user", user.Login)
	secs := s.store.ListCodespaceSecrets(scope)
	writeJSON(w, http.StatusOK, codespaceSecretsListJSON(secs, s.baseURL(r)))
}

func (s *Server) handleGetUserCodespaceSecret(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	sec := s.getCodespaceSecret(r, "user", user.Login)
	if sec == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, codespaceUserSecretJSON(sec, s.baseURL(r)))
}

func (s *Server) handlePutUserCodespaceSecret(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	name := r.PathValue("secret_name")
	value, ok := s.readSealedCodespaceSecret(w, r)
	if !ok {
		return
	}
	s.store.CreateCodespaceSecret(codespaceSecretScopeKey("user", user.Login), name, value, "", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteUserCodespaceSecret(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if !s.store.DeleteCodespaceSecret(codespaceSecretScopeKey("user", user.Login), r.PathValue("secret_name")) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListRepoCodespaceSecrets(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	scope := codespaceSecretScopeKey("repo", repo.FullName)
	secs := s.store.ListCodespaceSecrets(scope)
	out := make([]map[string]interface{}, len(secs))
	for i, sec := range secs {
		out[i] = codespaceRepoSecretJSON(sec)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"secrets": out, "total_count": len(out)})
}

func (s *Server) handleGetRepoCodespaceSecret(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	sec := s.getCodespaceSecret(r, "repo", repo.FullName)
	if sec == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, codespaceRepoSecretJSON(sec))
}

func (s *Server) handlePutRepoCodespaceSecret(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	name := r.PathValue("secret_name")
	value, ok := s.readSealedCodespaceSecret(w, r)
	if !ok {
		return
	}
	s.store.CreateCodespaceSecret(codespaceSecretScopeKey("repo", repo.FullName), name, value, "", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteRepoCodespaceSecret(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.DeleteCodespaceSecret(codespaceSecretScopeKey("repo", repo.FullName), r.PathValue("secret_name")) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListOrgCodespaceSecrets(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	scope := codespaceSecretScopeKey("org", org)
	secs := s.store.ListCodespaceSecrets(scope)
	out := make([]map[string]interface{}, len(secs))
	for i, sec := range secs {
		out[i] = codespaceOrgSecretJSON(sec, org, s.baseURL(r))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"secrets": out, "total_count": len(out)})
}

func (s *Server) handleGetOrgCodespaceSecret(w http.ResponseWriter, r *http.Request) {
	sec := s.getCodespaceSecret(r, "org", r.PathValue("org"))
	if sec == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, codespaceOrgSecretJSON(sec, r.PathValue("org"), s.baseURL(r)))
}

func (s *Server) handlePutOrgCodespaceSecret(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	name := r.PathValue("secret_name")
	var req struct {
		EncryptedValue        string `json:"encrypted_value"`
		KeyID                 string `json:"key_id"`
		Visibility            string `json:"visibility"`
		SelectedRepositoryIDs []int  `json:"selected_repository_ids"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	plain, ok := s.decryptSealedSecret(w, req.EncryptedValue, req.KeyID)
	if !ok {
		return
	}
	if req.Visibility == "" {
		if len(req.SelectedRepositoryIDs) > 0 {
			req.Visibility = "selected"
		} else {
			req.Visibility = "all"
		}
	}
	s.store.CreateCodespaceSecret(codespaceSecretScopeKey("org", org), name, plain, req.Visibility, req.SelectedRepositoryIDs)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteOrgCodespaceSecret(w http.ResponseWriter, r *http.Request) {
	if !s.store.DeleteCodespaceSecret(codespaceSecretScopeKey("org", r.PathValue("org")), r.PathValue("secret_name")) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListOrgCodespaceSecretRepos(w http.ResponseWriter, r *http.Request) {
	sec := s.getCodespaceSecret(r, "org", r.PathValue("org"))
	if sec == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	repos := make([]map[string]interface{}, 0, len(sec.SelectedRepoIDs))
	for _, id := range sec.SelectedRepoIDs {
		if repo := s.store.GetRepoByID(id); repo != nil {
			repos = append(repos, repoToJSON(repo, s.store, s.baseURL(r)))
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"repositories": repos, "total_count": len(repos)})
}

func (s *Server) handleSetOrgCodespaceSecretRepos(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	name := r.PathValue("secret_name")
	var req struct {
		SelectedRepositoryIDs []int `json:"selected_repository_ids"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !s.store.SetCodespaceSecretSelectedRepos(codespaceSecretScopeKey("org", org), name, req.SelectedRepositoryIDs) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetCodespacePublicKey(w http.ResponseWriter, r *http.Request) {
	s.writeActionsPublicKey(w)
}

func (s *Server) getCodespaceSecret(r *http.Request, kind, key string) *CodespaceSecret {
	return s.store.GetCodespaceSecret(codespaceSecretScopeKey(kind, key), r.PathValue("secret_name"))
}

func (s *Server) readSealedCodespaceSecret(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req sealedSecretBody
	if !decodeJSONBody(w, r, &req) {
		return "", false
	}
	return s.decryptSealedSecret(w, req.EncryptedValue, req.KeyID)
}

// --- lifecycle helpers ---

func (s *Server) startCodespace(cs *Codespace) error {
	if cs.ContainerID == "" {
		return fmt.Errorf("no container")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := dockerStartContainer(ctx, cs.ContainerID); err != nil {
		return err
	}
	s.store.SetCodespaceState(cs.ID, dockerStateToCodespaceState(cs.ContainerID), true)
	return nil
}

func (s *Server) stopCodespace(cs *Codespace) error {
	if cs.ContainerID == "" {
		return fmt.Errorf("no container")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := dockerStopContainer(ctx, cs.ContainerID); err != nil {
		return err
	}
	s.store.SetCodespaceState(cs.ID, dockerStateToCodespaceState(cs.ContainerID), false)
	return nil
}

// --- request/response shapes ---

type codespaceCreateRequest struct {
	RepositoryID int    `json:"repository_id"`
	Ref          string `json:"ref"`
	Machine      string `json:"machine"`
	DisplayName  string `json:"display_name"`
	Location     string `json:"location"`
}

type codespacePatchRequest struct {
	DisplayName            string `json:"display_name"`
	Machine                string `json:"machine"`
	RetentionPeriodMinutes int    `json:"retention_period_minutes"`
}

func (s *Server) codespaceToJSON(cs *Codespace, baseURL string) map[string]interface{} {
	owner := s.store.LookupUserByLogin(cs.OwnerLogin)
	ownerJSON := map[string]interface{}(nil)
	if owner != nil {
		ownerJSON = userToJSON(owner)
	}
	var repoJSON map[string]interface{}
	if cs.RepoKey != "" {
		if owner, repoName, ok := splitRepoFullName(cs.RepoKey); ok {
			if repo := s.store.GetRepo(owner, repoName); repo != nil {
				repoJSON = repoToJSON(repo, s.store, baseURL)
			}
		}
	}

	url := fmt.Sprintf("%s/api/v3/user/codespaces/%s", baseURL, cs.Name)
	return map[string]interface{}{
		"id":                       cs.ID,
		"name":                     cs.Name,
		"display_name":             cs.DisplayName,
		"environment_id":           fmt.Sprintf("%d", cs.ID),
		"owner":                    ownerJSON,
		"billable_owner":           ownerJSON,
		"repository":               repoJSON,
		"machine":                  codespaceMachineJSON(codespaceMachineByName(cs.MachineName)),
		"created_at":               cs.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":               cs.UpdatedAt.UTC().Format(time.RFC3339),
		"last_used_at":             cs.LastUsedAt.UTC().Format(time.RFC3339),
		"state":                    cs.State,
		"url":                      url,
		"html_url":                 fmt.Sprintf("%s/codespaces/%s", baseURL, cs.Name),
		"web_url":                  fmt.Sprintf("%s/codespaces/%s/web", baseURL, cs.Name),
		"billing_url":              fmt.Sprintf("%s/settings/billing", baseURL),
		"git_status":               map[string]interface{}{"ahead": 0, "behind": 0, "has_uncommitted_changes": false, "ref": cs.GitRef},
		"devcontainer_path":        cs.DevcontainerPath,
		"retention_period_minutes": cs.RetentionPeriodMinutes,
		"idle_timeout_minutes":     30,
		"location":                 "local",
		"machines_url":             url + "/machines",
		"prebuild":                 false,
		"pulls_url":                url + "/pulls",
		"recent_folders":           []string{},
		"start_url":                url + "/start",
		"stop_url":                 url + "/stop",
	}
}

func codespaceUserSecretJSON(sec *CodespaceSecret, baseURL string) map[string]interface{} {
	visibility := sec.Visibility
	if visibility == "" {
		visibility = "all"
	}
	return map[string]interface{}{
		"name":                      sec.Name,
		"created_at":                sec.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":                sec.UpdatedAt.UTC().Format(time.RFC3339),
		"visibility":                visibility,
		"selected_repositories_url": baseURL + "/api/v3/user/codespaces/secrets/" + sec.Name + "/repositories",
	}
}

func codespaceRepoSecretJSON(sec *CodespaceSecret) map[string]interface{} {
	return map[string]interface{}{
		"name":       sec.Name,
		"created_at": sec.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": sec.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func codespaceOrgSecretJSON(sec *CodespaceSecret, orgLogin, baseURL string) map[string]interface{} {
	out := codespaceUserSecretJSON(sec, baseURL)
	out["selected_repositories_url"] = baseURL + "/api/v3/orgs/" + orgLogin + "/codespaces/secrets/" + sec.Name + "/repositories"
	return out
}

func codespaceSecretsListJSON(secs []*CodespaceSecret, baseURL string) map[string]interface{} {
	out := make([]map[string]interface{}, len(secs))
	for i, sec := range secs {
		out[i] = codespaceUserSecretJSON(sec, baseURL)
	}
	return map[string]interface{}{"secrets": out, "total_count": len(out)}
}

// ─── organization codespaces + access controls ───────────────────────────

// OrgCodespacesAccess records which organization users can create codespaces
// billed to the organization.
type OrgCodespacesAccess struct {
	Visibility        string   `json:"visibility"` // disabled | selected_members | all_members | all_members_and_outside_collaborators
	SelectedUsernames []string `json:"selected_usernames,omitempty"`
}

var orgCodespacesAccessVisibilities = map[string]bool{
	"disabled":                              true,
	"selected_members":                      true,
	"all_members":                           true,
	"all_members_and_outside_collaborators": true,
}

// SetOrgCodespacesAccess replaces the org's codespaces access settings.
func (st *Store) SetOrgCodespacesAccess(orgLogin, visibility string, selected []string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.OrgCodespacesAccess[orgLogin] = &OrgCodespacesAccess{
		Visibility:        visibility,
		SelectedUsernames: selected,
	}
	if st.persist != nil {
		st.persist.MustPut("org_codespaces_access", orgLogin, st.OrgCodespacesAccess[orgLogin])
	}
}

// ModifyOrgCodespacesAccessUsers adds or removes usernames from the org's
// selected-members codespaces access list. Returns false when the org's
// access visibility is not selected_members.
func (st *Store) ModifyOrgCodespacesAccessUsers(orgLogin string, add bool, usernames []string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	access := st.OrgCodespacesAccess[orgLogin]
	if access == nil || access.Visibility != "selected_members" {
		return false
	}
	if add {
		present := map[string]bool{}
		for _, u := range access.SelectedUsernames {
			present[strings.ToLower(u)] = true
		}
		for _, u := range usernames {
			if !present[strings.ToLower(u)] {
				access.SelectedUsernames = append(access.SelectedUsernames, u)
				present[strings.ToLower(u)] = true
			}
		}
	} else {
		remove := map[string]bool{}
		for _, u := range usernames {
			remove[strings.ToLower(u)] = true
		}
		kept := access.SelectedUsernames[:0]
		for _, u := range access.SelectedUsernames {
			if !remove[strings.ToLower(u)] {
				kept = append(kept, u)
			}
		}
		access.SelectedUsernames = kept
	}
	if st.persist != nil {
		st.persist.MustPut("org_codespaces_access", orgLogin, access)
	}
	return true
}

// orgCodespacesInvalidUsers returns the usernames that are neither active
// organization members nor collaborators on any of the org's repositories.
func (st *Store) orgCodespacesInvalidUsers(org *Org, usernames []string) []string {
	st.mu.RLock()
	defer st.mu.RUnlock()

	collaborators := map[string]bool{}
	for repoKey, byLogin := range st.RepoCollaborators {
		repo := st.ReposByName[repoKey]
		if repo == nil || repo.OwnerType != "Organization" || repo.OwnerID != org.ID {
			continue
		}
		for login := range byLogin {
			collaborators[strings.ToLower(login)] = true
		}
	}

	var invalid []string
	for _, username := range usernames {
		u := st.UsersByLogin[username]
		if u != nil {
			m := st.Memberships[membershipKey(org.Login, u.ID)]
			if m != nil && m.State == MembershipStateActive {
				continue
			}
		}
		if collaborators[strings.ToLower(username)] {
			continue
		}
		invalid = append(invalid, username)
	}
	return invalid
}

func (s *Server) handleListOrgCodespaces(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	// Gather the org members' codespaces under the read lock; render
	// outside it (codespaceToJSON takes store locks itself).
	s.store.mu.RLock()
	memberLogins := map[string]bool{}
	for _, m := range s.store.Memberships {
		if m.OrgID == org.ID && m.State == MembershipStateActive {
			if u := s.store.Users[m.UserID]; u != nil {
				memberLogins[u.Login] = true
			}
		}
	}
	var list []*Codespace
	for _, cs := range s.store.Codespaces {
		if memberLogins[cs.OwnerLogin] {
			list = append(list, cs)
		}
	}
	s.store.mu.RUnlock()
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })

	base := s.baseURL(r)
	out := make([]map[string]interface{}, len(list))
	for i, cs := range list {
		out[i] = s.codespaceToJSON(cs, base)
		// The vendored OpenAPI description's codespace schema does not
		// declare html_url/billing_url, and this endpoint emits exactly the
		// documented members.
		delete(out[i], "html_url")
		delete(out[i], "billing_url")
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"codespaces": out, "total_count": len(out)})
}

func (s *Server) handleSetOrgCodespacesAccess(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Visibility        string   `json:"visibility"`
		SelectedUsernames []string `json:"selected_usernames"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !orgCodespacesAccessVisibilities[req.Visibility] {
		writeGHValidationError(w, "OrgCodespacesAccess", "visibility", "invalid")
		return
	}
	if len(req.SelectedUsernames) > 100 {
		writeGHValidationError(w, "OrgCodespacesAccess", "selected_usernames", "invalid")
		return
	}
	if len(req.SelectedUsernames) > 0 && req.Visibility != "selected_members" {
		writeGHValidationError(w, "OrgCodespacesAccess", "selected_usernames", "invalid")
		return
	}
	if invalid := s.store.orgCodespacesInvalidUsers(org, req.SelectedUsernames); len(invalid) > 0 {
		writeGHError(w, http.StatusBadRequest, "Users are neither members nor collaborators of this organization.")
		return
	}
	s.store.SetOrgCodespacesAccess(org.Login, req.Visibility, req.SelectedUsernames)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) modifyOrgCodespacesAccessUsers(w http.ResponseWriter, r *http.Request, add bool) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		SelectedUsernames []string `json:"selected_usernames"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if len(req.SelectedUsernames) == 0 || len(req.SelectedUsernames) > 100 {
		writeGHValidationError(w, "OrgCodespacesAccess", "selected_usernames", "invalid")
		return
	}
	if invalid := s.store.orgCodespacesInvalidUsers(org, req.SelectedUsernames); len(invalid) > 0 {
		writeGHError(w, http.StatusBadRequest, "Users are neither members nor collaborators of this organization.")
		return
	}
	if !s.store.ModifyOrgCodespacesAccessUsers(org.Login, add, req.SelectedUsernames) {
		writeGHValidationError(w, "OrgCodespacesAccess", "visibility", "invalid")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddOrgCodespacesAccessUsers(w http.ResponseWriter, r *http.Request) {
	s.modifyOrgCodespacesAccessUsers(w, r, true)
}

func (s *Server) handleRemoveOrgCodespacesAccessUsers(w http.ResponseWriter, r *http.Request) {
	s.modifyOrgCodespacesAccessUsers(w, r, false)
}

// ─── org codespaces secret selected-repository add/remove ────────────────

func (sec *CodespaceSecret) itemVisibility() string {
	if sec.Visibility == "" {
		return "all"
	}
	return sec.Visibility
}
func (sec *CodespaceSecret) selectedIDs() []int         { return sec.SelectedRepoIDs }
func (sec *CodespaceSecret) setSelectedIDs(ids []int)   { sec.SelectedRepoIDs = ids }
func (sec *CodespaceSecret) touchUpdated(now time.Time) { sec.UpdatedAt = now }

// orgCodespaceSecretSelectionChange adapts the shared per-repository
// selection core to the org codespaces secrets table.
func (s *Server) orgCodespaceSecretSelectionChange(w http.ResponseWriter, r *http.Request, add bool) {
	scope := codespaceSecretScopeKey("org", r.PathValue("org"))
	name := r.PathValue("secret_name")
	s.handleOrgSelectionChange(w, r, name, add,
		func() orgScopedItem {
			if sec := s.store.CodespaceSecrets[scope][name]; sec != nil {
				return sec
			}
			return nil
		},
		func() {})
}

func (s *Server) handleAddOrgCodespaceSecretRepo(w http.ResponseWriter, r *http.Request) {
	s.orgCodespaceSecretSelectionChange(w, r, true)
}

func (s *Server) handleRemoveOrgCodespaceSecretRepo(w http.ResponseWriter, r *http.Request) {
	s.orgCodespaceSecretSelectionChange(w, r, false)
}
