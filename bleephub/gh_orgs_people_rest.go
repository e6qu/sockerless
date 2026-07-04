package bleephub

import (
	"net/http"
	"sort"
	"strconv"
	"time"
)

// REST surface for organization people management: organization
// invitations (+ failed invitations and per-team invitation lists),
// outside collaborators, organization user blocks, organization
// interaction limits, organization roles (predefined catalog +
// team/user assignment), security managers, organization-member
// codespace administration lives in gh_codespaces.go, Copilot seat
// lookup, and org-wide security-product enablement.

func (s *Server) registerGHOrgsPeopleRoutes() {
	// Organization invitations.
	s.route("GET /api/v3/orgs/{org}/invitations", s.requirePerm(scopeMembers, permRead, s.handleListOrgInvitations))
	s.route("POST /api/v3/orgs/{org}/invitations", s.requirePerm(scopeMembers, permWrite, s.handleCreateOrgInvitation))
	s.route("DELETE /api/v3/orgs/{org}/invitations/{invitation_id}", s.requirePerm(scopeMembers, permWrite, s.handleCancelOrgInvitation))
	s.route("GET /api/v3/orgs/{org}/invitations/{invitation_id}/teams", s.requirePerm(scopeMembers, permRead, s.handleListOrgInvitationTeams))
	s.route("GET /api/v3/orgs/{org}/failed_invitations", s.requirePerm(scopeMembers, permRead, s.handleListFailedOrgInvitations))
	s.route("GET /api/v3/orgs/{org}/teams/{team_slug}/invitations", s.requirePerm(scopeMembers, permRead, s.handleListTeamInvitations))

	// Outside collaborators.
	s.route("GET /api/v3/orgs/{org}/outside_collaborators", s.requirePerm(scopeMembers, permRead, s.handleListOutsideCollaborators))
	s.route("PUT /api/v3/orgs/{org}/outside_collaborators/{username}", s.requirePerm(scopeMembers, permWrite, s.handleConvertMemberToOutsideCollaborator))
	s.route("DELETE /api/v3/orgs/{org}/outside_collaborators/{username}", s.requirePerm(scopeMembers, permWrite, s.handleRemoveOutsideCollaborator))

	// Organization user blocks.
	s.route("GET /api/v3/orgs/{org}/blocks", s.requirePerm(scopeOrgAdministration, permRead, s.handleListOrgBlocks))
	s.route("GET /api/v3/orgs/{org}/blocks/{username}", s.requirePerm(scopeOrgAdministration, permRead, s.handleCheckOrgBlock))
	s.route("PUT /api/v3/orgs/{org}/blocks/{username}", s.requirePerm(scopeOrgAdministration, permWrite, s.handleBlockOrgUser))
	s.route("DELETE /api/v3/orgs/{org}/blocks/{username}", s.requirePerm(scopeOrgAdministration, permWrite, s.handleUnblockOrgUser))

	// Organization interaction limits.
	s.route("GET /api/v3/orgs/{org}/interaction-limits", s.requirePerm(scopeOrgAdministration, permRead, s.handleGetOrgInteractionLimits))
	s.route("PUT /api/v3/orgs/{org}/interaction-limits", s.requirePerm(scopeOrgAdministration, permWrite, s.handleSetOrgInteractionLimits))
	s.route("DELETE /api/v3/orgs/{org}/interaction-limits", s.requirePerm(scopeOrgAdministration, permWrite, s.handleDeleteOrgInteractionLimits))

	s.registerGHOrgRolesRoutes()
}

func (s *Server) registerGHOrgRolesRoutes() {
	// Organization roles.
	s.route("GET /api/v3/orgs/{org}/organization-roles", s.requirePerm(scopeOrgAdministration, permRead, s.handleListOrganizationRoles))
	s.route("GET /api/v3/orgs/{org}/organization-roles/{role_id}", s.requirePerm(scopeOrgAdministration, permRead, s.handleGetOrganizationRole))
	s.route("GET /api/v3/orgs/{org}/organization-roles/{role_id}/teams", s.requirePerm(scopeOrgAdministration, permRead, s.handleListOrganizationRoleTeams))
	s.route("GET /api/v3/orgs/{org}/organization-roles/{role_id}/users", s.requirePerm(scopeOrgAdministration, permRead, s.handleListOrganizationRoleUsers))
	s.route("PUT /api/v3/orgs/{org}/organization-roles/teams/{team_slug}/{role_id}", s.requirePerm(scopeOrgAdministration, permWrite, s.handleAssignOrganizationRoleToTeam))
	s.route("DELETE /api/v3/orgs/{org}/organization-roles/teams/{team_slug}/{role_id}", s.requirePerm(scopeOrgAdministration, permWrite, s.handleRevokeOrganizationRoleFromTeam))
	s.route("DELETE /api/v3/orgs/{org}/organization-roles/teams/{team_slug}", s.requirePerm(scopeOrgAdministration, permWrite, s.handleRevokeAllOrganizationRolesFromTeam))
	s.route("PUT /api/v3/orgs/{org}/organization-roles/users/{username}/{role_id}", s.requirePerm(scopeOrgAdministration, permWrite, s.handleAssignOrganizationRoleToUser))
	s.route("DELETE /api/v3/orgs/{org}/organization-roles/users/{username}/{role_id}", s.requirePerm(scopeOrgAdministration, permWrite, s.handleRevokeOrganizationRoleFromUser))
	s.route("DELETE /api/v3/orgs/{org}/organization-roles/users/{username}", s.requirePerm(scopeOrgAdministration, permWrite, s.handleRevokeAllOrganizationRolesFromUser))

	// Security managers (the team-assignment alias of the
	// security_manager organization role).
	s.route("GET /api/v3/orgs/{org}/security-managers", s.requirePerm(scopeOrgAdministration, permRead, s.handleListSecurityManagerTeams))
	s.route("PUT /api/v3/orgs/{org}/security-managers/teams/{team_slug}", s.requirePerm(scopeOrgAdministration, permWrite, s.handleAddSecurityManagerTeam))
	s.route("DELETE /api/v3/orgs/{org}/security-managers/teams/{team_slug}", s.requirePerm(scopeOrgAdministration, permWrite, s.handleRemoveSecurityManagerTeam))

	// Org-wide security-product enablement.
	s.route("POST /api/v3/orgs/{org}/{security_product}/{enablement}", s.requirePerm(scopeOrgAdministration, permWrite, s.handleOrgSecurityProductEnablement))
}

// resolveOrgOwner resolves the {org} path parameter and requires the
// authenticated caller to be an active organization owner, writing the
// appropriate error otherwise.
func (s *Server) resolveOrgOwner(w http.ResponseWriter, r *http.Request) (*Org, *User) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return nil, nil
	}
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner.")
		return nil, nil
	}
	return org, user
}

// resolveOrgMember resolves the {org} path parameter and requires the
// authenticated caller to be an active organization member — the org's
// internal structure reads as 404 to everyone else, like real GitHub.
func (s *Server) resolveOrgMember(w http.ResponseWriter, r *http.Request) (*Org, *User) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return nil, nil
	}
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	if !isActiveOrgMember(s.store, user, org.Login) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil, nil
	}
	return org, user
}

// --- organization invitations ---

// orgInvitationJSON renders the GitHub `organization-invitation` shape.
func (s *Server) orgInvitationJSON(inv *OrgInvitation, org *Org, baseURL string) map[string]interface{} {
	var login, email interface{}
	if inv.Login != "" {
		login = inv.Login
	}
	if inv.Email != "" {
		email = inv.Email
	}
	inviter := map[string]interface{}(nil)
	if u := s.store.GetUserByID(inv.InviterID); u != nil {
		inviter = userToJSON(u)
	}
	var failedAt, failedReason interface{}
	if inv.FailedAt != nil {
		failedAt = inv.FailedAt.UTC().Format(time.RFC3339)
		failedReason = inv.FailedReason
	}
	return map[string]interface{}{
		"id":                   inv.ID,
		"node_id":              inv.NodeID,
		"login":                login,
		"email":                email,
		"role":                 inv.Role,
		"created_at":           inv.CreatedAt.UTC().Format(time.RFC3339),
		"failed_at":            failedAt,
		"failed_reason":        failedReason,
		"inviter":              inviter,
		"team_count":           len(inv.TeamIDs),
		"invitation_teams_url": baseURL + "/api/v3/orgs/" + org.Login + "/invitations/" + strconv.Itoa(inv.ID) + "/teams",
		"invitation_source":    inv.Source,
	}
}

func (s *Server) handleListOrgInvitations(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	roleFilter := r.URL.Query().Get("role")
	switch roleFilter {
	case "", "all", "admin", "direct_member", "billing_manager", "hiring_manager":
	default:
		writeGHValidationError(w, "OrganizationInvitation", "role", "invalid")
		return
	}
	sourceFilter := r.URL.Query().Get("invitation_source")
	switch sourceFilter {
	case "", "all", "member", "scim":
	default:
		writeGHValidationError(w, "OrganizationInvitation", "invitation_source", "invalid")
		return
	}

	invitations := s.store.ListPendingOrgInvitations(org.Login)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(invitations))
	for _, inv := range invitations {
		if roleFilter != "" && roleFilter != "all" && inv.Role != roleFilter {
			continue
		}
		if sourceFilter != "" && sourceFilter != "all" && inv.Source != sourceFilter {
			continue
		}
		result = append(result, s.orgInvitationJSON(inv, org, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleCreateOrgInvitation(w http.ResponseWriter, r *http.Request) {
	org, user := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}

	var req struct {
		InviteeID flexInt      `json:"invitee_id"`
		Email     string       `json:"email"`
		Role      string       `json:"role"`
		TeamIDs   flexIntSlice `json:"team_ids"`
	}
	if !decodeJSONBodyOptional(w, r, &req) {
		return
	}

	role := req.Role
	if role == "" {
		role = "direct_member"
	}
	switch role {
	case "direct_member", "admin", "billing_manager":
	case "reinstate":
		// bleephub keeps no record of removed members' previous roles, so
		// a reinstate invitation has no role to restore.
		writeGHError(w, http.StatusUnprocessableEntity, "Invitee was not previously a member of this organization, so there is no role to reinstate.")
		return
	default:
		writeGHValidationError(w, "OrganizationInvitation", "role", "invalid")
		return
	}

	if req.InviteeID == 0 && req.Email == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "One of invitee_id or email is required.")
		return
	}
	if req.InviteeID != 0 && req.Email != "" {
		writeGHError(w, http.StatusUnprocessableEntity, "Only one of invitee_id or email may be specified.")
		return
	}

	var invitee *User
	email := req.Email
	if req.InviteeID != 0 {
		invitee = s.store.GetUserByID(int(req.InviteeID))
		if invitee == nil {
			writeGHValidationError(w, "OrganizationInvitation", "invitee_id", "invalid")
			return
		}
	} else if u := s.store.LookupUserByEmail(email); u != nil {
		// An email invitation addressed to an existing account resolves to
		// that account, exactly as real GitHub links the invite.
		invitee = u
	}

	if invitee != nil && s.store.IsUserBlockedByOrg(org.Login, invitee.ID) {
		writeGHError(w, http.StatusUnprocessableEntity, "Invitee is blocked from this organization.")
		return
	}

	teamIDs := make([]int, 0, len(req.TeamIDs))
	for _, id := range req.TeamIDs {
		team := s.store.GetTeamByID(id)
		if team == nil || team.OrgID != org.ID {
			writeGHValidationError(w, "OrganizationInvitation", "team_ids", "invalid")
			return
		}
		teamIDs = append(teamIDs, id)
	}

	inv, reason := s.store.CreateOrgInvitation(org, user, invitee, email, role, teamIDs)
	if inv == nil {
		writeGHError(w, http.StatusUnprocessableEntity, reason)
		return
	}
	if invitee != nil {
		if m := s.store.GetMembership(org.Login, invitee.ID); m != nil {
			s.emitOrgMembershipEvent(org, "member_invited", m, invitee, user)
		}
	}
	s.recordAuditEvent("org.invite_member", user.Login, org.Login, map[string]interface{}{"invitation_id": inv.ID, "role": role})
	writeJSON(w, http.StatusCreated, s.orgInvitationJSON(inv, org, s.baseURL(r)))
}

func (s *Server) handleCancelOrgInvitation(w http.ResponseWriter, r *http.Request) {
	org, user := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("invitation_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.CancelOrgInvitation(org.Login, id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.recordAuditEvent("org.cancel_invitation", user.Login, org.Login, map[string]interface{}{"invitation_id": id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListOrgInvitationTeams(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("invitation_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	inv := s.store.GetOrgInvitation(org.Login, id)
	if inv == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(inv.TeamIDs))
	for _, teamID := range inv.TeamIDs {
		if team := s.store.GetTeamByID(teamID); team != nil {
			result = append(result, teamSimpleJSON(team, org, s.store, base))
		}
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleListFailedOrgInvitations(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	invitations := s.store.ListFailedOrgInvitations(org.Login)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(invitations))
	for _, inv := range invitations {
		result = append(result, s.orgInvitationJSON(inv, org, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

// handleListTeamInvitations — GET /api/v3/orgs/{org}/teams/{team_slug}/invitations:
// the org's pending invitations that carry this team.
func (s *Server) handleListTeamInvitations(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgMember(w, r)
	if org == nil {
		return
	}
	team := s.store.GetTeam(org.Login, r.PathValue("team_slug"))
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
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

// --- outside collaborators ---

func (s *Server) handleListOutsideCollaborators(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgMember(w, r)
	if org == nil {
		return
	}
	filter := r.URL.Query().Get("filter")
	switch filter {
	case "", "all", "2fa_disabled", "2fa_insecure":
	default:
		writeGHValidationError(w, "OutsideCollaborator", "filter", "invalid")
		return
	}
	collaborators := s.store.ListOutsideCollaborators(org.Login)
	// bleephub has no two-factor-authentication model, so every account
	// genuinely lacks 2FA: 2fa_disabled matches everyone and 2fa_insecure
	// (insecure 2FA methods) matches no one.
	if filter == "2fa_insecure" {
		collaborators = nil
	}
	result := make([]map[string]interface{}, 0, len(collaborators))
	for _, u := range collaborators {
		result = append(result, userToJSON(u))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleConvertMemberToOutsideCollaborator(w http.ResponseWriter, r *http.Request) {
	org, user := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	m := s.store.GetMembership(org.Login, target.ID)
	if m == nil || m.State != MembershipStateActive {
		writeGHError(w, http.StatusNotFound, r.PathValue("username")+" is not a member of the "+org.Login+" organization.")
		return
	}
	if m.Role == OrgRoleAdmin {
		writeGHError(w, http.StatusForbidden, "Cannot convert an organization owner to an outside collaborator.")
		return
	}
	// The converted member keeps the repository access their team
	// memberships confer, materialized as direct collaborator grants,
	// then loses the membership itself.
	s.store.GrantTeamRepoAccessAsCollaborator(org.Login, target)
	s.store.RemoveMembership(org.Login, target.ID)
	s.emitOrgMembershipEvent(org, "member_removed", m, target, user)
	s.recordAuditEvent("org.convert_member_to_outside_collaborator", user.Login, org.Login, map[string]interface{}{"user": target.Login})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveOutsideCollaborator(w http.ResponseWriter, r *http.Request) {
	org, user := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if m := s.store.GetMembership(org.Login, target.ID); m != nil && m.State == MembershipStateActive {
		writeGHError(w, http.StatusUnprocessableEntity, "You cannot specify an organization member to remove as an outside collaborator.")
		return
	}
	s.store.RemoveOutsideCollaborator(org.Login, target.Login)
	s.recordAuditEvent("org.remove_outside_collaborator", user.Login, org.Login, map[string]interface{}{"user": target.Login})
	w.WriteHeader(http.StatusNoContent)
}

// --- organization user blocks ---

func (s *Server) handleListOrgBlocks(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	blocked := s.store.ListOrgBlockedUsers(org.Login)
	result := make([]map[string]interface{}, 0, len(blocked))
	for _, u := range blocked {
		result = append(result, userToJSON(u))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleCheckOrgBlock(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil || !s.store.IsUserBlockedByOrg(org.Login, target.ID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleBlockOrgUser(w http.ResponseWriter, r *http.Request) {
	org, user := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if target.ID == user.ID {
		writeGHError(w, http.StatusUnprocessableEntity, "You cannot block yourself.")
		return
	}
	if m := s.store.GetMembership(org.Login, target.ID); m != nil && m.State == MembershipStateActive {
		writeGHError(w, http.StatusUnprocessableEntity, "You cannot block a member of this organization.")
		return
	}
	s.store.BlockUserForOrg(org.Login, target.ID)
	s.recordAuditEvent("org.block_user", user.Login, org.Login, map[string]interface{}{"blocked_user": target.Login})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnblockOrgUser(w http.ResponseWriter, r *http.Request) {
	org, user := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.UnblockUserForOrg(org.Login, target.ID)
	s.recordAuditEvent("org.unblock_user", user.Login, org.Login, map[string]interface{}{"blocked_user": target.Login})
	w.WriteHeader(http.StatusNoContent)
}

// --- organization interaction limits ---

// orgInteractionExpiryDurations maps GitHub's interaction-expiry enum to
// the concrete durations the restrictions run for.
var orgInteractionExpiryDurations = map[string]time.Duration{
	"one_day":    24 * time.Hour,
	"three_days": 3 * 24 * time.Hour,
	"one_week":   7 * 24 * time.Hour,
	"one_month":  30 * 24 * time.Hour,
	"six_months": 180 * 24 * time.Hour,
}

func orgInteractionLimitJSON(lim *OrgInteractionLimit) map[string]interface{} {
	return map[string]interface{}{
		"limit":      lim.Limit,
		"origin":     "organization",
		"expires_at": lim.ExpiresAt.UTC().Format(time.RFC3339),
	}
}

func (s *Server) handleGetOrgInteractionLimits(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	lim := s.store.GetOrgInteractionLimit(org.Login)
	if lim == nil {
		// No active restriction reads as an empty object, per the
		// documented anyOf response.
		writeJSON(w, http.StatusOK, map[string]interface{}{})
		return
	}
	writeJSON(w, http.StatusOK, orgInteractionLimitJSON(lim))
}

func (s *Server) handleSetOrgInteractionLimits(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	var req struct {
		Limit  string `json:"limit"`
		Expiry string `json:"expiry"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	switch req.Limit {
	case "existing_users", "contributors_only", "collaborators_only":
	default:
		writeGHValidationError(w, "InteractionLimit", "limit", "invalid")
		return
	}
	expiry := req.Expiry
	if expiry == "" {
		expiry = "one_day"
	}
	duration, ok := orgInteractionExpiryDurations[expiry]
	if !ok {
		writeGHValidationError(w, "InteractionLimit", "expiry", "invalid")
		return
	}
	lim := s.store.SetOrgInteractionLimit(org.Login, req.Limit, time.Now().UTC().Add(duration))
	writeJSON(w, http.StatusOK, orgInteractionLimitJSON(lim))
}

func (s *Server) handleDeleteOrgInteractionLimits(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	s.store.DeleteOrgInteractionLimit(org.Login)
	w.WriteHeader(http.StatusNoContent)
}

// --- organization roles ---

// predefinedOrgRole is one entry of GitHub's predefined organization
// role catalog served by GET /orgs/{org}/organization-roles.
type predefinedOrgRole struct {
	ID          int
	Name        string
	Description string
	BaseRole    string
	Permissions []string
}

// securityManagerOrgRoleID is the predefined security_manager role —
// the role the deprecated security-managers endpoints alias.
const securityManagerOrgRoleID = 143

// predefinedOrgRoles is GitHub's predefined organization role catalog:
// the five all-repository access roles (which grant only their base
// repository role, hence empty permission lists) plus security_manager.
// The IDs are bleephub's stable predefined-role identifiers, mirroring
// github.com's numbering for the all-repository family.
var predefinedOrgRoles = []predefinedOrgRole{
	{ID: 138, Name: "all_repo_read", Description: "Grants read access to all repositories in the organization.", BaseRole: "read"},
	{ID: 139, Name: "all_repo_triage", Description: "Grants triage access to all repositories in the organization.", BaseRole: "triage"},
	{ID: 140, Name: "all_repo_write", Description: "Grants write access to all repositories in the organization.", BaseRole: "write"},
	{ID: 141, Name: "all_repo_maintain", Description: "Grants maintenance access to all repositories in the organization.", BaseRole: "maintain"},
	{ID: 142, Name: "all_repo_admin", Description: "Grants admin access to all repositories in the organization.", BaseRole: "admin"},
	{ID: securityManagerOrgRoleID, Name: "security_manager", Description: "Grants the ability to manage security policies, security alerts, and security configurations for an organization and all its repositories.", BaseRole: "read", Permissions: []string{"manage_security_products"}},
}

func predefinedOrgRoleByID(id int) *predefinedOrgRole {
	for i := range predefinedOrgRoles {
		if predefinedOrgRoles[i].ID == id {
			return &predefinedOrgRoles[i]
		}
	}
	return nil
}

// orgRoleJSON renders the GitHub `organization-role` shape. Predefined
// roles carry a null organization and exist from the organization's
// creation.
func orgRoleJSON(role *predefinedOrgRole, org *Org) map[string]interface{} {
	permissions := role.Permissions
	if permissions == nil {
		permissions = []string{}
	}
	return map[string]interface{}{
		"id":           role.ID,
		"name":         role.Name,
		"description":  role.Description,
		"base_role":    role.BaseRole,
		"source":       "Predefined",
		"permissions":  permissions,
		"organization": nil,
		"created_at":   org.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":   org.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// resolveOrgRoleID parses the {role_id} path parameter into a
// predefined role, writing a 404 when it doesn't resolve.
func (s *Server) resolveOrgRoleID(w http.ResponseWriter, r *http.Request) *predefinedOrgRole {
	id, err := strconv.Atoi(r.PathValue("role_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	role := predefinedOrgRoleByID(id)
	if role == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return role
}

func (s *Server) handleListOrganizationRoles(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgMember(w, r)
	if org == nil {
		return
	}
	roles := make([]map[string]interface{}, 0, len(predefinedOrgRoles))
	for i := range predefinedOrgRoles {
		roles = append(roles, orgRoleJSON(&predefinedOrgRoles[i], org))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": len(roles),
		"roles":       roles,
	})
}

func (s *Server) handleGetOrganizationRole(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgMember(w, r)
	if org == nil {
		return
	}
	role := s.resolveOrgRoleID(w, r)
	if role == nil {
		return
	}
	writeJSON(w, http.StatusOK, orgRoleJSON(role, org))
}

func (s *Server) handleListOrganizationRoleTeams(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgMember(w, r)
	if org == nil {
		return
	}
	role := s.resolveOrgRoleID(w, r)
	if role == nil {
		return
	}
	teams := s.store.ListTeamsWithOrgRole(org.Login, role.ID)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(teams))
	for _, team := range teams {
		j := teamSimpleJSON(team, org, s.store, base)
		j["assignment"] = "direct"
		result = append(result, j)
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleListOrganizationRoleUsers(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgMember(w, r)
	if org == nil {
		return
	}
	role := s.resolveOrgRoleID(w, r)
	if role == nil {
		return
	}
	assignments := s.store.ListUsersWithOrgRole(org.Login, role.ID)
	userIDs := make([]int, 0, len(assignments))
	for id := range assignments {
		userIDs = append(userIDs, id)
	}
	sort.Ints(userIDs)
	result := make([]map[string]interface{}, 0, len(userIDs))
	for _, id := range userIDs {
		u := s.store.GetUserByID(id)
		if u == nil {
			continue
		}
		j := userToJSON(u)
		j["assignment"] = assignments[id]
		result = append(result, j)
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleAssignOrganizationRoleToTeam(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	team := s.store.GetTeam(org.Login, r.PathValue("team_slug"))
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	role := s.resolveOrgRoleID(w, r)
	if role == nil {
		return
	}
	s.store.AssignOrgRoleToTeam(org.Login, role.ID, team.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRevokeOrganizationRoleFromTeam(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	team := s.store.GetTeam(org.Login, r.PathValue("team_slug"))
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	role := s.resolveOrgRoleID(w, r)
	if role == nil {
		return
	}
	s.store.UnassignOrgRoleFromTeam(org.Login, role.ID, team.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRevokeAllOrganizationRolesFromTeam(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	team := s.store.GetTeam(org.Login, r.PathValue("team_slug"))
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.UnassignAllOrgRolesFromTeam(org.Login, team.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAssignOrganizationRoleToUser(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	role := s.resolveOrgRoleID(w, r)
	if role == nil {
		return
	}
	if m := s.store.GetMembership(org.Login, target.ID); m == nil || m.State != MembershipStateActive {
		writeGHError(w, http.StatusUnprocessableEntity, "User must be an active member of the organization to be assigned an organization role.")
		return
	}
	s.store.AssignOrgRoleToUser(org.Login, role.ID, target.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRevokeOrganizationRoleFromUser(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	role := s.resolveOrgRoleID(w, r)
	if role == nil {
		return
	}
	s.store.UnassignOrgRoleFromUser(org.Login, role.ID, target.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRevokeAllOrganizationRolesFromUser(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.UnassignAllOrgRolesFromUser(org.Login, target.ID)
	w.WriteHeader(http.StatusNoContent)
}

// --- security managers ---

func (s *Server) handleListSecurityManagerTeams(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgMember(w, r)
	if org == nil {
		return
	}
	teams := s.store.ListTeamsWithOrgRole(org.Login, securityManagerOrgRoleID)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(teams))
	for _, team := range teams {
		result = append(result, teamRefJSON(team, org, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleAddSecurityManagerTeam(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	team := s.store.GetTeam(org.Login, r.PathValue("team_slug"))
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.AssignOrgRoleToTeam(org.Login, securityManagerOrgRoleID, team.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveSecurityManagerTeam(w http.ResponseWriter, r *http.Request) {
	org, _ := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	team := s.store.GetTeam(org.Login, r.PathValue("team_slug"))
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.UnassignOrgRoleFromTeam(org.Login, securityManagerOrgRoleID, team.ID)
	w.WriteHeader(http.StatusNoContent)
}

// --- org-wide security-product enablement ---

// orgSecurityProductRepoFlags maps the security products bleephub
// models per-repository onto their Repo flag fields: Dependabot alerts
// are the vulnerability-alerts setting and Dependabot security updates
// are the automated-security-fixes setting — the same state the
// /repos/{owner}/{repo}/vulnerability-alerts and
// /repos/{owner}/{repo}/automated-security-fixes endpoints flip.
var orgSecurityProductRepoFlags = map[string]string{
	"dependabot_alerts":           "vulnerability_alerts_enabled",
	"dependabot_security_updates": "automated_security_fixes_enabled",
}

// orgSecurityProductsUnavailable lists the documented security products
// bleephub has no per-repository setting for; enabling them fails the
// same way a GitHub Enterprise Server instance without the feature's
// licensing does.
var orgSecurityProductsUnavailable = map[string]string{
	"dependency_graph":                "Dependency graph is not available for this organization.",
	"advanced_security":               "GitHub Advanced Security is not available for this organization.",
	"code_scanning_default_setup":     "Code scanning default setup is not available for this organization.",
	"secret_scanning":                 "Secret scanning is not available for this organization.",
	"secret_scanning_push_protection": "Secret scanning push protection is not available for this organization.",
}

// handleOrgSecurityProductEnablement — POST /api/v3/orgs/{org}/{security_product}/{enablement}.
func (s *Server) handleOrgSecurityProductEnablement(w http.ResponseWriter, r *http.Request) {
	product := r.PathValue("security_product")
	enablement := r.PathValue("enablement")
	_, flagged := orgSecurityProductRepoFlags[product]
	_, unavailable := orgSecurityProductsUnavailable[product]
	if !flagged && !unavailable {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if enablement != "enable_all" && enablement != "disable_all" {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	org, user := s.resolveOrgOwner(w, r)
	if org == nil {
		return
	}
	if unavailable {
		writeGHError(w, http.StatusUnprocessableEntity, orgSecurityProductsUnavailable[product])
		return
	}
	enable := enablement == "enable_all"
	field := orgSecurityProductRepoFlags[product]
	for _, repo := range s.store.ListReposForOrg(org.Login, RepoListOptions{NoPaginate: true}) {
		s.store.SetRepoFlag(repo.ID, field, enable)
	}
	s.recordAuditEvent("org.security_product_enablement", user.Login, org.Login, map[string]interface{}{"security_product": product, "enablement": enablement})
	w.WriteHeader(http.StatusNoContent)
}
