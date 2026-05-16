package bleephub

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// Checks API.
// CheckRun + CheckSuite are App-owned: real GitHub limits Create/Update to
// GitHub App installation tokens. Bleephub permission-gates by "checks"
// scope (read for reads, write for create/update).

func (s *Server) registerGHChecksRoutes() {
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/check-runs", s.requirePerm("checks", permWrite, s.handleCreateCheckRun))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/check-runs/{id}", s.requirePerm("checks", permRead, s.handleGetCheckRun))
	s.mux.HandleFunc("PATCH /api/v3/repos/{owner}/{repo}/check-runs/{id}", s.requirePerm("checks", permWrite, s.handleUpdateCheckRun))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/check-runs/{id}/annotations", s.requirePerm("checks", permRead, s.handleListCheckRunAnnotations))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/commits/{sha}/check-runs", s.requirePerm("checks", permRead, s.handleListCheckRunsForCommit))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/commits/{sha}/check-suites", s.requirePerm("checks", permRead, s.handleListCheckSuitesForCommit))
	s.mux.HandleFunc("POST /api/v3/repos/{owner}/{repo}/check-suites", s.requirePerm("checks", permWrite, s.handleCreateCheckSuite))
	s.mux.HandleFunc("PATCH /api/v3/repos/{owner}/{repo}/check-suites/preferences", s.requirePerm("administration", permWrite, s.handleUpdateCheckSuitePrefs))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/check-suites/{id}", s.requirePerm("checks", permRead, s.handleGetCheckSuite))
	s.mux.HandleFunc("GET /api/v3/repos/{owner}/{repo}/check-suites/{id}/check-runs", s.requirePerm("checks", permRead, s.handleListCheckRunsForSuite))
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
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
	writeJSON(w, http.StatusCreated, checkRunToJSON(s.store.GetCheckRun(cr.ID)))
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
	writeJSON(w, http.StatusOK, checkRunToJSON(cr))
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
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
	writeJSON(w, http.StatusOK, checkRunToJSON(s.store.GetCheckRun(id)))
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
		out = append(out, checkRunToJSON(cr))
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
		out = append(out, checkSuiteToJSON(ss))
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
	writeJSON(w, http.StatusCreated, checkSuiteToJSON(suite))
}

func (s *Server) handleUpdateCheckSuitePrefs(w http.ResponseWriter, r *http.Request) {
	repoKey := r.PathValue("owner") + "/" + r.PathValue("repo")
	var req struct {
		AutoTriggerChecks []*CheckSuitePref `json:"auto_trigger_checks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
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
	writeJSON(w, http.StatusOK, checkSuiteToJSON(suite))
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
		out = append(out, checkRunToJSON(cr))
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

func checkRunToJSON(cr *CheckRun) map[string]interface{} {
	if cr == nil {
		return nil
	}
	out := map[string]interface{}{
		"id":            cr.ID,
		"node_id":       cr.NodeID,
		"head_sha":      cr.HeadSHA,
		"name":          cr.Name,
		"status":        cr.Status,
		"conclusion":    cr.Conclusion,
		"started_at":    cr.StartedAt.UTC().Format(time.RFC3339),
		"external_id":   cr.ExternalID,
		"details_url":   cr.DetailsURL,
		"app":           map[string]interface{}{"id": cr.AppID},
		"check_suite":   map[string]interface{}{"id": cr.SuiteID},
		"pull_requests": []interface{}{},
	}
	if cr.CompletedAt != nil {
		out["completed_at"] = cr.CompletedAt.UTC().Format(time.RFC3339)
	}
	if cr.Output != nil {
		out["output"] = map[string]interface{}{
			"title":             cr.Output.Title,
			"summary":           cr.Output.Summary,
			"text":              cr.Output.Text,
			"annotations_count": cr.Output.AnnotationsCount,
			"images":            cr.Output.Images,
		}
	}
	return out
}

func checkSuiteToJSON(s *CheckSuite) map[string]interface{} {
	if s == nil {
		return nil
	}
	return map[string]interface{}{
		"id":          s.ID,
		"node_id":     s.NodeID,
		"head_branch": s.HeadBranch,
		"head_sha":    s.HeadSHA,
		"status":      s.Status,
		"conclusion":  s.Conclusion,
		"app":         map[string]interface{}{"id": s.AppID},
		"created_at":  s.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  s.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
