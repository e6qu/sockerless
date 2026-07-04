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
	// /rulesets/{ruleset_id}/history and /rulesets/rule-suites/{rule_suite_id}
	// both occupy two segments after /rulesets and cannot both be registered
	// directly with Go 1.22's mux; dispatch on the literal segments.
	s.route("GET /api/v3/repos/{owner}/{repo}/rulesets/{p1}/{p2}", s.handleRepoRulesetTwoSegDispatch)
	s.route("GET /api/v3/repos/{owner}/{repo}/rulesets/{ruleset_id}/history/{version_id}", s.handleGetRulesetVersion)

	s.registerGHOrgRulesetRoutes()
}

func (s *Server) handleRepoRulesetTwoSegDispatch(w http.ResponseWriter, r *http.Request) {
	p1 := r.PathValue("p1")
	p2 := r.PathValue("p2")
	switch {
	case p1 == "rule-suites":
		r.SetPathValue("rule_suite_id", p2)
		s.handleGetRepoRuleSuite(w, r)
	case p2 == "history":
		r.SetPathValue("ruleset_id", p1)
		s.handleListRulesetHistory(w, r)
	default:
		writeGHError(w, http.StatusNotFound, "Not Found")
	}
}

// handleGetRepoRuleSuite serves GET /repos/{owner}/{repo}/rulesets/rule-suites/{rule_suite_id}.
// bleephub does not evaluate rulesets on push, so no rule suites are ever
// recorded and every lookup is a real 404 — the same truthful empty state the
// organization-level rule-suite surface serves.
func (s *Server) handleGetRepoRuleSuite(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.resolveRepo(w, r)
	if repo == nil {
		return
	}
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	suiteID, err := strconv.Atoi(r.PathValue("rule_suite_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	suite := s.store.GetRepoRulesetSuite(repo.ID, suiteID)
	if suite == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, rulesetSuiteToJSON(suite))
}

func (s *Server) registerGHOrgRulesetRoutes() {
	s.route("GET /api/v3/orgs/{org}/rulesets", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleListOrgRulesets))
	s.route("POST /api/v3/orgs/{org}/rulesets", s.requireOrgAdmin(scopeOrgAdministration, permWrite, s.handleCreateOrgRuleset))
	s.route("GET /api/v3/orgs/{org}/rulesets/rule-suites", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleListOrgRuleSuites))
	s.route("GET /api/v3/orgs/{org}/rulesets/{ruleset_id}", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleGetOrgRuleset))
	s.route("PUT /api/v3/orgs/{org}/rulesets/{ruleset_id}", s.requireOrgAdmin(scopeOrgAdministration, permWrite, s.handleUpdateOrgRuleset))
	s.route("DELETE /api/v3/orgs/{org}/rulesets/{ruleset_id}", s.requireOrgAdmin(scopeOrgAdministration, permWrite, s.handleDeleteOrgRuleset))
	s.route("GET /api/v3/orgs/{org}/rulesets/{p1}/{p2}", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleOrgRulesetTwoSegDispatch("GET")))
	s.route("GET /api/v3/orgs/{org}/rulesets/{p1}/{p2}/{p3}", s.requireOrgAdmin(scopeOrgAdministration, permRead, s.handleOrgRulesetThreeSegDispatch("GET")))
}

func (s *Server) handleOrgRulesetTwoSegDispatch(method string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p1 := r.PathValue("p1")
		p2 := r.PathValue("p2")
		switch {
		case p1 == "rule-suites":
			r.SetPathValue("rule_suite_id", p2)
			s.handleGetOrgRuleSuite(w, r)
		case p2 == "history":
			r.SetPathValue("ruleset_id", p1)
			s.handleListOrgRulesetHistory(w, r)
		default:
			writeGHError(w, http.StatusNotFound, "Not Found")
		}
	}
}

func (s *Server) handleOrgRulesetThreeSegDispatch(method string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p1 := r.PathValue("p1")
		p2 := r.PathValue("p2")
		p3 := r.PathValue("p3")
		if p2 == "history" {
			r.SetPathValue("ruleset_id", p1)
			r.SetPathValue("version_id", p3)
			s.handleGetOrgRulesetVersion(w, r)
			return
		}
		writeGHError(w, http.StatusNotFound, "Not Found")
	}
}

// requireOrgAdmin enforces an organization-administration permission and
// verifies the caller is an admin of the target organization.
func (s *Server) requireOrgAdmin(scope permScope, level permLevel, next http.HandlerFunc) http.HandlerFunc {
	return s.requirePerm(scope, level, func(w http.ResponseWriter, r *http.Request) {
		user := ghUserFromContext(r.Context())
		org := s.store.GetOrg(r.PathValue("org"))
		if org == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		if !canAdminOrg(s.store, user, org) {
			writeGHError(w, http.StatusForbidden, "Must have admin rights to Organization.")
			return
		}
		next(w, r)
	})
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
	updated := s.store.UpdateRuleset(repo, rs, &body, user.ID)
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
		out[i] = rulesetVersionJSON(v, false)
	}
	writeJSON(w, http.StatusOK, out)
}

// rulesetVersionJSON renders the GitHub ruleset-version shape (plus the
// ruleset snapshot as `state` for the single-version endpoints).
func rulesetVersionJSON(v RulesetVersion, withState bool) map[string]interface{} {
	actor := map[string]interface{}{}
	if v.ActorID != 0 {
		actor["id"] = v.ActorID
		actor["type"] = "User"
	}
	out := map[string]interface{}{
		"version_id": v.VersionID,
		"actor":      actor,
		"updated_at": v.CreatedAt.UTC().Format(time.RFC3339),
	}
	if withState {
		out["state"] = rulesetToJSON(&v.Ruleset, true)
	}
	return out
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
	writeJSON(w, http.StatusOK, rulesetVersionJSON(*version, true))
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

func (s *Server) handleListOrgRulesets(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	rulesets := s.store.ListOrgRulesets(org.ID)
	out := make([]map[string]interface{}, len(rulesets))
	for i, rs := range rulesets {
		out[i] = rulesetToJSON(rs, false)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateOrgRuleset(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
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
	rs := s.store.CreateOrgRuleset(org.ID, body.Name, body.Target, body.Enforcement, body.Conditions, body.Rules)
	writeJSON(w, http.StatusCreated, rulesetToJSON(rs, true))
}

func (s *Server) handleGetOrgRuleset(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	rs := s.lookupOrgRuleset(w, r, org)
	if rs == nil {
		return
	}
	writeJSON(w, http.StatusOK, rulesetToJSON(rs, true))
}

func (s *Server) handleUpdateOrgRuleset(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	rs := s.lookupOrgRuleset(w, r, org)
	if rs == nil {
		return
	}
	var body Ruleset
	if !decodeJSONBody(w, r, &body) {
		return
	}
	if !s.store.UpdateOrgRuleset(rs.ID, user.ID, func(rs *Ruleset) {
		if body.Name != "" {
			rs.Name = body.Name
		}
		if body.Target != "" {
			rs.Target = body.Target
		}
		if body.Enforcement != "" {
			rs.Enforcement = body.Enforcement
		}
		if body.BypassActors != nil {
			rs.BypassActors = body.BypassActors
		}
		if body.CurrentUserCanBypass != "" {
			rs.CurrentUserCanBypass = body.CurrentUserCanBypass
		}
		if len(body.Conditions.RefName.Include) > 0 || len(body.Conditions.RefName.Exclude) > 0 {
			rs.Conditions = body.Conditions
		}
		if body.Rules != nil {
			rs.Rules = body.Rules
		}
	}) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	updated := s.store.GetRuleset(rs.ID)
	writeJSON(w, http.StatusOK, rulesetToJSON(updated, true))
}

func (s *Server) handleDeleteOrgRuleset(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	rs := s.lookupOrgRuleset(w, r, org)
	if rs == nil {
		return
	}
	s.store.DeleteOrgRuleset(rs.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListOrgRuleSuites(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	suites := s.store.ListOrgRulesetSuites(org.ID)
	out := make([]map[string]interface{}, len(suites))
	for i, suite := range suites {
		out[i] = rulesetSuiteToJSON(&suite)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetOrgRuleSuite(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	suiteID, err := strconv.Atoi(r.PathValue("rule_suite_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	suite := s.store.GetOrgRulesetSuite(org.ID, suiteID)
	if suite == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, rulesetSuiteToJSON(suite))
}

func (s *Server) handleListOrgRulesetHistory(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	rs := s.lookupOrgRuleset(w, r, org)
	if rs == nil {
		return
	}
	versions := s.store.GetRulesetHistory(rs)
	out := make([]map[string]interface{}, len(versions))
	for i, v := range versions {
		out[i] = rulesetVersionJSON(v, false)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetOrgRulesetVersion(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	rs := s.lookupOrgRuleset(w, r, org)
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
	writeJSON(w, http.StatusOK, rulesetVersionJSON(*version, true))
}

func (s *Server) lookupOrgRuleset(w http.ResponseWriter, r *http.Request, org *Org) *Ruleset {
	idStr := r.PathValue("ruleset_id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	rs := s.store.GetOrgRuleset(id)
	if rs == nil || rs.OrgID != org.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return rs
}

func rulesetSuiteToJSON(suite *RulesetSuite) map[string]interface{} {
	return map[string]interface{}{
		"id":         suite.ID,
		"node_id":    suite.NodeID,
		"ruleset_id": suite.RulesetID,
		"status":     suite.Status,
		"created_at": suite.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": suite.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
