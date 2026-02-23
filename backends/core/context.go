package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
)

// LoadContextEnv loads the active context's env vars into the process environment.
// Env vars already set take precedence (not overwritten).
// Does nothing if no active context is configured.
func LoadContextEnv(logger zerolog.Logger) {
	name := activeContextName()
	if name == "" {
		return
	}

	path := contextConfigPath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		logger.Warn().Str("context", name).Err(err).Msg("failed to read context config")
		return
	}

	var cfg contextConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		logger.Warn().Str("context", name).Err(err).Msg("failed to parse context config")
		return
	}

	applied := 0
	for k, v := range cfg.Env {
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
			applied++
		}
	}
	logger.Info().Str("context", name).Int("applied", applied).Msg("loaded context config")
}

type contextConfig struct {
	Backend      string            `json:"backend"`
	FrontendAddr string            `json:"frontend_addr,omitempty"`
	BackendAddr  string            `json:"backend_addr,omitempty"`
	Env          map[string]string `json:"env"`
}

func activeContextName() string {
	if name := os.Getenv("SOCKERLESS_CONTEXT"); name != "" {
		return name
	}
	data, err := os.ReadFile(activeFilePath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func sockerlessHomeDir() string {
	if d := os.Getenv("SOCKERLESS_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sockerless")
}

func activeFilePath() string {
	return filepath.Join(sockerlessHomeDir(), "active")
}

func contextConfigPath(name string) string {
	return filepath.Join(sockerlessHomeDir(), "contexts", name, "config.json")
}
