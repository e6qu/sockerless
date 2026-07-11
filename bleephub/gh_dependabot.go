package bleephub

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerGHDependabotRoutes() {
	// Alerts
	s.route("GET /api/v3/repos/{owner}/{repo}/dependabot/alerts",
		s.requirePerm(scopeSecurityEvents, permRead, s.handleListDependabotAlerts))
	s.route("GET /api/v3/repos/{owner}/{repo}/dependabot/alerts/{alert_number}",
		s.requirePerm(scopeSecurityEvents, permRead, s.handleGetDependabotAlert))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/dependabot/alerts/{alert_number}",
		s.requirePerm(scopeSecurityEvents, permWrite, s.handleUpdateDependabotAlert))

	// Repository-scoped secrets
	s.route("GET /api/v3/repos/{owner}/{repo}/dependabot/secrets",
		s.requirePerm(scopeDependabotSecrets, permRead, s.handleListDependabotRepoSecrets))
	s.route("GET /api/v3/repos/{owner}/{repo}/dependabot/secrets/public-key",
		s.requirePerm(scopeDependabotSecrets, permRead, s.handleGetDependabotRepoSecretsPublicKey))
	s.route("GET /api/v3/repos/{owner}/{repo}/dependabot/secrets/{secret_name}",
		s.requirePerm(scopeDependabotSecrets, permRead, s.handleGetDependabotRepoSecret))
	s.route("PUT /api/v3/repos/{owner}/{repo}/dependabot/secrets/{secret_name}",
		s.requirePerm(scopeDependabotSecrets, permWrite, s.handlePutDependabotRepoSecret))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/dependabot/secrets/{secret_name}",
		s.requirePerm(scopeDependabotSecrets, permWrite, s.handleDeleteDependabotRepoSecret))

	// Organization-scoped secrets
	s.route("GET /api/v3/orgs/{org}/dependabot/secrets",
		s.requirePerm(scopeDependabotSecrets, permRead, s.handleListDependabotOrgSecrets))
	s.route("GET /api/v3/orgs/{org}/dependabot/secrets/public-key",
		s.requirePerm(scopeDependabotSecrets, permRead, s.handleGetDependabotOrgSecretsPublicKey))
	s.route("GET /api/v3/orgs/{org}/dependabot/secrets/{secret_name}",
		s.requirePerm(scopeDependabotSecrets, permRead, s.handleGetDependabotOrgSecret))
	s.route("PUT /api/v3/orgs/{org}/dependabot/secrets/{secret_name}",
		s.requirePerm(scopeDependabotSecrets, permWrite, s.handlePutDependabotOrgSecret))
	s.route("DELETE /api/v3/orgs/{org}/dependabot/secrets/{secret_name}",
		s.requirePerm(scopeDependabotSecrets, permWrite, s.handleDeleteDependabotOrgSecret))
	s.route("GET /api/v3/orgs/{org}/dependabot/secrets/{secret_name}/repositories",
		s.requirePerm(scopeDependabotSecrets, permRead, s.handleListDependabotOrgSecretRepos))
	s.route("PUT /api/v3/orgs/{org}/dependabot/secrets/{secret_name}/repositories",
		s.requirePerm(scopeDependabotSecrets, permWrite, s.handleSetDependabotOrgSecretRepos))
	s.route("PUT /api/v3/orgs/{org}/dependabot/secrets/{secret_name}/repositories/{repository_id}",
		s.requirePerm(scopeDependabotSecrets, permWrite, s.handleAddDependabotOrgSecretRepo))
	s.route("DELETE /api/v3/orgs/{org}/dependabot/secrets/{secret_name}/repositories/{repository_id}",
		s.requirePerm(scopeDependabotSecrets, permWrite, s.handleRemoveDependabotOrgSecretRepo))

	// Organization-level alerts and repository access
	s.route("GET /api/v3/orgs/{org}/dependabot/alerts",
		s.requireOrgAdmin(scopeSecurityEvents, permRead, s.handleListDependabotOrgAlerts))
	s.route("GET /api/v3/orgs/{org}/dependabot/repository-access",
		s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleGetDependabotRepositoryAccess))
	s.route("PATCH /api/v3/orgs/{org}/dependabot/repository-access",
		s.requireOrgAdmin(scopeOrgAdministration, permWrite, s.handleUpdateDependabotRepositoryAccess))
	s.route("PUT /api/v3/orgs/{org}/dependabot/repository-access/default-level",
		s.requireOrgAdmin(scopeOrgAdministration, permWrite, s.handleSetDependabotRepositoryAccessDefaultLevel))
}

// --- alerts ---

func (s *Server) handleListDependabotAlerts(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}

	q := r.URL.Query()
	state := q.Get("state")
	severity := q.Get("severity")
	packageName := q.Get("package_name")
	ecosystem := q.Get("ecosystem")
	manifest := q.Get("manifest")
	sort := q.Get("sort")
	direction := q.Get("direction")

	alerts := s.store.ListDependabotAlerts(repo.FullName, state, severity, packageName, ecosystem, manifest, sort, direction)
	page := paginateAndLink(w, r, alerts)
	baseURL := s.baseURL(r)
	out := make([]map[string]interface{}, len(page))
	for i, a := range page {
		out[i] = dependabotAlertToJSON(a, baseURL, repo)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetDependabotAlert(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}

	a := s.lookupDependabotAlert(w, r, repo)
	if a == nil {
		return
	}
	writeJSON(w, http.StatusOK, dependabotAlertToJSON(a, s.baseURL(r), repo))
}

func (s *Server) handleUpdateDependabotAlert(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	user := ghUserFromContext(r.Context())
	if !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	a := s.lookupDependabotAlert(w, r, repo)
	if a == nil {
		return
	}

	var req struct {
		State            string `json:"state"`
		DismissedReason  string `json:"dismissed_reason"`
		DismissedComment string `json:"dismissed_comment"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.State == "" {
		writeGHValidationError(w, "DependabotAlert", "state", "missing_field")
		return
	}
	if err := s.store.UpdateDependabotAlert(a, req.State, req.DismissedReason, req.DismissedComment, user); err != nil {
		writeGHValidationError(w, "DependabotAlert", "state", "invalid")
		return
	}
	writeJSON(w, http.StatusOK, dependabotAlertToJSON(a, s.baseURL(r), repo))
}

func (s *Server) lookupDependabotAlert(w http.ResponseWriter, r *http.Request, repo *Repo) *DependabotAlert {
	number, err := strconv.Atoi(r.PathValue("alert_number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	a := s.store.GetDependabotAlert(repo.FullName, number)
	if a == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return a
}

func dependabotAlertToJSON(a *DependabotAlert, baseURL string, repo *Repo) map[string]interface{} {
	apiURL := fmt.Sprintf("%s/api/v3/repos/%s/dependabot/alerts/%d", baseURL, repo.FullName, a.Number)
	htmlURL := fmt.Sprintf("%s/%s/security/dependabot/%d", baseURL, repo.FullName, a.Number)
	published := a.CreatedAt.UTC().Format(time.RFC3339)
	updated := a.UpdatedAt.UTC().Format(time.RFC3339)

	var dismissedAt, fixedAt, autoDismissedAt interface{} = nil, nil, nil
	if a.DismissedAt != nil {
		dismissedAt = a.DismissedAt.UTC().Format(time.RFC3339)
	}
	if a.FixedAt != nil {
		fixedAt = a.FixedAt.UTC().Format(time.RFC3339)
	}
	if a.AutoDismissedAt != nil {
		autoDismissedAt = a.AutoDismissedAt.UTC().Format(time.RFC3339)
	}

	var firstPatched interface{} = nil
	if a.FirstPatchedVersion != "" {
		firstPatched = map[string]interface{}{"identifier": a.FirstPatchedVersion}
	}

	identifiers := []map[string]interface{}{
		{"type": "GHSA", "value": a.VulnerabilityID},
	}
	if a.CVEID != "" {
		identifiers = append(identifiers, map[string]interface{}{"type": "CVE", "value": a.CVEID})
	}

	pkg := map[string]interface{}{
		"ecosystem": a.PackageEcosystem,
		"name":      a.PackageName,
	}

	return map[string]interface{}{
		"number":     a.Number,
		"state":      a.State,
		"url":        apiURL,
		"html_url":   htmlURL,
		"created_at": published,
		"updated_at": updated,
		"dependency": map[string]interface{}{
			"package":       pkg,
			"manifest_path": a.ManifestPath,
			"scope":         nil,
			"relationship":  nil,
		},
		"security_advisory": map[string]interface{}{
			"ghsa_id":     a.VulnerabilityID,
			"cve_id":      nullOrString(a.CVEID),
			"summary":     a.Summary,
			"description": a.Description,
			"vulnerabilities": []map[string]interface{}{
				{
					"package":                  pkg,
					"severity":                 a.Severity,
					"vulnerable_version_range": a.VulnerableVersionRange,
					"first_patched_version":    firstPatched,
				},
			},
			"severity":    a.Severity,
			"cvss":        map[string]interface{}{"score": 0.0, "vector_string": nil},
			"cwes":        []map[string]interface{}{},
			"identifiers": identifiers,
			"references": []map[string]interface{}{
				{"url": "https://github.com/advisories/" + a.VulnerabilityID},
			},
			"published_at": published,
			"updated_at":   updated,
			"withdrawn_at": nil,
		},
		"security_vulnerability": map[string]interface{}{
			"package":                  pkg,
			"severity":                 a.Severity,
			"vulnerable_version_range": a.VulnerableVersionRange,
			"first_patched_version":    firstPatched,
		},
		"dismissed_at":      dismissedAt,
		"dismissed_by":      nil,
		"dismissed_reason":  nullOrString(a.DismissedReason),
		"dismissed_comment": nullOrString(a.DismissedComment),
		"fixed_at":          fixedAt,
		"auto_dismissed_at": autoDismissedAt,
	}
}

// --- repository secrets ---

func (s *Server) handleListDependabotRepoSecrets(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.store.mu.RLock()
	list := sortedDependabotSecretsJSON(s.store.DependabotSecrets[repo.FullName])
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": len(list),
		"secrets":     list,
	})
}

func (s *Server) handleGetDependabotRepoSecretsPublicKey(w http.ResponseWriter, _ *http.Request) {
	s.writeActionsPublicKey(w)
}

func (s *Server) handleGetDependabotRepoSecret(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))

	s.store.mu.RLock()
	sec := s.store.DependabotSecrets[repo.FullName][name]
	var body map[string]interface{}
	if sec != nil {
		body = dependabotSecretJSON(sec)
	}
	s.store.mu.RUnlock()

	if body == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) handlePutDependabotRepoSecret(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	rawName := r.PathValue("secret_name")
	if msg := actionsItemNameError("Secret", rawName); msg != "" {
		writeGHError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	name := strings.ToUpper(rawName)

	var body struct {
		EncryptedValue string `json:"encrypted_value"`
		KeyID          string `json:"key_id"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if ok := s.validateDependabotSecretKeyID(w, body.KeyID); !ok {
		return
	}
	if body.EncryptedValue == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "encrypted_value is required")
		return
	}

	created := s.store.UpsertDependabotSecret(repo.FullName, name, body.EncryptedValue, body.KeyID)
	s.recordAuditEvent("dependabot_secret.create", auditActor(r), "", map[string]interface{}{
		"scope": "repository", "repo": repo.FullName, "secret_name": name,
	})
	if created {
		writeJSON(w, http.StatusCreated, map[string]interface{}{})
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleDeleteDependabotRepoSecret(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))

	if !s.store.DeleteDependabotSecret(repo.FullName, name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.recordAuditEvent("dependabot_secret.destroy", auditActor(r), "", map[string]interface{}{
		"scope": "repository", "repo": repo.FullName, "secret_name": name,
	})
	w.WriteHeader(http.StatusNoContent)
}

// --- organization secrets ---

func (s *Server) resolveOrgForDependabot(w http.ResponseWriter, r *http.Request) (*Org, bool) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return org, true
}

func (s *Server) handleListDependabotOrgSecrets(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForDependabot(w, r)
	if !ok {
		return
	}
	base := s.baseURL(r)

	s.store.mu.RLock()
	m := s.store.DependabotOrgSecrets[org.Login]
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	list := make([]map[string]interface{}, 0, len(names))
	for _, n := range names {
		list = append(list, dependabotOrgSecretJSON(m[n], org.Login, base))
	}
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": len(list),
		"secrets":     list,
	})
}

func (s *Server) handleGetDependabotOrgSecretsPublicKey(w http.ResponseWriter, _ *http.Request) {
	s.writeActionsPublicKey(w)
}

func (s *Server) handleGetDependabotOrgSecret(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForDependabot(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))
	base := s.baseURL(r)

	s.store.mu.RLock()
	sec := s.store.DependabotOrgSecrets[org.Login][name]
	var body map[string]interface{}
	if sec != nil {
		body = dependabotOrgSecretJSON(sec, org.Login, base)
	}
	s.store.mu.RUnlock()

	if body == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) handlePutDependabotOrgSecret(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForDependabot(w, r)
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
		EncryptedValue        string `json:"encrypted_value"`
		KeyID                 string `json:"key_id"`
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
	if ok := s.validateDependabotSecretKeyID(w, body.KeyID); !ok {
		return
	}
	if body.EncryptedValue == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "encrypted_value is required")
		return
	}

	ids := body.SelectedRepositoryIDs
	for _, id := range ids {
		if s.store.GetRepoByID(id) == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
	}

	created := s.store.UpsertDependabotOrgSecret(org.Login, name, body.EncryptedValue, body.KeyID, body.Visibility, ids)
	s.recordAuditEvent("dependabot_secret.create", auditActor(r), org.Login, map[string]interface{}{
		"scope": "organization", "org": org.Login, "secret_name": name, "visibility": body.Visibility,
	})
	if created {
		writeJSON(w, http.StatusCreated, map[string]interface{}{})
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleDeleteDependabotOrgSecret(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForDependabot(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))

	if !s.store.DeleteDependabotOrgSecret(org.Login, name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.recordAuditEvent("dependabot_secret.destroy", auditActor(r), org.Login, map[string]interface{}{
		"scope": "organization", "org": org.Login, "secret_name": name,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListDependabotOrgSecretRepos(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForDependabot(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))

	s.store.mu.RLock()
	sec := s.store.DependabotOrgSecrets[org.Login][name]
	var ids []int
	if sec != nil {
		ids = append([]int(nil), sec.SelectedRepoIDs...)
	}
	s.store.mu.RUnlock()

	if sec == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.writeDependabotSelectedReposResponse(w, r, ids)
}

func (s *Server) handleSetDependabotOrgSecretRepos(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForDependabot(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))

	var body struct {
		SelectedRepositoryIDs []int `json:"selected_repository_ids"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}

	s.store.mu.RLock()
	sec := s.store.DependabotOrgSecrets[org.Login][name]
	s.store.mu.RUnlock()
	if sec == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	for _, id := range body.SelectedRepositoryIDs {
		if s.store.GetRepoByID(id) == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
	}

	s.store.SetDependabotOrgSecretSelectedRepos(org.Login, name, body.SelectedRepositoryIDs)
	w.WriteHeader(http.StatusNoContent)
}

// --- shared helpers ---

func (s *Server) validateDependabotSecretKeyID(w http.ResponseWriter, keyID string) bool {
	if keyID == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "key_id is required")
		return false
	}
	kp, err := s.store.ActionsKeyPair()
	if err != nil {
		s.logger.Error().Err(err).Msg("dependabot secrets keypair unavailable")
		writeGHError(w, http.StatusInternalServerError, "Internal Server Error")
		return false
	}
	if keyID != kp.KeyID {
		writeGHError(w, http.StatusUnprocessableEntity, "key_id does not match the current public key; fetch the public-key endpoint and re-encrypt")
		return false
	}
	return true
}

func dependabotSecretJSON(sec *DependabotSecret) map[string]interface{} {
	return map[string]interface{}{
		"name":       sec.Name,
		"created_at": sec.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": sec.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func dependabotOrgSecretJSON(sec *DependabotOrgSecret, orgLogin, baseURL string) map[string]interface{} {
	out := dependabotSecretJSON(&sec.DependabotSecret)
	out["visibility"] = sec.Visibility
	if sec.Visibility == "selected" {
		out["selected_repositories_url"] = baseURL + "/api/v3/orgs/" + orgLogin + "/dependabot/secrets/" + sec.Name + "/repositories"
	}
	return out
}

func sortedDependabotSecretsJSON(m map[string]*DependabotSecret) []map[string]interface{} {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]map[string]interface{}, 0, len(names))
	for _, n := range names {
		out = append(out, dependabotSecretJSON(m[n]))
	}
	return out
}

func (s *Server) writeDependabotSelectedReposResponse(w http.ResponseWriter, r *http.Request, ids []int) {
	s.store.mu.RLock()
	repos := make([]*Repo, 0, len(ids))
	for _, id := range ids {
		if repo := s.store.Repos[id]; repo != nil {
			repos = append(repos, repo)
		}
	}
	s.store.mu.RUnlock()
	sort.Slice(repos, func(i, j int) bool { return repos[i].ID < repos[j].ID })

	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(repos))
	for _, repo := range repos {
		out = append(out, dependabotMinimalRepoJSON(repo, s.store, base))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count":  len(out),
		"repositories": out,
	})
}

// dependabotMinimalRepoJSON renders a minimal-repository-compatible shape so
// the selected-repositories response passes the OpenAPI validator.
func dependabotMinimalRepoJSON(repo *Repo, st *Store, baseURL string) map[string]interface{} {
	out := repoToJSON(repo, st, baseURL)
	delete(out, "has_pull_requests")
	return out
}

// --- org alerts and repository access ---

func (s *Server) handleListDependabotOrgAlerts(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForDependabot(w, r)
	if !ok {
		return
	}

	alerts := s.store.ListDependabotAlertsByOrg(org.ID)
	page := paginateAndLink(w, r, alerts)
	baseURL := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(page))
	for _, a := range page {
		repo := s.store.ReposByName[a.RepoKey]
		if repo == nil {
			continue
		}
		alertJSON := dependabotAlertToJSON(a, baseURL, repo)
		alertJSON["repository"] = simpleRepoJSON(repo, s.store, baseURL)
		out = append(out, alertJSON)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetDependabotRepositoryAccess(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForDependabot(w, r)
	if !ok {
		return
	}
	ids := s.store.GetDependabotRepositoryAccess(org.Login)
	repos := s.dependabotAccessibleRepos(r, ids)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"default_level":           s.store.GetDependabotRepositoryAccessDefaultLevel(org.Login),
		"accessible_repositories": repos,
	})
}

// handleSetDependabotRepositoryAccessDefaultLevel implements
// PUT /orgs/{org}/dependabot/repository-access/default-level.
func (s *Server) handleSetDependabotRepositoryAccessDefaultLevel(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForDependabot(w, r)
	if !ok {
		return
	}
	var body struct {
		DefaultLevel string `json:"default_level"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if body.DefaultLevel != "public" && body.DefaultLevel != "internal" {
		writeGHValidationError(w, "DependabotRepositoryAccess", "default_level", "invalid")
		return
	}
	s.store.SetDependabotRepositoryAccessDefaultLevel(org.Login, body.DefaultLevel)
	w.WriteHeader(http.StatusNoContent)
}

// GetDependabotRepositoryAccessDefaultLevel returns the org's default
// repository access level for Dependabot updates ("public" until changed).
func (st *Store) GetDependabotRepositoryAccessDefaultLevel(orgLogin string) string {
	st.mu.RLock()
	defer st.mu.RUnlock()
	if level := st.DependabotRepoAccessDefaultLevel[orgLogin]; level != "" {
		return level
	}
	return "public"
}

// SetDependabotRepositoryAccessDefaultLevel stores the org's default
// repository access level for Dependabot updates.
func (st *Store) SetDependabotRepositoryAccessDefaultLevel(orgLogin, level string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.DependabotRepoAccessDefaultLevel[orgLogin] = level
	if st.persist != nil {
		st.persist.MustPut("dependabot_repo_access_default_level", orgLogin, level)
	}
}

// ─── org Dependabot secret selected-repository add/remove ────────────────

func (sec *DependabotOrgSecret) itemVisibility() string     { return sec.Visibility }
func (sec *DependabotOrgSecret) selectedIDs() []int         { return sec.SelectedRepoIDs }
func (sec *DependabotOrgSecret) setSelectedIDs(ids []int)   { sec.SelectedRepoIDs = ids }
func (sec *DependabotOrgSecret) touchUpdated(now time.Time) { sec.UpdatedAt = now }

// dependabotOrgSecretSelectionChange adapts the shared per-repository
// selection core to the org Dependabot secrets table.
func (s *Server) dependabotOrgSecretSelectionChange(w http.ResponseWriter, r *http.Request, add bool) {
	org, ok := s.resolveOrgForDependabot(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))
	s.handleOrgSelectionChange(w, r, name, add,
		func() orgScopedItem {
			if sec := s.store.DependabotOrgSecrets[org.Login][name]; sec != nil {
				return sec
			}
			return nil
		},
		func() {
			if s.store.persist != nil {
				s.store.persist.MustPut("dependabot_org_secrets", org.Login, s.store.DependabotOrgSecrets[org.Login])
			}
		})
}

func (s *Server) handleAddDependabotOrgSecretRepo(w http.ResponseWriter, r *http.Request) {
	s.dependabotOrgSecretSelectionChange(w, r, true)
}

func (s *Server) handleRemoveDependabotOrgSecretRepo(w http.ResponseWriter, r *http.Request) {
	s.dependabotOrgSecretSelectionChange(w, r, false)
}

func (s *Server) handleUpdateDependabotRepositoryAccess(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForDependabot(w, r)
	if !ok {
		return
	}

	var body struct {
		RepositoryIDsToAdd    []int `json:"repository_ids_to_add"`
		RepositoryIDsToRemove []int `json:"repository_ids_to_remove"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}

	existing := s.store.GetDependabotRepositoryAccess(org.Login)
	set := make(map[int]struct{}, len(existing))
	for _, id := range existing {
		set[id] = struct{}{}
	}
	for _, id := range body.RepositoryIDsToAdd {
		if s.store.GetRepoByID(id) == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		set[id] = struct{}{}
	}
	for _, id := range body.RepositoryIDsToRemove {
		delete(set, id)
	}

	ids := make([]int, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	s.store.SetDependabotRepositoryAccess(org.Login, ids)
	w.WriteHeader(http.StatusNoContent)
}

// --- user secrets (commented out: user-scoped Dependabot secrets are not a
// real GitHub REST surface; only repo/org-scoped secrets are documented) ---

// func (s *Server) handleListDependabotUserSecrets(w http.ResponseWriter, r *http.Request) {
// 	username := r.PathValue("username")
// 	list := make([]map[string]interface{}, 0)
// 	for _, sec := range s.store.ListDependabotUserSecrets(username) {
// 		list = append(list, dependabotSecretJSON(&sec.DependabotSecret))
// 	}
// 	writeJSON(w, http.StatusOK, map[string]interface{}{
// 		"total_count": len(list),
// 		"secrets":     list,
// 	})
// }

// func (s *Server) handleGetDependabotUserSecretsPublicKey(w http.ResponseWriter, _ *http.Request) {
// 	s.writeActionsPublicKey(w)
// }

// func (s *Server) handleGetDependabotUserSecret(w http.ResponseWriter, r *http.Request) {
// 	username := r.PathValue("username")
// 	name := strings.ToUpper(r.PathValue("secret_name"))

// 	sec := s.store.GetDependabotUserSecret(username, name)
// 	if sec == nil {
// 		writeGHError(w, http.StatusNotFound, "Not Found")
// 		return
// 	}
// 	writeJSON(w, http.StatusOK, dependabotSecretJSON(&sec.DependabotSecret))
// }

// func (s *Server) handlePutDependabotUserSecret(w http.ResponseWriter, r *http.Request) {
// 	username := r.PathValue("username")
// 	rawName := r.PathValue("secret_name")
// 	if msg := actionsItemNameError("Secret", rawName); msg != "" {
// 		writeGHError(w, http.StatusUnprocessableEntity, msg)
// 		return
// 	}
// 	name := strings.ToUpper(rawName)

// 	var body struct {
// 		EncryptedValue string `json:"encrypted_value"`
// 		KeyID          string `json:"key_id"`
// 	}
// 	if !decodeJSONBody(w, r, &body) {
// 		return
// 	}
// 	if ok := s.validateDependabotSecretKeyID(w, body.KeyID); !ok {
// 		return
// 	}
// 	if body.EncryptedValue == "" {
// 		writeGHError(w, http.StatusUnprocessableEntity, "encrypted_value is required")
// 		return
// 	}

// 	created := s.store.UpsertDependabotUserSecret(username, name, body.EncryptedValue, body.KeyID)
// 	s.recordAuditEvent("dependabot_secret.create", auditActor(r), "", map[string]interface{}{
// 		"scope": "user", "user": username, "secret_name": name,
// 	})
// 	if created {
// 		writeJSON(w, http.StatusCreated, map[string]interface{}{})
// 	} else {
// 		w.WriteHeader(http.StatusNoContent)
// 	}
// }

// func (s *Server) handleDeleteDependabotUserSecret(w http.ResponseWriter, r *http.Request) {
// 	username := r.PathValue("username")
// 	name := strings.ToUpper(r.PathValue("secret_name"))

// 	if !s.store.DeleteDependabotUserSecret(username, name) {
// 		writeGHError(w, http.StatusNotFound, "Not Found")
// 		return
// 	}
// 	s.recordAuditEvent("dependabot_secret.destroy", auditActor(r), "", map[string]interface{}{
// 		"scope": "user", "user": username, "secret_name": name,
// 	})
// 	w.WriteHeader(http.StatusNoContent)
// }

// dependabotAccessibleRepos returns a sorted array of repository JSON objects
// for the given repository IDs, omitting IDs that do not resolve.
func (s *Server) dependabotAccessibleRepos(r *http.Request, ids []int) []map[string]interface{} {
	s.store.mu.RLock()
	repos := make([]*Repo, 0, len(ids))
	for _, id := range ids {
		if repo := s.store.Repos[id]; repo != nil {
			repos = append(repos, repo)
		}
	}
	s.store.mu.RUnlock()
	sort.Slice(repos, func(i, j int) bool { return repos[i].ID < repos[j].ID })

	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(repos))
	for _, repo := range repos {
		out = append(out, simpleRepoJSON(repo, s.store, base))
	}
	return out
}
