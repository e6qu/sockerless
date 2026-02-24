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
	Backend      string            `json:"backend"`
	FrontendAddr string            `json:"frontend_addr,omitempty"`
	BackendAddr  string            `json:"backend_addr,omitempty"`
	Env          map[string]string `json:"env"`
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
	frontendAddr := fs.String("frontend-addr", "", "frontend management API address (e.g. http://localhost:9080)")
	backendAddr := fs.String("backend-addr", "", "backend API address (e.g. http://localhost:9100)")
	var sets multiFlag
	fs.Var(&sets, "set", "set env var as KEY=VALUE (repeatable)")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: sockerless context create <name> --backend <type> [--frontend-addr ADDR] [--backend-addr ADDR] [--set KEY=VALUE ...]")
		os.Exit(1)
	}
	name := fs.Arg(0)

	if *backend == "" {
		fmt.Fprintln(os.Stderr, "error: --backend is required")
		os.Exit(1)
	}

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
		Backend:      *backend,
		FrontendAddr: *frontendAddr,
		BackendAddr:  *backendAddr,
		Env:          env,
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
		// Read backend type for display
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
		// Show active context if no name given
		name := activeContextName()
		if name == "" {
			fmt.Fprintln(os.Stderr, "Usage: sockerless context show <name>")
			os.Exit(1)
		}
		args = []string{name}
	}
	name := args[0]

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
	if cfg.FrontendAddr != "" {
		fmt.Printf("Frontend: %s\n", cfg.FrontendAddr)
	}
	if cfg.BackendAddr != "" {
		fmt.Printf("Backend Addr: %s\n", cfg.BackendAddr)
	}
	if len(cfg.Env) > 0 {
		fmt.Println("Environment:")
		// Sort keys for stable output
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

	// Verify context exists
	if _, err := os.Stat(filepath.Join(contextDir(name), "config.json")); err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "error: context %q not found\n", name)
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
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

	// If this was the active context, clear the active file
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
	frontendAddr, backendAddr := activeAddrs()
	if frontendAddr == "" && backendAddr == "" {
		fmt.Fprintln(os.Stderr, "error: no server addresses configured in active context")
		os.Exit(1)
	}

	reloaded := 0
	if backendAddr != "" {
		data, err := mgmtPost(backendAddr, "/internal/v1/reload")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Backend reload failed: %v\n", err)
		} else {
			var resp map[string]any
			if err := json.Unmarshal(data, &resp); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not parse reload response: %v\n", err)
			}
			changed, _ := resp["changed"].(float64)
			fmt.Printf("Backend reloaded (%d vars changed)\n", int(changed))
			reloaded++
		}
	}

	if frontendAddr != "" {
		_, err := mgmtPost(frontendAddr, "/reload")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Frontend reload failed: %v\n", err)
		} else {
			fmt.Println("Frontend reloaded")
			reloaded++
		}
	}

	if reloaded == 0 {
		os.Exit(1)
	}
}
