package bleephub

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Global security advisories (GET /advisories and GET /advisories/{ghsa_id}).
// The global database view is backed by the repository security-advisory
// store: a repository advisory enters the global database when it is
// published, and stays there (with withdrawn_at set) if later withdrawn.

func (s *Server) registerGHGlobalAdvisoriesRoutes() {
	s.route("GET /api/v3/advisories", s.handleListGlobalAdvisories)
	s.route("GET /api/v3/advisories/{ghsa_id}", s.handleGetGlobalAdvisory)
}

// listGlobalAdvisories returns every advisory in the global database view,
// i.e. every repository advisory that has been published.
func (st *Store) listGlobalAdvisories() []*SecurityAdvisory {
	st.mu.RLock()
	defer st.mu.RUnlock()
	var out []*SecurityAdvisory
	for _, a := range st.SecurityAdvisories {
		if a.PublishedAt == nil {
			continue
		}
		if a.State != "published" && a.State != "withdrawn" {
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// advisoryDateInRange evaluates GitHub's date-range filter syntax: an exact
// date ("2023-06-01"), a range ("2023-01-01..2023-06-30"), or an open range
// (">=2023-01-01", "<=2023-06-30", ">2023-01-01", "<2023-06-30").
func advisoryDateInRange(t time.Time, expr string) bool {
	day := t.UTC().Format("2006-01-02")
	switch {
	case strings.Contains(expr, ".."):
		from, to, _ := strings.Cut(expr, "..")
		return day >= from && day <= to
	case strings.HasPrefix(expr, ">="):
		return day >= expr[2:]
	case strings.HasPrefix(expr, "<="):
		return day <= expr[2:]
	case strings.HasPrefix(expr, ">"):
		return day > expr[1:]
	case strings.HasPrefix(expr, "<"):
		return day < expr[1:]
	default:
		return day == expr
	}
}

func (s *Server) handleListGlobalAdvisories(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// The global database only contains reviewed advisories (repository
	// advisories are GitHub-reviewed by publication); the documented default
	// for the type filter is "reviewed".
	typeFilter := q.Get("type")
	if typeFilter == "" {
		typeFilter = "reviewed"
	}

	advisories := s.store.listGlobalAdvisories()
	filtered := advisories[:0:0]
	for _, a := range advisories {
		if typeFilter != "reviewed" {
			continue
		}
		if v := q.Get("ghsa_id"); v != "" && a.GHSAID != v {
			continue
		}
		if v := q.Get("cve_id"); v != "" && a.CVEID != v {
			continue
		}
		if v := q.Get("severity"); v != "" && a.Severity != v {
			continue
		}
		if v := q.Get("is_withdrawn"); v != "" {
			withdrawn := a.State == "withdrawn"
			if (v == "true") != withdrawn {
				continue
			}
		}
		if v := q.Get("cwes"); v != "" {
			if !advisoryHasAnyCWE(a, strings.Split(v, ",")) {
				continue
			}
		}
		// Repository advisories carry no package coordinates, so the
		// ecosystem and affects filters match nothing.
		if q.Get("ecosystem") != "" || q.Get("affects") != "" {
			continue
		}
		if v := q.Get("published"); v != "" && !advisoryDateInRange(*a.PublishedAt, v) {
			continue
		}
		if v := q.Get("updated"); v != "" && !advisoryDateInRange(a.UpdatedAt, v) {
			continue
		}
		if v := q.Get("modified"); v != "" && !advisoryDateInRange(a.UpdatedAt, v) {
			continue
		}
		filtered = append(filtered, a)
	}

	sortKey := q.Get("sort")
	if sortKey == "" {
		sortKey = "published"
	}
	direction := q.Get("direction")
	if direction == "" {
		direction = "desc"
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		var less bool
		switch sortKey {
		case "updated":
			less = filtered[i].UpdatedAt.Before(filtered[j].UpdatedAt)
		default:
			less = filtered[i].PublishedAt.Before(*filtered[j].PublishedAt)
		}
		if direction == "asc" {
			return less
		}
		return !less
	})

	perPage := 30
	if v := q.Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perPage = n
			if perPage > 100 {
				perPage = 100
			}
		}
	}
	start := 0
	if after := q.Get("after"); after != "" {
		if idx, ok := decodeAdvisoryCursor(after); ok {
			start = idx + 1
		}
	}
	if before := q.Get("before"); before != "" {
		if idx, ok := decodeAdvisoryCursor(before); ok {
			start = idx - perPage
		}
	}
	if start < 0 {
		start = 0
	}
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + perPage
	if end > len(filtered) {
		end = len(filtered)
	}
	page := filtered[start:end]

	// Cursor-based Link header, matching the global advisories endpoint's
	// before/after pagination.
	var links []string
	if end < len(filtered) {
		next := *r.URL
		nq := next.Query()
		nq.Del("before")
		nq.Set("after", encodeAdvisoryCursor(end-1))
		next.RawQuery = nq.Encode()
		links = append(links, fmt.Sprintf("<%s>; rel=\"next\"", next.String()))
	}
	if start > 0 {
		prev := *r.URL
		pq := prev.Query()
		pq.Del("after")
		pq.Set("before", encodeAdvisoryCursor(start))
		prev.RawQuery = pq.Encode()
		links = append(links, fmt.Sprintf("<%s>; rel=\"prev\"", prev.String()))
	}
	if len(links) > 0 {
		w.Header().Set("Link", strings.Join(links, ", "))
	}

	base := s.baseURL(r)
	out := make([]map[string]interface{}, 0, len(page))
	for _, a := range page {
		out = append(out, s.globalAdvisoryToJSON(a, base))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetGlobalAdvisory(w http.ResponseWriter, r *http.Request) {
	ghsaID := r.PathValue("ghsa_id")
	for _, a := range s.store.listGlobalAdvisories() {
		if a.GHSAID == ghsaID {
			writeJSON(w, http.StatusOK, s.globalAdvisoryToJSON(a, s.baseURL(r)))
			return
		}
	}
	writeGHError(w, http.StatusNotFound, "Not Found")
}

func advisoryHasAnyCWE(a *SecurityAdvisory, wanted []string) bool {
	for _, w := range wanted {
		w = strings.TrimSpace(w)
		for _, c := range a.CWEs {
			if c == w || cweName(c) == cweName(w) {
				return true
			}
		}
	}
	return false
}

func encodeAdvisoryCursor(idx int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(idx)))
}

func decodeAdvisoryCursor(cursor string) (int, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, false
	}
	idx, err := strconv.Atoi(string(raw))
	if err != nil {
		return 0, false
	}
	return idx, true
}

// globalAdvisoryToJSON renders the spec `global-advisory` shape from a
// published repository advisory.
func (s *Server) globalAdvisoryToJSON(a *SecurityAdvisory, baseURL string) map[string]interface{} {
	repo := s.store.GetRepoByID(a.RepoID)

	identifiers := []map[string]interface{}{
		{"type": "GHSA", "value": a.GHSAID},
	}
	if a.CVEID != "" {
		identifiers = append(identifiers, map[string]interface{}{"type": "CVE", "value": a.CVEID})
	}

	cwes := make([]map[string]interface{}, 0, len(a.CWEs))
	for _, cwe := range a.CWEs {
		cwes = append(cwes, map[string]interface{}{"cwe_id": cwe, "name": cweName(cwe)})
	}

	vulnerabilities := []map[string]interface{}{}
	for _, v := range a.Vulnerabilities {
		firstPatched := interface{}(nil)
		if v.FirstPatchedVersion != "" {
			firstPatched = v.FirstPatchedVersion
		}
		vulnerabilities = append(vulnerabilities, map[string]interface{}{
			"package":                  map[string]interface{}{"ecosystem": v.PackageEcosystem, "name": v.PackageName},
			"vulnerable_version_range": v.VulnerableVersionRange,
			"first_patched_version":    firstPatched,
			"vulnerable_functions":     []string{},
		})
	}
	if len(vulnerabilities) == 0 && a.VulnerableVersionRange != "" {
		vulnerabilities = append(vulnerabilities, map[string]interface{}{
			// Repository advisories carry a version range but no package
			// coordinates; the package members are nullable in the schema.
			"package":                  map[string]interface{}{"ecosystem": nil, "name": nil},
			"vulnerable_version_range": a.VulnerableVersionRange,
			"first_patched_version":    nil,
			"vulnerable_functions":     []string{},
		})
	}

	var cvssScore interface{}
	if a.CVSSScore != 0 {
		cvssScore = a.CVSSScore
	}

	publishedAt := a.PublishedAt.UTC().Format(time.RFC3339)
	var withdrawnAt interface{}
	if a.State == "withdrawn" {
		withdrawnAt = a.UpdatedAt.UTC().Format(time.RFC3339)
	}

	credits := []map[string]interface{}{}
	if u := s.store.GetUserByID(a.AuthorID); u != nil {
		credits = append(credits, map[string]interface{}{
			"user": userToJSON(u),
			"type": "reporter",
		})
	}

	out := map[string]interface{}{
		"ghsa_id":            a.GHSAID,
		"cve_id":             nullOrString(a.CVEID),
		"url":                baseURL + "/api/v3/advisories/" + a.GHSAID,
		"html_url":           baseURL + "/advisories/" + a.GHSAID,
		"summary":            a.Summary,
		"description":        nullOrString(a.Description),
		"type":               "reviewed",
		"severity":           a.Severity,
		"identifiers":        identifiers,
		"references":         []string{},
		"published_at":       publishedAt,
		"updated_at":         a.UpdatedAt.UTC().Format(time.RFC3339),
		"github_reviewed_at": publishedAt,
		"nvd_published_at":   nil,
		"withdrawn_at":       withdrawnAt,
		"vulnerabilities":    vulnerabilities,
		"cvss": map[string]interface{}{
			"vector_string": nullOrString(a.CVSSVector),
			"score":         cvssScore,
		},
		"cwes":    cwes,
		"credits": credits,
	}
	if repo != nil {
		out["repository_advisory_url"] = fmt.Sprintf("%s/api/v3/repos/%s/security-advisories/%s", baseURL, repo.FullName, a.GHSAID)
		out["source_code_location"] = baseURL + "/" + repo.FullName
	} else {
		out["repository_advisory_url"] = nil
		out["source_code_location"] = nil
	}
	return out
}
