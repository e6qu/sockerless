package bleephub

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Server) registerGHAppsRoutes() {
	// GitHub App application programming interface endpoints.
	s.route("POST /api/v3/app-manifests/{code}/conversions", s.handleManifestConversion)
	s.route("GET /api/v3/app", s.handleGetAuthenticatedApp)
	s.route("GET /api/v3/apps/{app_slug}", s.handleGetAppBySlug)
	s.route("GET /api/v3/app/installations", s.handleListAppInstallations)
	s.route("GET /api/v3/app/installation-requests", s.handleListAppInstallationRequests)
	s.route("GET /api/v3/app/installations/{id}", s.handleGetAppInstallation)
	s.route("POST /api/v3/app/installations/{id}/access_tokens", s.handleCreateInstallationToken)
	s.route("DELETE /api/v3/app/installations/{id}", s.handleDeleteAppInstallation)
	s.route("PUT /api/v3/app/installations/{id}/suspended", s.handleSuspendInstallation)
	s.route("DELETE /api/v3/app/installations/{id}/suspended", s.handleUnsuspendInstallation)
	s.route("GET /api/v3/repos/{owner}/{repo}/installation", s.handleGetRepoInstallation)
	s.route("GET /api/v3/orgs/{org}/installation", s.handleGetOrgInstallation)
	s.route("GET /api/v3/orgs/{org}/installations", s.handleListOrgInstallations)
	s.route("GET /api/v3/users/{username}/installation", s.handleGetUserInstallation)

	// App-manifest submission — the web-flow form post real GitHub serves at
	// github.com/settings/apps/new. Creates the app and 302-redirects to the
	// manifest's redirect_url with the one-time code that
	// POST /api/v3/app-manifests/{code}/conversions redeems.
	s.route("POST /settings/apps/new", s.handleManifestSubmission)
	s.route("GET /settings/apps", s.handleListBrowserGitHubApps)
	s.route("POST /apps/{app_slug}/installations/new", s.handleBrowserInstallApp)
	s.route("POST /settings/apps/{app_slug}/installations/new", s.handleBrowserInstallApp)
	s.route("POST /settings/installations/{id}/suspend", s.handleBrowserSuspendInstallation)
	s.route("POST /settings/installations/{id}/unsuspend", s.handleBrowserUnsuspendInstallation)
	s.route("DELETE /settings/installations/{id}", s.handleBrowserDeleteInstallation)

	s.registerGHAppsUserAndOperatorRoutes()
}

// registerGHAppsUserAndOperatorRoutes mounts the authenticated-user
// installation views and the operator-facing /internal app management
// surface.
func (s *Server) registerGHAppsUserAndOperatorRoutes() {
	// installations from the authenticated user's perspective.
	s.route("GET /api/v3/user/installations", s.handleListUserInstallations)
	s.route("GET /api/v3/user/installations/{id}/repositories", s.handleListUserInstallationRepos)
	s.route("PUT /api/v3/user/installations/{id}/repositories/{repo_id}", s.handleAddUserInstallationRepo)
	s.route("DELETE /api/v3/user/installations/{id}/repositories/{repo_id}", s.handleRemoveUserInstallationRepo)
	s.route("DELETE /api/v3/installation/token", s.handleRevokeInstallationToken)

	// installation-token-scoped repositories list.
	s.route("GET /api/v3/installation/repositories", s.handleListInstallationRepositories)

}

// handleManifestSubmission — POST /settings/apps/new. The browser half of the
// GitHub App Manifest flow: a logged-in user posts a form whose `manifest`
// field carries the app manifest JSON; GitHub registers the app and redirects
// to the manifest's redirect_url with a one-time `code` (echoing the optional
// `state`). The conversion endpoint below redeems the code for credentials.
func (s *Server) handleManifestSubmission(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(s.authenticateRequest(r))
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	if err := r.ParseForm(); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing form")
		return
	}
	var manifest struct {
		Name           string `json:"name"`
		URL            string `json:"url"`
		Description    string `json:"description"`
		RedirectURL    string `json:"redirect_url"`
		HookAttributes struct {
			URL    string `json:"url"`
			Active *bool  `json:"active"`
		} `json:"hook_attributes"`
		DefaultEvents      []string          `json:"default_events"`
		DefaultPermissions map[string]string `json:"default_permissions"`
	}
	raw := r.PostFormValue("manifest")
	if raw == "" {
		writeGHValidationError(w, "AppManifest", "manifest", "missing_field")
		return
	}
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if manifest.Name == "" {
		writeGHValidationError(w, "AppManifest", "name", "missing_field")
		return
	}
	if manifest.RedirectURL == "" {
		writeGHValidationError(w, "AppManifest", "redirect_url", "missing_field")
		return
	}
	redirect, err := url.Parse(manifest.RedirectURL)
	if err != nil {
		writeGHValidationError(w, "AppManifest", "redirect_url", "invalid")
		return
	}
	for scope, level := range manifest.DefaultPermissions {
		if !validPermLevelString(level) {
			writeGHValidationError(w, "AppManifest", "default_permissions."+scope, "invalid")
			return
		}
	}

	app, err := s.store.CreateAppE(user.ID, manifest.Name, manifest.Description, manifest.DefaultPermissions, manifest.DefaultEvents)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if manifest.URL != "" || manifest.HookAttributes.URL != "" {
		s.store.UpdateAppHookConfig(app.ID, func(a *App) {
			if manifest.URL != "" {
				// The manifest's `url` is the app homepage, served back
				// as external_url.
				a.ExternalURL = manifest.URL
			}
			if manifest.HookAttributes.URL != "" {
				a.WebhookURL = manifest.HookAttributes.URL
				if manifest.HookAttributes.Active != nil {
					a.WebhookActive = *manifest.HookAttributes.Active
				}
				a.WebhookEvents = manifest.DefaultEvents
			}
		})
	}

	q := redirect.Query()
	q.Set("code", s.store.RegisterManifestCode(app.ID))
	if state := r.URL.Query().Get("state"); state != "" {
		q.Set("state", state)
	}
	redirect.RawQuery = q.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

// handleListBrowserGitHubApps exposes the signed-in user's GitHub Apps through
// the same settings surface that owns the browser manifest flow.
func (s *Server) handleListBrowserGitHubApps(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(s.authenticateRequest(r))
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	apps := s.snapshotGitHubApps()
	out := make([]map[string]interface{}, 0, len(apps))
	for _, app := range apps {
		if app.OwnerID == user.ID {
			out = append(out, appToJSON(s.store, app, false))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleBrowserInstallApp implements the signed-in browser installation step
// behind GitHub's "Install App" flow. The app's registered default
// permissions/events are the installation grant; the form only chooses the
// target account and all-vs-selected repository access.
func (s *Server) handleBrowserInstallApp(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(s.authenticateRequest(r))
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	app := s.store.GetAppBySlug(r.PathValue("app_slug"))
	if app == nil {
		writeGHError(w, http.StatusNotFound, "App not found")
		return
	}
	if err := r.ParseForm(); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing form")
		return
	}
	targetLogin := r.PostFormValue("target_login")
	if targetLogin == "" {
		targetLogin = user.Login
	}
	targetType, targetID, ok := s.resolveInstallTarget(user, targetLogin)
	if !ok {
		writeGHError(w, http.StatusForbidden, "Must be able to install GitHub Apps on this account")
		return
	}
	for _, existing := range s.store.ListAppInstallations(app.ID) {
		if existing.TargetLogin == targetLogin {
			writeGHValidationError(w, "Installation", "target_login", "already_exists")
			return
		}
	}
	selection := r.PostFormValue("repository_selection")
	if selection == "" {
		selection = "all"
	}
	if selection != "all" && selection != "selected" {
		writeGHValidationError(w, "Installation", "repository_selection", "invalid")
		return
	}
	repoIDs, valid := s.resolveInstallationRepositorySelection(targetLogin, selection, r.PostForm["repository_ids"])
	if !valid {
		writeGHValidationError(w, "Installation", "repository_ids", "invalid")
		return
	}

	inst := s.store.CreateInstallation(app.ID, targetType, targetID, targetLogin, copyInstallationPermissions(app.Permissions), append([]string(nil), app.Events...))
	if inst == nil {
		writeGHError(w, http.StatusNotFound, "App not found")
		return
	}
	if selection == "selected" {
		s.store.SetInstallationRepositorySelection(inst.ID, "selected", repoIDs)
		inst = s.store.GetInstallation(inst.ID)
	}
	s.emitInstallationEvent(app, "created", inst)
	writeJSON(w, http.StatusCreated, installationToJSON(inst))
}

func (s *Server) handleBrowserSuspendInstallation(w http.ResponseWriter, r *http.Request) {
	s.handleBrowserInstallationState(w, r, true)
}

func (s *Server) handleBrowserUnsuspendInstallation(w http.ResponseWriter, r *http.Request) {
	s.handleBrowserInstallationState(w, r, false)
}

func (s *Server) handleBrowserInstallationState(w http.ResponseWriter, r *http.Request, suspend bool) {
	user := ghUserFromContext(s.authenticateRequest(r))
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	inst, ok := s.browserManageableInstallation(w, r, user)
	if !ok {
		return
	}
	var changed bool
	if suspend {
		changed = s.store.SuspendInstallation(inst.ID, user)
	} else {
		changed = s.store.UnsuspendInstallation(inst.ID)
	}
	if !changed {
		writeGHError(w, http.StatusConflict, "Installation state already matches request")
		return
	}
	if app := s.store.GetApp(inst.AppID); app != nil {
		action := "unsuspend"
		if suspend {
			action = "suspend"
		}
		s.emitInstallationEvent(app, action, s.store.GetInstallation(inst.ID))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleBrowserDeleteInstallation(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(s.authenticateRequest(r))
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	inst, ok := s.browserManageableInstallation(w, r, user)
	if !ok {
		return
	}
	s.store.DeleteInstallation(inst.ID)
	if app := s.store.GetApp(inst.AppID); app != nil {
		s.emitInstallationEvent(app, "deleted", inst)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) browserManageableInstallation(w http.ResponseWriter, r *http.Request, user *User) (*Installation, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid installation ID")
		return nil, false
	}
	inst := s.store.GetInstallation(id)
	if inst == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, false
	}
	if inst.TargetLogin == user.Login {
		return inst, true
	}
	if inst.TargetType == "Organization" {
		if m := s.store.GetMembership(inst.TargetLogin, user.ID); m != nil && m.State == MembershipStateActive && m.Role == OrgRoleAdmin {
			return inst, true
		}
	}
	writeGHError(w, http.StatusForbidden, "Must be able to manage GitHub Apps on this account")
	return nil, false
}

func (s *Server) resolveInstallTarget(user *User, targetLogin string) (string, int, bool) {
	if targetLogin == user.Login {
		return "User", user.ID, true
	}
	if org := s.store.GetOrg(targetLogin); org != nil {
		m := s.store.GetMembership(targetLogin, user.ID)
		return "Organization", org.ID, m != nil && m.State == MembershipStateActive && m.Role == OrgRoleAdmin
	}
	return "", 0, false
}

func (s *Server) resolveInstallationRepositorySelection(targetLogin, selection string, rawIDs []string) ([]int, bool) {
	if selection == "all" {
		return nil, true
	}
	if len(rawIDs) == 0 {
		return nil, false
	}
	repoByID := map[int]bool{}
	for _, repo := range s.store.ListReposByOwner(targetLogin) {
		repoByID[repo.ID] = true
	}
	ids := make([]int, 0, len(rawIDs))
	seen := map[int]bool{}
	for _, raw := range rawIDs {
		id, err := strconv.Atoi(raw)
		if err != nil || !repoByID[id] {
			return nil, false
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, true
}

func copyInstallationPermissions(perms map[string]string) map[string]string {
	if perms == nil {
		return nil
	}
	out := make(map[string]string, len(perms))
	for k, v := range perms {
		out[k] = v
	}
	return out
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
	writeJSON(w, http.StatusCreated, appToJSON(s.store, app, true))
}

func (s *Server) handleGetAuthenticatedApp(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	writeJSON(w, http.StatusOK, appToJSON(s.store, app, false))
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

	// Optional permissions + repo-subset override from request body.
	perms := inst.Permissions
	var repoIDs []int
	var body struct {
		Permissions   map[string]string `json:"permissions"`
		RepositoryIDs flexIntSlice      `json:"repository_ids"`
		Repositories  []string          `json:"repositories"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
			writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
			return
		}
	}

	if inst.SuspendedAt != nil {
		writeGHError(w, http.StatusForbidden, "This installation has been suspended.")
		return
	}

	// Requested permissions must be a subset of the installation's grants —
	// real GitHub rejects escalation with 422.
	if body.Permissions != nil {
		if scope, ok := validateRequestedPermissions(body.Permissions, inst.Permissions); !ok {
			writeGHError(w, http.StatusUnprocessableEntity,
				"The permissions requested are not granted to this installation (permission: "+scope+")")
			return
		}
		perms = body.Permissions
	}

	// Repository scoping: every requested repo must exist under the
	// installation target and be accessible to the installation — real
	// GitHub rejects unknown/inaccessible repos with 422.
	accessible := installationAccessibleRepoIDs(s.store, inst)
	for _, rid := range body.RepositoryIDs {
		if _, ok := accessible[rid]; !ok {
			writeGHError(w, http.StatusUnprocessableEntity,
				"There is at least one repository that does not exist or is not accessible to the integration")
			return
		}
		repoIDs = append(repoIDs, rid)
	}
	if len(repoIDs) == 0 {
		for _, name := range body.Repositories {
			repo := s.store.GetRepo(inst.TargetLogin, name)
			if repo == nil {
				writeGHError(w, http.StatusUnprocessableEntity,
					"There is at least one repository that does not exist or is not accessible to the integration")
				return
			}
			if _, ok := accessible[repo.ID]; !ok {
				writeGHError(w, http.StatusUnprocessableEntity,
					"There is at least one repository that does not exist or is not accessible to the integration")
				return
			}
			repoIDs = append(repoIDs, repo.ID)
		}
	}

	token, err := s.store.CreateInstallationTokenE(inst.ID, app.ID, perms, repoIDs)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// When the token is minted with a specific repository subset, real GitHub
	// returns repository_selection="selected" and a `repositories` array of the
	// scoped repos. Resolve the token's repo IDs against the installation's
	// owned repos.
	var scopedRepos []*Repo
	if len(token.RepositoryIDs) > 0 {
		owned := s.store.ListReposByOwner(inst.TargetLogin)
		byID := make(map[int]*Repo, len(owned))
		for _, repo := range owned {
			byID[repo.ID] = repo
		}
		for _, rid := range token.RepositoryIDs {
			if repo := byID[rid]; repo != nil {
				scopedRepos = append(scopedRepos, repo)
			}
		}
	}
	writeJSON(w, http.StatusCreated, installationTokenToJSON(token, inst, scopedRepos, s.store, s.baseURL(r)))
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
	repo := s.store.GetRepo(owner, r.PathValue("repo"))
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	inst := s.store.GetRepoInstallation(owner)
	if inst == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	// A "selected"-mode installation only covers repos on its allow-list;
	// real GitHub 404s for repos outside the selection.
	if _, ok := installationAccessibleRepoIDs(s.store, inst)[repo.ID]; !ok {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, installationToJSON(inst))
}

// JSON serializers

func appToJSON(st *Store, app *App, includePEM bool) map[string]interface{} {
	result := map[string]interface{}{
		"id":                  app.ID,
		"node_id":             app.NodeID,
		"slug":                app.Slug,
		"name":                app.Name,
		"client_id":           app.ClientID,
		"description":         app.Description,
		"external_url":        app.ExternalURL,
		"html_url":            "https://github.com/apps/" + app.Slug,
		"permissions":         app.Permissions,
		"events":              app.Events,
		"installations_count": st.CountAppInstallations(app.ID),
		"created_at":          app.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":          app.UpdatedAt.UTC().Format(time.RFC3339),
		"owner":               appOwnerJSON(st, app),
	}
	if includePEM {
		result["pem"] = app.PEMPrivateKey
		result["client_secret"] = app.ClientSecret
		result["webhook_secret"] = app.WebhookSecret
	}
	return result
}

// appOwnerJSON serializes the GitHub App's owning account as a Simple User,
// matching the `owner` object real GitHub returns on GET /app and
// GET /apps/{slug}. App loading and creation validate OwnerID, so a missing
// owner is corrupt state and is exposed as null rather than a fabricated user.
func appOwnerJSON(st *Store, app *App) map[string]interface{} {
	st.mu.RLock()
	owner := st.Users[app.OwnerID]
	st.mu.RUnlock()
	if owner == nil {
		return nil
	}
	return userToJSON(owner)
}

func installationToJSON(inst *Installation) map[string]interface{} {
	if inst == nil {
		return nil
	}
	// The account rides as a simple-user regardless of target type —
	// real GitHub serializes Organization targets in the same shape with
	// type "Organization". Node ID and avatar were snapshotted from the
	// target account at installation time.
	accountAPI := "/api/v3/users/" + inst.TargetLogin
	account := map[string]interface{}{
		"login":               inst.TargetLogin,
		"id":                  inst.TargetID,
		"node_id":             inst.TargetNodeID,
		"avatar_url":          inst.TargetAvatarURL,
		"gravatar_id":         "",
		"url":                 accountAPI,
		"html_url":            "/" + inst.TargetLogin,
		"followers_url":       accountAPI + "/followers",
		"following_url":       accountAPI + "/following{/other_user}",
		"gists_url":           accountAPI + "/gists{/gist_id}",
		"starred_url":         accountAPI + "/starred{/owner}{/repo}",
		"subscriptions_url":   accountAPI + "/subscriptions",
		"organizations_url":   accountAPI + "/orgs",
		"repos_url":           accountAPI + "/repos",
		"events_url":          accountAPI + "/events{/privacy}",
		"received_events_url": accountAPI + "/received_events",
		"type":                inst.TargetType,
		"site_admin":          false,
		"user_view_type":      "public",
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
		"account":                   account,
		"html_url":                  "/apps/" + inst.AppSlug + "/installations/" + strconv.Itoa(inst.ID),
		"access_tokens_url":         "/api/v3/app/installations/" + strconv.Itoa(inst.ID) + "/access_tokens",
		"repositories_url":          "/api/v3/installation/repositories",
		"suspended_at":              nil,
		"suspended_by":              nil,
	}
	if inst.SuspendedAt != nil {
		out["suspended_at"] = inst.SuspendedAt.UTC().Format(time.RFC3339)
		if inst.SuspendedBy != nil {
			out["suspended_by"] = userToJSON(inst.SuspendedBy)
		}
	}
	return out
}

func installationTokenToJSON(token *InstallationToken, inst *Installation, scopedRepos []*Repo, st *Store, baseURL string) map[string]interface{} {
	// repository_selection reflects the token's effective scope: "selected"
	// when minted with a repository subset, otherwise the installation's.
	selection := ""
	if inst != nil {
		selection = inst.RepositorySelection
	}
	if len(token.RepositoryIDs) > 0 {
		selection = "selected"
	}
	out := map[string]interface{}{
		"token":                token.Token,
		"expires_at":           token.ExpiresAt.UTC().Format(time.RFC3339),
		"permissions":          token.Permissions,
		"repository_selection": selection,
	}
	if len(token.RepositoryIDs) > 0 {
		repoJSON := make([]map[string]interface{}, 0, len(scopedRepos))
		for _, repo := range scopedRepos {
			repoJSON = append(repoJSON, repoToJSON(repo, st, baseURL))
		}
		out["repositories"] = repoJSON
	}
	return out
}

// handleListUserInstallations — GET /api/v3/user/installations.
// Real GitHub: scoped to installations the authenticated user has access
// to — installations on the user's own account plus installations on
// organizations where the user is an active member.
func (s *Server) handleListUserInstallations(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	var all []*Installation
	for _, inst := range s.snapshotInstallations() {
		if inst.TargetLogin == user.Login {
			all = append(all, inst)
			continue
		}
		if inst.TargetType == "Organization" {
			if m := s.store.GetMembership(inst.TargetLogin, user.ID); m != nil && m.State == MembershipStateActive {
				all = append(all, inst)
			}
		}
	}

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
		repoJSON = append(repoJSON, repoToJSON(repo, s.store, base))
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
	writeJSON(w, http.StatusOK, appToJSON(s.store, app, false))
}

// handleSuspendInstallation — PUT /api/v3/app/installations/{id}/suspended.
// JSON Web Token-authenticated GitHub App. 204 on success, 409 if already suspended.
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
// JSON Web Token-authenticated GitHub App. 204 on success, 409 if not suspended.
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

// findInstallationByTarget returns the JSON for the first installation matching
// targetLogin + targetType, or writes 404 and returns false.
func (s *Server) findInstallationByTarget(w http.ResponseWriter, targetLogin, targetType string) bool {
	for _, inst := range s.snapshotInstallations() {
		if inst.TargetLogin == targetLogin && inst.TargetType == targetType {
			writeJSON(w, http.StatusOK, installationToJSON(inst))
			return true
		}
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
	return false
}

// handleListOrgInstallations — GET /api/v3/orgs/{org}/installations.
// Lists the app installations on an organization. Real GitHub gates this
// on organization owner (or organization_administration:read); Bleephub's analogue
// is an active org admin membership.
func (s *Server) handleListOrgInstallations(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner.")
		return
	}
	var all []*Installation
	for _, inst := range s.snapshotInstallations() {
		if inst.TargetType == "Organization" && inst.TargetLogin == org.Login {
			all = append(all, inst)
		}
	}
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

// handleGetOrgInstallation — GET /api/v3/orgs/{org}/installation.
func (s *Server) handleGetOrgInstallation(w http.ResponseWriter, r *http.Request) {
	if ghUserFromContext(r.Context()) == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	s.findInstallationByTarget(w, r.PathValue("org"), "Organization")
}

// handleGetUserInstallation — GET /api/v3/users/{username}/installation.
func (s *Server) handleGetUserInstallation(w http.ResponseWriter, r *http.Request) {
	if ghUserFromContext(r.Context()) == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	s.findInstallationByTarget(w, r.PathValue("username"), "User")
}

// handleAddUserInstallationRepo — PUT /api/v3/user/installations/{id}/repositories/{repo_id}.
// User-auth. Adds a repo to a "selected"-mode installation's allow-list. Auto-switches mode
// to "selected" if it was "all" (real GH requires the mode to already be "selected" — bleephub
// is permissive in Bleephub). 204 on success.
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
		repoJSON = append(repoJSON, repoToJSON(repo, s.store, base))
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

func (s *Server) snapshotGitHubApps() []*App {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	out := make([]*App, 0, len(s.store.Apps))
	for _, app := range s.store.Apps {
		out = append(out, app)
	}
	return out
}

// installationAccessibleRepoIDs returns the set of repo IDs the installation
// can reach: the target's owned repos, narrowed to SelectedRepoIDs when the
// installation is in "selected" mode.
func installationAccessibleRepoIDs(st *Store, inst *Installation) map[int]struct{} {
	owned := st.ListReposByOwner(inst.TargetLogin)
	out := make(map[int]struct{}, len(owned))
	if inst.RepositorySelection == "selected" {
		ownedSet := make(map[int]struct{}, len(owned))
		for _, repo := range owned {
			ownedSet[repo.ID] = struct{}{}
		}
		for _, id := range inst.SelectedRepoIDs {
			if _, ok := ownedSet[id]; ok {
				out[id] = struct{}{}
			}
		}
		return out
	}
	for _, repo := range owned {
		out[repo.ID] = struct{}{}
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
	scheme, cred := authScheme(r.Header.Get("Authorization"))
	tokenStr := ""
	if scheme == "token" || scheme == "bearer" {
		tokenStr = cred
	}
	if !strings.HasPrefix(tokenStr, tokenPrefixInstallation) {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	if !s.store.RevokeInstallationToken(tokenStr) {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListAppInstallationRequests implements GET /app/installation-requests.
// Installation requests exist on GitHub only when a non-admin asks an owner
// to install the app; bleephub installations are created directly by their
// owners, so the store never holds a pending request and the list is empty.
func (s *Server) handleListAppInstallationRequests(w http.ResponseWriter, r *http.Request) {
	app := ghAppFromContext(r.Context())
	if app == nil {
		writeGHError(w, http.StatusUnauthorized, "A JSON web token could not be decoded")
		return
	}
	writeJSON(w, http.StatusOK, []map[string]interface{}{})
}
