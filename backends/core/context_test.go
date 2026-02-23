package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestLoadContextEnvNoContext(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SOCKERLESS_HOME", tmp)
	t.Setenv("SOCKERLESS_CONTEXT", "")

	logger := zerolog.Nop()
	// Should be a no-op, no panic
	LoadContextEnv(logger)
}

func TestLoadContextEnvSetsVars(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SOCKERLESS_HOME", tmp)
	t.Setenv("SOCKERLESS_CONTEXT", "test-ctx")

	// Clear target vars
	t.Setenv("MY_VAR_A", "")
	os.Unsetenv("MY_VAR_A")
	t.Setenv("MY_VAR_B", "")
	os.Unsetenv("MY_VAR_B")

	// Create context config
	dir := filepath.Join(tmp, "contexts", "test-ctx")
	os.MkdirAll(dir, 0o755)
	cfg := contextConfig{
		Backend: "ecs",
		Env: map[string]string{
			"MY_VAR_A": "value-a",
			"MY_VAR_B": "value-b",
		},
	}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644)

	logger := zerolog.Nop()
	LoadContextEnv(logger)

	if got := os.Getenv("MY_VAR_A"); got != "value-a" {
		t.Errorf("MY_VAR_A = %q, want %q", got, "value-a")
	}
	if got := os.Getenv("MY_VAR_B"); got != "value-b" {
		t.Errorf("MY_VAR_B = %q, want %q", got, "value-b")
	}
}

func TestLoadContextEnvDoesNotOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SOCKERLESS_HOME", tmp)
	t.Setenv("SOCKERLESS_CONTEXT", "test-ctx")
	t.Setenv("MY_EXISTING_VAR", "original")

	// Create context that tries to set the same var
	dir := filepath.Join(tmp, "contexts", "test-ctx")
	os.MkdirAll(dir, 0o755)
	cfg := contextConfig{
		Backend: "ecs",
		Env: map[string]string{
			"MY_EXISTING_VAR": "from-context",
		},
	}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644)

	logger := zerolog.Nop()
	LoadContextEnv(logger)

	if got := os.Getenv("MY_EXISTING_VAR"); got != "original" {
		t.Errorf("MY_EXISTING_VAR = %q, want %q (should not override)", got, "original")
	}
}

func TestLoadContextEnvMissingFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SOCKERLESS_HOME", tmp)
	t.Setenv("SOCKERLESS_CONTEXT", "nonexistent")

	logger := zerolog.Nop()
	// Should warn but not crash
	LoadContextEnv(logger)
}

func TestLoadContextEnvInvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SOCKERLESS_HOME", tmp)
	t.Setenv("SOCKERLESS_CONTEXT", "bad-json")

	dir := filepath.Join(tmp, "contexts", "bad-json")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("{invalid json"), 0o644)

	logger := zerolog.Nop()
	// Should warn but not crash
	LoadContextEnv(logger)
}

func TestActiveContextNameFromFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SOCKERLESS_HOME", tmp)
	t.Setenv("SOCKERLESS_CONTEXT", "")

	// Write active file
	os.WriteFile(filepath.Join(tmp, "active"), []byte("my-ctx\n"), 0o644)

	name := activeContextName()
	if name != "my-ctx" {
		t.Errorf("activeContextName() = %q, want %q", name, "my-ctx")
	}
}
