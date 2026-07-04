package bleephub

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// decodeJSONBody decodes the JSON request body into v.
// On failure it writes a GitHub-style 400 response and returns false.
// Usage: if !decodeJSONBody(w, r, &req) { return }
func decodeJSONBody(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return false
	}
	return true
}

// decodeJSONBodyOptional decodes like decodeJSONBody but tolerates an
// entirely absent body — for endpoints whose request body is optional on
// real GitHub (PUT membership endpoints: go-github sends no body at all
// when called without options).
func decodeJSONBodyOptional(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil && err != io.EOF {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return false
	}
	return true
}

func (s *Server) registerGHRestRoutes() {
	s.route("GET /api/v3/", s.handleGHApiRoot)
	s.route("GET /api/v3/user", s.handleGHUser)
	s.route("GET /api/v3/users/{username}", s.handleGHUserByLogin)
	s.route("GET /api/v3/rate_limit", s.handleGHRateLimit)
}

// handleGHApiRoot returns the API root meta information.
// gh reads X-OAuth-Scopes from response headers to check token permissions.
func (s *Server) handleGHApiRoot(w http.ResponseWriter, r *http.Request) {
	// Must be exact match for /api/v3/ — don't match sub-paths
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/v3")
	if trimmed != "/" && trimmed != "" {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"current_user_url":                     "/api/v3/user",
		"current_user_authorizations_html_url": "/settings/connections/applications{/client_id}",
		"authorizations_url":                   "/api/v3/authorizations",
		"code_search_url":                      "/api/v3/search/code?q={query}{&page,per_page,sort,order}",
		"commit_search_url":                    "/api/v3/search/commits?q={query}{&page,per_page,sort,order}",
		"emails_url":                           "/api/v3/user/emails",
		"emojis_url":                           "/api/v3/emojis",
		"events_url":                           "/api/v3/events",
		"feeds_url":                            "/api/v3/feeds",
		"followers_url":                        "/api/v3/user/followers",
		"following_url":                        "/api/v3/user/following{/target}",
		"gists_url":                            "/api/v3/gists{/gist_id}",
		"hub_url":                              "/api/v3/hub",
		"issue_search_url":                     "/api/v3/search/issues?q={query}{&page,per_page,sort,order}",
		"issues_url":                           "/api/v3/issues",
		"keys_url":                             "/api/v3/user/keys",
		"label_search_url":                     "/api/v3/search/labels?q={query}&repository_id={repository_id}{&page,per_page}",
		"notifications_url":                    "/api/v3/notifications",
		"organization_url":                     "/api/v3/orgs/{org}",
		"organization_repositories_url":        "/api/v3/orgs/{org}/repos{?type,page,per_page,sort}",
		"organization_teams_url":               "/api/v3/orgs/{org}/teams",
		"public_gists_url":                     "/api/v3/gists/public",
		"rate_limit_url":                       "/api/v3/rate_limit",
		"repository_url":                       "/api/v3/repos/{owner}/{repo}",
		"repository_search_url":                "/api/v3/search/repositories?q={query}{&page,per_page,sort,order}",
		"current_user_repositories_url":        "/api/v3/user/repos{?type,page,per_page,sort}",
		"starred_url":                          "/api/v3/user/starred{/owner}{/repo}",
		"starred_gists_url":                    "/api/v3/gists/starred",
		"user_url":                             "/api/v3/users/{user}",
		"user_organizations_url":               "/api/v3/user/orgs",
		"user_repositories_url":                "/api/v3/users/{user}/repos{?type,page,per_page,sort}",
		"user_search_url":                      "/api/v3/search/users?q={query}{&page,per_page,sort,order}",
	})
}

// handleGHUser returns the authenticated user in the private-user shape
// (the account owner sees their own private counters).
func (s *Server) handleGHUser(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	writeJSON(w, http.StatusOK, s.privateUserJSON(user))
}

// handleGHUserByLogin returns a user by login name.
func (s *Server) handleGHUserByLogin(w http.ResponseWriter, r *http.Request) {
	login := r.PathValue("username")
	user := s.store.LookupUserByLogin(login)
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.fullUserJSON(user))
}

// handleGHRateLimit returns rate limit status.
func (s *Server) handleGHRateLimit(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	reset := now.Unix() + 3600

	limit := map[string]interface{}{
		"limit":     5000,
		"remaining": 4999,
		"reset":     reset,
		"used":      1,
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"resources": map[string]interface{}{
			"core":    limit,
			"graphql": limit,
			"search": map[string]interface{}{
				"limit":     30,
				"remaining": 29,
				"reset":     reset,
				"used":      1,
			},
		},
		"rate": limit,
	})
}

// writeGHError writes a GitHub-style error JSON response.
func writeGHError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"message":           message,
		"documentation_url": "https://docs.github.com/rest",
	})
}

// writeGHValidationError writes a GitHub 422 validation error with detailed errors array.
func writeGHValidationError(w http.ResponseWriter, resource, field, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":           "Validation Failed",
		"documentation_url": "https://docs.github.com/rest",
		"errors": []map[string]string{
			{
				"resource": resource,
				"field":    field,
				"code":     code,
			},
		},
	})
}

// userToJSON converts a User to the GitHub `simple-user` shape — the
// user object nested inside repos, issues, pulls, comments, reviews,
// memberships, and installations. The full public/private-user shape
// (bio, counters, timestamps) belongs only on GET /user and
// GET /users/{username}; see fullUserJSON.
func userToJSON(u *User) map[string]interface{} {
	api := "/api/v3/users/" + u.Login
	return map[string]interface{}{
		"login":               u.Login,
		"id":                  u.ID,
		"node_id":             u.NodeID,
		"avatar_url":          u.AvatarURL,
		"gravatar_id":         "",
		"url":                 api,
		"html_url":            "/" + u.Login,
		"followers_url":       api + "/followers",
		"following_url":       api + "/following{/other_user}",
		"gists_url":           api + "/gists{/gist_id}",
		"starred_url":         api + "/starred{/owner}{/repo}",
		"subscriptions_url":   api + "/subscriptions",
		"organizations_url":   api + "/orgs",
		"repos_url":           api + "/repos",
		"events_url":          api + "/events{/privacy}",
		"received_events_url": api + "/received_events",
		"type":                u.Type,
		"site_admin":          u.SiteAdmin,
		"name":                u.Name,
		"email":               u.Email,
		"user_view_type":      "public",
	}
}

// fullUserJSON converts a User to the GitHub `public-user` shape served
// by GET /user, GET /users/{username}, and GET /user/{account_id}: the
// simple-user members plus profile fields and counters. Profile members
// (blog, company, location, hireable, twitter_username) come from the
// stored user profile (mutable via PATCH /user); unset company/location/
// twitter_username are null, matching real GitHub. Followers/following
// and repository counts are derived live from the store; gists are not a
// bleephub feature so public_gists is 0.
func (s *Server) fullUserJSON(u *User) map[string]interface{} {
	out := userToJSON(u)
	out["bio"] = u.Bio
	out["blog"] = u.Blog
	out["company"] = nullableString(u.Company)
	out["location"] = nullableString(u.Location)
	out["twitter_username"] = nullableString(u.TwitterUsername)
	if u.Hireable != nil {
		out["hireable"] = *u.Hireable
	} else {
		out["hireable"] = nil
	}
	out["followers"] = s.store.CountFollowers(u.Login)
	out["following"] = s.store.CountFollowing(u.Login)
	out["public_repos"] = s.store.CountPublicRepos(u.Login)
	out["public_gists"] = 0
	out["created_at"] = u.CreatedAt.Format(time.RFC3339)
	out["updated_at"] = u.UpdatedAt.Format(time.RFC3339)
	return out
}
