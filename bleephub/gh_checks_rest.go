package bleephub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Checks API.
// CheckRun + CheckSuite are App-owned: real GitHub limits Create/Update to
// GitHub App installation tokens. Bleephub permission-gates by "checks"
// scope (read for reads, write for create/update).

func (s *Server) registerGHChecksRoutes() {
	s.route("POST /api/v3/repos/{owner}/{repo}/check-runs", s.requirePerm(scopeChecks, permWrite, s.handleCreateCheckRun))
	s.route("GET /api/v3/repos/{owner}/{repo}/check-runs/{id}", s.requirePerm(scopeChecks, permRead, s.handleGetCheckRun))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/check-runs/{id}", s.requirePerm(scopeChecks, permWrite, s.handleUpdateCheckRun))
	s.route("GET /api/v3/repos/{owner}/{repo}/check-runs/{id}/annotations", s.requirePerm(scopeChecks, permRead, s.handleListCheckRunAnnotations))
	s.route("GET /api/v3/repos/{owner}/{repo}/commits/{sha}/check-runs", s.requirePerm(scopeChecks, permRead, s.handleListCheckRunsForCommit))
	s.route("GET /api/v3/repos/{owner}/{repo}/commits/{sha}/check-suites", s.requirePerm(scopeChecks, permRead, s.handleListCheckSuitesForCommit))
	s.route("POST /api/v3/repos/{owner}/{repo}/check-suites", s.requirePerm(scopeChecks, permWrite, s.handleCreateCheckSuite))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/check-suites/preferences", s.requirePerm(scopeAdministration, permWrite, s.handleUpdateCheckSuitePrefs))
	s.route("GET /api/v3/repos/{owner}/{repo}/check-suites/{id}", s.requirePerm(scopeChecks, permRead, s.handleGetCheckSuite))
	s.route("GET /api/v3/repos/{owner}/{repo}/check-suites/{id}/check-runs", s.requirePerm(scopeChecks, permRead, s.handleListCheckRunsForSuite))
	s.route("POST /api/v3/repos/{owner}/{repo}/check-runs/{id}/rerequest", s.requirePerm(scopeChecks, permWrite, s.handleRerequestCheckRun))
	s.route("POST /api/v3/repos/{owner}/{repo}/check-suites/{id}/rerequest", s.requirePerm(scopeChecks, permWrite, s.handleRerequestCheckSuite))
}

// handleRerequestCheckRun resets a completed check run to queued and fires
// the check_run "rerequested" webhook, asking the owning app to run it again.
func (s *Server) handleRerequestCheckRun(w http.ResponseWriter, r *http.Request) {
	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	cr := s.store.GetCheckRun(id)
	if cr == nil || cr.RepoKey != repoKey {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if cr.Status != "completed" {
		writeGHError(w, http.StatusUnprocessableEntity, "This check run is not yet completed and cannot be rerequested.")
		return
	}
	s.store.UpdateCheckRun(id, func(c *CheckRun) {
		c.Status = "queued"
		c.Conclusion = ""
		c.CompletedAt = nil
	})
	s.emitCheckRunEvent(repoKey, id, "rerequested")
	writeJSON(w, http.StatusCreated, map[string]interface{}{})
}

// handleRerequestCheckSuite marks a check suite queued and fires the
// check_suite "rerequested" webhook, asking apps to re-create their runs.
func (s *Server) handleRerequestCheckSuite(w http.ResponseWriter, r *http.Request) {
	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	suite := s.store.GetCheckSuite(id)
	if suite == nil || suite.RepoKey != repoKey {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	s.store.UpdateCheckSuite(id, func(cs *CheckSuite) {
		cs.Status = "queued"
		cs.Conclusion = ""
	})
	s.emitCheckSuiteEvent(repoKey, id, "rerequested")
	writeJSON(w, http.StatusCreated, map[string]interface{}{})
}

func (s *Server) handleCreateCheckRun(w http.ResponseWriter, r *http.Request) {
	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	repo := s.store.GetRepo(r.PathValue("owner"), r.PathValue("repo"))
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Name        string          `json:"name"`
		HeadSHA     string          `json:"head_sha"`
		Status      string          `json:"status"`
		Conclusion  string          `json:"conclusion"`
		ExternalID  string          `json:"external_id"`
		DetailsURL  string          `json:"details_url"`
		StartedAt   *time.Time      `json:"started_at"`
		CompletedAt *time.Time      `json:"completed_at"`
		Output      *CheckRunOutput `json:"output"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeGHValidationError(w, "CheckRun", "name", "missing_field")
		return
	}
	if req.HeadSHA == "" {
		writeGHValidationError(w, "CheckRun", "head_sha", "missing_field")
		return
	}
	appID := appIDFromContext(r.Context())
	cr := s.store.CreateCheckRun(repoKey, req.HeadSHA, req.Name, appID, 0)
	s.store.UpdateCheckRun(cr.ID, func(c *CheckRun) {
		if req.Status != "" {
			c.Status = req.Status
		}
		if req.Conclusion != "" {
			c.Conclusion = req.Conclusion
		}
		c.ExternalID = req.ExternalID
		c.DetailsURL = req.DetailsURL
		if req.StartedAt != nil {
			c.StartedAt = *req.StartedAt
		}
		if req.CompletedAt != nil {
			c.CompletedAt = req.CompletedAt
		}
		if req.Output != nil {
			c.Output = req.Output
			c.Output.AnnotationsCount = len(req.Output.Annotations)
		}
	})
	user := ghUserFromContext(r.Context())
	actor := ""
	if user != nil {
		actor = user.Login
	}
	s.recordAuditEvent("check_run.create", actor, "", map[string]interface{}{"repo": repoKey, "check_run_id": cr.ID})
	writeJSON(w, http.StatusCreated, s.checkRunToJSON(s.store.GetCheckRun(cr.ID), s.baseURL(r)))
}

func (s *Server) handleGetCheckRun(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	cr := s.store.GetCheckRun(id)
	if cr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.checkRunToJSON(cr, s.baseURL(r)))
}

func (s *Server) handleUpdateCheckRun(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	var req struct {
		Name        *string         `json:"name"`
		Status      *string         `json:"status"`
		Conclusion  *string         `json:"conclusion"`
		DetailsURL  *string         `json:"details_url"`
		ExternalID  *string         `json:"external_id"`
		StartedAt   *time.Time      `json:"started_at"`
		CompletedAt *time.Time      `json:"completed_at"`
		Output      *CheckRunOutput `json:"output"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	found := s.store.UpdateCheckRun(id, func(cr *CheckRun) {
		if req.Name != nil {
			cr.Name = *req.Name
		}
		if req.Status != nil {
			cr.Status = *req.Status
		}
		if req.Conclusion != nil {
			cr.Conclusion = *req.Conclusion
		}
		if req.DetailsURL != nil {
			cr.DetailsURL = *req.DetailsURL
		}
		if req.ExternalID != nil {
			cr.ExternalID = *req.ExternalID
		}
		if req.StartedAt != nil {
			cr.StartedAt = *req.StartedAt
		}
		if req.CompletedAt != nil {
			cr.CompletedAt = req.CompletedAt
		}
		if req.Output != nil {
			cr.Output = req.Output
			cr.Output.AnnotationsCount = len(req.Output.Annotations)
		}
	})
	if !found {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.checkRunToJSON(s.store.GetCheckRun(id), s.baseURL(r)))
}

func (s *Server) handleListCheckRunAnnotations(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	cr := s.store.GetCheckRun(id)
	if cr == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	out := []*CheckAnnotation{}
	if cr.Output != nil {
		out = cr.Output.Annotations
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListCheckRunsForCommit(w http.ResponseWriter, r *http.Request) {
	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	sha := r.PathValue("sha")
	q := r.URL.Query()
	status := q.Get("status")
	conclusion := q.Get("filter")
	appID, _ := strconv.Atoi(q.Get("app_id"))
	runs := s.store.ListCheckRunsForCommit(repoKey, sha, status, conclusion, appID)
	page := paginateAndLink(w, r, runs)
	out := make([]map[string]interface{}, 0, len(page))
	for _, cr := range page {
		out = append(out, s.checkRunToJSON(cr, s.baseURL(r)))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": len(runs),
		"check_runs":  out,
	})
}

func (s *Server) handleListCheckSuitesForCommit(w http.ResponseWriter, r *http.Request) {
	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	sha := r.PathValue("sha")
	suites := s.store.ListCheckSuitesForCommit(repoKey, sha, 0)
	out := make([]map[string]interface{}, 0, len(suites))
	for _, ss := range suites {
		out = append(out, s.checkSuiteToJSON(ss, s.baseURL(r)))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count":  len(suites),
		"check_suites": out,
	})
}

func (s *Server) handleCreateCheckSuite(w http.ResponseWriter, r *http.Request) {
	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	var req struct {
		HeadSHA string `json:"head_sha"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.HeadSHA == "" {
		writeGHValidationError(w, "CheckSuite", "head_sha", "missing_field")
		return
	}
	appID := appIDFromContext(r.Context())
	suite := s.store.CreateCheckSuite(repoKey, "", req.HeadSHA, appID)
	writeJSON(w, http.StatusCreated, s.checkSuiteToJSON(suite, s.baseURL(r)))
}

func (s *Server) handleUpdateCheckSuitePrefs(w http.ResponseWriter, r *http.Request) {
	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	var req struct {
		AutoTriggerChecks []*CheckSuitePref `json:"auto_trigger_checks"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	s.store.SetCheckSuitePreferences(repoKey, req.AutoTriggerChecks)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"preferences": map[string]interface{}{
			"auto_trigger_checks": req.AutoTriggerChecks,
		},
	})
}

func (s *Server) handleGetCheckSuite(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	suite := s.store.GetCheckSuite(id)
	if suite == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, s.checkSuiteToJSON(suite, s.baseURL(r)))
}

func (s *Server) handleListCheckRunsForSuite(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	runs := s.store.ListCheckRunsForSuite(id)
	page := paginateAndLink(w, r, runs)
	out := make([]map[string]interface{}, 0, len(page))
	for _, cr := range page {
		out = append(out, s.checkRunToJSON(cr, s.baseURL(r)))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_count": len(runs),
		"check_runs":  out,
	})
}

// appIDFromContext returns the AppID associated with the request's auth.
// Returns 0 for PAT auth (no App context).
func appIDFromContext(ctx interface {
	Value(any) any
},
) int {
	if t, _ := ctx.Value(ctxInstallationToken).(*InstallationToken); t != nil {
		return t.AppID
	}
	if t, _ := ctx.Value(ctxUserToServerToken).(*UserToServerToken); t != nil {
		return t.AppID
	}
	if a, _ := ctx.Value(ctxApp).(*App); a != nil {
		return a.ID
	}
	return 0
}

// checkAppJSON renders the check's owning GitHub App as the integration
// shape, or null for PAT-created checks (AppID 0), matching real GitHub's
// nullable app member.
func (s *Server) checkAppJSON(appID int) interface{} {
	if appID == 0 {
		return nil
	}
	app := s.store.GetApp(appID)
	if app == nil {
		return nil
	}
	return appToJSON(s.store, app, false)
}

// checkRunToJSON renders the GitHub check-run shape. base is the external
// base URL ("" for webhook payloads, which carry relative API paths like the
// other event payload builders).
func (s *Server) checkRunToJSON(cr *CheckRun, base string) map[string]interface{} {
	if cr == nil {
		return nil
	}
	api := base + "/api/v3/repos/" + cr.RepoKey
	var conclusion interface{}
	if cr.Conclusion != "" {
		conclusion = cr.Conclusion
	}
	var completedAt interface{}
	if cr.CompletedAt != nil {
		completedAt = cr.CompletedAt.UTC().Format(time.RFC3339)
	}
	output := map[string]interface{}{
		"title":             nil,
		"summary":           nil,
		"text":              nil,
		"annotations_count": 0,
		"annotations_url":   fmt.Sprintf("%s/check-runs/%d/annotations", api, cr.ID),
	}
	if cr.Output != nil {
		output["title"] = nilOrString(cr.Output.Title)
		output["summary"] = nilOrString(cr.Output.Summary)
		output["text"] = nilOrString(cr.Output.Text)
		output["annotations_count"] = cr.Output.AnnotationsCount
	}
	return map[string]interface{}{
		"id":            cr.ID,
		"node_id":       cr.NodeID,
		"head_sha":      cr.HeadSHA,
		"name":          cr.Name,
		"status":        cr.Status,
		"conclusion":    conclusion,
		"started_at":    cr.StartedAt.UTC().Format(time.RFC3339),
		"completed_at":  completedAt,
		"external_id":   cr.ExternalID,
		"url":           fmt.Sprintf("%s/check-runs/%d", api, cr.ID),
		"html_url":      fmt.Sprintf("%s/%s/runs/%d", base, cr.RepoKey, cr.ID),
		"details_url":   cr.DetailsURL,
		"app":           s.checkAppJSON(cr.AppID),
		"check_suite":   map[string]interface{}{"id": cr.SuiteID},
		"output":        output,
		"pull_requests": []interface{}{},
	}
}

// checkSuiteToJSON renders the GitHub check-suite shape, resolving the head
// commit from the repository's real git storage and embedding the repository
// as a minimal-repository.
func (s *Server) checkSuiteToJSON(suite *CheckSuite, base string) map[string]interface{} {
	if suite == nil {
		return nil
	}
	api := base + "/api/v3/repos/" + suite.RepoKey
	var conclusion interface{}
	if suite.Conclusion != "" {
		conclusion = suite.Conclusion
	}
	var headBranch interface{}
	if suite.HeadBranch != "" {
		headBranch = suite.HeadBranch
	}

	var repository interface{}
	var headCommit interface{}
	if owner, name, ok := splitRepoFullName(suite.RepoKey); ok {
		if repo := s.store.GetRepo(owner, name); repo != nil {
			repository = repoToJSON(repo, s.store, base)
		}
		if stor := s.store.GetGitStorage(owner, name); stor != nil {
			if commit, err := object.GetCommit(stor, plumbing.NewHash(suite.HeadSHA)); err == nil {
				headCommit = map[string]interface{}{
					"id":        commit.Hash.String(),
					"tree_id":   commit.TreeHash.String(),
					"message":   strings.TrimSpace(commit.Message),
					"timestamp": commit.Committer.When.UTC().Format(time.RFC3339),
					"author": map[string]interface{}{
						"name":  commit.Author.Name,
						"email": commit.Author.Email,
					},
					"committer": map[string]interface{}{
						"name":  commit.Committer.Name,
						"email": commit.Committer.Email,
					},
				}
			}
		}
	}

	return map[string]interface{}{
		"id":                      suite.ID,
		"node_id":                 suite.NodeID,
		"head_branch":             headBranch,
		"head_sha":                suite.HeadSHA,
		"status":                  suite.Status,
		"conclusion":              conclusion,
		"url":                     fmt.Sprintf("%s/check-suites/%d", api, suite.ID),
		"before":                  nil,
		"after":                   nil,
		"pull_requests":           []interface{}{},
		"app":                     s.checkAppJSON(suite.AppID),
		"repository":              repository,
		"head_commit":             headCommit,
		"latest_check_runs_count": len(s.store.ListCheckRunsForSuite(suite.ID)),
		"check_runs_url":          fmt.Sprintf("%s/check-suites/%d/check-runs", api, suite.ID),
		"created_at":              suite.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":              suite.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
