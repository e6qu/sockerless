package bleephub

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Secret represents an Actions secret at any scope (repository or
// environment; OrgSecret embeds it for the organization scope).
//
// Value carries a real json name so persistence round-trips it (workflow
// runs need the plaintext after a restart). Client responses never marshal
// this struct — the secrets handlers emit name/created_at/updated_at maps,
// matching real GitHub's never-return-the-value contract. On the wire the
// value only ever arrives as a libsodium sealed box ({encrypted_value,
// key_id} against the key from the public-key endpoint); the server opens
// the box once at PUT time and stores the plaintext for job injection.
type Secret struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Value     string    `json:"value"`
}

// envScopeKey keys Store.EnvSecrets / Store.EnvVariables. Environment
// scopes are per (repository, environment name); the unit separator can
// appear in neither an "owner/repo" key nor an environment name, so the
// pair packs into one collision-free string. Deliberately NOT NUL: these
// composites are also persistence bucket keys and must stay text-safe.
func envScopeKey(repoKey, envName string) string {
	return repoKey + "\x1f" + envName
}

// actionsItemNameRe is real GitHub's name rule for Actions secrets and
// variables: alphanumeric or underscores only, not starting with a digit.
var actionsItemNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// actionsItemNameError validates a secret/variable name against real
// GitHub's rules. kind is "Secret" or "Variable" (for the message).
// Returns "" when the name is valid.
func actionsItemNameError(kind, name string) string {
	if !actionsItemNameRe.MatchString(name) {
		return kind + " names can only contain alphanumeric characters or underscores and must not start with a number."
	}
	if strings.HasPrefix(strings.ToUpper(name), "GITHUB_") {
		return kind + " names must not start with the GITHUB_ prefix."
	}
	return ""
}

// validOrgItemVisibility reports whether v is a legal organization-level
// secret/variable visibility.
func validOrgItemVisibility(v string) bool {
	switch v {
	case "all", "private", "selected":
		return true
	}
	return false
}

// auditActor names the acting user for audit events; installation tokens
// have no user, so the actor may be empty.
func auditActor(r *http.Request) string {
	if u := ghUserFromContext(r.Context()); u != nil {
		return u.Login
	}
	return ""
}

func (s *Server) registerSecretsRoutes() {
	// Repository scope.
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/secrets", s.requirePerm(scopeSecrets, permRead, s.handleListSecrets))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/secrets/public-key", s.requirePerm(scopeSecrets, permRead, s.handleGetRepoSecretsPublicKey))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/secrets/{secret_name}", s.requirePerm(scopeSecrets, permRead, s.handleGetSecret))
	s.route("PUT /api/v3/repos/{owner}/{repo}/actions/secrets/{secret_name}", s.requirePerm(scopeSecrets, permWrite, s.handlePutSecret))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/actions/secrets/{secret_name}", s.requirePerm(scopeSecrets, permWrite, s.handleDeleteSecret))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/organization-secrets", s.requirePerm(scopeSecrets, permRead, s.handleListRepoOrgSecrets))

	// Environment scope.
	s.route("GET /api/v3/repos/{owner}/{repo}/environments/{env_name}/secrets", s.requirePerm(scopeSecrets, permRead, s.handleListEnvSecrets))
	s.route("GET /api/v3/repos/{owner}/{repo}/environments/{env_name}/secrets/public-key", s.requirePerm(scopeSecrets, permRead, s.handleGetEnvSecretsPublicKey))
	s.route("GET /api/v3/repos/{owner}/{repo}/environments/{env_name}/secrets/{secret_name}", s.requirePerm(scopeSecrets, permRead, s.handleGetEnvSecret))
	s.route("PUT /api/v3/repos/{owner}/{repo}/environments/{env_name}/secrets/{secret_name}", s.requirePerm(scopeSecrets, permWrite, s.handlePutEnvSecret))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/environments/{env_name}/secrets/{secret_name}", s.requirePerm(scopeSecrets, permWrite, s.handleDeleteEnvSecret))

	// Organization scope.
	s.route("GET /api/v3/orgs/{org}/actions/secrets", s.requirePerm(scopeSecrets, permRead, s.handleListOrgSecrets))
	s.route("GET /api/v3/orgs/{org}/actions/secrets/public-key", s.requirePerm(scopeSecrets, permRead, s.handleGetOrgSecretsPublicKey))
	s.route("GET /api/v3/orgs/{org}/actions/secrets/{secret_name}", s.requirePerm(scopeSecrets, permRead, s.handleGetOrgSecret))
	s.route("PUT /api/v3/orgs/{org}/actions/secrets/{secret_name}", s.requirePerm(scopeSecrets, permWrite, s.handlePutOrgSecret))
	s.route("DELETE /api/v3/orgs/{org}/actions/secrets/{secret_name}", s.requirePerm(scopeSecrets, permWrite, s.handleDeleteOrgSecret))
	s.route("GET /api/v3/orgs/{org}/actions/secrets/{secret_name}/repositories", s.requirePerm(scopeSecrets, permRead, s.handleListOrgSecretRepos))
	s.route("PUT /api/v3/orgs/{org}/actions/secrets/{secret_name}/repositories", s.requirePerm(scopeSecrets, permWrite, s.handleSetOrgSecretRepos))
	s.route("PUT /api/v3/orgs/{org}/actions/secrets/{secret_name}/repositories/{repository_id}", s.requirePerm(scopeSecrets, permWrite, s.handleAddOrgSecretRepo))
	s.route("DELETE /api/v3/orgs/{org}/actions/secrets/{secret_name}/repositories/{repository_id}", s.requirePerm(scopeSecrets, permWrite, s.handleRemoveOrgSecretRepo))

	s.registerVariablesRoutes()
}

// --- shared pieces ---

// writeActionsPublicKey serves the sealed-box public key. Every scope's
// public-key endpoint returns the same pair: like a GHES instance,
// bleephub has one Actions encryption key.
func (s *Server) writeActionsPublicKey(w http.ResponseWriter) {
	kp, err := s.store.ActionsKeyPair()
	if err != nil {
		s.logger.Error().Err(err).Msg("actions secrets keypair unavailable")
		writeGHError(w, http.StatusInternalServerError, "Internal Server Error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"key_id": kp.KeyID,
		"key":    kp.PublicKey,
	})
}

// decryptSealedSecret enforces the real secrets wire contract: the body's
// key_id must name the server's current key and encrypted_value must be a
// libsodium sealed box against it. Writes a GitHub-style 422 and returns
// false on any violation.
func (s *Server) decryptSealedSecret(w http.ResponseWriter, encryptedValue, keyID string) (string, bool) {
	if encryptedValue == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "encrypted_value is required")
		return "", false
	}
	if keyID == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "key_id is required")
		return "", false
	}
	kp, err := s.store.ActionsKeyPair()
	if err != nil {
		s.logger.Error().Err(err).Msg("actions secrets keypair unavailable")
		writeGHError(w, http.StatusInternalServerError, "Internal Server Error")
		return "", false
	}
	if keyID != kp.KeyID {
		writeGHError(w, http.StatusUnprocessableEntity, "key_id does not match the current public key; fetch the public-key endpoint and re-encrypt")
		return "", false
	}
	plain, err := s.store.OpenSealedSecret(encryptedValue)
	if err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "encrypted_value could not be decrypted: it must be a libsodium sealed box for the announced public key")
		return "", false
	}
	return plain, true
}

func secretJSON(sec *Secret) map[string]interface{} {
	return map[string]interface{}{
		"name":       sec.Name,
		"created_at": sec.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": sec.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// sortedSecretsJSON renders a scope's secrets sorted by name. Call with
// the store lock held (it only reads).
func sortedSecretsJSON(m map[string]*Secret) []map[string]interface{} {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]map[string]interface{}, 0, len(names))
	for _, n := range names {
		out = append(out, secretJSON(m[n]))
	}
	return out
}

// sealedSecretBody is the request body real clients PUT for repository
// and environment secrets.
type sealedSecretBody struct {
	EncryptedValue string `json:"encrypted_value"`
	KeyID          string `json:"key_id"`
}

// upsertSecret creates or updates a secret in one scope map, persisting
// the scope's collection. Returns true when the secret was created.
func (s *Server) upsertSecret(table map[string]map[string]*Secret, bucket, key, name, value string) bool {
	now := time.Now().UTC()
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	m := table[key]
	if m == nil {
		m = make(map[string]*Secret)
		table[key] = m
	}
	existing := m[name]
	if existing != nil {
		existing.Value = value
		existing.UpdatedAt = now
	} else {
		m[name] = &Secret{Name: name, CreatedAt: now, UpdatedAt: now, Value: value}
	}
	if s.store.persist != nil {
		s.store.persist.MustPut(bucket, key, m)
	}
	return existing == nil
}

// deleteSecret removes a secret from one scope map, persisting the
// remaining collection (or deleting the row when empty). Returns whether
// the secret existed.
func (s *Server) deleteSecret(table map[string]map[string]*Secret, bucket, key, name string) bool {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	m, ok := table[key]
	if !ok || m[name] == nil {
		return false
	}
	delete(m, name)
	if s.store.persist != nil {
		if len(m) > 0 {
			s.store.persist.MustPut(bucket, key, m)
		} else {
			s.store.persist.MustDelete(bucket, key)
		}
	}
	return true
}

// --- repository secrets ---

func (s *Server) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")

	s.store.mu.RLock()
	list := sortedSecretsJSON(s.store.RepoSecrets[repoKey])
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": len(list),
		"secrets":     list,
	})
}

func (s *Server) handleGetRepoSecretsPublicKey(w http.ResponseWriter, _ *http.Request) {
	s.writeActionsPublicKey(w)
}

func (s *Server) handleGetSecret(w http.ResponseWriter, r *http.Request) {
	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	name := strings.ToUpper(r.PathValue("secret_name"))

	s.store.mu.RLock()
	sec := s.store.RepoSecrets[repoKey][name]
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

func (s *Server) handlePutSecret(w http.ResponseWriter, r *http.Request) {
	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
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

	created := s.upsertSecret(s.store.RepoSecrets, "repo_secrets", repoKey, name, plain)
	s.recordAuditEvent("secret.create", auditActor(r), "", map[string]interface{}{
		"scope": "repository", "repo": repoKey, "secret_name": name,
	})
	if created {
		writeJSON(w, http.StatusCreated, map[string]interface{}{})
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	name := strings.ToUpper(r.PathValue("secret_name"))

	s.deleteSecret(s.store.RepoSecrets, "repo_secrets", repoKey, name)
	s.recordAuditEvent("secret.destroy", auditActor(r), "", map[string]interface{}{
		"scope": "repository", "repo": repoKey, "secret_name": name,
	})
	w.WriteHeader(http.StatusNoContent)
}

// handleListRepoOrgSecrets lists the organization secrets visible to a
// repository (GET /repos/{owner}/{repo}/actions/organization-secrets).
// The documented item shape is the plain actions-secret — visibility
// metadata stays on the org endpoints.
func (s *Server) handleListRepoOrgSecrets(w http.ResponseWriter, r *http.Request) {
	repo := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	list := make([]map[string]interface{}, 0)
	if org := s.store.GetOrg(r.PathValue("owner")); org != nil {
		s.store.mu.RLock()
		visible := make(map[string]*Secret)
		for name, sec := range s.store.OrgSecrets[org.Login] {
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

// --- environment secrets ---

// resolveEnvScope resolves an environment-scoped secrets/variables
// request to its repo key and scope key, writing a 404 when the repo or
// the environment does not exist. Real GitHub never auto-creates an
// environment through this surface — a PUT against a missing environment
// is a 404, so the sim must hold that line too.
func (s *Server) resolveEnvScope(w http.ResponseWriter, r *http.Request) (repoKey, scopeKey, envName string, ok bool) {
	repo := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return "", "", "", false
	}
	envName = r.PathValue("env_name")
	if s.store.Deployments.GetEnvironment(repo.ID, envName) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return "", "", "", false
	}
	return repo.FullName, envScopeKey(repo.FullName, envName), envName, true
}

func (s *Server) handleListEnvSecrets(w http.ResponseWriter, r *http.Request) {
	_, scopeKey, _, ok := s.resolveEnvScope(w, r)
	if !ok {
		return
	}

	s.store.mu.RLock()
	list := sortedSecretsJSON(s.store.EnvSecrets[scopeKey])
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": len(list),
		"secrets":     list,
	})
}

func (s *Server) handleGetEnvSecretsPublicKey(w http.ResponseWriter, r *http.Request) {
	if _, _, _, ok := s.resolveEnvScope(w, r); !ok {
		return
	}
	s.writeActionsPublicKey(w)
}

func (s *Server) handleGetEnvSecret(w http.ResponseWriter, r *http.Request) {
	_, scopeKey, _, ok := s.resolveEnvScope(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))

	s.store.mu.RLock()
	sec := s.store.EnvSecrets[scopeKey][name]
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

func (s *Server) handlePutEnvSecret(w http.ResponseWriter, r *http.Request) {
	repoKey, scopeKey, envName, ok := s.resolveEnvScope(w, r)
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

	created := s.upsertSecret(s.store.EnvSecrets, "env_secrets", scopeKey, name, plain)
	s.recordAuditEvent("secret.create", auditActor(r), "", map[string]interface{}{
		"scope": "environment", "repo": repoKey, "environment": envName, "secret_name": name,
	})
	if created {
		writeJSON(w, http.StatusCreated, map[string]interface{}{})
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleDeleteEnvSecret(w http.ResponseWriter, r *http.Request) {
	repoKey, scopeKey, envName, ok := s.resolveEnvScope(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))

	if !s.deleteSecret(s.store.EnvSecrets, "env_secrets", scopeKey, name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.recordAuditEvent("secret.destroy", auditActor(r), "", map[string]interface{}{
		"scope": "environment", "repo": repoKey, "environment": envName, "secret_name": name,
	})
	w.WriteHeader(http.StatusNoContent)
}

// --- organization secrets ---

// resolveOrgForActions resolves {org} or writes a 404.
func (s *Server) resolveOrgForActions(w http.ResponseWriter, r *http.Request) (*Org, bool) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	return org, true
}

// orgSecretJSON renders the organization-actions-secret shape;
// selected_repositories_url appears only for visibility "selected", as
// on real GitHub.
func orgSecretJSON(sec *OrgSecret, orgLogin, baseURL string) map[string]interface{} {
	out := secretJSON(&sec.Secret)
	out["visibility"] = sec.Visibility
	if sec.Visibility == "selected" {
		out["selected_repositories_url"] = baseURL + "/api/v3/orgs/" + orgLogin + "/actions/secrets/" + sec.Name + "/repositories"
	}
	return out
}

func (s *Server) handleListOrgSecrets(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	base := s.baseURL(r)

	s.store.mu.RLock()
	m := s.store.OrgSecrets[org.Login]
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	list := make([]map[string]interface{}, 0, len(names))
	for _, n := range names {
		list = append(list, orgSecretJSON(m[n], org.Login, base))
	}
	s.store.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": len(list),
		"secrets":     list,
	})
}

func (s *Server) handleGetOrgSecretsPublicKey(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.resolveOrgForActions(w, r); !ok {
		return
	}
	s.writeActionsPublicKey(w)
}

func (s *Server) handleGetOrgSecret(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))
	base := s.baseURL(r)

	s.store.mu.RLock()
	sec := s.store.OrgSecrets[org.Login][name]
	var body map[string]interface{}
	if sec != nil {
		body = orgSecretJSON(sec, org.Login, base)
	}
	s.store.mu.RUnlock()

	if body == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) handlePutOrgSecret(w http.ResponseWriter, r *http.Request) {
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

	now := time.Now().UTC()
	s.store.mu.Lock()
	m := s.store.OrgSecrets[org.Login]
	if m == nil {
		m = make(map[string]*OrgSecret)
		s.store.OrgSecrets[org.Login] = m
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
		s.store.persist.MustPut("org_secrets", org.Login, m)
	}
	s.store.mu.Unlock()

	s.recordAuditEvent("secret.create", auditActor(r), org.Login, map[string]interface{}{
		"scope": "organization", "org": org.Login, "secret_name": name, "visibility": body.Visibility,
	})
	if existing == nil {
		writeJSON(w, http.StatusCreated, map[string]interface{}{})
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleDeleteOrgSecret(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))

	s.store.mu.Lock()
	m := s.store.OrgSecrets[org.Login]
	existed := m[name] != nil
	if existed {
		delete(m, name)
		if s.store.persist != nil {
			if len(m) > 0 {
				s.store.persist.MustPut("org_secrets", org.Login, m)
			} else {
				s.store.persist.MustDelete("org_secrets", org.Login)
			}
		}
	}
	s.store.mu.Unlock()

	if !existed {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.recordAuditEvent("secret.destroy", auditActor(r), org.Login, map[string]interface{}{
		"scope": "organization", "org": org.Login, "secret_name": name,
	})
	w.WriteHeader(http.StatusNoContent)
}

// writeSelectedReposResponse renders {total_count, repositories} for the
// org secret/variable selected-repositories endpoints, dropping ids whose
// repository no longer exists.
func (s *Server) writeSelectedReposResponse(w http.ResponseWriter, r *http.Request, ids []int) {
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
		out = append(out, repoToJSON(repo, s.store, base))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count":  len(out),
		"repositories": out,
	})
}

// repoIDPathValue parses {repository_id}; non-numeric ids are 404s (the
// resource cannot exist).
func repoIDPathValue(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("repository_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return 0, false
	}
	return id, true
}

func (s *Server) handleListOrgSecretRepos(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))

	s.store.mu.RLock()
	sec := s.store.OrgSecrets[org.Login][name]
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

func (s *Server) handleSetOrgSecretRepos(w http.ResponseWriter, r *http.Request) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))
	s.setOrgItemSelectedRepos(w, r, name, false,
		func() orgScopedItem {
			if sec := s.store.OrgSecrets[org.Login][name]; sec != nil {
				return sec
			}
			return nil
		},
		func() {
			if s.store.persist != nil {
				s.store.persist.MustPut("org_secrets", org.Login, s.store.OrgSecrets[org.Login])
			}
		})
}

// setOrgItemSelectedRepos implements the set-selected-repositories
// endpoints (PUT .../{name}/repositories) shared by organization secrets
// and organization variables across the Actions and Copilot coding agent
// surfaces: 404 for a missing item or an unknown repository id, and —
// where the surface documents it (variables) — 409 unless the item's
// visibility is "selected". lookup and persistLocked run under the store
// write lock, mirroring handleOrgSelectionChange.
func (s *Server) setOrgItemSelectedRepos(w http.ResponseWriter, r *http.Request, name string, requireSelected bool,
	lookup func() orgScopedItem, persistLocked func()) {
	var body struct {
		SelectedRepositoryIDs []int `json:"selected_repository_ids"`
	}
	if !decodeJSONBody(w, r, &body) {
		return
	}

	s.store.mu.Lock()
	item := lookup()
	if item == nil {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if requireSelected && item.itemVisibility() != "selected" {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusConflict, "Conflict: visibility of "+name+" is not set to selected")
		return
	}
	for _, id := range body.SelectedRepositoryIDs {
		if s.store.Repos[id] == nil {
			s.store.mu.Unlock()
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
	}
	item.setSelectedIDs(append([]int(nil), body.SelectedRepositoryIDs...))
	item.touchUpdated(time.Now().UTC())
	persistLocked()
	s.store.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

// orgScopedItem abstracts the selected-repositories surface shared by
// organization secrets and organization variables, so the per-repo
// add/remove endpoints run through one core.
type orgScopedItem interface {
	itemVisibility() string
	selectedIDs() []int
	setSelectedIDs([]int)
	touchUpdated(time.Time)
}

func (sec *OrgSecret) itemVisibility() string     { return sec.Visibility }
func (sec *OrgSecret) selectedIDs() []int         { return sec.SelectedRepoIDs }
func (sec *OrgSecret) setSelectedIDs(ids []int)   { sec.SelectedRepoIDs = ids }
func (sec *OrgSecret) touchUpdated(now time.Time) { sec.UpdatedAt = now }
func (v *ActionsVariable) itemVisibility() string { return v.Visibility }
func (v *ActionsVariable) selectedIDs() []int     { return v.SelectedRepoIDs }
func (v *ActionsVariable) setSelectedIDs(ids []int) {
	v.SelectedRepoIDs = ids
}
func (v *ActionsVariable) touchUpdated(now time.Time) { v.UpdatedAt = now }

// handleOrgSelectionChange implements the per-repository add/remove
// endpoints (PUT/DELETE .../{name}/repositories/{repository_id}) for both
// org secrets and org variables: 404 for a missing item or (on add) an
// unknown repository, 409 unless the item's visibility is "selected".
// lookup and persistLocked run under the store write lock.
func (s *Server) handleOrgSelectionChange(w http.ResponseWriter, r *http.Request, name string, add bool,
	lookup func() orgScopedItem, persistLocked func()) {
	id, ok := repoIDPathValue(w, r)
	if !ok {
		return
	}

	s.store.mu.Lock()
	item := lookup()
	if item == nil {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if item.itemVisibility() != "selected" {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusConflict, "Conflict: visibility of "+name+" is not set to selected")
		return
	}
	if add && s.store.Repos[id] == nil {
		s.store.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	ids := item.selectedIDs()
	changed := false
	if add {
		present := false
		for _, existing := range ids {
			if existing == id {
				present = true
				break
			}
		}
		if !present {
			ids = append(ids, id)
			changed = true
		}
	} else {
		kept := ids[:0]
		for _, existing := range ids {
			if existing != id {
				kept = append(kept, existing)
			}
		}
		if len(kept) != len(ids) {
			ids = kept
			changed = true
		}
	}
	if changed {
		item.setSelectedIDs(ids)
		item.touchUpdated(time.Now().UTC())
		persistLocked()
	}
	s.store.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

// orgSecretSelectionChange adapts the shared core to the org-secrets table.
func (s *Server) orgSecretSelectionChange(w http.ResponseWriter, r *http.Request, add bool) {
	org, ok := s.resolveOrgForActions(w, r)
	if !ok {
		return
	}
	name := strings.ToUpper(r.PathValue("secret_name"))
	s.handleOrgSelectionChange(w, r, name, add,
		func() orgScopedItem {
			if sec := s.store.OrgSecrets[org.Login][name]; sec != nil {
				return sec
			}
			return nil
		},
		func() {
			if s.store.persist != nil {
				s.store.persist.MustPut("org_secrets", org.Login, s.store.OrgSecrets[org.Login])
			}
		})
}

func (s *Server) handleAddOrgSecretRepo(w http.ResponseWriter, r *http.Request) {
	s.orgSecretSelectionChange(w, r, true)
}

func (s *Server) handleRemoveOrgSecretRepo(w http.ResponseWriter, r *http.Request) {
	s.orgSecretSelectionChange(w, r, false)
}
