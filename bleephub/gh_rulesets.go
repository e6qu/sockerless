package bleephub

import (
	"net/http"
	"strconv"
	"time"
)

func (s *Server) registerGHRulesetRoutes() {
	s.route("GET /api/v3/repos/{owner}/{repo}/rulesets", s.handleListRulesets)
	s.route("POST /api/v3/repos/{owner}/{repo}/rulesets", s.handleCreateRuleset)
	s.route("GET /api/v3/repos/{owner}/{repo}/rulesets/{ruleset_id}", s.handleGetRuleset)
	s.route("PUT /api/v3/repos/{owner}/{repo}/rulesets/{ruleset_id}", s.handleUpdateRuleset)
	s.route("DELETE /api/v3/repos/{owner}/{repo}/rulesets/{ruleset_id}", s.handleDeleteRuleset)
	s.route("GET /api/v3/repos/{owner}/{repo}/rules/branches/{branch}", s.handleListBranchRules)
	s.route("GET /api/v3/repos/{owner}/{repo}/rulesets/{ruleset_id}/history", s.handleListRulesetHistory)
	s.route("GET /api/v3/repos/{owner}/{repo}/rulesets/{ruleset_id}/history/{version_id}", s.handleGetRulesetVersion)
}

func (s *Server) resolveRepo(w http.ResponseWriter, r *http.Request) *Repo {
	owner, repoName := r.PathValue("owner"), r.PathValue("repo")
	repo := s.store.ReposByName[owner+"/"+repoName]
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return repo
}

func (s *Server) handleListRulesets(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.resolveRepo(w, r)
	if repo == nil {
		return
	}
	if !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	rulesets := s.store.ListRulesetsForRepo(repo.ID)
	out := make([]map[string]interface{}, len(rulesets))
	for i, rs := range rulesets {
		out[i] = rulesetToJSON(rs, false)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateRuleset(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.resolveRepo(w, r)
	if repo == nil {
		return
	}
	if !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var body Ruleset
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if body.Name == "" {
		writeGHValidationError(w, "ruleset", "name", "missing_field")
		return
	}
	rs := s.store.CreateRuleset(repo, &body)
	writeJSON(w, http.StatusCreated, rulesetToJSON(rs, true))
}

func (s *Server) handleGetRuleset(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.resolveRepo(w, r)
	if repo == nil {
		return
	}
	if !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	rs := s.lookupRuleset(w, r, repo)
	if rs == nil {
		return
	}
	writeJSON(w, http.StatusOK, rulesetToJSON(rs, true))
}

func (s *Server) handleUpdateRuleset(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.resolveRepo(w, r)
	if repo == nil {
		return
	}
	if !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	rs := s.lookupRuleset(w, r, repo)
	if rs == nil {
		return
	}
	var body Ruleset
	if !decodeJSONBody(w, r, &body) {
		return
	}
	updated := s.store.UpdateRuleset(repo, rs, &body)
	writeJSON(w, http.StatusOK, rulesetToJSON(updated, true))
}

func (s *Server) handleDeleteRuleset(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.resolveRepo(w, r)
	if repo == nil {
		return
	}
	if !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	rs := s.lookupRuleset(w, r, repo)
	if rs == nil {
		return
	}
	s.store.DeleteRuleset(rs.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListBranchRules(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.resolveRepo(w, r)
	if repo == nil {
		return
	}
	if !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	branch := r.PathValue("branch")
	out := s.store.ListRulesForBranch(repo, branch)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListRulesetHistory(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.resolveRepo(w, r)
	if repo == nil {
		return
	}
	if !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	rs := s.lookupRuleset(w, r, repo)
	if rs == nil {
		return
	}
	versions := s.store.GetRulesetHistory(rs)
	out := make([]map[string]interface{}, len(versions))
	for i, v := range versions {
		out[i] = map[string]interface{}{
			"version_id": v.VersionID,
			"ruleset":    rulesetToJSON(&v.Ruleset, true),
			"created_at": v.CreatedAt.UTC().Format(time.RFC3339),
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetRulesetVersion(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.resolveRepo(w, r)
	if repo == nil {
		return
	}
	if !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	rs := s.lookupRuleset(w, r, repo)
	if rs == nil {
		return
	}
	versionID, err := strconv.Atoi(r.PathValue("version_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	version := s.store.GetRulesetVersion(rs, versionID)
	if version == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version_id": version.VersionID,
		"ruleset":    rulesetToJSON(&version.Ruleset, true),
		"created_at": version.CreatedAt.UTC().Format(time.RFC3339),
	})
}

func (s *Server) lookupRuleset(w http.ResponseWriter, r *http.Request, repo *Repo) *Ruleset {
	idStr := r.PathValue("ruleset_id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	rs := s.store.GetRuleset(id)
	if rs == nil || rs.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return rs
}

func rulesetToJSON(rs *Ruleset, includeBody bool) map[string]interface{} {
	m := map[string]interface{}{
		"id":                      rs.ID,
		"node_id":                 rs.NodeID,
		"name":                    rs.Name,
		"target":                  rs.Target,
		"source_type":             rs.SourceType,
		"source":                  rs.Source,
		"enforcement":             rs.Enforcement,
		"bypass_actors":           rs.BypassActors,
		"current_user_can_bypass": rs.CurrentUserCanBypass,
		"created_at":              rs.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":              rs.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if includeBody {
		m["conditions"] = rs.Conditions
		m["rules"] = rs.Rules
	}
	return m
}
