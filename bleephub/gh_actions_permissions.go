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
	// ArtifactAndLogRetentionDays is the org-wide artifact/log retention
	// setting (GET/PUT /orgs/{org}/actions/permissions/artifact-and-log-retention).
	ArtifactAndLogRetentionDays int
	// ForkPRApprovalPolicy controls when fork PR workflows require
	// maintainer approval (actions-fork-pr-contributor-approval enum).
	ForkPRApprovalPolicy string
	// ForkPRWorkflowsPrivateRepos holds the org's fork-PR-workflow policy
	// for private repositories (four booleans).
	ForkPRWorkflowsPrivateRepos *ForkPRWorkflowsPrivateRepos
	// SelfHostedRunnersEnabledRepositories is the org policy controlling
	// which repositories may use repository-level self-hosted runners
	// (all | selected | none) with its selected repository ids.
	SelfHostedRunnersEnabledRepositories string
	SelfHostedRunnersSelectedRepoIDs     []int
	// MaxCacheRetentionDays / MaxCacheSizeGB back the
	// /organizations/{org}/actions/cache/{retention,storage}-limit
	// policy endpoints.
	MaxCacheRetentionDays int
	MaxCacheSizeGB        int
}

// ForkPRWorkflowsPrivateRepos is the actions-fork-pr-workflows-private-repos
// settings shape.
type ForkPRWorkflowsPrivateRepos struct {
	RunWorkflowsFromForkPullRequests  bool `json:"run_workflows_from_fork_pull_requests"`
	SendWriteTokensToWorkflows        bool `json:"send_write_tokens_to_workflows"`
	SendSecretsAndVariables           bool `json:"send_secrets_and_variables"`
	RequireApprovalForForkPRWorkflows bool `json:"require_approval_for_fork_pr_workflows"`
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
		EnabledRepositories:                  "all",
		AllowedActions:                       "all",
		SelectedRepositoryIDs:                []int{},
		CacheRetentionLimitDays:              90,
		CacheStorageLimitBytes:               0,
		ArtifactAndLogRetentionDays:          90,
		ForkPRApprovalPolicy:                 "first_time_contributors",
		SelfHostedRunnersEnabledRepositories: "all",
		MaxCacheRetentionDays:                90,
		MaxCacheSizeGB:                       10,
	}
}

// orgArtifactAndLogRetentionMaxDays is the maximum artifact/log
// retention GitHub allows an organization to configure.
const orgArtifactAndLogRetentionMaxDays = 400

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
		// Materialize defaults for settings whose zero value is not a
		// valid configuration (enum-shaped policies and limits).
		if p.ArtifactAndLogRetentionDays == 0 {
			p.ArtifactAndLogRetentionDays = 90
		}
		if p.ForkPRApprovalPolicy == "" {
			p.ForkPRApprovalPolicy = "first_time_contributors"
		}
		if p.SelfHostedRunnersEnabledRepositories == "" {
			p.SelfHostedRunnersEnabledRepositories = "all"
		}
		if p.MaxCacheRetentionDays == 0 {
			p.MaxCacheRetentionDays = 90
		}
		if p.MaxCacheSizeGB == 0 {
			p.MaxCacheSizeGB = 10
		}
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
	s.route("GET /api/v3/orgs/{org}/actions/permissions/artifact-and-log-retention",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleGetOrgArtifactAndLogRetention)))
	s.route("PUT /api/v3/orgs/{org}/actions/permissions/artifact-and-log-retention",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleSetOrgArtifactAndLogRetention)))
	s.route("GET /api/v3/orgs/{org}/actions/permissions/fork-pr-contributor-approval",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleGetOrgForkPRContributorApproval)))
	s.route("PUT /api/v3/orgs/{org}/actions/permissions/fork-pr-contributor-approval",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleSetOrgForkPRContributorApproval)))
	s.route("GET /api/v3/orgs/{org}/actions/permissions/fork-pr-workflows-private-repos",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleGetOrgForkPRWorkflowsPrivateRepos)))
	s.route("PUT /api/v3/orgs/{org}/actions/permissions/fork-pr-workflows-private-repos",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleSetOrgForkPRWorkflowsPrivateRepos)))
	s.route("GET /api/v3/orgs/{org}/actions/permissions/self-hosted-runners",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleGetOrgSelfHostedRunnersSettings)))
	s.route("PUT /api/v3/orgs/{org}/actions/permissions/self-hosted-runners",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleSetOrgSelfHostedRunnersSettings)))
	s.route("GET /api/v3/orgs/{org}/actions/permissions/self-hosted-runners/repositories",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleListOrgSelfHostedRunnerRepos)))
	s.route("PUT /api/v3/orgs/{org}/actions/permissions/self-hosted-runners/repositories",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleSetOrgSelfHostedRunnerRepos)))
	s.route("PUT /api/v3/orgs/{org}/actions/permissions/self-hosted-runners/repositories/{repository_id}",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleAddOrgSelfHostedRunnerRepo)))
	s.route("DELETE /api/v3/orgs/{org}/actions/permissions/self-hosted-runners/repositories/{repository_id}",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleRemoveOrgSelfHostedRunnerRepo)))
	s.route("GET /api/v3/orgs/{org}/actions/cache/usage",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleOrgCacheUsage)))
	s.route("GET /api/v3/orgs/{org}/actions/cache/usage-by-repository",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleOrgCacheUsageByRepository)))

	// Org cache policy limits at the /organizations/{org} path (the
	// dotcom REST description's path for these settings).
	s.route("GET /api/v3/organizations/{org}/actions/cache/retention-limit",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleGetOrgMaxCacheRetention)))
	s.route("PUT /api/v3/organizations/{org}/actions/cache/retention-limit",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleSetOrgMaxCacheRetention)))
	s.route("GET /api/v3/organizations/{org}/actions/cache/storage-limit",
		s.requirePerm(scopeOrgAdministration, permRead, s.orgGated(s.handleGetOrgMaxCacheSize)))
	s.route("PUT /api/v3/organizations/{org}/actions/cache/storage-limit",
		s.requirePerm(scopeOrgAdministration, permWrite, s.orgGated(s.handleSetOrgMaxCacheSize)))

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
	s.route("POST /api/v3/repos/{owner}/{repo}/actions/runners/{runner_id}/labels",
		s.requirePerm(scopeAdministration, permWrite, s.handleAddRunnerLabels))
	s.route("POST /api/v3/orgs/{org}/actions/runners/{runner_id}/labels",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleAddRunnerLabels)))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/actions/runners/{runner_id}/labels/{name}",
		s.requirePerm(scopeAdministration, permWrite, s.handleRemoveRunnerLabel))
	s.route("DELETE /api/v3/orgs/{org}/actions/runners/{runner_id}/labels/{name}",
		s.requirePerm(scopeAdministration, permWrite, s.orgGated(s.handleRemoveRunnerLabel)))
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

// --- Org permissions extras ---

func (s *Server) handleGetOrgArtifactAndLogRetention(w http.ResponseWriter, r *http.Request) {
	p := s.store.GetOrgActionsPermissions(r.PathValue("org"))
	writeJSON(w, http.StatusOK, map[string]any{
		"days":                 p.ArtifactAndLogRetentionDays,
		"maximum_allowed_days": orgArtifactAndLogRetentionMaxDays,
	})
}

func (s *Server) handleSetOrgArtifactAndLogRetention(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Days *int `json:"days"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Days == nil || *req.Days < 1 || *req.Days > orgArtifactAndLogRetentionMaxDays {
		writeGHValidationError(w, "ActionsArtifactAndLogRetention", "days", "invalid")
		return
	}
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	p.ArtifactAndLogRetentionDays = *req.Days
	s.store.SetOrgActionsPermissions(org, p)
	w.WriteHeader(http.StatusNoContent)
}

// forkPRApprovalPolicies are the actions-fork-pr-contributor-approval
// approval_policy enum values.
var forkPRApprovalPolicies = map[string]bool{
	"first_time_contributors_new_to_github": true,
	"first_time_contributors":               true,
	"all_external_contributors":             true,
}

func (s *Server) handleGetOrgForkPRContributorApproval(w http.ResponseWriter, r *http.Request) {
	p := s.store.GetOrgActionsPermissions(r.PathValue("org"))
	writeJSON(w, http.StatusOK, map[string]any{
		"approval_policy": p.ForkPRApprovalPolicy,
	})
}

func (s *Server) handleSetOrgForkPRContributorApproval(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ApprovalPolicy string `json:"approval_policy"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !forkPRApprovalPolicies[req.ApprovalPolicy] {
		writeGHValidationError(w, "ActionsForkPRContributorApproval", "approval_policy", "invalid")
		return
	}
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	p.ForkPRApprovalPolicy = req.ApprovalPolicy
	s.store.SetOrgActionsPermissions(org, p)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetOrgForkPRWorkflowsPrivateRepos(w http.ResponseWriter, r *http.Request) {
	p := s.store.GetOrgActionsPermissions(r.PathValue("org"))
	settings := p.ForkPRWorkflowsPrivateRepos
	if settings == nil {
		settings = &ForkPRWorkflowsPrivateRepos{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_workflows_from_fork_pull_requests":  settings.RunWorkflowsFromForkPullRequests,
		"send_write_tokens_to_workflows":         settings.SendWriteTokensToWorkflows,
		"send_secrets_and_variables":             settings.SendSecretsAndVariables,
		"require_approval_for_fork_pr_workflows": settings.RequireApprovalForForkPRWorkflows,
	})
}

func (s *Server) handleSetOrgForkPRWorkflowsPrivateRepos(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RunWorkflowsFromForkPullRequests  *bool `json:"run_workflows_from_fork_pull_requests"`
		SendWriteTokensToWorkflows        *bool `json:"send_write_tokens_to_workflows"`
		SendSecretsAndVariables           *bool `json:"send_secrets_and_variables"`
		RequireApprovalForForkPRWorkflows *bool `json:"require_approval_for_fork_pr_workflows"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.RunWorkflowsFromForkPullRequests == nil {
		writeGHValidationError(w, "ActionsForkPRWorkflowsPrivateRepos", "run_workflows_from_fork_pull_requests", "missing_field")
		return
	}
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	settings := p.ForkPRWorkflowsPrivateRepos
	if settings == nil {
		settings = &ForkPRWorkflowsPrivateRepos{}
		p.ForkPRWorkflowsPrivateRepos = settings
	}
	settings.RunWorkflowsFromForkPullRequests = *req.RunWorkflowsFromForkPullRequests
	if req.SendWriteTokensToWorkflows != nil {
		settings.SendWriteTokensToWorkflows = *req.SendWriteTokensToWorkflows
	}
	if req.SendSecretsAndVariables != nil {
		settings.SendSecretsAndVariables = *req.SendSecretsAndVariables
	}
	if req.RequireApprovalForForkPRWorkflows != nil {
		settings.RequireApprovalForForkPRWorkflows = *req.RequireApprovalForForkPRWorkflows
	}
	s.store.SetOrgActionsPermissions(org, p)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetOrgSelfHostedRunnersSettings(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	out := map[string]any{
		"enabled_repositories": p.SelfHostedRunnersEnabledRepositories,
	}
	if p.SelfHostedRunnersEnabledRepositories == "selected" {
		out["selected_repositories_url"] = fmt.Sprintf(
			"%s/api/v3/orgs/%s/actions/permissions/self-hosted-runners/repositories", s.baseURL(r), org)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSetOrgSelfHostedRunnersSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EnabledRepositories string `json:"enabled_repositories"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	switch req.EnabledRepositories {
	case "all", "selected", "none":
	default:
		writeGHValidationError(w, "SelfHostedRunnersSettings", "enabled_repositories", "invalid")
		return
	}
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	p.SelfHostedRunnersEnabledRepositories = req.EnabledRepositories
	s.store.SetOrgActionsPermissions(org, p)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListOrgSelfHostedRunnerRepos(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	base := s.baseURL(r)
	repos := make([]map[string]any, 0, len(p.SelfHostedRunnersSelectedRepoIDs))
	for _, id := range p.SelfHostedRunnersSelectedRepoIDs {
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

func (s *Server) handleSetOrgSelfHostedRunnerRepos(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SelectedRepositoryIDs []int `json:"selected_repository_ids"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.SelectedRepositoryIDs == nil {
		writeGHValidationError(w, "SelfHostedRunnersSettings", "selected_repository_ids", "missing_field")
		return
	}
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	p.SelfHostedRunnersSelectedRepoIDs = req.SelectedRepositoryIDs
	s.store.SetOrgActionsPermissions(org, p)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAddOrgSelfHostedRunnerRepo(w http.ResponseWriter, r *http.Request) {
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
	p := s.store.GetOrgActionsPermissions(org)
	for _, id := range p.SelfHostedRunnersSelectedRepoIDs {
		if id == repoID {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	p.SelfHostedRunnersSelectedRepoIDs = append(p.SelfHostedRunnersSelectedRepoIDs, repoID)
	s.store.SetOrgActionsPermissions(org, p)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRemoveOrgSelfHostedRunnerRepo(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	repoID, err := strconv.Atoi(r.PathValue("repository_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	p := s.store.GetOrgActionsPermissions(org)
	kept := p.SelfHostedRunnersSelectedRepoIDs[:0]
	for _, id := range p.SelfHostedRunnersSelectedRepoIDs {
		if id != repoID {
			kept = append(kept, id)
		}
	}
	p.SelfHostedRunnersSelectedRepoIDs = kept
	s.store.SetOrgActionsPermissions(org, p)
	w.WriteHeader(http.StatusNoContent)
}

// --- Org cache usage + policy limits ---

// orgCacheUsageByRepo aggregates the finalized Actions cache entries of
// every repository owned by the org: repo full name → (count, bytes),
// plus the sorted repo names for stable listing.
func (s *Server) orgCacheUsageByRepo(org string) (map[string]struct {
	Count int
	Bytes int64
}, []string) {
	usage := map[string]struct {
		Count int
		Bytes int64
	}{}
	prefix := strings.ToLower(org) + "/"
	s.artifactStore.mu.RLock()
	for _, entry := range s.artifactStore.caches {
		if !entry.Finalized || !strings.HasPrefix(strings.ToLower(entry.Repo), prefix) {
			continue
		}
		u := usage[entry.Repo]
		u.Count++
		u.Bytes += entry.Size
		usage[entry.Repo] = u
	}
	s.artifactStore.mu.RUnlock()
	names := make([]string, 0, len(usage))
	for name := range usage {
		names = append(names, name)
	}
	sort.Strings(names)
	return usage, names
}

func (s *Server) handleOrgCacheUsage(w http.ResponseWriter, r *http.Request) {
	usage, names := s.orgCacheUsageByRepo(r.PathValue("org"))
	count := 0
	var bytes int64
	for _, name := range names {
		count += usage[name].Count
		bytes += usage[name].Bytes
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_active_caches_count":         count,
		"total_active_caches_size_in_bytes": bytes,
	})
}

func (s *Server) handleOrgCacheUsageByRepository(w http.ResponseWriter, r *http.Request) {
	usage, names := s.orgCacheUsageByRepo(r.PathValue("org"))
	page := paginateAndLink(w, r, names)
	out := make([]map[string]any, 0, len(page))
	for _, name := range page {
		out = append(out, map[string]any{
			"full_name":                   name,
			"active_caches_size_in_bytes": usage[name].Bytes,
			"active_caches_count":         usage[name].Count,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_count":             len(names),
		"repository_cache_usages": out,
	})
}

func (s *Server) handleGetOrgMaxCacheRetention(w http.ResponseWriter, r *http.Request) {
	p := s.store.GetOrgActionsPermissions(r.PathValue("org"))
	writeJSON(w, http.StatusOK, map[string]any{
		"max_cache_retention_days": p.MaxCacheRetentionDays,
	})
}

func (s *Server) handleSetOrgMaxCacheRetention(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MaxCacheRetentionDays *int `json:"max_cache_retention_days"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.MaxCacheRetentionDays == nil || *req.MaxCacheRetentionDays < 1 {
		writeGHError(w, http.StatusBadRequest, "max_cache_retention_days must be a positive integer")
		return
	}
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	p.MaxCacheRetentionDays = *req.MaxCacheRetentionDays
	s.store.SetOrgActionsPermissions(org, p)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetOrgMaxCacheSize(w http.ResponseWriter, r *http.Request) {
	p := s.store.GetOrgActionsPermissions(r.PathValue("org"))
	writeJSON(w, http.StatusOK, map[string]any{
		"max_cache_size_gb": p.MaxCacheSizeGB,
	})
}

func (s *Server) handleSetOrgMaxCacheSize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MaxCacheSizeGB *int `json:"max_cache_size_gb"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.MaxCacheSizeGB == nil || *req.MaxCacheSizeGB < 1 {
		writeGHError(w, http.StatusBadRequest, "max_cache_size_gb must be a positive integer")
		return
	}
	org := r.PathValue("org")
	p := s.store.GetOrgActionsPermissions(org)
	p.MaxCacheSizeGB = *req.MaxCacheSizeGB
	s.store.SetOrgActionsPermissions(org, p)
	w.WriteHeader(http.StatusNoContent)
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
	wf := s.findWorkflowByRunIDInRepo(runID, repoFullName(r))
	if wf == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var logIDs []int
	s.store.mu.RLock()
	for _, j := range wf.Jobs {
		if job := s.store.Jobs[j.JobID]; job != nil && job.PlanID != "" {
			if recs, ok := s.store.TimelineRecords[job.PlanID]; ok {
				for _, rec := range recs {
					if rec.Log != nil {
						logIDs = append(logIDs, rec.Log.ID)
					}
				}
			}
		}
	}
	s.store.mu.RUnlock()
	for _, logID := range logIDs {
		if err := s.artifactStore.deleteLogData(r.Context(), logID); err != nil {
			writeGHError(w, http.StatusInternalServerError, "log byte-store delete: "+err.Error())
			return
		}
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

// handleAddRunnerLabels — POST .../actions/runners/{runner_id}/labels
// (repo + org scope): appends custom labels to the runner, returning
// the full label set.
func (s *Server) handleAddRunnerLabels(w http.ResponseWriter, r *http.Request) {
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
	if len(req.Labels) == 0 {
		writeGHValidationError(w, "RunnerLabel", "labels", "missing_field")
		return
	}
	s.store.mu.Lock()
	a := s.store.Agents[id]
	if a != nil {
		a.AddLabels(req.Labels)
	}
	s.store.mu.Unlock()
	if a == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, runnerLabelsJSON(a.Labels))
}

// handleRemoveRunnerLabel — DELETE .../actions/runners/{runner_id}/labels/{name}
// (repo + org scope): removes one custom label. Read-only (system)
// labels cannot be removed (422); an absent label is 404.
func (s *Server) handleRemoveRunnerLabel(w http.ResponseWriter, r *http.Request) {
	if org := r.PathValue("org"); org != "" && s.store.GetOrg(org) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(r.PathValue("runner_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	name := r.PathValue("name")
	s.store.mu.Lock()
	a := s.store.Agents[id]
	found := false
	readOnly := false
	if a != nil {
		for _, l := range a.Labels {
			if l.Name == name {
				found = true
				readOnly = l.Type == "system"
				break
			}
		}
		if found && !readOnly {
			a.RemoveLabels([]string{name})
		}
	}
	s.store.mu.Unlock()
	if a == nil || !found {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if readOnly {
		writeGHError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("Label %q is a read-only label and cannot be removed", name))
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
