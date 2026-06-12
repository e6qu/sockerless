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

	err := Verify(context.Background(), srv.Client(), srv.URL, "o/r", "tok")
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

	if err := Verify(context.Background(), srv.Client(), srv.URL, "o/r", "tok"); err != nil {
		t.Fatalf("Verify should succeed: %v", err)
	}
}

func TestVerifyEmptyToken(t *testing.T) {
	if err := Verify(context.Background(), nil, "", "o/r", ""); err == nil {
		t.Fatal("Verify should fail on empty token without hitting the network")
	}
}

func TestVerifyUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := Verify(context.Background(), srv.Client(), srv.URL, "o/r", "tok")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("Verify should report 401, got: %v", err)
	}
}

// TestVerifyCapabilityProbe pins the fine-grained-PAT / GHES path: no
// X-OAuth-Scopes header → verify by minting a registration token.
func TestVerifyCapabilityProbe(t *testing.T) {
	var probed string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/user":
			w.WriteHeader(http.StatusOK) // no X-OAuth-Scopes header
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/actions/runners/registration-token"):
			probed = r.URL.Path
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"token":"t","expires_at":"2099-01-01T00:00:00Z"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	if err := Verify(context.Background(), srv.Client(), srv.URL, "owner/repo", "tok"); err != nil {
		t.Fatalf("capability probe should pass: %v", err)
	}
	if probed != "/repos/owner/repo/actions/runners/registration-token" {
		t.Fatalf("probe hit %q", probed)
	}
}

// TestVerifyCapabilityProbeDenied: header-less token that CANNOT mint
// registration tokens must fail with permission guidance.
func TestVerifyCapabilityProbeDenied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/user" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	err := Verify(context.Background(), srv.Client(), srv.URL, "owner/repo", "tok")
	if err == nil || !strings.Contains(err.Error(), "cannot mint runner registration tokens") {
		t.Fatalf("want capability-denied guidance, got: %v", err)
	}
}
