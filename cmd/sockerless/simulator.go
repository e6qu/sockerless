package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
)

func cmdSimulator(args []string) {
	if len(args) < 1 {
		simulatorUsage()
		os.Exit(1)
	}
	switch args[0] {
	case "list", "ls":
		simulatorList()
	case "add":
		simulatorAdd(args[1:])
	case "remove", "rm":
		simulatorRemove(args[1:])
	default:
		simulatorUsage()
		os.Exit(1)
	}
}

func simulatorUsage() {
	fmt.Fprintln(os.Stderr, `Usage: sockerless simulator <subcommand>

Subcommands:
  list     List configured simulators
  add      Add a simulator
  remove   Remove a simulator`)
}

func simulatorList() {
	if !configFileExists() {
		fmt.Println("No config.yaml found. Simulators require config.yaml.")
		return
	}
	cfg, err := loadConfigFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(cfg.Simulators) == 0 {
		fmt.Println("No simulators configured.")
		return
	}
	names := make([]string, 0, len(cfg.Simulators))
	for n := range cfg.Simulators {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		sim := cfg.Simulators[name]
		portStr := "auto"
		if sim.Port > 0 {
			portStr = fmt.Sprintf(":%d", sim.Port)
		}
		fmt.Printf("  %-20s (cloud: %s, port: %s)\n", name, sim.Cloud, portStr)
	}
}

func simulatorAdd(args []string) {
	fs := flag.NewFlagSet("simulator add", flag.ExitOnError)
	cloud := fs.String("cloud", "", "cloud type: aws, gcp, azure (required)")
	port := fs.Int("port", 0, "listen port (0 = auto)")
	grpcPort := fs.Int("grpc-port", 0, "gRPC port (GCP only)")
	logLevel := fs.String("log-level", "", "log level")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: sockerless simulator add <name> --cloud <type> [--port PORT] [--grpc-port PORT] [--log-level LEVEL]")
		os.Exit(1)
	}
	name := fs.Arg(0)

	if *cloud == "" {
		fmt.Fprintln(os.Stderr, "error: --cloud is required")
		os.Exit(1)
	}
	switch *cloud {
	case "aws", "gcp", "azure":
	default:
		fmt.Fprintf(os.Stderr, "error: invalid cloud %q (must be aws, gcp, or azure)\n", *cloud)
		os.Exit(1)
	}

	cfg, err := loadOrCreateConfigFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	cfg.Simulators[name] = &simulatorConfig{
		Cloud:    *cloud,
		Port:     *port,
		GRPCPort: *grpcPort,
		LogLevel: *logLevel,
	}

	if err := saveConfigFile(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Simulator %q added (cloud: %s)\n", name, *cloud)
}

func simulatorRemove(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: sockerless simulator remove <name>")
		os.Exit(1)
	}
	name := args[0]

	if !configFileExists() {
		fmt.Fprintf(os.Stderr, "error: no config.yaml found\n")
		os.Exit(1)
	}
	cfg, err := loadConfigFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if _, ok := cfg.Simulators[name]; !ok {
		fmt.Fprintf(os.Stderr, "error: simulator %q not found\n", name)
		os.Exit(1)
	}

	// Check references
	for envName, env := range cfg.Environments {
		if env.Simulator == name {
			fmt.Fprintf(os.Stderr, "error: simulator %q is referenced by environment %q\n", name, envName)
			os.Exit(1)
		}
	}

	delete(cfg.Simulators, name)
	if err := saveConfigFile(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Simulator %q removed\n", name)
}

// loadOrCreateConfigFile loads config.yaml or creates an empty one.
func loadOrCreateConfigFile() (*unifiedConfig, error) {
	if configFileExists() {
		return loadConfigFile()
	}
	return &unifiedConfig{
		Simulators:   make(map[string]*simulatorConfig),
		Environments: make(map[string]*environment),
	}, nil
}
