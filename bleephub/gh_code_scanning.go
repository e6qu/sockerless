package bleephub

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"gopkg.in/yaml.v3"
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
	s.route("POST /api/v3/repos/{owner}/{repo}/code-scanning/sarifs",
		s.requirePerm(scopeSecurityEvents, permWrite, s.handleCreateSARIFUpload))
	s.route("GET /api/v3/repos/{owner}/{repo}/code-scanning/sarifs/{sarif_id}",
		s.requirePerm(scopeSecurityEvents, permRead, s.handleGetSARIFUpload))

	// Default setup
	s.route("GET /api/v3/repos/{owner}/{repo}/code-scanning/default-setup", s.handleGetCodeScanningDefaultSetup)
	s.route("PATCH /api/v3/repos/{owner}/{repo}/code-scanning/default-setup", s.handleUpdateCodeScanningDefaultSetup)

	// Copilot Autofix
	s.route("GET /api/v3/repos/{owner}/{repo}/code-scanning/alerts/{alert_number}/autofix", s.handleGetCodeScanningAutofix)
	s.route("POST /api/v3/repos/{owner}/{repo}/code-scanning/alerts/{alert_number}/autofix", s.handleCreateCodeScanningAutofix)
	s.route("POST /api/v3/repos/{owner}/{repo}/code-scanning/alerts/{alert_number}/autofix/commits", s.handleCommitCodeScanningAutofix)

	// CodeQL databases
	s.route("POST /repos/{owner}/{repo}/code-scanning/codeql/databases/{language}",
		s.requirePerm(scopeSecurityEvents, permWrite, s.handleUploadCodeQLDatabase))
	s.route("GET /api/v3/repos/{owner}/{repo}/code-scanning/codeql/databases",
		s.requirePerm(scopeContents, permRead, s.handleListCodeQLDatabases))
	s.route("GET /api/v3/repos/{owner}/{repo}/code-scanning/codeql/databases/{language}",
		s.requirePerm(scopeContents, permRead, s.handleGetCodeQLDatabase))
	s.route("DELETE /api/v3/repos/{owner}/{repo}/code-scanning/codeql/databases/{language}",
		s.requirePerm(scopeContents, permWrite, s.handleDeleteCodeQLDatabase))
	s.route("GET /code-scanning/repos/{owner}/{repo}/codeql/databases/{language}/download", s.handleDownloadCodeQLDatabase)

	// CodeQL variant analyses
	s.route("POST /api/v3/repos/{owner}/{repo}/code-scanning/codeql/variant-analyses", s.handleCreateCodeQLVariantAnalysis)
	s.route("GET /api/v3/repos/{owner}/{repo}/code-scanning/codeql/variant-analyses/{codeql_variant_analysis_id}", s.handleGetCodeQLVariantAnalysis)
	s.route("GET /api/v3/repos/{owner}/{repo}/code-scanning/codeql/variant-analyses/{codeql_variant_analysis_id}/repos/{repo_owner}/{repo_name}", s.handleGetCodeQLVariantAnalysisRepoTask)
	s.route("GET /code-scanning/repos/{owner}/{repo}/codeql/variant-analyses/{codeql_variant_analysis_id}/query-pack", s.handleDownloadCodeQLVariantAnalysisQueryPack)

	// Organization alerts
	s.route("GET /api/v3/orgs/{org}/code-scanning/alerts",
		s.requireOrgAdmin(scopeSecurityEvents, permRead, s.handleListOrgCodeScanningAlerts))

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
	if !s.codeScanningRequestCanWriteRepo(r, repo) {
		writeGHError(w, http.StatusForbidden, "Must have push access to Repository.")
		return
	}

	var req map[string]interface{}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	commitSHA, _ := req["commit_sha"].(string)
	ref, _ := req["ref"].(string)
	if err := validateCodeScanningRef(ref); err != nil {
		writeGHValidationError(w, "SARIFUpload", "ref", "invalid")
		return
	}
	if err := s.validateCodeScanningCommit(repo, commitSHA); err != nil {
		writeGHValidationError(w, "SARIFUpload", "commit_sha", "invalid")
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

// codeScanningRequestCanWriteRepo accepts both user credentials with push
// access and a repository-selected GitHub App installation token. GitHub
// Actions' built-in token is installation-shaped rather than a human
// collaborator, so reducing authorization to canPushRepo would reject the
// official CodeQL producer even when its permission gate passed.
func (s *Server) codeScanningRequestCanWriteRepo(r *http.Request, repo *Repo) bool {
	if canPushRepo(s.store, ghUserFromContext(r.Context()), repo) {
		return true
	}
	return installationRequestCanAccessRepo(r, repo)
}

func installationRequestCanAccessRepo(r *http.Request, repo *Repo) bool {
	inst := ghInstallationFromContext(r.Context())
	token := ghInstallationTokenFromContext(r.Context())
	if inst == nil || token == nil || repo == nil {
		return false
	}
	return len(filterReposBySelection([]*Repo{repo}, inst, token)) == 1
}

func (s *Server) codeScanningRequestCanReadRepo(r *http.Request, repo *Repo) bool {
	return canReadRepo(s.store, ghUserFromContext(r.Context()), repo) || installationRequestCanAccessRepo(r, repo)
}

func (s *Server) validateCodeScanningCoordinate(repo *Repo, commitSHA, ref string) error {
	if err := validateCodeScanningRef(ref); err != nil {
		return err
	}
	return s.validateCodeScanningCommit(repo, commitSHA)
}

func validateCodeScanningRef(ref string) error {
	if !strings.HasPrefix(ref, "refs/heads/") && !strings.HasPrefix(ref, "refs/tags/") && !strings.HasPrefix(ref, "refs/pull/") {
		return fmt.Errorf("ref must be a fully-qualified Git reference")
	}
	if err := plumbing.ReferenceName(ref).Validate(); err != nil {
		return fmt.Errorf("ref is invalid: %w", err)
	}
	return nil
}

func (s *Server) validateCodeScanningCommit(repo *Repo, commitSHA string) error {
	if repo == nil || len(commitSHA) != 40 {
		return fmt.Errorf("commit_sha must be a full commit object ID")
	}
	if _, err := hex.DecodeString(commitSHA); err != nil {
		return fmt.Errorf("commit_sha must be hexadecimal: %w", err)
	}
	stor := s.gitStorageForRepo(repo)
	if stor == nil {
		return fmt.Errorf("repository git storage is unavailable")
	}
	if _, err := object.GetCommit(stor, plumbing.NewHash(commitSHA)); err != nil {
		return fmt.Errorf("commit_sha does not identify a repository commit: %w", err)
	}
	return nil
}

func (s *Server) handleGetSARIFUpload(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupCodeScanningReadableRepo(w, r)
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

	out := map[string]interface{}{
		"state":      "not-configured",
		"languages":  []string{},
		"updated_at": nil,
		"schedule":   nil,
	}
	if setup := s.store.GetCodeScanningDefaultSetup(repo.FullName); setup != nil {
		out["updated_at"] = setup.UpdatedAt.UTC().Format(time.RFC3339)
		if setup.State == "configured" {
			out["state"] = "configured"
			out["languages"] = setup.Languages
			out["query_suite"] = setup.QuerySuite
			// Default setup runs on GitHub's weekly periodic schedule.
			out["schedule"] = "weekly"
			if setup.RunnerType != "" {
				out["runner_type"] = setup.RunnerType
			}
			if setup.RunnerLabel != "" {
				out["runner_label"] = setup.RunnerLabel
			}
			if setup.ThreatModel != "" {
				out["threat_model"] = setup.ThreatModel
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// codeQLDefaultSetupLanguages is the language set accepted by the update
// endpoint (the code-scanning-default-setup-update schema enum).
var codeQLDefaultSetupLanguages = map[string]bool{
	"actions":               true,
	"c-cpp":                 true,
	"csharp":                true,
	"go":                    true,
	"java-kotlin":           true,
	"javascript-typescript": true,
	"python":                true,
	"ruby":                  true,
	"swift":                 true,
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
		State       string   `json:"state"`
		QuerySuite  string   `json:"query_suite"`
		RunnerType  string   `json:"runner_type"`
		RunnerLabel string   `json:"runner_label"`
		ThreatModel string   `json:"threat_model"`
		Languages   []string `json:"languages"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.State != "" && req.State != "configured" && req.State != "not-configured" {
		writeGHValidationError(w, "CodeScanningDefaultSetup", "state", "invalid")
		return
	}
	if req.QuerySuite != "" && req.QuerySuite != "default" && req.QuerySuite != "extended" {
		writeGHValidationError(w, "CodeScanningDefaultSetup", "query_suite", "invalid")
		return
	}
	if req.RunnerType != "" && req.RunnerType != "standard" && req.RunnerType != "labeled" {
		writeGHValidationError(w, "CodeScanningDefaultSetup", "runner_type", "invalid")
		return
	}
	if req.ThreatModel != "" && req.ThreatModel != "remote" && req.ThreatModel != "remote_and_local" {
		writeGHValidationError(w, "CodeScanningDefaultSetup", "threat_model", "invalid")
		return
	}
	for _, lang := range req.Languages {
		if !codeQLDefaultSetupLanguages[lang] {
			writeGHValidationError(w, "CodeScanningDefaultSetup", "languages", "invalid")
			return
		}
	}

	current := s.store.GetCodeScanningDefaultSetup(repo.FullName)
	configured := current != nil && current.State == "configured"

	if req.State == "not-configured" {
		if !configured {
			writeGHError(w, http.StatusConflict, "Code scanning default setup is already disabled")
			return
		}
		s.store.SetCodeScanningDefaultSetup(&CodeScanningDefaultSetup{
			RepoKey: repo.FullName,
			State:   "not-configured",
		})
		writeJSON(w, http.StatusOK, map[string]interface{}{})
		return
	}
	if req.State == "" && !configured {
		writeGHError(w, http.StatusConflict, "Code scanning default setup must be enabled before its configuration can be changed")
		return
	}

	setup := &CodeScanningDefaultSetup{
		RepoKey:    repo.FullName,
		State:      "configured",
		QuerySuite: "default",
	}
	if configured {
		*setup = *current
	}
	if req.QuerySuite != "" {
		setup.QuerySuite = req.QuerySuite
	}
	if req.RunnerType != "" {
		setup.RunnerType = req.RunnerType
	}
	if req.RunnerLabel != "" {
		setup.RunnerLabel = req.RunnerLabel
	}
	if req.ThreatModel != "" {
		setup.ThreatModel = req.ThreatModel
	}
	if len(req.Languages) > 0 {
		setup.Languages = req.Languages
	} else if len(setup.Languages) == 0 {
		setup.Languages = s.store.detectCodeQLLanguages(repo)
	}
	if setup.RunnerType == "labeled" && setup.RunnerLabel == "" {
		writeGHValidationError(w, "CodeScanningDefaultSetup", "runner_label", "missing_field")
		return
	}
	if len(setup.Languages) == 0 {
		writeGHError(w, http.StatusUnprocessableEntity, "CodeQL default setup cannot be enabled because no CodeQL-supported languages were detected in this repository")
		return
	}
	s.store.SetCodeScanningDefaultSetup(setup)
	writeJSON(w, http.StatusOK, map[string]interface{}{})
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

// --- organization alerts ---

func (s *Server) handleListOrgCodeScanningAlerts(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	q := r.URL.Query()
	alerts := s.store.ListCodeScanningAlertsByOrg(org.ID, q.Get("state"), q.Get("severity"), q.Get("tool_name"), q.Get("sort"), q.Get("direction"))
	page := paginateAndLink(w, r, alerts)
	baseURL := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(page))
	for _, a := range page {
		repo := s.store.ReposByName[a.RepoKey]
		if repo == nil {
			continue
		}
		alertJSON := codeScanningAlertToJSON(a, baseURL, repo)
		alertJSON["repository"] = simpleRepoJSON(repo, s.store, baseURL)
		out = append(out, alertJSON)
	}
	writeJSON(w, http.StatusOK, out)
}

// --- Copilot Autofix ---

func codeScanningAutofixJSON(fix *CodeScanningAutofix) map[string]interface{} {
	return map[string]interface{}{
		"status":      fix.Status,
		"description": fix.Description,
		"started_at":  fix.StartedAt.UTC().Format(time.RFC3339),
	}
}

func (s *Server) handleGetCodeScanningAutofix(w http.ResponseWriter, r *http.Request) {
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

	fix := s.store.GetCodeScanningAutofix(repo.FullName, a.Number)
	if fix == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, codeScanningAutofixJSON(fix))
}

func (s *Server) handleCreateCodeScanningAutofix(w http.ResponseWriter, r *http.Request) {
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
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "Must have push access to Repository.")
		return
	}
	a := s.lookupCodeScanningAlert(w, r, repo)
	if a == nil {
		return
	}
	if a.State != "open" {
		writeGHError(w, http.StatusUnprocessableEntity, "It is not possible to generate an autofix for this alert because it is not open")
		return
	}
	if len(a.Instances) == 0 || a.Instances[len(a.Instances)-1].Path == "" {
		writeGHError(w, http.StatusUnprocessableEntity, "It is not possible to generate an autofix for this alert because it has no source location")
		return
	}

	fix, created := s.store.CreateCodeScanningAutofix(a)
	status := http.StatusOK
	if created {
		status = http.StatusAccepted
	}
	writeJSON(w, status, codeScanningAutofixJSON(fix))
}

func (s *Server) handleCommitCodeScanningAutofix(w http.ResponseWriter, r *http.Request) {
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
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusForbidden, "Must have push access to Repository.")
		return
	}
	a := s.lookupCodeScanningAlert(w, r, repo)
	if a == nil {
		return
	}
	fix := s.store.GetCodeScanningAutofix(repo.FullName, a.Number)
	if fix == nil || fix.Status != "success" {
		writeGHError(w, http.StatusBadRequest, "The autofix for this alert is not available for committing")
		return
	}

	// The body is optional on real GitHub; target_ref defaults to the
	// repository's default branch.
	var req struct {
		TargetRef string `json:"target_ref"`
		Message   string `json:"message"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if !decodeJSONBody(w, r, &req) {
			return
		}
	}
	branch := strings.TrimPrefix(req.TargetRef, "refs/heads/")
	if branch == "" {
		branch = repo.DefaultBranch
	}
	message := req.Message
	if message == "" {
		message = fmt.Sprintf("Autofix for code scanning alert #%d", a.Number)
	}

	owner, repoName, ok := splitRepoFullName(repo.FullName)
	if !ok {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	stor := s.store.GetGitStorage(owner, repoName)
	if stor == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	// The target branch must already exist.
	branchRef := plumbing.NewBranchReferenceName(branch)
	ref, err := stor.Reference(branchRef)
	if err != nil || ref == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Branch "+branch+" does not exist")
		return
	}

	inst := a.Instances[len(a.Instances)-1]
	content := ""
	commit, err := object.GetCommit(stor, ref.Hash())
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "resolve branch head: "+err.Error())
		return
	}
	tree, err := commit.Tree()
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, "resolve branch tree: "+err.Error())
		return
	}
	// A FindEntry failure means the branch does not carry the flagged file;
	// the autofix commit then creates it. Every other read failure is a real
	// storage error and fails loud.
	if entry, findErr := tree.FindEntry(inst.Path); findErr == nil {
		blob, err := object.GetBlob(stor, entry.Hash)
		if err != nil {
			writeGHError(w, http.StatusInternalServerError, "read flagged file blob: "+err.Error())
			return
		}
		reader, err := blob.Reader()
		if err != nil {
			writeGHError(w, http.StatusInternalServerError, "open flagged file blob: "+err.Error())
			return
		}
		raw, readErr := io.ReadAll(reader)
		if closeErr := reader.Close(); readErr == nil {
			readErr = closeErr
		}
		if readErr != nil {
			writeGHError(w, http.StatusInternalServerError, "read flagged file: "+readErr.Error())
			return
		}
		content = string(raw)
	}

	fixed := applyAutofixEdit(content, inst, fix.Description)
	sig := repoSignature(user.Login, "bleephub@local")
	commitHash, err := createFileCommit(stor, branch, inst.Path, fixed, message, sig)
	if err != nil {
		writeGHError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	s.store.UpdateRepo(owner, repoName, func(r *Repo) {
		r.PushedAt = time.Now().UTC()
	})

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"target_ref": "refs/heads/" + branch,
		"sha":        commitHash.String(),
	})
}

// applyAutofixEdit produces the fixed file content for an autofix commit:
// the generated remediation line is inserted immediately above the alert's
// flagged start line (or at the top of a file the branch does not have yet).
func applyAutofixEdit(content string, inst CodeScanningAlertInstance, description string) string {
	lines := strings.Split(content, "\n")
	idx := inst.StartLine - 1
	if idx < 0 {
		idx = 0
	}
	if idx > len(lines) {
		idx = len(lines)
	}
	fixed := make([]string, 0, len(lines)+1)
	fixed = append(fixed, lines[:idx]...)
	fixed = append(fixed, "// autofix: "+description)
	fixed = append(fixed, lines[idx:]...)
	return strings.Join(fixed, "\n")
}

// --- CodeQL databases ---

// codeQLLanguages are the languages the CodeQL variant-analysis API
// accepts, matching GitHub's code-scanning-variant-analysis-language enum.
var codeQLLanguages = map[string]bool{
	"actions": true, "cpp": true, "csharp": true, "go": true, "java": true,
	"javascript": true, "python": true, "ruby": true, "rust": true, "swift": true,
}

func (s *Server) codeQLDatabaseJSON(db *CodeQLDatabase, baseURL string, repo *Repo) map[string]interface{} {
	uploader := s.store.GetUserByID(db.UploaderID)
	var uploaderJSON map[string]interface{}
	if uploader != nil {
		uploaderJSON = userToJSON(uploader)
	} else if db.UploaderID < 0 {
		if app := s.store.GetApp(-db.UploaderID); app != nil {
			uploaderJSON = userToJSON(&User{ID: -app.ID, Login: app.Slug + "[bot]", Type: "Bot"})
		}
	}
	return map[string]interface{}{
		"id":           db.ID,
		"name":         db.Name,
		"language":     db.Language,
		"uploader":     uploaderJSON,
		"content_type": db.ContentType,
		"size":         db.Size,
		"created_at":   db.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":   db.UpdatedAt.UTC().Format(time.RFC3339),
		"url":          fmt.Sprintf("%s/api/v3/repos/%s/code-scanning/codeql/databases/%s", baseURL, repo.FullName, db.Language),
		"commit_oid":   nullOrString(db.CommitOID),
	}
}

func (s *Server) handleListCodeQLDatabases(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupCodeScanningReadableRepo(w, r)
	if repo == nil {
		return
	}

	baseURL := s.baseURL(r)
	dbs := s.store.ListCodeQLDatabases(repo.FullName)
	out := make([]map[string]interface{}, 0, len(dbs))
	for _, db := range dbs {
		out = append(out, s.codeQLDatabaseJSON(db, baseURL, repo))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetCodeQLDatabase(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupCodeScanningReadableRepo(w, r)
	if repo == nil {
		return
	}
	db := s.store.GetCodeQLDatabase(repo.FullName, r.PathValue("language"))
	if db == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	// With the Accept header set to the database's content type, real
	// GitHub redirects to a download URL for the archive bytes.
	if strings.Contains(r.Header.Get("Accept"), db.ContentType) {
		loc := fmt.Sprintf("%s/code-scanning/repos/%s/codeql/databases/%s/download", s.baseURL(r), repo.FullName, db.Language)
		http.Redirect(w, r, loc, http.StatusFound)
		return
	}
	writeJSON(w, http.StatusOK, s.codeQLDatabaseJSON(db, s.baseURL(r), repo))
}

func (s *Server) handleDeleteCodeQLDatabase(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !s.codeScanningRequestCanWriteRepo(r, repo) {
		writeGHError(w, http.StatusForbidden, "Must have push access to Repository.")
		return
	}
	deleted, err := s.store.DeleteCodeQLDatabase(repo.FullName, r.PathValue("language"))
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !deleted {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) lookupCodeScanningReadableRepo(w http.ResponseWriter, r *http.Request) *Repo {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	if !s.codeScanningRequestCanReadRepo(r, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return repo
}

func (s *Server) handleUploadCodeQLDatabase(w http.ResponseWriter, r *http.Request) {
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
	if !s.codeScanningRequestCanWriteRepo(r, repo) {
		writeGHError(w, http.StatusForbidden, "Must have push access to Repository.")
		return
	}
	language := strings.ToLower(strings.TrimSpace(r.PathValue("language")))
	if !codeQLLanguages[language] {
		writeGHValidationError(w, "CodeQLDatabase", "language", "invalid")
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeGHValidationError(w, "CodeQLDatabase", "name", "missing_field")
		return
	}
	commitOID := strings.TrimSpace(r.URL.Query().Get("commit_oid"))
	if err := s.validateCodeScanningCoordinate(repo, commitOID, "refs/heads/"+repo.DefaultBranch); err != nil {
		writeGHValidationError(w, "CodeQLDatabase", "commit_oid", "invalid")
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/zip" {
		writeGHError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/zip")
		return
	}
	content, err := io.ReadAll(r.Body)
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Unable to read CodeQL database bundle")
		return
	}
	if err := validateCodeQLDatabaseBundle(content, language); err != nil {
		writeGHValidationError(w, "CodeQLDatabase", "data", "invalid")
		return
	}

	_, err = s.store.UpsertCodeQLDatabase(repo.FullName, language, name, "application/zip", commitOID, content, user.ID)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// validateCodeQLDatabaseBundle checks the relocatable ZIP shape emitted by
// `codeql database bundle`: one database root contains codeql-database.yml
// (or the legacy .dbinfo manifest) and a non-empty db-{language} dataset.
// Paths and entry types are validated without extracting attacker-controlled
// content onto the filesystem.
func validateCodeQLDatabaseBundle(content []byte, language string) error {
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return fmt.Errorf("open CodeQL database ZIP: %w", err)
	}
	manifestRoots := make(map[string]bool)
	datasetRoots := make(map[string]bool)
	for _, file := range zr.File {
		name := cleanPagesArchivePath(file.Name)
		if name == "" {
			return fmt.Errorf("unsafe CodeQL database ZIP path %q", file.Name)
		}
		if !file.Mode().IsRegular() && !file.FileInfo().IsDir() {
			return fmt.Errorf("CodeQL database ZIP entry %q is not a regular file or directory", file.Name)
		}
		parts := strings.Split(name, "/")
		base := parts[len(parts)-1]
		if file.Mode().IsRegular() && file.UncompressedSize64 > 0 && (base == "codeql-database.yml" || base == ".dbinfo") {
			if base == "codeql-database.yml" {
				if file.UncompressedSize64 > 1<<20 {
					return fmt.Errorf("CodeQL database manifest exceeds 1 MiB")
				}
				reader, err := file.Open()
				if err != nil {
					return fmt.Errorf("open CodeQL database manifest: %w", err)
				}
				manifest, readErr := io.ReadAll(io.LimitReader(reader, 1<<20))
				closeErr := reader.Close()
				if readErr != nil {
					return fmt.Errorf("read CodeQL database manifest: %w", readErr)
				}
				if closeErr != nil {
					return fmt.Errorf("close CodeQL database manifest: %w", closeErr)
				}
				var metadata map[string]interface{}
				if err := yaml.Unmarshal(manifest, &metadata); err != nil {
					return fmt.Errorf("parse CodeQL database manifest: %w", err)
				}
				if _, inProgress := metadata["inProgress"]; inProgress {
					return fmt.Errorf("CodeQL database is not finalized")
				}
				if primary, _ := metadata["primaryLanguage"].(string); primary != "" && primary != language {
					return fmt.Errorf("CodeQL database manifest language %q does not match upload language %q", primary, language)
				}
			}
			manifestRoots[strings.Join(parts[:len(parts)-1], "/")] = true
		}
		for i, part := range parts {
			if part == "db-"+language && file.Mode().IsRegular() && i < len(parts)-1 && file.UncompressedSize64 > 0 {
				datasetRoots[strings.Join(parts[:i], "/")] = true
			}
		}
	}
	for root := range manifestRoots {
		if datasetRoots[root] {
			return nil
		}
	}
	return fmt.Errorf("CodeQL database ZIP has no matching manifest and db-%s dataset", language)
}

func (s *Server) handleDownloadCodeQLDatabase(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}
	if !s.codeScanningRequestCanReadRepo(r, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	db := s.store.GetCodeQLDatabase(repo.FullName, r.PathValue("language"))
	if db == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}
	data, err := s.store.ReadCodeQLDatabaseContent(r.Context(), db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return
	}
	w.Header().Set("Content-Type", db.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(db.Size, 10))
	filename := db.Name
	if !strings.HasSuffix(strings.ToLower(filename), ".zip") {
		filename += ".zip"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// --- CodeQL variant analyses ---

// variantAnalysisRepoIdentifierJSON renders the compact repository
// identifier shape (code-scanning-variant-analysis-repository) used in
// scanned/skipped repository groups.
func variantAnalysisRepoIdentifierJSON(repo *Repo) map[string]interface{} {
	return map[string]interface{}{
		"id":               repo.ID,
		"name":             repo.Name,
		"full_name":        repo.FullName,
		"private":          repo.Private,
		"stargazers_count": repo.StargazersCount,
		"updated_at":       repo.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func variantAnalysisSkippedGroupJSON(repos []map[string]interface{}) map[string]interface{} {
	if repos == nil {
		repos = []map[string]interface{}{}
	}
	return map[string]interface{}{
		"repository_count": len(repos),
		"repositories":     repos,
	}
}

func (s *Server) variantAnalysisJSON(va *CodeQLVariantAnalysis, baseURL string, controllerRepo *Repo) map[string]interface{} {
	actor := s.store.GetUserByID(va.ActorID)
	var actorJSON map[string]interface{}
	if actor != nil {
		actorJSON = userToJSON(actor)
	}

	scanned := make([]map[string]interface{}, 0, len(va.ScannedRepositories))
	for _, task := range va.ScannedRepositories {
		repo := s.store.GetRepoByID(task.RepoID)
		if repo == nil {
			continue
		}
		scanned = append(scanned, map[string]interface{}{
			"repository":      variantAnalysisRepoIdentifierJSON(repo),
			"analysis_status": task.AnalysisStatus,
			"result_count":    task.ResultCount,
		})
	}

	noDBRepos := make([]map[string]interface{}, 0, len(va.NoCodeQLDBRepos))
	for _, id := range va.NoCodeQLDBRepos {
		if repo := s.store.GetRepoByID(id); repo != nil {
			noDBRepos = append(noDBRepos, variantAnalysisRepoIdentifierJSON(repo))
		}
	}
	notFound := va.NotFoundRepos
	if notFound == nil {
		notFound = []string{}
	}

	var completedAt interface{}
	if va.CompletedAt != nil {
		completedAt = va.CompletedAt.UTC().Format(time.RFC3339)
	}

	out := map[string]interface{}{
		"id":                   va.ID,
		"controller_repo":      simpleRepoJSON(controllerRepo, s.store, baseURL),
		"actor":                actorJSON,
		"query_language":       va.QueryLanguage,
		"query_pack_url":       fmt.Sprintf("%s/code-scanning/repos/%s/codeql/variant-analyses/%d/query-pack", baseURL, controllerRepo.FullName, va.ID),
		"created_at":           va.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":           va.UpdatedAt.UTC().Format(time.RFC3339),
		"completed_at":         completedAt,
		"status":               va.Status,
		"scanned_repositories": scanned,
		"skipped_repositories": map[string]interface{}{
			"access_mismatch_repos": variantAnalysisSkippedGroupJSON(nil),
			"not_found_repos": map[string]interface{}{
				"repository_count":      len(notFound),
				"repository_full_names": notFound,
			},
			"no_codeql_db_repos": variantAnalysisSkippedGroupJSON(noDBRepos),
			"over_limit_repos":   variantAnalysisSkippedGroupJSON(nil),
		},
	}
	if va.FailureReason != "" {
		out["failure_reason"] = va.FailureReason
	}
	return out
}

func (s *Server) handleCreateCodeQLVariantAnalysis(w http.ResponseWriter, r *http.Request) {
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
	if !canPushRepo(s.store, user, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		Language         string   `json:"language"`
		QueryPack        string   `json:"query_pack"`
		Repositories     []string `json:"repositories"`
		RepositoryLists  []string `json:"repository_lists"`
		RepositoryOwners []string `json:"repository_owners"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if !codeQLLanguages[req.Language] {
		writeGHValidationError(w, "VariantAnalysis", "language", "invalid")
		return
	}
	if req.QueryPack == "" {
		writeGHValidationError(w, "VariantAnalysis", "query_pack", "missing_field")
		return
	}
	queryPack, err := base64.StdEncoding.DecodeString(req.QueryPack)
	if err != nil {
		writeGHValidationError(w, "VariantAnalysis", "query_pack", "invalid")
		return
	}
	sources := 0
	if req.Repositories != nil {
		sources++
	}
	if req.RepositoryLists != nil {
		sources++
	}
	if req.RepositoryOwners != nil {
		sources++
	}
	if sources != 1 {
		writeGHValidationError(w, "VariantAnalysis", "repositories", "invalid")
		return
	}
	if len(req.RepositoryLists) > 1 || len(req.RepositoryOwners) > 1 {
		writeGHValidationError(w, "VariantAnalysis", "repositories", "invalid")
		return
	}

	targets := req.Repositories
	for _, owner := range req.RepositoryOwners {
		targets = append(targets, s.store.ListRepoFullNamesByOwner(owner)...)
	}
	// Repository lists are a github.com saved-list feature bleephub does not
	// model; a named list resolves to no repositories, so an analysis driven
	// only by lists fails with no_repos_queried below.

	va, err := s.store.CreateCodeQLVariantAnalysis(repo.FullName, user.ID, req.Language, queryPack, targets)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, s.variantAnalysisJSON(va, s.baseURL(r), repo))
}

func (s *Server) lookupCodeQLVariantAnalysis(w http.ResponseWriter, r *http.Request, repo *Repo) *CodeQLVariantAnalysis {
	id, err := strconv.Atoi(r.PathValue("codeql_variant_analysis_id"))
	if err != nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	va := s.store.GetCodeQLVariantAnalysis(repo.FullName, id)
	if va == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return nil
	}
	return va
}

func (s *Server) handleGetCodeQLVariantAnalysis(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	va := s.lookupCodeQLVariantAnalysis(w, r, repo)
	if va == nil {
		return
	}
	writeJSON(w, http.StatusOK, s.variantAnalysisJSON(va, s.baseURL(r), repo))
}

func (s *Server) handleGetCodeQLVariantAnalysisRepoTask(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Requires authentication")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}
	va := s.lookupCodeQLVariantAnalysis(w, r, repo)
	if va == nil {
		return
	}

	fullName := r.PathValue("repo_owner") + "/" + r.PathValue("repo_name")
	for _, task := range va.ScannedRepositories {
		if task.FullName != fullName {
			continue
		}
		taskRepo := s.store.GetRepoByID(task.RepoID)
		if taskRepo == nil {
			break
		}
		out := map[string]interface{}{
			"repository":      simpleRepoJSON(taskRepo, s.store, s.baseURL(r)),
			"analysis_status": task.AnalysisStatus,
		}
		if task.AnalysisStatus == "succeeded" {
			out["result_count"] = task.ResultCount
			if task.DatabaseCommitSHA != "" {
				out["database_commit_sha"] = task.DatabaseCommitSHA
			}
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}

func (s *Server) handleDownloadCodeQLVariantAnalysisQueryPack(w http.ResponseWriter, r *http.Request) {
	repo := s.lookupRepoFromPath(r)
	if repo == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}
	if !s.codeScanningRequestCanReadRepo(r, repo) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	id, err := strconv.Atoi(r.PathValue("codeql_variant_analysis_id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}
	va := s.store.GetCodeQLVariantAnalysis(repo.FullName, id)
	if va == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
		return
	}
	pack, err := s.store.ReadCodeQLVariantAnalysisQueryPack(r.Context(), va)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Length", strconv.Itoa(len(pack)))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, fmt.Sprintf("codeql-variant-analysis-%d-query-pack.tar.gz", va.ID)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pack)
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
