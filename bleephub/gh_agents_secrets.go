package bleephub

import (
	"net/http"
	"sort"
	"strings"
	"time"
)

// GitHub Copilot coding agent secrets and variables — the /agents/
// counterpart of the Actions secrets/variables surfaces, at repository
// and organization scope. Secrets arrive as libsodium sealed boxes
// against the shared public key; variables are plaintext. Organization
// items carry all|private|selected visibility with selected repositories.

func (s *Server) registerGHAgentsSecretsRoutes() {
	// Repository-scoped secrets.
	s.route("GET /api/v3/repos/{owner}/{repo}/agents/secrets", s.requirePerm(scopeSecrets, permRead, s.handleListAgentsRepoSecrets))
	s.route("GET /api/v3/repos/{owner}/{repo}/agents/secrets/public-key", s.requirePerm(scopeSecrets, permRead, s.handleGetAgentsRepoSecretsPublicKey))
	s.route("GET /api/v3/repos/{owner}/{repo}/agents/secrets/{secret_name}", s.requirePerm(scopeSecrets, permRead, s.handleGetAgentsRepoSecret))
	s.route("PUT /api/v3/repos/{owner}/{repo}/agents/secrets/{secret_name}", s.requirePerm(scopeSecrets, permWrite, s.handlePutAgentsRepoSecret))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/agents/secrets/{secret_name}", s.requirePerm(scopeSecrets, permWrite, s.handleDeleteAgentsRepoSecret))
	s.route("GET /api/v3/repos/{owner}/{repo}/agents/organization-secrets", s.requirePerm(scopeSecrets, permRead, s.handleListAgentsRepoOrgSecrets))

	// Repository-scoped variables.
	s.route("GET /api/v3/repos/{owner}/{repo}/agents/variables", s.requirePerm(scopeSecrets, permRead, s.handleListAgentsRepoVariables))
	s.route("POST /api/v3/repos/{owner}/{repo}/agents/variables", s.requirePerm(scopeSecrets, permWrite, s.handleCreateAgentsRepoVariable))
	s.route("GET /api/v3/repos/{owner}/{repo}/agents/variables/{name}", s.requirePerm(scopeSecrets, permRead, s.handleGetAgentsRepoVariable))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/agents/variables/{name}", s.requirePerm(scopeSecrets, permWrite, s.handlePatchAgentsRepoVariable))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/agents/variables/{name}", s.requirePerm(scopeSecrets, permWrite, s.handleDeleteAgentsRepoVariable))
	s.route("GET /api/v3/repos/{owner}/{repo}/agents/organization-variables", s.requirePerm(scopeSecrets, permRead, s.handleListAgentsRepoOrgVariables))

	// Organization-scoped secrets.
	s.route("GET /api/v3/orgs/{org}/agents/secrets", s.requirePerm(scopeSecrets, permRead, s.handleListAgentsOrgSecrets))
	s.route("GET /api/v3/orgs/{org}/agents/secrets/public-key", s.requirePerm(scopeSecrets, permRead, s.handleGetAgentsOrgSecretsPublicKey))
	s.route("GET /api/v3/orgs/{org}/agents/secrets/{secret_name}", s.requirePerm(scopeSecrets, permRead, s.handleGetAgentsOrgSecret))
	s.route("PUT /api/v3/orgs/{org}/agents/secrets/{secret_name}", s.requirePerm(scopeSecrets, permWrite, s.handlePutAgentsOrgSecret))
	s.route("DELETE /api/v3/orgs/{org}/agents/secrets/{secret_name}", s.requirePerm(scopeSecrets, permWrite, s.handleDeleteAgentsOrgSecret))
	s.route("GET /api/v3/orgs/{org}/agents/secrets/{secret_name}/repositories", s.requirePerm(scopeSecrets, permRead, s.handleListAgentsOrgSecretRepos))
	s.route("PUT /api/v3/orgs/{org}/agents/secrets/{secret_name}/repositories", s.requirePerm(scopeSecrets, permWrite, s.handleSetAgentsOrgSecretRepos))
	s.route("PUT /api/v3/orgs/{org}/agents/secrets/{secret_name}/repositories/{repository_id}", s.requirePerm(scopeSecrets, permWrite, s.handleAddAgentsOrgSecretRepo))
	s.route("DELETE /api/v3/orgs/{org}/agents/secrets/{secret_name}/repositories/{repository_id}", s.requirePerm(scopeSecrets, permWrite, s.handleRemoveAgentsOrgSecretRepo))

	// Organization-scoped variables.
	s.route("GET /api/v3/orgs/{org}/agents/variables", s.requirePerm(scopeSecrets, permRead, s.handleListAgentsOrgVariables))
	s.route("POST /api/v3/orgs/{org}/agents/variables", s.requirePerm(scopeSecrets, permWrite, s.handleCreateAgentsOrgVariable))
	s.route("GET /api/v3/orgs/{org}/agents/variables/{name}", s.requirePerm(scopeSecrets, permRead, s.handleGetAgentsOrgVariable))
	s.route("PATCH /api/v3/orgs/{org}/agents/variables/{name}", s.requirePerm(scopeSecrets, permWrite, s.handlePatchAgentsOrgVariable))
	s.route("DELETE /api/v3/orgs/{org}/agents/variables/{name}", s.requirePerm(scopeSecrets, permWrite, s.handleDeleteAgentsOrgVariable))
	s.route("GET /api/v3/orgs/{org}/agents/variables/{name}/repositories", s.requirePerm(scopeSecrets, permRead, s.handleListAgentsOrgVariableRepos))
	s.route("PUT /api/v3/orgs/{org}/agents/variables/{name}/repositories", s.requirePerm(scopeSecrets, permWrite, s.handleSetAgentsOrgVariableRepos))
	s.route("PUT /api/v3/orgs/{org}/agents/variables/{name}/repositories/{repository_id}", s.requirePerm(scopeSecrets, permWrite, s.handleAddAgentsOrgVariableRepo))
	s.route("DELETE /api/v3/orgs/{org}/agents/variables/{name}/repositories/{repository_id}", s.requirePerm(scopeSecrets, permWrite, s.handleRemoveAgentsOrgVariableRepo))
}

// resolveAgentsRepo resolves {owner}/{repo} or writes a 404.
func (s *Server) resolveAgentsRepo(w http.ResponseWriter, r *http.Request) (*Repo, bool) {
	repo := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return repo, true
}

// --- repository secrets ---

func (s *Server) handleListAgentsRepoSecrets(w http.ResponseWriter, r *http.Request) {
	repo, ok := s.resolveAgentsRepo(w, r)
	if !ok {
		return
	}

	s.store.mu.RLock()
	list := sortedSecretsJSON(s.store.AgentsRepoSecrets[repo.FullName])
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": len(list),
		"secrets":     list,
	})
}

func (s *Server) handleGetAgentsRepoSecretsPublicKey(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.resolveAgentsRepo(w, r); !ok {
		return
	}
	s.writeActionsPublicKey(w)
}

func (s *Server) handleGetAgentsRepoSecret(w http.ResponseWriter, r *http.Request) {
	repo, ok := s.resolveAgentsRepo(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))

	s.store.mu.RLock()
	sec := s.store.AgentsRepoSecrets[repo.FullName][name]
	var body map[string]interface{}
	if sec != nil {
		body = secretJSON(sec)
	}
	s.store.mu.RUnlock()

	if body == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) handlePutAgentsRepoSecret(w http.ResponseWriter, r *http.Request) {
	repo, ok := s.resolveAgentsRepo(w, r)
	if !ok {
		return
	}
	rawName := r.PathValue("secret_name")
	if msg := actionsItemNameError("Secret", rawName); msg != "" {
		writeGHError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	name := strings.ToUpper(rawName)

	var body sealedSecretBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	plain, ok := s.decryptSealedSecret(w, body.EncryptedValue, body.KeyID)
	if !ok {
		return
	}

	created := s.upsertSecret(s.store.AgentsRepoSecrets, "agents_repo_secrets", repo.FullName, name, plain)
	s.recordAuditEvent("agents_secret.create", auditActor(r), "", map[string]interface{}{
		"scope": "repository", "repo": repo.FullName, "secret_name": name,
	})
	if created {
		writeJSON(w, http.StatusCreated, map[string]interface{}{})
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleDeleteAgentsRepoSecret(w http.ResponseWriter, r *http.Request) {
	repo, ok := s.resolveAgentsRepo(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))

	if !s.deleteSecret(s.store.AgentsRepoSecrets, "agents_repo_secrets", repo.FullName, name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.recordAuditEvent("agents_secret.destroy", auditActor(r), "", map[string]interface{}{
		"scope": "repository", "repo": repo.FullName, "secret_name": name,
	})
	w.WriteHeader(http.StatusNoContent)
}

// handleListAgentsRepoOrgSecrets lists the organization Copilot coding
// agent secrets visible to a repository. The documented item shape is the
// plain actions-secret — visibility metadata stays on the org endpoints.
func (s *Server) handleListAgentsRepoOrgSecrets(w http.ResponseWriter, r *http.Request) {
	repo, ok := s.resolveAgentsRepo(w, r)
	if !ok {
		return
	}

	list := make([]map[string]interface{}, 0)
	if org := s.store.GetOrg(r.PathValue("owner")); org != nil {
		s.store.mu.RLock()
		visible := make(map[string]*Secret)
		for name, sec := range s.store.AgentsOrgSecrets[org.Login] {
			if orgItemVisibleToRepo(sec.Visibility, sec.SelectedRepoIDs, repo) {
				visible[name] = &sec.Secret
			}
		}
		list = sortedSecretsJSON(visible)
		s.store.mu.RUnlock()
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": len(list),
		"secrets":     list,
	})
}

// --- organization secrets ---

// agentsOrgSecretJSON renders the organization-actions-secret shape with
// the /agents/ selected-repositories URL.
func agentsOrgSecretJSON(sec *OrgSecret, orgLogin, baseURL string) map[string]interface{} {
	out := secretJSON(&sec.Secret)
	out["visibility"] = sec.Visibility
	if sec.Visibility == "selected" {
		out["selected_repositories_url"] = baseURL + "/api/v3/orgs/" + orgLogin + "/agents/secrets/" + sec.Name + "/repositories"
	}
	return out
}

func (s *Server) handleListAgentsOrgSecrets(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	base := s.baseURL(r)

	s.store.mu.RLock()
	m := s.store.AgentsOrgSecrets[org.Login]
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	list := make([]map[string]interface{}, 0, len(names))
	for _, n := range names {
		list = append(list, agentsOrgSecretJSON(m[n], org.Login, base))
	}
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": len(list),
		"secrets":     list,
	})
}

func (s *Server) handleGetAgentsOrgSecretsPublicKey(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.resolveOrgForActions(w, r); !ok {
		return
	}
	s.writeActionsPublicKey(w)
}

func (s *Server) handleGetAgentsOrgSecret(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))
	base := s.baseURL(r)

	s.store.mu.RLock()
	sec := s.store.AgentsOrgSecrets[org.Login][name]
	var body map[string]interface{}
	if sec != nil {
		body = agentsOrgSecretJSON(sec, org.Login, base)
	}
	s.store.mu.RUnlock()

	if body == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) handlePutAgentsOrgSecret(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	rawName := r.PathValue("secret_name")
	if msg := actionsItemNameError("Secret", rawName); msg != "" {
		writeGHError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	name := strings.ToUpper(rawName)

	var body struct {
		sealedSecretBody
		Visibility            string `json:"visibility"`
		SelectedRepositoryIDs []int  `json:"selected_repository_ids"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if body.Visibility == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "visibility is required and must be one of: all, private, selected")
		return
	}
	if !validOrgItemVisibility(body.Visibility) {
		writeGHError(w, http.StatusUnprocessableEntity, "visibility must be one of: all, private, selected")
		return
	}
	plain, ok := s.decryptSealedSecret(w, body.EncryptedValue, body.KeyID)
	if !ok {
		return
	}

	ids := body.SelectedRepositoryIDs
	if body.Visibility != "selected" {
		ids = nil
	}
	for _, id := range ids {
		if s.store.GetRepoByID(id) == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
	}

	now := time.Now().UTC()
	s.store.mu.Lock()
	m := s.store.AgentsOrgSecrets[org.Login]
	if m == nil {
		m = make(map[string]*OrgSecret)
		s.store.AgentsOrgSecrets[org.Login] = m
	}
	existing := m[name]
	if existing != nil {
		existing.Value = plain
		existing.UpdatedAt = now
		existing.Visibility = body.Visibility
		existing.SelectedRepoIDs = ids
	} else {
		m[name] = &OrgSecret{
			Secret:          Secret{Name: name, CreatedAt: now, UpdatedAt: now, Value: plain},
			Visibility:      body.Visibility,
			SelectedRepoIDs: ids,
		}
	}
	if s.store.persist != nil {
		s.store.persist.MustPut("agents_org_secrets", org.Login, m)
	}
	s.store.mu.Unlock()

	s.recordAuditEvent("agents_secret.create", auditActor(r), org.Login, map[string]interface{}{
		"scope": "organization", "org": org.Login, "secret_name": name, "visibility": body.Visibility,
	})
	if existing == nil {
		writeJSON(w, http.StatusCreated, map[string]interface{}{})
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleDeleteAgentsOrgSecret(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))

	s.store.mu.Lock()
	m := s.store.AgentsOrgSecrets[org.Login]
	existed := m[name] != nil
	if existed {
		delete(m, name)
		if s.store.persist != nil {
			if len(m) > 0 {
				s.store.persist.MustPut("agents_org_secrets", org.Login, m)
			} else {
				s.store.persist.MustDelete("agents_org_secrets", org.Login)
			}
		}
	}
	s.store.mu.Unlock()

	if !existed {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.recordAuditEvent("agents_secret.destroy", auditActor(r), org.Login, map[string]interface{}{
		"scope": "organization", "org": org.Login, "secret_name": name,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAgentsOrgSecretRepos(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))

	s.store.mu.RLock()
	sec := s.store.AgentsOrgSecrets[org.Login][name]
	var ids []int
	if sec != nil {
		ids = append([]int(nil), sec.SelectedRepoIDs...)
	}
	s.store.mu.RUnlock()

	if sec == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.writeSelectedReposResponse(w, r, ids)
}

func (s *Server) handleSetAgentsOrgSecretRepos(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))
	s.setOrgItemSelectedRepos(w, r, name, false,
		func() orgScopedItem {
			if sec := s.store.AgentsOrgSecrets[org.Login][name]; sec != nil {
				return sec
			}
			return nil
		},
		func() {
			if s.store.persist != nil {
				s.store.persist.MustPut("agents_org_secrets", org.Login, s.store.AgentsOrgSecrets[org.Login])
			}
		})
}

// agentsOrgSecretSelectionChange adapts the shared per-repo add/remove
// core to the Copilot coding agent org-secrets table.
func (s *Server) agentsOrgSecretSelectionChange(w http.ResponseWriter, r *http.Request, add bool) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))
	s.handleOrgSelectionChange(w, r, name, add,
		func() orgScopedItem {
			if sec := s.store.AgentsOrgSecrets[org.Login][name]; sec != nil {
				return sec
			}
			return nil
		},
		func() {
			if s.store.persist != nil {
				s.store.persist.MustPut("agents_org_secrets", org.Login, s.store.AgentsOrgSecrets[org.Login])
			}
		})
}

func (s *Server) handleAddAgentsOrgSecretRepo(w http.ResponseWriter, r *http.Request) {
	s.agentsOrgSecretSelectionChange(w, r, true)
}

func (s *Server) handleRemoveAgentsOrgSecretRepo(w http.ResponseWriter, r *http.Request) {
	s.agentsOrgSecretSelectionChange(w, r, false)
}

// --- variables ---

// agentsVariableTable binds one Copilot coding agent variables scope
// (repository or organization) to its in-store map and persistence bucket
// so the verb handlers share a single locked CRUD core (the same pattern
// as the Actions variableTable).
type agentsVariableTable struct {
	s      *Server
	bucket string // "agents_repo_variables" | "agents_org_variables"
	key    string // repo full name or org login
}

func (t agentsVariableTable) rows() map[string]map[string]*ActionsVariable {
	if t.bucket == "agents_org_variables" {
		return t.s.store.AgentsOrgVariables
	}
	return t.s.store.AgentsRepoVariables
}

// persistLocked writes the scope's collection through (or removes the row
// when the collection emptied). Caller holds the store write lock.
func (t agentsVariableTable) persistLocked(m map[string]*ActionsVariable) {
	if t.s.store.persist == nil {
		return
	}
	if len(m) > 0 {
		t.s.store.persist.MustPut(t.bucket, t.key, m)
	} else {
		t.s.store.persist.MustDelete(t.bucket, t.key)
	}
}

// list returns the scope's variables sorted by name (copies, so callers
// can render without the lock).
func (t agentsVariableTable) list() []*ActionsVariable {
	t.s.store.mu.RLock()
	defer t.s.store.mu.RUnlock()
	m := t.rows()[t.key]
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*ActionsVariable, 0, len(names))
	for _, n := range names {
		out = append(out, cloneVariable(m[n]))
	}
	return out
}

func (t agentsVariableTable) get(name string) *ActionsVariable {
	t.s.store.mu.RLock()
	defer t.s.store.mu.RUnlock()
	v := t.rows()[t.key][name]
	if v == nil {
		return nil
	}
	return cloneVariable(v)
}

// create inserts a new variable; false when the name already exists.
func (t agentsVariableTable) create(v *ActionsVariable) bool {
	t.s.store.mu.Lock()
	defer t.s.store.mu.Unlock()
	rows := t.rows()
	m := rows[t.key]
	if m == nil {
		m = make(map[string]*ActionsVariable)
		rows[t.key] = m
	}
	if m[v.Name] != nil {
		return false
	}
	m[v.Name] = v
	t.persistLocked(m)
	return true
}

// patch mutates the named variable, optionally renaming it. Returns the
// HTTP status to write: 204 applied, 404 unknown, 409 rename collision.
func (t agentsVariableTable) patch(name, newName string, apply func(*ActionsVariable)) int {
	t.s.store.mu.Lock()
	defer t.s.store.mu.Unlock()
	m := t.rows()[t.key]
	v := m[name]
	if v == nil {
		return http.StatusNotFound
	}
	if newName != "" && newName != name {
		if m[newName] != nil {
			return http.StatusConflict
		}
		delete(m, name)
		v.Name = newName
		m[newName] = v
	}
	apply(v)
	v.UpdatedAt = time.Now().UTC()
	t.persistLocked(m)
	return http.StatusNoContent
}

// remove deletes the named variable; false when it did not exist.
func (t agentsVariableTable) remove(name string) bool {
	t.s.store.mu.Lock()
	defer t.s.store.mu.Unlock()
	m := t.rows()[t.key]
	if m[name] == nil {
		return false
	}
	delete(m, name)
	t.persistLocked(m)
	return true
}

// --- repository variables ---

func (s *Server) agentsRepoVariableTableFor(w http.ResponseWriter, r *http.Request) (agentsVariableTable, bool) {
	repo, ok := s.resolveAgentsRepo(w, r)
	if !ok {
		return agentsVariableTable{}, false
	}
	return agentsVariableTable{s, "agents_repo_variables", repo.FullName}, true
}

func (s *Server) handleListAgentsRepoVariables(w http.ResponseWriter, r *http.Request) {
	t, ok := s.agentsRepoVariableTableFor(w, r)
	if !ok {
		return
	}
	vars := t.list()
	list := make([]map[string]interface{}, 0, len(vars))
	for _, v := range vars {
		list = append(list, variableJSON(v))
	}
	writeVariablesList(w, list)
}

func (s *Server) handleCreateAgentsRepoVariable(w http.ResponseWriter, r *http.Request) {
	t, ok := s.agentsRepoVariableTableFor(w, r)
	if !ok {
		return
	}
	var body struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if msg := actionsItemNameError("Variable", body.Name); msg != "" {
		writeGHError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	name := strings.ToUpper(body.Name)
	now := time.Now().UTC()
	if !t.create(&ActionsVariable{Name: name, Value: body.Value, CreatedAt: now, UpdatedAt: now}) {
		writeGHError(w, http.StatusConflict, "Variable already exists")
		return
	}
	s.recordAuditEvent("agents_variable.create", auditActor(r), "", map[string]interface{}{
		"scope": "repository", "repo": t.key, "variable_name": name,
	})
	writeJSON(w, http.StatusCreated, map[string]interface{}{})
}

func (s *Server) handleGetAgentsRepoVariable(w http.ResponseWriter, r *http.Request) {
	t, ok := s.agentsRepoVariableTableFor(w, r)
	if !ok {
		return
	}
	v := t.get(strings.ToUpper(r.PathValue("name")))
	if v == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, variableJSON(v))
}

func (s *Server) handlePatchAgentsRepoVariable(w http.ResponseWriter, r *http.Request) {
	t, ok := s.agentsRepoVariableTableFor(w, r)
	if !ok {
		return
	}
	newName, value, ok := decodeVariablePatch(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("name"))
	status := t.patch(name, newName, func(v *ActionsVariable) {
		if value != nil {
			v.Value = *value
		}
	})
	if writeVariablePatchStatus(w, status) {
		s.recordAuditEvent("agents_variable.update", auditActor(r), "", map[string]interface{}{
			"scope": "repository", "repo": t.key, "variable_name": name,
		})
	}
}

func (s *Server) handleDeleteAgentsRepoVariable(w http.ResponseWriter, r *http.Request) {
	t, ok := s.agentsRepoVariableTableFor(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("name"))
	if !t.remove(name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.recordAuditEvent("agents_variable.destroy", auditActor(r), "", map[string]interface{}{
		"scope": "repository", "repo": t.key, "variable_name": name,
	})
	w.WriteHeader(http.StatusNoContent)
}

// handleListAgentsRepoOrgVariables lists the organization Copilot coding
// agent variables visible to a repository. The documented item shape is
// the plain actions-variable — visibility metadata stays on the org
// endpoints.
func (s *Server) handleListAgentsRepoOrgVariables(w http.ResponseWriter, r *http.Request) {
	repo, ok := s.resolveAgentsRepo(w, r)
	if !ok {
		return
	}

	list := make([]map[string]interface{}, 0)
	if org := s.store.GetOrg(r.PathValue("owner")); org != nil {
		s.store.mu.RLock()
		m := s.store.AgentsOrgVariables[org.Login]
		names := make([]string, 0, len(m))
		for name, v := range m {
			if orgItemVisibleToRepo(v.Visibility, v.SelectedRepoIDs, repo) {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, n := range names {
			list = append(list, variableJSON(m[n]))
		}
		s.store.mu.RUnlock()
	}
	writeVariablesList(w, list)
}

// --- organization variables ---

// agentsOrgVariableJSON renders the organization-actions-variable shape
// with the /agents/ selected-repositories URL.
func agentsOrgVariableJSON(v *ActionsVariable, orgLogin, baseURL string) map[string]interface{} {
	out := variableJSON(v)
	out["visibility"] = v.Visibility
	if v.Visibility == "selected" {
		out["selected_repositories_url"] = baseURL + "/api/v3/orgs/" + orgLogin + "/agents/variables/" + v.Name + "/repositories"
	}
	return out
}

func (s *Server) handleListAgentsOrgVariables(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	t := agentsVariableTable{s, "agents_org_variables", org.Login}
	base := s.baseURL(r)
	vars := t.list()
	list := make([]map[string]interface{}, 0, len(vars))
	for _, v := range vars {
		list = append(list, agentsOrgVariableJSON(v, org.Login, base))
	}
	writeVariablesList(w, list)
}

func (s *Server) handleCreateAgentsOrgVariable(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	var body struct {
		Name                  string `json:"name"`
		Value                 string `json:"value"`
		Visibility            string `json:"visibility"`
		SelectedRepositoryIDs []int  `json:"selected_repository_ids"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if msg := actionsItemNameError("Variable", body.Name); msg != "" {
		writeGHError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if body.Visibility == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "visibility is required and must be one of: all, private, selected")
		return
	}
	if !validOrgItemVisibility(body.Visibility) {
		writeGHError(w, http.StatusUnprocessableEntity, "visibility must be one of: all, private, selected")
		return
	}
	ids := body.SelectedRepositoryIDs
	if body.Visibility != "selected" {
		ids = nil
	}
	for _, id := range ids {
		if s.store.GetRepoByID(id) == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
	}

	name := strings.ToUpper(body.Name)
	now := time.Now().UTC()
	t := agentsVariableTable{s, "agents_org_variables", org.Login}
	v := &ActionsVariable{
		Name: name, Value: body.Value,
		Visibility: body.Visibility, SelectedRepoIDs: ids,
		CreatedAt: now, UpdatedAt: now,
	}
	if !t.create(v) {
		writeGHError(w, http.StatusConflict, "Variable already exists")
		return
	}
	s.recordAuditEvent("agents_variable.create", auditActor(r), org.Login, map[string]interface{}{
		"scope": "organization", "org": org.Login, "variable_name": name, "visibility": body.Visibility,
	})
	writeJSON(w, http.StatusCreated, map[string]interface{}{})
}

func (s *Server) handleGetAgentsOrgVariable(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	t := agentsVariableTable{s, "agents_org_variables", org.Login}
	v := t.get(strings.ToUpper(r.PathValue("name")))
	if v == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, agentsOrgVariableJSON(v, org.Login, s.baseURL(r)))
}

func (s *Server) handlePatchAgentsOrgVariable(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	t := agentsVariableTable{s, "agents_org_variables", org.Login}
	if name, applied := s.patchOrgScopedVariable(w, r, t.patch); applied {
		s.recordAuditEvent("agents_variable.update", auditActor(r), org.Login, map[string]interface{}{
			"scope": "organization", "org": org.Login, "variable_name": name,
		})
	}
}

func (s *Server) handleDeleteAgentsOrgVariable(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("name"))
	t := agentsVariableTable{s, "agents_org_variables", org.Login}
	if !t.remove(name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.recordAuditEvent("agents_variable.destroy", auditActor(r), org.Login, map[string]interface{}{
		"scope": "organization", "org": org.Login, "variable_name": name,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAgentsOrgVariableRepos(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("name"))

	s.store.mu.RLock()
	v := s.store.AgentsOrgVariables[org.Login][name]
	var visibility string
	var ids []int
	if v != nil {
		visibility = v.Visibility
		ids = append([]int(nil), v.SelectedRepoIDs...)
	}
	s.store.mu.RUnlock()

	if v == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if visibility != "selected" {
		writeGHError(w, http.StatusConflict, "Conflict: visibility of "+name+" is not set to selected")
		return
	}
	s.writeSelectedReposResponse(w, r, ids)
}

func (s *Server) handleSetAgentsOrgVariableRepos(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("name"))
	s.setOrgItemSelectedRepos(w, r, name, true,
		func() orgScopedItem {
			if v := s.store.AgentsOrgVariables[org.Login][name]; v != nil {
				return v
			}
			return nil
		},
		func() {
			if s.store.persist != nil {
				s.store.persist.MustPut("agents_org_variables", org.Login, s.store.AgentsOrgVariables[org.Login])
			}
		})
}

// agentsOrgVariableSelectionChange adapts the shared per-repo add/remove
// core to the Copilot coding agent org-variables table.
func (s *Server) agentsOrgVariableSelectionChange(w http.ResponseWriter, r *http.Request, add bool) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("name"))
	s.handleOrgSelectionChange(w, r, name, add,
		func() orgScopedItem {
			if v := s.store.AgentsOrgVariables[org.Login][name]; v != nil {
				return v
			}
			return nil
		},
		func() {
			if s.store.persist != nil {
				s.store.persist.MustPut("agents_org_variables", org.Login, s.store.AgentsOrgVariables[org.Login])
			}
		})
}

func (s *Server) handleAddAgentsOrgVariableRepo(w http.ResponseWriter, r *http.Request) {
	s.agentsOrgVariableSelectionChange(w, r, true)
}

func (s *Server) handleRemoveAgentsOrgVariableRepo(w http.ResponseWriter, r *http.Request) {
	s.agentsOrgVariableSelectionChange(w, r, false)
}
