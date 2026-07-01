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
}

// dispatchRoutes are real GitHub sub-resource paths served through a single
// two-/three-segment wildcard handler because Go 1.22's ServeMux rejects
// registering a literal and a wildcard that overlap at the same position
// (e.g. /pulls/comments/{id} vs /pulls/{number}/comments). The wildcard fans
// out to the real GitHub paths listed; it is a routing implementation detail,
// not an invented path. Keyed by the normalized wildcard pattern.
var dispatchRoutes = map[string]string{
	"DELETE /repos/{}/{}/issues/{}/{}":      "→ DELETE /repos/{}/{}/issues/{}/labels/{} (remove a label)",
	"GET /repos/{}/{}/git/refs/{}":          "→ GET /repos/{}/{}/git/refs/{} (single ref lookup)",
	"GET /repos/{}/{}/pulls/{}/{}":          "→ GET /repos/{}/{}/pulls/comments/{} (a review comment)",
	"PATCH /repos/{}/{}/pulls/{}/{}":        "→ PATCH /repos/{}/{}/pulls/comments/{} (edit a review comment)",
	"DELETE /repos/{}/{}/pulls/{}/{}":       "→ DELETE /repos/{}/{}/pulls/comments/{} (delete a review comment)",
	"GET /repos/{}/{}/releases/{}/{}":       "→ GET /repos/{}/{}/releases/{}/assets (list release assets)",
	"POST /repos/{}/{}/releases/{}/{}":      "→ POST /repos/{}/{}/releases/{}/reactions (react to a release)",
	"DELETE /repos/{}/{}/releases/{}/{}/{}": "→ DELETE /repos/{}/{}/releases/{}/reactions/{} (remove a release reaction)",
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
