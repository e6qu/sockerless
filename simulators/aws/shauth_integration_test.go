package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	sim "github.com/sockerless/simulator"
	uiauth "github.com/sockerless/simulator-ui-auth"
)

// consoleAssets stands in for the embedded single-page application. The
// simulator mounts its Shauth relying party from RegisterUI, which the
// production build reaches through registerUI with the embedded `dist`
// directory — and which the `noui` build compiles away entirely. Handing
// RegisterUI a filesystem directly exercises the mounting contract itself, so
// these tests assert the wiring under every build tag rather than only the one
// that embeds the console.
func consoleAssets() fstest.MapFS {
	return fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>console</title>")}}
}

// Shauth is an ADDITION to the AWS simulator's own authentication, never a
// replacement for it: the AWS surface keeps demanding a valid Signature
// Version 4 request, and Shauth sits alongside as the console's OpenID Connect
// relying party whose assertion the console federates through
// AssumeRoleWithWebIdentity into real AWS credentials.
//
// The ui-auth package is unit-tested on its own, which proves the package and
// not the wiring. This asserts the AWS simulator actually mounts it, so the
// integration cannot regress unnoticed. simulators/gcp and simulators/azure
// carry the same test; the contract is shared and the three must not drift.

func awsShauthConfig() sim.Config {
	return sim.Config{
		Provider:                   "aws",
		ListenAddr:                 ":0",
		LogLevel:                   "error",
		UIOIDCIssuer:               "https://shauth.example.com",
		UIOIDCClientID:             "sockerless-aws",
		UIOIDCClientSecret:         "test-client-secret",
		UIPublicURL:                "https://aws.example.com",
		UISessionSecret:            "0123456789abcdef0123456789abcdef",
		ApplicationReleaseRevision: "0123456789abcdef0123456789abcdef01234567",
	}
}

func TestShauthIsMountedAlongsideAWSAuth(t *testing.T) {
	t.Setenv("SIM_RUNTIME", "process")
	srv, _, _, err := buildSimulatorWithOptions(awsShauthConfig(), simulatorBuildOptions{})
	if err != nil {
		t.Fatalf("buildSimulator with Shauth configured: %v", err)
	}
	srv.RegisterUI(consoleAssets())

	get := func(path string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec.Code
	}

	for _, path := range []string{
		uiauth.LoginPath,
		uiauth.CallbackPath,
		uiauth.SessionPath,
		uiauth.FederationSubjectPath,
		uiauth.FrontchannelLogoutPath,
		uiauth.LogoutCompletePath,
		uiauth.SignedOutPath,
		uiauth.ValidationPath,
	} {
		if code := get(path); code == http.StatusNotFound {
			t.Errorf("%s is not routed — Shauth is not mounted on the aws simulator", path)
		}
	}

	if code := get(uiauth.SessionPath); code != http.StatusUnauthorized {
		t.Errorf("%s anonymously = %d, want 401", uiauth.SessionPath, code)
	}
	if code := get(uiauth.FederationSubjectPath); code != http.StatusUnauthorized {
		t.Errorf("%s anonymously = %d, want 401", uiauth.FederationSubjectPath, code)
	}
}

// TestShauthAbsentWhenUnconfiguredAWS pins the opt-in half: with no identity
// provider coordinate the simulator serves no relying party at all.
func TestShauthAbsentWhenUnconfiguredAWS(t *testing.T) {
	t.Setenv("SIM_RUNTIME", "process")
	srv, _, _, err := buildSimulatorWithOptions(
		sim.Config{Provider: "aws", ListenAddr: ":0", LogLevel: "error"}, simulatorBuildOptions{})
	if err != nil {
		t.Fatalf("buildSimulator: %v", err)
	}
	srv.RegisterUI(consoleAssets())
	req := httptest.NewRequest(http.MethodGet, uiauth.SessionPath, nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("%s with no identity provider configured = %d, want 404", uiauth.SessionPath, rec.Code)
	}
}
