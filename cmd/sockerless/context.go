package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

// contextConfig is the on-disk shape `sockerless config migrate` reads from
// the deprecated `~/.sockerless/contexts/<name>/config.json` layout to fold
// into `config.yaml`. The runtime no longer consults this shape directly.
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

func requireConfigFile() *unifiedConfig {
	if !configFileExists() {
		fmt.Fprintln(os.Stderr, "error: no config.yaml present. Run `sockerless config init` or `sockerless config migrate` first.")
		os.Exit(1)
	}
	cfg, err := loadConfigFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}
	return cfg
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

	cfg := requireConfigFile()
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
}

func contextList() {
	cfg := requireConfigFile()
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

	cfg := requireConfigFile()
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
}

func contextUse(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: sockerless context use <name>")
		os.Exit(1)
	}
	name := args[0]

	cfg := requireConfigFile()
	if _, ok := cfg.Environments[name]; !ok {
		fmt.Fprintf(os.Stderr, "error: context %q not found\n", name)
		os.Exit(1)
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

	cfg := requireConfigFile()
	if _, ok := cfg.Environments[name]; !ok {
		fmt.Fprintf(os.Stderr, "error: context %q not found\n", name)
		os.Exit(1)
	}
	delete(cfg.Environments, name)
	if err := saveConfigFile(cfg); err != nil {
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
