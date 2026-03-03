package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// adminConfig is the structure of admin.json.
type adminConfig struct {
	Components []struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Addr string `json:"addr"`
	} `json:"components"`
	Processes []ProcessConfig `json:"processes,omitempty"`
}

// ProcessConfig defines a managed process in admin.json.
type ProcessConfig struct {
	Name   string            `json:"name"`
	Binary string            `json:"binary"`
	Args   []string          `json:"args"`
	Env    map[string]string `json:"env"`
	Addr   string            `json:"addr"`
	Type   string            `json:"type"` // backend, frontend, simulator, coordinator
}

// contextConfig mirrors the CLI context config structure.
type contextConfig struct {
	Backend      string            `json:"backend"`
	FrontendAddr string            `json:"frontend_addr,omitempty"`
	BackendAddr  string            `json:"backend_addr,omitempty"`
	Env          map[string]string `json:"env"`
}

// sockerlessDir returns the sockerless configuration directory.
func sockerlessDir() string {
	if d := os.Getenv("SOCKERLESS_HOME"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".sockerless")
	}
	return filepath.Join(home, ".sockerless")
}

// loadConfigFile loads components from an admin.json config file.
func loadConfigFile(reg *Registry, path string) (*adminConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg adminConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}

	for _, c := range cfg.Components {
		reg.Add(Component{
			Name: c.Name,
			Type: c.Type,
			Addr: normalizeAddr(c.Addr),
		})
	}
	return &cfg, nil
}

// discoverFromContexts scans ~/.sockerless/contexts/ for component addresses.
func discoverFromContexts(reg *Registry) {
	contextsDir := filepath.Join(sockerlessDir(), "contexts")
	entries, err := os.ReadDir(contextsDir)
	if err != nil {
		return // no contexts directory — not an error
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		data, err := os.ReadFile(filepath.Join(contextsDir, name, "config.json"))
		if err != nil {
			continue
		}
		var cfg contextConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}

		if cfg.BackendAddr != "" {
			compName := cfg.Backend
			if compName == "" {
				compName = name
			}
			reg.Add(Component{
				Name: compName,
				Type: "backend",
				Addr: normalizeAddr(cfg.BackendAddr),
			})
		}
		if cfg.FrontendAddr != "" {
			reg.Add(Component{
				Name: "frontend",
				Type: "frontend",
				Addr: normalizeAddr(cfg.FrontendAddr),
			})
		}
	}

	// Also check for admin.json in the sockerless dir
	adminPath := filepath.Join(sockerlessDir(), "admin.json")
	if _, err := os.Stat(adminPath); err == nil {
		if _, err := loadConfigFile(reg, adminPath); err != nil {
			log.Printf("warning: failed to load %s: %v", adminPath, err)
		}
	}
}

// listContexts returns all CLI contexts with their configs.
func listContexts() []map[string]any {
	contextsDir := filepath.Join(sockerlessDir(), "contexts")
	entries, err := os.ReadDir(contextsDir)
	if err != nil {
		return nil
	}

	activeCtx := activeContextName()
	var contexts []map[string]any
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		ctx := map[string]any{
			"name":   name,
			"active": name == activeCtx,
		}

		data, err := os.ReadFile(filepath.Join(contextsDir, name, "config.json"))
		if err == nil {
			var cfg contextConfig
			if json.Unmarshal(data, &cfg) == nil {
				ctx["backend"] = cfg.Backend
				ctx["frontend_addr"] = cfg.FrontendAddr
				ctx["backend_addr"] = cfg.BackendAddr
			}
		}
		contexts = append(contexts, ctx)
	}
	return contexts
}

// activeContextName returns the active context name.
func activeContextName() string {
	if name := os.Getenv("SOCKERLESS_CONTEXT"); name != "" {
		return name
	}
	data, err := os.ReadFile(filepath.Join(sockerlessDir(), "active"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// decodeJSON decodes JSON from a reader into v.
func decodeJSON(r io.Reader, v any) error {
	return json.NewDecoder(r).Decode(v)
}
