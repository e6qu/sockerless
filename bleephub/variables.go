package bleephub

import (
	"net/http"
	"sort"
	"strings"
	"time"
)

// GitHub Actions configuration variables (plaintext counterpart of
// secrets) at all three scopes: repository, environment, organization.
// gh CLI's `gh variable set` POSTs first and falls back to PATCH on a
// 409, so the duplicate-create conflict status is load-bearing.

func (s *Server) registerVariablesRoutes() {
	// Repository scope.
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/variables", s.requirePerm(scopeSecrets, permRead, s.handleListRepoVariables))
	s.route("POST /api/v3/repos/{owner}/{repo}/actions/variables", s.requirePerm(scopeSecrets, permWrite, s.handleCreateRepoVariable))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/variables/{name}", s.requirePerm(scopeSecrets, permRead, s.handleGetRepoVariable))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/actions/variables/{name}", s.requirePerm(scopeSecrets, permWrite, s.handlePatchRepoVariable))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/actions/variables/{name}", s.requirePerm(scopeSecrets, permWrite, s.handleDeleteRepoVariable))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/organization-variables", s.requirePerm(scopeSecrets, permRead, s.handleListRepoOrgVariables))

	// Environment scope.
	s.route("GET /api/v3/repos/{owner}/{repo}/environments/{env_name}/variables", s.requirePerm(scopeSecrets, permRead, s.handleListEnvVariables))
	s.route("POST /api/v3/repos/{owner}/{repo}/environments/{env_name}/variables", s.requirePerm(scopeSecrets, permWrite, s.handleCreateEnvVariable))
	s.route("GET /api/v3/repos/{owner}/{repo}/environments/{env_name}/variables/{name}", s.requirePerm(scopeSecrets, permRead, s.handleGetEnvVariable))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/environments/{env_name}/variables/{name}", s.requirePerm(scopeSecrets, permWrite, s.handlePatchEnvVariable))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/environments/{env_name}/variables/{name}", s.requirePerm(scopeSecrets, permWrite, s.handleDeleteEnvVariable))

	// Organization scope.
	s.route("GET /api/v3/orgs/{org}/actions/variables", s.requirePerm(scopeSecrets, permRead, s.handleListOrgVariables))
	s.route("POST /api/v3/orgs/{org}/actions/variables", s.requirePerm(scopeSecrets, permWrite, s.handleCreateOrgVariable))
	s.route("GET /api/v3/orgs/{org}/actions/variables/{name}", s.requirePerm(scopeSecrets, permRead, s.handleGetOrgVariable))
	s.route("PATCH /api/v3/orgs/{org}/actions/variables/{name}", s.requirePerm(scopeSecrets, permWrite, s.handlePatchOrgVariable))
	s.route("DELETE /api/v3/orgs/{org}/actions/variables/{name}", s.requirePerm(scopeSecrets, permWrite, s.handleDeleteOrgVariable))
	s.route("GET /api/v3/orgs/{org}/actions/variables/{name}/repositories", s.requirePerm(scopeSecrets, permRead, s.handleListOrgVariableRepos))
	s.route("PUT /api/v3/orgs/{org}/actions/variables/{name}/repositories", s.requirePerm(scopeSecrets, permWrite, s.handleSetOrgVariableRepos))
	s.route("PUT /api/v3/orgs/{org}/actions/variables/{name}/repositories/{repository_id}", s.requirePerm(scopeSecrets, permWrite, s.handleAddOrgVariableRepo))
	s.route("DELETE /api/v3/orgs/{org}/actions/variables/{name}/repositories/{repository_id}", s.requirePerm(scopeSecrets, permWrite, s.handleRemoveOrgVariableRepo))
}

// --- shared pieces ---

func variableJSON(v *ActionsVariable) map[string]interface{} {
	return map[string]interface{}{
		"name":       v.Name,
		"value":      v.Value,
		"created_at": v.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": v.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// orgVariableJSON renders the organization-actions-variable shape;
// selected_repositories_url appears only for visibility "selected".
func orgVariableJSON(v *ActionsVariable, orgLogin, baseURL string) map[string]interface{} {
	out := variableJSON(v)
	out["visibility"] = v.Visibility
	if v.Visibility == "selected" {
		out["selected_repositories_url"] = baseURL + "/api/v3/orgs/" + orgLogin + "/actions/variables/" + v.Name + "/repositories"
	}
	return out
}

// variableTable binds one variables scope (repository, organization, or
// environment) to its in-store map and persistence bucket so the verb
// handlers share a single locked CRUD core.
type variableTable struct {
	s      *Server
	bucket string // "repo_variables" | "org_variables" | "env_variables"
	key    string // repo full name, org login, or envScopeKey(...)
}

func (s *Server) repoVariableTableFor(w http.ResponseWriter, r *http.Request) (variableTable, bool) {
	repo := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return variableTable{}, false
	}
	return variableTable{s, "repo_variables", repo.FullName}, true
}

func (t variableTable) rows() map[string]map[string]*ActionsVariable {
	switch t.bucket {
	case "repo_variables":
		return t.s.store.RepoVariables
	case "org_variables":
		return t.s.store.OrgVariables
	default:
		return t.s.store.EnvVariables
	}
}

// persistLocked writes the scope's collection through (or removes the row
// when the collection emptied). Caller holds the store write lock.
func (t variableTable) persistLocked(m map[string]*ActionsVariable) {
	if t.s.store.persist == nil {
		return
	}
	if len(m) > 0 {
		t.s.store.persist.MustPut(t.bucket, t.key, m)
	} else {
		t.s.store.persist.MustDelete(t.bucket, t.key)
	}
}

func cloneVariable(v *ActionsVariable) *ActionsVariable {
	cp := *v
	cp.SelectedRepoIDs = append([]int(nil), v.SelectedRepoIDs...)
	return &cp
}

// list returns the scope's variables sorted by name (copies, so callers
// can render without the lock).
func (t variableTable) list() []*ActionsVariable {
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

func (t variableTable) get(name string) *ActionsVariable {
	t.s.store.mu.RLock()
	defer t.s.store.mu.RUnlock()
	v := t.rows()[t.key][name]
	if v == nil {
		return nil
	}
	return cloneVariable(v)
}

// create inserts a new variable; false when the name already exists.
func (t variableTable) create(v *ActionsVariable) bool {
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
func (t variableTable) patch(name, newName string, apply func(*ActionsVariable)) int {
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
func (t variableTable) remove(name string) bool {
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

func writeVariablesList(w http.ResponseWriter, list []map[string]interface{}) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": len(list),
		"variables":   list,
	})
}

// --- repository variables ---

func (s *Server) handleListRepoVariables(w http.ResponseWriter, r *http.Request) {
	t, ok := s.repoVariableTableFor(w, r)
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

func (s *Server) handleCreateRepoVariable(w http.ResponseWriter, r *http.Request) {
	t, ok := s.repoVariableTableFor(w, r)
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
	s.recordAuditEvent("variable.create", auditActor(r), "", map[string]interface{}{
		"scope": "repository", "repo": t.key, "variable_name": name,
	})
	writeJSON(w, http.StatusCreated, map[string]interface{}{})
}

func (s *Server) handleGetRepoVariable(w http.ResponseWriter, r *http.Request) {
	t, ok := s.repoVariableTableFor(w, r)
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

// decodeVariablePatch decodes a variable PATCH body and validates the
// optional rename target, writing the 4xx itself on failure.
func decodeVariablePatch(w http.ResponseWriter, r *http.Request) (newName string, value *string, ok bool) {
	var body struct {
		Name  *string `json:"name"`
		Value *string `json:"value"`
	}
	if !decodeJSONBodyOptional(w, r, &body) {
		return "", nil, false
	}
	if body.Name != nil && *body.Name != "" {
		if msg := actionsItemNameError("Variable", *body.Name); msg != "" {
			writeGHError(w, http.StatusUnprocessableEntity, msg)
			return "", nil, false
		}
		newName = strings.ToUpper(*body.Name)
	}
	return newName, body.Value, true
}

// writeVariablePatchStatus maps a variableTable.patch status to the
// response, returning true when the patch was applied.
func writeVariablePatchStatus(w http.ResponseWriter, status int) bool {
	switch status {
	case http.StatusNotFound:
		writeGHError(w, http.StatusNotFound, "Not Found")
		return false
	case http.StatusConflict:
		writeGHError(w, http.StatusConflict, "Variable already exists")
		return false
	default:
		w.WriteHeader(http.StatusNoContent)
		return true
	}
}

func (s *Server) handlePatchRepoVariable(w http.ResponseWriter, r *http.Request) {
	t, ok := s.repoVariableTableFor(w, r)
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
		s.recordAuditEvent("variable.update", auditActor(r), "", map[string]interface{}{
			"scope": "repository", "repo": t.key, "variable_name": name,
		})
	}
}

func (s *Server) handleDeleteRepoVariable(w http.ResponseWriter, r *http.Request) {
	t, ok := s.repoVariableTableFor(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("name"))
	if !t.remove(name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.recordAuditEvent("variable.destroy", auditActor(r), "", map[string]interface{}{
		"scope": "repository", "repo": t.key, "variable_name": name,
	})
	w.WriteHeader(http.StatusNoContent)
}

// handleListRepoOrgVariables lists the organization variables visible to
// a repository (GET /repos/{owner}/{repo}/actions/organization-variables).
// The documented item shape is the plain actions-variable — visibility
// metadata stays on the org endpoints.
func (s *Server) handleListRepoOrgVariables(w http.ResponseWriter, r *http.Request) {
	repo := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	list := make([]map[string]interface{}, 0)
	if org := s.store.GetOrg(r.PathValue("owner")); org != nil {
		s.store.mu.RLock()
		m := s.store.OrgVariables[org.Login]
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

// --- environment variables ---

func (s *Server) envVariableTableFor(w http.ResponseWriter, r *http.Request) (variableTable, string, string, bool) {
	repoKey, scopeKey, envName, ok := s.resolveEnvScope(w, r)
	if !ok {
		return variableTable{}, "", "", false
	}
	return variableTable{s, "env_variables", scopeKey}, repoKey, envName, true
}

func (s *Server) handleListEnvVariables(w http.ResponseWriter, r *http.Request) {
	t, _, _, ok := s.envVariableTableFor(w, r)
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

func (s *Server) handleCreateEnvVariable(w http.ResponseWriter, r *http.Request) {
	t, repoKey, envName, ok := s.envVariableTableFor(w, r)
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
	s.recordAuditEvent("variable.create", auditActor(r), "", map[string]interface{}{
		"scope": "environment", "repo": repoKey, "environment": envName, "variable_name": name,
	})
	writeJSON(w, http.StatusCreated, map[string]interface{}{})
}

func (s *Server) handleGetEnvVariable(w http.ResponseWriter, r *http.Request) {
	t, _, _, ok := s.envVariableTableFor(w, r)
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

func (s *Server) handlePatchEnvVariable(w http.ResponseWriter, r *http.Request) {
	t, repoKey, envName, ok := s.envVariableTableFor(w, r)
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
		s.recordAuditEvent("variable.update", auditActor(r), "", map[string]interface{}{
			"scope": "environment", "repo": repoKey, "environment": envName, "variable_name": name,
		})
	}
}

func (s *Server) handleDeleteEnvVariable(w http.ResponseWriter, r *http.Request) {
	t, repoKey, envName, ok := s.envVariableTableFor(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("name"))
	if !t.remove(name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.recordAuditEvent("variable.destroy", auditActor(r), "", map[string]interface{}{
		"scope": "environment", "repo": repoKey, "environment": envName, "variable_name": name,
	})
	w.WriteHeader(http.StatusNoContent)
}

// --- organization variables ---

func (s *Server) handleListOrgVariables(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	t := variableTable{s, "org_variables", org.Login}
	base := s.baseURL(r)
	vars := t.list()
	list := make([]map[string]interface{}, 0, len(vars))
	for _, v := range vars {
		list = append(list, orgVariableJSON(v, org.Login, base))
	}
	writeVariablesList(w, list)
}

func (s *Server) handleCreateOrgVariable(w http.ResponseWriter, r *http.Request) {
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

	name := strings.ToUpper(body.Name)
	now := time.Now().UTC()
	t := variableTable{s, "org_variables", org.Login}
	v := &ActionsVariable{
		Name: name, Value: body.Value,
		Visibility: body.Visibility, SelectedRepoIDs: ids,
		CreatedAt: now, UpdatedAt: now,
	}
	if !t.create(v) {
		writeGHError(w, http.StatusConflict, "Variable already exists")
		return
	}
	s.recordAuditEvent("variable.create", auditActor(r), org.Login, map[string]interface{}{
		"scope": "organization", "org": org.Login, "variable_name": name, "visibility": body.Visibility,
	})
	writeJSON(w, http.StatusCreated, map[string]interface{}{})
}

func (s *Server) handleGetOrgVariable(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	t := variableTable{s, "org_variables", org.Login}
	v := t.get(strings.ToUpper(r.PathValue("name")))
	if v == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, orgVariableJSON(v, org.Login, s.baseURL(r)))
}

func (s *Server) handlePatchOrgVariable(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	t := variableTable{s, "org_variables", org.Login}
	if name, applied := s.patchOrgScopedVariable(w, r, t.patch); applied {
		s.recordAuditEvent("variable.update", auditActor(r), org.Login, map[string]interface{}{
			"scope": "organization", "org": org.Login, "variable_name": name,
		})
	}
}

// patchOrgScopedVariable implements the organization-variable PATCH body
// (value, visibility, selected repository ids, optional rename) shared by
// the Actions and Copilot coding agent surfaces. patch is the scope
// table's locked mutate-or-rename core; the upper-cased variable name and
// whether the patch applied are returned so the caller can audit.
func (s *Server) patchOrgScopedVariable(w http.ResponseWriter, r *http.Request,
	patch func(name, newName string, apply func(*ActionsVariable)) int) (string, bool) {
	var body struct {
		Name                  *string `json:"name"`
		Value                 *string `json:"value"`
		Visibility            *string `json:"visibility"`
		SelectedRepositoryIDs []int   `json:"selected_repository_ids"`
	}
	if !decodeJSONBodyOptional(w, r, &body) {
		return "", false
	}
	newName := ""
	if body.Name != nil && *body.Name != "" {
		if msg := actionsItemNameError("Variable", *body.Name); msg != "" {
			writeGHError(w, http.StatusUnprocessableEntity, msg)
			return "", false
		}
		newName = strings.ToUpper(*body.Name)
	}
	if body.Visibility != nil && !validOrgItemVisibility(*body.Visibility) {
		writeGHError(w, http.StatusUnprocessableEntity, "visibility must be one of: all, private, selected")
		return "", false
	}

	name := strings.ToUpper(r.PathValue("name"))
	status := patch(name, newName, func(v *ActionsVariable) {
		if body.Value != nil {
			v.Value = *body.Value
		}
		if body.Visibility != nil {
			v.Visibility = *body.Visibility
		}
		if v.Visibility != "selected" {
			v.SelectedRepoIDs = nil
		} else if body.SelectedRepositoryIDs != nil {
			v.SelectedRepoIDs = append([]int(nil), body.SelectedRepositoryIDs...)
		}
	})
	return name, writeVariablePatchStatus(w, status)
}

func (s *Server) handleDeleteOrgVariable(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("name"))
	t := variableTable{s, "org_variables", org.Login}
	if !t.remove(name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.recordAuditEvent("variable.destroy", auditActor(r), org.Login, map[string]interface{}{
		"scope": "organization", "org": org.Login, "variable_name": name,
	})
	w.WriteHeader(http.StatusNoContent)
}

// --- organization variable selected-repositories endpoints ---
//
// Unlike org secrets, the variables spec documents 409 Conflict on the
// list/set endpoints too when visibility is not "selected".

func (s *Server) handleListOrgVariableRepos(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("name"))

	s.store.mu.RLock()
	v := s.store.OrgVariables[org.Login][name]
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

func (s *Server) handleSetOrgVariableRepos(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("name"))
	s.setOrgItemSelectedRepos(w, r, name, true,
		func() orgScopedItem {
			if v := s.store.OrgVariables[org.Login][name]; v != nil {
				return v
			}
			return nil
		},
		func() {
			if s.store.persist != nil {
				s.store.persist.MustPut("org_variables", org.Login, s.store.OrgVariables[org.Login])
			}
		})
}

// orgVariableSelectionChange adapts the shared per-repo add/remove core
// (handleOrgSelectionChange) to the org-variables table.
func (s *Server) orgVariableSelectionChange(w http.ResponseWriter, r *http.Request, add bool) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("name"))
	s.handleOrgSelectionChange(w, r, name, add,
		func() orgScopedItem {
			if v := s.store.OrgVariables[org.Login][name]; v != nil {
				return v
			}
			return nil
		},
		func() {
			if s.store.persist != nil {
				s.store.persist.MustPut("org_variables", org.Login, s.store.OrgVariables[org.Login])
			}
		})
}

func (s *Server) handleAddOrgVariableRepo(w http.ResponseWriter, r *http.Request) {
	s.orgVariableSelectionChange(w, r, true)
}

func (s *Server) handleRemoveOrgVariableRepo(w http.ResponseWriter, r *http.Request) {
	s.orgVariableSelectionChange(w, r, false)
}
