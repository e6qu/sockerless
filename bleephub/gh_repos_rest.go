package bleephub

import (
	"net/http"
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
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Private     flexBool `json:"private"`
		AutoInit    flexBool `json:"auto_init"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeGHValidationError(w, "Repository", "name", "missing_field")
		return
	}

	repo := s.store.CreateRepo(user, req.Name, req.Description, bool(req.Private))
	if repo == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Repository creation failed.")
		return
	}

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

	s.store.UpdateRepo(owner, name, func(r *Repo) {
		if v, ok := req["description"].(string); ok {
			r.Description = v
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
		if v, ok := coerceBool(req["archived"]); ok {
			r.Archived = v
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

	repos := s.store.ListReposByOwner(user.Login)
	result := make([]map[string]interface{}, 0, len(repos))
	base := s.baseURL(r)
	for _, repo := range repos {
		result = append(result, repoToJSON(repo, s.store, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleListUserRepos(w http.ResponseWriter, r *http.Request) {
	login := r.PathValue("username")
	repos := s.store.ListReposByOwner(login)
	result := make([]map[string]interface{}, 0, len(repos))
	base := s.baseURL(r)
	for _, repo := range repos {
		result = append(result, repoToJSON(repo, s.store, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
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
	if repo.Owner != nil {
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
		"homepage":          nil,
		"license":           nil,
		"default_branch":    repo.DefaultBranch,
		"visibility":        repo.Visibility,
		"language":          repo.Language,
		"archived":          repo.Archived,
		"disabled":          false,
		"forks":             0,
		"forks_count":       0,
		"size":              0,
		"stargazers_count":  repo.StargazersCount,
		"watchers":          repo.StargazersCount,
		"watchers_count":    repo.StargazersCount,
		"open_issues":       openIssues,
		"open_issues_count": openIssues,
		"has_issues":        true,
		"has_projects":      false,
		"has_wiki":          false,
		"has_pages":         false,
		"has_downloads":     false,
		"has_discussions":   false,
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
	return out
}
