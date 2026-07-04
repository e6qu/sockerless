package bleephub

import (
	"net/http"
	"strconv"
)

// Legacy ID-addressed team endpoints (/teams/{team_id}/…). These are
// aliases of the slug-addressed /orgs/{org}/teams/{team_slug}/…
// surface: the numeric ID resolves to the same store entities and the
// handlers run the same logic, so both surfaces always agree.

func (s *Server) registerGHLegacyTeamRoutes() {
	s.route("GET /api/v3/teams/{team_id}", s.requirePerm(scopeMembers, permRead, s.handleLegacyGetTeam))
	s.route("PATCH /api/v3/teams/{team_id}", s.requirePerm(scopeMembers, permWrite, s.handleLegacyUpdateTeam))
	s.route("DELETE /api/v3/teams/{team_id}", s.requirePerm(scopeMembers, permWrite, s.handleLegacyDeleteTeam))
	s.route("GET /api/v3/teams/{team_id}/teams", s.requirePerm(scopeMembers, permRead, s.handleLegacyListChildTeams))
	s.route("GET /api/v3/teams/{team_id}/invitations", s.requirePerm(scopeMembers, permRead, s.handleLegacyListTeamInvitations))

	s.route("GET /api/v3/teams/{team_id}/members", s.requirePerm(scopeMembers, permRead, s.handleLegacyListTeamMembers))
	s.route("GET /api/v3/teams/{team_id}/members/{username}", s.requirePerm(scopeMembers, permRead, s.handleLegacyCheckTeamMember))
	s.route("PUT /api/v3/teams/{team_id}/members/{username}", s.requirePerm(scopeMembers, permWrite, s.handleLegacyAddTeamMember))
	s.route("DELETE /api/v3/teams/{team_id}/members/{username}", s.requirePerm(scopeMembers, permWrite, s.handleLegacyRemoveTeamMember))

	s.route("GET /api/v3/teams/{team_id}/memberships/{username}", s.requirePerm(scopeMembers, permRead, s.handleLegacyGetTeamMembership))
	s.route("PUT /api/v3/teams/{team_id}/memberships/{username}", s.requirePerm(scopeMembers, permWrite, s.handleLegacyPutTeamMembership))
	s.route("DELETE /api/v3/teams/{team_id}/memberships/{username}", s.requirePerm(scopeMembers, permWrite, s.handleLegacyDeleteTeamMembership))

	s.route("GET /api/v3/teams/{team_id}/repos", s.requirePerm(scopeMembers, permRead, s.handleLegacyListTeamRepos))
	s.route("GET /api/v3/teams/{team_id}/repos/{owner}/{repo}", s.requirePerm(scopeMembers, permRead, s.handleLegacyCheckTeamRepo))
	s.route("PUT /api/v3/teams/{team_id}/repos/{owner}/{repo}", s.requirePerm(scopeMembers, permWrite, s.handleLegacyAddTeamRepo))
	s.route("DELETE /api/v3/teams/{team_id}/repos/{owner}/{repo}", s.requirePerm(scopeMembers, permWrite, s.handleLegacyRemoveTeamRepo))
}

// resolveLegacyTeam resolves the numeric {team_id} path parameter to the
// team and its organization, writing a 404 when either doesn't resolve.
func (s *Server) resolveLegacyTeam(w http.ResponseWriter, r *http.Request) (*Team, *Org) {
	id, err := strconv.Atoi(r.PathValue("team_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	team := s.store.GetTeamByID(id)
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	org := s.store.GetOrgByID(team.OrgID)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	return team, org
}

// resolveLegacyTeamForMember resolves {team_id} and additionally
// requires the caller to be an active member of the owning org — team
// structure is invisible to non-members, matching the slug surface.
func (s *Server) resolveLegacyTeamForMember(w http.ResponseWriter, r *http.Request) (*Team, *Org, *User) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return nil, nil, nil
	}
	team, org := s.resolveLegacyTeam(w, r)
	if team == nil {
		return nil, nil, nil
	}
	if !isActiveOrgMember(s.store, user, org.Login) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil, nil
	}
	return team, org, user
}

// legacyTeamMembershipJSON renders the documented `team-membership`
// members (url, role, state) with the legacy ID-addressed URL.
func legacyTeamMembershipJSON(baseURL string, teamID int, login string, role TeamRole, state MembershipState) map[string]interface{} {
	return map[string]interface{}{
		"url":   baseURL + "/api/v3/teams/" + strconv.Itoa(teamID) + "/memberships/" + login,
		"role":  role,
		"state": state,
	}
}

func (s *Server) handleLegacyGetTeam(w http.ResponseWriter, r *http.Request) {
	team, org, _ := s.resolveLegacyTeamForMember(w, r)
	if team == nil {
		return
	}
	writeJSON(w, http.StatusOK, teamToJSON(team, org, s.store, s.baseURL(r)))
}

func (s *Server) handleLegacyUpdateTeam(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	team, org := s.resolveLegacyTeam(w, r)
	if team == nil {
		return
	}
	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner.")
		return
	}
	s.applyTeamUpdate(w, r, org, team)
}

func (s *Server) handleLegacyDeleteTeam(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	team, org := s.resolveLegacyTeam(w, r)
	if team == nil {
		return
	}
	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner.")
		return
	}
	if !s.store.DeleteTeam(org.Login, team.Slug) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.recordAuditEvent("team.delete", user.Login, org.Login, map[string]interface{}{"team_slug": team.Slug})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLegacyListChildTeams(w http.ResponseWriter, r *http.Request) {
	team, org, _ := s.resolveLegacyTeamForMember(w, r)
	if team == nil {
		return
	}
	children := s.store.ListChildTeams(org.Login, team.ID)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(children))
	for _, child := range children {
		result = append(result, teamSimpleJSON(child, org, s.store, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleLegacyListTeamInvitations(w http.ResponseWriter, r *http.Request) {
	team, org, _ := s.resolveLegacyTeamForMember(w, r)
	if team == nil {
		return
	}
	invitations := s.store.ListPendingOrgInvitationsForTeam(org.Login, team.ID)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(invitations))
	for _, inv := range invitations {
		result = append(result, s.orgInvitationJSON(inv, org, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleLegacyListTeamMembers(w http.ResponseWriter, r *http.Request) {
	team, org, _ := s.resolveLegacyTeamForMember(w, r)
	if team == nil {
		return
	}
	members := s.store.ListTeamMembers(org.Login, team.Slug)
	result := make([]map[string]interface{}, 0, len(members))
	for _, u := range members {
		result = append(result, userToJSON(u))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleLegacyCheckTeamMember(w http.ResponseWriter, r *http.Request) {
	team, org, _ := s.resolveLegacyTeamForMember(w, r)
	if team == nil {
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if _, isMember := s.store.GetTeamMembership(org.Login, team.Slug, target.ID); !isMember {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleLegacyAddTeamMember — PUT /api/v3/teams/{team_id}/members/{username}.
// Unlike the memberships endpoint, the legacy add-member call never
// invites: the target must already be an active member of the owning
// organization (422 otherwise).
func (s *Server) handleLegacyAddTeamMember(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	team, org := s.resolveLegacyTeam(w, r)
	if team == nil {
		return
	}
	if !s.canManageTeam(user, org, team, false) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner or team maintainer.")
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if m := s.store.GetMembership(org.Login, target.ID); m == nil || m.State != MembershipStateActive {
		writeGHError(w, http.StatusUnprocessableEntity, "User is not an active member of this organization.")
		return
	}
	s.store.SetTeamMembership(org.Login, team.Slug, target.ID, TeamRoleMember)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLegacyRemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	team, org := s.resolveLegacyTeam(w, r)
	if team == nil {
		return
	}
	if !s.canManageTeam(user, org, team, false) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner or team maintainer.")
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.RemoveTeamMembership(org.Login, team.Slug, target.ID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLegacyGetTeamMembership(w http.ResponseWriter, r *http.Request) {
	team, org, _ := s.resolveLegacyTeamForMember(w, r)
	if team == nil {
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	role, isMember := s.store.GetTeamMembership(org.Login, team.Slug, target.ID)
	if !isMember {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	state := MembershipStateActive
	if m := s.store.GetMembership(org.Login, target.ID); m != nil {
		state = m.State
	}
	writeJSON(w, http.StatusOK, legacyTeamMembershipJSON(s.baseURL(r), team.ID, target.Login, role, state))
}

// handleLegacyPutTeamMembership — PUT /api/v3/teams/{team_id}/memberships/{username}.
// Same semantics as the slug-addressed membership PUT: a non-member of
// the org gets a pending org invitation alongside the team membership.
func (s *Server) handleLegacyPutTeamMembership(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	team, org := s.resolveLegacyTeam(w, r)
	if team == nil {
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
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

	orgMembership := s.store.GetMembership(org.Login, target.ID)
	if orgMembership == nil {
		orgMembership = s.store.SetMembership(org.Login, target.ID, OrgRoleMember, MembershipStatePending)
		s.emitOrgMembershipEvent(org, "member_invited", orgMembership, target, user)
	}

	s.store.SetTeamMembership(org.Login, team.Slug, target.ID, role)

	writeJSON(w, http.StatusOK, legacyTeamMembershipJSON(s.baseURL(r), team.ID, target.Login, role, orgMembership.State))
}

func (s *Server) handleLegacyDeleteTeamMembership(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	team, org := s.resolveLegacyTeam(w, r)
	if team == nil {
		return
	}
	if !s.canManageTeam(user, org, team, false) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner or team maintainer.")
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.RemoveTeamMembership(org.Login, team.Slug, target.ID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleLegacyListTeamRepos — GET /api/v3/teams/{team_id}/repos. The
// legacy list serves the minimal-repository shape.
func (s *Server) handleLegacyListTeamRepos(w http.ResponseWriter, r *http.Request) {
	team, org, _ := s.resolveLegacyTeamForMember(w, r)
	if team == nil {
		return
	}
	repos := s.store.ListTeamRepos(org.Login, team.Slug)
	page := paginateAndLink(w, r, repos)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(page))
	for _, repo := range page {
		result = append(result, minimalRepoJSON(repo, s.store, base))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleLegacyCheckTeamRepo(w http.ResponseWriter, r *http.Request) {
	team, org, _ := s.resolveLegacyTeamForMember(w, r)
	if team == nil {
		return
	}
	s.writeTeamRepoCheck(w, r, org.Login, team)
}

func (s *Server) handleLegacyAddTeamRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	team, org := s.resolveLegacyTeam(w, r)
	if team == nil {
		return
	}
	if !s.canManageTeam(user, org, team, false) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner or team maintainer.")
		return
	}
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
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
	s.store.SetTeamRepoPermission(org.Login, team.Slug, owner+"/"+repo, perm)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLegacyRemoveTeamRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	team, org := s.resolveLegacyTeam(w, r)
	if team == nil {
		return
	}
	if !s.canManageTeam(user, org, team, false) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner or team maintainer.")
		return
	}
	if !s.store.RemoveTeamRepo(org.Login, team.Slug, r.PathValue("owner")+"/"+r.PathValue("repo")) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
