package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
[[label]]
name = "sockerless-ecs"
docker_host = "tcp://localhost:3375"
image = "ecr.example/sockerless-live:runner-amd64"

[[label]]
name = "sockerless-lambda"
docker_host = "tcp://localhost:3376"
image = "ecr.example/sockerless-live:runner-amd64"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Labels) != 2 {
		t.Fatalf("want 2 labels, got %d", len(cfg.Labels))
	}
	if l := cfg.LookupLabel("sockerless-ecs"); l == nil || l.DockerHost != "tcp://localhost:3375" {
		t.Fatalf("LookupLabel(ecs) = %+v", l)
	}
	if l := cfg.LookupLabel("nope"); l != nil {
		t.Fatalf("LookupLabel(nope) should return nil, got %+v", l)
	}
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "no-such-file.toml"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if len(cfg.Labels) != 0 {
		t.Fatalf("missing file should yield empty config, got %+v", cfg)
	}
}

func TestLoadValidationErrors(t *testing.T) {
	cases := map[string]string{
		"missing name": `[[label]]
docker_host = "tcp://x"
image = "img"`,
		"missing docker_host": `[[label]]
name = "x"
image = "img"`,
		"missing image": `[[label]]
name = "x"
docker_host = "tcp://x"`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.toml")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			if _, err := Load(path); err == nil {
				t.Fatalf("Load should fail for %q", name)
			}
		})
	}
}
