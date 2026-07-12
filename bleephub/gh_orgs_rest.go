package bleephub

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerGHOrgRoutes() {
	s.route("POST /api/v3/admin/organizations", s.handleAdminCreateOrg)
	// GitHub has no REST endpoint to self-create an org (real creation is the
	// GHES admin API above or the web UI), so this provisioning convenience is
	// sim-control under /internal/, not the GitHub namespace.
	s.route("POST /internal/orgs", s.handleCreateOrg)
	s.route("GET /api/v3/user/orgs", s.handleListAuthUserOrgs)
	s.route("GET /api/v3/organizations", s.handleListAllOrgs)
	s.route("GET /api/v3/orgs/{org}", s.handleGetOrg)
	s.route("PATCH /api/v3/orgs/{org}", s.handleUpdateOrg)
	s.route("DELETE /api/v3/orgs/{org}", s.handleDeleteOrg)
	s.route("GET /api/v3/users/{username}/orgs", s.handleListUserOrgs)
	s.route("POST /api/v3/orgs/{org}/repos", s.handleCreateOrgRepo)

	s.registerGHTeamRoutes()
	s.registerGHMemberRoutes()
	s.registerGHOrgHookRoutes()
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

	if v, ok := req["default_repository_permission"].(string); ok {
		switch v {
		case "read", "write", "admin", "none":
		default:
			writeGHValidationError(w, "Organization", "default_repository_permission", "invalid")
			return
		}
	}

	s.store.UpdateOrg(login, func(o *Org) {
		setStr := func(key string, dst *string) {
			if v, ok := req[key].(string); ok {
				*dst = v
			}
		}
		setStr("name", &o.Name)
		setStr("description", &o.Description)
		setStr("email", &o.Email)
		setStr("company", &o.Company)
		setStr("blog", &o.Blog)
		setStr("location", &o.Location)
		setStr("twitter_username", &o.TwitterUsername)
		setStr("billing_email", &o.BillingEmail)
		setStr("default_repository_permission", &o.DefaultRepositoryPermission)
		if v, ok := req["members_can_create_repositories"].(bool); ok {
			o.MembersCanCreateRepositories = &v
		}
		if v, ok := req["web_commit_signoff_required"].(bool); ok {
			o.WebCommitSignoffRequired = v
		}
	})

	updated := s.store.GetOrg(login)
	writeJSON(w, http.StatusOK, orgToJSON(updated, s.store, s.baseURL(r)))
}

// handleListAllOrgs — GET /api/v3/organizations: the global org list,
// ordered by id, starting after the `since` cursor.
func (s *Server) handleListAllOrgs(w http.ResponseWriter, r *http.Request) {
	if ghUserFromContext(r.Context()) == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	since := 0
	if v := r.URL.Query().Get("since"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			since = n
		}
	}
	orgs := s.store.ListOrgsAll(since)
	base := s.baseURL(r)
	result := make([]map[string]interface{}, 0, len(orgs))
	for _, org := range orgs {
		result = append(result, orgSimpleJSON(org, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
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

	if instTok := ghInstallationTokenFromContext(r.Context()); instTok != nil {
		inst := ghInstallationFromContext(r.Context())
		if inst == nil ||
			inst.TargetType != "Organization" ||
			inst.TargetID != org.ID ||
			!strings.EqualFold(inst.TargetLogin, org.Login) ||
			!hasPerm(instTok.Permissions, scopeAdministration, permWrite) {
			writeGHError(w, http.StatusForbidden, "Resource not accessible by integration")
			return
		}
	} else {
		m := s.store.GetMembership(orgLogin, user.ID)
		if m == nil {
			writeGHError(w, http.StatusForbidden, "Must be a member of the organization.")
			return
		}
	}

	var req struct {
		Name                      string   `json:"name"`
		Description               string   `json:"description"`
		Homepage                  string   `json:"homepage"`
		Private                   flexBool `json:"private"`
		Visibility                string   `json:"visibility"`
		DefaultBranch             string   `json:"default_branch"`
		AutoInit                  flexBool `json:"auto_init"`
		GitignoreTemplate         string   `json:"gitignore_template"`
		LicenseTemplate           string   `json:"license_template"`
		HasIssues                 *bool    `json:"has_issues"`
		HasProjects               *bool    `json:"has_projects"`
		HasWiki                   *bool    `json:"has_wiki"`
		HasDiscussions            *bool    `json:"has_discussions"`
		HasPullRequests           *bool    `json:"has_pull_requests"`
		AllowSquashMerge          *bool    `json:"allow_squash_merge"`
		AllowMergeCommit          *bool    `json:"allow_merge_commit"`
		AllowRebaseMerge          *bool    `json:"allow_rebase_merge"`
		AllowAutoMerge            *bool    `json:"allow_auto_merge"`
		DeleteBranchOnMerge       *bool    `json:"delete_branch_on_merge"`
		UseSquashPRTitleAsDefault *bool    `json:"use_squash_pr_title_as_default"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "Repository creation failed.")
		return
	}

	private := bool(req.Private)
	if req.Visibility != "" {
		switch req.Visibility {
		case "public":
			private = false
		case "private", "internal":
			private = true
		default:
			writeGHValidationError(w, "Repository", "visibility", "invalid")
			return
		}
	}

	defaultBranch := req.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	repo := s.store.CreateOrgRepo(org, user, req.Name, req.Description, private)
	if repo == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Repository creation failed.")
		return
	}

	s.store.UpdateRepo(org.Login, req.Name, func(r *Repo) {
		r.Homepage = req.Homepage
		if req.HasIssues != nil {
			r.HasIssues = *req.HasIssues
		}
		if req.HasProjects != nil {
			r.HasProjects = *req.HasProjects
		}
		if req.HasWiki != nil {
			r.HasWiki = *req.HasWiki
		}
		if req.HasDiscussions != nil {
			r.HasDiscussions = boolPointer(*req.HasDiscussions)
		}
		if req.HasPullRequests != nil {
			r.HasPullRequests = *req.HasPullRequests
		}
		if req.AllowSquashMerge != nil {
			r.AllowSquashMerge = *req.AllowSquashMerge
		}
		if req.AllowMergeCommit != nil {
			r.AllowMergeCommit = *req.AllowMergeCommit
		}
		if req.AllowRebaseMerge != nil {
			r.AllowRebaseMerge = *req.AllowRebaseMerge
		}
		if req.AllowAutoMerge != nil {
			r.AllowAutoMerge = *req.AllowAutoMerge
		}
		if req.DeleteBranchOnMerge != nil {
			r.DeleteBranchOnMerge = *req.DeleteBranchOnMerge
		}
		if req.UseSquashPRTitleAsDefault != nil {
			r.UseSquashPRTitleAsDefault = *req.UseSquashPRTitleAsDefault
		}
	})

	if defaultBranch != "main" {
		s.store.UpdateRepo(org.Login, req.Name, func(r *Repo) {
			r.DefaultBranch = defaultBranch
		})
	}

	if bool(req.AutoInit) || req.GitignoreTemplate != "" || req.LicenseTemplate != "" {
		if err := s.initRepoFiles(r.Context(), repo, defaultBranch, req.Description, req.GitignoreTemplate, req.LicenseTemplate, bool(req.AutoInit)); err != nil {
			if _, deleteErr := s.store.DeleteRepo(org.Login, req.Name); deleteErr != nil {
				writeGHError(w, http.StatusInternalServerError, "repository rollback failed: "+deleteErr.Error())
				return
			}
			writeGHError(w, http.StatusUnprocessableEntity, "Repository creation failed.")
			return
		}
	}

	repo = s.store.GetRepo(org.Login, req.Name)
	writeJSON(w, http.StatusCreated, fullRepoJSONForViewer(repo, s.store, s.baseURL(r), ghUserFromContext(r.Context())))
}

// orgAsSimpleUserJSON converts an Org to the simple-user shape GitHub uses
// as the owner field of repositories owned by an organization. The fields
// are identical to userToJSON; only the type differs.
func orgAsSimpleUserJSON(org *Org) map[string]interface{} {
	api := "/api/v3/users/" + org.Login
	return map[string]interface{}{
		"login":               org.Login,
		"id":                  org.ID,
		"node_id":             org.NodeID,
		"avatar_url":          org.AvatarURL,
		"gravatar_id":         "",
		"url":                 api,
		"html_url":            "/" + org.Login,
		"followers_url":       api + "/followers",
		"following_url":       api + "/following{/other_user}",
		"gists_url":           api + "/gists{/gist_id}",
		"starred_url":         api + "/starred{/owner}{/repo}",
		"subscriptions_url":   api + "/subscriptions",
		"organizations_url":   api + "/orgs",
		"repos_url":           api + "/repos",
		"events_url":          api + "/events{/privacy}",
		"received_events_url": api + "/received_events",
		"type":                org.Type,
		"site_admin":          false,
		"name":                org.Name,
		"email":               org.Email,
		"user_view_type":      "public",
	}
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
	out["company"] = org.Company
	out["blog"] = org.Blog
	out["location"] = org.Location
	out["twitter_username"] = org.TwitterUsername
	out["billing_email"] = org.BillingEmail
	out["is_verified"] = false
	out["web_commit_signoff_required"] = org.WebCommitSignoffRequired
	// two_factor_requirement_enabled: bleephub has no 2FA model, so the
	// requirement is honestly never enabled.
	out["two_factor_requirement_enabled"] = false
	out["default_repository_permission"] = org.DefaultRepositoryPermission
	if org.DefaultRepositoryPermission == "" {
		out["default_repository_permission"] = "read" // GitHub's default
	}
	membersCanCreate := true // GitHub's default
	if org.MembersCanCreateRepositories != nil {
		membersCanCreate = *org.MembersCanCreateRepositories
	}
	out["members_can_create_repositories"] = membersCanCreate
	return out
}
