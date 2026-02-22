package bleephub

import (
	"encoding/json"
	"net/http"
)

func (s *Server) registerGHMemberRoutes() {
	s.mux.HandleFunc("GET /api/v3/orgs/{org}/members", s.handleListOrgMembers)
	s.mux.HandleFunc("GET /api/v3/orgs/{org}/memberships/{username}", s.handleGetOrgMembership)
	s.mux.HandleFunc("PUT /api/v3/orgs/{org}/memberships/{username}", s.handleSetOrgMembership)
	s.mux.HandleFunc("DELETE /api/v3/orgs/{org}/memberships/{username}", s.handleRemoveOrgMembership)

	s.mux.HandleFunc("GET /api/v3/orgs/{org}/teams/{team_slug}/members", s.handleListTeamMembers)
	s.mux.HandleFunc("PUT /api/v3/orgs/{org}/teams/{team_slug}/memberships/{username}", s.handleAddTeamMember)
	s.mux.HandleFunc("DELETE /api/v3/orgs/{org}/teams/{team_slug}/memberships/{username}", s.handleRemoveTeamMember)

	s.mux.HandleFunc("PUT /api/v3/orgs/{org}/teams/{team_slug}/repos/{owner}/{repo}", s.handleAddTeamRepo)
	s.mux.HandleFunc("DELETE /api/v3/orgs/{org}/teams/{team_slug}/repos/{owner}/{repo}", s.handleRemoveTeamRepo)
}

func (s *Server) handleListOrgMembers(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	org := s.store.GetOrg(orgLogin)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	members := s.store.ListOrgMembers(orgLogin)
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

	writeJSON(w, http.StatusOK, membershipToJSON(m, target, org))
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}

	s.store.SetMembership(orgLogin, target.ID, req.Role)
	m := s.store.GetMembership(orgLogin, target.ID)

	writeJSON(w, http.StatusOK, membershipToJSON(m, target, org))
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

	if !s.store.RemoveMembership(orgLogin, target.ID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

	slug := r.PathValue("team_slug")
	team := s.store.GetTeam(orgLogin, slug)
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.store.mu.RLock()
	result := make([]map[string]interface{}, 0, len(team.MemberIDs))
	for _, uid := range team.MemberIDs {
		if u, ok := s.store.Users[uid]; ok {
			result = append(result, userToJSON(u))
		}
	}
	s.store.mu.RUnlock()

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

	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner.")
		return
	}

	slug := r.PathValue("team_slug")
	if s.store.GetTeam(orgLogin, slug) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	username := r.PathValue("username")
	target := s.store.LookupUserByLogin(username)
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.store.AddTeamMember(orgLogin, slug, target.ID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"url":   s.baseURL(r) + "/api/v3/orgs/" + orgLogin + "/teams/" + slug + "/memberships/" + username,
		"role":  "member",
		"state": "active",
	})
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

	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner.")
		return
	}

	slug := r.PathValue("team_slug")
	username := r.PathValue("username")
	target := s.store.LookupUserByLogin(username)
	if target == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.store.RemoveTeamMember(orgLogin, slug, target.ID)
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

	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner.")
		return
	}

	slug := r.PathValue("team_slug")
	if s.store.GetTeam(orgLogin, slug) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	fullName := owner + "/" + repo

	if s.store.GetRepo(owner, repo) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	s.store.AddTeamRepo(orgLogin, slug, fullName)
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

	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner.")
		return
	}

	slug := r.PathValue("team_slug")
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	fullName := owner + "/" + repo

	s.store.RemoveTeamRepo(orgLogin, slug, fullName)
	w.WriteHeader(http.StatusNoContent)
}

// membershipToJSON converts a Membership to a JSON-compatible map.
func membershipToJSON(m *Membership, user *User, org *Org) map[string]interface{} {
	return map[string]interface{}{
		"url":   "/api/v3/orgs/" + org.Login + "/memberships/" + user.Login,
		"state": m.State,
		"role":  m.Role,
		"user":  userToJSON(user),
		"organization": map[string]interface{}{
			"login": org.Login,
			"id":    org.ID,
		},
	}
}
