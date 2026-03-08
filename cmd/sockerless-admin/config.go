package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
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
	Backend string            `json:"backend"`
	Addr    string            `json:"addr,omitempty"`
	Env     map[string]string `json:"env"`
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

// discoverFromContexts scans config.yaml or ~/.sockerless/contexts/ for component addresses.
func discoverFromContexts(reg *Registry) {
	// Try config.yaml first
	if discoverFromYAMLConfig(reg) {
		// Also check for admin.json in the sockerless dir
		adminPath := filepath.Join(sockerlessDir(), "admin.json")
		if _, err := os.Stat(adminPath); err == nil {
			if _, err := loadConfigFile(reg, adminPath); err != nil {
				log.Printf("warning: failed to load %s: %v", adminPath, err)
			}
		}
		return
	}

	// Fallback: old JSON contexts
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

		if cfg.Addr != "" {
			compName := cfg.Backend
			if compName == "" {
				compName = name
			}
			reg.Add(Component{
				Name: compName,
				Type: "backend",
				Addr: normalizeAddr(cfg.Addr),
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
	// Try config.yaml first
	if results := listContextsFromYAML(); results != nil {
		return results
	}

	// Fallback: old JSON contexts
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
				ctx["addr"] = cfg.Addr
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

// --- Unified config.yaml support ---

type yamlConfig struct {
	Simulators   map[string]*yamlSimulator   `yaml:"simulators,omitempty"`
	Environments map[string]*yamlEnvironment `yaml:"environments"`
}

type yamlSimulator struct {
	Cloud    string `yaml:"cloud"`
	Port     int    `yaml:"port,omitempty"`
	GRPCPort int    `yaml:"grpc_port,omitempty"`
	LogLevel string `yaml:"log_level,omitempty"`
}

type yamlEnvironment struct {
	Backend   string     `yaml:"backend"`
	Addr      string     `yaml:"addr,omitempty"`
	LogLevel  string     `yaml:"log_level,omitempty"`
	Simulator string     `yaml:"simulator,omitempty"`
	Common    yamlCommon `yaml:"common,omitempty"`
}

type yamlCommon struct {
	AgentImage   string `yaml:"agent_image,omitempty"`
	AgentToken   string `yaml:"agent_token,omitempty"`
	CallbackURL  string `yaml:"callback_url,omitempty"`
	EndpointURL  string `yaml:"endpoint_url,omitempty"`
	PollInterval string `yaml:"poll_interval,omitempty"`
	AgentTimeout string `yaml:"agent_timeout,omitempty"`
}

func configFilePath() string {
	if p := os.Getenv("SOCKERLESS_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(sockerlessDir(), "config.yaml")
}

func loadYAMLConfig() (*yamlConfig, error) {
	data, err := os.ReadFile(configFilePath())
	if err != nil {
		return nil, err
	}
	var cfg yamlConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config.yaml: %w", err)
	}
	if cfg.Environments == nil {
		cfg.Environments = make(map[string]*yamlEnvironment)
	}
	if cfg.Simulators == nil {
		cfg.Simulators = make(map[string]*yamlSimulator)
	}
	return &cfg, nil
}

// discoverFromYAMLConfig reads config.yaml and registers environments as backend
// components and simulators as simulator components.
func discoverFromYAMLConfig(reg *Registry) bool {
	cfg, err := loadYAMLConfig()
	if err != nil {
		return false
	}

	for name, sim := range cfg.Simulators {
		if sim.Port > 0 {
			reg.Add(Component{
				Name: name,
				Type: "simulator",
				Addr: normalizeAddr(fmt.Sprintf(":%d", sim.Port)),
			})
		}
	}

	for name, env := range cfg.Environments {
		if env.Addr != "" {
			reg.Add(Component{
				Name: name,
				Type: "backend",
				Addr: normalizeAddr(env.Addr),
			})
		}
	}

	return true
}

// listContextsFromYAML returns contexts from config.yaml.
func listContextsFromYAML() []map[string]any {
	cfg, err := loadYAMLConfig()
	if err != nil {
		return nil
	}

	activeCtx := activeContextName()
	var contexts []map[string]any
	for name, env := range cfg.Environments {
		ctx := map[string]any{
			"name":    name,
			"active":  name == activeCtx,
			"backend": env.Backend,
			"addr":    env.Addr,
		}
		if env.Simulator != "" {
			ctx["simulator"] = env.Simulator
		}
		contexts = append(contexts, ctx)
	}
	return contexts
}
