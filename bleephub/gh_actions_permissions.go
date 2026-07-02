package bleephub

// GitHub Actions permissions + runner-label REST surface.
//
// Org-scoped endpoints mirror the GHES /orgs/{org}/actions/permissions paths.
// Repo-scoped endpoints mirror /repos/{owner}/{repo}/actions/permissions paths.
// Runner labels are exposed at both repo and org scope.
//
// Store types and helpers live alongside the handlers so the surface is
// self-contained; persistence is wired through store.go.

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// OrgActionsPermissions models the organization-level Actions settings.
type OrgActionsPermissions struct {
	EnabledRepositories     string          `json:"enabled_repositories"`
	SelectedRepositoriesURL string          `json:"selected_repositories_url,omitempty"`
	AllowedActions          string          `json:"allowed_actions"`
	SelectedActionsURL      string          `json:"selected_actions_url,omitempty"`
	SelectedRepositoryIDs   []int           `json:"selected_repository_ids,omitempty"`
	ActionsAllowed          *ActionsAllowed `json:"actions_allowed,omitempty"`
	WorkflowPermissions     *WorkflowPermissions
	CacheRetentionLimitDays int
	CacheStorageLimitBytes  int64
}

// RepoActionsPermissions models the repository-level Actions settings.
type RepoActionsPermissions struct {
	Enabled                     bool            `json:"enabled"`
	AllowedActions              string          `json:"allowed_actions"`
	SelectedActionsURL          string          `json:"selected_actions_url,omitempty"`
	ActionsAllowed              *ActionsAllowed `json:"actions_allowed,omitempty"`
	AccessLevel                 string          `json:"access_level"`
	WorkflowPermissions         *WorkflowPermissions
	ForkPRContributorApproval   string `json:"fork_pull_request_member_approval"`
	ForkPRWorkflowsPrivateRepos string `json:"fork_pull_request_workflows_private_repos"`
	ArtifactAndLogRetentionDays int    `json:"artifact_and_log_retention_days"`
	CacheRetentionLimitDays     int
	CacheStorageLimitBytes      int64
}

// ActionsAllowed is the "selected actions" allow-list shape.
type ActionsAllowed struct {
	GithubOwnedAllowed bool     `json:"github_owned_allowed"`
	VerifiedAllowed    bool     `json:"verified_allowed"`
	PatternsAllowed    []string `json:"patterns_allowed"`
}

// WorkflowPermissions is the default workflow-token permissions shape.
type WorkflowPermissions struct {
	DefaultWorkflowPermissions   string `json:"default_workflow_permissions"`
	CanApprovePullRequestReviews bool   `json:"can_approve_pull_request_reviews"`
}

// defaultOrgActionsPermissions returns the GitHub-default org settings.
func defaultOrgActionsPermissions() *OrgActionsPermissions {
	return &OrgActionsPermissions{
		EnabledRepositories:     "all",
		AllowedActions:          "all",
		SelectedRepositoryIDs:   []int{},
		CacheRetentionLimitDays: 90,
		CacheStorageLimitBytes:  0,
	}
}

// defaultRepoActionsPermissions returns the GitHub-default repo settings.
func defaultRepoActionsPermissions() *RepoActionsPermissions {
	return &RepoActionsPermissions{
		Enabled:                     true,
		AllowedActions:              "all",
		AccessLevel:                 "none",
		ForkPRContributorApproval:   "none",
		ForkPRWorkflowsPrivateRepos: "none",
		ArtifactAndLogRetentionDays: 90,
		CacheRetentionLimitDays:     0,
		CacheStorageLimitBytes:      0,
	}
}

func (st *Store) getOrgActionsPermissionsLocked(orgLogin string) *OrgActionsPermissions {
	if st.OrgActionsPermissions == nil {
		st.OrgActionsPermissions = map[string]*OrgActionsPermissions{}
	}
	if p, ok := st.OrgActionsPermissions[orgLogin]; ok && p != nil {
		return p
	}
	p := defaultOrgActionsPermissions()
	st.OrgActionsPermissions[orgLogin] = p
	return p
}

// GetOrgActionsPermissions returns the org's Actions settings, materializing
// defaults on first read.
func (st *Store) GetOrgActionsPermissions(orgLogin string) *OrgActionsPermissions {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.getOrgActionsPermissionsLocked(orgLogin)
}

// SetOrgActionsPermissions stores the org's Actions settings and persists.
func (st *Store) SetOrgActionsPermissions(orgLogin string, p *OrgActionsPermissions) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.OrgActionsPermissions == nil {
		st.OrgActionsPermissions = map[string]*OrgActionsPermissions{}
	}
	st.OrgActionsPermissions[orgLogin] = p
	if st.persist != nil {
		st.persist.MustPut("org_actions_permissions", orgLogin, p)
	}
}

func (st *Store) getRepoActionsPermissionsLocked(repoKey string) *RepoActionsPermissions {
	if st.RepoActionsPermissions == nil {
		st.RepoActionsPermissions = map[string]*RepoActionsPermissions{}
	}
	if p, ok := st.RepoActionsPermissions[repoKey]; ok && p != nil {
		return p
	}
	p := defaultRepoActionsPermissions()
	st.RepoActionsPermissions[repoKey] = p
	return p
}

// GetRepoActionsPermissions returns the repo's Actions settings, materializing
// defaults on first read.
func (st *Store) GetRepoActionsPermissions(repoKey string) *RepoActionsPermissions {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.getRepoActionsPermissionsLocked(repoKey)
}

// SetRepoActionsPermissions stores the repo's Actions settings and persists.
func (st *Store) SetRepoActionsPermissions(repoKey string, p *RepoActionsPermissions) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.RepoActionsPermissions == nil {
		st.RepoActionsPermissions = map[string]*RepoActionsPermissions{}
	}
	st.RepoActionsPermissions[repoKey] = p
	if st.persist != nil {
		st.persist.MustPut("repo_actions_permissions", repoKey, p)
	}
}

// persistOrgActionsPermissionsLocked writes the org permissions when the store
// lock is already held.
func (st *Store) persistOrgActionsPermissionsLocked(orgLogin string) {
	if st.persist == nil {
		return
	}
	if p := st.getOrgActionsPermissionsLocked(orgLogin); p != nil {
		st.persist.MustPut("org_actions_permissions", orgLogin, p)
	}
}

// AddOrgSelectedRepo adds a repository to the org's selected list (no-op if
// already present).
func (st *Store) AddOrgSelectedRepo(orgLogin string, repoID int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	p := st.getOrgActionsPermissionsLocked(orgLogin)
	for _, id := range p.SelectedRepositoryIDs {
		if id == repoID {
			return
		}
	}
	p.SelectedRepositoryIDs = append(p.SelectedRepositoryIDs, repoID)
	st.persistOrgActionsPermissionsLocked(orgLogin)
}

// RemoveOrgSelectedRepo drops a repository from the org's selected list.
func (st *Store) RemoveOrgSelectedRepo(orgLogin string, repoID int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	p := st.getOrgActionsPermissionsLocked(orgLogin)
	kept := p.SelectedRepositoryIDs[:0]
	for _, id := range p.SelectedRepositoryIDs {
		if id != repoID {
			kept = append(kept, id)
		}
	}
	p.SelectedRepositoryIDs = kept
	st.persistOrgActionsPermissionsLocked(orgLogin)
}

// SetOrgSelectedRepos replaces the org's selected repository list.
func (st *Store) SetOrgSelectedRepos(orgLogin string, repoIDs []int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	p := st.getOrgActionsPermissionsLocked(orgLogin)
	p.SelectedRepositoryIDs = repoIDs
	st.persistOrgActionsPermissionsLocked(orgLogin)
}

// ListOrgSelectedRepos returns the org's selected repository IDs.
func (st *Store) ListOrgSelectedRepos(orgLogin string) []int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	p := st.getOrgActionsPermissionsLocked(orgLogin)
	out := make([]int, len(p.SelectedRepositoryIDs))
	copy(out, p.SelectedRepositoryIDs)
	return out
}

// SetLabels replaces all custom labels on an agent while preserving system
// (read-only) labels. Names supplied that are system labels are treated as
// system labels.
func (a *Agent) SetLabels(names []string) {
	custom := []Label{}
	for _, l := range a.Labels {
		if l.Type == "system" {
			custom = append(custom, l)
		}
	}
	for _, name := range names {
		custom = append(custom, Label{
			ID:   a.nextLabelID(),
			Name: name,
			Type: a.labelTypeForName(name),
		})
	}
	a.Labels = custom
}

// AddLabels appends custom labels, deduplicating by name.
func (a *Agent) AddLabels(names []string) {
	have := map[string]bool{}
	for _, l := range a.Labels {
		have[l.Name] = true
	}
	for _, name := range names {
		if have[name] {
			continue
		}
		a.Labels = append(a.Labels, Label{
			ID:   a.nextLabelID(),
			Name: name,
			Type: a.labelTypeForName(name),
		})
		have[name] = true
	}
}

// RemoveLabels removes custom labels by name; system labels are never removed.
func (a *Agent) RemoveLabels(names []string) {
	drop := map[string]bool{}
	for _, n := range names {
		drop[n] = true
	}
	kept := a.Labels[:0]
	for _, l := range a.Labels {
		if l.Type == "system" || !drop[l.Name] {
			kept = append(kept, l)
		}
	}
	a.Labels = kept
}

// ClearLabels removes every custom label, leaving system labels in place.
func (a *Agent) ClearLabels() {
	kept := a.Labels[:0]
	for _, l := range a.Labels {
		if l.Type == "system" {
			kept = append(kept, l)
		}
	}
	a.Labels = kept
}

func (a *Agent) labelTypeForName(name string) string {
	for _, l := range a.Labels {
		if l.Name == name {
			return l.Type
		}
	}
	return "custom"
}

func (a *Agent) nextLabelID() int {
	max := 0
	for _, l := range a.Labels {
		if l.ID > max {
			max = l.ID
		}
	}
	return max + 1
}

func (s *Server) registerGHActionsPermissionsRoutes() {
	// Org permissions.
	s.route("GET /api/v3/orgs/{org}/actions/permissions",
		s.requirePerm(scopeAdministration, permRead, s.orgGated(s.handleGetOrgActionsPermissions)))
	s.route("PUT /api/v3/orgs/{org}/actions/permissions",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleSetOrgActionsPermissions)))
	s.route("GET /api/v3/orgs/{org}/actions/permissions/repositories",
		s.requirePerm(scopeAdministration, permRead, s.orgGated(s.handleListOrgSelectedRepos)))
	s.route("PUT /api/v3/orgs/{org}/actions/permissions/repositories",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleSetOrgSelectedRepos)))
	s.route("PUT /api/v3/orgs/{org}/actions/permissions/repositories/{repository_id}",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleAddOrgSelectedRepo)))
	s.route("DELETE /api/v3/orgs/{org}/actions/permissions/repositories/{repository_id}",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleRemoveOrgSelectedRepo)))
	s.route("GET /api/v3/orgs/{org}/actions/permissions/selected-actions",
		s.requirePerm(scopeAdministration, permRead, s.orgGated(s.handleGetOrgAllowedActions)))
	s.route("PUT /api/v3/orgs/{org}/actions/permissions/selected-actions",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleSetOrgAllowedActions)))
	s.route("GET /api/v3/orgs/{org}/actions/permissions/workflow",
		s.requirePerm(scopeAdministration, permRead, s.orgGated(s.handleGetOrgWorkflowPermissions)))
	s.route("PUT /api/v3/orgs/{org}/actions/permissions/workflow",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleSetOrgWorkflowPermissions)))
	s.route("GET /api/v3/orgs/{org}/actions/cache/retention-limit",
		s.requirePerm(scopeAdministration, permRead, s.orgGated(s.handleGetOrgCacheRetentionLimit)))
	s.route("PUT /api/v3/orgs/{org}/actions/cache/retention-limit",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleSetOrgCacheRetentionLimit)))
	s.route("GET /api/v3/orgs/{org}/actions/cache/storage-limit",
		s.requirePerm(scopeAdministration, permRead, s.orgGated(s.handleGetOrgCacheStorageLimit)))
	s.route("PUT /api/v3/orgs/{org}/actions/cache/storage-limit",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleSetOrgCacheStorageLimit)))

	// Repo permissions.
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/permissions",
		s.requirePerm(scopeActions, permRead, s.handleGetRepoActionsPermissions))
	s.route("PUT /api/v3/repos/{owner}/{repo}/actions/permissions",
		s.requirePerm(scopeActions, permWrite, s.handleSetRepoActionsPermissions))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/permissions/access",
		s.requirePerm(scopeActions, permRead, s.handleGetRepoActionsAccessLevel))
	s.route("PUT /api/v3/repos/{owner}/{repo}/actions/permissions/access",
		s.requirePerm(scopeActions, permWrite, s.handleSetRepoActionsAccessLevel))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/permissions/selected-actions",
		s.requirePerm(scopeActions, permRead, s.handleGetRepoAllowedActions))
	s.route("PUT /api/v3/repos/{owner}/{repo}/actions/permissions/selected-actions",
		s.requirePerm(scopeActions, permWrite, s.handleSetRepoAllowedActions))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/permissions/workflow",
		s.requirePerm(scopeActions, permRead, s.handleGetRepoWorkflowPermissions))
	s.route("PUT /api/v3/repos/{owner}/{repo}/actions/permissions/workflow",
		s.requirePerm(scopeActions, permWrite, s.handleSetRepoWorkflowPermissions))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/permissions/fork-pr-contributor-approval",
		s.requirePerm(scopeActions, permRead, s.handleGetRepoForkPRContributorApproval))
	s.route("PUT /api/v3/repos/{owner}/{repo}/actions/permissions/fork-pr-contributor-approval",
		s.requirePerm(scopeActions, permWrite, s.handleSetRepoForkPRContributorApproval))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/permissions/fork-pr-workflows-private-repos",
		s.requirePerm(scopeActions, permRead, s.handleGetRepoForkPRWorkflowsPrivateRepos))
	s.route("PUT /api/v3/repos/{owner}/{repo}/actions/permissions/fork-pr-workflows-private-repos",
		s.requirePerm(scopeActions, permWrite, s.handleSetRepoForkPRWorkflowsPrivateRepos))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/permissions/artifact-and-log-retention",
		s.requirePerm(scopeActions, permRead, s.handleGetRepoArtifactAndLogRetention))
	s.route("PUT /api/v3/repos/{owner}/{repo}/actions/permissions/artifact-and-log-retention",
		s.requirePerm(scopeActions, permWrite, s.handleSetRepoArtifactAndLogRetention))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/cache/retention-limit",
		s.requirePerm(scopeActions, permRead, s.handleGetRepoCacheRetentionLimit))
	s.route("PUT /api/v3/repos/{owner}/{repo}/actions/cache/retention-limit",
		s.requirePerm(scopeActions, permWrite, s.handleSetRepoCacheRetentionLimit))
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/cache/storage-limit",
		s.requirePerm(scopeActions, permRead, s.handleGetRepoCacheStorageLimit))
	s.route("PUT /api/v3/repos/{owner}/{repo}/actions/cache/storage-limit",
		s.requirePerm(scopeActions, permWrite, s.handleSetRepoCacheStorageLimit))

	// Run logs delete.
	s.route("DELETE /api/v3/repos/{owner}/{repo}/actions/runs/{run_id}/logs",
		s.requirePerm(scopeActions, permWrite, s.handleDeleteRunLogs))

	// Runner labels.
	s.route("GET /api/v3/repos/{owner}/{repo}/actions/runners/{runner_id}/labels",
		s.requirePerm(scopeAdministration, permRead, s.handleListRunnerLabels))
	s.route("PUT /api/v3/repos/{owner}/{repo}/actions/runners/{runner_id}/labels",
		s.requirePerm(scopeAdministration, permWrite, s.handleSetRunnerLabels))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/actions/runners/{runner_id}/labels",
		s.requirePerm(scopeAdministration, permWrite, s.handleRemoveAllRunnerLabels))
	s.route("GET /api/v3/orgs/{org}/actions/runners/{runner_id}/labels",
		s.requirePerm(scopeAdministration, permRead, s.orgGated(s.handleListRunnerLabels)))
	s.route("PUT /api/v3/orgs/{org}/actions/runners/{runner_id}/labels",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleSetRunnerLabels)))
	s.route("DELETE /api/v3/orgs/{org}/actions/runners/{runner_id}/labels",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleRemoveAllRunnerLabels)))
}

// --- Org permissions handlers ---

func (s *Server) handleGetOrgActionsPermissions(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	writeJSON(w, http.StatusOK, orgActionsPermissionsJSON(p, s.baseURL(r), org))
}

func (s *Server) handleSetOrgActionsPermissions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EnabledRepositories string `json:"enabled_repositories"`
		AllowedActions      string `json:"allowed_actions"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	if req.EnabledRepositories != "" {
		p.EnabledRepositories = req.EnabledRepositories
	}
	if req.AllowedActions != "" {
		p.AllowedActions = req.AllowedActions
	}
	s.store.SetOrgActionsPermissions(org, p)
	writeJSON(w, http.StatusOK, orgActionsPermissionsJSON(p, s.baseURL(r), org))
}

func (s *Server) handleListOrgSelectedRepos(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	ids := s.store.ListOrgSelectedRepos(org)
	base := s.baseURL(r)
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

func (s *Server) handleSetOrgSelectedRepos(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SelectedRepositoryIDs []int `json:"selected_repository_ids"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	org := r.PathValue("org")
	s.store.SetOrgSelectedRepos(org, req.SelectedRepositoryIDs)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddOrgSelectedRepo(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoID, err := strconv.Atoi(r.PathValue("repository_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.mu.RLock()
	exists := s.store.Repos[repoID] != nil
	s.store.mu.RUnlock()
	if !exists {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.AddOrgSelectedRepo(org, repoID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveOrgSelectedRepo(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoID, err := strconv.Atoi(r.PathValue("repository_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.RemoveOrgSelectedRepo(org, repoID)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetOrgAllowedActions(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	writeJSON(w, http.StatusOK, allowedActionsJSON(p.ActionsAllowed))
}

func (s *Server) handleSetOrgAllowedActions(w http.ResponseWriter, r *http.Request) {
	var req ActionsAllowed
	if !decodeJSONBody(w, r, &req) {
		return
	}
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	p.ActionsAllowed = &req
	p.AllowedActions = "selected"
	s.store.SetOrgActionsPermissions(org, p)
	writeJSON(w, http.StatusOK, allowedActionsJSON(p.ActionsAllowed))
}

func (s *Server) handleGetOrgWorkflowPermissions(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	writeJSON(w, http.StatusOK, workflowPermissionsJSON(p.WorkflowPermissions))
}

func (s *Server) handleSetOrgWorkflowPermissions(w http.ResponseWriter, r *http.Request) {
	var req WorkflowPermissions
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.DefaultWorkflowPermissions == "" {
		req.DefaultWorkflowPermissions = "read"
	}
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	p.WorkflowPermissions = &req
	s.store.SetOrgActionsPermissions(org, p)
	writeJSON(w, http.StatusOK, workflowPermissionsJSON(p.WorkflowPermissions))
}

func (s *Server) handleGetOrgCacheRetentionLimit(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	writeJSON(w, http.StatusOK, map[string]int{
		"retention_limit_in_days": p.CacheRetentionLimitDays,
	})
}

func (s *Server) handleSetOrgCacheRetentionLimit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RetentionLimitInDays int `json:"retention_limit_in_days"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	p.CacheRetentionLimitDays = req.RetentionLimitInDays
	s.store.SetOrgActionsPermissions(org, p)
	writeJSON(w, http.StatusOK, map[string]int{
		"retention_limit_in_days": p.CacheRetentionLimitDays,
	})
}

func (s *Server) handleGetOrgCacheStorageLimit(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	writeJSON(w, http.StatusOK, map[string]int64{
		"storage_limit_in_bytes": p.CacheStorageLimitBytes,
	})
}

func (s *Server) handleSetOrgCacheStorageLimit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StorageLimitInBytes int64 `json:"storage_limit_in_bytes"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	p.CacheStorageLimitBytes = req.StorageLimitInBytes
	s.store.SetOrgActionsPermissions(org, p)
	writeJSON(w, http.StatusOK, map[string]int64{
		"storage_limit_in_bytes": p.CacheStorageLimitBytes,
	})
}

// --- Repo permissions handlers ---

func (s *Server) handleGetRepoActionsPermissions(w http.ResponseWriter, r *http.Request) {
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	writeJSON(w, http.StatusOK, repoActionsPermissionsJSON(p, s.baseURL(r), repo))
}

func (s *Server) handleSetRepoActionsPermissions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled        bool   `json:"enabled"`
		AllowedActions string `json:"allowed_actions"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	p.Enabled = req.Enabled
	if req.AllowedActions != "" {
		p.AllowedActions = req.AllowedActions
	}
	s.store.SetRepoActionsPermissions(repo, p)
	writeJSON(w, http.StatusOK, repoActionsPermissionsJSON(p, s.baseURL(r), repo))
}

func (s *Server) handleGetRepoActionsAccessLevel(w http.ResponseWriter, r *http.Request) {
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	writeJSON(w, http.StatusOK, map[string]string{
		"access_level": p.AccessLevel,
	})
}

func (s *Server) handleSetRepoActionsAccessLevel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AccessLevel string `json:"access_level"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	p.AccessLevel = req.AccessLevel
	s.store.SetRepoActionsPermissions(repo, p)
	writeJSON(w, http.StatusOK, map[string]string{
		"access_level": p.AccessLevel,
	})
}

func (s *Server) handleGetRepoAllowedActions(w http.ResponseWriter, r *http.Request) {
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	writeJSON(w, http.StatusOK, allowedActionsJSON(p.ActionsAllowed))
}

func (s *Server) handleSetRepoAllowedActions(w http.ResponseWriter, r *http.Request) {
	var req ActionsAllowed
	if !decodeJSONBody(w, r, &req) {
		return
	}
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	p.ActionsAllowed = &req
	p.AllowedActions = "selected"
	s.store.SetRepoActionsPermissions(repo, p)
	writeJSON(w, http.StatusOK, allowedActionsJSON(p.ActionsAllowed))
}

func (s *Server) handleGetRepoWorkflowPermissions(w http.ResponseWriter, r *http.Request) {
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	writeJSON(w, http.StatusOK, workflowPermissionsJSON(p.WorkflowPermissions))
}

func (s *Server) handleSetRepoWorkflowPermissions(w http.ResponseWriter, r *http.Request) {
	var req WorkflowPermissions
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.DefaultWorkflowPermissions == "" {
		req.DefaultWorkflowPermissions = "read"
	}
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	p.WorkflowPermissions = &req
	s.store.SetRepoActionsPermissions(repo, p)
	writeJSON(w, http.StatusOK, workflowPermissionsJSON(p.WorkflowPermissions))
}

func (s *Server) handleGetRepoForkPRContributorApproval(w http.ResponseWriter, r *http.Request) {
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	writeJSON(w, http.StatusOK, map[string]string{
		"require_approval": p.ForkPRContributorApproval,
	})
}

func (s *Server) handleSetRepoForkPRContributorApproval(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RequireApproval string `json:"require_approval"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	p.ForkPRContributorApproval = req.RequireApproval
	s.store.SetRepoActionsPermissions(repo, p)
	writeJSON(w, http.StatusOK, map[string]string{
		"require_approval": p.ForkPRContributorApproval,
	})
}

func (s *Server) handleGetRepoForkPRWorkflowsPrivateRepos(w http.ResponseWriter, r *http.Request) {
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	writeJSON(w, http.StatusOK, map[string]string{
		"policy": p.ForkPRWorkflowsPrivateRepos,
	})
}

func (s *Server) handleSetRepoForkPRWorkflowsPrivateRepos(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Policy string `json:"policy"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	p.ForkPRWorkflowsPrivateRepos = req.Policy
	s.store.SetRepoActionsPermissions(repo, p)
	writeJSON(w, http.StatusOK, map[string]string{
		"policy": p.ForkPRWorkflowsPrivateRepos,
	})
}

func (s *Server) handleGetRepoArtifactAndLogRetention(w http.ResponseWriter, r *http.Request) {
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	writeJSON(w, http.StatusOK, map[string]int{
		"artifact_and_log_retention_days": p.ArtifactAndLogRetentionDays,
	})
}

func (s *Server) handleSetRepoArtifactAndLogRetention(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ArtifactAndLogRetentionDays int `json:"artifact_and_log_retention_days"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	p.ArtifactAndLogRetentionDays = req.ArtifactAndLogRetentionDays
	s.store.SetRepoActionsPermissions(repo, p)
	writeJSON(w, http.StatusOK, map[string]int{
		"artifact_and_log_retention_days": p.ArtifactAndLogRetentionDays,
	})
}

func (s *Server) handleGetRepoCacheRetentionLimit(w http.ResponseWriter, r *http.Request) {
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	writeJSON(w, http.StatusOK, map[string]int{
		"retention_limit_in_days": p.CacheRetentionLimitDays,
	})
}

func (s *Server) handleSetRepoCacheRetentionLimit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RetentionLimitInDays int `json:"retention_limit_in_days"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	p.CacheRetentionLimitDays = req.RetentionLimitInDays
	s.store.SetRepoActionsPermissions(repo, p)
	writeJSON(w, http.StatusOK, map[string]int{
		"retention_limit_in_days": p.CacheRetentionLimitDays,
	})
}

func (s *Server) handleGetRepoCacheStorageLimit(w http.ResponseWriter, r *http.Request) {
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	writeJSON(w, http.StatusOK, map[string]int64{
		"storage_limit_in_bytes": p.CacheStorageLimitBytes,
	})
}

func (s *Server) handleSetRepoCacheStorageLimit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StorageLimitInBytes int64 `json:"storage_limit_in_bytes"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	repo := repoFullName(r)
	p := s.store.GetRepoActionsPermissions(repo)
	p.CacheStorageLimitBytes = req.StorageLimitInBytes
	s.store.SetRepoActionsPermissions(repo, p)
	writeJSON(w, http.StatusOK, map[string]int64{
		"storage_limit_in_bytes": p.CacheStorageLimitBytes,
	})
}

// --- Run logs ---

func (s *Server) handleDeleteRunLogs(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.Atoi(r.PathValue("run_id"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "invalid run_id")
		return
	}
	wf := s.findWorkflowByRunID(runID)
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.mu.Lock()
	for _, j := range wf.Jobs {
		delete(s.store.LogLines, j.JobID)
		if job := s.store.Jobs[j.JobID]; job != nil && job.PlanID != "" {
			if recs, ok := s.store.TimelineRecords[job.PlanID]; ok {
				for _, rec := range recs {
					if rec.Log != nil {
						delete(s.store.LogFiles, rec.Log.ID)
					}
				}
				delete(s.store.TimelineRecords, job.PlanID)
			}
		}
	}
	s.store.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// --- Runner labels ---

func (s *Server) handleListRunnerLabels(w http.ResponseWriter, r *http.Request) {
	if org := r.PathValue("org"); org != "" && s.store.GetOrg(org) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(r.PathValue("runner_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.mu.RLock()
	a := s.store.Agents[id]
	s.store.mu.RUnlock()
	if a == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, runnerLabelsJSON(a.Labels))
}

func (s *Server) handleSetRunnerLabels(w http.ResponseWriter, r *http.Request) {
	if org := r.PathValue("org"); org != "" && s.store.GetOrg(org) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(r.PathValue("runner_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Labels []string `json:"labels"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	s.store.mu.Lock()
	a := s.store.Agents[id]
	if a != nil {
		a.SetLabels(req.Labels)
	}
	s.store.mu.Unlock()
	if a == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, runnerLabelsJSON(a.Labels))
}

func (s *Server) handleRemoveAllRunnerLabels(w http.ResponseWriter, r *http.Request) {
	if org := r.PathValue("org"); org != "" && s.store.GetOrg(org) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
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
		a.ClearLabels()
	}
	s.store.mu.Unlock()
	if a == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, runnerLabelsJSON(a.Labels))
}

// --- JSON helpers ---

func orgActionsPermissionsJSON(p *OrgActionsPermissions, baseURL, org string) map[string]any {
	apiBase := fmt.Sprintf("%s/api/v3/orgs/%s/actions/permissions", baseURL, org)
	out := map[string]any{
		"enabled_repositories": p.EnabledRepositories,
		"allowed_actions":      p.AllowedActions,
	}
	if p.EnabledRepositories == "selected" {
		out["selected_repositories_url"] = apiBase + "/repositories"
	}
	if p.AllowedActions == "selected" {
		out["selected_actions_url"] = apiBase + "/selected-actions"
	}
	return out
}

func repoActionsPermissionsJSON(p *RepoActionsPermissions, baseURL, repo string) map[string]any {
	owner, name, _ := strings.Cut(repo, "/")
	apiBase := fmt.Sprintf("%s/api/v3/repos/%s/%s/actions/permissions", baseURL, owner, name)
	out := map[string]any{
		"enabled":         p.Enabled,
		"allowed_actions": p.AllowedActions,
	}
	if p.AllowedActions == "selected" {
		out["selected_actions_url"] = apiBase + "/selected-actions"
	}
	return out
}

func allowedActionsJSON(a *ActionsAllowed) map[string]any {
	if a == nil {
		return map[string]any{
			"github_owned_allowed": true,
			"verified_allowed":     false,
			"patterns_allowed":     []string{},
		}
	}
	patterns := a.PatternsAllowed
	if patterns == nil {
		patterns = []string{}
	}
	return map[string]any{
		"github_owned_allowed": a.GithubOwnedAllowed,
		"verified_allowed":     a.VerifiedAllowed,
		"patterns_allowed":     patterns,
	}
}

func workflowPermissionsJSON(w *WorkflowPermissions) map[string]any {
	if w == nil {
		return map[string]any{
			"default_workflow_permissions":     "read",
			"can_approve_pull_request_reviews": false,
		}
	}
	return map[string]any{
		"default_workflow_permissions":     w.DefaultWorkflowPermissions,
		"can_approve_pull_request_reviews": w.CanApprovePullRequestReviews,
	}
}

func runnerLabelsJSON(labels []Label) map[string]any {
	out := make([]map[string]any, 0, len(labels))
	for _, l := range labels {
		labelType := "custom"
		if l.Type == "system" {
			labelType = "read-only"
		}
		out = append(out, map[string]any{
			"id":   l.ID,
			"name": l.Name,
			"type": labelType,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		idI, _ := out[i]["id"].(int)
		idJ, _ := out[j]["id"].(int)
		return idI < idJ
	})
	return map[string]any{
		"total_count": len(out),
		"labels":      out,
	}
}
