package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ociListFake spins up an httptest.Server that speaks the minimum slice
// of the OCI distribution v2 API that OCIListImages needs: /v2/_catalog
// + /v2/<repo>/tags/list, both returning JSON. Bearer auth is enforced
// when `authToken` is non-empty so the tests can confirm OCIListImages
// passes the token through.
type ociListFake struct {
	repos     map[string][]string // repo → tags
	authToken string
	server    *httptest.Server
}

func newOCIListFake(t *testing.T, repos map[string][]string, authToken string) *ociListFake {
	t.Helper()
	f := &ociListFake{repos: repos, authToken: authToken}
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/_catalog", func(w http.ResponseWriter, r *http.Request) {
		if !f.checkAuth(w, r) {
			return
		}
		names := make([]string, 0, len(f.repos))
		for name := range f.repos {
			names = append(names, name)
		}
		_ = json.NewEncoder(w).Encode(map[string][]string{"repositories": names})
	})
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		if !f.checkAuth(w, r) {
			return
		}
		// path is /v2/<repo>/tags/list
		if !strings.HasSuffix(r.URL.Path, "/tags/list") {
			http.NotFound(w, r)
			return
		}
		repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/"), "/tags/list")
		tags, ok := f.repos[repo]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"name": repo, "tags": tags})
	})
	f.server = httptest.NewTLSServer(mux)
	return f
}

func (f *ociListFake) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	if f.authToken == "" {
		return true
	}
	if r.Header.Get("Authorization") != f.authToken {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}

func (f *ociListFake) close() { f.server.Close() }

// host returns the registry hostname (no scheme, no port query formatting).
func (f *ociListFake) host() string {
	return strings.TrimPrefix(f.server.URL, "https://")
}

// TestOCIListImages_HappyPath — two repos, each with tags, catalog +
// per-repo tag queries produce ImageSummaries with fully-qualified
// RepoTags.
func TestOCIListImages_HappyPath(t *testing.T) {
	f := newOCIListFake(t, map[string][]string{
		"alpine":      {"latest", "3.19"},
		"myorg/tools": {"v1", "v2"},
	}, "")
	defer f.close()

	// OCIListImages uses https by default; override the httptest-server's
	// shared transport so the test can ignore the self-signed cert.
	old := ociListClient
	ociListClient = f.server.Client()
	defer func() { ociListClient = old }()

	got, err := OCIListImages(context.Background(), OCIListOptions{
		Registry: f.host(),
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d summaries, want 4 (2 repos × 2 tags)", len(got))
	}
	seen := make(map[string]bool)
	for _, s := range got {
		if len(s.RepoTags) != 1 {
			t.Errorf("summary %s: expected 1 RepoTag, got %v", s.ID, s.RepoTags)
			continue
		}
		seen[s.RepoTags[0]] = true
	}
	for _, want := range []string{
		f.host() + "/alpine:latest",
		f.host() + "/alpine:3.19",
		f.host() + "/myorg/tools:v1",
		f.host() + "/myorg/tools:v2",
	} {
		if !seen[want] {
			t.Errorf("missing RepoTag %q in result", want)
		}
	}
}

// TestOCIListImages_EmptyRegistry returns an empty slice without error
// when the catalog is empty.
func TestOCIListImages_EmptyRegistry(t *testing.T) {
	f := newOCIListFake(t, map[string][]string{}, "")
	defer f.close()
	old := ociListClient
	ociListClient = f.server.Client()
	defer func() { ociListClient = old }()

	got, err := OCIListImages(context.Background(), OCIListOptions{Registry: f.host()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 summaries, got %d", len(got))
	}
}

// TestOCIListImages_BearerTokenPropagated verifies that the bearer
// token from OCIListOptions is forwarded to the registry as
// `Authorization: <token>` on every request.
func TestOCIListImages_BearerTokenPropagated(t *testing.T) {
	token := "Bearer test-token-xyz"
	f := newOCIListFake(t, map[string][]string{"repo": {"tag"}}, token)
	defer f.close()
	old := ociListClient
	ociListClient = f.server.Client()
	defer func() { ociListClient = old }()

	// Wrong token → 401 → catalog fails → OCIListImages returns error.
	_, err := OCIListImages(context.Background(), OCIListOptions{
		Registry:  f.host(),
		AuthToken: "Bearer wrong-token",
	})
	if err == nil {
		t.Fatal("expected error with wrong token")
	}

	// Correct token → 200 → success.
	got, err := OCIListImages(context.Background(), OCIListOptions{
		Registry:  f.host(),
		AuthToken: token,
	})
	if err != nil {
		t.Fatalf("unexpected error with correct token: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(got))
	}
}

// TestOCIListImages_PerRepoFailureSwallowed — when one repo's
// /tags/list returns 404, OCIListImages skips it and still emits
// summaries for the other repos.
func TestOCIListImages_PerRepoFailureSwallowed(t *testing.T) {
	// Build a custom mux where /v2/broken/tags/list returns 500.
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/_catalog", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string][]string{"repositories": {"good", "broken"}})
	})
	mux.HandleFunc("/v2/good/tags/list", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "good", "tags": []string{"v1"}})
	})
	mux.HandleFunc("/v2/broken/tags/list", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	server := httptest.NewTLSServer(mux)
	defer server.Close()
	old := ociListClient
	ociListClient = server.Client()
	defer func() { ociListClient = old }()

	got, err := OCIListImages(context.Background(), OCIListOptions{
		Registry: strings.TrimPrefix(server.URL, "https://"),
	})
	if err != nil {
		t.Fatalf("expected catalog success + per-repo failure swallowed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 summary from the healthy repo, got %d", len(got))
	}
}
