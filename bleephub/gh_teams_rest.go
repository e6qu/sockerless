package bleephub

import (
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) registerGHTeamRoutes() {
	s.mux.HandleFunc("POST /api/v3/orgs/{org}/teams", s.handleCreateTeam)
	s.mux.HandleFunc("GET /api/v3/orgs/{org}/teams", s.handleListTeams)
	s.mux.HandleFunc("GET /api/v3/orgs/{org}/teams/{team_slug}", s.handleGetTeam)
	s.mux.HandleFunc("PATCH /api/v3/orgs/{org}/teams/{team_slug}", s.handleUpdateTeam)
	s.mux.HandleFunc("DELETE /api/v3/orgs/{org}/teams/{team_slug}", s.handleDeleteTeam)
}

func (s *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Privacy     string `json:"privacy"`
		Permission  string `json:"permission"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	if req.Name == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}

	team := s.store.CreateTeam(orgLogin, req.Name, req.Description, req.Privacy, req.Permission)
	if team == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}

	writeJSON(w, http.StatusCreated, teamToJSON(team, org, s.baseURL(r)))
}

func (s *Server) handleListTeams(w http.ResponseWriter, r *http.Request) {
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

	teams := s.store.ListTeams(orgLogin)
	result := make([]map[string]interface{}, 0, len(teams))
	base := s.baseURL(r)
	for _, team := range teams {
		result = append(result, teamToJSON(team, org, base))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetTeam(w http.ResponseWriter, r *http.Request) {
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

	slug := r.PathValue("team_slug")
	team := s.store.GetTeam(orgLogin, slug)
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	writeJSON(w, http.StatusOK, teamToJSON(team, org, s.baseURL(r)))
}

func (s *Server) handleUpdateTeam(w http.ResponseWriter, r *http.Request) {
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
	team := s.store.GetTeam(orgLogin, slug)
	if team == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}

	s.store.UpdateTeam(orgLogin, slug, func(t *Team) {
		if v, ok := req["name"].(string); ok {
			t.Name = v
			t.Slug = slugify(v)
		}
		if v, ok := req["description"].(string); ok {
			t.Description = v
		}
		if v, ok := req["privacy"].(string); ok {
			t.Privacy = v
		}
		if v, ok := req["permission"].(string); ok {
			t.Permission = v
		}
	})

	updated := s.store.GetTeam(orgLogin, slug)
	if updated == nil {
		// Slug may have changed due to name update — re-fetch
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, teamToJSON(updated, org, s.baseURL(r)))
}

func (s *Server) handleDeleteTeam(w http.ResponseWriter, r *http.Request) {
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
	if !s.store.DeleteTeam(orgLogin, slug) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// teamToJSON converts a Team to a JSON-compatible map with snake_case keys.
func teamToJSON(team *Team, org *Org, baseURL string) map[string]interface{} {
	return map[string]interface{}{
		"id":              team.ID,
		"node_id":         team.NodeID,
		"url":             baseURL + "/api/v3/orgs/" + org.Login + "/teams/" + team.Slug,
		"html_url":        baseURL + "/orgs/" + org.Login + "/teams/" + team.Slug,
		"name":            team.Name,
		"slug":            team.Slug,
		"description":     team.Description,
		"privacy":         team.Privacy,
		"permission":      team.Permission,
		"members_url":     baseURL + "/api/v3/orgs/" + org.Login + "/teams/" + team.Slug + "/members{/member}",
		"repositories_url": baseURL + "/api/v3/orgs/" + org.Login + "/teams/" + team.Slug + "/repos",
		"organization":    orgToJSON(org, baseURL),
		"created_at":      team.CreatedAt.Format(time.RFC3339),
		"updated_at":      team.UpdatedAt.Format(time.RFC3339),
	}
}
