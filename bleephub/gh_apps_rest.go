package bleephub

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
)

func (s *Server) registerGHAppsRoutes() {
	// GitHub App API endpoints
	s.mux.HandleFunc("POST /api/v3/app-manifests/{code}/conversions", s.handleManifestConversion)
	s.mux.HandleFunc("GET /api/v3/app", s.handleGetAuthenticatedApp)
	s.mux.HandleFunc("GET /api/v3/apps/{app_slug}", s.handleGetAppBySlug)
	s.mux.HandleFunc("GET /api/v3/app/installations", s.handleListAppInstallations)
	s.mux.HandleFunc("GET /api/v3/app/installations/{id}", s.handleGetAppInstallation)
	s.mux.HandleFunc("POST /api/v3/app/installations/{id}/access_tokens", s.handleCreateInstallationToken)
	s.mux.HandleFunc("DELETE /api/v3/app/installations/{id}", s.handleDeleteAppInstallation)
	s.mux.HandleFunc("PUT /api/v3/app/installations/{id}/suspended", s.handleSuspendInstallation)
	s.mux.HandleFunc("DELETE /api/v3/app/installations/{id}/suspended", s.handleUnsuspendInstallation)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/installation", s.handleGetRepoInstallation)
	s.mux.HandleFunc("GET /api/v3/orgs/{org}/installation", s.handleGetOrgInstallation)
	s.mux.HandleFunc("GET /api/v3/users/{username}/installation", s.handleGetUserInstallation)

	// Phase 132 — installations from the authenticated user's perspective.
	s.mux.HandleFunc("GET /api/v3/user/installations", s.handleListUserInstallations)
	s.mux.HandleFunc("GET /api/v3/user/installations/{id}/repositories", s.handleListUserInstallationRepos)
	s.mux.HandleFunc("PUT /api/v3/user/installations/{id}/repositories/{repo_id}", s.handleAddUserInstallationRepo)
	s.mux.HandleFunc("DELETE /api/v3/user/installations/{id}/repositories/{repo_id}", s.handleRemoveUserInstallationRepo)
	s.mux.HandleFunc("DELETE /api/v3/installation/token", s.handleRevokeInstallationToken)

	// Phase 153 — installation-token-scoped repositories list.
	s.mux.HandleFunc("GET /api/v3/installation/repositories", s.handleListInstallationRepositories)

	// Management endpoints for testing
	s.mux.HandleFunc("POST /api/v3/bleephub/apps", s.handleCreateApp)
	s.mux.HandleFunc("POST /api/v3/bleephub/apps/{app_id}/installations", s.handleCreateInstallationMgmt)
}

func (s *Server) handleManifestConversion(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	appID, ok := s.store.ConsumeManifestCode(code)
	if !ok {
		writeGHError(w, http.StatusNotFound, "Manifest code not found or already used")
		return
	}
	app := s.store.GetApp(appID)
	if app == nil {
		writeGHError(w, http.StatusNotFound, "App not found")
		return
	}
	writeJSON(w, http.StatusCreated, appToJSON(app, true))
}

func (s *Server) handleGetAuthenticatedApp(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	writeJSON(w, http.StatusOK, appToJSON(app, false))
}

func (s *Server) handleListAppInstallations(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	installations := s.store.ListAppInstallations(app.ID)
	result := make([]map[string]interface{}, 0, len(installations))
	for _, inst := range installations {
		result = append(result, installationToJSON(inst))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetAppInstallation(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid installation ID")
		return
	}
	inst := s.store.GetInstallation(id)
	if inst == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if inst.AppID != app.ID {
		writeGHError(w, http.StatusForbidden, "Installation does not belong to this app")
		return
	}
	writeJSON(w, http.StatusOK, installationToJSON(inst))
}

func (s *Server) handleCreateInstallationToken(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid installation ID")
		return
	}
	inst := s.store.GetInstallation(id)
	if inst == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if inst.AppID != app.ID {
		writeGHError(w, http.StatusForbidden, "Installation does not belong to this app")
		return
	}

	// Optional permissions + repo-subset override from request body
	perms := inst.Permissions
	var repoIDs []int
	var body struct {
		Permissions   map[string]string `json:"permissions"`
		RepositoryIDs []int             `json:"repository_ids"`
		Repositories  []string          `json:"repositories"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if body.Permissions != nil {
				perms = body.Permissions
			}
			repoIDs = body.RepositoryIDs
			if len(repoIDs) == 0 && len(body.Repositories) > 0 {
				for _, name := range body.Repositories {
					if repo := s.store.GetRepo(inst.TargetLogin, name); repo != nil {
						repoIDs = append(repoIDs, repo.ID)
					}
				}
			}
		}
	}

	if inst.SuspendedAt != nil {
		writeGHError(w, http.StatusForbidden, "This installation has been suspended.")
		return
	}

	token := s.store.CreateInstallationToken(inst.ID, app.ID, perms, repoIDs)
	writeJSON(w, http.StatusCreated, installationTokenToJSON(token))
}

func (s *Server) handleDeleteAppInstallation(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid installation ID")
		return
	}
	inst := s.store.GetInstallation(id)
	if inst == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if inst.AppID != app.ID {
		writeGHError(w, http.StatusForbidden, "Installation does not belong to this app")
		return
	}
	s.store.DeleteInstallation(id)
	s.emitInstallationEvent(app, "deleted", inst)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetRepoInstallation(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	owner := r.PathValue("owner")
	inst := s.store.GetRepoInstallation(owner)
	if inst == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, installationToJSON(inst))
}

// Management endpoints

func (s *Server) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	var req struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Permissions map[string]string `json:"permissions"`
		Events      []string          `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.Name == "" {
		writeGHValidationError(w, "App", "name", "missing_field")
		return
	}

	app := s.store.CreateApp(user.ID, req.Name, req.Description, req.Permissions, req.Events)
	writeJSON(w, http.StatusCreated, appToJSON(app, true))
}

func (s *Server) handleCreateInstallationMgmt(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	appID, err := strconv.Atoi(r.PathValue("app_id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid app ID")
		return
	}
	app := s.store.GetApp(appID)
	if app == nil {
		writeGHError(w, http.StatusNotFound, "App not found")
		return
	}

	var req struct {
		TargetType  string            `json:"target_type"`
		TargetID    int               `json:"target_id"`
		TargetLogin string            `json:"target_login"`
		Permissions map[string]string `json:"permissions"`
		Events      []string          `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.TargetType == "" {
		req.TargetType = "User"
	}

	inst := s.store.CreateInstallation(appID, req.TargetType, req.TargetID, req.TargetLogin, req.Permissions, req.Events)
	if inst != nil {
		s.emitInstallationEvent(app, "created", inst)
	}
	writeJSON(w, http.StatusCreated, installationToJSON(inst))
}

// JSON serializers

func appToJSON(app *App, includePEM bool) map[string]interface{} {
	result := map[string]interface{}{
		"id":                  app.ID,
		"node_id":             app.NodeID,
		"slug":                app.Slug,
		"name":                app.Name,
		"client_id":           app.ClientID,
		"description":         app.Description,
		"external_url":        app.ExternalURL,
		"html_url":            "https://github.com/apps/" + app.Slug,
		"events_url":          "/api/v3/apps/" + app.Slug + "/events",
		"hooks_url":           "/api/v3/app/hook/deliveries",
		"installations_url":   "/api/v3/app/installations",
		"permissions":         app.Permissions,
		"events":              app.Events,
		"installations_count": 0,
		"created_at":          app.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":          app.UpdatedAt.UTC().Format(time.RFC3339),
		"owner": map[string]interface{}{
			"login":      "admin",
			"id":         app.OwnerID,
			"type":       "User",
			"html_url":   "/admin",
			"avatar_url": "",
		},
	}
	if includePEM {
		result["pem"] = app.PEMPrivateKey
		result["client_secret"] = app.ClientSecret
		result["webhook_secret"] = app.WebhookSecret
	}
	return result
}

func installationToJSON(inst *Installation) map[string]interface{} {
	if inst == nil {
		return nil
	}
	out := map[string]interface{}{
		"id":                        inst.ID,
		"app_id":                    inst.AppID,
		"app_slug":                  inst.AppSlug,
		"target_type":               inst.TargetType,
		"target_id":                 inst.TargetID,
		"permissions":               inst.Permissions,
		"events":                    inst.Events,
		"repository_selection":      inst.RepositorySelection,
		"single_file_name":          inst.SingleFileName,
		"has_multiple_single_files": false,
		"single_file_paths":         []string{},
		"created_at":                inst.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":                inst.UpdatedAt.UTC().Format(time.RFC3339),
		"account": map[string]interface{}{
			"login":      inst.TargetLogin,
			"id":         inst.TargetID,
			"type":       inst.TargetType,
			"html_url":   "/" + inst.TargetLogin,
			"avatar_url": "",
		},
		"html_url":          "/apps/" + inst.AppSlug + "/installations/" + strconv.Itoa(inst.ID),
		"access_tokens_url": "/api/v3/app/installations/" + strconv.Itoa(inst.ID) + "/access_tokens",
		"repositories_url":  "/api/v3/installation/repositories",
		"events_url":        "/api/v3/apps/" + inst.AppSlug + "/installations/" + strconv.Itoa(inst.ID) + "/events",
		"suspended_at":      nil,
		"suspended_by":      nil,
	}
	if inst.SuspendedAt != nil {
		out["suspended_at"] = inst.SuspendedAt.UTC().Format(time.RFC3339)
		if inst.SuspendedBy != nil {
			out["suspended_by"] = map[string]interface{}{
				"login": inst.SuspendedBy.Login,
				"id":    inst.SuspendedBy.ID,
				"type":  inst.SuspendedBy.Type,
			}
		}
	}
	return out
}

func installationTokenToJSON(token *InstallationToken) map[string]interface{} {
	return map[string]interface{}{
		"token":       token.Token,
		"expires_at":  token.ExpiresAt.UTC().Format(time.RFC3339),
		"permissions": token.Permissions,
	}
}

// handleListUserInstallations — GET /api/v3/user/installations.
// Real GitHub: scoped to installations the authenticated user has
// access to. Bleephub returns every installation since users are
// unscoped in the sim. Auth required (must have a user token).
func (s *Server) handleListUserInstallations(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	s.store.mu.RLock()
	all := make([]*Installation, 0, len(s.store.Installations))
	for _, inst := range s.store.Installations {
		all = append(all, inst)
	}
	s.store.mu.RUnlock()

	page := paginateAndLink(w, r, all)
	installations := make([]map[string]interface{}, 0, len(page))
	for _, inst := range page {
		installations = append(installations, installationToJSON(inst))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count":   len(all),
		"installations": installations,
	})
}

// handleListUserInstallationRepos — GET /api/v3/user/installations/{id}/repositories.
// Returns repos accessible via this installation. With
// `RepositorySelection=all` (the default in bleephub's CreateInstallation),
// this returns every repo owned by the installation's target login.
// Real GitHub additionally supports `selected` selection — that path
// would read a per-installation repo allow-list, which bleephub
// doesn't model today; the response just enumerates all owned repos.
func (s *Server) handleListUserInstallationRepos(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid installation ID")
		return
	}
	inst := s.store.GetInstallation(id)
	if inst == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	repos := s.store.ListReposByOwner(inst.TargetLogin)
	page := paginateAndLink(w, r, repos)
	base := s.baseURL(r)
	repoJSON := make([]map[string]interface{}, 0, len(page))
	for _, repo := range page {
		repoJSON = append(repoJSON, repoToJSON(repo, base))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count":          len(repos),
		"repository_selection": inst.RepositorySelection,
		"repositories":         repoJSON,
	})
}

// handleGetAppBySlug — GET /api/v3/apps/{app_slug}.
// Real GitHub: anonymous-readable public app lookup. Returns the public
// fields (no PEM, no client_secret). 404 when the slug doesn't match.
func (s *Server) handleGetAppBySlug(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("app_slug")
	app := s.store.GetAppBySlug(slug)
	if app == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, appToJSON(app, false))
}

// handleSuspendInstallation — PUT /api/v3/app/installations/{id}/suspended.
// JWT-auth (App). 204 on success, 409 if already suspended.
func (s *Server) handleSuspendInstallation(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid installation ID")
		return
	}
	inst := s.store.GetInstallation(id)
	if inst == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if inst.AppID != app.ID {
		writeGHError(w, http.StatusForbidden, "Installation does not belong to this app")
		return
	}
	if !s.store.SuspendInstallation(id, &User{Login: app.Slug + "[bot]", Type: "Bot", ID: -app.ID}) {
		writeGHError(w, http.StatusConflict, "Installation already suspended")
		return
	}
	s.emitInstallationEvent(app, "suspend", inst)
	w.WriteHeader(http.StatusNoContent)
}

// handleUnsuspendInstallation — DELETE /api/v3/app/installations/{id}/suspended.
// JWT-auth (App). 204 on success, 409 if not suspended.
func (s *Server) handleUnsuspendInstallation(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid installation ID")
		return
	}
	inst := s.store.GetInstallation(id)
	if inst == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if inst.AppID != app.ID {
		writeGHError(w, http.StatusForbidden, "Installation does not belong to this app")
		return
	}
	if !s.store.UnsuspendInstallation(id) {
		writeGHError(w, http.StatusConflict, "Installation not suspended")
		return
	}
	s.emitInstallationEvent(app, "unsuspend", inst)
	w.WriteHeader(http.StatusNoContent)
}

// handleGetOrgInstallation — GET /api/v3/orgs/{org}/installation.
// User-auth. Returns the installation associated with the org (any App's installation
// where target_login = {org}, target_type = "Organization").
func (s *Server) handleGetOrgInstallation(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	org := r.PathValue("org")
	for _, inst := range s.snapshotInstallations() {
		if inst.TargetLogin == org && inst.TargetType == "Organization" {
			writeJSON(w, http.StatusOK, installationToJSON(inst))
			return
		}
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}

// handleGetUserInstallation — GET /api/v3/users/{username}/installation.
// User-auth. Returns the installation associated with the user (target_type = "User").
func (s *Server) handleGetUserInstallation(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	username := r.PathValue("username")
	for _, inst := range s.snapshotInstallations() {
		if inst.TargetLogin == username && inst.TargetType == "User" {
			writeJSON(w, http.StatusOK, installationToJSON(inst))
			return
		}
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}

// handleAddUserInstallationRepo — PUT /api/v3/user/installations/{id}/repositories/{repo_id}.
// User-auth. Adds a repo to a "selected"-mode installation's allow-list. Auto-switches mode
// to "selected" if it was "all" (real GH requires the mode to already be "selected" — bleephub
// is permissive in the sim). 204 on success.
func (s *Server) handleAddUserInstallationRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	instID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid installation ID")
		return
	}
	repoID, err := strconv.Atoi(r.PathValue("repo_id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid repository ID")
		return
	}
	added, ok := s.store.AddInstallationRepo(instID, repoID)
	if !ok {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if inst := s.store.GetInstallation(instID); inst != nil && added {
		if app := s.store.GetApp(inst.AppID); app != nil {
			s.emitInstallationRepositoriesEvent(app, "added", inst, []int{repoID})
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveUserInstallationRepo — DELETE /api/v3/user/installations/{id}/repositories/{repo_id}.
func (s *Server) handleRemoveUserInstallationRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	instID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid installation ID")
		return
	}
	repoID, err := strconv.Atoi(r.PathValue("repo_id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid repository ID")
		return
	}
	removed, ok := s.store.RemoveInstallationRepo(instID, repoID)
	if !ok {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if inst := s.store.GetInstallation(instID); inst != nil && removed {
		if app := s.store.GetApp(inst.AppID); app != nil {
			s.emitInstallationRepositoriesEvent(app, "removed", inst, []int{repoID})
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListInstallationRepositories — GET /api/v3/installation/repositories.
// Installation-token-scoped (ghs_) repo list. Real GitHub: returns the repos the
// installation has access to. When the token was minted with a repository_ids
// subset, only those repos are returned.
func (s *Server) handleListInstallationRepositories(w http.ResponseWriter, r *http.Request) {
	tok := ghInstallationTokenFromContext(r.Context())
	inst := ghInstallationFromContext(r.Context())
	if tok == nil || inst == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	allRepos := s.store.ListReposByOwner(inst.TargetLogin)
	filtered := filterReposBySelection(allRepos, inst, tok)
	page := paginateAndLink(w, r, filtered)
	base := s.baseURL(r)
	repoJSON := make([]map[string]interface{}, 0, len(page))
	for _, repo := range page {
		repoJSON = append(repoJSON, repoToJSON(repo, base))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count":          len(filtered),
		"repository_selection": inst.RepositorySelection,
		"repositories":         repoJSON,
	})
}

// snapshotInstallations returns a slice copy of every installation under
// a single RLock; lets handlers iterate without holding the store lock.
func (s *Server) snapshotInstallations() []*Installation {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	out := make([]*Installation, 0, len(s.store.Installations))
	for _, inst := range s.store.Installations {
		out = append(out, inst)
	}
	return out
}

// filterReposBySelection applies the installation's repository_selection mode
// + token-scoped repository_ids subset.
func filterReposBySelection(all []*Repo, inst *Installation, tok *InstallationToken) []*Repo {
	allowed := map[int]struct{}{}
	if inst.RepositorySelection == "selected" {
		for _, id := range inst.SelectedRepoIDs {
			allowed[id] = struct{}{}
		}
	} else {
		for _, r := range all {
			allowed[r.ID] = struct{}{}
		}
	}
	if len(tok.RepositoryIDs) > 0 {
		narrowed := map[int]struct{}{}
		for _, id := range tok.RepositoryIDs {
			if _, ok := allowed[id]; ok {
				narrowed[id] = struct{}{}
			}
		}
		allowed = narrowed
	}
	out := make([]*Repo, 0, len(all))
	for _, r := range all {
		if _, ok := allowed[r.ID]; ok {
			out = append(out, r)
		}
	}
	return out
}

// emitInstallationEvent fires an `installation` webhook (action one of:
// created | deleted | suspend | unsuspend | new_permissions_accepted) to
// the app's configured webhook URL, and records the delivery on the
// app-level deliveries queue.
func (s *Server) emitInstallationEvent(app *App, action string, inst *Installation) {
	if app == nil || app.WebhookURL == "" || !app.WebhookActive {
		return
	}
	sender := s.store.LookupUserByLogin(inst.TargetLogin)
	payload := buildInstallationEventPayload(app, action, inst, sender)
	go s.deliverAppWebhook(app, "installation", action, inst.ID, mustMarshal(payload))
}

// emitInstallationRepositoriesEvent fires an `installation_repositories`
// webhook (action: added | removed).
func (s *Server) emitInstallationRepositoriesEvent(app *App, action string, inst *Installation, repoIDsChanged []int) {
	if app == nil || app.WebhookURL == "" || !app.WebhookActive {
		return
	}
	sender := s.store.LookupUserByLogin(inst.TargetLogin)
	payload := buildInstallationRepositoriesEventPayload(app, action, inst, repoIDsChanged, sender)
	go s.deliverAppWebhook(app, "installation_repositories", action, inst.ID, mustMarshal(payload))
}

// deliverAppWebhook is the app-level analogue of deliverWebhook: same
// retry shape, but records to AppHookDeliveries.
func (s *Server) deliverAppWebhook(app *App, event, action string, installationID int, payloadBytes []byte) {
	hook := &Webhook{
		ID:     -app.ID, // negative ID flags an app-level hook (middleware reads ID < 0)
		URL:    app.WebhookURL,
		Secret: app.WebhookSecret,
		Events: app.WebhookEvents,
		Active: app.WebhookActive,
	}
	guid := uuid.New().String()
	backoffs := []time.Duration{0, 1 * time.Second, 5 * time.Second}
	for attempt, backoff := range backoffs {
		if attempt > 0 {
			time.Sleep(backoff)
		}
		delivery := s.doDeliverAttempt(hook, event, action, guid, payloadBytes, attempt > 0)
		delivery.AppID = app.ID
		delivery.InstallationID = installationID
		s.store.AddAppDelivery(app.ID, delivery)
		if delivery.StatusCode >= 200 && delivery.StatusCode < 300 {
			return
		}
	}
}

// handleRevokeInstallationToken — DELETE /api/v3/installation/token.
// Real GitHub: 204 No Content; the token used in the request's
// Authorization header is revoked. Auth: must be presented as a
// Bearer ghs_* installation token (the middleware sets ctxInstallation
// when it recognises the prefix). The bare token string is parsed
// from the header so we can drop it from the InstallationTokens map.
func (s *Server) handleRevokeInstallationToken(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	tokenStr := ""
	switch {
	case len(auth) > 6 && auth[:6] == "token ":
		tokenStr = auth[6:]
	case len(auth) > 7 && auth[:7] == "Bearer ":
		tokenStr = auth[7:]
	}
	if tokenStr == "" || tokenStr[:4] != "ghs_" {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	if !s.store.RevokeInstallationToken(tokenStr) {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
