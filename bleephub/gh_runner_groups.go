package bleephub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// RunnerGroup models an organization runner group. bleephub's runner
// pool is global, so groups are an organizational overlay: runners
// carry a RunnerGroupID and the group APIs manage membership and
// repository visibility; job routing stays label-based (the single
// pool serves every group).
type RunnerGroup struct {
	ID                       int       `json:"id"`
	Name                     string    `json:"name"`
	Visibility               string    `json:"visibility"` // all | selected | private
	Default                  bool      `json:"default"`
	AllowsPublicRepositories bool      `json:"allows_public_repositories"`
	SelectedRepoIDs          []int     `json:"selected_repository_ids,omitempty"`
	CreatedAt                time.Time `json:"created_at"`
}

const defaultRunnerGroupID = 1

func (s *Server) registerRunnerGroupRoutes() {
	s.route("GET /api/v3/orgs/{org}/actions/runner-groups",
		s.requirePerm(scopeAdministration, permRead, s.orgGated(s.handleListRunnerGroups)))
	s.route("POST /api/v3/orgs/{org}/actions/runner-groups",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleCreateRunnerGroup)))
	s.route("GET /api/v3/orgs/{org}/actions/runner-groups/{runner_group_id}",
		s.requirePerm(scopeAdministration, permRead, s.orgGated(s.handleGetRunnerGroup)))
	s.route("PATCH /api/v3/orgs/{org}/actions/runner-groups/{runner_group_id}",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleUpdateRunnerGroup)))
	s.route("DELETE /api/v3/orgs/{org}/actions/runner-groups/{runner_group_id}",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleDeleteRunnerGroup)))
	s.route("GET /api/v3/orgs/{org}/actions/runner-groups/{runner_group_id}/runners",
		s.requirePerm(scopeAdministration, permRead, s.orgGated(s.handleListGroupRunners)))
	s.route("PUT /api/v3/orgs/{org}/actions/runner-groups/{runner_group_id}/runners",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleSetGroupRunners)))
	s.route("PUT /api/v3/orgs/{org}/actions/runner-groups/{runner_group_id}/runners/{runner_id}",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleAddGroupRunner)))
	s.route("DELETE /api/v3/orgs/{org}/actions/runner-groups/{runner_group_id}/runners/{runner_id}",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleRemoveGroupRunner)))
	s.route("GET /api/v3/orgs/{org}/actions/runner-groups/{runner_group_id}/repositories",
		s.requirePerm(scopeAdministration, permRead, s.orgGated(s.handleListGroupRepos)))
	s.route("PUT /api/v3/orgs/{org}/actions/runner-groups/{runner_group_id}/repositories",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleSetGroupRepos)))
	s.route("PUT /api/v3/orgs/{org}/actions/runner-groups/{runner_group_id}/repositories/{repository_id}",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleAddGroupRepo)))
	s.route("DELETE /api/v3/orgs/{org}/actions/runner-groups/{runner_group_id}/repositories/{repository_id}",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleRemoveGroupRepo)))
}

// orgGated 404s requests for unknown orgs before the handler runs.
func (s *Server) orgGated(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.store.GetOrg(r.PathValue("org")) == nil {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		h(w, r)
	}
}

// ensureDefaultRunnerGroupLocked materializes the implicit Default
// group (every GitHub org has one). Callers hold the store lock.
func (s *Server) ensureDefaultRunnerGroupLocked() {
	if _, ok := s.store.RunnerGroups[defaultRunnerGroupID]; ok {
		return
	}
	s.store.RunnerGroups[defaultRunnerGroupID] = &RunnerGroup{
		ID:                       defaultRunnerGroupID,
		Name:                     "Default",
		Visibility:               "all",
		Default:                  true,
		AllowsPublicRepositories: true,
		CreatedAt:                time.Now().UTC(),
	}
}

func runnerGroupJSON(g *RunnerGroup, baseURL, org string) map[string]any {
	apiBase := fmt.Sprintf("%s/api/v3/orgs/%s/actions/runner-groups/%d", baseURL, org, g.ID)
	out := map[string]any{
		"id":                              g.ID,
		"name":                            g.Name,
		"visibility":                      g.Visibility,
		"default":                         g.Default,
		"runners_url":                     apiBase + "/runners",
		"inherited":                       false,
		"allows_public_repositories":      g.AllowsPublicRepositories,
		"restricted_to_workflows":         false,
		"selected_workflows":              []any{},
		"workflow_restrictions_read_only": false,
	}
	if g.Visibility == "selected" {
		out["selected_repositories_url"] = apiBase + "/repositories"
	}
	return out
}

func (s *Server) handleListRunnerGroups(w http.ResponseWriter, r *http.Request) {
	s.store.mu.Lock()
	s.ensureDefaultRunnerGroupLocked()
	groups := make([]*RunnerGroup, 0, len(s.store.RunnerGroups))
	for _, g := range s.store.RunnerGroups {
		groups = append(groups, g)
	}
	s.store.mu.Unlock()
	sortRunnerGroups(groups)

	page := paginateAndLink(w, r, groups)
	base := s.baseURL(r)
	org := r.PathValue("org")
	out := make([]map[string]any, 0, len(page))
	for _, g := range page {
		out = append(out, runnerGroupJSON(g, base, org))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_count":   len(groups),
		"runner_groups": out,
	})
}

func sortRunnerGroups(groups []*RunnerGroup) {
	for i := 1; i < len(groups); i++ {
		for j := i; j > 0 && groups[j-1].ID > groups[j].ID; j-- {
			groups[j-1], groups[j] = groups[j], groups[j-1]
		}
	}
}

func (s *Server) handleCreateRunnerGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name                     string `json:"name"`
		Visibility               string `json:"visibility"`
		SelectedRepositoryIDs    []int  `json:"selected_repository_ids"`
		Runners                  []int  `json:"runners"`
		AllowsPublicRepositories *bool  `json:"allows_public_repositories"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeGHValidationError(w, "RunnerGroup", "name", "missing_field")
		return
	}
	visibility := req.Visibility
	if visibility == "" {
		visibility = "all"
	}
	switch visibility {
	case "all", "selected", "private":
	default:
		writeGHValidationError(w, "RunnerGroup", "visibility", "invalid")
		return
	}

	s.store.mu.Lock()
	s.ensureDefaultRunnerGroupLocked()
	id := s.store.NextRunnerGroupID
	if id <= defaultRunnerGroupID {
		id = defaultRunnerGroupID + 1
	}
	s.store.NextRunnerGroupID = id + 1
	g := &RunnerGroup{
		ID:                       id,
		Name:                     req.Name,
		Visibility:               visibility,
		AllowsPublicRepositories: req.AllowsPublicRepositories != nil && *req.AllowsPublicRepositories,
		SelectedRepoIDs:          req.SelectedRepositoryIDs,
		CreatedAt:                time.Now().UTC(),
	}
	s.store.RunnerGroups[id] = g
	for _, runnerID := range req.Runners {
		if a := s.store.Agents[runnerID]; a != nil {
			a.RunnerGroupID = id
		}
	}
	s.persistRunnerGroupLocked(g)
	s.store.mu.Unlock()

	writeJSON(w, http.StatusCreated, runnerGroupJSON(g, s.baseURL(r), r.PathValue("org")))
}

func (s *Server) persistRunnerGroupLocked(g *RunnerGroup) {
	if s.store.persist != nil {
		s.store.persist.MustPut("runner_groups", strconv.Itoa(g.ID), g)
	}
}

// lookupRunnerGroup resolves the path's runner_group_id; nil + handled
// response when missing.
func (s *Server) lookupRunnerGroup(w http.ResponseWriter, r *http.Request) *RunnerGroup {
	id, err := strconv.Atoi(r.PathValue("runner_group_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	s.store.mu.Lock()
	s.ensureDefaultRunnerGroupLocked()
	g := s.store.RunnerGroups[id]
	s.store.mu.Unlock()
	if g == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return g
}

func (s *Server) handleGetRunnerGroup(w http.ResponseWriter, r *http.Request) {
	g := s.lookupRunnerGroup(w, r)
	if g == nil {
		return
	}
	writeJSON(w, http.StatusOK, runnerGroupJSON(g, s.baseURL(r), r.PathValue("org")))
}

func (s *Server) handleUpdateRunnerGroup(w http.ResponseWriter, r *http.Request) {
	g := s.lookupRunnerGroup(w, r)
	if g == nil {
		return
	}
	var req struct {
		Name                     *string `json:"name"`
		Visibility               *string `json:"visibility"`
		AllowsPublicRepositories *bool   `json:"allows_public_repositories"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	s.store.mu.Lock()
	if req.Name != nil && *req.Name != "" {
		g.Name = *req.Name
	}
	if req.Visibility != nil {
		switch *req.Visibility {
		case "all", "selected", "private":
			g.Visibility = *req.Visibility
		}
	}
	if req.AllowsPublicRepositories != nil {
		g.AllowsPublicRepositories = *req.AllowsPublicRepositories
	}
	s.persistRunnerGroupLocked(g)
	s.store.mu.Unlock()
	writeJSON(w, http.StatusOK, runnerGroupJSON(g, s.baseURL(r), r.PathValue("org")))
}

func (s *Server) handleDeleteRunnerGroup(w http.ResponseWriter, r *http.Request) {
	g := s.lookupRunnerGroup(w, r)
	if g == nil {
		return
	}
	if g.Default {
		// Real GitHub refuses to delete the default group.
		writeGHError(w, http.StatusBadRequest, "Cannot delete the default runner group")
		return
	}
	s.store.mu.Lock()
	delete(s.store.RunnerGroups, g.ID)
	// Members fall back to the default group, like real GitHub.
	for _, a := range s.store.Agents {
		if a.RunnerGroupID == g.ID {
			a.RunnerGroupID = defaultRunnerGroupID
		}
	}
	if s.store.persist != nil {
		s.store.persist.MustDelete("runner_groups", strconv.Itoa(g.ID))
	}
	s.store.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListGroupRunners(w http.ResponseWriter, r *http.Request) {
	g := s.lookupRunnerGroup(w, r)
	if g == nil {
		return
	}
	s.store.mu.RLock()
	members := make([]*Agent, 0)
	for _, a := range s.store.Agents {
		if agentGroupID(a) == g.ID {
			members = append(members, a)
		}
	}
	busy := s.busyAgentIDsLocked()
	s.store.mu.RUnlock()

	page := paginateAndLink(w, r, members)
	runners := make([]map[string]any, 0, len(page))
	for _, a := range page {
		runners = append(runners, runnerJSON(a, busy[a.ID]))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_count": len(members),
		"runners":     runners,
	})
}

// agentGroupID resolves an agent's group; unset means the default group.
func agentGroupID(a *Agent) int {
	if a.RunnerGroupID == 0 {
		return defaultRunnerGroupID
	}
	return a.RunnerGroupID
}

func (s *Server) handleSetGroupRunners(w http.ResponseWriter, r *http.Request) {
	g := s.lookupRunnerGroup(w, r)
	if g == nil {
		return
	}
	var req struct {
		Runners []int `json:"runners"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	want := map[int]bool{}
	for _, id := range req.Runners {
		want[id] = true
	}
	s.store.mu.Lock()
	for _, a := range s.store.Agents {
		switch {
		case want[a.ID]:
			a.RunnerGroupID = g.ID
		case agentGroupID(a) == g.ID:
			a.RunnerGroupID = defaultRunnerGroupID
		}
	}
	s.store.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddGroupRunner(w http.ResponseWriter, r *http.Request) {
	g := s.lookupRunnerGroup(w, r)
	if g == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("runner_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.mu.Lock()
	a := s.store.Agents[id]
	if a != nil {
		a.RunnerGroupID = g.ID
	}
	s.store.mu.Unlock()
	if a == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveGroupRunner(w http.ResponseWriter, r *http.Request) {
	g := s.lookupRunnerGroup(w, r)
	if g == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("runner_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.mu.Lock()
	a := s.store.Agents[id]
	if a != nil && agentGroupID(a) == g.ID {
		a.RunnerGroupID = defaultRunnerGroupID
	}
	s.store.mu.Unlock()
	if a == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListGroupRepos(w http.ResponseWriter, r *http.Request) {
	g := s.lookupRunnerGroup(w, r)
	if g == nil {
		return
	}
	base := s.baseURL(r)
	s.store.mu.RLock()
	ids := append([]int(nil), g.SelectedRepoIDs...)
	s.store.mu.RUnlock()
	repos := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		s.store.mu.RLock()
		repo := s.store.Repos[id]
		s.store.mu.RUnlock()
		if repo != nil {
			repos = append(repos, repoToJSON(repo, s.store, base))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_count":  len(repos),
		"repositories": repos,
	})
}

func (s *Server) handleSetGroupRepos(w http.ResponseWriter, r *http.Request) {
	g := s.lookupRunnerGroup(w, r)
	if g == nil {
		return
	}
	var req struct {
		SelectedRepositoryIDs []int `json:"selected_repository_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}
	s.store.mu.Lock()
	g.SelectedRepoIDs = req.SelectedRepositoryIDs
	s.persistRunnerGroupLocked(g)
	s.store.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddGroupRepo(w http.ResponseWriter, r *http.Request) {
	g := s.lookupRunnerGroup(w, r)
	if g == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("repository_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.mu.Lock()
	exists := s.store.Repos[id] != nil
	if exists {
		found := false
		for _, rid := range g.SelectedRepoIDs {
			if rid == id {
				found = true
				break
			}
		}
		if !found {
			g.SelectedRepoIDs = append(g.SelectedRepoIDs, id)
			s.persistRunnerGroupLocked(g)
		}
	}
	s.store.mu.Unlock()
	if !exists {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveGroupRepo(w http.ResponseWriter, r *http.Request) {
	g := s.lookupRunnerGroup(w, r)
	if g == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("repository_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.mu.Lock()
	kept := g.SelectedRepoIDs[:0]
	for _, rid := range g.SelectedRepoIDs {
		if rid != id {
			kept = append(kept, rid)
		}
	}
	g.SelectedRepoIDs = kept
	s.persistRunnerGroupLocked(g)
	s.store.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}
