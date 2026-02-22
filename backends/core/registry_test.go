package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/sockerless/api"
)

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		input    string
		registry string
		repo     string
		tag      string
	}{
		{"alpine", "registry-1.docker.io", "library/alpine", "latest"},
		{"alpine:3.18", "registry-1.docker.io", "library/alpine", "3.18"},
		{"nginx:latest", "registry-1.docker.io", "library/nginx", "latest"},
		{"myuser/myimage:v1", "registry-1.docker.io", "myuser/myimage", "v1"},
		{"ghcr.io/owner/repo:tag", "ghcr.io", "owner/repo", "tag"},
		{"gcr.io/project/image:v2", "gcr.io", "project/image", "v2"},
		{"docker.io/library/alpine:3.19", "registry-1.docker.io", "library/alpine", "3.19"},
		{"docker.io/myuser/myimage:latest", "registry-1.docker.io", "myuser/myimage", "latest"},
		{"localhost:5000/myimage:test", "localhost:5000", "myimage", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			rc := parseImageRef(tt.input)
			if rc.Registry != tt.registry {
				t.Errorf("registry: got %q, want %q", rc.Registry, tt.registry)
			}
			if rc.Repository != tt.repo {
				t.Errorf("repo: got %q, want %q", rc.Repository, tt.repo)
			}
			if rc.Tag != tt.tag {
				t.Errorf("tag: got %q, want %q", rc.Tag, tt.tag)
			}
		})
	}
}

func TestParseWWWAuthenticate(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		realm   string
		service string
		scope   string
	}{
		{
			name:    "docker hub",
			header:  `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/alpine:pull"`,
			realm:   "https://auth.docker.io/token",
			service: "registry.docker.io",
			scope:   "repository:library/alpine:pull",
		},
		{
			name:    "ghcr",
			header:  `Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:owner/repo:pull"`,
			realm:   "https://ghcr.io/token",
			service: "ghcr.io",
			scope:   "repository:owner/repo:pull",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			realm, params := parseWWWAuthenticate(tt.header)
			if realm != tt.realm {
				t.Errorf("realm: got %q, want %q", realm, tt.realm)
			}
			if params["service"] != tt.service {
				t.Errorf("service: got %q, want %q", params["service"], tt.service)
			}
			if params["scope"] != tt.scope {
				t.Errorf("scope: got %q, want %q", params["scope"], tt.scope)
			}
		})
	}
}

func TestSplitAuthParams(t *testing.T) {
	input := `realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/alpine:pull"`
	parts := splitAuthParams(input)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d: %v", len(parts), parts)
	}
}

func TestFetchImageConfigDisabled(t *testing.T) {
	os.Unsetenv("SOCKERLESS_FETCH_IMAGE_CONFIG")
	cfg, err := FetchImageConfig("alpine:latest")
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		t.Error("expected nil config when disabled")
	}
}

func TestFetchImageConfigCaching(t *testing.T) {
	// Clear cache
	imageConfigCache.Lock()
	imageConfigCache.m = make(map[string]*api.ContainerConfig)
	imageConfigCache.Unlock()

	os.Setenv("SOCKERLESS_FETCH_IMAGE_CONFIG", "true")
	defer os.Unsetenv("SOCKERLESS_FETCH_IMAGE_CONFIG")

	// Pre-populate cache
	testCfg := &api.ContainerConfig{
		Cmd:        []string{"cached-cmd"},
		WorkingDir: "/cached",
	}
	imageConfigCache.Lock()
	imageConfigCache.m["cached-image:latest"] = testCfg
	imageConfigCache.Unlock()

	// Should return cached value without any network call
	cfg, err := FetchImageConfig("cached-image:latest")
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		t.Fatal("expected cached config, got nil")
	}
	if cfg.WorkingDir != "/cached" {
		t.Errorf("WorkingDir: got %q, want %q", cfg.WorkingDir, "/cached")
	}
	if len(cfg.Cmd) != 1 || cfg.Cmd[0] != "cached-cmd" {
		t.Errorf("Cmd: got %v, want [cached-cmd]", cfg.Cmd)
	}

	// Clean up cache
	imageConfigCache.Lock()
	imageConfigCache.m = make(map[string]*api.ContainerConfig)
	imageConfigCache.Unlock()
}

// TestMockRegistryEndToEnd tests the full registry v2 API flow using an
// httptest server. Since fetchConfigFromRegistry uses https://, we test
// the individual HTTP requests against the mock directly.
func TestMockRegistryEndToEnd(t *testing.T) {
	wantEnv := []string{"PATH=/usr/local/bin:/usr/bin", "PYTHON_VERSION=3.12"}
	wantCmd := []string{"python3"}
	wantEntrypoint := []string{"/docker-entrypoint.sh"}
	wantWorkingDir := "/app"
	wantLabels := map[string]string{"maintainer": "test"}

	configJSON, _ := json.Marshal(map[string]any{
		"architecture": "amd64",
		"os":           "linux",
		"config": map[string]any{
			"Env":        wantEnv,
			"Cmd":        wantCmd,
			"Entrypoint": wantEntrypoint,
			"WorkingDir": wantWorkingDir,
			"Labels":     wantLabels,
		},
	})

	configDigest := "sha256:cfg0000000000000000000000000000000000000000000000000000000000000"
	manifestJSON, _ := json.Marshal(map[string]any{
		"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		"config": map[string]any{
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"digest":    configDigest,
		},
	})

	// Use a pointer so handlers can reference the server URL
	var mockSrv *httptest.Server
	mux := http.NewServeMux()

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"token": "test-tok"})
	})

	mux.HandleFunc("/v2/library/testimg/manifests/v1", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("Www-Authenticate", fmt.Sprintf(
				`Bearer realm="%s/token",service="test",scope="repository:library/testimg:pull"`,
				mockSrv.URL))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if auth != "Bearer test-tok" {
			t.Errorf("unexpected auth: %s", auth)
		}
		w.Header().Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
		w.Write(manifestJSON)
	})

	mux.HandleFunc("/v2/library/testimg/blobs/"+configDigest, func(w http.ResponseWriter, r *http.Request) {
		w.Write(configJSON)
	})

	mockSrv = httptest.NewServer(mux)
	defer mockSrv.Close()

	// Step 1: Unauthenticated request returns 401 with Www-Authenticate
	resp, err := http.Get(mockSrv.URL + "/v2/library/testimg/manifests/v1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}

	// Step 2: Parse the auth challenge
	authHeader := resp.Header.Get("Www-Authenticate")
	realm, params := parseWWWAuthenticate(authHeader)
	if realm != mockSrv.URL+"/token" {
		t.Errorf("realm: got %q, want %q", realm, mockSrv.URL+"/token")
	}

	// Step 3: Get token
	tokenResp, err := http.Get(realm + "?service=" + params["service"] + "&scope=" + params["scope"])
	if err != nil {
		t.Fatal(err)
	}
	defer tokenResp.Body.Close()
	var tr tokenResponse
	json.NewDecoder(tokenResp.Body).Decode(&tr)
	if tr.Token != "test-tok" {
		t.Errorf("token: got %q, want %q", tr.Token, "test-tok")
	}

	// Step 4: Fetch manifest with token
	req, _ := http.NewRequest("GET", mockSrv.URL+"/v2/library/testimg/manifests/v1", nil)
	req.Header.Set("Authorization", "Bearer "+tr.Token)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	mResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer mResp.Body.Close()

	var sm singleManifest
	json.NewDecoder(mResp.Body).Decode(&sm)
	if sm.Config.Digest != configDigest {
		t.Errorf("config digest: got %q, want %q", sm.Config.Digest, configDigest)
	}

	// Step 5: Fetch config blob
	bResp, err := http.Get(mockSrv.URL + "/v2/library/testimg/blobs/" + configDigest)
	if err != nil {
		t.Fatal(err)
	}
	defer bResp.Body.Close()

	var ociCfg ociImageConfig
	json.NewDecoder(bResp.Body).Decode(&ociCfg)

	if ociCfg.Config.WorkingDir != wantWorkingDir {
		t.Errorf("WorkingDir: got %q, want %q", ociCfg.Config.WorkingDir, wantWorkingDir)
	}
	if len(ociCfg.Config.Env) != 2 {
		t.Errorf("Env count: got %d, want 2", len(ociCfg.Config.Env))
	} else {
		if ociCfg.Config.Env[0] != wantEnv[0] {
			t.Errorf("Env[0]: got %q, want %q", ociCfg.Config.Env[0], wantEnv[0])
		}
	}
	if len(ociCfg.Config.Cmd) != 1 || ociCfg.Config.Cmd[0] != "python3" {
		t.Errorf("Cmd: got %v, want [python3]", ociCfg.Config.Cmd)
	}
	if len(ociCfg.Config.Entrypoint) != 1 || ociCfg.Config.Entrypoint[0] != "/docker-entrypoint.sh" {
		t.Errorf("Entrypoint: got %v, want [/docker-entrypoint.sh]", ociCfg.Config.Entrypoint)
	}
	if ociCfg.Config.Labels["maintainer"] != "test" {
		t.Errorf("Labels: got %v, want {maintainer: test}", ociCfg.Config.Labels)
	}
}

// TestMockRegistryManifestList tests the manifest list → platform manifest → config chain.
func TestMockRegistryManifestList(t *testing.T) {
	configJSON, _ := json.Marshal(map[string]any{
		"architecture": "amd64",
		"os":           "linux",
		"config": map[string]any{
			"Env":        []string{"PATH=/usr/local/bin"},
			"Cmd":        []string{"/bin/sh"},
			"WorkingDir": "/home",
		},
	})

	configDigest := "sha256:cfgml000000000000000000000000000000000000000000000000000000000"
	platformDigest := "sha256:platf000000000000000000000000000000000000000000000000000000000"

	platformManifestJSON, _ := json.Marshal(map[string]any{
		"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
		"config": map[string]any{
			"mediaType": "application/vnd.docker.container.image.v1+json",
			"digest":    configDigest,
		},
	})

	manifestListJSON, _ := json.Marshal(map[string]any{
		"mediaType": "application/vnd.docker.distribution.manifest.list.v2+json",
		"manifests": []map[string]any{
			{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"digest":    platformDigest,
				"platform": map[string]string{
					"architecture": "arm64",
					"os":           "linux",
				},
			},
			{
				"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
				"digest":    platformDigest,
				"platform": map[string]string{
					"architecture": "amd64",
					"os":           "linux",
				},
			},
		},
	})

	mux := http.NewServeMux()

	mux.HandleFunc("/v2/library/multiarch/manifests/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.docker.distribution.manifest.list.v2+json")
		w.Write(manifestListJSON)
	})

	mux.HandleFunc("/v2/library/multiarch/manifests/"+platformDigest, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
		w.Write(platformManifestJSON)
	})

	mux.HandleFunc("/v2/library/multiarch/blobs/"+configDigest, func(w http.ResponseWriter, r *http.Request) {
		w.Write(configJSON)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Fetch the manifest list
	req, _ := http.NewRequest("GET", srv.URL+"/v2/library/multiarch/manifests/latest", nil)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.list.v2+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var ml manifestList
	json.NewDecoder(resp.Body).Decode(&ml)
	if len(ml.Manifests) != 2 {
		t.Fatalf("expected 2 manifests in list, got %d", len(ml.Manifests))
	}

	// Find amd64 platform
	var amd64Digest string
	for _, m := range ml.Manifests {
		if m.Platform.Architecture == "amd64" {
			amd64Digest = m.Digest
			break
		}
	}
	if amd64Digest == "" {
		t.Fatal("no amd64 manifest found")
	}

	// Fetch platform manifest
	pResp, err := http.Get(srv.URL + "/v2/library/multiarch/manifests/" + amd64Digest)
	if err != nil {
		t.Fatal(err)
	}
	defer pResp.Body.Close()

	var sm singleManifest
	json.NewDecoder(pResp.Body).Decode(&sm)
	if sm.Config.Digest != configDigest {
		t.Errorf("config digest: got %q, want %q", sm.Config.Digest, configDigest)
	}

	// Fetch config blob
	cResp, err := http.Get(srv.URL + "/v2/library/multiarch/blobs/" + configDigest)
	if err != nil {
		t.Fatal(err)
	}
	defer cResp.Body.Close()

	var ociCfg ociImageConfig
	json.NewDecoder(cResp.Body).Decode(&ociCfg)
	if ociCfg.Config.WorkingDir != "/home" {
		t.Errorf("WorkingDir: got %q, want %q", ociCfg.Config.WorkingDir, "/home")
	}
}

// TestFetchImageConfigGracefulFallback tests that network errors result in
// nil config (not an error), allowing the caller to use synthetic config.
func TestFetchImageConfigGracefulFallback(t *testing.T) {
	// Clear cache
	imageConfigCache.Lock()
	imageConfigCache.m = make(map[string]*api.ContainerConfig)
	imageConfigCache.Unlock()

	os.Setenv("SOCKERLESS_FETCH_IMAGE_CONFIG", "true")
	defer os.Unsetenv("SOCKERLESS_FETCH_IMAGE_CONFIG")

	// Try to fetch from a non-existent registry — should return nil, nil
	cfg, err := FetchImageConfig("nonexistent.invalid/image:tag")
	if err != nil {
		t.Errorf("expected nil error for network failure, got: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config for network failure")
	}

	// Clean up cache
	imageConfigCache.Lock()
	imageConfigCache.m = make(map[string]*api.ContainerConfig)
	imageConfigCache.Unlock()
}
