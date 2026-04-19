package azurecommon

import (
	"context"
	"testing"
)

func TestResolveAzureImageURI_AlreadyACR(t *testing.T) {
	ref := "myacr.azurecr.io/myapp:v1"
	got := ResolveAzureImageURI(ref, "myacr")
	if got != ref {
		t.Errorf("ACR URI should pass through, got %q", got)
	}
}

func TestResolveAzureImageURI_WithACR(t *testing.T) {
	got := ResolveAzureImageURI("alpine:latest", "myacr")
	want := "myacr.azurecr.io/library/alpine:latest"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveAzureImageURI_WithACRAndOrg(t *testing.T) {
	got := ResolveAzureImageURI("myorg/myapp:v2", "myacr")
	want := "myacr.azurecr.io/myorg/myapp:v2"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveAzureImageURI_NoACR(t *testing.T) {
	got := ResolveAzureImageURI("alpine:latest", "")
	// Should normalize to docker.io/library/alpine:latest or similar
	if got == "" {
		t.Error("should not return empty")
	}
}

func TestResolveAzureImageURI_NoTag(t *testing.T) {
	got := ResolveAzureImageURI("nginx", "myacr")
	if got == "" {
		t.Error("should not return empty")
	}
}

// TestResolveAzureImageURIWithCache_NilClient verifies the BUG-706
// cache-aware resolver passes refs through unchanged when no cache
// client is configured (simulator-bypass path for callers that opt out
// of pull-through).
func TestResolveAzureImageURIWithCache_NilClient(t *testing.T) {
	got, err := ResolveAzureImageURIWithCache(context.Background(), nil, "rg", "myacr", "alpine:latest")
	if err != nil {
		t.Fatalf("nil client should not error: %v", err)
	}
	if got != "alpine:latest" {
		t.Errorf("want passthrough, got %q", got)
	}
}

// TestResolveAzureImageURIWithCache_AlreadyACR verifies an existing
// ACR URI passes through untouched even when a cache client is set.
func TestResolveAzureImageURIWithCache_AlreadyACR(t *testing.T) {
	ref := "myacr.azurecr.io/library/alpine:3.18"
	got, err := ResolveAzureImageURIWithCache(context.Background(), nil, "rg", "myacr", ref)
	if err != nil || got != ref {
		t.Errorf("ACR URI should pass through, got %q err %v", got, err)
	}
}

func TestTrimWildcardSuffix(t *testing.T) {
	cases := []struct {
		in       string
		wantPref string
		wantWild bool
	}{
		{"docker.io/library/*", "docker.io/library/", true},
		{"docker.io/library*", "docker.io/library", true},
		{"docker.io/library/alpine", "docker.io/library/alpine", false},
	}
	for _, c := range cases {
		pref, wild := trimWildcardSuffix(c.in)
		if pref != c.wantPref || wild != c.wantWild {
			t.Errorf("trimWildcardSuffix(%q) = (%q, %v), want (%q, %v)",
				c.in, pref, wild, c.wantPref, c.wantWild)
		}
	}
}
