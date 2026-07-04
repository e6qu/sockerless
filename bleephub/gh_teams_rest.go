package bleephub

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) registerGHTeamRoutes() {
	s.route("GET /api/v3/user/teams", s.handleListAuthUserTeams)
	s.route("POST /api/v3/orgs/{org}/teams", s.requirePerm(scopeMembers, permWrite, s.handleCreateTeam))
	s.route("GET /api/v3/orgs/{org}/teams", s.requirePerm(scopeMembers, permRead, s.handleListTeams))
	s.route("GET /api/v3/orgs/{org}/teams/{team_slug}", s.requirePerm(scopeMembers, permRead, s.handleGetTeam))
	s.route("PATCH /api/v3/orgs/{org}/teams/{team_slug}", s.requirePerm(scopeMembers, permWrite, s.handleUpdateTeam))
	s.route("DELETE /api/v3/orgs/{org}/teams/{team_slug}", s.requirePerm(scopeMembers, permWrite, s.handleDeleteTeam))
	s.route("GET /api/v3/orgs/{org}/teams/{team_slug}/teams", s.requirePerm(scopeMembers, permRead, s.handleListChildTeams))

	s.route("GET /api/v3/orgs/{org}/teams/{team_slug}/members", s.requirePerm(scopeMembers, permRead, s.handleListTeamMembers))
	s.route("GET /api/v3/orgs/{org}/teams/{team_slug}/memberships/{username}", s.requirePerm(scopeMembers, permRead, s.handleGetTeamMembership))
	s.route("PUT /api/v3/orgs/{org}/teams/{team_slug}/memberships/{username}", s.requirePerm(scopeMembers, permWrite, s.handleAddTeamMember))
	s.route("DELETE /api/v3/orgs/{org}/teams/{team_slug}/memberships/{username}", s.requirePerm(scopeMembers, permWrite, s.handleRemoveTeamMember))

	s.route("GET /api/v3/orgs/{org}/teams/{team_slug}/repos", s.requirePerm(scopeMembers, permRead, s.handleListTeamRepos))
	s.route("GET /api/v3/orgs/{org}/teams/{team_slug}/repos/{owner}/{repo}", s.requirePerm(scopeMembers, permRead, s.handleCheckTeamRepo))
	s.route("PUT /api/v3/orgs/{org}/teams/{team_slug}/repos/{owner}/{repo}", s.requirePerm(scopeMembers, permWrite, s.handleAddTeamRepo))
	s.route("DELETE /api/v3/orgs/{org}/teams/{team_slug}/repos/{owner}/{repo}", s.requirePerm(scopeMembers, permWrite, s.handleRemoveTeamRepo))
}

// validTeamEnums checks the privacy / permission / notification_setting
// enum values a create/update body may carry ("" = absent, allowed).
func validTeamEnums(privacy, permission, notification string) (string, bool) {
	switch TeamPrivacy(privacy) {
	case "", TeamPrivacyClosed, TeamPrivacySecret:
	default:
		return "privacy", false
	}
	switch TeamPermission(permission) {
	case "", TeamPermissionPull, TeamPermissionPush, TeamPermissionAdmin:
	default:
		return "permission", false
	}
	switch TeamNotificationSetting(notification) {
	case "", TeamNotificationsEnabled, TeamNotificationsDisabled:
	default:
		return "notification_setting", false
	}
	return "", true
}

func (s *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	orgLogin := r.PathValue("org")
	org := s.store.GetOrg(orgLogin)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner.")
		return
	}

	var req struct {
		Name                string   `json:"name"`
		Description         string   `json:"description"`
		Privacy             string   `json:"privacy"`
		Permission          string   `json:"permission"`
		NotificationSetting string   `json:"notification_setting"`
		ParentTeamID        flexInt  `json:"parent_team_id"`
		Maintainers         []string `json:"maintainers"`
		RepoNames           []string `json:"repo_names"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}
	if field, ok := validTeamEnums(req.Privacy, req.Permission, req.NotificationSetting); !ok {
		writeGHValidationError(w, "Team", field, "invalid")
		return
	}
	if req.ParentTeamID != 0 {
		parent := s.store.GetTeamByID(int(req.ParentTeamID))
		if parent == nil || parent.OrgID != org.ID {
			writeGHValidationError(w, "Team", "parent_team_id", "invalid")
			return
		}
	}

	// The create body may seed maintainers (by login) and repos (by
	// "org/repo" full name). Resolve them BEFORE creating the team so an
	// unknown entry rejects the whole request instead of leaving a
	// half-built team behind.
	maintainerIDs := make([]int, 0, len(req.Maintainers))
	for _, login := range req.Maintainers {
		maintainer := s.store.LookupUserByLogin(login)
		if maintainer == nil {
			writeGHValidationError(w, "Team", "maintainers", "invalid")
			return
		}
		maintainerIDs = append(maintainerIDs, maintainer.ID)
	}
	for _, fullName := range req.RepoNames {
		owner, name, found := strings.Cut(fullName, "/")
		if !found || s.store.GetRepo(owner, name) == nil {
			writeGHValidationError(w, "Team", "repo_names", "invalid")
			return
		}
	}

	team := s.store.CreateTeam(orgLogin, req.Name, TeamOptions{
		Description:         req.Description,
		Privacy:             TeamPrivacy(req.Privacy),
		Permission:          TeamPermission(req.Permission),
		NotificationSetting: TeamNotificationSetting(req.NotificationSetting),
		ParentID:            int(req.ParentTeamID),
	})
	if team == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}
	// Real GitHub makes the authenticated creator a team maintainer
	// automatically, even when the request omits them from maintainers.
	s.store.SetTeamMembership(orgLogin, team.Slug, user.ID, TeamRoleMaintainer)
	for _, id := range maintainerIDs {
		if id != user.ID {
			s.store.SetTeamMembership(orgLogin, team.Slug, id, TeamRoleMaintainer)
		}
	}
	for _, fullName := range req.RepoNames {
		s.store.SetTeamRepoPermission(orgLogin, team.Slug, fullName, "")
	}

	s.recordAuditEvent("team.create", user.Login, orgLogin, map[string]interface{}{"team_id": team.ID, "team_slug": team.Slug})
	writeJSON(w, http.StatusCreated, teamToJSON(team, org, s.store, s.baseURL(r)))
}

func (s *Server) handleListTeams(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	orgLogin := r.PathValue("org")
	org := s.store.GetOrg(orgLogin)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !isActiveOrgMember(s.store, user, orgLogin) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	teams := s.store.ListTeams(orgLogin)
	result := make([]map[string]interface{}, 0, len(teams))
	base := s.baseURL(r)
	for _, team := range teams {
		result = append(result, teamSimpleJSON(team, org, s.store, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleGetTeam(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	orgLogin := r.PathValue("org")
	org := s.store.GetOrg(orgLogin)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !isActiveOrgMember(s.store, user, orgLogin) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	slug := r.PathValue("team_slug")
	team := s.store.GetTeam(orgLogin, slug)
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	writeJSON(w, http.StatusOK, teamToJSON(team, org, s.store, s.baseURL(r)))
}

func (s *Server) handleUpdateTeam(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	orgLogin := r.PathValue("org")
	org := s.store.GetOrg(orgLogin)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner.")
		return
	}

	slug := r.PathValue("team_slug")
	team := s.store.GetTeam(orgLogin, slug)
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.applyTeamUpdate(w, r, org, team)
}

// applyTeamUpdate validates and applies a team PATCH body against an
// already-resolved team, writing the team-full response (or a
// validation error). Shared by the slug-addressed and the legacy
// ID-addressed update endpoints so both surfaces stay identical.
func (s *Server) applyTeamUpdate(w http.ResponseWriter, r *http.Request, org *Org, team *Team) {
	orgLogin := org.Login
	slug := team.Slug

	var req map[string]interface{}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	privacy, _ := req["privacy"].(string)
	permission, _ := req["permission"].(string)
	notification, _ := req["notification_setting"].(string)
	if field, ok := validTeamEnums(privacy, permission, notification); !ok {
		writeGHValidationError(w, "Team", field, "invalid")
		return
	}

	// parent_team_id: a number re-parents, an explicit null detaches.
	parentID := -1 // -1 = absent
	if raw, present := req["parent_team_id"]; present {
		switch v := raw.(type) {
		case nil:
			parentID = 0
		case float64:
			parentID = int(v)
		default:
			writeGHValidationError(w, "Team", "parent_team_id", "invalid")
			return
		}
	}
	if parentID > 0 {
		parent := s.store.GetTeamByID(parentID)
		if parent == nil || parent.OrgID != org.ID {
			writeGHValidationError(w, "Team", "parent_team_id", "invalid")
			return
		}
		if parentID == team.ID || s.store.TeamParentWouldCycle(team.ID, parentID) {
			writeGHValidationError(w, "Team", "parent_team_id", "invalid")
			return
		}
	}

	s.store.UpdateTeam(orgLogin, slug, func(t *Team) {
		if v, ok := req["name"].(string); ok {
			t.Name = v
			t.Slug = slugify(v)
		}
		if v, ok := req["description"].(string); ok {
			t.Description = v
		}
		if privacy != "" {
			t.Privacy = TeamPrivacy(privacy)
		}
		if permission != "" {
			t.Permission = TeamPermission(permission)
		}
		if notification != "" {
			t.NotificationSetting = TeamNotificationSetting(notification)
		}
		if parentID >= 0 {
			t.ParentID = parentID
		}
	})

	// Re-fetch by ID: a name change re-keys the slug index.
	updated := s.store.GetTeamByID(team.ID)
	if updated == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, teamToJSON(updated, org, s.store, s.baseURL(r)))
}

// handleListChildTeams — GET /api/v3/orgs/{org}/teams/{team_slug}/teams.
func (s *Server) handleListChildTeams(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	orgLogin := r.PathValue("org")
	org := s.store.GetOrg(orgLogin)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	team := s.store.GetTeam(orgLogin, r.PathValue("team_slug"))
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	children := s.store.ListChildTeams(orgLogin, team.ID)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(children))
	for _, child := range children {
		result = append(result, teamSimpleJSON(child, org, s.store, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleDeleteTeam(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	orgLogin := r.PathValue("org")
	org := s.store.GetOrg(orgLogin)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner.")
		return
	}

	slug := r.PathValue("team_slug")
	if !s.store.DeleteTeam(orgLogin, slug) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.recordAuditEvent("team.delete", user.Login, orgLogin, map[string]interface{}{"team_slug": slug})
	w.WriteHeader(http.StatusNoContent)
}

// handleListAuthUserTeams — GET /api/v3/user/teams.
// Returns every team the authenticated user belongs to, across all orgs.
// Real GitHub shape: array of team objects, each with an embedded "organization" field.
// OIDC relying parties call this endpoint to map team membership → roles at sign-in.
func (s *Server) handleListAuthUserTeams(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	teams := s.store.ListTeamsByUser(user.ID)
	s.logger.Debug().Int("user_id", user.ID).Str("user_login", user.Login).Int("team_count", len(teams)).Msg("GET /api/v3/user/teams")
	result := make([]map[string]interface{}, 0, len(teams))
	base := s.baseURL(r)
	for _, team := range teams {
		org := s.store.GetOrgByID(team.OrgID)
		if org == nil {
			continue
		}
		result = append(result, teamToJSON(team, org, s.store, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

// canManageTeam reports whether a user may mutate team membership or repo
// grants. Organization owners always can; team maintainers can too, except
// only owners may promote another user to maintainer.
func (s *Server) canManageTeam(user *User, org *Org, team *Team, addingMaintainer bool) bool {
	if canAdminOrg(s.store, user, org) {
		return true
	}
	role, isMember := team.roleOf(user.ID)
	return isMember && role == TeamRoleMaintainer && !addingMaintainer
}

func (s *Server) handleListTeamMembers(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	orgLogin := r.PathValue("org")
	if s.store.GetOrg(orgLogin) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !isActiveOrgMember(s.store, user, orgLogin) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	slug := r.PathValue("team_slug")
	team := s.store.GetTeam(orgLogin, slug)
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	members := s.store.ListTeamMembers(orgLogin, slug)
	result := make([]map[string]interface{}, 0, len(members))
	for _, u := range members {
		result = append(result, userToJSON(u))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleAddTeamMember(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	orgLogin := r.PathValue("org")
	org := s.store.GetOrg(orgLogin)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	slug := r.PathValue("team_slug")
	team := s.store.GetTeam(orgLogin, slug)
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	username := r.PathValue("username")
	target := s.store.LookupUserByLogin(username)
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	if !decodeJSONBodyOptional(w, r, &req) {
		return
	}
	role := TeamRole(req.Role)
	if role == "" {
		role = TeamRoleMember
	}
	if role != TeamRoleMember && role != TeamRoleMaintainer {
		writeGHValidationError(w, "TeamMembership", "role", "invalid")
		return
	}

	if !s.canManageTeam(user, org, team, role == TeamRoleMaintainer) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner or team maintainer.")
		return
	}

	orgMembership := s.store.GetMembership(orgLogin, target.ID)
	if orgMembership == nil {
		orgMembership = s.store.SetMembership(orgLogin, target.ID, OrgRoleMember, MembershipStatePending)
		s.emitOrgMembershipEvent(org, "member_invited", orgMembership, target, user)
	}

	s.store.SetTeamMembership(orgLogin, slug, target.ID, role)

	writeJSON(w, http.StatusOK, teamMembershipJSON(s.baseURL(r), orgLogin, slug, target, team, org, role, orgMembership.State))
}

// handleGetTeamMembership — GET /api/v3/orgs/{org}/teams/{team_slug}/memberships/{username}.
// The membership state mirrors the user's org membership: a team member
// whose org invitation is still pending reads as pending.
func (s *Server) handleGetTeamMembership(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	orgLogin := r.PathValue("org")
	org := s.store.GetOrg(orgLogin)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !isActiveOrgMember(s.store, user, orgLogin) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	slug := r.PathValue("team_slug")
	team := s.store.GetTeam(orgLogin, slug)
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	username := r.PathValue("username")
	target := s.store.LookupUserByLogin(username)
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	role, isMember := s.store.GetTeamMembership(orgLogin, slug, target.ID)
	if !isMember {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	state := MembershipStateActive
	if m := s.store.GetMembership(orgLogin, target.ID); m != nil {
		state = m.State
	}
	writeJSON(w, http.StatusOK, teamMembershipJSON(s.baseURL(r), orgLogin, slug, target, team, org, role, state))
}

func teamMembershipJSON(baseURL, orgLogin, slug string, user *User, team *Team, org *Org, role TeamRole, state MembershipState) map[string]interface{} {
	api := baseURL + "/api/v3/orgs/" + orgLogin + "/teams/" + slug
	return map[string]interface{}{
		"url":              api + "/memberships/" + user.Login,
		"role":             role,
		"state":            state,
		"user":             userToJSON(user),
		"team":             teamRefJSON(team, org, baseURL),
		"organization_url": baseURL + "/api/v3/orgs/" + orgLogin,
	}
}

func (s *Server) handleRemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	orgLogin := r.PathValue("org")
	org := s.store.GetOrg(orgLogin)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	slug := r.PathValue("team_slug")
	team := s.store.GetTeam(orgLogin, slug)
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	if !s.canManageTeam(user, org, team, false) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner or team maintainer.")
		return
	}

	username := r.PathValue("username")
	target := s.store.LookupUserByLogin(username)
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	if !s.store.RemoveTeamMembership(orgLogin, slug, target.ID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListTeamRepos — GET /api/v3/orgs/{org}/teams/{team_slug}/repos.
func (s *Server) handleListTeamRepos(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	orgLogin := r.PathValue("org")
	if s.store.GetOrg(orgLogin) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !isActiveOrgMember(s.store, user, orgLogin) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	team := s.store.GetTeam(orgLogin, r.PathValue("team_slug"))
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	repos := s.store.ListTeamRepos(orgLogin, team.Slug)
	page := paginateAndLink(w, r, repos)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(page))
	for _, repo := range page {
		perm, _ := s.store.GetTeamRepoPermission(orgLogin, team.Slug, repo.FullName)
		perms, roleName := teamRepoPermissionsJSON(perm)
		j := repoToJSON(repo, s.store, base)
		j["permissions"] = perms
		j["role_name"] = roleName
		result = append(result, j)
	}
	writeJSON(w, http.StatusOK, result)
}

// handleCheckTeamRepo — GET /api/v3/orgs/{org}/teams/{team_slug}/repos/{owner}/{repo}.
// 204 when the team manages the repo; 200 with the repository body when the
// client asks via the repository media type (go-github's IsTeamRepoBySlug
// sends Accept: application/vnd.github.v3.repository+json); 404 otherwise.
func (s *Server) handleCheckTeamRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	orgLogin := r.PathValue("org")
	if s.store.GetOrg(orgLogin) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !isActiveOrgMember(s.store, user, orgLogin) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	team := s.store.GetTeam(orgLogin, r.PathValue("team_slug"))
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.writeTeamRepoCheck(w, r, orgLogin, team)
}

// writeTeamRepoCheck answers a team-manages-repository check for an
// already-resolved team: 204 when linked, 200 with the team-repository
// body under the repository media type, 404 otherwise. Shared by the
// slug-addressed and the legacy ID-addressed check endpoints.
func (s *Server) writeTeamRepoCheck(w http.ResponseWriter, r *http.Request, orgLogin string, team *Team) {
	owner, name := r.PathValue("owner"), r.PathValue("repo")
	fullName := owner + "/" + name
	if _, linked := s.store.GetTeamRepoPermission(orgLogin, team.Slug, fullName); !linked {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "vnd.github.v3.repository") {
		j := repoToJSON(repo, s.store, s.baseURL(r))
		perm, _ := s.store.GetTeamRepoPermission(orgLogin, team.Slug, fullName)
		perms, roleName := teamRepoPermissionsJSON(perm)
		j["permissions"] = perms
		j["role_name"] = roleName
		// team-repository (unlike repository / minimal-repository) does
		// not carry has_discussions or has_pull_requests.
		delete(j, "has_discussions")
		delete(j, "has_pull_requests")
		writeJSON(w, http.StatusOK, j)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddTeamRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	orgLogin := r.PathValue("org")
	org := s.store.GetOrg(orgLogin)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	slug := r.PathValue("team_slug")
	team := s.store.GetTeam(orgLogin, slug)
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	if !s.canManageTeam(user, org, team, false) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner or team maintainer.")
		return
	}

	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	fullName := owner + "/" + repo

	if s.store.GetRepo(owner, repo) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		Permission string `json:"permission"`
	}
	decodeJSONBodyOptional(w, r, &req)
	perm := TeamPermission(req.Permission)
	switch perm {
	case "", TeamPermissionPull, TeamPermissionPush, TeamPermissionAdmin:
	default:
		writeGHValidationError(w, "TeamRepo", "permission", "invalid")
		return
	}

	s.store.SetTeamRepoPermission(orgLogin, slug, fullName, perm)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveTeamRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	orgLogin := r.PathValue("org")
	org := s.store.GetOrg(orgLogin)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	slug := r.PathValue("team_slug")
	team := s.store.GetTeam(orgLogin, slug)
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	if !s.canManageTeam(user, org, team, false) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner or team maintainer.")
		return
	}

	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	fullName := owner + "/" + repo

	if !s.store.RemoveTeamRepo(orgLogin, slug, fullName) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// teamRepoPermissionsJSON expands a team's permission level into the
// boolean permissions object + role_name GitHub serves on team repos.
func teamRepoPermissionsJSON(perm TeamPermission) (map[string]interface{}, string) {
	perms := map[string]interface{}{
		"pull":     true,
		"triage":   perm == TeamPermissionPush || perm == TeamPermissionAdmin,
		"push":     perm == TeamPermissionPush || perm == TeamPermissionAdmin,
		"maintain": perm == TeamPermissionAdmin,
		"admin":    perm == TeamPermissionAdmin,
	}
	switch perm {
	case TeamPermissionPush:
		return perms, "write"
	case TeamPermissionAdmin:
		return perms, "admin"
	default:
		return perms, "read"
	}
}

// teamRefJSON converts a Team to the GitHub `team-simple` shape — the
// flat reference object used for a team's `parent` member. All bleephub
// teams are organization-owned, so type is "organization" (the other
// enum value is "enterprise").
func teamRefJSON(team *Team, org *Org, baseURL string) map[string]interface{} {
	api := baseURL + "/api/v3/orgs/" + org.Login + "/teams/" + team.Slug
	return map[string]interface{}{
		"id":                   team.ID,
		"node_id":              team.NodeID,
		"url":                  api,
		"html_url":             baseURL + "/orgs/" + org.Login + "/teams/" + team.Slug,
		"name":                 team.Name,
		"slug":                 team.Slug,
		"description":          team.Description,
		"privacy":              team.Privacy,
		"notification_setting": team.NotificationSetting,
		"permission":           team.Permission,
		"members_url":          api + "/members{/member}",
		"repositories_url":     api + "/repos",
		"type":                 "organization",
	}
}

// teamSimpleJSON converts a Team to the GitHub `team` shape used in org
// team list responses: team-simple plus a nullable parent reference.
// Must not be called with st.mu held (parent resolution takes RLock).
func teamSimpleJSON(team *Team, org *Org, st *Store, baseURL string) map[string]interface{} {
	out := teamRefJSON(team, org, baseURL)
	out["parent"] = nil
	if team.ParentID != 0 {
		if parent := st.GetTeamByID(team.ParentID); parent != nil {
			out["parent"] = teamRefJSON(parent, org, baseURL)
		}
	}
	return out
}

// teamToJSON converts a Team to the GitHub `team-full` shape served by
// single-team operations. Member and repository counts come straight
// from the team's stored membership and repo links. Must not be called
// with st.mu held (the embedded organization-full derives counts).
func teamToJSON(team *Team, org *Org, st *Store, baseURL string) map[string]interface{} {
	out := teamSimpleJSON(team, org, st, baseURL)
	out["organization"] = orgToJSON(org, st, baseURL)
	out["members_count"] = len(team.MemberIDs)
	out["repos_count"] = len(team.RepoNames)
	out["created_at"] = team.CreatedAt.Format(time.RFC3339)
	out["updated_at"] = team.UpdatedAt.Format(time.RFC3339)
	return out
}
