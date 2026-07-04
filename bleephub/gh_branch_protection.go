package bleephub

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// BranchProtection matches the GitHub REST API response shape for
// GET /repos/{owner}/{repo}/branches/{branch}/protection. All pointer
// sub-fields are omitempty in JSON so that an unset rule does not appear
// in the canonical response.
type BranchProtection struct {
	RequiredStatusChecks           *BPStatusChecks       `json:"required_status_checks,omitempty"`
	RequiredPullRequestReviews     *BPPullRequestReviews `json:"required_pull_request_reviews,omitempty"`
	EnforceAdmins                  *BPEnforceAdmins      `json:"enforce_admins,omitempty"`
	Restrictions                   *BPRestrictions       `json:"restrictions,omitempty"`
	RequiredLinearHistory          *BPEnabled            `json:"required_linear_history,omitempty"`
	AllowForcePushes               *BPEnabled            `json:"allow_force_pushes,omitempty"`
	AllowDeletions                 *BPEnabled            `json:"allow_deletions,omitempty"`
	BlockCreations                 *BPEnabled            `json:"block_creations,omitempty"`
	RequiredConversationResolution *BPEnabled            `json:"required_conversation_resolution,omitempty"`
	RequiredSignatures             *BPEnabledURL         `json:"required_signatures,omitempty"`
	URL                            string                `json:"url,omitempty"`
}

// IsProtected reports whether the branch has any protection rule enabled.
func (bp *BranchProtection) IsProtected() bool {
	if bp == nil {
		return false
	}
	return bp.RequiredStatusChecks != nil ||
		bp.RequiredPullRequestReviews != nil ||
		bp.EnforceAdmins != nil ||
		bp.Restrictions != nil ||
		bp.RequiredLinearHistory != nil ||
		bp.AllowForcePushes != nil ||
		bp.AllowDeletions != nil ||
		bp.BlockCreations != nil ||
		bp.RequiredConversationResolution != nil ||
		bp.RequiredSignatures != nil
}

// BPStatusChecks is the required_status_checks object. contexts and checks
// are required members of the published status-check-policy shape, so they
// serialize even when empty (hydrateBranchProtectionURLs normalizes nil
// slices before responses are written).
type BPStatusChecks struct {
	URL              string    `json:"url,omitempty"`
	EnforcementLevel string    `json:"enforcement_level,omitempty"`
	Contexts         []string  `json:"contexts"`
	Checks           []BPCheck `json:"checks"`
	Strict           bool      `json:"strict"`
	ContextsURL      string    `json:"contexts_url,omitempty"`
}

// BPCheck is an entry in required_status_checks.checks. app_id is a
// required, nullable member — it serializes as null when no app is pinned.
type BPCheck struct {
	Context string `json:"context"`
	AppID   *int64 `json:"app_id"`
}

// BPPullRequestReviews is the required_pull_request_reviews object.
type BPPullRequestReviews struct {
	URL                          string              `json:"url,omitempty"`
	DismissStaleReviews          bool                `json:"dismiss_stale_reviews"`
	RequireCodeOwnerReviews      bool                `json:"require_code_owner_reviews"`
	RequiredApprovingReviewCount int                 `json:"required_approving_review_count"`
	DismissalRestrictions        *BPRestrictions     `json:"dismissal_restrictions,omitempty"`
	BypassPullRequestAllowances  *BPBypassAllowances `json:"bypass_pull_request_allowances,omitempty"`
}

// BPEnforceAdmins is the enforce_admins object.
type BPEnforceAdmins struct {
	URL     string `json:"url,omitempty"`
	Enabled bool   `json:"enabled"`
}

// BPRestrictions is the restrictions object (push + dismissal). users,
// teams, and apps are required members of the published
// branch-restriction-policy shape, so they serialize even when empty.
type BPRestrictions struct {
	Users    []BPActor `json:"users"`
	Teams    []BPActor `json:"teams"`
	Apps     []BPActor `json:"apps"`
	URL      string    `json:"url,omitempty"`
	UsersURL string    `json:"users_url,omitempty"`
	TeamsURL string    `json:"teams_url,omitempty"`
	AppsURL  string    `json:"apps_url,omitempty"`
}

// BPBypassAllowances lists users/teams/apps that can bypass pull-request requirements.
type BPBypassAllowances struct {
	Users []BPActor `json:"users,omitempty"`
	Teams []BPActor `json:"teams,omitempty"`
	Apps  []BPActor `json:"apps,omitempty"`
}

// BPActor is a lightweight user/team/app reference used in restrictions.
type BPActor struct {
	Login string `json:"login"`
	ID    int    `json:"id"`
	Type  string `json:"type"`
}

// BPEnabled is the shape used by required_linear_history, allow_force_pushes,
// allow_deletions, block_creations, and required_conversation_resolution.
type BPEnabled struct {
	Enabled bool `json:"enabled"`
}

// BPEnabledURL is the shape used by required_signatures which also carries a URL.
type BPEnabledURL struct {
	URL     string `json:"url,omitempty"`
	Enabled bool   `json:"enabled"`
}

// bpRequest is the PUT body for the top-level protection endpoint. GitHub
// accepts a sparse body: missing sub-objects leave existing rules unchanged,
// while an explicit null disables the corresponding rule.
type bpRequest struct {
	RequiredStatusChecks           *BPStatusChecks       `json:"required_status_checks"`
	RequiredPullRequestReviews     *BPPullRequestReviews `json:"required_pull_request_reviews"`
	EnforceAdmins                  *flexBool             `json:"enforce_admins"`
	Restrictions                   *BPRestrictions       `json:"restrictions"`
	RequiredLinearHistory          *flexBool             `json:"required_linear_history"`
	AllowForcePushes               *flexBool             `json:"allow_force_pushes"`
	AllowDeletions                 *flexBool             `json:"allow_deletions"`
	BlockCreations                 *flexBool             `json:"block_creations"`
	RequiredConversationResolution *flexBool             `json:"required_conversation_resolution"`
	RequiredSignatures             *flexBool             `json:"required_signatures"`
}

func (s *Server) registerGHBranchProtectionRoutes() {
	// Top-level branch protection
	s.route("GET /api/v3/repos/{owner}/{repo}/branches/{branch}/protection", s.handleBranchProtectionGet)
	s.route("PUT /api/v3/repos/{owner}/{repo}/branches/{branch}/protection",
		s.requirePerm(scopeAdministration, permWrite, s.handleBranchProtectionPut))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/branches/{branch}/protection",
		s.requirePerm(scopeAdministration, permWrite, s.handleBranchProtectionDelete))

	// Required status checks
	s.route("GET /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/required_status_checks", s.handleBPStatusChecksGet)
	s.route("PUT /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/required_status_checks",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPStatusChecksPut))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/required_status_checks",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPStatusChecksPatch))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/required_status_checks",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPStatusChecksDelete))

	// Required commit signatures
	s.route("GET /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/required_signatures", s.handleBPRequiredSignaturesGet)
	s.route("POST /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/required_signatures",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPRequiredSignaturesPost))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/required_signatures",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPRequiredSignaturesDelete))

	// Restrictions apps
	s.route("GET /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/restrictions/apps", s.handleBPRestrictionsAppsGet)
	s.route("POST /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/restrictions/apps",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPRestrictionsAppsPost))
	s.route("PUT /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/restrictions/apps",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPRestrictionsAppsPut))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/restrictions/apps",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPRestrictionsAppsDelete))

	// Required status checks contexts
	s.route("GET /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/required_status_checks/contexts", s.handleBPContextsGet)
	s.route("POST /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/required_status_checks/contexts",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPContextsPost))
	s.route("PUT /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/required_status_checks/contexts",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPContextsPut))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/required_status_checks/contexts",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPContextsDelete))

	// Required pull request reviews
	s.route("GET /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/required_pull_request_reviews", s.handleBPReviewsGet)
	s.route("PATCH /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/required_pull_request_reviews",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPReviewsPatch))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/required_pull_request_reviews",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPReviewsDelete))

	// Restrictions
	s.route("GET /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/restrictions", s.handleBPRestrictionsGet)
	s.route("PUT /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/restrictions",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPRestrictionsPut))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/restrictions",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPRestrictionsDelete))

	// Restrictions users
	s.route("GET /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/restrictions/users", s.handleBPRestrictionsUsersGet)
	s.route("POST /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/restrictions/users",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPRestrictionsUsersPost))
	s.route("PUT /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/restrictions/users",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPRestrictionsUsersPut))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/restrictions/users",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPRestrictionsUsersDelete))

	// Restrictions teams
	s.route("GET /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/restrictions/teams", s.handleBPRestrictionsTeamsGet)
	s.route("POST /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/restrictions/teams",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPRestrictionsTeamsPost))
	s.route("PUT /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/restrictions/teams",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPRestrictionsTeamsPut))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/restrictions/teams",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPRestrictionsTeamsDelete))

	// Enforce admins
	s.route("GET /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/enforce_admins", s.handleBPEnforceAdminsGet)
	s.route("POST /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/enforce_admins",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPEnforceAdminsPost))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/enforce_admins",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPEnforceAdminsDelete))

	// Allow force pushes
	s.route("GET /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/allow_force_pushes", s.handleBPAllowForcePushesGet)
	s.route("PUT /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/allow_force_pushes",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPAllowForcePushesPut))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/allow_force_pushes",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPAllowForcePushesDelete))

	// Allow deletions
	s.route("GET /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/allow_deletions", s.handleBPAllowDeletionsGet)
	s.route("PUT /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/allow_deletions",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPAllowDeletionsPut))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/branches/{branch}/protection/allow_deletions",
		s.requirePerm(scopeAdministration, permWrite, s.handleBPAllowDeletionsDelete))
}

// branchProtectionURL returns the canonical URL for the top-level protection resource.
func (s *Server) branchProtectionURL(baseURL, fullName, branch string) string {
	return baseURL + "/repos/" + fullName + "/branches/" + branch + "/protection"
}

func (s *Server) branchProtectionSubURL(baseURL, fullName, branch, sub string) string {
	return s.branchProtectionURL(baseURL, fullName, branch) + "/" + sub
}

func (s *Server) getBranchProtection(r *http.Request) (*Repo, string, *BranchProtection) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		return nil, "", nil
	}
	branch := r.PathValue("branch")
	s.store.Misc.mu.RLock()
	bp := s.store.Misc.branchProtection[bpKey(repo.ID, branch)]
	s.store.Misc.mu.RUnlock()
	return repo, branch, bp
}

func (s *Server) setBranchProtection(repo *Repo, branch string, bp *BranchProtection) {
	key := bpKey(repo.ID, branch)
	s.store.Misc.mu.Lock()
	if bp == nil || !bp.IsProtected() {
		delete(s.store.Misc.branchProtection, key)
		if s.store.Misc.persist != nil {
			s.store.Misc.persist.MustDelete("branch_protection", key)
		}
	} else {
		s.store.Misc.branchProtection[key] = bp
		if s.store.Misc.persist != nil {
			s.store.Misc.persist.MustPut("branch_protection", key, bp)
		}
	}
	s.store.Misc.mu.Unlock()
}

func (s *Server) branchProtectionNotFound(w http.ResponseWriter) {
	writeGHError(w, http.StatusNotFound, "Branch not protected")
}

// --- Top-level protection ---

func (s *Server) handleBranchProtectionGet(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil {
		s.branchProtectionNotFound(w)
		return
	}
	bp = s.hydrateBranchProtectionURLs(bp, repo, branch, s.baseURL(r))
	writeJSON(w, http.StatusOK, bp)
}

func (s *Server) handleBranchProtectionPut(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	branch := r.PathValue("branch")

	var req bpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return
	}

	s.store.Misc.mu.Lock()
	bp := s.store.Misc.branchProtection[bpKey(repo.ID, branch)]
	if bp == nil {
		bp = &BranchProtection{}
	}
	bp = s.applyBranchProtectionRequest(bp, &req)
	key := bpKey(repo.ID, branch)
	s.store.Misc.branchProtection[key] = bp
	if s.store.Misc.persist != nil {
		s.store.Misc.persist.MustPut("branch_protection", key, bp)
	}
	s.store.Misc.mu.Unlock()

	bp = s.hydrateBranchProtectionURLs(bp, repo, branch, s.baseURL(r))
	writeJSON(w, http.StatusOK, bp)
}

func (s *Server) handleBranchProtectionDelete(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	branch := r.PathValue("branch")
	s.setBranchProtection(repo, branch, nil)
	w.WriteHeader(http.StatusNoContent)
}

// applyBranchProtectionRequest merges a sparse PUT body into an existing rule.
// A present but null field clears the rule; an absent field leaves it unchanged.
func (s *Server) applyBranchProtectionRequest(bp *BranchProtection, req *bpRequest) *BranchProtection {
	if req.RequiredStatusChecks != nil {
		if !s.isEmptyStatusChecks(req.RequiredStatusChecks) {
			bp.RequiredStatusChecks = req.RequiredStatusChecks
		} else {
			bp.RequiredStatusChecks = nil
		}
	}
	if req.RequiredPullRequestReviews != nil {
		if !s.isEmptyReviews(req.RequiredPullRequestReviews) {
			bp.RequiredPullRequestReviews = req.RequiredPullRequestReviews
		} else {
			bp.RequiredPullRequestReviews = nil
		}
	}
	if req.EnforceAdmins != nil {
		if bool(*req.EnforceAdmins) {
			bp.EnforceAdmins = &BPEnforceAdmins{Enabled: true}
		} else {
			bp.EnforceAdmins = nil
		}
	}
	if req.Restrictions != nil {
		if !s.isEmptyRestrictions(req.Restrictions) {
			bp.Restrictions = req.Restrictions
		} else {
			bp.Restrictions = nil
		}
	}
	if req.RequiredLinearHistory != nil {
		if bool(*req.RequiredLinearHistory) {
			bp.RequiredLinearHistory = &BPEnabled{Enabled: true}
		} else {
			bp.RequiredLinearHistory = nil
		}
	}
	if req.AllowForcePushes != nil {
		if bool(*req.AllowForcePushes) {
			bp.AllowForcePushes = &BPEnabled{Enabled: true}
		} else {
			bp.AllowForcePushes = nil
		}
	}
	if req.AllowDeletions != nil {
		if bool(*req.AllowDeletions) {
			bp.AllowDeletions = &BPEnabled{Enabled: true}
		} else {
			bp.AllowDeletions = nil
		}
	}
	if req.BlockCreations != nil {
		if bool(*req.BlockCreations) {
			bp.BlockCreations = &BPEnabled{Enabled: true}
		} else {
			bp.BlockCreations = nil
		}
	}
	if req.RequiredConversationResolution != nil {
		if bool(*req.RequiredConversationResolution) {
			bp.RequiredConversationResolution = &BPEnabled{Enabled: true}
		} else {
			bp.RequiredConversationResolution = nil
		}
	}
	if req.RequiredSignatures != nil {
		if bool(*req.RequiredSignatures) {
			bp.RequiredSignatures = &BPEnabledURL{Enabled: true}
		} else {
			bp.RequiredSignatures = nil
		}
	}
	return bp
}

func (s *Server) isEmptyStatusChecks(sc *BPStatusChecks) bool {
	return sc == nil || (!sc.Strict && len(sc.Contexts) == 0 && len(sc.Checks) == 0)
}

func (s *Server) isEmptyReviews(r *BPPullRequestReviews) bool {
	return r == nil || (!r.DismissStaleReviews && !r.RequireCodeOwnerReviews && r.RequiredApprovingReviewCount == 0 && r.DismissalRestrictions == nil && r.BypassPullRequestAllowances == nil)
}

func (s *Server) isEmptyRestrictions(r *BPRestrictions) bool {
	return r == nil || (len(r.Users) == 0 && len(r.Teams) == 0 && len(r.Apps) == 0)
}

func (s *Server) hydrateBranchProtectionURLs(bp *BranchProtection, repo *Repo, branch, baseURL string) *BranchProtection {
	if bp == nil {
		return nil
	}
	bp.URL = s.branchProtectionURL(baseURL, repo.FullName, branch)
	if bp.RequiredStatusChecks != nil {
		bp.RequiredStatusChecks.URL = s.branchProtectionSubURL(baseURL, repo.FullName, branch, "required_status_checks")
		bp.RequiredStatusChecks.ContextsURL = s.branchProtectionSubURL(baseURL, repo.FullName, branch, "required_status_checks/contexts")
		if bp.RequiredStatusChecks.Contexts == nil {
			bp.RequiredStatusChecks.Contexts = []string{}
		}
		if bp.RequiredStatusChecks.Checks == nil {
			bp.RequiredStatusChecks.Checks = []BPCheck{}
		}
	}
	if bp.RequiredPullRequestReviews != nil {
		bp.RequiredPullRequestReviews.URL = s.branchProtectionSubURL(baseURL, repo.FullName, branch, "required_pull_request_reviews")
		if bp.RequiredPullRequestReviews.DismissalRestrictions != nil {
			s.hydrateRestrictionsURLs(bp.RequiredPullRequestReviews.DismissalRestrictions, baseURL, repo.FullName, branch, "dismissal_restrictions")
		}
	}
	if bp.EnforceAdmins != nil {
		bp.EnforceAdmins.URL = s.branchProtectionSubURL(baseURL, repo.FullName, branch, "enforce_admins")
	}
	if bp.Restrictions != nil {
		s.hydrateRestrictionsURLs(bp.Restrictions, baseURL, repo.FullName, branch, "restrictions")
	}
	if bp.RequiredSignatures != nil {
		bp.RequiredSignatures.URL = s.branchProtectionSubURL(baseURL, repo.FullName, branch, "required_signatures")
	}
	return bp
}

func (s *Server) hydrateRestrictionsURLs(r *BPRestrictions, baseURL, fullName, branch, sub string) {
	r.URL = s.branchProtectionSubURL(baseURL, fullName, branch, sub)
	r.UsersURL = s.branchProtectionSubURL(baseURL, fullName, branch, sub+"/users")
	r.TeamsURL = s.branchProtectionSubURL(baseURL, fullName, branch, sub+"/teams")
	r.AppsURL = s.branchProtectionSubURL(baseURL, fullName, branch, sub+"/apps")
	if r.Users == nil {
		r.Users = []BPActor{}
	}
	if r.Teams == nil {
		r.Teams = []BPActor{}
	}
	if r.Apps == nil {
		r.Apps = []BPActor{}
	}
}

// --- Required status checks ---

func (s *Server) handleBPStatusChecksGet(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.RequiredStatusChecks == nil {
		s.branchProtectionNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.statusCheckPolicyJSON(bp.RequiredStatusChecks, repo, branch, s.baseURL(r)))
}

// statusCheckPolicyJSON renders required_status_checks in the published
// status-check-policy shape: url, strict, contexts, checks, and contexts_url
// are all required members — contexts/checks are present even when empty.
func (s *Server) statusCheckPolicyJSON(sc *BPStatusChecks, repo *Repo, branch, baseURL string) map[string]interface{} {
	contexts := sc.Contexts
	if contexts == nil {
		contexts = []string{}
	}
	checks := make([]map[string]interface{}, 0, len(sc.Checks))
	for _, c := range sc.Checks {
		var appID interface{}
		if c.AppID != nil {
			appID = *c.AppID
		}
		checks = append(checks, map[string]interface{}{"context": c.Context, "app_id": appID})
	}
	return map[string]interface{}{
		"url":          s.branchProtectionSubURL(baseURL, repo.FullName, branch, "required_status_checks"),
		"strict":       sc.Strict,
		"contexts":     contexts,
		"checks":       checks,
		"contexts_url": s.branchProtectionSubURL(baseURL, repo.FullName, branch, "required_status_checks/contexts"),
	}
}

// handleBPStatusChecksPatch merges a partial update into an existing
// required_status_checks rule. Absent members are left unchanged; present
// members replace the stored value.
func (s *Server) handleBPStatusChecksPatch(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.RequiredStatusChecks == nil {
		s.branchProtectionNotFound(w)
		return
	}
	var req struct {
		Strict   *bool      `json:"strict"`
		Contexts *[]string  `json:"contexts"`
		Checks   *[]BPCheck `json:"checks"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Strict != nil {
		bp.RequiredStatusChecks.Strict = *req.Strict
	}
	if req.Contexts != nil {
		bp.RequiredStatusChecks.Contexts = *req.Contexts
	}
	if req.Checks != nil {
		bp.RequiredStatusChecks.Checks = *req.Checks
	}
	s.setBranchProtection(repo, branch, bp)
	writeJSON(w, http.StatusOK, s.statusCheckPolicyJSON(bp.RequiredStatusChecks, repo, branch, s.baseURL(r)))
}

func (s *Server) handleBPStatusChecksPut(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil {
		s.branchProtectionNotFound(w)
		return
	}
	var req BPStatusChecks
	if !decodeJSONBody(w, r, &req) {
		return
	}
	bp.RequiredStatusChecks = &req
	s.setBranchProtection(repo, branch, bp)
	req.URL = s.branchProtectionSubURL(s.baseURL(r), repo.FullName, branch, "required_status_checks")
	req.ContextsURL = s.branchProtectionSubURL(s.baseURL(r), repo.FullName, branch, "required_status_checks/contexts")
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleBPStatusChecksDelete(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil {
		s.branchProtectionNotFound(w)
		return
	}
	bp.RequiredStatusChecks = nil
	s.setBranchProtection(repo, branch, bp)
	w.WriteHeader(http.StatusNoContent)
}

// --- Contexts ---

func (s *Server) handleBPContextsGet(w http.ResponseWriter, r *http.Request) {
	repo, _, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.RequiredStatusChecks == nil {
		s.branchProtectionNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, bp.RequiredStatusChecks.Contexts)
}

func (s *Server) handleBPContextsPost(w http.ResponseWriter, r *http.Request) {
	s.handleBPContextsPut(w, r)
}

func (s *Server) handleBPContextsPut(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.RequiredStatusChecks == nil {
		s.branchProtectionNotFound(w)
		return
	}
	var contexts []string
	if !decodeStringArrayBody(w, r, &contexts) {
		return
	}
	bp.RequiredStatusChecks.Contexts = contexts
	s.setBranchProtection(repo, branch, bp)
	writeJSON(w, http.StatusOK, contexts)
}

func (s *Server) handleBPContextsDelete(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.RequiredStatusChecks == nil {
		s.branchProtectionNotFound(w)
		return
	}
	bp.RequiredStatusChecks.Contexts = nil
	s.setBranchProtection(repo, branch, bp)
	w.WriteHeader(http.StatusNoContent)
}

// --- Required pull request reviews ---

func (s *Server) handleBPReviewsGet(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.RequiredPullRequestReviews == nil {
		s.branchProtectionNotFound(w)
		return
	}
	rev := *bp.RequiredPullRequestReviews
	rev.URL = s.branchProtectionSubURL(s.baseURL(r), repo.FullName, branch, "required_pull_request_reviews")
	writeJSON(w, http.StatusOK, rev)
}

func (s *Server) handleBPReviewsPatch(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil {
		s.branchProtectionNotFound(w)
		return
	}
	if bp.RequiredPullRequestReviews == nil {
		bp.RequiredPullRequestReviews = &BPPullRequestReviews{}
	}
	var req BPPullRequestReviews
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.RequiredApprovingReviewCount > 0 || req.DismissStaleReviews || req.RequireCodeOwnerReviews || req.DismissalRestrictions != nil || req.BypassPullRequestAllowances != nil {
		bp.RequiredPullRequestReviews = &req
	} else {
		bp.RequiredPullRequestReviews = nil
	}
	s.setBranchProtection(repo, branch, bp)
	req.URL = s.branchProtectionSubURL(s.baseURL(r), repo.FullName, branch, "required_pull_request_reviews")
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleBPReviewsDelete(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil {
		s.branchProtectionNotFound(w)
		return
	}
	bp.RequiredPullRequestReviews = nil
	s.setBranchProtection(repo, branch, bp)
	w.WriteHeader(http.StatusNoContent)
}

// --- Restrictions ---

func (s *Server) handleBPRestrictionsGet(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.Restrictions == nil {
		s.branchProtectionNotFound(w)
		return
	}
	res := *bp.Restrictions
	s.hydrateRestrictionsURLs(&res, s.baseURL(r), repo.FullName, branch, "restrictions")
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleBPRestrictionsPut(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil {
		s.branchProtectionNotFound(w)
		return
	}
	var req BPRestrictions
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !s.isEmptyRestrictions(&req) {
		bp.Restrictions = &req
	} else {
		bp.Restrictions = nil
	}
	s.setBranchProtection(repo, branch, bp)
	s.hydrateRestrictionsURLs(&req, s.baseURL(r), repo.FullName, branch, "restrictions")
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleBPRestrictionsDelete(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil {
		s.branchProtectionNotFound(w)
		return
	}
	bp.Restrictions = nil
	s.setBranchProtection(repo, branch, bp)
	w.WriteHeader(http.StatusNoContent)
}

// --- Restrictions users ---

func (s *Server) handleBPRestrictionsUsersGet(w http.ResponseWriter, r *http.Request) {
	repo, _, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.Restrictions == nil {
		s.branchProtectionNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, bp.Restrictions.Users)
}

func (s *Server) handleBPRestrictionsUsersPost(w http.ResponseWriter, r *http.Request) {
	s.handleBPRestrictionsUsersPut(w, r)
}

func (s *Server) handleBPRestrictionsUsersPut(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.Restrictions == nil {
		s.branchProtectionNotFound(w)
		return
	}
	var users []BPActor
	if !decodeJSONBody(w, r, &users) {
		return
	}
	bp.Restrictions.Users = users
	s.setBranchProtection(repo, branch, bp)
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) handleBPRestrictionsUsersDelete(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.Restrictions == nil {
		s.branchProtectionNotFound(w)
		return
	}
	bp.Restrictions.Users = nil
	s.setBranchProtection(repo, branch, bp)
	w.WriteHeader(http.StatusNoContent)
}

// --- Restrictions teams ---

func (s *Server) handleBPRestrictionsTeamsGet(w http.ResponseWriter, r *http.Request) {
	repo, _, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.Restrictions == nil {
		s.branchProtectionNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, bp.Restrictions.Teams)
}

func (s *Server) handleBPRestrictionsTeamsPost(w http.ResponseWriter, r *http.Request) {
	s.handleBPRestrictionsTeamsPut(w, r)
}

func (s *Server) handleBPRestrictionsTeamsPut(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.Restrictions == nil {
		s.branchProtectionNotFound(w)
		return
	}
	var teams []BPActor
	if !decodeJSONBody(w, r, &teams) {
		return
	}
	bp.Restrictions.Teams = teams
	s.setBranchProtection(repo, branch, bp)
	writeJSON(w, http.StatusOK, teams)
}

func (s *Server) handleBPRestrictionsTeamsDelete(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.Restrictions == nil {
		s.branchProtectionNotFound(w)
		return
	}
	bp.Restrictions.Teams = nil
	s.setBranchProtection(repo, branch, bp)
	w.WriteHeader(http.StatusNoContent)
}

// --- Enforce admins ---

func (s *Server) handleBPEnforceAdminsGet(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.EnforceAdmins == nil {
		s.branchProtectionNotFound(w)
		return
	}
	ea := *bp.EnforceAdmins
	ea.URL = s.branchProtectionSubURL(s.baseURL(r), repo.FullName, branch, "enforce_admins")
	writeJSON(w, http.StatusOK, ea)
}

func (s *Server) handleBPEnforceAdminsPost(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil {
		s.branchProtectionNotFound(w)
		return
	}
	bp.EnforceAdmins = &BPEnforceAdmins{Enabled: true}
	s.setBranchProtection(repo, branch, bp)
	resp := *bp.EnforceAdmins
	resp.URL = s.branchProtectionSubURL(s.baseURL(r), repo.FullName, branch, "enforce_admins")
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleBPEnforceAdminsDelete(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil {
		s.branchProtectionNotFound(w)
		return
	}
	bp.EnforceAdmins = nil
	s.setBranchProtection(repo, branch, bp)
	w.WriteHeader(http.StatusNoContent)
}

// --- Allow force pushes ---

func (s *Server) handleBPAllowForcePushesGet(w http.ResponseWriter, r *http.Request) {
	repo, _, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.AllowForcePushes == nil {
		s.branchProtectionNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, bp.AllowForcePushes)
}

func (s *Server) handleBPAllowForcePushesPut(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil {
		s.branchProtectionNotFound(w)
		return
	}
	var req BPEnabled
	if !decodeJSONBody(w, r, &req) {
		return
	}
	bp.AllowForcePushes = &req
	s.setBranchProtection(repo, branch, bp)
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleBPAllowForcePushesDelete(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil {
		s.branchProtectionNotFound(w)
		return
	}
	bp.AllowForcePushes = nil
	s.setBranchProtection(repo, branch, bp)
	w.WriteHeader(http.StatusNoContent)
}

// --- Allow deletions ---

func (s *Server) handleBPAllowDeletionsGet(w http.ResponseWriter, r *http.Request) {
	repo, _, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.AllowDeletions == nil {
		s.branchProtectionNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, bp.AllowDeletions)
}

func (s *Server) handleBPAllowDeletionsPut(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil {
		s.branchProtectionNotFound(w)
		return
	}
	var req BPEnabled
	if !decodeJSONBody(w, r, &req) {
		return
	}
	bp.AllowDeletions = &req
	s.setBranchProtection(repo, branch, bp)
	writeJSON(w, http.StatusOK, req)
}

func (s *Server) handleBPAllowDeletionsDelete(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil {
		s.branchProtectionNotFound(w)
		return
	}
	bp.AllowDeletions = nil
	s.setBranchProtection(repo, branch, bp)
	w.WriteHeader(http.StatusNoContent)
}

// --- Required commit signatures ---

func (s *Server) requiredSignaturesJSON(bp *BranchProtection, repo *Repo, branch, baseURL string) map[string]interface{} {
	enabled := bp.RequiredSignatures != nil && bp.RequiredSignatures.Enabled
	return map[string]interface{}{
		"url":     s.branchProtectionSubURL(baseURL, repo.FullName, branch, "required_signatures"),
		"enabled": enabled,
	}
}

func (s *Server) handleBPRequiredSignaturesGet(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil {
		s.branchProtectionNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.requiredSignaturesJSON(bp, repo, branch, s.baseURL(r)))
}

func (s *Server) handleBPRequiredSignaturesPost(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil {
		s.branchProtectionNotFound(w)
		return
	}
	bp.RequiredSignatures = &BPEnabledURL{Enabled: true}
	s.setBranchProtection(repo, branch, bp)
	writeJSON(w, http.StatusOK, s.requiredSignaturesJSON(bp, repo, branch, s.baseURL(r)))
}

func (s *Server) handleBPRequiredSignaturesDelete(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil {
		s.branchProtectionNotFound(w)
		return
	}
	bp.RequiredSignatures = nil
	s.setBranchProtection(repo, branch, bp)
	w.WriteHeader(http.StatusNoContent)
}

// --- Restrictions apps ---

// bpRestrictedAppsJSON renders the restriction's app actors as full GitHub
// App (integration) objects, the shape the restrictions/apps endpoints
// return.
func (s *Server) bpRestrictedAppsJSON(actors []BPActor) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(actors))
	for _, actor := range actors {
		s.store.mu.RLock()
		app := s.store.AppsBySlug[actor.Login]
		s.store.mu.RUnlock()
		if app != nil {
			out = append(out, appToJSON(s.store, app, false))
		}
	}
	return out
}

// decodeBPAppSlugs decodes the {"apps": ["slug", ...]} body shared by the
// restrictions/apps mutation endpoints and resolves every slug to a
// registered GitHub App. Writes a 422 and returns nil when a slug does not
// resolve.
func (s *Server) decodeBPAppSlugs(w http.ResponseWriter, r *http.Request) ([]BPActor, bool) {
	var req struct {
		Apps []string `json:"apps"`
	}
	if !decodeJSONBody(w, r, &req) {
		return nil, false
	}
	actors := make([]BPActor, 0, len(req.Apps))
	for _, slug := range req.Apps {
		s.store.mu.RLock()
		app := s.store.AppsBySlug[slug]
		s.store.mu.RUnlock()
		if app == nil {
			writeGHError(w, http.StatusUnprocessableEntity, "Could not resolve to a GitHub App: "+slug)
			return nil, false
		}
		actors = append(actors, BPActor{Login: app.Slug, ID: app.ID, Type: "App"})
	}
	return actors, true
}

func (s *Server) handleBPRestrictionsAppsGet(w http.ResponseWriter, r *http.Request) {
	repo, _, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.Restrictions == nil {
		s.branchProtectionNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.bpRestrictedAppsJSON(bp.Restrictions.Apps))
}

func (s *Server) handleBPRestrictionsAppsPost(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.Restrictions == nil {
		s.branchProtectionNotFound(w)
		return
	}
	actors, ok := s.decodeBPAppSlugs(w, r)
	if !ok {
		return
	}
	for _, actor := range actors {
		exists := false
		for _, cur := range bp.Restrictions.Apps {
			if cur.ID == actor.ID {
				exists = true
				break
			}
		}
		if !exists {
			bp.Restrictions.Apps = append(bp.Restrictions.Apps, actor)
		}
	}
	s.setBranchProtection(repo, branch, bp)
	writeJSON(w, http.StatusOK, s.bpRestrictedAppsJSON(bp.Restrictions.Apps))
}

func (s *Server) handleBPRestrictionsAppsPut(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.Restrictions == nil {
		s.branchProtectionNotFound(w)
		return
	}
	actors, ok := s.decodeBPAppSlugs(w, r)
	if !ok {
		return
	}
	bp.Restrictions.Apps = actors
	s.setBranchProtection(repo, branch, bp)
	writeJSON(w, http.StatusOK, s.bpRestrictedAppsJSON(bp.Restrictions.Apps))
}

func (s *Server) handleBPRestrictionsAppsDelete(w http.ResponseWriter, r *http.Request) {
	repo, branch, bp := s.getBranchProtection(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if bp == nil || bp.Restrictions == nil {
		s.branchProtectionNotFound(w)
		return
	}
	actors, ok := s.decodeBPAppSlugs(w, r)
	if !ok {
		return
	}
	remaining := bp.Restrictions.Apps[:0]
	for _, cur := range bp.Restrictions.Apps {
		removed := false
		for _, actor := range actors {
			if cur.ID == actor.ID {
				removed = true
				break
			}
		}
		if !removed {
			remaining = append(remaining, cur)
		}
	}
	bp.Restrictions.Apps = remaining
	s.setBranchProtection(repo, branch, bp)
	writeJSON(w, http.StatusOK, s.bpRestrictedAppsJSON(bp.Restrictions.Apps))
}

// --- Helpers ---

// decodeStringArrayBody decodes either a bare JSON array or {"contexts":[...]}.
func decodeStringArrayBody(w http.ResponseWriter, r *http.Request, out *[]string) bool {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return false
	}
	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) == 0 {
		*out = nil
		return true
	}
	if trimmed[0] == '[' {
		if err := json.Unmarshal([]byte(trimmed), out); err != nil {
			writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
			return false
		}
		return true
	}
	var obj struct {
		Contexts []string `json:"contexts"`
	}
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
		return false
	}
	*out = obj.Contexts
	return true
}

// branchProtectionForRepo returns the protection rule for a repo+branch, or nil.
func (s *Server) branchProtectionForRepo(repoID int, branch string) *BranchProtection {
	s.store.Misc.mu.RLock()
	defer s.store.Misc.mu.RUnlock()
	return s.store.Misc.branchProtection[bpKey(repoID, branch)]
}

// canMergePullRequest checks branch protection rules for a PR merge.
// It returns (ok, errorMessage). ok==false with empty message means the caller
// should fall back to the existing required-status-check message.
func (s *Server) canMergePullRequest(repo *Repo, pr *PullRequest, user *User) (bool, string) {
	bp := s.branchProtectionForRepo(repo.ID, pr.BaseRefName)
	if bp == nil {
		return true, ""
	}

	isAdmin := canAdminRepo(s.store, user, repo)

	// Admin bypass only when enforce_admins is not enabled.
	if isAdmin && (bp.EnforceAdmins == nil || !bp.EnforceAdmins.Enabled) {
		return true, ""
	}

	// Required status checks
	headSha := s.prHeadSha(repo, pr)
	if headSha != "" {
		st := s.evaluateChecksForMerge(repo, pr.BaseRefName, headSha)
		if len(st.MissingRequired) > 0 {
			return false, fmt.Sprintf("Required status check %q is expected.", st.MissingRequired[0])
		}
	}

	// Required approving review count
	if bp.RequiredPullRequestReviews != nil && bp.RequiredPullRequestReviews.RequiredApprovingReviewCount > 0 {
		count := s.countApprovingReviews(pr.ID)
		if count < bp.RequiredPullRequestReviews.RequiredApprovingReviewCount {
			return false, fmt.Sprintf("At least %d approving review is required by the branch protection rules.", bp.RequiredPullRequestReviews.RequiredApprovingReviewCount)
		}
	}

	// Requested changes block merge
	if s.hasRequestedChanges(pr.ID) {
		return false, "Changes have been requested on this pull request."
	}

	return true, ""
}

func (s *Server) countApprovingReviews(prID int) int {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	latestByUser := map[int]string{}
	for _, rev := range s.store.PRReviews {
		if rev.PRID != prID {
			continue
		}
		latestByUser[rev.AuthorID] = rev.State
	}
	count := 0
	for _, state := range latestByUser {
		if state == "APPROVED" {
			count++
		}
	}
	return count
}

func (s *Server) hasRequestedChanges(prID int) bool {
	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	latestByUser := map[int]string{}
	for _, rev := range s.store.PRReviews {
		if rev.PRID != prID {
			continue
		}
		latestByUser[rev.AuthorID] = rev.State
	}
	for _, state := range latestByUser {
		if state == "CHANGES_REQUESTED" {
			return true
		}
	}
	return false
}

// requiredCheckContexts returns the base branch's protected status-check
// contexts from the typed model.
func (s *Server) requiredCheckContexts(repoID int, baseBranch string) []string {
	bp := s.branchProtectionForRepo(repoID, baseBranch)
	if bp == nil || bp.RequiredStatusChecks == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	for _, c := range bp.RequiredStatusChecks.Contexts {
		add(c)
	}
	for _, c := range bp.RequiredStatusChecks.Checks {
		add(c.Context)
	}
	return out
}

// branchProtectionRuleForPR returns a GraphQL-shaped map for baseRef.branchProtectionRule.
func (s *Server) branchProtectionRuleForPR(repo *Repo, baseBranch string) map[string]interface{} {
	bp := s.branchProtectionForRepo(repo.ID, baseBranch)
	if bp == nil {
		return nil
	}
	strict := false
	count := 0
	if bp.RequiredStatusChecks != nil {
		strict = bp.RequiredStatusChecks.Strict
	}
	if bp.RequiredPullRequestReviews != nil {
		count = bp.RequiredPullRequestReviews.RequiredApprovingReviewCount
	}
	return map[string]interface{}{
		"requiresStrictStatusChecks":   strict,
		"requiredApprovingReviewCount": count,
	}
}
