package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func cmdServer(args []string) {
	if len(args) < 1 {
		serverUsage()
		os.Exit(1)
	}
	switch args[0] {
	case "start":
		serverStart(args[1:])
	case "stop":
		serverStop()
	case "restart":
		serverStop()
		time.Sleep(500 * time.Millisecond)
		serverStart(args[1:])
	default:
		serverUsage()
		os.Exit(1)
	}
}

func serverUsage() {
	fmt.Fprintln(os.Stderr, `Usage: sockerless server <subcommand>

Subcommands:
  start     Start the backend server
  stop      Stop running server
  restart   Restart server`)
}

func serverStart(args []string) {
	fs := flag.NewFlagSet("server start", flag.ExitOnError)
	backendBin := fs.String("backend-bin", "", "path to backend binary (default: sockerless-backend-{type})")
	addr := fs.String("addr", ":3375", "listen address (Docker API + management)")
	_ = fs.Parse(args)

	name := activeContextName()
	if name == "" {
		fmt.Fprintln(os.Stderr, "error: no active context; run 'sockerless context use <name>' first")
		os.Exit(1)
	}

	var cfg contextConfig

	// Try config.yaml first
	if configFileExists() {
		ucfg, err := loadConfigFile()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if env, ok := ucfg.Environments[name]; ok {
			cfg.Backend = env.Backend
			cfg.Addr = env.Addr
		} else {
			fmt.Fprintf(os.Stderr, "error: context %q not found in config.yaml\n", name)
			os.Exit(1)
		}
	} else {
		data, err := os.ReadFile(filepath.Join(contextDir(name), "config.json"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	runDir := serverRunDir(name)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	bBin := *backendBin
	if bBin == "" {
		bBin = "sockerless-backend-" + cfg.Backend
	}
	bBinPath, err := exec.LookPath(bBin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: backend binary %q not found in PATH\n", bBin)
		os.Exit(1)
	}

	cmd := exec.Command(bBinPath, "-addr", *addr)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting server: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(runDir, "backend.pid"), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write PID file: %v\n", err)
	}
	fmt.Printf("Server started (PID %d) on %s\n", cmd.Process.Pid, *addr)

	time.Sleep(1 * time.Second)
	fmt.Println("Server started. Use 'sockerless status' to verify.")
}

func serverStop() {
	name := activeContextName()
	if name == "" {
		fmt.Fprintln(os.Stderr, "error: no active context")
		os.Exit(1)
	}

	runDir := serverRunDir(name)

	pidFile := filepath.Join(runDir, "backend.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		fmt.Println("No running server found")
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		os.Remove(pidFile)
		fmt.Println("No running server found")
		return
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(pidFile)
		fmt.Println("No running server found")
		return
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not signal server (PID %d): %v\n", pid, err)
	} else {
		fmt.Printf("Sent SIGTERM to server (PID %d)\n", pid)
	}
	os.Remove(pidFile)
}

func serverRunDir(contextName string) string {
	return filepath.Join(sockerlessDir(), "run", contextName)
}
