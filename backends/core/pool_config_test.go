package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePoolsConfig_Valid(t *testing.T) {
	cfg := PoolsConfig{
		DefaultPool: "prod",
		Pools: []PoolConfig{
			{Name: "prod", BackendType: "ecs-fargate", MaxConcurrency: 10, QueueSize: 5},
			{Name: "dev", BackendType: "memory", MaxConcurrency: 0, QueueSize: 0},
		},
	}
	if err := ValidatePoolsConfig(cfg); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestValidatePoolsConfig_EmptyPools(t *testing.T) {
	cfg := PoolsConfig{DefaultPool: "default", Pools: nil}
	err := ValidatePoolsConfig(cfg)
	if err == nil {
		t.Fatal("expected error for empty pools")
	}
	if got := err.Error(); got != "pools config: at least one pool is required" {
		t.Errorf("unexpected error: %s", got)
	}
}

func TestValidatePoolsConfig_EmptyName(t *testing.T) {
	cfg := PoolsConfig{
		DefaultPool: "default",
		Pools:       []PoolConfig{{Name: "", BackendType: "memory"}},
	}
	err := ValidatePoolsConfig(cfg)
	if err == nil {
		t.Fatal("expected error for empty pool name")
	}
}

func TestValidatePoolsConfig_DuplicateNames(t *testing.T) {
	cfg := PoolsConfig{
		DefaultPool: "a",
		Pools: []PoolConfig{
			{Name: "a", BackendType: "memory"},
			{Name: "a", BackendType: "lambda"},
		},
	}
	err := ValidatePoolsConfig(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate pool names")
	}
}

func TestValidatePoolsConfig_InvalidBackendType(t *testing.T) {
	cfg := PoolsConfig{
		DefaultPool: "x",
		Pools:       []PoolConfig{{Name: "x", BackendType: "kubernetes"}},
	}
	err := ValidatePoolsConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid backend type")
	}
}

func TestValidatePoolsConfig_NegativeConcurrency(t *testing.T) {
	cfg := PoolsConfig{
		DefaultPool: "x",
		Pools:       []PoolConfig{{Name: "x", BackendType: "memory", MaxConcurrency: -1}},
	}
	err := ValidatePoolsConfig(cfg)
	if err == nil {
		t.Fatal("expected error for negative max_concurrency")
	}
}

func TestValidatePoolsConfig_NegativeQueueSize(t *testing.T) {
	cfg := PoolsConfig{
		DefaultPool: "x",
		Pools:       []PoolConfig{{Name: "x", BackendType: "memory", QueueSize: -1}},
	}
	err := ValidatePoolsConfig(cfg)
	if err == nil {
		t.Fatal("expected error for negative queue_size")
	}
}

func TestValidatePoolsConfig_MissingDefault(t *testing.T) {
	cfg := PoolsConfig{
		DefaultPool: "",
		Pools:       []PoolConfig{{Name: "a", BackendType: "memory"}},
	}
	err := ValidatePoolsConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing default_pool")
	}
}

func TestValidatePoolsConfig_DefaultNotInPools(t *testing.T) {
	cfg := PoolsConfig{
		DefaultPool: "nonexistent",
		Pools:       []PoolConfig{{Name: "a", BackendType: "memory"}},
	}
	err := ValidatePoolsConfig(cfg)
	if err == nil {
		t.Fatal("expected error for default_pool not matching any pool")
	}
}

func TestDefaultPoolsConfig(t *testing.T) {
	cfg := DefaultPoolsConfig()

	if err := ValidatePoolsConfig(cfg); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
	if cfg.DefaultPool != "default" {
		t.Errorf("DefaultPool = %q, want %q", cfg.DefaultPool, "default")
	}
	if len(cfg.Pools) != 1 {
		t.Fatalf("expected 1 pool, got %d", len(cfg.Pools))
	}
	p := cfg.Pools[0]
	if p.Name != "default" {
		t.Errorf("pool name = %q, want %q", p.Name, "default")
	}
	if p.BackendType != "memory" {
		t.Errorf("backend_type = %q, want %q", p.BackendType, "memory")
	}
	if p.MaxConcurrency != 0 {
		t.Errorf("max_concurrency = %d, want 0", p.MaxConcurrency)
	}
	if p.QueueSize != 0 {
		t.Errorf("queue_size = %d, want 0", p.QueueSize)
	}
}

func writePoolsJSON(t *testing.T, dir string, cfg PoolsConfig) string {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "pools.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadPoolsConfig_FromEnv(t *testing.T) {
	tmp := t.TempDir()
	cfg := PoolsConfig{
		DefaultPool: "env-pool",
		Pools:       []PoolConfig{{Name: "env-pool", BackendType: "lambda", MaxConcurrency: 5}},
	}
	path := writePoolsJSON(t, tmp, cfg)
	t.Setenv("SOCKERLESS_POOLS_CONFIG", path)
	t.Setenv("SOCKERLESS_HOME", t.TempDir()) // different dir, no pools.json

	got, err := LoadPoolsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.DefaultPool != "env-pool" {
		t.Errorf("DefaultPool = %q, want %q", got.DefaultPool, "env-pool")
	}
	if len(got.Pools) != 1 || got.Pools[0].BackendType != "lambda" {
		t.Errorf("unexpected pools: %+v", got.Pools)
	}
}

func TestLoadPoolsConfig_FromHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SOCKERLESS_HOME", home)
	t.Setenv("SOCKERLESS_POOLS_CONFIG", "")

	cfg := PoolsConfig{
		DefaultPool: "home-pool",
		Pools:       []PoolConfig{{Name: "home-pool", BackendType: "aca-jobs"}},
	}
	writePoolsJSON(t, home, cfg)

	got, err := LoadPoolsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.DefaultPool != "home-pool" {
		t.Errorf("DefaultPool = %q, want %q", got.DefaultPool, "home-pool")
	}
}

func TestLoadPoolsConfig_FallbackDefault(t *testing.T) {
	t.Setenv("SOCKERLESS_POOLS_CONFIG", "")
	t.Setenv("SOCKERLESS_HOME", t.TempDir())

	got, err := LoadPoolsConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.DefaultPool != "default" {
		t.Errorf("DefaultPool = %q, want %q", got.DefaultPool, "default")
	}
	if len(got.Pools) != 1 || got.Pools[0].BackendType != "memory" {
		t.Errorf("expected default single-pool config, got %+v", got.Pools)
	}
}

func TestLoadPoolsConfig_EnvFileMissing(t *testing.T) {
	t.Setenv("SOCKERLESS_POOLS_CONFIG", "/nonexistent/pools.json")

	_, err := LoadPoolsConfig()
	if err == nil {
		t.Fatal("expected error for missing env file")
	}
}

func TestLoadPoolsConfig_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "pools.json")
	os.WriteFile(path, []byte("{not valid json"), 0o644)
	t.Setenv("SOCKERLESS_POOLS_CONFIG", path)

	_, err := LoadPoolsConfig()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadPoolsConfig_InvalidConfig(t *testing.T) {
	tmp := t.TempDir()
	// Valid JSON but fails validation (empty pools)
	cfg := PoolsConfig{DefaultPool: "x", Pools: nil}
	writePoolsJSON(t, tmp, cfg)
	t.Setenv("SOCKERLESS_POOLS_CONFIG", filepath.Join(tmp, "pools.json"))

	_, err := LoadPoolsConfig()
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

func TestPoolsConfig_GetPool(t *testing.T) {
	cfg := PoolsConfig{
		DefaultPool: "a",
		Pools: []PoolConfig{
			{Name: "a", BackendType: "memory"},
			{Name: "b", BackendType: "lambda"},
		},
	}

	p := cfg.GetPool("a")
	if p == nil {
		t.Fatal("expected pool 'a' to be found")
	}
	if p.BackendType != "memory" {
		t.Errorf("pool 'a' backend_type = %q, want %q", p.BackendType, "memory")
	}

	p = cfg.GetPool("b")
	if p == nil {
		t.Fatal("expected pool 'b' to be found")
	}
	if p.BackendType != "lambda" {
		t.Errorf("pool 'b' backend_type = %q, want %q", p.BackendType, "lambda")
	}

	p = cfg.GetPool("nonexistent")
	if p != nil {
		t.Errorf("expected nil for nonexistent pool, got %+v", p)
	}
}

func TestPoolsConfig_PoolNames(t *testing.T) {
	cfg := PoolsConfig{
		DefaultPool: "a",
		Pools: []PoolConfig{
			{Name: "a", BackendType: "memory"},
			{Name: "b", BackendType: "lambda"},
			{Name: "c", BackendType: "ecs-fargate"},
		},
	}

	names := cfg.PoolNames()
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}
	expected := []string{"a", "b", "c"}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("names[%d] = %q, want %q", i, names[i], want)
		}
	}
}
