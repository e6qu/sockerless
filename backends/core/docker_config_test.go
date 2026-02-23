package core

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDockerConfig_MultiRegistry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{
		"auths": {
			"https://index.docker.io/v1/": {"auth": "` + base64.StdEncoding.EncodeToString([]byte("hubuser:hubpass")) + `"},
			"ghcr.io": {"auth": "` + base64.StdEncoding.EncodeToString([]byte("ghuser:ghtoken")) + `"}
		}
	}`
	os.WriteFile(path, []byte(content), 0644)

	cfg, err := LoadDockerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Auths) != 2 {
		t.Fatalf("expected 2 auths, got %d", len(cfg.Auths))
	}

	u, p, ok := cfg.GetRegistryAuth("ghcr.io")
	if !ok || u != "ghuser" || p != "ghtoken" {
		t.Fatalf("ghcr.io: got %q %q %v", u, p, ok)
	}
}

func TestLoadDockerConfig_EmptyAuths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"auths": {}}`), 0644)

	cfg, err := LoadDockerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Auths) != 0 {
		t.Fatalf("expected 0 auths, got %d", len(cfg.Auths))
	}
}

func TestLoadDockerConfig_MissingFile(t *testing.T) {
	cfg, err := LoadDockerConfig("/nonexistent/config.json")
	if err != nil {
		t.Fatal("expected no error for missing file")
	}
	if cfg == nil || cfg.Auths == nil {
		t.Fatal("expected non-nil config with empty auths")
	}
}

func TestLoadDockerConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`not json`), 0644)

	_, err := LoadDockerConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDockerConfig_HubAliasMatching(t *testing.T) {
	cfg := &DockerConfig{
		Auths: map[string]DockerAuthEntry{
			"https://index.docker.io/v1/": {Auth: base64.StdEncoding.EncodeToString([]byte("user:pass"))},
		},
	}

	// All Docker Hub aliases should resolve
	aliases := []string{"docker.io", "index.docker.io", "registry-1.docker.io", "https://index.docker.io/v1/"}
	for _, alias := range aliases {
		u, p, ok := cfg.GetRegistryAuth(alias)
		if !ok || u != "user" || p != "pass" {
			t.Errorf("alias %q: got %q %q %v", alias, u, p, ok)
		}
	}

	// Non-hub should not match
	_, _, ok := cfg.GetRegistryAuth("ghcr.io")
	if ok {
		t.Error("ghcr.io should not match Docker Hub auth")
	}
}

func TestDockerConfig_AuthDecode(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("myuser:mypass"))
	cfg := &DockerConfig{
		Auths: map[string]DockerAuthEntry{
			"myregistry.example.com": {Auth: encoded},
		},
	}

	u, p, ok := cfg.GetRegistryAuth("myregistry.example.com")
	if !ok || u != "myuser" || p != "mypass" {
		t.Fatalf("expected myuser:mypass, got %q:%q ok=%v", u, p, ok)
	}
}
