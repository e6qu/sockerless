package bleephub

import (
	"net/http"
)

func (s *Server) registerGHMemberRoutes() {
	s.route("GET /api/v3/orgs/{org}/members", s.handleListOrgMembers)
	s.route("GET /api/v3/orgs/{org}/members/{username}", s.handleCheckOrgMember)
	s.route("DELETE /api/v3/orgs/{org}/members/{username}", s.handleRemoveOrgMember)
	s.route("GET /api/v3/orgs/{org}/public_members", s.handleListPublicOrgMembers)
	s.route("GET /api/v3/orgs/{org}/public_members/{username}", s.handleCheckPublicOrgMember)
	s.route("PUT /api/v3/orgs/{org}/public_members/{username}", s.handlePublicizeOrgMembership)
	s.route("DELETE /api/v3/orgs/{org}/public_members/{username}", s.handleConcealOrgMembership)
	s.route("GET /api/v3/orgs/{org}/memberships/{username}", s.handleGetOrgMembership)
	s.route("PUT /api/v3/orgs/{org}/memberships/{username}", s.handleSetOrgMembership)
	s.route("DELETE /api/v3/orgs/{org}/memberships/{username}", s.handleRemoveOrgMembership)

	// The authenticated user's own memberships (the invitee side of the
	// PUT-membership invitation flow: list, inspect, accept).
	s.route("GET /api/v3/user/memberships/orgs", s.handleListAuthUserMemberships)
	s.route("GET /api/v3/user/memberships/orgs/{org}", s.handleGetAuthUserMembership)
	s.route("PATCH /api/v3/user/memberships/orgs/{org}", s.handleUpdateAuthUserMembership)
}

func (s *Server) handleListOrgMembers(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	org := s.store.GetOrg(orgLogin)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	// Real GitHub only exposes the full member list to authenticated members of
	// the org; a non-member (or anonymous caller) sees just the publicized
	// members. This mirrors the behaviour of GET /orgs/{org}/members vs
	// /orgs/{org}/public_members.
	user := ghUserFromContext(r.Context())
	isMember := false
	if user != nil {
		if m := s.store.GetMembership(orgLogin, user.ID); m != nil && m.State == MembershipStateActive {
			isMember = true
		}
	}
	var members []*User
	if isMember {
		members = s.store.ListOrgMembers(orgLogin)
	} else {
		members = s.store.ListPublicOrgMembers(orgLogin)
	}
	result := make([]map[string]interface{}, 0, len(members))
	for _, u := range members {
		result = append(result, userToJSON(u))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleGetOrgMembership(w http.ResponseWriter, r *http.Request) {
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

	username := r.PathValue("username")
	target := s.store.LookupUserByLogin(username)
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	m := s.store.GetMembership(orgLogin, target.ID)
	if m == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	writeJSON(w, http.StatusOK, membershipToJSON(m, target, org, s.baseURL(r)))
}

func (s *Server) handleSetOrgMembership(w http.ResponseWriter, r *http.Request) {
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
	role := OrgRole(req.Role)
	if role == "" {
		role = OrgRoleMember
	}
	if role != OrgRoleAdmin && role != OrgRoleMember {
		writeGHValidationError(w, "Membership", "role", "invalid")
		return
	}

	// Real GitHub semantics: adding a NEW member creates a pending
	// invitation the invitee accepts via PATCH /user/memberships/orgs/{org};
	// updating an existing membership only changes the role. Self-PUT by an
	// existing member keeps the active state.
	existing := s.store.GetMembership(orgLogin, target.ID)
	state := MembershipStatePending
	if existing != nil {
		state = existing.State
	} else if target.ID == user.ID {
		state = MembershipStateActive
	}
	m := s.store.SetMembership(orgLogin, target.ID, role, state)
	if m == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if existing == nil {
		action := "member_invited"
		if state == MembershipStateActive {
			action = "member_added"
		}
		s.emitOrgMembershipEvent(org, action, m, target, user)
	}

	writeJSON(w, http.StatusOK, membershipToJSON(m, target, org, s.baseURL(r)))
}

func (s *Server) handleRemoveOrgMembership(w http.ResponseWriter, r *http.Request) {
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

	username := r.PathValue("username")
	target := s.store.LookupUserByLogin(username)
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	m := s.store.GetMembership(orgLogin, target.ID)
	if m == nil || !s.store.RemoveMembership(orgLogin, target.ID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.emitOrgMembershipEvent(org, "member_removed", m, target, user)

	w.WriteHeader(http.StatusNoContent)
}

// handleCheckOrgMember — GET /api/v3/orgs/{org}/members/{username}.
// 204 when the user is an active member, 404 otherwise. (Real GitHub also
// has a 302 variant when the REQUESTER is not a member; bleephub's
// requesters are unscoped so the direct answer is always available.)
func (s *Server) handleCheckOrgMember(w http.ResponseWriter, r *http.Request) {
	if ghUserFromContext(r.Context()) == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	orgLogin := r.PathValue("org")
	if s.store.GetOrg(orgLogin) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	m := s.store.GetMembership(orgLogin, target.ID)
	if m == nil || m.State != MembershipStateActive {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRemoveOrgMember — DELETE /api/v3/orgs/{org}/members/{username}.
// Removes the member (and their team memberships) like the memberships
// DELETE, but 404s when the user isn't a member at all.
func (s *Server) handleRemoveOrgMember(w http.ResponseWriter, r *http.Request) {
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
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	m := s.store.GetMembership(orgLogin, target.ID)
	if m == nil || !s.store.RemoveMembership(orgLogin, target.ID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.emitOrgMembershipEvent(org, "member_removed", m, target, user)
	w.WriteHeader(http.StatusNoContent)
}

// handleListPublicOrgMembers — GET /api/v3/orgs/{org}/public_members.
// Anonymous-readable, like the real endpoint.
func (s *Server) handleListPublicOrgMembers(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	if s.store.GetOrg(orgLogin) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	members := s.store.ListPublicOrgMembers(orgLogin)
	result := make([]map[string]interface{}, 0, len(members))
	for _, u := range members {
		result = append(result, userToJSON(u))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

// handleCheckPublicOrgMember — GET /api/v3/orgs/{org}/public_members/{username}.
func (s *Server) handleCheckPublicOrgMember(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	if s.store.GetOrg(orgLogin) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	m := s.store.GetMembership(orgLogin, target.ID)
	if m == nil || m.State != MembershipStateActive || !m.Public {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePublicizeOrgMembership — PUT /api/v3/orgs/{org}/public_members/{username}.
// Real GitHub only lets users publicize THEIR OWN membership (403 otherwise).
func (s *Server) handlePublicizeOrgMembership(w http.ResponseWriter, r *http.Request) {
	s.setMembershipVisibility(w, r, true)
}

// handleConcealOrgMembership — DELETE /api/v3/orgs/{org}/public_members/{username}.
func (s *Server) handleConcealOrgMembership(w http.ResponseWriter, r *http.Request) {
	s.setMembershipVisibility(w, r, false)
}

func (s *Server) setMembershipVisibility(w http.ResponseWriter, r *http.Request, public bool) {
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
	target := s.store.LookupUserByLogin(r.PathValue("username"))
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if target.ID != user.ID {
		writeGHError(w, http.StatusForbidden, "You can only publicize or conceal your own membership.")
		return
	}
	if !s.store.SetMembershipPublic(orgLogin, target.ID, public) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListAuthUserMemberships — GET /api/v3/user/memberships/orgs.
// Supports the documented `state` filter (active | pending).
func (s *Server) handleListAuthUserMemberships(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	state := MembershipState(r.URL.Query().Get("state"))
	switch state {
	case "", MembershipStateActive, MembershipStatePending:
	default:
		writeGHValidationError(w, "Membership", "state", "invalid")
		return
	}
	memberships := s.store.ListMembershipsByUser(user.ID, state)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(memberships))
	for _, m := range memberships {
		org := s.store.GetOrgByID(m.OrgID)
		if org == nil {
			continue
		}
		result = append(result, membershipToJSON(m, user, org, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

// handleGetAuthUserMembership — GET /api/v3/user/memberships/orgs/{org}.
func (s *Server) handleGetAuthUserMembership(w http.ResponseWriter, r *http.Request) {
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
	m := s.store.GetMembership(orgLogin, user.ID)
	if m == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, membershipToJSON(m, user, org, s.baseURL(r)))
}

// handleUpdateAuthUserMembership — PATCH /api/v3/user/memberships/orgs/{org}.
// The accept half of the invitation flow: the only documented body is
// {"state":"active"}, turning a pending membership active.
func (s *Server) handleUpdateAuthUserMembership(w http.ResponseWriter, r *http.Request) {
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
	var req struct {
		State string `json:"state"`
	}
	if !decodeJSONBodyOptional(w, r, &req) {
		return
	}
	if MembershipState(req.State) != MembershipStateActive {
		writeGHValidationError(w, "Membership", "state", "invalid")
		return
	}
	m := s.store.GetMembership(orgLogin, user.ID)
	if m == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if m.State == MembershipStatePending {
		m = s.store.SetMembership(orgLogin, user.ID, m.Role, MembershipStateActive)
		s.emitOrgMembershipEvent(org, "member_added", m, user, user)
	}
	writeJSON(w, http.StatusOK, membershipToJSON(m, user, org, s.baseURL(r)))
}

// membershipToJSON converts a Membership to the GitHub
// `org-membership` shape: organization is the organization-simple
// object and user the simple-user object.
func membershipToJSON(m *Membership, user *User, org *Org, baseURL string) map[string]interface{} {
	return map[string]interface{}{
		"url":              baseURL + "/api/v3/orgs/" + org.Login + "/memberships/" + user.Login,
		"organization_url": baseURL + "/api/v3/orgs/" + org.Login,
		"state":            m.State,
		"role":             m.Role,
		"user":             userToJSON(user),
		"organization":     orgSimpleJSON(org, baseURL),
	}
}
