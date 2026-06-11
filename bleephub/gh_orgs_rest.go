package bleephub

import (
	"net/http"
	"time"
)

func (s *Server) registerGHOrgRoutes() {
	s.route("POST /api/v3/admin/organizations", s.handleAdminCreateOrg)
	// GitHub has no REST endpoint to self-create an org (real creation is the
	// GHES admin API above or the web UI), so this provisioning convenience is
	// sim-control under /internal/, not the GitHub namespace.
	s.route("POST /internal/orgs", s.handleCreateOrg)
	s.route("GET /api/v3/user/orgs", s.handleListAuthUserOrgs)
	s.route("GET /api/v3/orgs/{org}", s.handleGetOrg)
	s.route("PATCH /api/v3/orgs/{org}", s.handleUpdateOrg)
	s.route("DELETE /api/v3/orgs/{org}", s.handleDeleteOrg)
	s.route("GET /api/v3/users/{username}/orgs", s.handleListUserOrgs)
	s.route("POST /api/v3/orgs/{org}/repos", s.handleCreateOrgRepo)

	s.registerGHTeamRoutes()
	s.registerGHMemberRoutes()
}

// handleAdminCreateOrg implements the GHES admin org-creation endpoint:
// POST /admin/organizations — the standard GitHub Enterprise Server path for
// provisioning organizations. Body: { login, admin, profile_name }.
// `admin` is the login of the user who becomes the org owner.
// Requires a site-admin token (matches real GHES behaviour).
func (s *Server) handleAdminCreateOrg(w http.ResponseWriter, r *http.Request) {
	caller := ghUserFromContext(r.Context())
	if caller == nil || !caller.SiteAdmin {
		writeGHError(w, http.StatusForbidden, "Must be a site administrator.")
		return
	}

	var req struct {
		Login       string `json:"login"`
		Admin       string `json:"admin"`
		ProfileName string `json:"profile_name"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Login == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}
	if req.Admin == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "Admin login is required")
		return
	}

	adminUser := s.store.LookupUserByLogin(req.Admin)
	if adminUser == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Admin user not found")
		return
	}

	name := req.ProfileName
	if name == "" {
		name = req.Login
	}

	org := s.store.CreateOrg(adminUser, req.Login, name, "")
	if org == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Organization creation failed.")
		return
	}

	writeJSON(w, http.StatusCreated, orgToJSON(org, s.store, s.baseURL(r)))
}

func (s *Server) handleCreateOrg(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	var req struct {
		Login       string `json:"login"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Login == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}

	org := s.store.CreateOrg(user, req.Login, req.Name, req.Description)
	if org == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Organization creation failed.")
		return
	}

	s.recordAuditEvent("org.create", user.Login, org.Login, map[string]interface{}{"org_id": org.ID})
	writeJSON(w, http.StatusCreated, orgToJSON(org, s.store, s.baseURL(r)))
}

func (s *Server) handleGetOrg(w http.ResponseWriter, r *http.Request) {
	login := r.PathValue("org")
	org := s.store.GetOrg(login)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, orgToJSON(org, s.store, s.baseURL(r)))
}

func (s *Server) handleUpdateOrg(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	login := r.PathValue("org")
	org := s.store.GetOrg(login)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner.")
		return
	}

	var req map[string]interface{}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	s.store.UpdateOrg(login, func(o *Org) {
		if v, ok := req["name"].(string); ok {
			o.Name = v
		}
		if v, ok := req["description"].(string); ok {
			o.Description = v
		}
		if v, ok := req["email"].(string); ok {
			o.Email = v
		}
	})

	updated := s.store.GetOrg(login)
	writeJSON(w, http.StatusOK, orgToJSON(updated, s.store, s.baseURL(r)))
}

func (s *Server) handleDeleteOrg(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	login := r.PathValue("org")
	org := s.store.GetOrg(login)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	if !canAdminOrg(s.store, user, org) {
		writeGHError(w, http.StatusForbidden, "Must be an organization owner.")
		return
	}

	s.store.DeleteOrg(login)
	s.recordAuditEvent("org.delete", user.Login, login, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAuthUserOrgs(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	orgs := s.store.ListOrgsByUser(user.ID)
	result := make([]map[string]interface{}, 0, len(orgs))
	base := s.baseURL(r)
	for _, org := range orgs {
		result = append(result, orgSimpleJSON(org, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleListUserOrgs(w http.ResponseWriter, r *http.Request) {
	login := r.PathValue("username")
	user := s.store.LookupUserByLogin(login)
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	orgs := s.store.ListOrgsByUser(user.ID)
	result := make([]map[string]interface{}, 0, len(orgs))
	base := s.baseURL(r)
	for _, org := range orgs {
		result = append(result, orgSimpleJSON(org, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleCreateOrgRepo(w http.ResponseWriter, r *http.Request) {
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
		writeGHError(w, http.StatusForbidden, "Must be a member of the organization.")
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Private     bool   `json:"private"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "Repository creation failed.")
		return
	}

	repo := s.store.CreateOrgRepo(org, user, req.Name, req.Description, req.Private)
	if repo == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Repository creation failed.")
		return
	}

	writeJSON(w, http.StatusCreated, fullRepoJSON(repo, s.store, s.baseURL(r)))
}

// orgSimpleJSON converts an Org to the GitHub `organization-simple`
// shape — the org object used in org list responses. The schema
// enumerates exactly these twelve members; profile fields belong to
// organization-full (orgToJSON).
func orgSimpleJSON(org *Org, baseURL string) map[string]interface{} {
	api := baseURL + "/api/v3/orgs/" + org.Login
	return map[string]interface{}{
		"login":              org.Login,
		"id":                 org.ID,
		"node_id":            org.NodeID,
		"url":                api,
		"repos_url":          api + "/repos",
		"events_url":         api + "/events",
		"hooks_url":          api + "/hooks",
		"issues_url":         api + "/issues",
		"members_url":        api + "/members{/member}",
		"public_members_url": api + "/public_members{/member}",
		"avatar_url":         org.AvatarURL,
		"description":        org.Description,
	}
}

// orgToJSON converts an Org to the GitHub `organization-full` shape
// served by single-org operations. public_repos is derived live from
// the store; bleephub has no org archiving, gists, or org-level
// follower graph, so archived_at is null and those counters are 0. The
// has_*_projects toggles are false because bleephub serves no classic
// projects surface. Must not be called with st.mu held.
func orgToJSON(org *Org, st *Store, baseURL string) map[string]interface{} {
	out := orgSimpleJSON(org, baseURL)
	out["name"] = org.Name
	out["email"] = org.Email
	out["type"] = org.Type
	out["html_url"] = baseURL + "/" + org.Login
	out["created_at"] = org.CreatedAt.Format(time.RFC3339)
	out["updated_at"] = org.UpdatedAt.Format(time.RFC3339)
	out["archived_at"] = nil
	out["public_repos"] = st.CountPublicRepos(org.Login)
	out["public_gists"] = 0
	out["followers"] = 0
	out["following"] = 0
	out["has_organization_projects"] = false
	out["has_repository_projects"] = false
	return out
}
