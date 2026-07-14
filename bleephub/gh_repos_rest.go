package bleephub

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
)

func (s *Server) registerGHRepoRoutes() {
	s.route("POST /api/v3/user/repos", s.requirePerm(scopeContents, permWrite, s.handleCreateRepo))
	s.route("GET /api/v3/user/repos", s.handleListAuthUserRepos)
	s.route("GET /api/v3/repos/{owner}/{repo}", s.handleGetRepo)
	s.route("PATCH /api/v3/repos/{owner}/{repo}", s.requirePerm(scopeAdministration, permWrite, s.handleUpdateRepo))
	s.route("DELETE /api/v3/repos/{owner}/{repo}", s.requirePerm(scopeAdministration, permWrite, s.handleDeleteRepo))
	s.route("GET /api/v3/users/{username}/repos", s.handleListUserRepos)
	s.route("GET /api/v3/orgs/{org}/repos", s.handleListOrgRepos)
	s.route("GET /api/v3/repos/{owner}/{repo}/topics", s.handleGetRepoTopics)
	s.route("PUT /api/v3/repos/{owner}/{repo}/topics", s.requirePerm(scopeContents, permWrite, s.handlePutRepoTopics))
	s.route("GET /api/v3/repos/{owner}/{repo}/languages", s.handleGetRepoLanguages)
	s.route("GET /api/v3/repos/{owner}/{repo}/compare/{range...}", s.handleCompareRefs)
	s.route("POST /api/v3/repos/{owner}/{repo}/merges", s.requirePerm(scopeContents, permWrite, s.handleMergeRefs))
	s.route("POST /api/v3/repos/{owner}/{repo}/forks", s.handleCreateFork)
	s.route("GET /api/v3/repos/{owner}/{repo}/forks", s.handleListForks)
	s.route("GET /api/v3/repos/{owner}/{repo}/stargazers", s.handleListStargazers)
	s.route("PUT /api/v3/user/starred/{owner}/{repo}", s.handleStarRepo)
	s.route("DELETE /api/v3/user/starred/{owner}/{repo}", s.handleUnstarRepo)
	s.route("GET /api/v3/user/starred", s.handleListStarredRepos)
	s.route("GET /api/v3/users/{username}/starred", s.handleListUserStarredRepos)
	s.route("GET /api/v3/repos/{owner}/{repo}/collaborators", s.requirePerm(scopeContents, permRead, s.handleListCollaborators))
	s.route("GET /api/v3/repos/{owner}/{repo}/collaborators/{username}/permission", s.requirePerm(scopeContents, permRead, s.handleGetCollaboratorPermission))
	s.route("PUT /api/v3/repos/{owner}/{repo}/collaborators/{username}", s.requirePerm(scopeAdministration, permWrite, s.handleAddCollaborator))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/collaborators/{username}", s.requirePerm(scopeAdministration, permWrite, s.handleRemoveCollaborator))
	s.registerGHRepoRefRoutes()
	s.registerGHRepoObjectRoutes()
	s.registerGHGitDataRoutes()
	s.registerGHRepoSettingsRoutes()
}

func (s *Server) registerGHRepoSettingsRoutes() {
	s.route("GET /api/v3/repos/{owner}/{repo}/keys", s.requirePerm(scopeAdministration, permRead, s.handleListRepoDeployKeys))
	s.route("POST /api/v3/repos/{owner}/{repo}/keys", s.requirePerm(scopeAdministration, permWrite, s.handleCreateRepoDeployKey))
	s.route("GET /api/v3/repos/{owner}/{repo}/keys/{key_id}", s.requirePerm(scopeAdministration, permRead, s.handleGetRepoDeployKey))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/keys/{key_id}", s.requirePerm(scopeAdministration, permWrite, s.handleDeleteRepoDeployKey))
	s.route("POST /api/v3/repos/{owner}/{repo}/transfer", s.requirePerm(scopeAdministration, permWrite, s.handleTransferRepo))
	s.route("POST /api/v3/repos/{owner}/{repo}/merge-upstream", s.requirePerm(scopeContents, permWrite, s.handleMergeUpstream))
	s.route("POST /api/v3/repos/{owner}/{repo}/branches/{branch}/rename", s.requirePerm(scopeAdministration, permWrite, s.handleRenameBranch))
	s.route("PUT /api/v3/repos/{owner}/{repo}/subscription", s.requirePerm(scopeContents, permRead, s.handleSetRepoSubscription))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/subscription", s.requirePerm(scopeContents, permRead, s.handleDeleteRepoSubscription))
	s.route("GET /api/v3/repos/{owner}/{repo}/automated-security-fixes", s.requirePerm(scopeAdministration, permRead, s.handleCheckAutomatedSecurityFixes))
	s.route("GET /api/v3/repos/{owner}/{repo}/private-vulnerability-reporting", s.handleCheckPrivateVulnerabilityReporting)
	s.route("GET /api/v3/repos/{owner}/{repo}/vulnerability-alerts", s.requirePerm(scopeAdministration, permRead, s.handleCheckVulnerabilityAlerts))
	s.route("GET /api/v3/repos/{owner}/{repo}/interaction-limits", s.requirePerm(scopeAdministration, permRead, s.handleGetInteractionLimits))
	s.route("PUT /api/v3/repos/{owner}/{repo}/automated-security-fixes", s.requirePerm(scopeAdministration, permWrite, s.handleEnableAutomatedSecurityFixes))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/automated-security-fixes", s.requirePerm(scopeAdministration, permWrite, s.handleDisableAutomatedSecurityFixes))
	s.route("PUT /api/v3/repos/{owner}/{repo}/private-vulnerability-reporting", s.requirePerm(scopeAdministration, permWrite, s.handleEnablePrivateVulnerabilityReporting))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/private-vulnerability-reporting", s.requirePerm(scopeAdministration, permWrite, s.handleDisablePrivateVulnerabilityReporting))
	s.route("PUT /api/v3/repos/{owner}/{repo}/vulnerability-alerts", s.requirePerm(scopeAdministration, permWrite, s.handleEnableVulnerabilityAlerts))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/vulnerability-alerts", s.requirePerm(scopeAdministration, permWrite, s.handleDisableVulnerabilityAlerts))
	s.route("PUT /api/v3/repos/{owner}/{repo}/interaction-limits", s.requirePerm(scopeAdministration, permWrite, s.handleSetInteractionLimits))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/interaction-limits", s.requirePerm(scopeAdministration, permWrite, s.handleDeleteInteractionLimits))
}

func (s *Server) repoFromRequest(w http.ResponseWriter, r *http.Request) *Repo {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return repo
}

func (s *Server) handleListRepoDeployKeys(w http.ResponseWriter, r *http.Request) {
	repo := s.repoFromRequest(w, r)
	if repo == nil {
		return
	}
	keys := s.store.ListRepoDeployKeys(repo.ID)
	out := make([]map[string]interface{}, 0, len(keys))
	base := s.baseURL(r)
	for _, k := range keys {
		out = append(out, deployKeyToJSON(k, repo.FullName, base))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateRepoDeployKey(w http.ResponseWriter, r *http.Request) {
	repo := s.repoFromRequest(w, r)
	if repo == nil {
		return
	}
	var req struct {
		Title    string `json:"title"`
		Key      string `json:"key"`
		ReadOnly bool   `json:"read_only"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Title == "" || req.Key == "" {
		writeGHValidationError(w, "DeployKey", "key", "missing_field")
		return
	}
	key := s.store.CreateRepoDeployKey(repo.ID, req.Title, req.Key, req.ReadOnly)
	writeJSON(w, http.StatusCreated, deployKeyToJSON(key, repo.FullName, s.baseURL(r)))
}

func (s *Server) handleGetRepoDeployKey(w http.ResponseWriter, r *http.Request) {
	repo := s.repoFromRequest(w, r)
	if repo == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("key_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	key := s.store.GetRepoDeployKey(id)
	if key == nil || key.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, deployKeyToJSON(key, repo.FullName, s.baseURL(r)))
}

func (s *Server) handleDeleteRepoDeployKey(w http.ResponseWriter, r *http.Request) {
	repo := s.repoFromRequest(w, r)
	if repo == nil {
		return
	}
	id, err := strconv.Atoi(r.PathValue("key_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	key := s.store.GetRepoDeployKey(id)
	if key == nil || key.RepoID != repo.ID {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.DeleteRepoDeployKey(id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func deployKeyToJSON(k *RepoDeployKey, fullName, base string) map[string]interface{} {
	return map[string]interface{}{
		"id":         k.ID,
		"key":        k.Key,
		"url":        base + "/api/v3/repos/" + fullName + "/keys/" + strconv.Itoa(k.ID),
		"title":      k.Title,
		"verified":   k.Verified,
		"created_at": k.CreatedAt.Format(time.RFC3339),
		"read_only":  k.ReadOnly,
	}
}

func (s *Server) handleTransferRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	repo := s.repoFromRequest(w, r)
	if repo == nil {
		return
	}
	if !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "Must have admin rights to Repository.")
		return
	}
	var req struct {
		NewOwner string `json:"new_owner"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.NewOwner == "" {
		writeGHValidationError(w, "Transfer", "new_owner", "missing_field")
		return
	}
	owner, _, _ := splitRepoFullName(repo.FullName)
	if !s.store.TransferRepo(owner, repo.Name, req.NewOwner) {
		writeGHError(w, http.StatusUnprocessableEntity, "Repository transfer failed.")
		return
	}
	updated := s.store.GetRepo(req.NewOwner, repo.Name)
	s.recordAuditEvent("repo.transfer", user.Login, "", map[string]interface{}{"repo": updated.FullName, "repo_id": updated.ID})
	writeJSON(w, http.StatusAccepted, minimalRepoJSON(updated, s.store, s.baseURL(r)))
}

func (s *Server) handleMergeUpstream(w http.ResponseWriter, r *http.Request) {
	repo := s.repoFromRequest(w, r)
	if repo == nil {
		return
	}
	var req struct {
		Branch string `json:"branch"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	branch := req.Branch
	if branch == "" {
		branch = repo.DefaultBranch
	}
	if !repo.Fork {
		writeGHError(w, http.StatusUnprocessableEntity, "Repository is not a fork.")
		return
	}

	sourceID := repo.SourceID
	if sourceID == 0 {
		sourceID = repo.ParentID
	}
	source := s.store.GetRepoByID(sourceID)
	if source == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	owner, name, _ := splitRepoFullName(repo.FullName)
	targetStor := s.store.GetGitStorage(owner, name)
	srcOwner, srcName, _ := splitRepoFullName(source.FullName)
	srcStor := s.store.GetGitStorage(srcOwner, srcName)
	if targetStor == nil || srcStor == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Branch not found")
		return
	}

	srcRef, err := srcStor.Reference(plumbing.NewBranchReferenceName(branch))
	if err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Branch not found")
		return
	}
	if _, err := targetStor.Reference(plumbing.NewBranchReferenceName(branch)); err != nil {
		if err := targetStor.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), srcRef.Hash())); err != nil {
			writeGHError(w, http.StatusUnprocessableEntity, "Merge failed")
			return
		}
	} else {
		if err := targetStor.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), srcRef.Hash())); err != nil {
			writeGHError(w, http.StatusUnprocessableEntity, "Merge failed")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":     fmt.Sprintf("Successfully merged upstream branch %s into %s", branch, branch),
		"merge_type":  "fast-forward",
		"base_branch": branch,
	})
}

func (s *Server) handleRenameBranch(w http.ResponseWriter, r *http.Request) {
	repo := s.repoFromRequest(w, r)
	if repo == nil {
		return
	}
	branch := r.PathValue("branch")
	var req struct {
		NewName string `json:"new_name"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.NewName == "" {
		writeGHValidationError(w, "Branch", "new_name", "missing_field")
		return
	}
	if !s.store.RenameBranch(repo.ID, branch, req.NewName) {
		writeGHError(w, http.StatusUnprocessableEntity, "Branch rename failed.")
		return
	}

	owner, name, _ := splitRepoFullName(repo.FullName)
	stor := s.store.GetGitStorage(owner, name)
	base := s.baseURL(r)
	api := base + "/api/v3/repos/" + repo.FullName
	branchURL := api + "/branches/" + req.NewName
	result := map[string]interface{}{
		"name":           req.NewName,
		"protected":      false,
		"protection":     map[string]interface{}{},
		"protection_url": branchURL + "/protection",
		"_links": map[string]interface{}{
			"self": branchURL,
			"html": base + "/" + repo.FullName + "/tree/" + req.NewName,
		},
	}
	if stor != nil {
		if ref, err := stor.Reference(plumbing.NewBranchReferenceName(req.NewName)); err == nil {
			if commit := resolveCommit(stor, ref.Hash()); commit != nil {
				result["commit"] = commitToJSON(commit, repo, base)
			} else {
				result["commit"] = map[string]interface{}{"sha": ref.Hash().String()}
			}
		}
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleSetRepoSubscription(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	repo := s.repoFromRequest(w, r)
	if repo == nil {
		return
	}
	if repo.Private && !canReadRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Subscribed bool `json:"subscribed"`
		Ignored    bool `json:"ignored"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !s.store.SetRepoSubscription(user.ID, repo.ID, req.Subscribed) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, subscriptionToJSON(s.store.GetRepoSubscription(user.ID, repo.ID), repo, s.baseURL(r)))
}

func (s *Server) handleDeleteRepoSubscription(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	repo := s.repoFromRequest(w, r)
	if repo == nil {
		return
	}
	if !s.store.DeleteRepoSubscription(user.ID, repo.ID) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func subscriptionToJSON(sub *RepoSubscription, repo *Repo, base string) map[string]interface{} {
	createdAt := repo.CreatedAt
	subscribed := false
	ignored := false
	if sub != nil {
		createdAt = sub.CreatedAt
		subscribed = sub.Subscribed
		ignored = sub.Ignored
	}
	return map[string]interface{}{
		"subscribed":     subscribed,
		"ignored":        ignored,
		"reason":         nil,
		"created_at":     createdAt.Format(time.RFC3339),
		"url":            base + "/api/v3/repos/" + repo.FullName + "/subscription",
		"repository_url": base + "/api/v3/repos/" + repo.FullName,
	}
}

func (s *Server) handleEnableAutomatedSecurityFixes(w http.ResponseWriter, r *http.Request) {
	s.setRepoFlag(w, r, "automated_security_fixes_enabled", true)
}

func (s *Server) handleDisableAutomatedSecurityFixes(w http.ResponseWriter, r *http.Request) {
	s.setRepoFlag(w, r, "automated_security_fixes_enabled", false)
}

func (s *Server) handleEnablePrivateVulnerabilityReporting(w http.ResponseWriter, r *http.Request) {
	s.setRepoFlag(w, r, "private_vulnerability_reporting_enabled", true)
}

func (s *Server) handleDisablePrivateVulnerabilityReporting(w http.ResponseWriter, r *http.Request) {
	s.setRepoFlag(w, r, "private_vulnerability_reporting_enabled", false)
}

func (s *Server) handleEnableVulnerabilityAlerts(w http.ResponseWriter, r *http.Request) {
	s.setRepoFlag(w, r, "vulnerability_alerts_enabled", true)
}

func (s *Server) handleDisableVulnerabilityAlerts(w http.ResponseWriter, r *http.Request) {
	s.setRepoFlag(w, r, "vulnerability_alerts_enabled", false)
}

func (s *Server) setRepoFlag(w http.ResponseWriter, r *http.Request, field string, value bool) {
	repo := s.repoFromRequest(w, r)
	if repo == nil {
		return
	}
	if !s.store.SetRepoFlag(repo.ID, field, value) {
		writeGHError(w, http.StatusUnprocessableEntity, "Setting update failed.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCheckAutomatedSecurityFixes — GET /repos/{o}/{r}/automated-security-fixes.
func (s *Server) handleCheckAutomatedSecurityFixes(w http.ResponseWriter, r *http.Request) {
	repo := s.repoFromRequest(w, r)
	if repo == nil {
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enabled": repo.AutomatedSecurityFixesEnabled,
		"paused":  false,
	})
}

// handleCheckPrivateVulnerabilityReporting — GET /repos/{o}/{r}/private-vulnerability-reporting.
func (s *Server) handleCheckPrivateVulnerabilityReporting(w http.ResponseWriter, r *http.Request) {
	repo := s.repoFromRequest(w, r)
	if repo == nil {
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enabled": repo.PrivateVulnerabilityReportingEnabled,
	})
}

// handleCheckVulnerabilityAlerts — GET /repos/{o}/{r}/vulnerability-alerts.
// The check is a status code, not a body: 204 when Dependabot alerts are
// enabled, 404 when disabled.
func (s *Server) handleCheckVulnerabilityAlerts(w http.ResponseWriter, r *http.Request) {
	repo := s.repoFromRequest(w, r)
	if repo == nil {
		return
	}
	if !repo.VulnerabilityAlertsEnabled {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetInteractionLimits — GET /repos/{o}/{r}/interaction-limits.
// Returns the active restriction, or an empty object when none is in
// effect (an expired restriction is no longer in effect).
func (s *Server) handleGetInteractionLimits(w http.ResponseWriter, r *http.Request) {
	repo := s.repoFromRequest(w, r)
	if repo == nil {
		return
	}
	if repo.InteractionLimit == "" || repo.InteractionLimitExpiry == nil || time.Now().After(*repo.InteractionLimitExpiry) {
		writeJSON(w, http.StatusOK, map[string]interface{}{})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"limit":      repo.InteractionLimit,
		"origin":     "repository",
		"expires_at": repo.InteractionLimitExpiry.UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleSetInteractionLimits(w http.ResponseWriter, r *http.Request) {
	repo := s.repoFromRequest(w, r)
	if repo == nil {
		return
	}
	var req struct {
		Limit  string `json:"limit"`
		Expiry string `json:"expiry"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Limit == "" {
		writeGHValidationError(w, "InteractionLimit", "limit", "missing_field")
		return
	}
	if !isInteractionGroup(req.Limit) {
		writeGHValidationError(w, "InteractionLimit", "limit", "invalid")
		return
	}
	expiresAt, ok := interactionLimitExpiry(req.Expiry, time.Now().UTC())
	if !ok {
		writeGHValidationError(w, "InteractionLimit", "expiry", "invalid")
		return
	}
	if !s.store.SetRepoInteractionLimit(repo.ID, req.Limit, &expiresAt) {
		writeGHError(w, http.StatusUnprocessableEntity, "Interaction limit update failed.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"limit":      req.Limit,
		"origin":     "repository",
		"expires_at": expiresAt.Format(time.RFC3339),
	})
}

func (s *Server) handleDeleteInteractionLimits(w http.ResponseWriter, r *http.Request) {
	repo := s.repoFromRequest(w, r)
	if repo == nil {
		return
	}
	if !s.store.SetRepoInteractionLimit(repo.ID, "", nil) {
		writeGHError(w, http.StatusUnprocessableEntity, "Interaction limit update failed.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCreateRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	var req struct {
		Name                      string   `json:"name"`
		Description               string   `json:"description"`
		Homepage                  string   `json:"homepage"`
		Private                   flexBool `json:"private"`
		Visibility                string   `json:"visibility"`
		DefaultBranch             string   `json:"default_branch"`
		AutoInit                  flexBool `json:"auto_init"`
		GitignoreTemplate         string   `json:"gitignore_template"`
		LicenseTemplate           string   `json:"license_template"`
		HasIssues                 *bool    `json:"has_issues"`
		HasProjects               *bool    `json:"has_projects"`
		HasWiki                   *bool    `json:"has_wiki"`
		HasDiscussions            *bool    `json:"has_discussions"`
		HasPullRequests           *bool    `json:"has_pull_requests"`
		AllowSquashMerge          *bool    `json:"allow_squash_merge"`
		AllowMergeCommit          *bool    `json:"allow_merge_commit"`
		AllowRebaseMerge          *bool    `json:"allow_rebase_merge"`
		AllowAutoMerge            *bool    `json:"allow_auto_merge"`
		DeleteBranchOnMerge       *bool    `json:"delete_branch_on_merge"`
		UseSquashPRTitleAsDefault *bool    `json:"use_squash_pr_title_as_default"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeGHValidationError(w, "Repository", "name", "missing_field")
		return
	}

	private := bool(req.Private)
	if req.Visibility != "" {
		switch req.Visibility {
		case "public":
			private = false
		case "private", "internal":
			private = true
		default:
			writeGHValidationError(w, "Repository", "visibility", "invalid")
			return
		}
	}

	defaultBranch := req.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	repo := s.store.CreateRepo(user, req.Name, req.Description, private)
	if repo == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Repository creation failed.")
		return
	}

	s.store.UpdateRepo(user.Login, req.Name, func(r *Repo) {
		r.Homepage = req.Homepage
		if req.HasIssues != nil {
			r.HasIssues = *req.HasIssues
		}
		if req.HasProjects != nil {
			r.HasProjects = *req.HasProjects
		}
		if req.HasWiki != nil {
			r.HasWiki = *req.HasWiki
		}
		if req.HasDiscussions != nil {
			r.HasDiscussions = boolPointer(*req.HasDiscussions)
		}
		if req.HasPullRequests != nil {
			r.HasPullRequests = *req.HasPullRequests
		}
		if req.AllowSquashMerge != nil {
			r.AllowSquashMerge = *req.AllowSquashMerge
		}
		if req.AllowMergeCommit != nil {
			r.AllowMergeCommit = *req.AllowMergeCommit
		}
		if req.AllowRebaseMerge != nil {
			r.AllowRebaseMerge = *req.AllowRebaseMerge
		}
		if req.AllowAutoMerge != nil {
			r.AllowAutoMerge = *req.AllowAutoMerge
		}
		if req.DeleteBranchOnMerge != nil {
			r.DeleteBranchOnMerge = *req.DeleteBranchOnMerge
		}
		if req.UseSquashPRTitleAsDefault != nil {
			r.UseSquashPRTitleAsDefault = *req.UseSquashPRTitleAsDefault
		}
	})

	if defaultBranch != "main" {
		s.store.UpdateRepo(user.Login, req.Name, func(r *Repo) {
			r.DefaultBranch = defaultBranch
		})
	}

	if bool(req.AutoInit) || req.GitignoreTemplate != "" || req.LicenseTemplate != "" {
		if err := s.initRepoFiles(r.Context(), repo, defaultBranch, req.Description, req.GitignoreTemplate, req.LicenseTemplate, bool(req.AutoInit)); err != nil {
			if _, deleteErr := s.store.DeleteRepo(user.Login, req.Name); deleteErr != nil {
				writeGHError(w, http.StatusInternalServerError, "repository rollback failed: "+deleteErr.Error())
				return
			}
			writeGHError(w, http.StatusUnprocessableEntity, "Repository creation failed.")
			return
		}
	}

	repo = s.store.GetRepo(user.Login, req.Name)
	s.recordAuditEvent("repo.create", user.Login, "", map[string]interface{}{"repo": repo.FullName, "repo_id": repo.ID})
	writeJSON(w, http.StatusCreated, fullRepoJSONForViewer(repo, s.store, s.baseURL(r), user))
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
	writeJSON(w, http.StatusOK, fullRepoJSONForViewer(repo, s.store, s.baseURL(r), user))
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
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if newName, ok := req["name"].(string); ok && newName != "" && newName != repo.Name {
		if !s.store.RenameRepo(owner, name, newName) {
			writeGHError(w, http.StatusUnprocessableEntity, "Repository rename failed.")
			return
		}
		// Update artifacts that embed the repo full name.
		oldFull := owner + "/" + name
		newFull := owner + "/" + newName
		s.artifactStore.mu.Lock()
		for _, art := range s.artifactStore.artifacts {
			if art.RepoFullName == oldFull {
				art.RepoFullName = newFull
			}
		}
		s.artifactStore.mu.Unlock()

		name = newName
	}

	s.store.UpdateRepo(owner, name, func(r *Repo) {
		if v, ok := req["description"].(string); ok {
			r.Description = v
		}
		if v, ok := req["homepage"].(string); ok {
			r.Homepage = v
		}
		if v, ok := req["default_branch"].(string); ok {
			r.DefaultBranch = v
		}
		if v, ok := coerceBool(req["private"]); ok {
			r.Private = v
			if v {
				r.Visibility = "private"
			} else {
				r.Visibility = "public"
			}
		}
		if v, ok := coerceBool(req["has_issues"]); ok {
			r.HasIssues = v
		}
		if v, ok := coerceBool(req["has_projects"]); ok {
			r.HasProjects = v
		}
		if v, ok := coerceBool(req["has_wiki"]); ok {
			r.HasWiki = v
		}
		if v, ok := coerceBool(req["has_discussions"]); ok {
			r.HasDiscussions = boolPointer(v)
		}
		if v, ok := coerceBool(req["has_pull_requests"]); ok {
			r.HasPullRequests = v
		}
		if v, ok := coerceBool(req["archived"]); ok {
			switch {
			case v && (!r.Archived || r.ArchivedAt == nil):
				now := time.Now().UTC()
				r.ArchivedAt = &now
			case !v:
				r.ArchivedAt = nil
			}
			r.Archived = v
		}
		if v, ok := coerceBool(req["is_template"]); ok {
			r.IsTemplate = v
		}
		if v, ok := coerceBool(req["web_commit_signoff_required"]); ok {
			r.WebCommitSignoffRequired = v
		}
		if v, ok := coerceBool(req["allow_squash_merge"]); ok {
			r.AllowSquashMerge = v
		}
		if v, ok := coerceBool(req["allow_merge_commit"]); ok {
			r.AllowMergeCommit = v
		}
		if v, ok := coerceBool(req["allow_rebase_merge"]); ok {
			r.AllowRebaseMerge = v
		}
		if v, ok := coerceBool(req["allow_auto_merge"]); ok {
			r.AllowAutoMerge = v
		}
		if v, ok := coerceBool(req["allow_update_branch"]); ok {
			r.AllowUpdateBranch = v
		}
		if v, ok := coerceBool(req["delete_branch_on_merge"]); ok {
			r.DeleteBranchOnMerge = v
		}
		if v, ok := coerceBool(req["use_squash_pr_title_as_default"]); ok {
			r.UseSquashPRTitleAsDefault = v
		}
		if v, ok := req["squash_merge_commit_title"].(string); ok {
			r.SquashMergeCommitTitle = v
		}
		if v, ok := req["squash_merge_commit_message"].(string); ok {
			r.SquashMergeCommitMessage = v
		}
		if v, ok := req["merge_commit_title"].(string); ok {
			r.MergeCommitTitle = v
		}
		if v, ok := req["merge_commit_message"].(string); ok {
			r.MergeCommitMessage = v
		}
		if v, ok := req["pull_request_creation_policy"].(string); ok {
			r.PullRequestCreationPolicy = v
		}
	})

	updated := s.store.GetRepo(owner, name)
	writeJSON(w, http.StatusOK, fullRepoJSONForViewer(updated, s.store, s.baseURL(r), user))
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

	if _, err := s.store.DeleteRepo(owner, name); err != nil {
		writeGHError(w, http.StatusInternalServerError, "repository delete failed: "+err.Error())
		return
	}
	s.recordAuditEvent("repo.destroy", user.Login, "", map[string]interface{}{"repo": owner + "/" + name})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetRepoTopics(w http.ResponseWriter, r *http.Request) {
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

	names := repo.Topics
	if names == nil {
		names = []string{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"names": names,
	})
}

func (s *Server) handlePutRepoTopics(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")

	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	user := ghUserFromContext(r.Context())
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "Must have push access to Repository.")
		return
	}

	var req struct {
		Names []string `json:"names"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if len(req.Names) > 20 {
		writeGHValidationError(w, "Repository", "names", "invalid")
		return
	}
	for _, n := range req.Names {
		if n == "" || len(n) > 50 || strings.ContainsAny(n, " /\\:") {
			writeGHValidationError(w, "Repository", "names", "invalid")
			return
		}
	}

	// Capture the stored topics inside the write callback (under st.mu) — the
	// repo pointer is shared, so reading r.Topics after the lock is released
	// would race a concurrent UpdateRepo writer.
	names := []string{}
	s.store.UpdateRepo(owner, name, func(r *Repo) {
		r.Topics = req.Names
		r.UpdatedAt = time.Now().UTC()
		names = append([]string{}, r.Topics...)
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"names": names,
	})
}

func (s *Server) handleListAuthUserRepos(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}

	opts := repoListOptionsFromQuery(r)
	// GitHub rejects type combined with visibility or affiliation.
	if r.URL.Query().Get("type") != "" && (r.URL.Query().Get("visibility") != "" || r.URL.Query().Get("affiliation") != "") {
		writeGHValidationError(w, "Repository", "type", "invalid")
		return
	}
	opts.NoPaginate = true // REST handlers use paginateAndLink for Link headers

	repos := s.filterReposForFineGrainedPAT(r, s.store.ListReposForAuthUser(user, opts))
	result := make([]map[string]interface{}, 0, len(repos))
	base := s.baseURL(r)
	for _, repo := range repos {
		result = append(result, repoToJSONForViewer(repo, s.store, base, user))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleListUserRepos(w http.ResponseWriter, r *http.Request) {
	login := r.PathValue("username")
	user := s.store.LookupUserByLogin(login)
	if user == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	opts := repoListOptionsFromQuery(r)
	opts.Affiliation = ""  // not applicable
	opts.Visibility = ""   // not applicable
	opts.NoPaginate = true // REST handlers use paginateAndLink for Link headers
	repos := s.store.ListReposForUser(user, opts)
	result := make([]map[string]interface{}, 0, len(repos))
	base := s.baseURL(r)
	viewer := ghUserFromContext(r.Context())
	for _, repo := range repos {
		result = append(result, repoToJSONForViewer(repo, s.store, base, viewer))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func (s *Server) handleListOrgRepos(w http.ResponseWriter, r *http.Request) {
	orgLogin := r.PathValue("org")
	org := s.store.GetOrg(orgLogin)
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	opts := repoListOptionsFromQuery(r)
	opts.Affiliation = ""  // not applicable
	opts.NoPaginate = true // REST handlers use paginateAndLink for Link headers
	repos := s.filterReposForFineGrainedPAT(r, s.store.ListReposForOrg(org.Login, opts))
	result := make([]map[string]interface{}, 0, len(repos))
	base := s.baseURL(r)
	viewer := ghUserFromContext(r.Context())
	for _, repo := range repos {
		result = append(result, repoToJSONForViewer(repo, s.store, base, viewer))
	}
	writeJSON(w, http.StatusOK, paginateAndLink(w, r, result))
}

func repoListOptionsFromQuery(r *http.Request) RepoListOptions {
	q := r.URL.Query()
	return RepoListOptions{
		Type:        q.Get("type"),
		Visibility:  q.Get("visibility"),
		Affiliation: q.Get("affiliation"),
		Sort:        q.Get("sort"),
		Direction:   q.Get("direction"),
		PerPage:     queryInt(q, "per_page", 30),
		Page:        queryInt(q, "page", 1),
	}
}

func queryInt(q url.Values, key string, def int) int {
	s := q.Get(key)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// baseURL computes the external base URL. BLEEPHUB_EXTERNAL_URL wins
// when configured (the GHES "external URL" knob — job messages and
// links must carry an address RUNNERS can reach, not whichever
// interface a triggering API call happened to arrive on); otherwise
// the request's Host.
func (s *Server) baseURL(r *http.Request) string {
	if s.externalURL != "" {
		return s.externalURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// repoToJSON converts a Repo to the GitHub `repository` shape (also a
// valid `minimal-repository`). The hypermedia *_url members carry the
// literal URI-template placeholders real GitHub emits ({/sha}, {+path},
// …). Counters for features bleephub does not model (forks, size) are
// 0; watchers mirrors stargazers exactly as on real GitHub; the has_*
// toggles reflect the surfaces bleephub actually serves. Must not be
// called with st.mu held: it derives open_issues_count from the store.
func repoToJSON(repo *Repo, st *Store, baseURL string) map[string]interface{} {
	return repoToJSONForViewer(repo, st, baseURL, nil)
}

func repoToJSONForViewer(repo *Repo, st *Store, baseURL string, viewer *User) map[string]interface{} {
	// Read every mutable repo field off a private snapshot: UpdateRepo mutates
	// description, topics, homepage, timestamps, etc. under st.mu.Lock, so
	// reading the live pointer here would race a concurrent writer. The
	// snapshot takes st.mu only for the copy — the store lookups below
	// (GetOrg / CountOpenIssues / …) take their own locks, so they must run
	// after the snapshot releases the lock, never nested under it.
	repo = st.snapRepo(repo)
	ownerJSON := map[string]interface{}{}
	if repo.OwnerType == "Organization" {
		parts := strings.SplitN(repo.FullName, "/", 2)
		if len(parts) == 2 {
			if org := st.GetOrg(parts[0]); org != nil {
				ownerJSON = orgAsSimpleUserJSON(org)
			}
		}
	} else if repo.Owner != nil {
		ownerJSON = userToJSON(repo.Owner)
	}

	topics := repo.Topics
	if topics == nil {
		topics = []string{}
	}

	host := baseURL
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	api := baseURL + "/api/v3/repos/" + repo.FullName
	openIssues := st.CountOpenIssues(repo.ID)
	forks := st.CountForks(repo.ID)

	return map[string]interface{}{
		"id":                repo.ID,
		"node_id":           repo.NodeID,
		"name":              repo.Name,
		"full_name":         repo.FullName,
		"owner":             ownerJSON,
		"private":           repo.Private,
		"html_url":          baseURL + "/" + repo.FullName,
		"description":       repo.Description,
		"fork":              repo.Fork,
		"url":               api,
		"archive_url":       api + "/{archive_format}{/ref}",
		"assignees_url":     api + "/assignees{/user}",
		"blobs_url":         api + "/git/blobs{/sha}",
		"branches_url":      api + "/branches{/branch}",
		"collaborators_url": api + "/collaborators{/collaborator}",
		"comments_url":      api + "/comments{/number}",
		"commits_url":       api + "/commits{/sha}",
		"compare_url":       api + "/compare/{base}...{head}",
		"contents_url":      api + "/contents/{+path}",
		"contributors_url":  api + "/contributors",
		"deployments_url":   api + "/deployments",
		"downloads_url":     api + "/downloads",
		"events_url":        api + "/events",
		"forks_url":         api + "/forks",
		"git_commits_url":   api + "/git/commits{/sha}",
		"git_refs_url":      api + "/git/refs{/sha}",
		"git_tags_url":      api + "/git/tags{/sha}",
		"hooks_url":         api + "/hooks",
		"issue_comment_url": api + "/issues/comments{/number}",
		"issue_events_url":  api + "/issues/events{/number}",
		"issues_url":        api + "/issues{/number}",
		"keys_url":          api + "/keys{/key_id}",
		"labels_url":        api + "/labels{/name}",
		"languages_url":     api + "/languages",
		"merges_url":        api + "/merges",
		"milestones_url":    api + "/milestones{/number}",
		"notifications_url": api + "/notifications{?since,all,participating}",
		"pulls_url":         api + "/pulls{/number}",
		"releases_url":      api + "/releases{/id}",
		"stargazers_url":    api + "/stargazers",
		"statuses_url":      api + "/statuses/{sha}",
		"subscribers_url":   api + "/subscribers",
		"subscription_url":  api + "/subscription",
		"tags_url":          api + "/tags",
		"teams_url":         api + "/teams",
		"trees_url":         api + "/git/trees{/sha}",
		"clone_url":         baseURL + "/" + repo.FullName + ".git",
		"git_url":           "git://" + host + "/" + repo.FullName + ".git",
		"ssh_url":           sshGitURL(repo.FullName),
		"svn_url":           baseURL + "/" + repo.FullName,
		"mirror_url":        nil,
		"homepage":          nilOrString(repo.Homepage),
		"license":           licenseJSON(repo),
		"default_branch":    repo.DefaultBranch,
		"visibility":        repo.Visibility,
		"language":          repo.Language,
		"archived":          repo.Archived,
		"disabled":          false,
		"forks":             forks,
		"forks_count":       forks,
		"size":              st.RepoSize(repo.FullName),
		"stargazers_count":  repo.StargazersCount,
		"watchers":          repo.StargazersCount,
		"watchers_count":    repo.StargazersCount,
		"open_issues":       openIssues,
		"open_issues_count": openIssues,
		"has_issues":        repo.HasIssues,
		"has_projects":      repo.HasProjects,
		"has_wiki":          repo.HasWiki,
		"has_pages":         st.HasPagesSite(repo.ID),
		"has_downloads":     false,
		"has_discussions":   repoHasDiscussions(repo),
		"has_pull_requests": repo.HasPullRequests,
		"topics":            topics,
		"permissions":       repoPermissionsJSON(st, viewer, repo),
		"created_at":        repo.CreatedAt.Format(time.RFC3339),
		"updated_at":        repo.UpdatedAt.Format(time.RFC3339),
		"pushed_at":         nullableTimestamp(repo.PushedAt),
	}
}

func repoPermissionsJSON(st *Store, viewer *User, repo *Repo) map[string]bool {
	return map[string]bool{
		"admin": canAdminRepo(st, viewer, repo),
		"push":  canPushRepo(st, viewer, repo),
		"pull":  canReadRepo(st, viewer, repo),
	}
}

// fullRepoJSON converts a Repo to the GitHub `full-repository` shape
// served by single-repo operations (GET/PATCH /repos/{owner}/{repo},
// repo creation). It is the repository shape plus the network/subscriber
// counters that exist only on full-repository, both derived from real
// store state: the fork network and the watch subscriptions.
func fullRepoJSON(repo *Repo, st *Store, baseURL string) map[string]interface{} {
	return fullRepoJSONForViewer(repo, st, baseURL, nil)
}

func fullRepoJSONForViewer(repo *Repo, st *Store, baseURL string, viewer *User) map[string]interface{} {
	out := repoToJSONForViewer(repo, st, baseURL, viewer)
	out["network_count"] = out["forks_count"]
	out["subscribers_count"] = len(st.ListRepoSubscribers(repo.ID))
	out["organization"] = repoOrganizationJSON(repo, st)
	out["allow_squash_merge"] = repo.AllowSquashMerge
	out["allow_merge_commit"] = repo.AllowMergeCommit
	out["allow_rebase_merge"] = repo.AllowRebaseMerge
	out["allow_auto_merge"] = repo.AllowAutoMerge
	out["allow_update_branch"] = repo.AllowUpdateBranch
	out["delete_branch_on_merge"] = repo.DeleteBranchOnMerge
	out["allow_forking"] = false
	out["web_commit_signoff_required"] = repo.WebCommitSignoffRequired
	out["is_template"] = repo.IsTemplate
	out["use_squash_pr_title_as_default"] = repo.UseSquashPRTitleAsDefault
	if repo.SquashMergeCommitTitle != "" {
		out["squash_merge_commit_title"] = repo.SquashMergeCommitTitle
	}
	if repo.SquashMergeCommitMessage != "" {
		out["squash_merge_commit_message"] = repo.SquashMergeCommitMessage
	}
	if repo.MergeCommitTitle != "" {
		out["merge_commit_title"] = repo.MergeCommitTitle
	}
	if repo.MergeCommitMessage != "" {
		out["merge_commit_message"] = repo.MergeCommitMessage
	}
	if repo.PullRequestCreationPolicy != "" {
		out["pull_request_creation_policy"] = repo.PullRequestCreationPolicy
	}
	if repo.Fork {
		if parent := st.GetRepoByID(repo.ParentID); parent != nil {
			out["parent"] = repoToJSONForViewer(parent, st, baseURL, viewer)
		}
		if source := st.GetRepoByID(repo.SourceID); source != nil {
			out["source"] = repoToJSONForViewer(source, st, baseURL, viewer)
		}
	}
	return out
}

func nilOrString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullableTimestamp(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func nullableTimePtr(t *time.Time) interface{} {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func licenseJSON(repo *Repo) interface{} {
	if repo.LicenseKey == "" {
		return nil
	}
	return map[string]interface{}{
		"key":     repo.LicenseKey,
		"name":    repo.LicenseName,
		"spdx_id": repo.LicenseSPDX,
		"url":     nil,
		"node_id": "MDc6TGljZW5zZQ==" + repo.LicenseKey,
	}
}

func repoOrganizationJSON(repo *Repo, st *Store) interface{} {
	if repo.OwnerType != "Organization" {
		return nil
	}
	parts := strings.SplitN(repo.FullName, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	org := st.GetOrg(parts[0])
	if org == nil {
		return nil
	}
	return orgAsSimpleUserJSON(org)
}

func (s *Server) handleListStargazers(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	if s.store.GetRepo(owner, name) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	ids := s.store.ListRepoStargazers(owner, name)
	out := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		if u := s.store.GetUserByID(id); u != nil {
			out = append(out, userToJSON(u))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleStarRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	if !s.store.StarRepo(user.ID, owner, name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnstarRepo(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	if !s.store.UnstarRepo(user.ID, owner, name) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListStarredRepos(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	names := s.store.ListStarredRepos(user.ID)
	repos := make([]*Repo, 0, len(names))
	for _, fullName := range names {
		parts := strings.SplitN(fullName, "/", 2)
		if len(parts) != 2 {
			continue
		}
		if repo := s.store.GetRepo(parts[0], parts[1]); repo != nil {
			repos = append(repos, repo)
		}
	}
	repos = s.filterReposForFineGrainedPAT(r, repos)
	out := make([]map[string]interface{}, 0, len(repos))
	for _, repo := range repos {
		out = append(out, fullRepoJSONForViewer(repo, s.store, s.baseURL(r), user))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListUserStarredRepos(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	u := s.store.LookupUserByLogin(username)
	if u == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	names := s.store.ListStarredRepos(u.ID)
	out := make([]map[string]interface{}, 0, len(names))
	viewer := ghUserFromContext(r.Context())
	for _, fullName := range names {
		parts := strings.SplitN(fullName, "/", 2)
		if len(parts) != 2 {
			continue
		}
		if repo := s.store.GetRepo(parts[0], parts[1]); repo != nil {
			out = append(out, fullRepoJSONForViewer(repo, s.store, s.baseURL(r), viewer))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListCollaborators(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	collabs := s.store.ListRepoCollaborators(owner, name)
	out := make([]map[string]interface{}, 0, len(collabs)+1)
	if repo.Owner != nil {
		out = append(out, collaboratorJSON(repo.Owner, "admin"))
	}
	for login, perm := range collabs {
		if u := s.store.LookupUserByLogin(login); u != nil {
			out = append(out, collaboratorJSON(u, perm))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetCollaboratorPermission(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	username := r.PathValue("username")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	u := s.store.LookupUserByLogin(username)
	if u == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	perm := s.store.GetRepoCollaboratorPermission(owner, name, username)
	if perm == "" {
		if repo.Owner != nil && strings.EqualFold(repo.Owner.Login, username) {
			perm = "admin"
		} else {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
	}
	userJSON := userToJSON(u)
	userJSON["role_name"] = githubRoleName(perm)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"permission": perm,
		"user":       userJSON,
		"role_name":  githubRoleName(perm),
	})
}

func (s *Server) handleAddCollaborator(w http.ResponseWriter, r *http.Request) {
	actor := ghUserFromContext(r.Context())
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	repo := s.store.GetRepo(owner, name)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	username := r.PathValue("username")
	u := s.store.LookupUserByLogin(username)
	if u == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Permission string `json:"permission"`
	}
	if r.Header.Get("Content-Length") != "0" && r.ContentLength > 0 {
		if !decodeJSONBody(w, r, &req) {
			return
		}
	}
	// A PUT naming an existing collaborator (or the owner, who always has
	// admin) updates the permission in place and answers 204, as on real
	// GitHub.
	isOwner := repo.Owner != nil && strings.EqualFold(repo.Owner.Login, username)
	if isOwner || s.store.GetRepoCollaboratorPermission(owner, name, username) != "" {
		if !isOwner && !s.store.AddRepoCollaborator(owner, name, username, req.Permission) {
			writeGHError(w, http.StatusNotFound, "Not Found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Inviting a new user creates a pending repository invitation — the
	// invitee joins the collaborator list when they accept it. Re-inviting
	// while an invitation is pending updates that invitation instead of
	// creating a second one.
	inviterID := 0
	if actor != nil {
		inviterID = actor.ID
	} else if repo.Owner != nil {
		inviterID = repo.Owner.ID
	}
	var inv *RepoInvitation
	for _, pending := range s.store.ListPendingRepoInvitations(repo.FullName) {
		if strings.EqualFold(pending.InviteeLogin, username) {
			inv = s.store.UpdateRepoInvitation(repo.FullName, pending.ID, req.Permission)
			break
		}
	}
	if inv == nil {
		inv = s.store.CreateRepoInvitation(repo.FullName, u.Login, "", inviterID, req.Permission)
	}
	writeJSON(w, http.StatusCreated, invitationJSON(inv, repo, s.store, s.baseURL(r)))
}

func githubRoleName(perm string) string {
	switch perm {
	case "push":
		return "write"
	case "admin":
		return "admin"
	case "pull":
		return "read"
	default:
		return perm
	}
}

func (s *Server) handleRemoveCollaborator(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("repo")
	if s.store.GetRepo(owner, name) == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	username := r.PathValue("username")
	if !s.store.RemoveRepoCollaborator(owner, name, username) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func collaboratorJSON(u *User, perm string) map[string]interface{} {
	json := userToJSON(u)
	json["permissions"] = collaboratorPermsJSON(perm)
	json["role_name"] = perm
	return json
}

func collaboratorPermsJSON(perm string) map[string]bool {
	levels := map[string]int{"pull": 1, "push": 2, "admin": 3}
	level := levels[perm]
	return map[string]bool{
		"pull":  level >= 1,
		"push":  level >= 2,
		"admin": level >= 3,
	}
}

// simpleRepoJSON returns a GitHub `simple-repository`-shaped map. It is a
// trimmed subset of repoToJSON with only the fields the simple-repository
// schema allows, used by alert/list surfaces that embed a repository object.
func simpleRepoJSON(repo *Repo, st *Store, baseURL string) map[string]interface{} {
	api := baseURL + "/api/v3/repos/" + repo.FullName
	return map[string]interface{}{
		"id":                repo.ID,
		"node_id":           repo.NodeID,
		"name":              repo.Name,
		"full_name":         repo.FullName,
		"owner":             repoOwnerREST(repo, st, baseURL),
		"private":           repo.Private,
		"html_url":          baseURL + "/" + repo.FullName,
		"description":       repo.Description,
		"fork":              repo.Fork,
		"url":               api,
		"archive_url":       api + "/{archive_format}{/ref}",
		"assignees_url":     api + "/assignees{/user}",
		"blobs_url":         api + "/git/blobs{/sha}",
		"branches_url":      api + "/branches{/branch}",
		"collaborators_url": api + "/collaborators{/collaborator}",
		"comments_url":      api + "/comments{/number}",
		"commits_url":       api + "/commits{/sha}",
		"compare_url":       api + "/compare/{base}...{head}",
		"contents_url":      api + "/contents/{+path}",
		"contributors_url":  api + "/contributors",
		"deployments_url":   api + "/deployments",
		"downloads_url":     api + "/downloads",
		"events_url":        api + "/events",
		"forks_url":         api + "/forks",
		"git_commits_url":   api + "/git/commits{/sha}",
		"git_refs_url":      api + "/git/refs{/sha}",
		"git_tags_url":      api + "/git/tags{/sha}",
		"hooks_url":         api + "/hooks",
		"issue_comment_url": api + "/issues/comments{/number}",
		"issue_events_url":  api + "/issues/events{/number}",
		"issues_url":        api + "/issues{/number}",
		"keys_url":          api + "/keys{/key_id}",
		"labels_url":        api + "/labels{/name}",
		"languages_url":     api + "/languages",
		"merges_url":        api + "/merges",
		"milestones_url":    api + "/milestones{/number}",
		"notifications_url": api + "/notifications{?since,all,participating}",
		"pulls_url":         api + "/pulls{/number}",
		"releases_url":      api + "/releases{/id}",
		"stargazers_url":    api + "/stargazers",
		"statuses_url":      api + "/statuses/{sha}",
		"subscribers_url":   api + "/subscribers",
		"subscription_url":  api + "/subscription",
		"tags_url":          api + "/tags",
		"teams_url":         api + "/teams",
		"trees_url":         api + "/git/trees{/sha}",
	}
}
