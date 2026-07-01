package bleephub

import (
	"fmt"
	"net/http"
	"strconv"
)

func (s *Server) registerGHCodeScanningRoutes() {
	// Alerts
	s.route("GET /api/v3/repos/{owner}/{repo}/code-scanning/alerts", s.handleListCodeScanningAlerts)
	s.route("GET /api/v3/repos/{owner}/{repo}/code-scanning/alerts/{alert_number}", s.handleGetCodeScanningAlert)
	s.route("PATCH /api/v3/repos/{owner}/{repo}/code-scanning/alerts/{alert_number}", s.handleUpdateCodeScanningAlert)
	s.route("GET /api/v3/repos/{owner}/{repo}/code-scanning/alerts/{alert_number}/instances", s.handleListCodeScanningAlertInstances)

	// Analyses
	s.route("GET /api/v3/repos/{owner}/{repo}/code-scanning/analyses", s.handleListCodeScanningAnalyses)
	s.route("GET /api/v3/repos/{owner}/{repo}/code-scanning/analyses/{analysis_id}", s.handleGetCodeScanningAnalysis)
	s.route("DELETE /api/v3/repos/{owner}/{repo}/code-scanning/analyses/{analysis_id}", s.handleDeleteCodeScanningAnalysis)

	// SARIF upload
	s.route("POST /api/v3/repos/{owner}/{repo}/code-scanning/sarifs", s.handleCreateSARIFUpload)
	s.route("GET /api/v3/repos/{owner}/{repo}/code-scanning/sarifs/{sarif_id}", s.handleGetSARIFUpload)

	// Default setup
	s.route("GET /api/v3/repos/{owner}/{repo}/code-scanning/default-setup", s.handleGetCodeScanningDefaultSetup)
	s.route("PATCH /api/v3/repos/{owner}/{repo}/code-scanning/default-setup", s.handleUpdateCodeScanningDefaultSetup)

	// Internal seeding endpoint: real GitHub creates alerts by uploading SARIF.
	s.route("POST /internal/repos/{owner}/{repo}/code-scanning/alerts", s.handleSeedCodeScanningAlert)
}

func (s *Server) handleListCodeScanningAlerts(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}

	q := r.URL.Query()
	state := q.Get("state")
	severity := q.Get("severity")
	toolName := q.Get("tool_name")
	rule := q.Get("rule")
	sort := q.Get("sort")
	direction := q.Get("direction")

	alerts := s.store.ListCodeScanningAlerts(repo.FullName, state, severity, toolName, rule, sort, direction)
	page := paginateAndLink(w, r, alerts)
	baseURL := s.baseURL(r)
	out := make([]map[string]interface{}, len(page))
	for i, a := range page {
		out[i] = codeScanningAlertToJSON(a, baseURL, repo)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetCodeScanningAlert(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}

	a := s.lookupCodeScanningAlert(w, r, repo)
	if a == nil {
		return
	}
	writeJSON(w, http.StatusOK, codeScanningAlertToJSON(a, s.baseURL(r), repo))
}

func (s *Server) handleUpdateCodeScanningAlert(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	a := s.lookupCodeScanningAlert(w, r, repo)
	if a == nil {
		return
	}

	var req struct {
		State            string `json:"state"`
		DismissedReason  string `json:"dismissed_reason"`
		DismissedComment string `json:"dismissed_comment"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.State == "" {
		writeGHValidationError(w, "CodeScanningAlert", "state", "missing_field")
		return
	}
	if err := s.store.UpdateCodeScanningAlert(a, req.State, req.DismissedReason, req.DismissedComment); err != nil {
		writeGHValidationError(w, "CodeScanningAlert", "state", "invalid")
		return
	}
	writeJSON(w, http.StatusOK, codeScanningAlertToJSON(a, s.baseURL(r), repo))
}

func (s *Server) handleListCodeScanningAlertInstances(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}

	a := s.lookupCodeScanningAlert(w, r, repo)
	if a == nil {
		return
	}
	out := make([]map[string]interface{}, len(a.Instances))
	for i, inst := range a.Instances {
		out[i] = codeScanningInstanceToJSON(inst)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListCodeScanningAnalyses(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}

	q := r.URL.Query()
	ref := q.Get("ref")
	toolName := q.Get("tool_name")

	analyses := s.store.ListCodeScanningAnalyses(repo.FullName, ref, toolName)
	page := paginateAndLink(w, r, analyses)
	baseURL := s.baseURL(r)
	out := make([]map[string]interface{}, len(page))
	for i, a := range page {
		out[i] = codeScanningAnalysisToJSON(a, baseURL, repo)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetCodeScanningAnalysis(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}

	id, err := strconv.Atoi(r.PathValue("analysis_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	a := s.store.GetCodeScanningAnalysis(repo.FullName, id)
	if a == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, codeScanningAnalysisToJSON(a, s.baseURL(r), repo))
}

func (s *Server) handleDeleteCodeScanningAnalysis(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	id, err := strconv.Atoi(r.PathValue("analysis_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.store.DeleteCodeScanningAnalysis(repo.FullName, id) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCreateSARIFUpload(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req map[string]interface{}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	upload, err := s.store.CreateSARIFUpload(repo.FullName, req)
	if err != nil {
		writeGHValidationError(w, "SARIFUpload", "sarif", "invalid")
		return
	}
	baseURL := s.baseURL(r)
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"id":  upload.ID,
		"url": fmt.Sprintf("%s/api/v3/repos/%s/code-scanning/sarifs/%s", baseURL, repo.FullName, upload.ID),
	})
}

func (s *Server) handleGetSARIFUpload(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}

	id := r.PathValue("sarif_id")
	upload := s.store.GetSARIFUpload(repo.FullName, id)
	if upload == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	baseURL := s.baseURL(r)
	var analysesURL interface{} = nil
	if upload.Status == "complete" {
		analysesURL = fmt.Sprintf("%s/api/v3/repos/%s/code-scanning/analyses", baseURL, repo.FullName)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"processing_status": upload.Status,
		"analyses_url":      analysesURL,
		"errors":            upload.Errors,
	})
}

func (s *Server) handleGetCodeScanningDefaultSetup(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"state":       "configured",
		"languages":   []string{"go", "python", "javascript-typescript"},
		"schedule":    "weekly",
		"query_suite": "default",
		"updated_at":  repo.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (s *Server) handleUpdateCodeScanningDefaultSetup(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !canAdminRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		State      string   `json:"state"`
		QuerySuite string   `json:"query_suite"`
		Languages  []string `json:"languages"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	_ = req
	writeJSON(w, http.StatusOK, map[string]interface{}{})
}

func (s *Server) handleSeedCodeScanningAlert(w http.ResponseWriter, r *http.Request) {
	user := s.internalTokenUser(r)
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "Requires authentication"})
		return
	}
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}

	var req struct {
		RuleID          string                      `json:"rule_id"`
		RuleSeverity    string                      `json:"rule_severity"`
		RuleDescription string                      `json:"rule_description"`
		ToolName        string                      `json:"tool_name"`
		State           string                      `json:"state"`
		Instances       []CodeScanningAlertInstance `json:"instances"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.RuleID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "rule_id is required"})
		return
	}

	a := s.store.CreateCodeScanningAlert(repo.FullName, req.RuleID, req.RuleSeverity, req.RuleDescription, req.ToolName, req.State, req.Instances)
	writeJSON(w, http.StatusCreated, codeScanningAlertToJSON(a, s.baseURL(r), repo))
}

func (s *Server) lookupCodeScanningAlert(w http.ResponseWriter, r *http.Request, repo *Repo) *CodeScanningAlert {
	number, err := strconv.Atoi(r.PathValue("alert_number"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	a := s.store.GetCodeScanningAlert(repo.FullName, number)
	if a == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return a
}

func codeScanningAlertToJSON(a *CodeScanningAlert, baseURL string, repo *Repo) map[string]interface{} {
	apiURL := fmt.Sprintf("%s/api/v3/repos/%s/code-scanning/alerts/%d", baseURL, repo.FullName, a.Number)
	htmlURL := fmt.Sprintf("%s/%s/security/code-scanning/%d", baseURL, repo.FullName, a.Number)
	instancesURL := fmt.Sprintf("%s/instances", apiURL)

	var dismissedBy interface{} = nil
	var dismissedAt interface{} = nil
	var fixedAt interface{} = nil
	if a.DismissedAt != nil {
		dismissedAt = a.DismissedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	if a.FixedAt != nil {
		fixedAt = a.FixedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}

	var mostRecent map[string]interface{} = nil
	if len(a.Instances) > 0 {
		mostRecent = codeScanningInstanceToJSON(a.Instances[len(a.Instances)-1])
	}

	return map[string]interface{}{
		"number":            a.Number,
		"created_at":        a.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":        a.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"url":               apiURL,
		"html_url":          htmlURL,
		"instances_url":     instancesURL,
		"state":             a.State,
		"fixed_at":          fixedAt,
		"dismissed_by":      dismissedBy,
		"dismissed_at":      dismissedAt,
		"dismissed_reason":  nullOrString(a.DismissedReason),
		"dismissed_comment": nullOrString(a.DismissedComment),
		"rule": map[string]interface{}{
			"id":          a.RuleID,
			"severity":    nullOrString(a.RuleSeverity),
			"description": nullOrString(a.RuleDescription),
			"name":        a.RuleID,
		},
		"tool": map[string]interface{}{
			"name":    nullOrString(a.ToolName),
			"guid":    nil,
			"version": nil,
		},
		"most_recent_instance": mostRecent,
	}
}

func codeScanningInstanceToJSON(inst CodeScanningAlertInstance) map[string]interface{} {
	return map[string]interface{}{
		"ref":          inst.Ref,
		"analysis_key": inst.AnalysisKey,
		"category":     inst.Category,
		"state":        inst.State,
		"commit_sha":   inst.CommitSHA,
		"message": map[string]interface{}{
			"text": inst.Message,
		},
		"location": map[string]interface{}{
			"path":         inst.Path,
			"start_line":   inst.StartLine,
			"end_line":     inst.EndLine,
			"start_column": inst.StartColumn,
			"end_column":   inst.EndColumn,
		},
	}
}

func codeScanningAnalysisToJSON(a *CodeScanningAnalysis, baseURL string, repo *Repo) map[string]interface{} {
	apiURL := fmt.Sprintf("%s/api/v3/repos/%s/code-scanning/analyses/%d", baseURL, repo.FullName, a.ID)
	return map[string]interface{}{
		"ref":           a.Ref,
		"commit_sha":    a.CommitSHA,
		"analysis_key":  a.AnalysisKey,
		"environment":   "",
		"category":      a.Category,
		"error":         "",
		"created_at":    a.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"results_count": a.ResultsCount,
		"rules_count":   a.RulesCount,
		"id":            a.ID,
		"url":           apiURL,
		"sarif_id":      a.SARIFUploadID,
		"tool": map[string]interface{}{
			"name":    nullOrString(a.ToolName),
			"guid":    nil,
			"version": nil,
		},
		"deletable": true,
		"warning":   "",
	}
}
