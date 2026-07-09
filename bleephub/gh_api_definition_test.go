package bleephub

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// This test enforces the core fidelity invariant the project cares about:
// every route bleephub serves under the GitHub-compatible /api/v3 surface must
// be a REAL GitHub API path — bleephub must not invent paths under the GitHub
// namespace. It validates the registered route table (Server.routePatterns,
// recorded by Server.route) against the official github/rest-api-description
// OpenAPI document, vendored (gzipped) at testdata/github-openapi.json.gz so
// the test is hermetic. Refresh the vendored copy with
// scripts/update-github-openapi.sh.

var paramSegment = regexp.MustCompile(`\{[^}]+\}`)

// normalizePath collapses every "{param}" path segment to "{}", so routes
// match GitHub's templates regardless of parameter naming (bleephub's
// {number} vs GitHub's {issue_number}, etc.).
func normalizePath(path string) string {
	return paramSegment.ReplaceAllString(path, "{}")
}

// loadGitHubOperations parses the vendored OpenAPI description and returns the
// set of normalized "METHOD /path" operations GitHub documents.
func loadGitHubOperations(t *testing.T) map[string]bool {
	t.Helper()
	f, err := os.Open("testdata/github-openapi.json.gz")
	if err != nil {
		t.Fatalf("open vendored OpenAPI: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gunzip OpenAPI: %v", err)
	}
	defer gz.Close()

	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.NewDecoder(gz).Decode(&doc); err != nil {
		t.Fatalf("decode OpenAPI: %v", err)
	}
	if len(doc.Paths) < 500 {
		t.Fatalf("vendored OpenAPI looks truncated: only %d paths", len(doc.Paths))
	}

	ops := make(map[string]bool, len(doc.Paths)*3)
	for path, methods := range doc.Paths {
		norm := normalizePath(path)
		for method := range methods {
			switch method {
			case "get", "post", "put", "patch", "delete", "head":
				ops[strings.ToUpper(method)+" "+norm] = true
			}
		}
	}
	return ops
}

// allowedGHESOnly lists real GitHub Enterprise Server /api/v3 endpoints that
// are NOT present in the dotcom api.github.com description (GHES admin/staff
// tools, the actions/runner registration endpoint, and enterprise-only
// surfaces). Each entry MUST be a documented real GitHub endpoint — this is an
// allow-list of real-but-undescribed paths, never a place to hide an invented
// one. Keyed by normalized "METHOD /path" (without the /api/v3 prefix).
var allowedGHESOnly = map[string]string{
	"POST /actions/runner-registration":                              "GHES runner registration endpoint (actions/runner config.sh)",
	"POST /admin/organizations":                                      "GHES admin/staff-tools API (create org)",
	"POST /admin/users":                                              "GHES admin/staff-tools API (create user)",
	"PATCH /admin/users/{}":                                          "GHES admin/staff-tools API (update user)",
	"DELETE /admin/users/{}":                                         "GHES admin/staff-tools API (delete user)",
	"GET /orgs/{}/audit-log":                                         "Org audit log — real GitHub (Enterprise), absent from the dotcom bundled description",
	"GET /repos/{}/{}/git/refs":                                      "GHES / real GitHub git-refs listing endpoint, absent from the dotcom bundled description",
	"GET /repos/{}/{}/branches/{}/protection/allow_deletions":        "Branch protection allow-deletions setting — real GitHub endpoint, absent from the bundled dotcom description",
	"PUT /repos/{}/{}/branches/{}/protection/allow_deletions":        "Branch protection allow-deletions setting — real GitHub endpoint, absent from the bundled dotcom description",
	"DELETE /repos/{}/{}/branches/{}/protection/allow_deletions":     "Branch protection allow-deletions setting — real GitHub endpoint, absent from the bundled dotcom description",
	"GET /repos/{}/{}/branches/{}/protection/allow_force_pushes":     "Branch protection allow-force-pushes setting — real GitHub endpoint, absent from the bundled dotcom description",
	"PUT /repos/{}/{}/branches/{}/protection/allow_force_pushes":     "Branch protection allow-force-pushes setting — real GitHub endpoint, absent from the bundled dotcom description",
	"DELETE /repos/{}/{}/branches/{}/protection/allow_force_pushes":  "Branch protection allow-force-pushes setting — real GitHub endpoint, absent from the bundled dotcom description",
	"PUT /repos/{}/{}/branches/{}/protection/required_status_checks": "Branch protection required status checks update — real GitHub endpoint, absent from the bundled dotcom description",
	"PUT /repos/{}/{}/branches/{}/protection/restrictions":           "Branch protection push restrictions update — real GitHub endpoint, absent from the bundled dotcom description",
	"GET /repos/{}/{}/projects":                                      "Projects classic (v1) repo-scoped list — real GitHub endpoint, absent from the bundled dotcom description",
	"POST /repos/{}/{}/projects":                                     "Projects classic (v1) repo-scoped create — real GitHub endpoint, absent from the bundled dotcom description",
	"PATCH /repos/{}/{}/secret-scanning/alerts":                      "Secret scanning bulk update by query — real GitHub endpoint, absent from the bundled dotcom description",
	"GET /orgs/{}/migrations/{}/repos/{}/lock":                       "Organization migration repository lock status — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"GET /projects/{}":                                               "Projects classic (v1) get project — real GitHub endpoint, absent from the bundled dotcom description",
	"PATCH /projects/{}":                                             "Projects classic (v1) update project — real GitHub endpoint, absent from the bundled dotcom description",
	"DELETE /projects/{}":                                            "Projects classic (v1) delete project — real GitHub endpoint, absent from the bundled dotcom description",
	"POST /projects/columns/{}/moves":                                "Projects classic (v1) move column — real GitHub endpoint, absent from the bundled dotcom description",
	"POST /projects/columns/cards/{}/moves":                          "Projects classic (v1) move card — real GitHub endpoint, absent from the bundled dotcom description",
	"GET /users/{}/codespaces":                                       "GHES user codespaces list by username — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"GET /repos/{}/{}/codespaces/{}":                                 "GHES repo-scoped codespace fetch — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"DELETE /repos/{}/{}/codespaces/{}":                              "GHES repo-scoped codespace deletion — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"POST /repos/{}/{}/codespaces/{}/start":                          "GHES repo-scoped codespace start — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"POST /repos/{}/{}/codespaces/{}/stop":                           "GHES repo-scoped codespace stop — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"GET /repos/{}/{}/packages":                                      "GHES repo-scoped package list — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"GET /repos/{}/{}/packages/{}/{}":                                "GHES repo-scoped package get/delete — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"DELETE /repos/{}/{}/packages/{}/{}":                             "GHES repo-scoped package delete — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"GET /repos/{}/{}/packages/{}/{}/versions":                       "GHES repo-scoped package version list — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"GET /repos/{}/{}/packages/{}/{}/versions/{}":                    "GHES repo-scoped package version get/delete — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"DELETE /repos/{}/{}/packages/{}/{}/versions/{}":                 "GHES repo-scoped package version delete — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"GET /repos/{}/{}/packages/{}/{}/versions/{}/files":              "GHES repo-scoped package version file list — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"GET /repos/{}/{}/packages/{}/{}/versions/{}/files/{}":           "GHES repo-scoped package version file download — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"GET /repos/{}/{}/pages/deployments/{}/status":                   "GitHub Pages deployment status endpoint, absent from the bundled dotcom description",
	"GET /users/{}/packages/{}/{}/versions/{}/files":                 "GHES user-scoped package version file list — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"GET /users/{}/packages/{}/{}/versions/{}/files/{}":              "GHES user-scoped package version file download — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"PUT /users/{}/site_admin":                                       "GHES admin/staff-tools API (promote user to site admin)",
	"DELETE /users/{}/site_admin":                                    "GHES admin/staff-tools API (demote user from site admin)",
	"PUT /users/{}/suspended":                                        "GHES admin/staff-tools API (suspend user)",
	"DELETE /users/{}/suspended":                                     "GHES admin/staff-tools API (unsuspend user)",
	"GET /orgs/{}/packages/{}/{}/versions/{}/files":                  "GHES org-scoped package version file list — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"GET /orgs/{}/packages/{}/{}/versions/{}/files/{}":               "GHES org-scoped package version file download — real GitHub (GHES) endpoint, absent from the bundled dotcom description",
	"GET /orgs/{}/actions/cache/retention-limit":                     "GHES org Actions cache retention limit endpoint, absent from the bundled dotcom description",
	"PUT /orgs/{}/actions/cache/retention-limit":                     "GHES org Actions cache retention limit endpoint, absent from the bundled dotcom description",
	"GET /orgs/{}/actions/cache/storage-limit":                       "GHES org Actions cache storage limit endpoint, absent from the bundled dotcom description",
	"PUT /orgs/{}/actions/cache/storage-limit":                       "GHES org Actions cache storage limit endpoint, absent from the bundled dotcom description",
}

// dispatchRoutes are real GitHub sub-resource paths served through a single
// two-/three-segment wildcard handler because Go 1.22's ServeMux rejects
// registering a literal and a wildcard that overlap at the same position
// (e.g. /pulls/comments/{id} vs /pulls/{number}/comments). The wildcard fans
// out to the real GitHub paths listed; it is a routing implementation detail,
// not an invented path. Keyed by the normalized wildcard pattern.
var dispatchRoutes = map[string]string{
	"DELETE /repos/{}/{}/issues/{}/{}":         "→ DELETE /repos/{}/{}/issues/{}/labels/{} (remove a label)",
	"GET /repos/{}/{}/issues/{}/{}":            "→ GET /repos/{}/{}/issues/comments/{comment_id}, /issues/{number}/reactions, or /issues/events/{event_id}",
	"GET /repos/{}/{}/issues/{}/{}/{}":         "→ GET /repos/{}/{}/issues/comments/{comment_id}/reactions or /issues/{number}/dependencies/blocked_by",
	"DELETE /repos/{}/{}/issues/{}/{}/{}":      "→ DELETE /repos/{}/{}/issues/{number}/reactions/{reaction_id} (or /comments/{comment_id}/reactions/{reaction_id})",
	"GET /repos/{}/{}/git/refs/{}":             "→ GET /repos/{}/{}/git/refs/{} (single ref lookup)",
	"GET /repos/{}/{}/pulls/{}/{}":             "→ GET /repos/{}/{}/pulls/comments/{} (a review comment)",
	"PATCH /repos/{}/{}/pulls/{}/{}":           "→ PATCH /repos/{}/{}/pulls/comments/{} (edit a review comment)",
	"DELETE /repos/{}/{}/pulls/{}/{}":          "→ DELETE /repos/{}/{}/pulls/comments/{} (delete a review comment)",
	"GET /repos/{}/{}/pulls/{}/{}/{}":          "→ GET /repos/{}/{}/pulls/{number}/reviews/{review_id} or /pulls/comments/{comment_id}/reactions",
	"POST /repos/{}/{}/pulls/{}/{}/{}":         "→ POST /repos/{}/{}/pulls/comments/{comment_id}/reactions",
	"PUT /repos/{}/{}/pulls/{}/{}/{}":          "→ PUT /repos/{}/{}/pulls/{number}/reviews/{review_id}",
	"DELETE /repos/{}/{}/pulls/{}/{}/{}":       "→ DELETE /repos/{}/{}/pulls/{number}/reviews/{review_id}",
	"GET /repos/{}/{}/releases/{}/{}":          "→ GET /repos/{}/{}/releases/{}/assets (list release assets) or /releases/tags/{tag}",
	"POST /repos/{}/{}/releases/{}/{}":         "→ POST /repos/{}/{}/releases/{}/reactions (react to a release) or /releases/{release_id}/assets",
	"PATCH /repos/{}/{}/releases/{}/{}":        "→ PATCH /repos/{}/{}/releases/assets/{asset_id} (edit a release asset)",
	"DELETE /repos/{}/{}/releases/{}/{}":       "→ DELETE /repos/{}/{}/releases/assets/{asset_id} (delete a release asset)",
	"DELETE /repos/{}/{}/releases/{}/{}/{}":    "→ DELETE /repos/{}/{}/releases/{}/reactions/{} (remove a release reaction)",
	"GET /orgs/{}/rulesets/{}/{}":              "→ GET /orgs/{org}/rulesets/rule-suites/{rule_suite_id} or /orgs/{org}/rulesets/{ruleset_id}/history",
	"GET /repos/{}/{}/rulesets/{}/{}":          "→ GET /repos/{}/{}/rulesets/rule-suites/{rule_suite_id} or /repos/{}/{}/rulesets/{ruleset_id}/history",
	"GET /orgs/{}/rulesets/{}/{}/{}":           "→ GET /orgs/{org}/rulesets/{ruleset_id}/history/{version_id}",
	"GET /projects/{}/{}":                      "→ GET /projects/{project_id}/columns or GET /projects/columns/{column_id} (Projects classic dispatch)",
	"POST /projects/{}/{}":                     "→ POST /projects/{project_id}/columns (Projects classic dispatch)",
	"PATCH /projects/{}/{}":                    "→ PATCH /projects/columns/{column_id} (Projects classic dispatch)",
	"DELETE /projects/{}/{}":                   "→ DELETE /projects/columns/{column_id} (Projects classic dispatch)",
	"GET /projects/columns/{}/{}":              "→ GET /projects/columns/{column_id}/cards or GET /projects/columns/cards/{card_id} (Projects classic dispatch)",
	"POST /projects/columns/{}/{}":             "→ POST /projects/columns/{column_id}/cards (Projects classic dispatch)",
	"PATCH /projects/columns/{}/{}":            "→ PATCH /projects/columns/cards/{card_id} (Projects classic dispatch)",
	"DELETE /projects/columns/{}/{}":           "→ DELETE /projects/columns/cards/{card_id} (Projects classic dispatch)",
	"POST /repos/{}/{}/security-advisories/{}": "→ POST /repos/{}/{}/security-advisories/reports (report vulnerability)",
	"GET /user/codespaces/{}/{}":               "→ GET /user/codespaces/{codespace_name}/machines (conflicts with the literal /user/codespaces/secrets/{secret_name})",
	"GET /user/codespaces/{}/{}/{}":            "→ GET /user/codespaces/{codespace_name}/exports/{export_id} (conflicts with the literal /user/codespaces/secrets/{secret_name}/repositories)",
}

func TestRegisteredAPIv3RoutesExistInGitHubSpec(t *testing.T) {
	s := newTestServer()
	s.registerRoutes()
	ghOps := loadGitHubOperations(t)

	var offenders []string
	for _, pat := range s.routePatterns {
		method, path, found := strings.Cut(pat, " ")
		if !found {
			continue
		}
		// Only the GitHub-compatible REST surface is validated here.
		// /api/graphql, /_apis (runner protocol), /internal (sim-control),
		// /login (OAuth), and /.well-known are out of scope for the REST spec.
		if !strings.HasPrefix(path, "/api/v3/") {
			continue
		}
		norm := method + " " + normalizePath(strings.TrimPrefix(path, "/api/v3"))
		if ghOps[norm] {
			continue
		}
		if _, ok := allowedGHESOnly[norm]; ok {
			continue
		}
		if _, ok := dispatchRoutes[norm]; ok {
			continue
		}
		offenders = append(offenders, pat+"  (normalized: "+norm+")")
	}

	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Errorf("%d /api/v3 route(s) are not real GitHub API paths (invented under the GitHub namespace, "+
			"a parameter/path-shape mismatch, or a real GHES endpoint that must be added to allowedGHESOnly):\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}
