package gcpcommon

import (
	"context"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// TestRegistryCoordinate: the one coordinate yields the host image references
// carry and the URL registry HTTP is dialed at, for the real registry and a
// relocated one with and without a scheme.
func TestRegistryCoordinate(t *testing.T) {
	cases := []struct {
		coordinate string
		host       string
		url        string
	}{
		{"", "us-central1-docker.pkg.dev", ""},
		{"127.0.0.1:4567", "127.0.0.1:4567", "https://127.0.0.1:4567"},
		{"http://127.0.0.1:4567", "127.0.0.1:4567", "http://127.0.0.1:4567"},
		{" https://registry.internal.example/ ", "registry.internal.example", "https://registry.internal.example"},
	}
	for _, tc := range cases {
		if got := OverlayRegistryHost("us-central1", tc.coordinate); got != tc.host {
			t.Errorf("OverlayRegistryHost(%q) = %q, want %q", tc.coordinate, got, tc.host)
		}
		if got := RegistryEndpointURL(tc.coordinate); got != tc.url {
			t.Errorf("RegistryEndpointURL(%q) = %q, want %q", tc.coordinate, got, tc.url)
		}
	}
	if !IsRelocatedRegistry("127.0.0.1:4567", "http://127.0.0.1:4567") {
		t.Error("the coordinate's own host is the relocated registry")
	}
	if IsRelocatedRegistry("127.0.0.1:4567", "") || IsRelocatedRegistry("ghcr.io", "http://127.0.0.1:4567") {
		t.Error("no other host is")
	}
}

// TestARAuthProviderRegistryEndpoint: the auth provider recognises and dials
// the registry from the same coordinate the references were built from — the
// real Artifact Registry host is dialed at itself, a relocated one at the
// coordinate's URL, and a foreign registry is neither ours nor relocated.
func TestARAuthProviderRegistryEndpoint(t *testing.T) {
	relocated := NewARAuthProvider(context.Background, zerolog.Nop(), "http://127.0.0.1:4567")
	if !relocated.IsCloudRegistry("127.0.0.1:4567") || !relocated.IsCloudRegistry("europe-west1-docker.pkg.dev") {
		t.Error("the relocated coordinate and the real host are both Artifact Registry")
	}
	if got := relocated.RegistryEndpoint("127.0.0.1:4567"); got != "http://127.0.0.1:4567" {
		t.Errorf("relocated RegistryEndpoint = %q", got)
	}
	if relocated.IsCloudRegistry("ghcr.io") || relocated.RegistryEndpoint("ghcr.io") != "" {
		t.Error("a foreign registry is dialed at itself with no cloud credential")
	}

	real := NewARAuthProvider(context.Background, zerolog.Nop(), "")
	if real.IsCloudRegistry("127.0.0.1:4567") {
		t.Error("with no coordinate, an address is not Artifact Registry")
	}
	if got := real.RegistryEndpoint("us-central1-docker.pkg.dev"); got != "" {
		t.Errorf("the real registry is dialed at its own host, got endpoint %q", got)
	}
}

// TestCheckTagExistsRejectsMalformedReference: a reference the probe cannot
// split is reported, not treated as a cache miss.
func TestCheckTagExistsRejectsMalformedReference(t *testing.T) {
	present, err := CheckTagExists(context.Background(), "", "no-registry-or-tag")
	if err == nil || present {
		t.Fatalf("CheckTagExists = %v, %v; want an error", present, err)
	}
	if !strings.Contains(err.Error(), "no-registry-or-tag") {
		t.Errorf("error does not name the reference: %v", err)
	}
}
