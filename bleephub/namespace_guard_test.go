package bleephub

import (
	"strings"
	"testing"
)

// TestNoInventedGitHubNamespacePaths guards the fidelity invariant directly
// against the registered route table (Server.RegisteredRoutes), NOT by probing
// the catch-all fallback. The GitHub-compatible surface (/api/v3, /api/graphql)
// must contain ONLY real GitHub paths; sim-control endpoints with no GitHub
// equivalent live under /internal/. A misregistered or missing route is a
// visible failure here rather than a silent catch-all 404.
func TestNoInventedGitHubNamespacePaths(t *testing.T) {
	s := newTestServer()
	s.registerRoutes()
	routes := s.routePatterns // every pattern registered via route(), catch-all excluded
	if len(routes) == 0 {
		t.Fatal("no routes registered — registry not wired")
	}

	registered := make(map[string]bool, len(routes))
	for _, r := range routes {
		registered[r] = true
	}

	// 1. No route under the GitHub API namespace may carry a sim-only segment.
	//    `bleephub`/`sim`/`internal` are not real GitHub path segments.
	for _, r := range routes {
		_, path, found := strings.Cut(r, " ")
		if !found {
			path = r
		}
		if strings.HasPrefix(path, "/api/v3/") || strings.HasPrefix(path, "/api/graphql") {
			for _, bad := range []string{"/bleephub", "/sim/", "/internal"} {
				if strings.Contains(path, bad) {
					t.Errorf("invented segment %q under GitHub namespace: %q", bad, r)
				}
			}
		}
	}

	// 2. The relocated sim-control routes must exist under /internal/.
	for _, want := range []string{
		"POST /internal/exec/submit",
		"GET /internal/exec/jobs/{jobId}",
		"POST /internal/exec/workflow",
		"GET /internal/exec/workflows/{workflowId}",
		"POST /internal/exec/workflows/{workflowId}/cancel",
	} {
		if !registered[want] {
			t.Errorf("relocated sim-control route missing from registry: %q", want)
		}
	}

	// 3. The former invented paths must be gone from the registry entirely.
	for _, gone := range routes {
		if strings.Contains(gone, "/api/v3/bleephub") {
			t.Errorf("former invented path still registered: %q", gone)
		}
	}
	for _, gone := range []string{
		"GET /internal/apps",
		"POST /internal/apps",
		"POST /internal/apps/{app_id}/installations",
		"GET /internal/installations",
		"POST /internal/installations/{id}/suspend",
		"POST /internal/installations/{id}/unsuspend",
		"DELETE /internal/installations/{id}",
		"POST /internal/oauth-apps",
		"GET /internal/oauth-apps",
	} {
		if registered[gone] {
			t.Errorf("legacy app-management route still registered: %q", gone)
		}
	}
}
