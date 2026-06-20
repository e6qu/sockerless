package bleephub

import (
	"net/http"
	"testing"
)

// TestPrivateRepoReadVisibility asserts that repo-scoped read endpoints hide
// private-repo content from callers without access (404, matching real
// GitHub) while still serving the owner.
func TestPrivateRepoReadVisibility(t *testing.T) {
	s := newTestServer()
	s.registerRoutes()

	owner := s.store.LookupUserByLogin("admin")
	if owner == nil {
		t.Fatal("seeded admin user missing")
	}
	repo := s.store.CreateRepo(owner, "secret", "", true) // private
	if !repo.Private {
		t.Fatal("repo should be private")
	}
	// Seed a release + an issue so the list/get handlers have content to leak.
	s.store.Releases.Create(repo.ID, owner.ID, "v1", "main", "First", "notes", false, false)

	readPaths := []string{
		"/api/v3/repos/admin/secret/releases",
		"/api/v3/repos/admin/secret/releases/latest",
		"/api/v3/repos/admin/secret/issues",
		"/api/v3/repos/admin/secret/pulls",
		"/api/v3/repos/admin/secret/deployments",
		"/api/v3/repos/admin/secret/environments",
		"/api/v3/repos/admin/secret/actions/runs",
	}

	for _, p := range readPaths {
		// Unauthenticated → 404 (content hidden).
		w := runRequest(s, "GET", p)
		if w.Code != http.StatusNotFound {
			t.Errorf("anon GET %s: code = %d, want 404 (body=%s)", p, w.Code, w.Body.String())
		}
		// Owner (admin token) → not 404.
		wa := runAuthedRequest(s, "GET", p)
		if wa.Code == http.StatusNotFound {
			t.Errorf("owner GET %s: code = 404, want readable", p)
		}
	}
}

// TestPublicRepoReadStillWorks confirms the visibility gate is a no-op for
// public repos (anonymous reads keep working).
func TestPublicRepoReadStillWorks(t *testing.T) {
	s := newTestServer()
	s.registerRoutes()
	owner := s.store.LookupUserByLogin("admin")
	repo := s.store.CreateRepo(owner, "pub", "", false) // public
	s.store.Releases.Create(repo.ID, owner.ID, "v1", "main", "First", "notes", false, false)

	w := runRequest(s, "GET", "/api/v3/repos/admin/pub/releases")
	if w.Code != http.StatusOK {
		t.Errorf("anon GET public releases: code = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
}
