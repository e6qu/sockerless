package bleephub

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerGHRepoRoutes() {
	s.route("POST /api/v3/user/repos", s.requirePerm(scopeContents, permWrite, s.handleCreateRepo))
	s.route("GET /api/v3/user/repos", s.handleListAuthUserRepos)
	s.route("GET /api/v3/repos/{owner}/{repo}", s.handleGetRepo)
	s.route("PATCH /api/v3/repos/{owner}/{repo}", s.requirePerm(scopeAdministration, permWrite, s.handleUpdateRepo))
	s.route("DELETE /api/v3/repos/{owner}/{repo}", s.requirePerm(scopeAdministration, permWrite, s.handleDeleteRepo))
	s.route("GET /api/v3/users/{username}/repos", s.handleListUserRepos)
	s.route("GET /api/v3/orgs/{org}/repos", s.handleListOrgRepos)
	s.registerGHRepoRefRoutes()
	s.registerGHRepoObjectRoutes()
}

func (s *Server) handleCreateRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
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
		writeGHValidationError(w, "Repository", "name", "missing_field")
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

	repo := s.store.CreateRepo(user, req.Name, req.Description, private)
	if repo == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Repository creation failed.")
		return
	}

	s.store.UpdateRepo(user.Login, req.Name, func(r *Repo) {
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
		s.store.UpdateRepo(user.Login, req.Name, func(r *Repo) {
			r.DefaultBranch = defaultBranch
		})
	}

	if bool(req.AutoInit) || req.GitignoreTemplate != "" || req.LicenseTemplate != "" {
		if err := s.initRepoFiles(r.Context(), repo, defaultBranch, req.Description, req.GitignoreTemplate, req.LicenseTemplate, bool(req.AutoInit)); err != nil {
			s.store.DeleteRepo(user.Login, req.Name)
			writeGHError(w, http.StatusUnprocessableEntity, "Repository creation failed.")
			return
		}
	}

	repo = s.store.GetRepo(user.Login, req.Name)
	s.recordAuditEvent("repo.create", user.Login, "", map[string]interface{}{"repo": repo.FullName, "repo_id": repo.ID})
	writeJSON(w, http.StatusCreated, fullRepoJSON(repo, s.store, s.baseURL(r)))
}

func (s *Server) handleGetRepo(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	user := ghUserFromContext(r.Context())
	if repo.Private && !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, fullRepoJSON(repo, s.store, s.baseURL(r)))
}

func (s *Server) handleUpdateRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "Must have admin rights to Repository.")
		return
	}

	var req map[string]interface{}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	// GitHub supports renaming a repo via PATCH, but bleephub does not yet
	// implement the full cascade required for a rename. Reject explicitly
	// rather than silently ignore.
	if newName, ok := req["name"].(string); ok && newName != "" && newName != repo.Name {
		writeGHError(w, http.StatusUnprocessableEntity, "Repository rename is not supported.")
		return
	}

	s.store.UpdateRepo(owner, name, func(r *Repo) {
		if v, ok := req["description"].(string); ok {
			r.Description = v
		}
		if v, ok := req["homepage"].(string); ok {
			r.Homepage = v
		}
		if v, ok := req["default_branch"].(string); ok {
			r.DefaultBranch = v
		}
		if v, ok := coerceBool(req["private"]); ok {
			r.Private = v
			if v {
				r.Visibility = "private"
			} else {
				r.Visibility = "public"
			}
		}
		if v, ok := coerceBool(req["has_issues"]); ok {
			r.HasIssues = v
		}
		if v, ok := coerceBool(req["has_projects"]); ok {
			r.HasProjects = v
		}
		if v, ok := coerceBool(req["has_wiki"]); ok {
			r.HasWiki = v
		}
		if v, ok := coerceBool(req["has_pull_requests"]); ok {
			r.HasPullRequests = v
		}
		if v, ok := coerceBool(req["archived"]); ok {
			r.Archived = v
		}
		if v, ok := coerceBool(req["is_template"]); ok {
			r.IsTemplate = v
		}
		if v, ok := coerceBool(req["web_commit_signoff_required"]); ok {
			r.WebCommitSignoffRequired = v
		}
		if v, ok := coerceBool(req["allow_squash_merge"]); ok {
			r.AllowSquashMerge = v
		}
		if v, ok := coerceBool(req["allow_merge_commit"]); ok {
			r.AllowMergeCommit = v
		}
		if v, ok := coerceBool(req["allow_rebase_merge"]); ok {
			r.AllowRebaseMerge = v
		}
		if v, ok := coerceBool(req["allow_auto_merge"]); ok {
			r.AllowAutoMerge = v
		}
		if v, ok := coerceBool(req["allow_update_branch"]); ok {
			r.AllowUpdateBranch = v
		}
		if v, ok := coerceBool(req["delete_branch_on_merge"]); ok {
			r.DeleteBranchOnMerge = v
		}
		if v, ok := coerceBool(req["use_squash_pr_title_as_default"]); ok {
			r.UseSquashPRTitleAsDefault = v
		}
		if v, ok := req["squash_merge_commit_title"].(string); ok {
			r.SquashMergeCommitTitle = v
		}
		if v, ok := req["squash_merge_commit_message"].(string); ok {
			r.SquashMergeCommitMessage = v
		}
		if v, ok := req["merge_commit_title"].(string); ok {
			r.MergeCommitTitle = v
		}
		if v, ok := req["merge_commit_message"].(string); ok {
			r.MergeCommitMessage = v
		}
		if v, ok := req["pull_request_creation_policy"].(string); ok {
			r.PullRequestCreationPolicy = v
		}
	})

	updated := s.store.GetRepo(owner, name)
	writeJSON(w, http.StatusOK, fullRepoJSON(updated, s.store, s.baseURL(r)))
}

func (s *Server) handleDeleteRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "Must have admin rights to Repository.")
		return
	}

	s.store.DeleteRepo(owner, name)
	s.recordAuditEvent("repo.destroy", user.Login, "", map[string]interface{}{"repo": owner + "/" + name})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAuthUserRepos(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	opts := repoListOptionsFromQuery(r)
	// GitHub rejects type combined with visibility or affiliation.
	if r.URL.Query().Get("type") != "" && (r.URL.Query().Get("visibility") != "" || r.URL.Query().Get("affiliation") != "") {
		writeGHValidationError(w, "Repository", "type", "invalid")
		return
	}
	opts.NoPaginate = true // REST handlers use paginateAndLink for Link headers

	repos := s.store.ListReposForAuthUser(user, opts)
	result := make([]map[string]interface{}, 0, len(repos))
	base := s.baseURL(r)
	for _, repo := range repos {
		result = append(result, repoToJSON(repo, s.store, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleListUserRepos(w http.ResponseWriter, r *http.Request) {
	login := r.PathValue("username")
	user := s.store.LookupUserByLogin(login)
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	opts := repoListOptionsFromQuery(r)
	opts.Affiliation = ""  // not applicable
	opts.Visibility = ""   // not applicable
	opts.NoPaginate = true // REST handlers use paginateAndLink for Link headers
	repos := s.store.ListReposForUser(user, opts)
	result := make([]map[string]interface{}, 0, len(repos))
	base := s.baseURL(r)
	for _, repo := range repos {
		result = append(result, repoToJSON(repo, s.store, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleListOrgRepos(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	org := s.store.GetOrg(orgLogin)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	opts := repoListOptionsFromQuery(r)
	opts.Affiliation = ""  // not applicable
	opts.NoPaginate = true // REST handlers use paginateAndLink for Link headers
	repos := s.store.ListReposForOrg(org.Login, opts)
	result := make([]map[string]interface{}, 0, len(repos))
	base := s.baseURL(r)
	for _, repo := range repos {
		result = append(result, repoToJSON(repo, s.store, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func repoListOptionsFromQuery(r *http.Request) RepoListOptions {
	q := r.URL.Query()
	return RepoListOptions{
		Type:        q.Get("type"),
		Visibility:  q.Get("visibility"),
		Affiliation: q.Get("affiliation"),
		Sort:        q.Get("sort"),
		Direction:   q.Get("direction"),
		PerPage:     queryInt(q, "per_page", 30),
		Page:        queryInt(q, "page", 1),
	}
}

func queryInt(q url.Values, key string, def int) int {
	s := q.Get(key)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// baseURL computes the external base URL. BLEEPHUB_EXTERNAL_URL wins
// when configured (the GHES "external URL" knob — job messages and
// links must carry an address RUNNERS can reach, not whichever
// interface a triggering API call happened to arrive on); otherwise
// the request's Host.
func (s *Server) baseURL(r *http.Request) string {
	if s.externalURL != "" {
		return s.externalURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// repoToJSON converts a Repo to the GitHub `repository` shape (also a
// valid `minimal-repository`). The hypermedia *_url members carry the
// literal URI-template placeholders real GitHub emits ({/sha}, {+path},
// …). Counters for features bleephub does not model (forks, size) are
// 0; watchers mirrors stargazers exactly as on real GitHub; the has_*
// toggles reflect the surfaces bleephub actually serves. Must not be
// called with st.mu held: it derives open_issues_count from the store.
func repoToJSON(repo *Repo, st *Store, baseURL string) map[string]interface{} {
	ownerJSON := map[string]interface{}{}
	if repo.OwnerType == "Organization" {
		parts := strings.SplitN(repo.FullName, "/", 2)
		if len(parts) == 2 {
			if org := st.GetOrg(parts[0]); org != nil {
				ownerJSON = orgAsSimpleUserJSON(org)
			}
		}
	} else if repo.Owner != nil {
		ownerJSON = userToJSON(repo.Owner)
	}

	topics := repo.Topics
	if topics == nil {
		topics = []string{}
	}

	host := baseURL
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	api := baseURL + "/api/v3/repos/" + repo.FullName
	openIssues := st.CountOpenIssues(repo.ID)

	return map[string]interface{}{
		"id":                repo.ID,
		"node_id":           repo.NodeID,
		"name":              repo.Name,
		"full_name":         repo.FullName,
		"owner":             ownerJSON,
		"private":           repo.Private,
		"html_url":          baseURL + "/" + repo.FullName,
		"description":       repo.Description,
		"fork":              repo.Fork,
		"url":               api,
		"archive_url":       api + "/{archive_format}{/ref}",
		"assignees_url":     api + "/assignees{/user}",
		"blobs_url":         api + "/git/blobs{/sha}",
		"branches_url":      api + "/branches{/branch}",
		"collaborators_url": api + "/collaborators{/collaborator}",
		"comments_url":      api + "/comments{/number}",
		"commits_url":       api + "/commits{/sha}",
		"compare_url":       api + "/compare/{base}...{head}",
		"contents_url":      api + "/contents/{+path}",
		"contributors_url":  api + "/contributors",
		"deployments_url":   api + "/deployments",
		"downloads_url":     api + "/downloads",
		"events_url":        api + "/events",
		"forks_url":         api + "/forks",
		"git_commits_url":   api + "/git/commits{/sha}",
		"git_refs_url":      api + "/git/refs{/sha}",
		"git_tags_url":      api + "/git/tags{/sha}",
		"hooks_url":         api + "/hooks",
		"issue_comment_url": api + "/issues/comments{/number}",
		"issue_events_url":  api + "/issues/events{/number}",
		"issues_url":        api + "/issues{/number}",
		"keys_url":          api + "/keys{/key_id}",
		"labels_url":        api + "/labels{/name}",
		"languages_url":     api + "/languages",
		"merges_url":        api + "/merges",
		"milestones_url":    api + "/milestones{/number}",
		"notifications_url": api + "/notifications{?since,all,participating}",
		"pulls_url":         api + "/pulls{/number}",
		"releases_url":      api + "/releases{/id}",
		"stargazers_url":    api + "/stargazers",
		"statuses_url":      api + "/statuses/{sha}",
		"subscribers_url":   api + "/subscribers",
		"subscription_url":  api + "/subscription",
		"tags_url":          api + "/tags",
		"teams_url":         api + "/teams",
		"trees_url":         api + "/git/trees{/sha}",
		"clone_url":         baseURL + "/" + repo.FullName + ".git",
		"git_url":           "git://" + host + "/" + repo.FullName + ".git",
		"ssh_url":           "git@bleephub.local:" + repo.FullName + ".git",
		"svn_url":           baseURL + "/" + repo.FullName,
		"mirror_url":        nil,
		"homepage":          nilOrString(repo.Homepage),
		"license":           licenseJSON(repo),
		"default_branch":    repo.DefaultBranch,
		"visibility":        repo.Visibility,
		"language":          repo.Language,
		"archived":          repo.Archived,
		"disabled":          false,
		"forks":             0,
		"forks_count":       0,
		"size":              st.RepoSize(repo.FullName),
		"stargazers_count":  repo.StargazersCount,
		"watchers":          repo.StargazersCount,
		"watchers_count":    repo.StargazersCount,
		"open_issues":       openIssues,
		"open_issues_count": openIssues,
		"has_issues":        repo.HasIssues,
		"has_projects":      repo.HasProjects,
		"has_wiki":          repo.HasWiki,
		"has_pages":         false,
		"has_downloads":     false,
		"has_discussions":   false,
		"has_pull_requests": repo.HasPullRequests,
		"topics":            topics,
		"permissions": map[string]bool{
			"admin": true,
			"push":  true,
			"pull":  true,
		},
		"created_at": repo.CreatedAt.Format(time.RFC3339),
		"updated_at": repo.UpdatedAt.Format(time.RFC3339),
		"pushed_at":  repo.PushedAt.Format(time.RFC3339),
	}
}

// fullRepoJSON converts a Repo to the GitHub `full-repository` shape
// served by single-repo operations (GET/PATCH /repos/{owner}/{repo},
// repo creation). It is the repository shape plus the network/subscriber
// counters that exist only on full-repository — bleephub models neither
// forks networks nor watch subscriptions, so both are truthfully 0.
func fullRepoJSON(repo *Repo, st *Store, baseURL string) map[string]interface{} {
	out := repoToJSON(repo, st, baseURL)
	out["network_count"] = 0
	out["subscribers_count"] = 0
	out["organization"] = repoOrganizationJSON(repo, st)
	out["allow_squash_merge"] = repo.AllowSquashMerge
	out["allow_merge_commit"] = repo.AllowMergeCommit
	out["allow_rebase_merge"] = repo.AllowRebaseMerge
	out["allow_auto_merge"] = repo.AllowAutoMerge
	out["allow_update_branch"] = repo.AllowUpdateBranch
	out["delete_branch_on_merge"] = repo.DeleteBranchOnMerge
	out["allow_forking"] = false
	out["web_commit_signoff_required"] = repo.WebCommitSignoffRequired
	out["is_template"] = repo.IsTemplate
	out["use_squash_pr_title_as_default"] = repo.UseSquashPRTitleAsDefault
	if repo.SquashMergeCommitTitle != "" {
		out["squash_merge_commit_title"] = repo.SquashMergeCommitTitle
	}
	if repo.SquashMergeCommitMessage != "" {
		out["squash_merge_commit_message"] = repo.SquashMergeCommitMessage
	}
	if repo.MergeCommitTitle != "" {
		out["merge_commit_title"] = repo.MergeCommitTitle
	}
	if repo.MergeCommitMessage != "" {
		out["merge_commit_message"] = repo.MergeCommitMessage
	}
	if repo.PullRequestCreationPolicy != "" {
		out["pull_request_creation_policy"] = repo.PullRequestCreationPolicy
	}
	return out
}

func nilOrString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func licenseJSON(repo *Repo) interface{} {
	if repo.LicenseKey == "" {
		return nil
	}
	return map[string]interface{}{
		"key":     repo.LicenseKey,
		"name":    repo.LicenseName,
		"spdx_id": repo.LicenseSPDX,
		"url":     nil,
		"node_id": "MDc6TGljZW5zZQ==" + repo.LicenseKey,
	}
}

func repoOrganizationJSON(repo *Repo, st *Store) interface{} {
	if repo.OwnerType != "Organization" {
		return nil
	}
	parts := strings.SplitN(repo.FullName, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	org := st.GetOrg(parts[0])
	if org == nil {
		return nil
	}
	return orgAsSimpleUserJSON(org)
}
