package scopes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerifyMissingScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "read:user, public_repo")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{Transport: rewriteHost{addr: srv.Listener.Addr().String()}}
	err := Verify(context.Background(), client, "tok")
	if err == nil {
		t.Fatal("Verify should fail when scopes missing")
	}
	for _, want := range []string{"repo", "workflow"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message missing %q: %v", want, err)
		}
	}
}

func TestVerifyAllScopesPresent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-OAuth-Scopes", "repo, workflow, read:org")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{Transport: rewriteHost{addr: srv.Listener.Addr().String()}}
	if err := Verify(context.Background(), client, "tok"); err != nil {
		t.Fatalf("Verify should succeed: %v", err)
	}
}

func TestVerifyEmptyToken(t *testing.T) {
	if err := Verify(context.Background(), nil, ""); err == nil {
		t.Fatal("Verify should fail on empty token without hitting the network")
	}
}

func TestVerifyUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := &http.Client{Transport: rewriteHost{addr: srv.Listener.Addr().String()}}
	err := Verify(context.Background(), client, "tok")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("Verify should report 401, got: %v", err)
	}
}

// rewriteHost reroutes requests destined for api.github.com to the
// test server's address. Avoids touching the real GitHub API in tests.
type rewriteHost struct{ addr string }

func (r rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = r.addr
	req.Host = r.addr
	return http.DefaultTransport.RoundTrip(req)
}
