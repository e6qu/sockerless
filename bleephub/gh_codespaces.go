package bleephub

import (
	"context"
	"fmt"
	"net/http"
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

	// User-scoped secrets.
	s.route("GET /api/v3/user/codespaces/secrets", s.requirePerm(scopeCodespaces, permRead, s.handleListUserCodespaceSecrets))
	s.route("GET /api/v3/user/codespaces/secrets/public-key", s.requirePerm(scopeCodespaces, permRead, s.handleGetCodespacePublicKey))
	s.route("GET /api/v3/user/codespaces/secrets/{secret_name}", s.requirePerm(scopeCodespaces, permRead, s.handleGetUserCodespaceSecret))
	s.route("PUT /api/v3/user/codespaces/secrets/{secret_name}", s.requirePerm(scopeCodespaces, permWrite, s.handlePutUserCodespaceSecret))
	s.route("DELETE /api/v3/user/codespaces/secrets/{secret_name}", s.requirePerm(scopeCodespaces, permWrite, s.handleDeleteUserCodespaceSecret))

	// Repository-scoped secrets.
	s.route("GET /api/v3/repos/{owner}/{repo}/codespaces/secrets", s.requirePerm(scopeCodespaces, permRead, s.handleListRepoCodespaceSecrets))
	s.route("GET /api/v3/repos/{owner}/{repo}/codespaces/secrets/public-key", s.requirePerm(scopeCodespaces, permRead, s.handleGetCodespacePublicKey))
	s.route("GET /api/v3/repos/{owner}/{repo}/codespaces/secrets/{secret_name}", s.requirePerm(scopeCodespaces, permRead, s.handleGetRepoCodespaceSecret))
	s.route("PUT /api/v3/repos/{owner}/{repo}/codespaces/secrets/{secret_name}", s.requirePerm(scopeCodespaces, permWrite, s.handlePutRepoCodespaceSecret))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/codespaces/secrets/{secret_name}", s.requirePerm(scopeCodespaces, permWrite, s.handleDeleteRepoCodespaceSecret))

	// Organization-scoped secrets.
	s.route("GET /api/v3/orgs/{org}/codespaces/secrets", s.requireOrgAdminOrCodespaceScope(s.handleListOrgCodespaceSecrets))
	s.route("GET /api/v3/orgs/{org}/codespaces/secrets/public-key", s.requireOrgAdminOrCodespaceScope(s.handleGetCodespacePublicKey))
	s.route("GET /api/v3/orgs/{org}/codespaces/secrets/{secret_name}", s.requireOrgAdminOrCodespaceScope(s.handleGetOrgCodespaceSecret))
	s.route("PUT /api/v3/orgs/{org}/codespaces/secrets/{secret_name}", s.requireOrgAdminOrCodespaceScope(s.handlePutOrgCodespaceSecret))
	s.route("DELETE /api/v3/orgs/{org}/codespaces/secrets/{secret_name}", s.requireOrgAdminOrCodespaceScope(s.handleDeleteOrgCodespaceSecret))
	s.route("GET /api/v3/orgs/{org}/codespaces/secrets/{secret_name}/repositories", s.requireOrgAdminOrCodespaceScope(s.handleListOrgCodespaceSecretRepos))
	s.route("PUT /api/v3/orgs/{org}/codespaces/secrets/{secret_name}/repositories", s.requireOrgAdminOrCodespaceScope(s.handleSetOrgCodespaceSecretRepos))
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
		s.logger.Warn().Err(err).Msg("codespace create failed")
	}
	status := http.StatusCreated
	if cs.State == "Unavailable" {
		status = http.StatusCreated
	}
	writeJSON(w, status, s.codespaceToJSON(cs, s.baseURL(r)))
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
	if !s.store.DeleteCodespace(cs.ID) {
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
		s.logger.Warn().Err(err).Int("codespace_id", cs.ID).Msg("codespace start failed")
		cs.State = "Unavailable"
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
		s.logger.Warn().Err(err).Int("codespace_id", cs.ID).Msg("codespace stop failed")
		cs.State = "Unavailable"
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
		s.logger.Warn().Err(err).Msg("codespace create failed")
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
	if !s.store.DeleteCodespace(cs.ID) {
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
		s.logger.Warn().Err(err).Int("codespace_id", cs.ID).Msg("codespace start failed")
		cs.State = "Unavailable"
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
		s.logger.Warn().Err(err).Int("codespace_id", cs.ID).Msg("codespace stop failed")
		cs.State = "Unavailable"
	}
	writeJSON(w, http.StatusAccepted, s.codespaceToJSON(cs, s.baseURL(r)))
}

// --- machines ---

func (s *Server) handleListCodespaceMachines(w http.ResponseWriter, r *http.Request) {
	if s.lookupRepoFromPath(r) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	machines := make([]map[string]interface{}, len(codespaceMachines))
	for i, m := range codespaceMachines {
		machines[i] = map[string]interface{}{
			"name":                  m.Name,
			"display_name":          m.DisplayName,
			"operating_system":      "linux",
			"storage_in_bytes":      34359738368,
			"memory_in_bytes":       4294967296,
			"cpus":                  2,
			"prebuild_availability": "ready",
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"machines": machines, "total_count": len(machines)})
}

// --- secrets handlers ---

func (s *Server) handleListUserCodespaceSecrets(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	scope := codespaceSecretScopeKey("user", user.Login)
	secs := s.store.ListCodespaceSecrets(scope)
	writeJSON(w, http.StatusOK, codespaceSecretsListJSON(secs))
}

func (s *Server) handleGetUserCodespaceSecret(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	sec := s.getCodespaceSecret(r, "user", user.Login)
	if sec == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, codespaceUserSecretJSON(sec))
}

func (s *Server) handlePutUserCodespaceSecret(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	name := r.PathValue("secret_name")
	value, ok := s.readSealedCodespaceSecret(w, r)
	if !ok {
		return
	}
	sec := s.store.CreateCodespaceSecret(codespaceSecretScopeKey("user", user.Login), name, value, "", nil)
	w.WriteHeader(http.StatusNoContent)
	_ = sec
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
	sec := s.store.CreateCodespaceSecret(codespaceSecretScopeKey("repo", repo.FullName), name, value, "", nil)
	w.WriteHeader(http.StatusNoContent)
	_ = sec
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
		out[i] = codespaceOrgSecretJSON(sec)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"secrets": out, "total_count": len(out)})
}

func (s *Server) handleGetOrgCodespaceSecret(w http.ResponseWriter, r *http.Request) {
	sec := s.getCodespaceSecret(r, "org", r.PathValue("org"))
	if sec == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, codespaceOrgSecretJSON(sec))
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
	sec := s.store.CreateCodespaceSecret(codespaceSecretScopeKey("org", org), name, plain, req.Visibility, req.SelectedRepositoryIDs)
	w.WriteHeader(http.StatusNoContent)
	_ = sec
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
	cs.State = dockerStateToCodespaceState(cs.ContainerID)
	cs.LastUsedAt = time.Now().UTC()
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
	cs.State = dockerStateToCodespaceState(cs.ContainerID)
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
		"id":             cs.ID,
		"name":           cs.Name,
		"display_name":   cs.DisplayName,
		"environment_id": fmt.Sprintf("%d", cs.ID),
		"owner":          ownerJSON,
		"billable_owner": ownerJSON,
		"repository":     repoJSON,
		"machine": map[string]interface{}{
			"name":                  cs.MachineName,
			"display_name":          cs.MachineDisplayName,
			"operating_system":      "linux",
			"storage_in_bytes":      34359738368,
			"memory_in_bytes":       4294967296,
			"cpus":                  2,
			"prebuild_availability": "ready",
		},
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

func codespaceUserSecretJSON(sec *CodespaceSecret) map[string]interface{} {
	visibility := sec.Visibility
	if visibility == "" {
		visibility = "all"
	}
	return map[string]interface{}{
		"name":                      sec.Name,
		"created_at":                sec.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":                sec.UpdatedAt.UTC().Format(time.RFC3339),
		"visibility":                visibility,
		"selected_repositories_url": "",
	}
}

func codespaceRepoSecretJSON(sec *CodespaceSecret) map[string]interface{} {
	return map[string]interface{}{
		"name":       sec.Name,
		"created_at": sec.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": sec.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func codespaceOrgSecretJSON(sec *CodespaceSecret) map[string]interface{} {
	return codespaceUserSecretJSON(sec)
}

func codespaceSecretsListJSON(secs []*CodespaceSecret) map[string]interface{} {
	out := make([]map[string]interface{}, len(secs))
	for i, sec := range secs {
		out[i] = codespaceUserSecretJSON(sec)
	}
	return map[string]interface{}{"secrets": out, "total_count": len(out)}
}
