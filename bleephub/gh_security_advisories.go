package bleephub

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

func (s *Server) registerGHSecurityAdvisoriesRoutes() {
	s.route("GET /api/v3/repos/{owner}/{repo}/security-advisories", s.requirePerm(scopeSecurityEvents, permRead, s.handleListSecurityAdvisories))
	s.route("POST /api/v3/repos/{owner}/{repo}/security-advisories", s.requirePerm(scopeSecurityEvents, permWrite, s.handleCreateSecurityAdvisory))
	s.route("GET /api/v3/repos/{owner}/{repo}/security-advisories/{ghsa_id}", s.requirePerm(scopeSecurityEvents, permRead, s.handleGetSecurityAdvisory))
	s.route("PATCH /api/v3/repos/{owner}/{repo}/security-advisories/{ghsa_id}", s.requirePerm(scopeSecurityEvents, permWrite, s.handleUpdateSecurityAdvisory))
	s.route("POST /api/v3/repos/{owner}/{repo}/security-advisories/{ghsa_id}/cve", s.requirePerm(scopeSecurityEvents, permWrite, s.handleRequestCVE))
	s.route("POST /api/v3/repos/{owner}/{repo}/security-advisories/{ghsa_id}/forks", s.requirePerm(scopeSecurityEvents, permWrite, s.handleCreateTemporaryFork))
	// The literal /reports path conflicts with the {ghsa_id} wildcard in Go 1.22's mux,
	// so the wildcard dispatches to the real /security-advisories/reports endpoint.
	s.route("POST /api/v3/repos/{owner}/{repo}/security-advisories/{ghsa_id}", s.requirePerm(scopeSecurityEvents, permWrite, s.handleSecurityAdvisoryReportsDispatch))
	s.route("GET /api/v3/orgs/{org}/security-advisories", s.requireOrgAdmin(scopeSecurityEvents, permRead, s.handleListOrgSecurityAdvisories))
}

// handleListOrgSecurityAdvisories implements GET /orgs/{org}/security-advisories:
// the union of every advisory filed against the organization's repositories.
func (s *Server) handleListOrgSecurityAdvisories(w http.ResponseWriter, r *http.Request) {
	org := s.store.GetOrg(r.PathValue("org"))
	if org == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	type advisoryRow struct {
		advisory *SecurityAdvisory
		repo     *Repo
	}
	s.store.mu.RLock()
	var rows []advisoryRow
	for repoKey, byGHSA := range s.store.SecurityAdvisoriesByRepo {
		repo := s.store.ReposByName[repoKey]
		if repo == nil || repo.OwnerType != "Organization" || repo.OwnerID != org.ID {
			continue
		}
		for _, a := range byGHSA {
			rows = append(rows, advisoryRow{advisory: a, repo: repo})
		}
	}
	s.store.mu.RUnlock()

	if state := r.URL.Query().Get("state"); state != "" {
		kept := rows[:0]
		for _, row := range rows {
			if row.advisory.State == state {
				kept = append(kept, row)
			}
		}
		rows = kept
	}

	sortKey := r.URL.Query().Get("sort")
	asc := r.URL.Query().Get("direction") == "asc"
	sortTime := func(row advisoryRow) time.Time {
		switch sortKey {
		case "updated":
			return row.advisory.UpdatedAt
		case "published":
			if row.advisory.PublishedAt != nil {
				return *row.advisory.PublishedAt
			}
			return time.Time{}
		default: // created
			return row.advisory.CreatedAt
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ti, tj := sortTime(rows[i]), sortTime(rows[j])
		if ti.Equal(tj) {
			// Tiebreak equal timestamps by ID for a stable order.
			if asc {
				return rows[i].advisory.ID < rows[j].advisory.ID
			}
			return rows[i].advisory.ID > rows[j].advisory.ID
		}
		if asc {
			return ti.Before(tj)
		}
		return tj.Before(ti)
	})

	page := paginateAndLink(w, r, rows)
	baseURL := s.baseURL(r)
	out := make([]map[string]interface{}, len(page))
	for i, row := range page {
		out[i] = securityAdvisoryToJSON(row.advisory, row.repo, baseURL, s.store)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListSecurityAdvisories(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}

	state := r.URL.Query().Get("state")
	severity := r.URL.Query().Get("severity")

	advisories := s.store.ListSecurityAdvisories(repo.ID)
	filtered := make([]*SecurityAdvisory, 0, len(advisories))
	for _, a := range advisories {
		if state != "" && a.State != state {
			continue
		}
		if severity != "" && a.Severity != severity {
			continue
		}
		filtered = append(filtered, a)
	}

	page := paginateAndLink(w, r, filtered)
	baseURL := s.baseURL(r)
	out := make([]map[string]interface{}, len(page))
	for i, a := range page {
		out[i] = securityAdvisoryToJSON(a, repo, baseURL, s.store)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateSecurityAdvisory(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
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

	var req CreateAdvisoryReq
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Summary == "" || req.Severity == "" {
		writeGHValidationError(w, "SecurityAdvisory", "summary", "missing_field")
		return
	}
	if !validAdvisorySeverity(req.Severity) {
		writeGHValidationError(w, "SecurityAdvisory", "severity", "invalid")
		return
	}

	adv, err := s.store.CreateSecurityAdvisoryE(repo.ID, user.ID, req)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if adv == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}
	s.deriveDependabotAlertsForPublishedAdvisory(adv)
	writeJSON(w, http.StatusCreated, securityAdvisoryToJSON(adv, repo, s.baseURL(r), s.store))
}

func (s *Server) handleGetSecurityAdvisory(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	repo := s.lookupReadableRepoFromPath(w, r)
	if repo == nil {
		return
	}

	adv := s.store.GetSecurityAdvisoryByGHSA(repo.ID, r.PathValue("ghsa_id"))
	if adv == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	writeJSON(w, http.StatusOK, securityAdvisoryToJSON(adv, repo, s.baseURL(r), s.store))
}

func (s *Server) handleUpdateSecurityAdvisory(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
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

	adv := s.store.GetSecurityAdvisoryByGHSA(repo.ID, r.PathValue("ghsa_id"))
	if adv == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	var req struct {
		Summary     string   `json:"summary"`
		Description string   `json:"description"`
		Severity    string   `json:"severity"`
		CVSSScore   float64  `json:"cvss_score"`
		CVSSVector  string   `json:"cvss_vector"`
		CWEs        []string `json:"cwe_ids"`
		State       string   `json:"state"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	if req.State != "" && !validAdvisoryState(req.State) {
		writeGHValidationError(w, "SecurityAdvisory", "state", "invalid")
		return
	}
	if req.Severity != "" && !validAdvisorySeverity(req.Severity) {
		writeGHValidationError(w, "SecurityAdvisory", "severity", "invalid")
		return
	}

	publishedBefore := adv.PublishedAt != nil && adv.State == "published"
	if !s.store.UpdateSecurityAdvisory(adv.ID, func(a *SecurityAdvisory) {
		if req.Summary != "" {
			a.Summary = req.Summary
		}
		if req.Description != "" {
			a.Description = req.Description
		}
		if req.Severity != "" {
			a.Severity = req.Severity
		}
		if req.CVSSScore != 0 {
			a.CVSSScore = req.CVSSScore
		}
		if req.CVSSVector != "" {
			a.CVSSVector = req.CVSSVector
		}
		if req.CWEs != nil {
			a.CWEs = req.CWEs
		}
		if req.State != "" {
			a.State = req.State
			if req.State == "published" && a.PublishedAt == nil {
				now := time.Now().UTC()
				a.PublishedAt = &now
			}
		}
	}) {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	if !publishedBefore && adv.PublishedAt != nil && adv.State == "published" {
		s.deriveDependabotAlertsForPublishedAdvisory(adv)
	}
	writeJSON(w, http.StatusOK, securityAdvisoryToJSON(adv, repo, s.baseURL(r), s.store))
}

func (s *Server) handleRequestCVE(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
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

	adv := s.store.GetSecurityAdvisoryByGHSA(repo.ID, r.PathValue("ghsa_id"))
	if adv == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	ok, err := s.store.RequestCVEE(adv.ID)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleCreateTemporaryFork(w http.ResponseWriter, r *http.Request) {
	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
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

	adv := s.store.GetSecurityAdvisoryByGHSA(repo.ID, r.PathValue("ghsa_id"))
	if adv == nil {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}
	fork := s.store.CreateTemporaryFork(repo.ID, adv.GHSAID)
	if fork == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}
	writeJSON(w, http.StatusAccepted, fullRepoJSON(fork, s.store, s.baseURL(r)))
}

func (s *Server) handleSecurityAdvisoryReportsDispatch(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("ghsa_id") != "reports" {
		writeGHError(w, http.StatusNotFound, "Not Found")
		return
	}

	user := ghUserFromContext(r.Context())
	if user == nil {
		writeGHError(w, http.StatusUnauthorized, "Bad credentials")
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

	var req CreateAdvisoryReq
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Summary == "" || req.Severity == "" {
		writeGHValidationError(w, "SecurityAdvisory", "summary", "missing_field")
		return
	}
	if !validAdvisorySeverity(req.Severity) {
		writeGHValidationError(w, "SecurityAdvisory", "severity", "invalid")
		return
	}

	req.State = "triage"
	adv, err := s.store.CreateSecurityAdvisoryE(repo.ID, user.ID, req)
	if err != nil {
		writeGHError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if adv == nil {
		writeGHError(w, http.StatusUnprocessableEntity, "Validation Failed")
		return
	}
	s.store.CreateSecurityAdvisoryReport(SecurityAdvisoryReport{
		AdvisoryID:             adv.ID,
		ReporterID:             user.ID,
		Summary:                adv.Summary,
		Description:            adv.Description,
		Severity:               adv.Severity,
		CVSSScore:              adv.CVSSScore,
		CVSSVector:             adv.CVSSVector,
		CWEs:                   adv.CWEs,
		VulnerableVersionRange: adv.VulnerableVersionRange,
		CreatedAt:              time.Now().UTC(),
	})
	adv.SubmissionAccepted = true
	writeJSON(w, http.StatusCreated, securityAdvisoryToJSON(adv, repo, s.baseURL(r), s.store))
}

func securityAdvisoryToJSON(a *SecurityAdvisory, repo *Repo, baseURL string, st *Store) map[string]interface{} {
	apiURL := fmt.Sprintf("%s/api/v3/repos/%s/security-advisories/%s", baseURL, repo.FullName, a.GHSAID)
	htmlURL := fmt.Sprintf("%s/%s/security/advisories/%s", baseURL, repo.FullName, a.GHSAID)

	identifiers := []map[string]interface{}{
		{"type": "GHSA", "value": a.GHSAID},
	}
	if a.CVEID != "" {
		identifiers = append(identifiers, map[string]interface{}{"type": "CVE", "value": a.CVEID})
	}

	cwes := make([]map[string]interface{}, 0, len(a.CWEs))
	cweIDs := make([]string, 0, len(a.CWEs))
	for _, cwe := range a.CWEs {
		cwes = append(cwes, map[string]interface{}{"cwe_id": cwe, "name": cweName(cwe)})
		cweIDs = append(cweIDs, cwe)
	}

	var author interface{} = nil
	if u := st.GetUserByID(a.AuthorID); u != nil {
		author = userToJSON(u)
	}

	var publishedAt interface{} = nil
	if a.PublishedAt != nil {
		publishedAt = a.PublishedAt.UTC().Format(time.RFC3339)
	}

	cvssScore := interface{}(nil)
	if a.CVSSScore != 0 {
		cvssScore = a.CVSSScore
	}

	var privateFork interface{} = nil
	if a.PrivateForkID != 0 {
		if fork := st.GetRepoByID(a.PrivateForkID); fork != nil {
			privateFork = minimalRepoJSON(fork, st, baseURL)
		}
	}

	vulnerabilities := []map[string]interface{}{}
	for _, v := range a.Vulnerabilities {
		firstPatched := interface{}(nil)
		if v.FirstPatchedVersion != "" {
			firstPatched = v.FirstPatchedVersion
		}
		vulnerabilities = append(vulnerabilities, map[string]interface{}{
			"package": map[string]interface{}{
				"ecosystem": v.PackageEcosystem,
				"name":      v.PackageName,
			},
			"vulnerable_version_range": v.VulnerableVersionRange,
			"patched_versions":         firstPatched,
			"vulnerable_functions":     []string{},
		})
	}
	if len(vulnerabilities) == 0 && a.VulnerableVersionRange != "" {
		vulnerabilities = append(vulnerabilities, map[string]interface{}{
			"package": map[string]interface{}{
				"ecosystem": nil,
				"name":      nil,
			},
			"vulnerable_version_range": a.VulnerableVersionRange,
			"patched_versions":         nil,
			"vulnerable_functions":     []string{},
		})
	}

	return map[string]interface{}{
		"ghsa_id":             a.GHSAID,
		"cve_id":              nullOrString(a.CVEID),
		"url":                 apiURL,
		"html_url":            htmlURL,
		"summary":             a.Summary,
		"description":         nullOrString(a.Description),
		"severity":            a.Severity,
		"author":              author,
		"publisher":           nil,
		"identifiers":         identifiers,
		"state":               a.State,
		"created_at":          a.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":          a.UpdatedAt.UTC().Format(time.RFC3339),
		"published_at":        publishedAt,
		"closed_at":           nil,
		"withdrawn_at":        nil,
		"submission":          map[string]interface{}{"accepted": a.SubmissionAccepted},
		"vulnerabilities":     vulnerabilities,
		"cvss":                map[string]interface{}{"vector_string": nullOrString(a.CVSSVector), "score": cvssScore},
		"cwes":                cwes,
		"cwe_ids":             cweIDs,
		"credits":             []map[string]interface{}{},
		"credits_detailed":    []map[string]interface{}{},
		"collaborating_users": []map[string]interface{}{},
		"collaborating_teams": []map[string]interface{}{},
		"private_fork":        privateFork,
	}
}

func cweName(cwe string) string {
	if strings.HasPrefix(cwe, "CWE-") {
		return cwe
	}
	return "CWE-" + cwe
}
