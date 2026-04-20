package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type contextConfig struct {
	Backend string            `json:"backend"`
	Addr    string            `json:"addr,omitempty"`
	Env     map[string]string `json:"env"`
}

// multiFlag collects repeated --set KEY=VALUE flags.
type multiFlag []string

func (f *multiFlag) String() string { return strings.Join(*f, ", ") }
func (f *multiFlag) Set(v string) error {
	*f = append(*f, v)
	return nil
}

func contextCreate(args []string) {
	fs := flag.NewFlagSet("context create", flag.ExitOnError)
	backend := fs.String("backend", "", "backend type (required)")
	addr := fs.String("addr", "", "server address (e.g. :3375)")
	simulator := fs.String("simulator", "", "simulator name (from config.yaml simulators)")
	var sets multiFlag
	fs.Var(&sets, "set", "set env var as KEY=VALUE (repeatable)")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: sockerless context create <name> --backend <type> [--addr ADDR] [--simulator SIM] [--set KEY=VALUE ...]")
		os.Exit(1)
	}
	name := fs.Arg(0)

	if *backend == "" {
		fmt.Fprintln(os.Stderr, "error: --backend is required")
		os.Exit(1)
	}

	// If config.yaml exists, add to it
	if configFileExists() {
		cfg, err := loadConfigFile()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
			os.Exit(1)
		}
		env := &environment{
			Backend:   *backend,
			Addr:      *addr,
			Simulator: *simulator,
		}
		cfg.Environments[name] = env
		if err := saveConfigFile(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Context %q created (backend: %s)\n", name, *backend)
		return
	}

	// Fallback: old JSON context
	env := make(map[string]string)
	for _, s := range sets {
		k, v, ok := strings.Cut(s, "=")
		if !ok {
			fmt.Fprintf(os.Stderr, "error: invalid --set value %q (expected KEY=VALUE)\n", s)
			os.Exit(1)
		}
		env[k] = v
	}

	cfg := contextConfig{
		Backend: *backend,
		Addr:    *addr,
		Env:     env,
	}

	dir := contextDir(name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Context %q created (backend: %s)\n", name, *backend)
}

func contextList() {
	// Try config.yaml first
	if configFileExists() {
		cfg, err := loadConfigFile()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(cfg.Environments) == 0 {
			fmt.Println("No contexts configured.")
			return
		}
		active := activeContextName()
		names := make([]string, 0, len(cfg.Environments))
		for n := range cfg.Environments {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			marker := "  "
			if name == active {
				marker = "* "
			}
			env := cfg.Environments[name]
			extra := env.Backend
			if env.Simulator != "" {
				extra += ", sim:" + env.Simulator
			}
			fmt.Printf("%s%-20s (%s)\n", marker, name, extra)
		}
		return
	}

	// Fallback: old JSON contexts
	contextsDir := filepath.Join(sockerlessDir(), "contexts")
	entries, err := os.ReadDir(contextsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No contexts configured.")
			return
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	active := activeContextName()
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	if len(names) == 0 {
		fmt.Println("No contexts configured.")
		return
	}

	for _, name := range names {
		marker := "  "
		if name == active {
			marker = "* "
		}
		data, err := os.ReadFile(filepath.Join(contextsDir, name, "config.json"))
		if err != nil {
			fmt.Printf("%s%s\n", marker, name)
			continue
		}
		var cfg contextConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			fmt.Printf("%s%s\n", marker, name)
			continue
		}
		fmt.Printf("%s%-20s (%s)\n", marker, name, cfg.Backend)
	}
}

func contextShow(args []string) {
	if len(args) < 1 {
		name := activeContextName()
		if name == "" {
			fmt.Fprintln(os.Stderr, "Usage: sockerless context show <name>")
			os.Exit(1)
		}
		args = []string{name}
	}
	name := args[0]

	// Try config.yaml first
	if configFileExists() {
		cfg, err := loadConfigFile()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		env, ok := cfg.Environments[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "error: context %q not found\n", name)
			os.Exit(1)
		}
		fmt.Printf("Context:   %s\n", name)
		fmt.Printf("Backend:   %s\n", env.Backend)
		if env.Addr != "" {
			fmt.Printf("Address:   %s\n", env.Addr)
		}
		if env.LogLevel != "" {
			fmt.Printf("Log Level: %s\n", env.LogLevel)
		}
		if env.Simulator != "" {
			fmt.Printf("Simulator: %s\n", env.Simulator)
		}
		if env.AWS != nil {
			fmt.Printf("AWS Region: %s\n", env.AWS.Region)
		}
		if env.GCP != nil {
			fmt.Printf("GCP Project: %s\n", env.GCP.Project)
		}
		if env.Azure != nil {
			fmt.Printf("Azure Subscription: %s\n", env.Azure.SubscriptionID)
		}
		if env.Common.AgentImage != "" {
			fmt.Printf("Agent Image: %s\n", env.Common.AgentImage)
		}
		if env.Common.PollInterval != "" {
			fmt.Printf("Poll Interval: %s\n", env.Common.PollInterval)
		}
		return
	}

	// Fallback: old JSON context
	data, err := os.ReadFile(filepath.Join(contextDir(name), "config.json"))
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "error: context %q not found\n", name)
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}

	var cfg contextConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Context: %s\n", name)
	fmt.Printf("Backend: %s\n", cfg.Backend)
	if cfg.Addr != "" {
		fmt.Printf("Address: %s\n", cfg.Addr)
	}
	if len(cfg.Env) > 0 {
		fmt.Println("Environment:")
		keys := make([]string, 0, len(cfg.Env))
		for k := range cfg.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %s=%s\n", k, cfg.Env[k])
		}
	}
}

func contextUse(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: sockerless context use <name>")
		os.Exit(1)
	}
	name := args[0]

	// Verify context exists in config.yaml or old JSON
	found := false
	if configFileExists() {
		cfg, err := loadConfigFile()
		if err == nil {
			if _, ok := cfg.Environments[name]; ok {
				found = true
			}
		}
	}
	if !found {
		if _, err := os.Stat(filepath.Join(contextDir(name), "config.json")); err != nil {
			fmt.Fprintf(os.Stderr, "error: context %q not found\n", name)
			os.Exit(1)
		}
	}

	dir := sockerlessDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(activeFile(), []byte(name+"\n"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Switched to context %q\n", name)
}

func contextDelete(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: sockerless context delete <name>")
		os.Exit(1)
	}
	name := args[0]

	// Try config.yaml first
	if configFileExists() {
		cfg, err := loadConfigFile()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if _, ok := cfg.Environments[name]; ok {
			delete(cfg.Environments, name)
			if err := saveConfigFile(cfg); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			if activeContextName() == name {
				os.Remove(activeFile())
			}
			fmt.Printf("Context %q deleted\n", name)
			return
		}
	}

	// Fallback: old JSON context
	dir := contextDir(name)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "error: context %q not found\n", name)
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}

	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if activeContextName() == name {
		os.Remove(activeFile())
	}

	fmt.Printf("Context %q deleted\n", name)
}

func contextCurrent() {
	name := activeContextName()
	if name == "" {
		fmt.Println("No active context")
		return
	}
	fmt.Println(name)
}

func contextReload() {
	addr := activeAddr()
	if addr == "" {
		fmt.Fprintln(os.Stderr, "error: no server address configured in active context")
		os.Exit(1)
	}

	data, err := mgmtPost(addr, "/internal/v1/reload")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Reload failed: %v\n", err)
		os.Exit(1)
	}
	var resp map[string]any
	if err := json.Unmarshal(data, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not parse reload response: %v\n", err)
	}
	changed, _ := resp["changed"].(float64)
	fmt.Printf("Reloaded (%d vars changed)\n", int(changed))
}
