package bleephub

import (
	"net/http"
	"sort"
)

func (s *Server) registerGHEnterpriseDependabotRoutes() {
	s.route("GET /api/v3/enterprises/{enterprise}/dependabot/alerts", s.requireEnterpriseMember(s.handleListEnterpriseDependabotAlerts))
	s.route("GET /api/v3/enterprises/{enterprise}/dependabot/repository-access", s.requireEnterpriseOwner(s.handleGetEnterpriseDependabotRepositoryAccess))
	s.route("PATCH /api/v3/enterprises/{enterprise}/dependabot/repository-access", s.requireEnterpriseOwner(s.handleUpdateEnterpriseDependabotRepositoryAccess))
	s.route("PUT /api/v3/enterprises/{enterprise}/dependabot/repository-access/default-level", s.requireEnterpriseOwner(s.handleSetEnterpriseDependabotDefaultLevel))
}

// handleListEnterpriseDependabotAlerts lists Dependabot alerts across every
// organization-owned repository on the instance. Matching real GitHub,
// alerts surface only for organizations the caller owns.
func (s *Server) handleListEnterpriseDependabotAlerts(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())

	var alerts []*DependabotAlert
	for _, org := range s.store.ListOrgsAll(0) {
		if !canAdminOrg(s.store, user, org) {
			continue
		}
		alerts = append(alerts, s.store.ListDependabotAlertsByOrg(org.ID)...)
	}

	q := r.URL.Query()
	states := splitCommaList(q.Get("state"))
	severities := splitCommaList(q.Get("severity"))
	ecosystems := splitCommaList(q.Get("ecosystem"))
	packages := splitCommaList(q.Get("package"))
	inList := func(list []string, v string) bool {
		if len(list) == 0 {
			return true
		}
		for _, x := range list {
			if x == v {
				return true
			}
		}
		return false
	}
	filtered := alerts[:0]
	for _, a := range alerts {
		if inList(states, a.State) && inList(severities, a.Severity) &&
			inList(ecosystems, a.PackageEcosystem) && inList(packages, a.PackageName) {
			filtered = append(filtered, a)
		}
	}

	sortField := q.Get("sort")
	direction := q.Get("direction")
	if direction == "" {
		direction = "desc"
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		var less bool
		switch sortField {
		case "updated":
			less = filtered[i].UpdatedAt.Before(filtered[j].UpdatedAt)
		default:
			less = filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
		}
		if direction == "asc" {
			return less
		}
		return !less
	})

	page := paginateAndLink(w, r, filtered)
	baseURL := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(page))
	for _, a := range page {
		repo := s.store.GetRepoByFullName(a.RepoKey)
		if repo == nil {
			continue
		}
		alertJSON := dependabotAlertToJSON(a, baseURL, repo)
		alertJSON["repository"] = simpleRepoJSON(repo, s.store, baseURL)
		out = append(out, alertJSON)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetEnterpriseDependabotRepositoryAccess(w http.ResponseWriter, r *http.Request) {
	s.store.mu.RLock()
	ids := append([]int(nil), s.store.EnterpriseSettings.DependabotAccessibleRepoIDs...)
	level := s.store.EnterpriseSettings.DependabotDefaultLevel
	s.store.mu.RUnlock()

	repos := s.dependabotAccessibleRepos(r, ids)
	// The endpoint paginates the repository list with page/per_page while the
	// envelope (default_level + list) stays a single object.
	pp := parsePagination(r)
	start := (pp.Page - 1) * pp.PerPage
	if start > len(repos) {
		start = len(repos)
	}
	end := start + pp.PerPage
	if end > len(repos) {
		end = len(repos)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"default_level":           nullOrString(level),
		"accessible_repositories": repos[start:end],
	})
}

func (s *Server) handleUpdateEnterpriseDependabotRepositoryAccess(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepositoryIDsToAdd    []int `json:"repository_ids_to_add"`
		RepositoryIDsToRemove []int `json:"repository_ids_to_remove"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	s.store.mu.RLock()
	existing := append([]int(nil), s.store.EnterpriseSettings.DependabotAccessibleRepoIDs...)
	s.store.mu.RUnlock()

	set := make(map[int]struct{}, len(existing))
	for _, id := range existing {
		set[id] = struct{}{}
	}
	for _, id := range req.RepositoryIDsToAdd {
		if s.store.GetRepoByID(id) == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		set[id] = struct{}{}
	}
	for _, id := range req.RepositoryIDsToRemove {
		delete(set, id)
	}

	ids := make([]int, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	s.store.SetEnterpriseDependabotRepoAccess(ids)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSetEnterpriseDependabotDefaultLevel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DefaultLevel string `json:"default_level"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.DefaultLevel != "public" && req.DefaultLevel != "internal" {
		writeGHValidationError(w, "DependabotRepositoryAccess", "default_level", "invalid")
		return
	}
	s.store.SetEnterpriseDependabotDefaultLevel(req.DefaultLevel)
	w.WriteHeader(http.StatusNoContent)
}
