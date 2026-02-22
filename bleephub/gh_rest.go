package bleephub

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (s *Server) registerGHRestRoutes() {
	s.mux.HandleFunc("GET /api/v3/", s.handleGHApiRoot)
	s.mux.HandleFunc("GET /api/v3/user", s.handleGHUser)
	s.mux.HandleFunc("GET /api/v3/users/{username}", s.handleGHUserByLogin)
	s.mux.HandleFunc("GET /api/v3/rate_limit", s.handleGHRateLimit)
}

// handleGHApiRoot returns the API root meta information.
// gh reads X-OAuth-Scopes from response headers to check token permissions.
func (s *Server) handleGHApiRoot(w http.ResponseWriter, r *http.Request) {
	// Must be exact match for /api/v3/ — don't match sub-paths
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/v3")
	if trimmed != "/" && trimmed != "" {
		http.NotFound(w, r)
		return
	}

	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"current_user_url":                 "/api/v3/user",
		"current_user_authorizations_html": "/api/v3/settings/connections/applications{/client_id}",
		"authorizations_url":               "/api/v3/authorizations",
		"emails_url":                       "/api/v3/user/emails",
		"emojis_url":                       "/api/v3/emojis",
		"events_url":                       "/api/v3/events",
		"feeds_url":                        "/api/v3/feeds",
		"followers_url":                    "/api/v3/user/followers",
		"following_url":                    "/api/v3/user/following{/target}",
		"gists_url":                        "/api/v3/gists{/gist_id}",
		"hub_url":                          "/api/v3/hub",
		"issue_search_url":                 "/api/v3/search/issues?q={query}",
		"issues_url":                       "/api/v3/issues",
		"rate_limit_url":                   "/api/v3/rate_limit",
		"repository_url":                   "/api/v3/repos/{owner}/{repo}",
		"starred_url":                      "/api/v3/user/starred{/owner}{/repo}",
		"user_url":                         "/api/v3/users/{user}",
	})
}

// handleGHUser returns the authenticated user.
func (s *Server) handleGHUser(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	writeJSON(w, http.StatusOK, userToJSON(user))
}

// handleGHUserByLogin returns a user by login name.
func (s *Server) handleGHUserByLogin(w http.ResponseWriter, r *http.Request) {
	login := r.PathValue("username")
	user := s.store.LookupUserByLogin(login)
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, userToJSON(user))
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

// userToJSON converts a User to a JSON-compatible map.
func userToJSON(u *User) map[string]interface{} {
	return map[string]interface{}{
		"login":      u.Login,
		"id":         u.ID,
		"node_id":    u.NodeID,
		"avatar_url": u.AvatarURL,
		"url":        "/api/v3/users/" + u.Login,
		"html_url":   "/" + u.Login,
		"type":       u.Type,
		"site_admin": u.SiteAdmin,
		"name":       u.Name,
		"email":      u.Email,
		"bio":        u.Bio,
		"created_at": u.CreatedAt.Format(time.RFC3339),
		"updated_at": u.UpdatedAt.Format(time.RFC3339),
	}
}
