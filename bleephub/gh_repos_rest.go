package bleephub

import (
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) registerGHRepoRoutes() {
	s.mux.HandleFunc("POST /api/v3/user/repos", s.handleCreateRepo)
	s.mux.HandleFunc("GET /api/v3/user/repos", s.handleListAuthUserRepos)
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}", s.handleGetRepo)
	s.mux.HandleFunc("PATCH /api/v3/repos/{owner}/{repo}", s.handleUpdateRepo)
	s.mux.HandleFunc("DELETE /api/v3/repos/{owner}/{repo}", s.handleDeleteRepo)
	s.mux.HandleFunc("GET /api/v3/users/{username}/repos", s.handleListUserRepos)
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
		Name        string `json:"name"`
		Description string `json:"description"`
		Private     bool   `json:"private"`
		AutoInit    bool   `json:"auto_init"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.Name == "" {
		writeGHValidationError(w, "Repository", "name", "missing_field")
		return
	}

	repo := s.store.CreateRepo(user, req.Name, req.Description, req.Private)
	if repo == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Repository creation failed.")
		return
	}

	writeJSON(w, http.StatusCreated, repoToJSON(repo, s.baseURL(r)))
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
	writeJSON(w, http.StatusOK, repoToJSON(repo, s.baseURL(r)))
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}

	s.store.UpdateRepo(owner, name, func(r *Repo) {
		if v, ok := req["description"].(string); ok {
			r.Description = v
		}
		if v, ok := req["default_branch"].(string); ok {
			r.DefaultBranch = v
		}
		if v, ok := req["private"].(bool); ok {
			r.Private = v
			if v {
				r.Visibility = "private"
			} else {
				r.Visibility = "public"
			}
		}
		if v, ok := req["archived"].(bool); ok {
			r.Archived = v
		}
	})

	updated := s.store.GetRepo(owner, name)
	writeJSON(w, http.StatusOK, repoToJSON(updated, s.baseURL(r)))
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
		result = append(result, repoToJSON(repo, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleListUserRepos(w http.ResponseWriter, r *http.Request) {
	login := r.PathValue("username")
	repos := s.store.ListReposByOwner(login)
	result := make([]map[string]interface{}, 0, len(repos))
	base := s.baseURL(r)
	for _, repo := range repos {
		result = append(result, repoToJSON(repo, base))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

// baseURL computes the external base URL from the request.
func (s *Server) baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// repoToJSON converts a Repo to a JSON-compatible map.
func repoToJSON(repo *Repo, baseURL string) map[string]interface{} {
	ownerJSON := map[string]interface{}{}
	if repo.Owner != nil {
		ownerJSON = userToJSON(repo.Owner)
	}

	topics := repo.Topics
	if topics == nil {
		topics = []string{}
	}

	return map[string]interface{}{
		"id":               repo.ID,
		"node_id":          repo.NodeID,
		"name":             repo.Name,
		"full_name":        repo.FullName,
		"owner":            ownerJSON,
		"private":          repo.Private,
		"html_url":         baseURL + "/" + repo.FullName,
		"description":      repo.Description,
		"fork":             repo.Fork,
		"url":              baseURL + "/api/v3/repos/" + repo.FullName,
		"clone_url":        baseURL + "/" + repo.FullName + ".git",
		"ssh_url":          "git@bleephub.local:" + repo.FullName + ".git",
		"default_branch":   repo.DefaultBranch,
		"visibility":       repo.Visibility,
		"language":         repo.Language,
		"archived":         repo.Archived,
		"stargazers_count": repo.StargazersCount,
		"topics":           topics,
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
